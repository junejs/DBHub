package secret

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeyLen is the required master key length in bytes: 32 bytes (AES-256).
// `MASTER_KEY` must base64-decode to exactly this many bytes (16-ops §3).
const KeyLen = 32

// Sentinel errors for master-key loading.
var (
	// ErrMasterKeyEmpty is returned when the env value is missing or whitespace-only.
	ErrMasterKeyEmpty = errors.New("secret: master key is empty")

	// ErrMasterKeyMalformed is returned when the value is not valid base64 (StdEncoding).
	ErrMasterKeyMalformed = errors.New("secret: master key is not valid base64")

	// ErrMasterKeyWrongLength is returned when the decoded bytes are not exactly KeyLen.
	ErrMasterKeyWrongLength = errors.New("secret: master key must decode to 32 bytes")
)

// MasterKey is a validated AES-256 key. The raw bytes are kept unexported so
// callers cannot accidentally log them via reflection or fmt printing; the only
// ways out are the Crypto cipher operations (which expand round keys internally)
// and Equal (constant-time comparison).
//
// Construct via LoadMasterKey. The zero value is NOT a valid key — Crypto.New
// rejects nil.
type MasterKey struct {
	bytes [KeyLen]byte
}

// LoadMasterKey decodes and validates a base64-encoded 32-byte master key as
// documented in 16-ops §2 (`MASTER_KEY`). The input is the raw env value:
//
//	mk, err := secret.LoadMasterKey(os.Getenv("MASTER_KEY"))
//
// The returned *MasterKey owns a private copy of the decoded bytes; the input
// string is not retained.
//
// Validation is strict by design (acceptance criterion: "非法主密钥快速失败"):
//   - empty / whitespace-only → ErrMasterKeyEmpty
//   - not valid StdEncoding base64 → ErrMasterKeyMalformed
//   - decodes to anything other than 32 bytes → ErrMasterKeyWrongLength
//
// All three errors are stable sentinels so callers can branch on errors.Is.
func LoadMasterKey(raw string) (*MasterKey, error) {
	if len(raw) == 0 {
		return nil, ErrMasterKeyEmpty
	}

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMasterKeyMalformed, err)
	}
	if len(decoded) != KeyLen {
		return nil, fmt.Errorf("%w: got %d bytes", ErrMasterKeyWrongLength, len(decoded))
	}

	mk := &MasterKey{}
	copy(mk.bytes[:], decoded)
	// Wipe the temporary decoded slice; we hold our own copy.
	for i := range decoded {
		decoded[i] = 0
	}
	return mk, nil
}

// Equal reports whether two master keys are equal in constant time. Useful in
// rotation flows that compare the active key against a candidate replacement.
func (m *MasterKey) Equal(other *MasterKey) bool {
	if m == nil || other == nil {
		return m == other
	}
	return subtle.ConstantTimeCompare(m.bytes[:], other.bytes[:]) == 1
}

// IsZero reports whether the key is all-zero bytes (i.e. an unset / wiped key).
// Never an "ok" state in production; useful in self-tests and rotation flows
// to refuse to encrypt with a placeholder.
func (m *MasterKey) IsZero() bool {
	if m == nil {
		return true
	}
	for _, b := range m.bytes {
		if b != 0 {
			return false
		}
	}
	return true
}

// String implements fmt.Stringer and ALWAYS returns the redacted sentinel —
// the master key must never appear in logs, errors, or audit records (PRD §6).
// The method exists precisely so a stray `%v` / `slog.Info("...", "key", mk)`
// does NOT leak the key. The same applies to GoString (for `%#v`).
func (m *MasterKey) String() string   { return Redacted }
func (m *MasterKey) GoString() string { return Redacted }
