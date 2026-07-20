// Package secret implements the at-rest credential encryption primitives and
// the Secret Provider abstraction used by IdP and business database connection
// configuration (PRD D25, 16-ops §3).
//
// Layering:
//
//	MasterKey  — validated 32-byte AES-256 key (loaded from env, never persisted)
//	   │
//	   ▼
//	Crypto     — AES-256-GCM with versioned ciphertext; Encrypt/Decrypt
//	   │
//	   ▼
//	Provider   — resolves a SecretRef into plaintext (local ciphertext OR external manager)
//
// Ciphertext wire format (stable, designed for forward-compatible key rotation):
//
//	+-----------+-------------+----------------------------+
//	| version   | nonce       | ciphertext + GCM auth tag |
//	| 1 byte    | 12 bytes    | variable (plaintext + 16) |
//	+-----------+-------------+----------------------------+
//
// The leading version byte lets a future build decrypt legacy ciphertext while
// encrypting new rows under a different scheme — the rotation seam. v1 only
// emits and accepts version 0x01 (AES-256-GCM). Any other byte fast-fails with
// ErrUnknownCiphertextVersion.
//
// Security notes:
//   - Nonces are cryptographically random (crypto/rand), so the same plaintext
//     encrypted twice produces different ciphertexts (acceptance criterion).
//   - The GCM auth tag covers nonce+ciphertext; any tampering is detected on
//     Decrypt and surfaces as a wrapped cipher error.
//   - Plaintext is never logged here; callers receive a Secret wrapper that
//     redacts itself in any fmt/slog/json output (see redact.go).
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// CiphertextVersionV1 is the only version emitted and accepted by v1.
// Reserved for future key rotation / algorithm migration.
const CiphertextVersionV1 byte = 0x01

// CurrentVersion aliases the version new ciphertexts are sealed under. Future
// builds may bump this while still accepting CiphertextVersionV1 for decrypt.
const CurrentVersion byte = CiphertextVersionV1

// Wire-format constants. Exposed for tests and external size validation only.
const (
	// VersionLen is the size of the leading version byte.
	VersionLen = 1
	// NonceLen is the GCM nonce length in bytes (standard for AES-GCM).
	NonceLen = 12
	// TagLen is the GCM authentication tag length appended to ciphertext.
	TagLen = 16
	// MinCiphertextLen is the smallest legal ciphertext blob: version + nonce + tag
	// (zero-length plaintext). Shorter inputs are rejected before any decrypt work.
	MinCiphertextLen = VersionLen + NonceLen + TagLen
)

// Sentinel errors. Wrap with fmt.Errorf("...: %w", err) at call sites to add
// context while keeping errors.Is readable.
var (
	// ErrEmptyPlaintext is returned when encrypting a zero-length plaintext.
	// Credentials are never empty in practice; refusing to seal empty input
	// prevents callers from silently writing a "valid" ciphertext for a
	// missing value (which would mask a config bug).
	ErrEmptyPlaintext = errors.New("secret: empty plaintext")

	// ErrCiphertextTooShort is returned when the input is shorter than the
	// minimum legal ciphertext length (version + nonce + tag).
	ErrCiphertextTooShort = errors.New("secret: ciphertext too short")

	// ErrUnknownCiphertextVersion is returned when the leading version byte is
	// not recognised. v1 only accepts 0x01.
	ErrUnknownCiphertextVersion = errors.New("secret: unknown ciphertext version")

	// ErrTamperedCiphertext is returned when GCM authentication fails. The
	// underlying error from cipher.GCM is wrapped for callers that want the
	// raw detail; never log it verbatim if it may include key material (it
	// does not, but defensive coding is cheap).
	ErrTamperedCiphertext = errors.New("secret: ciphertext authentication failed")
)

// Crypto performs AES-256-GCM encryption and decryption with a versioned
// ciphertext envelope. Construct once per process via New and reuse; the type
// is safe for concurrent use because crypto/cipher.AEAD is.
type Crypto struct {
	gcm cipher.AEAD
}

