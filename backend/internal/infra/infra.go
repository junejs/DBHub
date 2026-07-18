// Package infra declares backend infrastructure dependencies.
// This file ensures intended framework versions are locked in go.mod.
package infra

import (
	_ "github.com/go-chi/chi/v5"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/ogen-go/ogen"
	_ "github.com/uptrace/bun"
)
