// Package api adapts hand-written services (internal/service) to the
// ogen-generated transport layer (internal/oas). Handlers here are intentionally
// thin: ogen decodes the request → handler calls the service → ogen encodes the
// response. Domain logic stays in the service package and is unit-tested there.
package api

import (
	"context"
	"strings"

	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
)

// Handler implements oas.Handler by embedding the generated UnimplementedHandler
// (which returns 501 for every operation) and overriding only the operations
// already implemented. As features land, override more methods here; the rest
// stay 501 until then.
type Handler struct {
	oas.UnimplementedHandler
	projects *service.ProjectService
}

func NewHandler(projects *service.ProjectService) *Handler {
	return &Handler{projects: projects}
}

// Healthz implements healthz.
func (h *Handler) Healthz(_ context.Context) (oas.HealthzOK, error) {
	return oas.HealthzOK{Data: strings.NewReader("ok")}, nil
}

// Readyz implements readyz.
func (h *Handler) Readyz(_ context.Context) (oas.ReadyzRes, error) {
	// TODO(v1): 校验平台元数据库连通后再返回 ready。
	return &oas.ReadyzOK{Data: strings.NewReader("ready")}, nil
}

// ListProjects implements listProjects.
func (h *Handler) ListProjects(ctx context.Context, params oas.ListProjectsParams) (oas.ListProjectsRes, error) {
	limit := service.DefaultPageSize
	if params.PageSize.Set {
		limit = params.PageSize.Value
	}
	pageToken := ""
	if params.PageToken.Set {
		pageToken = params.PageToken.Value
	}

	items, next, err := h.projects.List(ctx, limit, pageToken)
	if err != nil {
		// 返回 err：ogen 经 ErrorHandler 映射为 500。
		return nil, err
	}

	out := make([]oas.Project, len(items))
	for i, p := range items {
		out[i] = toOasProject(p)
	}
	resp := &oas.ListProjectsResponse{}
	resp.SetItems(out)
	if next != "" {
		resp.SetNextPageToken(oas.NewOptString(next))
	}
	return resp, nil
}

// toOasProject 将领域 Project 映射为契约类型（传输层与领域层解耦）。
func toOasProject(p service.Project) oas.Project {
	return oas.Project{
		Name:        oas.NewOptString("projects/" + p.Key),
		Key:         oas.NewOptString(p.Key),
		Title:       oas.NewOptString(p.Title),
		Description: oas.NewOptString(p.Description),
	}
}
