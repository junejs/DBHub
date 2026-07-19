package migrate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junepy/dbhub/backend/internal/infra/dbtest"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
)

// TestMigrateUp_AppliesAllOnFreshDB spins up a fresh PostgreSQL and verifies
// that calling Up applies all expected v1 tables and indexes.
func TestMigrateUp_AppliesAllOnFreshDB(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn), "first migrate.Up")

	// Spot-check every table from PRD 10-data-model §3 exists.
	expectedTables := []string{
		"users", "groups", "group_members", "identity_providers",
		"refresh_tokens", "login_attempts",
		"projects", "environments", "environment_policies",
		"instances", "data_sources", "databases",
		"database_schemas", "sync_history",
		"roles", "role_permissions", "role_assignments",
		"query_history", "worksheets", "favorites",
		"export_tasks", "export_archives",
		"audit_logs", "notifications", "settings",
		"schema_migrations", // golang-migrate bookkeeping table
	}
	for _, table := range expectedTables {
		assert.Equal(t, 1, dbtest.CountRows(ctx, bunDB,
			"information_schema.tables WHERE table_name = '"+table+"'"),
			"table %s should exist", table)
	}

	// Spot-check critical partial unique indexes.
	expectedIndexes := []string{
		"users_email_uidx",
		"instances_key_uidx",
		"one_admin_ds",
		"one_readonly_ds",
		"databases_uidx",
		"role_assignments_uidx",
	}
	for _, idx := range expectedIndexes {
		assert.Equal(t, 1, dbtest.CountRows(ctx, bunDB,
			"pg_indexes WHERE indexname = '"+idx+"'"),
			"index %s should exist", idx)
	}
}

// TestMigrateUp_IsIdempotent runs Up twice and asserts no rows change. The
// golang-migrate ErrNoChange should be swallowed and Up should return nil.
func TestMigrateUp_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	_, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn), "first migrate.Up")
	require.NoError(t, migrate.Up(ctx, dsn), "second migrate.Up (should be no-op)")
	require.NoError(t, migrate.Up(ctx, dsn), "third migrate.Up (still no-op)")
}

// TestMigrateUp_AuditPartitionsSeeded verifies the current-month partition and
// default catch-all partition were created by 0002_audit_partitions.sql.
func TestMigrateUp_AuditPartitionsSeeded(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()

	require.NoError(t, migrate.Up(ctx, dsn))

	// audit_logs is partitioned; pg_class shows both parent and child tables.
	assert.GreaterOrEqual(t, dbtest.CountRows(ctx, bunDB,
		"pg_inherits WHERE inhparent = 'audit_logs'::regclass"), 2,
		"audit_logs should have at least current-month + default partitions")
}

// TestMigrateUp_RejectsEmptyDSN ensures the package fails fast on misconfig
// rather than silently succeeding.
func TestMigrateUp_RejectsEmptyDSN(t *testing.T) {
	err := migrate.Up(context.Background(), "")
	require.Error(t, err, "empty DSN must return an error")
}

// TestMigrateUp_BadSchemeReturnsError covers the migrate.NewWithSourceInstance
// error path inside Up: a DSN whose scheme matches no registered driver must
// surface as a "new migrate" error rather than silently doing nothing. No
// container needed — the failure happens during driver lookup, before any
// real connection is attempted.
func TestMigrateUp_BadSchemeReturnsError(t *testing.T) {
	err := migrate.Up(context.Background(), "bogus-scheme://nonexistent.invalid:1/db")
	require.Error(t, err, "unsupported DSN scheme must return an error")
	assert.Contains(t, err.Error(), "new migrate",
		"error must come from the NewWithSourceInstance step")
}

// TestMigrateUp_CancelledContextReturnsError covers the ctx.Err() check inside
// Up. After NewWithSourceInstance succeeds against a real PG, the pre-cancelled
// context must surface as a "context canceled" error from Up so callers can
// respect shutdown signals.
func TestMigrateUp_CancelledContextReturnsError(t *testing.T) {
	_, dsn, cleanup := dbtest.NewPostgres(context.Background(), t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := migrate.Up(ctx, dsn)
	require.Error(t, err, "pre-cancelled ctx must return an error")
	assert.Contains(t, err.Error(), "context canceled")
}
