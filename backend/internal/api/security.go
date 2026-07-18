package api

import (
	"context"

	"github.com/junepy/dbhub/backend/internal/oas"
)

// SecurityHandler 实现 oas.SecurityHandler。
//
// 安全提示：这是放行占位实现，仅用于脚手架阶段让服务可启动——它接受任意凭据。
// 真正的认证（JWT / Cookie 校验）+ 由 x-requires-permission 驱动的 ACL 横切层，
// 必须在任何部署前替换此处。
type SecurityHandler struct{}

func NewSecurityHandler() *SecurityHandler { return &SecurityHandler{} }

// HandleBearerAuth 校验 API 端 Bearer（JWT）令牌。
func (SecurityHandler) HandleBearerAuth(ctx context.Context, operationName oas.OperationName, t oas.BearerAuth) (context.Context, error) {
	_ = operationName
	_ = t
	// TODO(v1): 校验并解析 JWT（t.Token），将调用者身份注入 ctx。
	return ctx, nil
}

// HandleCookieAuth 校验 Web 端会话 Cookie。
func (SecurityHandler) HandleCookieAuth(ctx context.Context, operationName oas.OperationName, t oas.CookieAuth) (context.Context, error) {
	_ = operationName
	_ = t
	// TODO(v1): 校验 access-token cookie / 刷新。
	return ctx, nil
}
