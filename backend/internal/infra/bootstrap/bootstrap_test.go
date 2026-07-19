package bootstrap_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/junepy/dbhub/backend/internal/infra/bootstrap"
	"github.com/junepy/dbhub/backend/internal/infra/dbtest"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
)

// captureLogs swaps slog's default JSON handler for a buffer-backed one for the
// duration of t. Returns the buffer the caller can inspect; the original default
// is restored via t.Cleanup. Used by tests that need to assert no password ever
// lands in any log line.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestEnsureFirstAdmin_EmptyDBCreatesUserWithBcryptHash(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	logs := captureLogs(t)
	const password = "S3cret-bootstrap-pw!"
	err := bootstrap.EnsureFirstAdmin(ctx, bunDB, "root@example.com", password)
	require.NoError(t, err)

	// One user, bcrypt-hashed password (NOT plaintext), status=active, source=local.
	var got struct {
		Email        string
		Name         string
		Status       string
		Source       string
		PasswordHash string
	}
	require.NoError(t, bunDB.NewSelect().TableExpr("users").
		Column("email", "name", "status", "source", "password_hash").
		Where("email = ?", "root@example.com").
		Scan(ctx, &got))

	assert.Equal(t, "root@example.com", got.Email)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "local", got.Source)
	assert.NotEmpty(t, got.PasswordHash)
	assert.NotEqual(t, password, got.PasswordHash, "plaintext must never be stored")
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte(password)))

	// Password must not appear anywhere in the logs (case sensitive).
	assert.False(t, strings.Contains(logs.String(), password),
		"password leaked into logs:\n%s", logs.String())

	// User id + status must be logged for ops observability.
	assert.Contains(t, logs.String(), `"status":"ok"`)
}

func TestEnsureFirstAdmin_ExistingAdminIsNotModified(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	// Seed an existing user by hand to simulate a previously bootstrapped DB.
	existingHash, err := bcrypt.GenerateFromPassword([]byte("previous"), bootstrap.BcryptCost)
	require.NoError(t, err)
	_, err = bunDB.ExecContext(ctx,
		`INSERT INTO users(email, name, status, source, password_hash)
		 VALUES (?, ?, ?, ?, ?)`,
		"alice@example.com", "Alice", "active", "local", string(existingHash))
	require.NoError(t, err)

	logs := captureLogs(t)
	// Now call EnsureFirstAdmin with DIFFERENT credentials — they must be ignored.
	err = bootstrap.EnsureFirstAdmin(ctx, bunDB, "root@example.com", "would-be-ignored")
	require.NoError(t, err)

	// alice still there, root not added.
	var aliceCount, rootCount int
	require.NoError(t, bunDB.NewSelect().TableExpr("users").
		ColumnExpr("count(*)").Where("email = ?", "alice@example.com").Scan(ctx, &aliceCount))
	require.NoError(t, bunDB.NewSelect().TableExpr("users").
		ColumnExpr("count(*)").Where("email = ?", "root@example.com").Scan(ctx, &rootCount))

	assert.Equal(t, 1, aliceCount)
	assert.Equal(t, 0, rootCount, "existing admin must NOT be overwritten")

	// Skip log line should be present.
	assert.Contains(t, logs.String(), "skipping")
	// The ignored password must not appear.
	assert.False(t, strings.Contains(logs.String(), "would-be-ignored"))
}

func TestEnsureFirstAdmin_MissingEnvVarsDoesNotCreate(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	logs := captureLogs(t)
	err := bootstrap.EnsureFirstAdmin(ctx, bunDB, "", "")
	require.NoError(t, err, "missing env vars must not error; only warn")

	var n int
	require.NoError(t, bunDB.NewSelect().TableExpr("users").
		ColumnExpr("count(*)").Scan(ctx, &n))
	assert.Equal(t, 0, n, "no user should be created when env vars are empty")

	// Warning must mention the situation so an operator knows to act.
	assert.Contains(t, logs.String(), "BOOTSTRAP_ADMIN_EMAIL")
}

func TestEnsureFirstAdmin_NilDBReturnsError(t *testing.T) {
	err := bootstrap.EnsureFirstAdmin(context.Background(), nil, "x@y.com", "pw")
	require.Error(t, err)
}

// TestEnsureFirstAdmin_PasswordNeverLogged runs a parametrized check across
// many "dangerous" password shapes (multi-line, SQL injection-like, JSON) and
// asserts none of them ever appear in logs. bcrypt rejects passwords longer
// than 72 bytes, so we cap test fixtures at 70 bytes — still large enough
// to exercise the log-safety invariant for varied shapes.
func TestEnsureFirstAdmin_PasswordNeverLogged(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	dangerousPasswords := []string{
		"line1\nline2\nline3",   // multi-line
		`' OR 1=1 --`,           // SQL-injection shape
		`{"inject":"value"}`,    // JSON literal
		strings.Repeat("x", 70), // long-but-valid
		"mixed\x00null\x01control-bytes-and-more-padding-here", // control bytes
	}
	for i, pw := range dangerousPasswords {
		_, err := bunDB.ExecContext(ctx, "TRUNCATE TABLE users RESTART IDENTITY CASCADE")
		require.NoError(t, err, "reset users before iteration %d", i)

		logs := captureLogs(t)
		email := "root" + string(rune('0'+i)) + "@example.com"
		err = bootstrap.EnsureFirstAdmin(ctx, bunDB, email, pw)
		require.NoError(t, err, "iteration %d", i)

		logged := logs.String()
		assert.False(t, strings.Contains(logged, pw),
			"iteration %d: password leaked into logs", i)
	}
}
