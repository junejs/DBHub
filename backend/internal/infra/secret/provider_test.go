package secret

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeManager is an in-memory ExternalManager for Provider tests. It returns
// whatever plaintext was registered for a given ref, or an error. It is NOT
// safe for concurrent mutation — tests build it once before Resolve.
type fakeManager struct {
	scheme string
	store  map[string][]byte
	err    error // if non-nil, Resolve returns this regardless of ref
}

func (m *fakeManager) Scheme() string { return m.scheme }

func (m *fakeManager) Resolve(_ context.Context, ref string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if v, ok := m.store[ref]; ok {
		return v, nil
	}
	return nil, errors.New("fakeManager: ref not found: " + ref)
}

func TestSecretRef_IsEmpty(t *testing.T) {
	assert.True(t, SecretRef{}.IsEmpty(), "zero value is empty")
	assert.True(t, SecretRef{ExternalRef: ""}.IsEmpty(), "no ref + no ciphertext is empty")
	assert.False(t, SecretRef{ExternalRef: "vault://x"}.IsEmpty(), "external ref set is not empty")
	assert.False(t, SecretRef{LocalCiphertext: []byte{1}}.IsEmpty(), "ciphertext set is not empty")
}

func TestNewLocalProvider_NilCrypto(t *testing.T) {
	_, err := NewLocalProvider(nil)
	require.Error(t, err)
}

func TestNewDefaultProvider_NilCrypto(t *testing.T) {
	_, err := NewDefaultProvider(nil)
	require.Error(t, err)
}

func TestLocalProvider_ResolvesLocalCiphertext(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewLocalProvider(c)
	require.NoError(t, err)

	blob, err := c.Encrypt([]byte("db-password"))
	require.NoError(t, err)

	s, err := p.Resolve(context.Background(), SecretRef{LocalCiphertext: blob})
	require.NoError(t, err)
	defer s.Zero()
	assert.Equal(t, "db-password", string(s.Reveal()))
}

func TestLocalProvider_RejectsExternalRef(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewLocalProvider(c)
	require.NoError(t, err)

	_, err = p.Resolve(context.Background(), SecretRef{ExternalRef: "vault://kv/x"})
	require.Error(t, err, "LocalProvider must refuse external refs")
	assert.ErrorIs(t, err, ErrExternalManagerNotConfigured)
}

