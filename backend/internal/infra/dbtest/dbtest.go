// Package dbtest spins up a real PostgreSQL container for integration tests.
// Kept in its own package (rather than under _test.go files) so any test in the
// module can call NewTestDB without rebuilding the same helper.
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/junepy/dbhub/backend/internal/infra/db"
)

// NewPostgres spins up a fresh Postgres 17 container, returns a connected
// *bun.DB, the raw DSN, and a cleanup function the caller defers. The DSN is
// also returned because golang-migrate wants the DSN, not the bun pool.
//
// Tests must be skipped if Docker is not available; we do NOT silently fall back
// to SQLite or anything else (per 16-ops §7.2: real PostgreSQL only).
func NewPostgres(ctx context.Context, t *testing.T) (*bun.DB, string, func()) {
	t.Helper()

	//nolint:staticcheck // RunContainer is the documented v0.43 entry point.
	pgC, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17"),
		tcpostgres.WithDatabase("dbhub_test"),
		tcpostgres.WithUsername("dbhub"),
		tcpostgres.WithPassword("dbhub"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start postgres container")

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "get DSN")

	bunDB, err := db.Open(ctx, dsn)
	require.NoError(t, err, "open bun DB")

	cleanup := func() {
		_ = bunDB.Close()
		if err := pgC.Terminate(context.Background()); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	}
	return bunDB, dsn, cleanup
}

// NewPostgresSQLDB returns just the raw *sql.DB for tests that need to talk to
// the underlying pgx connection without going through bun (e.g. golang-migrate).
func NewPostgresSQLDB(ctx context.Context, t *testing.T) (*sql.DB, string, func()) {
	t.Helper()

	//nolint:staticcheck // RunContainer is the documented v0.43 entry point.
	pgC, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:17"),
		tcpostgres.WithDatabase("dbhub_test"),
		tcpostgres.WithUsername("dbhub"),
		tcpostgres.WithPassword("dbhub"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start postgres container")

	dsn, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "get DSN")

	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err, "parse DSN")

	sqlDB := sql.OpenDB(stdlib.GetConnector(*cfg))
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, sqlDB.PingContext(pingCtx), "ping SQL DB")

	cleanup := func() {
		_ = sqlDB.Close()
		if err := pgC.Terminate(context.Background()); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	}
	return sqlDB, dsn, cleanup
}

// CountRows returns the total count for the given table expression. Used by
// idempotency tests to assert that re-running seed/migrate does not duplicate
// rows. Panics on error; call only after migrations have run successfully.
func CountRows(ctx context.Context, bunDB *bun.DB, tableExpr string) int {
	var n int
	q := bunDB.NewSelect().TableExpr(tableExpr).ColumnExpr("count(*)")
	if err := q.Scan(ctx, &n); err != nil {
		panic(fmt.Sprintf("CountRows(%s): %v", tableExpr, err))
	}
	return n
}

// DialectName returns the bun dialect name; used by tests that branch on it.
func DialectName(bunDB *bun.DB) string {
	return bunDB.Dialect().Name().String()
}

// silence unused import lint when file is referenced but pgdialect isn't used.
var _ = pgdialect.New
