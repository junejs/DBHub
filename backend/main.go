package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/junepy/dbhub/backend/internal/api"
	"github.com/junepy/dbhub/backend/internal/infra/bootstrap"
	"github.com/junepy/dbhub/backend/internal/infra/config"
	"github.com/junepy/dbhub/backend/internal/infra/db"
	"github.com/junepy/dbhub/backend/internal/infra/httplog"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
	"github.com/junepy/dbhub/backend/internal/infra/seed"
	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
)

func main() {
	code := run()
	os.Exit(code)
}

func run() int {
	// 1) Load + validate config first. Required-field failures exit before
	// any external connection is opened (16-ops §2; ZZZ-85 验收标准).
	cfg, err := config.Load()
	if err != nil {
		// Use a fallback logger to stderr: we cannot trust env-derived
		// log config when env validation itself failed.
		fallback := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		fallback.Error("config load failed", "error", err)
		return 1
	}

	// 2) Build the request-scoped logger (16-ops §5, NFR §1.5). The default
	// logger carries request_id from the chi context whenever present.
	logger := httplog.NewLogger(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	// 3) Open platform DB pool. This is the single source of truth for the
	// metadata store (16-ops §2).
	platformDB, err := db.Open(context.Background(), cfg.DBDSN)
	if err != nil {
		logger.Error("open platform DB", "error", err)
		return 1
	}
	defer func() {
		if err := platformDB.Close(); err != nil {
			logger.Warn("close platform DB", "error", err)
		}
	}()

	// 4) Apply pending migrations (16-ops §1: migrate before ready).
	if err := migrate.Up(context.Background(), cfg.DBDSN); err != nil {
		logger.Error("apply migrations", "error", err)
		return 1
	}
	logger.Info("migrations applied")

	// 5) Seed builtin rows (environments, policies, roles, role_permissions).
	if err := seed.SeedBuiltin(context.Background(), platformDB); err != nil {
		logger.Error("seed builtin rows", "error", err)
		return 1
	}
	logger.Info("seed complete")

	// 6) Bootstrap first admin from env vars (idempotent; logs only the user id).
	if err := bootstrap.EnsureFirstAdmin(context.Background(), platformDB, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword); err != nil {
		logger.Error("bootstrap first admin", "error", err)
		return 1
	}

	// --- 装配：repo -> service -> (handler, security) -> ogen server ---
	projectRepo := service.NewBunProjectRepo(platformDB)
	projectSvc := service.NewProjectService(projectRepo)
	handler := api.NewHandler(projectSvc, platformDB)
	security := api.NewSecurityHandler()

	// ogen 从 openapi.yaml 生成的服务端（实现 http.Handler）。
	// 自定义 ErrorHandler 把 *service.Error 与未知错误都映射为契约 §4 的统一 Error 形状。
	oasServer, err := oas.NewServer(handler, security,
		oas.WithErrorHandler(api.ErrorHandler),
	)
	if err != nil {
		logger.Error("create ogen server", "error", err)
		return 1
	}

	// chi 根路由：横切中间件包住生成的服务端。
	// RequestID 生成 -> httplog.RequestIDMiddleware 把 request_id 注入 slog 上下文。
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(httplog.RequestIDMiddleware)
	r.Use(middleware.RealIP) //nolint:staticcheck // trusted proxy boundary is operator-controlled
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	// TODO(v1): 在此插入 认证 / ACL(x-requires-permission) / 审计 横切中间件。
	r.Mount("/", oasServer)

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: r,
	}

	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1) //nolint:gocritic // listener goroutine has no other exit path
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("server shutting down")
	// 10s 优雅停机：拒绝新请求 -> 等待在途 -> 关闭平台 PG 池（由 defer 兜底）。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		return 1
	}
	logger.Info("server stopped")
	return 0
}
