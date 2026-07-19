.PHONY: build dev test test-race lint migrate gen cover

build:
	go build -o bin/server main.go

dev:
	go run main.go

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

gen: ## 从 ../openapi.yaml 重新生成 internal/oas（契约 → Go 代码）
	go run github.com/ogen-go/ogen/cmd/ogen --config ogen.yml --target internal/oas --package oas --clean ../openapi.yaml

migrate: ## 应用嵌入式 migrations（DB_DSN 必须指向平台 PG）
	@if [ -z "$$DB_DSN" ]; then \
		echo "DB_DSN not set; refusing to run migrations against an unknown DB."; \
		exit 1; \
	fi
	@echo "Applying embedded migrations from internal/infra/migrate/migrations/ against $$DB_DSN"
	go run ./cmd/migrate