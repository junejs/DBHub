package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeRepo 返回固定结果，用于断言业务逻辑（不触碰真实存储）。
type fakeRepo struct {
	items []Project
}

func (f *fakeRepo) List(_ context.Context, _ int, _ string) ([]Project, string, error) {
	return f.items, "", nil
}

func TestProjectService_List_ReturnsItems(t *testing.T) {
	svc := NewProjectService(&fakeRepo{items: []Project{{Key: "orders", Title: "Orders"}}})

	got, next, err := svc.List(context.Background(), 0, "")
	assert.NoError(t, err)
	assert.Equal(t, "", next)
	assert.Len(t, got, 1)
	assert.Equal(t, "orders", got[0].Key)
}

// limitRecorder 记录收到的 limit，用于断言归一化逻辑。
type limitRecorder struct{ seen *int }

func (l *limitRecorder) List(_ context.Context, limit int, _ string) ([]Project, string, error) {
	*l.seen = limit
	return nil, "", nil
}

func TestProjectService_List_NormalizesPageSize(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, DefaultPageSize},
		{-1, DefaultPageSize},
		{MaxPageSize + 1, DefaultPageSize},
		{10, 10},
		{MaxPageSize, MaxPageSize},
	}
	for _, c := range cases {
		var seen int
		svc := NewProjectService(&limitRecorder{seen: &seen})
		_, _, _ = svc.List(context.Background(), c.in, "")
		assert.Equalf(t, c.want, seen, "input limit=%d", c.in)
	}
}
