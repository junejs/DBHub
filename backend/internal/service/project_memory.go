package service

import "context"

// MemoryProjectRepo 是基于内存的 ProjectRepo，用于 dev/demo，直至 bun/pgx 接入。
// 注意：pageToken 在此实现中被忽略（真实实现用 keyset 游标）。
type MemoryProjectRepo struct {
	items []Project
}

func NewMemoryProjectRepo(items []Project) *MemoryProjectRepo {
	return &MemoryProjectRepo{items: items}
}

func (m *MemoryProjectRepo) List(_ context.Context, limit int, _ string) ([]Project, string, error) {
	if limit <= 0 {
		limit = len(m.items)
	}
	if limit > len(m.items) {
		limit = len(m.items)
	}
	return m.items[:limit], "", nil
}
