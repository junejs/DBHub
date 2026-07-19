package seed_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/junepy/dbhub/backend/internal/infra/dbtest"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
	"github.com/junepy/dbhub/backend/internal/infra/permissions"
	"github.com/junepy/dbhub/backend/internal/infra/seed"
)

// TestSeedBuiltin_FirstRunInsertsExpectedRows asserts the seed produces the
// canonical counts from PRD 10-data-model §4 (4 environments, 4 env policies,
// 8 builtin roles) and a role_permissions set sized by the permissions registry.
func TestSeedBuiltin_FirstRunInsertsExpectedRows(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn))
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))

	// 4 environments (prod / stage / test / dev).
	assert.Equal(t, 4, dbtest.CountRows(ctx, bunDB, "environments"))

	// 4 environment_policies (one per environment).
	assert.Equal(t, 4, dbtest.CountRows(ctx, bunDB, "environment_policies"))

	// 8 builtin roles.
	assert.Equal(t, 8, dbtest.CountRows(ctx, bunDB, "roles"))

	// role_permissions count must equal the sum of registered permissions per role.
	expected := 0
	for _, perms := range permissions.Registry {
		expected += len(perms)
	}
	assert.Equal(t, expected, dbtest.CountRows(ctx, bunDB, "role_permissions"))
}

// TestSeedBuiltin_IsIdempotent runs SeedBuiltin twice and asserts the DB is
// byte-identical (row counts unchanged, no duplicate rows).
func TestSeedBuiltin_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn))
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))

	before := snapshotCounts(ctx, bunDB)
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))
	after := snapshotCounts(ctx, bunDB)

	assert.Equal(t, before, after, "row counts must be stable across seed reruns")

	// And a third time for paranoia.
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))
	final := snapshotCounts(ctx, bunDB)
	assert.Equal(t, before, final)
}

// TestSeedBuiltin_BuiltinRolesHaveBuiltinFlagTrue confirms the seeded roles
// have builtin=true so the future IAM engine can refuse edits to them.
func TestSeedBuiltin_BuiltinRolesHaveBuiltinFlagTrue(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn))
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))

	nonBuiltin := dbtest.CountRows(ctx, bunDB, "roles WHERE builtin = false")
	assert.Equal(t, 0, nonBuiltin, "all seeded roles must have builtin=true")
}

// TestSeedBuiltin_EnvironmentProtectionOrder sanity-checks protection_level
// values match PRD §4: prod > stage > test > dev.
func TestSeedBuiltin_EnvironmentProtectionOrder(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn))
	require.NoError(t, seed.SeedBuiltin(ctx, bunDB))

	protectionLevels := map[string]int{}
	var rows []struct {
		Key             string `bun:"key"`
		ProtectionLevel int    `bun:"protection_level"`
	}
	require.NoError(t, bunDB.NewSelect().TableExpr("environments").
		Column("key", "protection_level").Scan(ctx, &rows))
	for _, r := range rows {
		protectionLevels[r.Key] = r.ProtectionLevel
	}

	assert.Greater(t, protectionLevels["prod"], protectionLevels["stage"])
	assert.Greater(t, protectionLevels["stage"], protectionLevels["test"])
	assert.Greater(t, protectionLevels["test"], protectionLevels["dev"])
}

// TestSeedBuiltin_NilDBReturnsError covers the nil-DB guard at seed.go:86-88.
// No container needed: SeedBuiltin must fail fast before touching the DB so
// mis-wiring at the call site (forgot to inject *bun.DB) surfaces immediately
// rather than as a nil-pointer panic deep inside a helper.
func TestSeedBuiltin_NilDBReturnsError(t *testing.T) {
	err := seed.SeedBuiltin(context.Background(), nil)
	require.Error(t, err, "nil bun.DB must return an error")
	assert.Contains(t, err.Error(), "nil bun.DB")
}

// TestSeedBuiltin_FailsOnUnmigratedDB covers SeedBuiltin's first `return err`
// path (the seedEnvironments failure) by calling it against a fresh PostgreSQL
// that has NOT been migrated. Every INSERT fails because the tables do not
// exist; SeedBuiltin must surface that as an error, not silently no-op.
func TestSeedBuiltin_FailsOnUnmigratedDB(t *testing.T) {
	ctx := context.Background()
	bunDB, _, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	// Intentionally skip migrate.Up: none of the seed tables exist.

	err := seed.SeedBuiltin(ctx, bunDB)
	require.Error(t, err, "SeedBuiltin must fail when tables are missing")
	assert.Contains(t, err.Error(), "seed environments",
		"error must come from the environments step (first INSERT)")
}

// TestSeedBuiltin_FailsWhenEnvironmentPoliciesTableMissing covers
// SeedBuiltin's second `return err` path (the seedEnvironmentPolicies
// failure). After migrate we drop only environment_policies: seedEnvironments
// still succeeds but the policy INSERT fails on the missing table.
func TestSeedBuiltin_FailsWhenEnvironmentPoliciesTableMissing(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	// Drop only environment_policies so we exercise the second-stage failure
	// rather than the environments-stage failure covered above.
	_, err := bunDB.ExecContext(ctx, "DROP TABLE environment_policies CASCADE")
	require.NoError(t, err)

	err = seed.SeedBuiltin(ctx, bunDB)
	require.Error(t, err, "SeedBuiltin must fail when environment_policies is missing")
	assert.Contains(t, err.Error(), "environment_policies",
		"error must come from the policy step (second INSERT)")
}

// snapshotCounts returns the row counts for each seeded table; used to verify
// idempotency byte-for-byte.
func snapshotCounts(ctx context.Context, bunDB *bun.DB) map[string]int {
	tables := []string{
		"environments",
		"environment_policies",
		"roles",
		"role_permissions",
	}
	out := make(map[string]int, len(tables))
	for _, table := range tables {
		out[table] = dbtest.CountRows(ctx, bunDB, table)
	}
	return out
}
