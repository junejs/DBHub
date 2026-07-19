// Command migrate applies the embedded platform PostgreSQL migrations against
// the DSN supplied via the DB_DSN environment variable. It is a thin CLI
// wrapper around internal/infra/migrate.Up, intended for ops use when the
// server itself is not running (e.g. during a deploy that wants to run the
// migration step in isolation before rolling the new binary).
//
// Migrations are also auto-applied at server startup (16-ops §1); this CLI
// exists so the same code path can be invoked out-of-band.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/junepy/dbhub/backend/internal/infra/migrate"
)

func main() {
	code := run()
	os.Exit(code)
}

func run() int {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DB_DSN not set; refusing to run migrations against an unknown DB.")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := migrate.Up(ctx, dsn); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 1
	}
	fmt.Println("migrations applied")
	return 0
}
