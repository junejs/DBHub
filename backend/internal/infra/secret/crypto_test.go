package secret

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// masterKeyB64 is a deterministic 32-byte test master key, base64-encoded.
// Generated once; safe to commit because it never leaves tests.
const masterKeyB64 = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

// mustMasterKey loads a MasterKey from the test constant, failing the test on
// any error. Used to keep table-driven cases compact.
func mustMasterKey(tb testing.TB, b64 string) *MasterKey {
	tb.Helper()
	mk, err := LoadMasterKey(b64)
	require.NoError(tb, err, "load master key")
	return mk
}

// mustCrypto builds a Crypto from the test master key.
func mustCrypto(tb testing.TB) *Crypto {
	tb.Helper()
	c, err := New(mustMasterKey(tb, masterKeyB64))
	require.NoError(tb, err, "build crypto")
	return c
}

func TestLoadMasterKey_Success(t *testing.T) {
	mk := mustMasterKey(t, masterKeyB64)
	assert.False(t, mk.IsZero(), "loaded key must not be all-zero")
	assert.Equal(t, Redacted, mk.String(), "String() must redact")
	assert.Equal(t, Redacted, mk.GoString(), "GoString() must redact")
}

func TestLoadMasterKey_Table(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty string", "", ErrMasterKeyEmpty},
		{"whitespace-only is not trimmed (rejected as malformed base64)", "    ", ErrMasterKeyMalformed},
		{"not base64 at all", "not-base64!!", ErrMasterKeyMalformed},
		{"16 bytes (too short)", base64.StdEncoding.EncodeToString(make([]byte, 16)), ErrMasterKeyWrongLength},
		{"64 bytes (too long)", base64.StdEncoding.EncodeToString(make([]byte, 64)), ErrMasterKeyWrongLength},
		{"31 bytes (off by one)", base64.StdEncoding.EncodeToString(make([]byte, 31)), ErrMasterKeyWrongLength},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadMasterKey(c.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, c.wantErr, "expected specific sentinel")
		})
	}
}

func TestMasterKey_Equal_ConstantTime(t *testing.T) {
	a := mustMasterKey(t, masterKeyB64)
	b := mustMasterKey(t, masterKeyB64)
	otherB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xAB}, KeyLen))
	other := mustMasterKey(t, otherB64)

	assert.True(t, a.Equal(b), "same key must compare equal")
	assert.False(t, a.Equal(other), "different keys must compare unequal")
	assert.False(t, a.Equal(nil), "non-nil vs nil must compare unequal")
	assert.True(t, (*MasterKey)(nil).Equal(nil), "both nil must compare equal")
}

func TestMasterKey_String_NeverLeaks(t *testing.T) {
	mk := mustMasterKey(t, masterKeyB64)
	// String/GoString/Format must all return the redacted sentinel.
	assert.Equal(t, Redacted, mk.String())
	assert.Equal(t, Redacted, mk.GoString())

	// Defensive: ensure the redacted form does not accidentally contain
	// any byte from the raw key.
	raw, _ := base64.StdEncoding.DecodeString(masterKeyB64)
	for _, b := range raw {
		// Skip zero bytes — they'd appear in any all-zero string.
		if b == 0 {
			continue
		}
		assert.NotContains(t, mk.String(), string(rune(b)),
			"redacted form must not contain raw key byte 0x%02x", b)
	}
}

func TestCrypto_EncryptDecrypt_RoundTrip(t *testing.T) {
	c := mustCrypto(t)
	cases := []struct {
		name      string
		plaintext []byte
	}{
		{"short ascii", []byte("p@ssw0rd")},
		{"one byte", []byte{0x01}},
		{"256 bytes (cover >1 block)", bytes.Repeat([]byte("x"), 256)},
		{"unicode", []byte("数据库密码")},
		{"binary with zero bytes", []byte{0x00, 0x01, 0x02, 0x00, 0xFF}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			blob, err := c.Encrypt(tc.plaintext)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(blob), MinCiphertextLen)

			s, err := c.Decrypt(blob)
			require.NoError(t, err)
			defer s.Zero()

			assert.True(t, bytes.Equal(s.Reveal(), tc.plaintext),
				"round-trip mismatch: got %x want %x", s.Reveal(), tc.plaintext)
		})
	}
}