func TestLocalProvider_RejectsEmptyCiphertext(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewLocalProvider(c)
	require.NoError(t, err)

	_, err = p.Resolve(context.Background(), SecretRef{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLocalCiphertextMissing)
}

func TestLocalProvider_PropagatesDecryptErrors(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewLocalProvider(c)
	require.NoError(t, err)

	// Tampered ciphertext (too short) → must propagate ErrCiphertextTooShort.
	_, err = p.Resolve(context.Background(), SecretRef{LocalCiphertext: []byte{0x01}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCiphertextTooShort)
}

func TestDefaultProvider_LocalPath(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	blob, err := c.Encrypt([]byte("local-secret"))
	require.NoError(t, err)

	s, err := p.Resolve(context.Background(), SecretRef{LocalCiphertext: blob})
	require.NoError(t, err)
	defer s.Zero()
	assert.Equal(t, "local-secret", string(s.Reveal()))
}

func TestDefaultProvider_ExternalRefPreferredOverLocal(t *testing.T) {
	// Decision rule (D25): secret_ref non-empty -> external wins, local ignored.
	c := mustCrypto(t)
	blob, err := c.Encrypt([]byte("local-secret"))
	require.NoError(t, err)

	mgr := &fakeManager{
		scheme: "vault",
		store:  map[string][]byte{"vault://kv/dbhub/prod": []byte("external-secret")},
	}
	p, err := NewDefaultProvider(c, mgr)
	require.NoError(t, err)

	// Both ExternalRef AND LocalCiphertext set — ExternalRef must win.
	s, err := p.Resolve(context.Background(), SecretRef{
		ExternalRef:     "vault://kv/dbhub/prod",
		LocalCiphertext: blob,
	})
	require.NoError(t, err)
	defer s.Zero()
	assert.Equal(t, "external-secret", string(s.Reveal()),
		"external ref must take priority over local ciphertext")
}

func TestDefaultProvider_ExternalRefMissingManager(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c) // no managers registered
	require.NoError(t, err)

	_, err = p.Resolve(context.Background(), SecretRef{ExternalRef: "vault://kv/x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExternalManagerNotConfigured)
}

func TestDefaultProvider_ExternalRefUnparsable(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	// No "://" → scheme cannot be extracted.
	_, err = p.Resolve(context.Background(), SecretRef{ExternalRef: "not-a-url"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExternalRefUnparsable)
}

func TestDefaultProvider_ExternalManager_PropagatesError(t *testing.T) {
	c := mustCrypto(t)
	mgrErr := errors.New("vault timed out")
	mgr := &fakeManager{scheme: "vault", err: mgrErr}
	p, err := NewDefaultProvider(c, mgr)
	require.NoError(t, err)

	_, err = p.Resolve(context.Background(), SecretRef{ExternalRef: "vault://kv/x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, mgrErr, "manager error must be wrapped, not swallowed")
}

func TestDefaultProvider_EmptyRef(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	// Both nil ciphertext and empty slice are treated as "no source configured";
	// callers must detect this via SecretRef.IsEmpty() before calling Resolve.
	cases := []struct {
		name string
		ref  SecretRef
	}{
		{"zero value", SecretRef{}},
		{"nil ciphertext", SecretRef{LocalCiphertext: nil}},
		{"empty slice ciphertext", SecretRef{LocalCiphertext: []byte{}}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.Resolve(context.Background(), tc.ref)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrEmptySecretRef)
		})
	}
}

func TestDefaultProvider_RegisterManager(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	mgr := &fakeManager{scheme: "aws", store: map[string][]byte{
		"aws://sm/abc": []byte("aws-secret"),
	}}
	require.NoError(t, p.RegisterManager(mgr))

	s, err := p.Resolve(context.Background(), SecretRef{ExternalRef: "aws://sm/abc"})
	require.NoError(t, err)
	defer s.Zero()
	assert.Equal(t, "aws-secret", string(s.Reveal()))
}

func TestDefaultProvider_RegisterManager_DuplicateScheme(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	mgr1 := &fakeManager{scheme: "vault"}
	mgr2 := &fakeManager{scheme: "vault"}
	require.NoError(t, p.RegisterManager(mgr1))
	err = p.RegisterManager(mgr2)
	require.Error(t, err, "duplicate scheme must be rejected")
}

func TestDefaultProvider_RegisterManager_NilOrEmptyScheme(t *testing.T) {
	c := mustCrypto(t)
	p, err := NewDefaultProvider(c)
	require.NoError(t, err)

	require.Error(t, p.RegisterManager(nil))

	// A manager whose Scheme() returns "" is invalid.
	empty := &fakeManager{scheme: ""}
	require.Error(t, p.RegisterManager(empty))
}

// TestSchemeOf covers the URL-scheme routing helper. We don't ship a URL parser
// (the manager owns its own URL shape), but we must correctly extract the
// leading scheme to route to the right adapter.
func TestSchemeOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"vault://kv/x", "vault"},
		{"VAULT://KV/X", "vault"}, // scheme lower-cased
		{"aws://sm/abc", "aws"},
		{"gcp://projects/x/secrets/y", "gcp"},
		{"aliyun://kms/key/abc", "aliyun"},
		{"no-scheme", ""},   // no "://"
		{"host:port", ""},   // ':' but not '://'
		{"http://", "http"}, // trailing scheme-only is still routed
		{"://nothing", ""},  // empty scheme
		{"", ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, schemeOf(c.in))
		})
	}
}

// TestDefaultProvider_SatisfiesProvider pins the fact that *DefaultProvider
// satisfies the Provider interface; if a future rename breaks the contract
// this test fails to compile.
func TestDefaultProvider_SatisfiesProvider(t *testing.T) {
	var _ Provider = (*DefaultProvider)(nil)
	var _ Provider = (*LocalProvider)(nil)
}

// TestProvider_ContextPropagatedToManager ensures callers' ctx reaches the
// ExternalManager so cancellation / timeouts work end-to-end.
func TestProvider_ContextPropagatedToManager(t *testing.T) {
	c := mustCrypto(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	called := false
	mgr := &ctxRecordingManager{scheme: "vault", called: &called, ctx: ctx}
	p, err := NewDefaultProvider(c, mgr)
	require.NoError(t, err)

	_, err = p.Resolve(ctx, SecretRef{ExternalRef: "vault://kv/x"})
	// We don't assert on the error value; the contract under test is that the
	// manager observed the SAME ctx the caller passed. If it did, the test
	// inside Resolve already recorded the match.
	require.NoError(t, err)
	assert.True(t, called, "manager.Resolve must be called")
}

// ctxRecordingManager asserts that the ctx passed to Resolve is the same one
// the caller handed Provider.Resolve.
type ctxRecordingManager struct {
	scheme string
	called *bool
	ctx    context.Context
}

func (m *ctxRecordingManager) Scheme() string { return m.scheme }

func (m *ctxRecordingManager) Resolve(ctx context.Context, _ string) ([]byte, error) {
	*m.called = true
	if ctx != m.ctx {
		return nil, errors.New("ctx mismatch: manager did not receive caller's ctx")
	}
	return []byte("ok"), nil
}
