package bootstrap

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsUniqueViolation covers the helper used to translate a PostgreSQL
// unique-violation error into "already bootstrapped, treat as success".
// Exercising it directly is the only way to cover the pgconn.PgError
// branch without simulating a concurrent insert against a real DB.
func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error -> false",
			err:  nil,
			want: false,
		},
		{
			name: "non-pg error -> false",
			err:  errors.New("some random error"),
			want: false,
		},
		{
			name: "pg unique violation -> true",
			err: &pgconn.PgError{
				Code:       pgerrcode.UniqueViolation,
				Message:    "duplicate key value violates unique constraint",
				Detail:     "Key (lower(email))=(root@example.com) already exists.",
				TableName:  "users",
				ColumnName: "email",
			},
			want: true,
		},
		{
			name: "pg other code -> false",
			err: &pgconn.PgError{
				Code:    pgerrcode.NotNullViolation,
				Message: "null value in column \"email\" violates not-null constraint",
			},
			want: false,
		},
		{
			name: "wrapped pg unique violation -> true",
			err: errors.Join(
				errors.New("outer wrapper"),
				&pgconn.PgError{
					Code:    pgerrcode.UniqueViolation,
					Message: "wrapped unique violation",
				},
			),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isUniqueViolation(tc.err)
			if got != tc.want {
				t.Fatalf("isUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
