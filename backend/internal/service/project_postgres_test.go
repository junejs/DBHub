package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junepy/dbhub/backend/internal/infra/dbtest"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
	"github.com/junepy/dbhub/backend/internal/service"
)

// TestBunProjectRepo_ListReturnsRowsInKeyOrder checks that List returns rows
// ordered by `key` ascending, regardless of insertion order.
func TestBunProjectRepo_ListReturnsRowsInKeyOrder(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	// Insert in non-alphabetical order so we can prove ordering is by key.
	for _, key := range []string{"zeta", "alpha", "mike", "bravo", "yankee"} {
		_, err := bunDB.ExecContext(ctx,
			`INSERT INTO projects(key, name) VALUES (?, ?)`, key, "Project "+key)
		require.NoError(t, err)
	}

	repo := service.NewBunProjectRepo(bunDB)
	items, next, err := repo.List(ctx, 10, "")
	require.NoError(t, err)
	assert.Empty(t, next, "first page must signal end-of-pages with empty token")
	require.Len(t, items, 5)
	for i, want := range []string{"alpha", "bravo", "mike", "yankee", "zeta"} {
		assert.Equal(t, want, items[i].Key, "position %d", i)
	}
}

// TestBunProjectRepo_ListKeysetPagination walks through pages and asserts that
// page tokens correctly resume from where the previous page ended.
func TestBunProjectRepo_ListKeysetPagination(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	// Insert 7 rows with predictable keys.
	for _, key := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		_, err := bunDB.ExecContext(ctx,
			`INSERT INTO projects(key, name) VALUES (?, ?)`, key, "P-"+key)
		require.NoError(t, err)
	}

	repo := service.NewBunProjectRepo(bunDB)

	// Page 1: limit=3, token="" -> a, b, c
	items, next, err := repo.List(ctx, 3, "")
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "a", items[0].Key)
	assert.Equal(t, "c", items[2].Key)
	assert.Equal(t, "c", next, "next_page_token must be the last seen key")

	// Page 2: limit=3, token="c" -> d, e, f
	items, next, err = repo.List(ctx, 3, "c")
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "d", items[0].Key)
	assert.Equal(t, "f", items[2].Key)
	assert.Equal(t, "f", next)

	// Page 3: limit=3, token="f" -> g, then empty token (no more pages).
	items, next, err = repo.List(ctx, 3, "f")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "g", items[0].Key)
	assert.Empty(t, next, "no more rows -> empty next_page_token")
}

// TestBunProjectRepo_ListExcludesSoftDeleted ensures soft-deleted rows are not
// returned (per PRD 10-data-model §6 partial unique index semantics).
func TestBunProjectRepo_ListExcludesSoftDeleted(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	for _, key := range []string{"live1", "deleted1", "live2"} {
		_, err := bunDB.ExecContext(ctx,
			`INSERT INTO projects(key, name, deleted_at) VALUES (?, ?, ?)`,
			key, "P-"+key, nil)
		require.NoError(t, err)
	}
	_, err := bunDB.ExecContext(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE key = ?`, "deleted1")
	require.NoError(t, err)

	repo := service.NewBunProjectRepo(bunDB)
	items, _, err := repo.List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, it := range items {
		assert.NotEqual(t, "deleted1", it.Key)
	}
}

// TestBunProjectRepo_ListEmptyTable returns no items and no next token.
func TestBunProjectRepo_ListEmptyTable(t *testing.T) {
	ctx := context.Background()
	bunDB, dsn, cleanup := dbtest.NewPostgres(ctx, t)
	defer cleanup()
	require.NoError(t, migrate.Up(ctx, dsn))

	repo := service.NewBunProjectRepo(bunDB)
	items, next, err := repo.List(ctx, 10, "")
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Empty(t, next)
}

// TestBunProjectRepo_ListNilDBReturnsError guards against accidental
// nil-pointer panics if a caller forgets to inject the DB.
func TestBunProjectRepo_ListNilDBReturnsError(t *testing.T) {
	repo := service.NewBunProjectRepo(nil)
	_, _, err := repo.List(context.Background(), 10, "")
	require.Error(t, err)
}
