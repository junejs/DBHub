// Package infra declares backend infrastructure dependencies.
// This file ensures intended framework versions are locked in go.mod.
// It is imported by main.go for its side effects (the `_` imports prevent
// go mod tidy from removing pins for packages we use indirectly).
package infra

import (
	_ "github.com/go-chi/chi/v5"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/ogen-go/ogen"
	_ "github.com/uptrace/bun"
	_ "github.com/uptrace/bun/driver/pgdriver"
	_ "golang.org/x/crypto/bcrypt"
)
