package migrate

import "testing"

// TestRewriteForMigrate covers every branch of the unexported DSN-scheme
// rewriter. The function translates postgres:// and postgresql:// DSNs into
// pgx5:// (the scheme golang-migrate's pgx/v5 driver registers under), and
// leaves anything else untouched. Testing it directly is the only way to
// exercise the postgresql:// and default branches — every container-backed
// test happens to use the postgres:// form, so without this unit test the
// postgresql:// branch and the identity branch stay uncovered.
func TestRewriteForMigrate(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "postgres scheme rewritten to pgx5",
			dsn:  "postgres://user:pw@host:5432/db?sslmode=disable",
			want: "pgx5://user:pw@host:5432/db?sslmode=disable",
		},
		{
			name: "postgresql scheme rewritten to pgx5",
			dsn:  "postgresql://user:pw@host:5432/db",
			want: "pgx5://user:pw@host:5432/db",
		},
		{
			name: "already pgx5 left unchanged",
			dsn:  "pgx5://user:pw@host:5432/db",
			want: "pgx5://user:pw@host:5432/db",
		},
		{
			name: "keyword DSN left unchanged (no scheme to translate)",
			dsn:  "host=localhost port=5432 user=dbhub",
			want: "host=localhost port=5432 user=dbhub",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteForMigrate(tc.dsn)
			if got != tc.want {
				t.Fatalf("rewriteForMigrate(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
		})
	}
}
