// Package service contains hand-written business logic, kept independent of the
// generated transport layer (internal/oas). The HTTP handlers in internal/api
// adapt these services to the ogen-generated Handler interface.
package service

import "context"

// 分页上下界，与契约 page_size 约定一致（12-api-contract.md §3.2）。
const (
	DefaultPageSize = 50
	MaxPageSize     = 5000
)

// Project 是项目的领域表示（团队隔离边界）。
type Project struct {
	Key         string // projects/{key}
	Title       string
	Description string
}

// ProjectRepo 是项目的持久化端口。生产实现可包装 bun/pgx；测试注入 fake（D50：interface + mock 注入）。
type ProjectRepo interface {
	List(ctx context.Context, limit int, pageToken string) (items []Project, nextPageToken string, err error)
}

// ProjectService 实现项目列表等用例。
//
// TODO(v1): List 须返回调用者可见的项目（成员所属项目 + workspaceAdmin 全见）。
// 鉴权由横切层按契约 x-requires-permission 驱动，调用者身份经 ctx 注入。
type ProjectService struct {
	repo ProjectRepo
}

func NewProjectService(repo ProjectRepo) *ProjectService {
	return &ProjectService{repo: repo}
}

// List 返回一页项目；越界的 limit 归一到默认值。
func (s *ProjectService) List(ctx context.Context, limit int, pageToken string) ([]Project, string, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	return s.repo.List(ctx, limit, pageToken)
}
