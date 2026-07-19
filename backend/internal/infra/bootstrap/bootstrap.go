// Package bootstrap implements the one-time, idempotent creation of the platform's
// first admin user from environment variables (PRD 16-ops §2 BOOTSTRAP_ADMIN_*).
//
// Contract:
//   - On an empty DB and with non-empty BOOTSTRAP_ADMIN_EMAIL + BOOTSTRAP_ADMIN_PASSWORD,
//     insert a user (kind='user', status='active', source='local') with a bcrypt(cost=12)
//     password hash and log a success line containing only the user id.
//   - On a DB that already has at least one active user, do nothing and log "skip"
//     (the env vars are no-ops).
//   - On unique-violation of users_email_uidx (concurrent startup inserting the same
//     email), treat as success: the other process already won.
//   - On empty env vars on an empty DB, log a warning and continue startup.
//   - NEVER log the password or its hash.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is fixed at 12 per PRD 16-ops §3.
const BcryptCost = 12

// EnsureFirstAdmin makes sure at least one active user exists; if not, it creates
// one using email + password (must be non-empty). Returns nil if the database is
// already bootstrapped OR the bootstrap succeeded. Returns an error only on real
// failures (DB unreachable, bcrypt failure, etc.).
//
// The function logs only the user id and the outcome; the password is never
// referenced in any log line.
func EnsureFirstAdmin(ctx context.Context, db *bun.DB, email, password string) error {
	if db == nil {
		return fmt.Errorf("bootstrap.EnsureFirstAdmin: nil bun.DB")
	}

	// 1. Any active user already present? -> nothing to do.
	exists, err := hasAnyActiveUser(ctx, db)
	if err != nil {
		return fmt.Errorf("bootstrap.EnsureFirstAdmin: check existing user: %w", err)
	}
	if exists {
		slog.Info("bootstrap: first admin already exists; skipping")
		return nil
	}

	// 2. Empty env vars on empty DB -> warn and continue.
	if email == "" || password == "" {
		slog.Warn("bootstrap: empty database and BOOTSTRAP_ADMIN_EMAIL/PASSWORD not set; " +
			"no admin user will be created. Set env vars or create an admin manually.")
		return nil
	}

	// 3. Hash and insert.
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return fmt.Errorf("bootstrap.EnsureFirstAdmin: hash password: %w", err)
	}

	user := newUser{
		Email:        email,
		Name:         email, // display name defaults to email until admin updates it
		Status:       "active",
		Source:       "local",
		PasswordHash: string(hash),
	}

	if _, err := db.NewInsert().Model(&user).Exec(ctx); err != nil {
		// Concurrent startup inserted the same email -> treat as already bootstrapped.
		if isUniqueViolation(err) {
			slog.Info("bootstrap: first admin email already taken by concurrent process; skipping")
			return nil
		}
		return fmt.Errorf("bootstrap.EnsureFirstAdmin: insert user: %w", err)
	}

	// Look up the inserted id (citext keeps the case-insensitive lookup safe).
	var gotID int64
	if err := db.NewSelect().TableExpr("users").Column("id").
		Where("email = ?", email).Scan(ctx, &gotID); err != nil {
		return fmt.Errorf("bootstrap.EnsureFirstAdmin: lookup inserted id: %w", err)
	}

	slog.Info("bootstrap: first admin created", "user_id", gotID, "status", "ok")
	return nil
}

func hasAnyActiveUser(ctx context.Context, db *bun.DB) (bool, error) {
	var count int64
	if err := db.NewSelect().
		TableExpr("users").
		ColumnExpr("count(*)").
		Where("deleted_at IS NULL").
		Scan(ctx, &count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgerrcode.UniqueViolation
	}
	return false
}

// newUser matches the users table columns we set. Local to this package.
// The `bun:"table:users"` opt is required because bun's inflector otherwise
// derives `newUser` -> `new_users`.
type newUser struct {
	bun.BaseModel `bun:"table:users"`
	Email         string `bun:"email"`
	Name          string `bun:"name"`
	Status        string `bun:"status"`
	Source        string `bun:"source"`
	PasswordHash  string `bun:"password_hash"`
}