func TestCrypto_Encrypt_SamePlaintextDifferentCiphertext(t *testing.T) {
	c := mustCrypto(t)
	pt := []byte("same-secret")

	blob1, err := c.Encrypt(pt)
	require.NoError(t, err)
	blob2, err := c.Encrypt(pt)
	require.NoError(t, err)
	blob3, err := c.Encrypt(pt)
	require.NoError(t, err)

	assert.False(t, bytes.Equal(blob1, blob2), "two encrypts of same pt must differ (random nonce)")
	assert.False(t, bytes.Equal(blob1, blob3), "third encrypt must also differ")
	assert.False(t, bytes.Equal(blob2, blob3), "third pair must also differ")

	// All three must still decrypt back to the original.
	for i, b := range [][]byte{blob1, blob2, blob3} {
		s, err := c.Decrypt(b)
		require.NoError(t, err, "blob %d failed to decrypt", i)
		assert.True(t, bytes.Equal(s.Reveal(), pt), "blob %d round-trip mismatch", i)
		s.Zero()
	}
}

func TestCrypto_Encrypt_RejectsEmptyPlaintext(t *testing.T) {
	c := mustCrypto(t)
	_, err := c.Encrypt(nil)
	assert.ErrorIs(t, err, ErrEmptyPlaintext)
	_, err = c.Encrypt([]byte{})
	assert.ErrorIs(t, err, ErrEmptyPlaintext)
}

func TestCrypto_Decrypt_Table(t *testing.T) {
	c := mustCrypto(t)
	validBlob, err := c.Encrypt([]byte("hello"))
	require.NoError(t, err)

	// Truncate a real blob to the boundary length so we can probe the off-by-one.
	short := make([]byte, MinCiphertextLen-1)

	// Unknown version: copy a real blob and bump the version byte.
	unknownVersion := append([]byte{}, validBlob...)
	unknownVersion[0] = 0xFF

	// Tampered ciphertext: flip a byte in the ciphertext body (the tag is the
	// trailing 16 bytes, so flipping the last byte flips a tag bit).
	tampered := append([]byte{}, validBlob...)
	tampered[len(tampered)-1] ^= 0x01

	// Tampered nonce: flip a byte in the nonce.
	tamperedNonce := append([]byte{}, validBlob...)
	tamperedNonce[VersionLen+1] ^= 0x01

	cases := []struct {
		name    string
		input   []byte
		wantErr error
	}{
		{"nil", nil, ErrCiphertextTooShort},
		{"empty", []byte{}, ErrCiphertextTooShort},
		{"one byte (just version)", []byte{CiphertextVersionV1}, ErrCiphertextTooShort},
		{"min-1 bytes (boundary)", short, ErrCiphertextTooShort},
		{"unknown version 0xFF", unknownVersion, ErrUnknownCiphertextVersion},
		{"flipped trailing byte (auth fail)", tampered, ErrTamperedCiphertext},
		{"flipped nonce byte (auth fail)", tamperedNonce, ErrTamperedCiphertext},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Decrypt(tc.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr, "expected %v, got %v", tc.wantErr, err)
		})
	}
}

func TestCrypto_Decrypt_WrongKeyFails(t *testing.T) {
	// Encrypt with one key, decrypt with another — must fail authentication.
	key1 := mustCrypto(t)
	otherB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xCD}, KeyLen))
	c2, err := New(mustMasterKey(t, otherB64))
	require.NoError(t, err)

	blob, err := key1.Encrypt([]byte("classified"))
	require.NoError(t, err)

	_, err = c2.Decrypt(blob)
	assert.ErrorIs(t, err, ErrTamperedCiphertext, "decrypt under wrong key must fail")
}

func TestCrypto_MinCiphertextLen_Formula(t *testing.T) {
	// Sanity: encrypting zero-length plaintext would yield exactly
	// MinCiphertextLen bytes — but we refuse empty input, so encrypt 1 byte
	// and assert it adds exactly 1 to MinCiphertextLen.
	c := mustCrypto(t)
	blob, err := c.Encrypt([]byte{0x42})
	require.NoError(t, err)
	assert.Equal(t, MinCiphertextLen+1, len(blob),
		"len(blob) must equal version+nonce+tag+plaintext")
}

func TestCrypto_NilMasterKey_Rejected(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err, "New(nil) must fail")
}

func TestEncodeDecodeBase64_RoundTrip(t *testing.T) {
	in := []byte("any ciphertext bytes \x00\x01\x02")
	encoded := EncodeBase64(in)
	decoded, err := DecodeBase64(encoded)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(in, decoded), "base64 round-trip mismatch")
}

func TestDecodeBase64_Malformed(t *testing.T) {
	_, err := DecodeBase64("!!!not base64!!!")
	require.Error(t, err)
}

