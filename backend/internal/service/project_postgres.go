// Package service — bun-backed ProjectRepo that reads from the `projects` table.
//
// Keyset pagination: ordered by projects.key ASC; the opaque page_token is the
// key of the last row returned. Soft-deleted rows (deleted_at IS NOT NULL) are
// always excluded. limit is clamped at the service layer; the repo receives a
// pre-normalized positive limit.
package service

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/junepy/dbhub/backend/internal/service/projectsqlc"
)

// BunProjectRepo is the production ProjectRepo backed by PostgreSQL via bun.
//
// List signature mirrors ProjectRepo; pageToken is the last seen `key` value;
// nextPageToken is empty when there are no more rows (caller stops paginating).
type BunProjectRepo struct {
	db *bun.DB
}

// NewBunProjectRepo constructs a bun-backed ProjectRepo. Caller owns db lifecycle.
func NewBunProjectRepo(db *bun.DB) *BunProjectRepo {
	return &BunProjectRepo{db: db}
}

// List implements ProjectRepo using keyset pagination on projects.key.
//
// Implementation notes:
//   - `where deleted_at is null` enforces the partial-unique-index semantics (10-data-model §3.2).
//   - limit + 1 trick: fetch one extra row beyond `limit` to know whether more pages exist.
//     If we got `limit + 1` rows, the last one is dropped from the result and its key
//     becomes nextPageToken; otherwise nextPageToken is empty.
//   - Empty page_token means "start from the beginning" (no WHERE clause on key).
func (r *BunProjectRepo) List(ctx context.Context, limit int, pageToken string) ([]Project, string, error) {
	if r.db == nil {
		return nil, "", fmt.Errorf("BunProjectRepo.List: nil db")
	}
	if limit <= 0 {
		return nil, "", fmt.Errorf("BunProjectRepo.List: non-positive limit %d", limit)
	}

	// Fetch limit + 1 so we can detect whether another page exists.
	const fetchExtra = 1
	rows, err := fetchProjectsPage(ctx, r.db, pageToken, limit+fetchExtra)
	if err != nil {
		return nil, "", fmt.Errorf("BunProjectRepo.List: %w", err)
	}

	var nextToken string
	if len(rows) > limit {
		// Drop the probe row; the next page resumes after the LAST RETURNED row
		// (rows[limit-1].Key), not after the probe row. With where key > token,
		// token = rows[limit-1].Key means rows[limit-1] is the boundary — the
		// next call picks up from rows[limit] onwards, which we just dropped.
		rows = rows[:limit]
		nextToken = rows[len(rows)-1].Key
	}

	items := make([]Project, len(rows))
	for i, row := range rows {
		items[i] = Project{
			Key:         row.Key,
			Title:       row.Name,
			Description: "",
		}
	}
	return items, nextToken, nil
}

// fetchProjectsPage runs the SELECT, optionally scoped by pageToken, and returns
// at most `fetchLimit` rows ordered by key ASC. Split out so the limit+1 trick
// is contained.
func fetchProjectsPage(ctx context.Context, db *bun.DB, pageToken string, fetchLimit int) ([]projectsqlc.Project, error) {
	q := db.NewSelect().
		TableExpr("projects AS p").
		Column("p.key", "p.name").
		Where("p.deleted_at IS NULL").
		OrderExpr("p.key ASC").
		Limit(fetchLimit)
	if pageToken != "" {
		q = q.Where("p.key > ?", pageToken)
	}
	var rows []projectsqlc.Project
	if err := q.Scan(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
