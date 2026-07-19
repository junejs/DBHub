// Package db wires the platform PostgreSQL connection pool used by all service-layer
// repos (D45: pgx/v5 + bun). The single source of DSN is the DB_DSN environment variable
// (16-ops §2). Pool lifecycle: caller owns the returned *bun.DB and is responsible for
// calling Close() at shutdown.
//
// Two layers:
//  1. pgx/v5 ConnConfig (parsed from DSN) -> stdlib.GetConnector -> *sql.DB
//  2. *sql.DB + pgdialect.New() -> bun.NewDB
//
// We deliberately expose database/sql rather than pgxpool directly so the same DSN
// can be passed to golang-migrate's pgx/v5 driver (see package migrate) without
// opening a second pool against the same database.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// Open builds a bun.DB on top of pgx/v5's database/sql shim with the PostgreSQL dialect.
func Open(ctx context.Context, dsn string) (*bun.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("db.Open: empty DSN")
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db.Open: parse DSN: %w", err)
	}

	sqlDB := sql.OpenDB(stdlib.GetConnector(*cfg))
	// Reasonable defaults for a single-node MVP (D24). Tunable later via env.
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db.Open: ping: %w", err)
	}

	bunDB := bun.NewDB(sqlDB, pgdialect.New())
	return bunDB, nil
}

// Ping verifies the platform DB is reachable. Used by /readyz and the migration
// preflight. Uses a short timeout so an unreachable DB fails fast.
func Ping(ctx context.Context, bunDB *bun.DB) error {
	if bunDB == nil {
		return fmt.Errorf("db.Ping: nil bun.DB")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := bunDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("db.Ping: %w", err)
	}
	return nil
}
