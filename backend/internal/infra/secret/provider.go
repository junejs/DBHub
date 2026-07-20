package secret

import (
	"context"
	"errors"
	"fmt"
)

// Sentinel errors for the Provider layer.
var (
	// ErrEmptySecretRef is returned when neither ExternalRef nor LocalCiphertext
	// is present. A row that has no secret configured should be handled before
	// reaching the Provider (e.g. fall back to anonymous auth), so Resolve
	// refuses to silently return an empty Secret.
	ErrEmptySecretRef = errors.New("secret: secret reference has neither external ref nor local ciphertext")

	// ErrLocalCiphertextMissing is returned when ExternalRef is empty (so the
	// local path is taken) but LocalCiphertext is also empty/nil.
	ErrLocalCiphertextMissing = errors.New("secret: local ciphertext is empty but no external ref set")

	// ErrExternalManagerNotConfigured is returned when ExternalRef is non-empty
	// but no ExternalManager is registered for the ref's scheme. v1 does not
	// ship any external manager (D25 / 18-roadmap §v2); deployments that set
	// `secret_ref` must register an adapter before they can use it.
	ErrExternalManagerNotConfigured = errors.New("secret: external secret manager not configured for ref scheme")

	// ErrExternalRefUnparsable is returned when ExternalRef does not parse as a
	// `scheme://...` URL with a non-empty scheme. The Provider never inspects
	// the host/path — that's the manager's job — but the scheme must be present
	// so we can route to the correct adapter.
	ErrExternalRefUnparsable = errors.New("secret: external ref is not a scheme://... URL")
)

// SecretRef describes how to retrieve a single secret. It mirrors the
// per-row pair stored on `identity_providers` and `data_sources` (DDL §3.1-3.2):
//
//   - ExternalRef (DB column `secret_ref TEXT`): opaque reference into an
//     external Secret Manager (e.g. "vault://kv/dbhub/prod#password"). When
//     non-empty, the Provider routes through ExternalManager and ignores
//     LocalCiphertext.
//   - LocalCiphertext (DB column `secrets_enc BYTEA` / JSONB
//     `connection.password_enc`): AES-256-GCM ciphertext envelope produced by
//     Crypto.Encrypt. Used iff ExternalRef is empty.
//
// Decision rule (D25): ExternalRef wins when set; otherwise local decrypt.
type SecretRef struct {
	// ExternalRef routes through an ExternalManager when non-empty. Format is
	// manager-specific but must begin with `scheme://`.
	ExternalRef string

	// LocalCiphertext is the AES-256-GCM ciphertext envelope. Ignored when
	// ExternalRef is non-empty.
	LocalCiphertext []byte
}

// IsEmpty reports whether the ref carries no usable source. Used by callers
// to short-circuit with a clear sentinel before invoking Resolve.
func (r SecretRef) IsEmpty() bool {
	return r.ExternalRef == "" && len(r.LocalCiphertext) == 0
}

// ExternalManager is the seam for future Secret Manager adapters (Vault, AWS
// Secrets Manager, GCP SM, Aliyun KMS, ...). v1 does NOT ship any adapter
// (per 18-roadmap §v2); the interface is published so that:
//
//  1. Provider can route `secret_ref` lookups through it without changing
//     shape when adapters arrive.
//  2. Operators wiring a custom adapter in-tree (or via a private fork) have
//     a stable contract to satisfy.
//
// Implementations MUST be safe for concurrent use. Resolve returns plaintext
// exactly as the manager stores it; the Provider wraps it in a Secret.
type ExternalManager interface {
	// Scheme returns the URL scheme this manager handles (e.g. "vault", "aws",
	// "gcp", "aliyun"). Must be lowercase ASCII, no trailing "://". Used by
	// Provider to route ExternalRef values by their leading scheme.
	Scheme() string

	// Resolve returns the plaintext bytes for an external secret reference.
	// The format of `ref` beyond the leading `scheme://` is manager-specific.
	// ctx is propagated for cancellation / timeouts against the manager's API.
	Resolve(ctx context.Context, ref string) ([]byte, error)
}

// Provider resolves a SecretRef into a Secret. The single Resolve method
// encapsulates the local-vs-external decision so callers (data-source
// connectors, IdP clients) don't reimplement the branching.
//
// The zero-source-of-truth for this contract is D25 + 10-data-model §3.1-3.2.
type Provider interface {
	// Resolve returns the plaintext secret described by ref. If ref.ExternalRef
	// is non-empty, the configured ExternalManager is consulted; otherwise the
	// local ciphertext is decrypted with the master key.
	//
	// The caller owns the returned Secret and is responsible for calling Zero
	// once consumed.
	Resolve(ctx context.Context, ref SecretRef) (*Secret, error)
}

// LocalProvider resolves secrets purely from local ciphertext. It is the v1
// default: every deployment has a master key; not every deployment wires up
// an external Secret Manager. Construct via NewLocalProvider.
type LocalProvider struct {
	crypto *Crypto
}

// NewLocalProvider builds a Provider that only handles the local-ciphertext
// path. Returns nil + error if crypto is nil — a Provider without a cipher is
// not a Provider.
func NewLocalProvider(crypto *Crypto) (*LocalProvider, error) {
	if crypto == nil {
		return nil, fmt.Errorf("secret.NewLocalProvider: nil crypto")
	}
	return &LocalProvider{crypto: crypto}, nil
}

