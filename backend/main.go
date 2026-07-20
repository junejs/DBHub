package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/junepy/dbhub/backend/internal/api"
	"github.com/junepy/dbhub/backend/internal/infra/bootstrap"
	"github.com/junepy/dbhub/backend/internal/infra/db"
	"github.com/junepy/dbhub/backend/internal/infra/migrate"
	"github.com/junepy/dbhub/backend/internal/infra/secret"
	"github.com/junepy/dbhub/backend/internal/infra/seed"
	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
)

func main() {
	code := run()
	os.Exit(code)
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		logger.Error("DB_DSN not set; refusing to start without a platform database")
		return 1
	}

	// Validate MASTER_KEY format at startup so misconfiguration fails fast
	// (D25 / 16-ops §3: base64-encoded 32 bytes). The Crypto/Provider instance
	// is constructed by secret-consuming services (IdP, data source) when they
	// land; here we only exercise validation to refuse boot on a bad key.
	if _, err := secret.LoadMasterKey(os.Getenv("MASTER_KEY")); err != nil {
		logger.Error("invalid MASTER_KEY; refusing to start",
			"error", err,
			"hint", "generate a valid key with: openssl rand -base64 32")
		return 1
	}

	// 1) Open platform DB connection pool. This is the single source of truth for
	// the metadata store (16-ops §2).
	platformDB, err := db.Open(context.Background(), dsn)
	if err != nil {
		logger.Error("open platform DB", "error", err)
		return 1
	}
	defer func() {
		if err := platformDB.Close(); err != nil {
			logger.Warn("close platform DB", "error", err)
		}
	}()

	// 2) Apply pending migrations (16-ops §1: migrate before ready).
	if err := migrate.Up(context.Background(), dsn); err != nil {
		logger.Error("apply migrations", "error", err)
		return 1
	}
	logger.Info("migrations applied")

	// 3) Seed builtin rows (environments, policies, roles, role_permissions).
	if err := seed.SeedBuiltin(context.Background(), platformDB); err != nil {
		logger.Error("seed builtin rows", "error", err)
		return 1
	}
	logger.Info("seed complete")

	// 4) Bootstrap first admin from env vars (idempotent; logs only the user id).
	bootstrapEmail := os.Getenv("BOOTSTRAP_ADMIN_EMAIL")
	bootstrapPassword := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if err := bootstrap.EnsureFirstAdmin(context.Background(), platformDB, bootstrapEmail, bootstrapPassword); err != nil {
		logger.Error("bootstrap first admin", "error", err)
		return 1
	}

	// --- 装配：repo -> service -> (handler, security) -> ogen server ---
	projectRepo := service.NewBunProjectRepo(platformDB)
	projectSvc := service.NewProjectService(projectRepo)
	handler := api.NewHandler(projectSvc, platformDB)
	security := api.NewSecurityHandler()

	// ogen 从 openapi.yaml 生成的服务端（实现 http.Handler）。
	oasServer, err := oas.NewServer(handler, security)
	if err != nil {
		logger.Error("create ogen server", "error", err)
		return 1
	}

	// chi 根路由：横切中间件包住生成的服务端。
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) //nolint:staticcheck // trusted proxy boundary is operator-controlled
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	// TODO(v1): 在此插入 认证 / ACL(x-requires-permission) / 审计 横切中间件。
	r.Mount("/", oasServer)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
	return 0
}