// New builds a Crypto from a validated MasterKey. The MasterKey is not retained
// beyond the constructor — the resulting AEAD holds the expanded round keys.
func New(masterKey *MasterKey) (*Crypto, error) {
	if masterKey == nil {
		return nil, fmt.Errorf("secret.New: nil master key")
	}
	block, err := aes.NewCipher(masterKey.bytes[:])
	if err != nil {
		// Should be unreachable: MasterKey validation already guarantees 32 bytes.
		return nil, fmt.Errorf("secret.New: init AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret.New: init GCM: %w", err)
	}
	return &Crypto{gcm: gcm}, nil
}

// Encrypt seals plaintext under a fresh random nonce and prepends the current
// version byte. The same plaintext encrypted twice yields different ciphertext
// because the nonce is random per call (see TestCrypto_Encrypt_SamePlaintextDifferentCiphertext).
//
// Returns ErrEmptyPlaintext for empty input; callers that legitimately need to
// store "no secret" should store nil ciphertext instead.
func (c *Crypto) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, ErrEmptyPlaintext
	}

	nonce := make([]byte, NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		// crypto/rand.Read on Linux reads /dev/urandom which essentially never
		// fails; treat any failure as fatal.
		return nil, fmt.Errorf("secret.Crypto.Encrypt: read nonce: %w", err)
	}

	// Layout: out[0]=version, out[1:13]=nonce, out[13:]=ciphertext+tag.
	// We grow one buffer and let GCM seal into the tail so the tag is computed
	// in place.
	out := make([]byte, VersionLen+NonceLen, VersionLen+NonceLen+len(plaintext)+TagLen)
	out[0] = CurrentVersion
	copy(out[VersionLen:], nonce)

	// Seal appends ciphertext+tag to out[:VersionLen+NonceLen] and returns the
	// newly-extended slice.
	sealed := c.gcm.Seal(out[:VersionLen+NonceLen], nonce, plaintext, additionalData(out[0]))
	return sealed, nil
}

// Decrypt authenticates and decrypts a ciphertext blob produced by Encrypt
// (or a compatible future encryptor). It fast-fails on:
//   - too-short input (ErrCiphertextTooShort),
//   - unknown version byte (ErrUnknownCiphertextVersion),
//   - tampered ciphertext / wrong key (ErrTamperedCiphertext, wrapping the
//     underlying GCM error for callers that want it).
//
// On success the caller receives a Secret that redacts itself in any
// fmt/slog/json output; use Secret.Reveal to obtain the plaintext bytes.
func (c *Crypto) Decrypt(blob []byte) (*Secret, error) {
	if len(blob) < MinCiphertextLen {
		return nil, ErrCiphertextTooShort
	}
	version := blob[0]
	if version != CiphertextVersionV1 {
		return nil, fmt.Errorf("%w: got 0x%02x, want 0x%02x", ErrUnknownCiphertextVersion, version, CiphertextVersionV1)
	}
	nonce := blob[VersionLen : VersionLen+NonceLen]
	ct := blob[VersionLen+NonceLen:]

	plaintext, err := c.gcm.Open(nil, nonce, ct, additionalData(version))
	if err != nil {
		// Do NOT include the raw err in logs; wrap so callers see a stable sentinel.
		return nil, fmt.Errorf("%w: %v", ErrTamperedCiphertext, err)
	}

	// Copy into a buffer we own, so the caller's view can be Zero()ed without
	// aliasing the input blob.
	buf := make([]byte, len(plaintext))
	copy(buf, plaintext)
	// Best-effort: wipe the temporary returned by GCM.Open.
	for i := range plaintext {
		plaintext[i] = 0
	}
	return &Secret{plaintext: buf}, nil
}

// additionalData binds the version byte into the GCM auth tag, so flipping the
// version byte of a v1 ciphertext also fails authentication. Empty AAD is the
// spec baseline; we keep this minimal to stay interoperable with any future
// tooling that reads our ciphertext with the standard GCM primitive.
func additionalData(version byte) []byte {
	return []byte{version}
}

// EncodeBase64 is a convenience helper for callers that need to ship ciphertext
// through a text-only channel (e.g. JSON config). Round-trips with DecodeBase64.
func EncodeBase64(ciphertext []byte) string {
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// DecodeBase64 is the inverse of EncodeBase64. It does NOT validate that the
// decoded bytes are valid ciphertext — that happens in Crypto.Decrypt.
func DecodeBase64(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("secret.DecodeBase64: %w", err)
	}
	return b, nil
}
