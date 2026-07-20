package secret

import "fmt"

// Redacted is the canonical placeholder substituted for any sensitive value
// before it touches logs, errors, audit records, or API responses
// (PRD §6 安全边界 / 08-nfr §1.3). Always use this constant rather than
// ad-hoc strings like "***" or "<hidden>" so redaction is grep-able.
const Redacted = "[REDACTED]"

// Secret wraps a plaintext credential (or any sensitive byte sequence) so it
// is safe to pass through code that may format, log, JSON-encode, or error-wrap
// it. The bytes are only retrievable via Reveal; every other representation
// (String / GoString / MarshalJSON / MarshalText / Format) yields Redacted.
//
// Lifecycle:
//
//	s, err := crypto.Decrypt(blob)   // s now owns the plaintext
//	defer s.Zero()                   // best-effort wipe on scope exit
//	use(s.Reveal())                  // hand bytes to the consumer
//
// The type is safe for concurrent Reveal / Zero only in the sense that the
// returned slice aliases internal storage; callers must not concurrently
// read and Zero the same Secret. In practice Secrets are not shared across
// goroutines (one decrypt per connection setup, used and discarded).
type Secret struct {
	plaintext []byte
}

// NewSecret wraps an existing plaintext slice. The caller passes ownership of
// the bytes — do not retain or mutate them afterwards. Production code should
// obtain Secrets via Crypto.Decrypt; this constructor exists for tests and
// adapters that already hold the plaintext (e.g. an ExternalManager that
// fetched bytes over the network).
func NewSecret(plaintext []byte) *Secret {
	return &Secret{plaintext: plaintext}
}

// Reveal returns the underlying plaintext bytes. The returned slice aliases
// the Secret's internal storage, so:
//   - Do not retain it beyond the Secret's lifetime (Zero() will wipe it).
//   - Do not mutate it concurrently with Zero().
//
// Returns nil after Zero() has been called, so callers that re-reveal can
// detect the use-after-zero state with a nil check.
func (s *Secret) Reveal() []byte {
	if s == nil {
		return nil
	}
	return s.plaintext
}

// Zero overwrites the plaintext with zeros. Best-effort: Go's GC may have
// copied the slice, and there is no way to pin the original bytes. Still, we
// clear the in-process view so a subsequent heap dump / goroutine stack trace
// cannot read it. Safe to call multiple times; safe on a nil receiver.
func (s *Secret) Zero() {
	if s == nil {
		return
	}
	for i := range s.plaintext {
		s.plaintext[i] = 0
	}
	s.plaintext = nil
}

// Len returns the plaintext length without revealing the bytes. Returns 0 for
// a nil or zeroed Secret. Useful for assertions like "secret was loaded" in
// logs without exposing the value.
func (s *Secret) Len() int {
	if s == nil {
		return 0
	}
	return len(s.plaintext)
}

// String implements fmt.Stringer. Always returns Redacted — never the plaintext.
// This is the single most important method on the type: it makes the Secret
// safe to pass to slog.Info("...", "password", secret) and similar.
func (s *Secret) String() string { return Redacted }

// GoString implements fmt.GoStringer. Always returns Redacted — never the
// plaintext. Used by `%#v` formatting.
func (s *Secret) GoString() string { return Redacted }

// Format makes Secret safe under all fmt verbs (`%s`, `%v`, `%+v`, `%#v`,
// `%x`, `%q`, …). Any verb returns Redacted. Without this, `fmt.Sprintf("%x",
// secret)` would dump the plaintext bytes as hex.
func (s *Secret) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprint(f, Redacted)
}

// MarshalText makes Secret safe when embedded in a struct serialized via
// encoding/json (which calls MarshalText if MarshalJSON is absent) or any
// other text-based encoder. Always returns the Redacted sentinel.
func (s *Secret) MarshalText() ([]byte, error) { return []byte(Redacted), nil }

// RedactString returns Redacted regardless of input. Convenience helper for
// sites that have a string value and want the same redaction behaviour as
// Secret.String(), typically when scrubbing a struct field for an audit log.
func RedactString(_ string) string { return Redacted }