// TestCrypto_Verify_VersionByteIsBoundToAuthTag is a forward-compat probe:
// re-seal a ciphertext with version 0x01 (the only valid version), then flip
// ONLY the version byte. Our Decrypt checks the version first and returns
// ErrUnknownCiphertextVersion before reaching GCM.Open; this test pins that
// behaviour so a future "accept multiple versions" change is forced to
// re-derive the auth semantics rather than accidentally accepting the
// version-flipped blob.
func TestCrypto_Verify_VersionByteIsBoundToAuthTag(t *testing.T) {
	mk := mustMasterKey(t, masterKeyB64)
	block, err := aes.NewCipher(mk.bytes[:])
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)

	nonce := make([]byte, NonceLen)
	for i := range nonce {
		nonce[i] = byte(i) // deterministic; fine for this test
	}
	plaintext := []byte("payload")
	ad := additionalData(CiphertextVersionV1)
	sealed := gcm.Seal(nil, nonce, plaintext, ad)

	// Build a v1 blob exactly like Crypto.Encrypt does.
	blob := make([]byte, 0, VersionLen+len(sealed))
	blob = append(blob, CiphertextVersionV1)
	blob = append(blob, nonce...)
	blob = append(blob, sealed...)

	// Round-trip the v1 blob successfully first.
	c := mustCrypto(t)
	s, err := c.Decrypt(blob)
	require.NoError(t, err, "v1 blob must decrypt")
	assert.True(t, bytes.Equal(s.Reveal(), plaintext))
	s.Zero()

	// Now flip ONLY the version byte. Decrypt must reject it as an unknown
	// version before reaching the auth step.
	flipped := append([]byte{}, blob...)
	flipped[0] = 0x02
	_, err = c.Decrypt(flipped)
	assert.ErrorIs(t, err, ErrUnknownCiphertextVersion)
}

func TestSecret_RedactsInAllForms(t *testing.T) {
	s := NewSecret([]byte("super-secret-value"))
	defer s.Zero()

	assert.Equal(t, Redacted, s.String())
	assert.Equal(t, Redacted, s.GoString())
	for _, verb := range []string{"%s", "%v", "%+v", "%#v", "%x", "%X", "%q"} {
		out := fmt.Sprintf(verb, s)
		assert.Equalf(t, Redacted, out, "verb %s produced %q", verb, out)
	}

	// MarshalText
	got, err := s.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, Redacted, string(got))

	// MarshalJSON via encoding/json on the Secret directly: because we
	// implement MarshalText and NOT MarshalJSON, json.Marshal falls back to
	// MarshalText and produces a JSON string with the redacted value.
	jsonOut, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Equal(t, `"`+Redacted+`"`, string(jsonOut))
}

func TestSecret_RevealAndZero(t *testing.T) {
	s := NewSecret([]byte("hello"))
	assert.Equal(t, 5, s.Len())
	assert.Equal(t, "hello", string(s.Reveal()))

	s.Zero()
	assert.Equal(t, 0, s.Len(), "Len must be 0 after Zero")
	assert.Nil(t, s.Reveal(), "Reveal must return nil after Zero")

	// Zero is idempotent and safe on a nil receiver.
	s.Zero()
	var nilSecret *Secret
	nilSecret.Zero()
	assert.Nil(t, nilSecret.Reveal())
	assert.Equal(t, 0, nilSecret.Len())
	assert.Equal(t, Redacted, nilSecret.String())
}

func TestSecret_NeverLeaksViaSprintf(t *testing.T) {
	// Defense-in-depth: assert that the plaintext never appears in a
	// fmt.Sprintf of the Secret under every verb we can think of.
	pt := []byte("LEAK-ME-IF-YOU-CAN")
	s := NewSecret(pt)
	defer s.Zero()

	for _, verb := range []string{"%s", "%v", "%+v", "%#v", "%x", "%X", "%q", "%t", "%b", "%o"} {
		out := fmt.Sprintf(verb, s)
		assert.NotContainsf(t, out, string(pt), "verb %s leaked plaintext: %q", verb, out)
		// Sanity: redacted sentinel must appear in the output for any verb that
		// prints a string form (i.e. all of them given our Format override).
		assert.Containsf(t, out, Redacted, "verb %s missing redacted sentinel", verb)
	}
}

func TestRedactString(t *testing.T) {
	assert.Equal(t, Redacted, RedactString("anything"))
	assert.Equal(t, Redacted, RedactString(""))
}
