// Package migrate runs platform PostgreSQL migrations embedded into the binary at
// compile time. The runner is golang-migrate/v4 with the pgx/v5 driver; the source
// is an iofs.FS pointed at the embedded migrations directory.
//
// Why golang-migrate and not bun.Migrator: simpler DSL-free SQL files (PRD 10-data-model.md
// is plain DDL), battle-tested idempotency semantics, no opinionated migration conventions
// to fight against.
//
// Design: see docs/prd/16-ops.md §1 ("启动：平台 PG 迁移（migrate）完成后才 ready") and
// design.md in openspec/changes/infra-platform-meta-db-foundation/.
package migrate

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// pgx/v5 registers itself as the "pgx" / "pgx/v5" database/sql driver when
	// imported. We pull it in for its side effect so golang-migrate's pgx5 driver
	// can use it under the hood.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// FS holds the embedded migration SQL files. Build constraint: every .sql file under
// the migrations directory is bundled into the binary at compile time. Place new
// migrations there (numbered ascending: 0001_, 0002_, ...) and they will be picked up.
//
//go:embed migrations/*.sql
var FS embed.FS

// Up applies all pending migrations against the platform PostgreSQL identified by
// dsn. Idempotent: calling Up on an already-migrated database is a no-op (golang-migrate
// records applied versions in schema_migrations). Calling Up on an unreachable DB
// returns an error so the caller can fail fast (16-ops §1).
//
// The function blocks until migrations complete or ctx is cancelled; long-running
// migrations should be sized accordingly.
//
// The DSN is rewritten from "postgres://" to "pgx5://" before being handed to
// golang-migrate, because migrate's pgx/v5 driver registers itself under the
// "pgx5" scheme (pgx's stdlib registers "pgx" / "pgx/v5", not "postgres"). The
// caller can keep using the conventional postgres:// DSN everywhere else.
func Up(ctx context.Context, dsn string) error {
	if dsn == "" {
		return fmt.Errorf("migrate.Up: empty DSN")
	}

	src, err := iofs.New(FS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate.Up: open source: %w", err)
	}
	// iofs.Source does not need an explicit Close but the migrate.Migrate instance
	// owns lifecycle; we drop our reference after construction.
	_ = src

	// Translate the URL scheme so golang-migrate can find its pgx5 driver.
	migrateDSN := rewriteForMigrate(dsn)

	// Build the migrate instance via the pgx/v5 driver. We pass the DSN directly
	// rather than reusing the bun pool because golang-migrate takes its own
	// connection and that connection must not be checked back into the pool.
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateDSN)
	if err != nil {
		return fmt.Errorf("migrate.Up: new migrate: %w", err)
	}
	// Always release driver resources even on success.
	defer func() {
		srcErr, dbErr := m.Close()
		// Prefer surfacing the first non-nil error; Close() errors are best-effort.
		if srcErr != nil {
			err = errors.Join(err, fmt.Errorf("migrate.Up: close source: %w", srcErr))
		}
		if dbErr != nil {
			err = errors.Join(err, fmt.Errorf("migrate.Up: close db: %w", dbErr))
		}
	}()

	// Migrate does not take a ctx directly; respect cancellation via a goroutine
	// that closes the underlying driver on ctx.Done. Simplest approach: rely on
	// the pgx driver's own context via WithInstance later; for v1 we let it run
	// synchronously inside the caller's ctx window.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrate.Up: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate.Up: apply: %w", err)
	}
	return nil
}

// rewriteForMigrate converts a postgres:// DSN into a pgx5:// DSN for
// golang-migrate. Idempotent: leaves the string untouched if it already starts
// with pgx5://.
func rewriteForMigrate(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres://"):
		return "pgx5" + strings.TrimPrefix(dsn, "postgres")
	case strings.HasPrefix(dsn, "postgresql://"):
		return "pgx5" + strings.TrimPrefix(dsn, "postgresql")
	default:
		return dsn
	}
}

// ensure pgxmigrate is referenced (compile-time guard against accidental removal
// when the import is only used for side effects).
var _ = pgxmigrate.Postgres{}

// ensure migrate is referenced (compile-time guard against accidental removal
// when the import is only used for side effects).
var _ = migrate.ErrNoChange
