package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/junepy/dbhub/backend/internal/api"
	"github.com/junepy/dbhub/backend/internal/oas"
	"github.com/junepy/dbhub/backend/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// --- 装配：repo -> service -> (handler, security) -> ogen server ---
	projectRepo := service.NewMemoryProjectRepo([]service.Project{
		{Key: "platform", Title: "Platform", Description: "DBHUB 平台自身"},
	})
	projectSvc := service.NewProjectService(projectRepo)
	handler := api.NewHandler(projectSvc)
	security := api.NewSecurityHandler()

	// ogen 从 openapi.yaml 生成的服务端（实现 http.Handler）。
	oasServer, err := oas.NewServer(handler, security)
	if err != nil {
		logger.Error("create ogen server", "error", err)
		os.Exit(1)
	}

	// chi 根路由：横切中间件包住生成的服务端。
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
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
}
