package api

import (
	"context"
	"io"
	"testing"

	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRepo 返回固定项目 + 固定 next token，用于断言 handler 的映射逻辑。
type stubRepo struct {
	items []service.Project
}

func (s stubRepo) List(_ context.Context, _ int, _ string) ([]service.Project, string, error) {
	return s.items, "next-token", nil
}

// recordingRepo 记录收到的 limit，用于断言参数透传。
type recordingRepo struct{ seen *int }

func (r *recordingRepo) List(_ context.Context, limit int, _ string) ([]service.Project, string, error) {
	*r.seen = limit
	return nil, "", nil
}

func TestHandler_ListProjects_MapsDomainToContract(t *testing.T) {
	h := NewHandler(service.NewProjectService(stubRepo{items: []service.Project{
		{Key: "orders", Title: "Orders", Description: "order mgmt"},
	}}), nil)

	res, err := h.ListProjects(context.Background(), oas.ListProjectsParams{})
	require.NoError(t, err)

	got, ok := res.(*oas.ListProjectsResponse)
	require.True(t, ok)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "orders", got.Items[0].Key.Value)
	assert.Equal(t, "projects/orders", got.Items[0].Name.Value)
	assert.Equal(t, "Orders", got.Items[0].Title.Value)
	assert.Equal(t, "next-token", got.NextPageToken.Value)
	assert.True(t, got.NextPageToken.Set)
}

func TestHandler_ListProjects_PassesPageSize(t *testing.T) {
	var seen int
	h := NewHandler(service.NewProjectService(&recordingRepo{seen: &seen}), nil)

	_, err := h.ListProjects(context.Background(), oas.ListProjectsParams{
		PageSize: oas.OptInt{Value: 7, Set: true},
	})
	require.NoError(t, err)
	assert.Equal(t, 7, seen)
}

func TestHandler_ListProjects_DefaultsPageSizeWhenAbsent(t *testing.T) {
	var seen int
	h := NewHandler(service.NewProjectService(&recordingRepo{seen: &seen}), nil)

	_, err := h.ListProjects(context.Background(), oas.ListProjectsParams{})
	require.NoError(t, err)
	assert.Equal(t, service.DefaultPageSize, seen)
}

func TestHandler_Healthz(t *testing.T) {
	h := NewHandler(service.NewProjectService(stubRepo{}), nil)
	res, err := h.Healthz(context.Background())
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Data)
	assert.Equal(t, "ok", string(body))
}

func TestHandler_Readyz_NilDBReturnsOK(t *testing.T) {
	// When no platform DB is wired (unit test mode), /readyz stays ready.
	h := NewHandler(service.NewProjectService(stubRepo{}), nil)
	res, err := h.Readyz(context.Background())
	require.NoError(t, err)
	_, ok := res.(*oas.ReadyzOK)
	assert.True(t, ok, "expected ReadyzOK with nil platform DB")
}

// TestHandler_Readyz_PinguFailureReturnsContractError exercises the real-PG
// failure path without spinning up Postgres: we inject a stub bun.DB whose
// PingContext always errors. The returned *oas.Error must carry the unified
// contract shape with details[].reason = DB_NOT_READY.
func TestHandler_Readyz_PingFailureReturnsContractError(t *testing.T) {
	failing := newFailingPlatformDB(t)
	h := NewHandler(service.NewProjectService(stubRepo{}), failing)

	res, err := h.Readyz(context.Background())
	require.NoError(t, err)
	oasErr, ok := res.(*oas.Error)
	require.True(t, ok, "expected *oas.Error on ping failure, got %T", res)
	assert.Equal(t, 503, oasErr.Code)
	require.Len(t, oasErr.Details, 1)
	assert.Equal(t, service.ReasonDBNotReady, oasErr.Details[0].Reason.Value)
	assert.Equal(t, service.ErrorDomain, oasErr.Details[0].Domain.Value)
	assert.Equal(t, service.ErrorInfoType, oasErr.Details[0].Type.Value)
}