// Resolve decrypts ref.LocalCiphertext. If ref.ExternalRef is non-empty it
// returns ErrExternalManagerNotConfigured (LocalProvider has no manager by
// definition); callers that need both paths should use DefaultProvider.
func (p *LocalProvider) Resolve(_ context.Context, ref SecretRef) (*Secret, error) {
	if ref.ExternalRef != "" {
		// A local-only Provider explicitly cannot honour an external ref;
		// surface the same sentinel DefaultProvider would if no manager were
		// registered for the scheme.
		return nil, fmt.Errorf("%w: scheme=%s", ErrExternalManagerNotConfigured, schemeOf(ref.ExternalRef))
	}
	if len(ref.LocalCiphertext) == 0 {
		return nil, ErrLocalCiphertextMissing
	}
	return p.crypto.Decrypt(ref.LocalCiphertext)
}

// DefaultProvider is the production-grade Provider. It picks the resolution
// path based on SecretRef.ExternalRef:
//
//   - non-empty ExternalRef → look up a registered ExternalManager by scheme
//     and delegate.
//   - empty ExternalRef → decrypt LocalCiphertext with the master key.
//
// The default fallback for an unset ExternalRef is always the local path, so
// a deployment that never configures an external manager behaves identically
// to LocalProvider. Managers can be added at runtime via RegisterManager (e.g.
// during main.go wiring) without rebuilding the Provider.
type DefaultProvider struct {
	crypto   *Crypto
	managers map[string]ExternalManager
}

// NewDefaultProvider builds a Provider that prefers ExternalManager when an
// ExternalRef is set, and falls back to local ciphertext otherwise. Accepts
// zero or more ExternalManager implementations to register up-front; further
// managers can be added via RegisterManager.
func NewDefaultProvider(crypto *Crypto, managers ...ExternalManager) (*DefaultProvider, error) {
	if crypto == nil {
		return nil, fmt.Errorf("secret.NewDefaultProvider: nil crypto")
	}
	p := &DefaultProvider{
		crypto:   crypto,
		managers: make(map[string]ExternalManager, len(managers)),
	}
	for _, m := range managers {
		if m == nil {
			continue
		}
		p.managers[m.Scheme()] = m
	}
	return p, nil
}

// RegisterManager attaches an ExternalManager so future Resolve calls with an
// ExternalRef whose scheme matches m.Scheme() are routed through it. Returns
// an error if a manager for the same scheme is already registered — silent
// overwrite during a long-running process is the kind of subtle bug that
// takes down production.
func (p *DefaultProvider) RegisterManager(m ExternalManager) error {
	if m == nil {
		return fmt.Errorf("secret.DefaultProvider.RegisterManager: nil manager")
	}
	scheme := m.Scheme()
	if scheme == "" {
		return fmt.Errorf("secret.DefaultProvider.RegisterManager: empty scheme")
	}
	if _, dup := p.managers[scheme]; dup {
		return fmt.Errorf("secret.DefaultProvider.RegisterManager: scheme %q already registered", scheme)
	}
	p.managers[scheme] = m
	return nil
}

// Resolve implements Provider. Decision rule (D25): external wins when set.
func (p *DefaultProvider) Resolve(ctx context.Context, ref SecretRef) (*Secret, error) {
	if ref.IsEmpty() {
		return nil, ErrEmptySecretRef
	}

	if ref.ExternalRef != "" {
		scheme := schemeOf(ref.ExternalRef)
		if scheme == "" {
			return nil, fmt.Errorf("%w: %q", ErrExternalRefUnparsable, ref.ExternalRef)
		}
		m, ok := p.managers[scheme]
		if !ok {
			return nil, fmt.Errorf("%w: scheme=%s", ErrExternalManagerNotConfigured, scheme)
		}
		// The ExternalRef is passed verbatim; the manager owns its own URL shape.
		plaintext, err := m.Resolve(ctx, ref.ExternalRef)
		if err != nil {
			return nil, fmt.Errorf("secret.DefaultProvider.Resolve: external manager %q: %w", scheme, err)
		}
		return NewSecret(plaintext), nil
	}

	if len(ref.LocalCiphertext) == 0 {
		return nil, ErrLocalCiphertextMissing
	}
	return p.crypto.Decrypt(ref.LocalCiphertext)
}

// schemeOf extracts the lowercase URL scheme from a `scheme://...` reference.
// Returns "" if the input does not contain "://" or the scheme portion is
// empty. Used purely for routing — no validation of the rest of the URL.
func schemeOf(ref string) string {
	for i := 0; i < len(ref); i++ {
		if ref[i] == ':' {
			if i+2 < len(ref) && ref[i+1] == '/' && ref[i+2] == '/' {
				return lowerASCII(ref[:i])
			}
			// A ':' that isn't followed by '//' is not a URL scheme separator
			// we recognise; bail out so we don't mis-route "host:port" style.
			return ""
		}
	}
	return ""
}

// lowerASCII returns a lowercase copy of s, ASCII only. schemeOf only feeds it
// the leading scheme segment (typically 3-6 ASCII chars), so the alloc is
// bounded; we avoid strings.ToLower here to keep the helper dependency-free.
func lowerASCII(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
