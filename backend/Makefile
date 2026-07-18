.PHONY: build dev test lint migrate gen

build:
	go build -o bin/server main.go

dev:
	go run main.go

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

gen: ## 从 ../openapi.yaml 重新生成 internal/oas（契约 → Go 代码）
	go run github.com/ogen-go/ogen/cmd/ogen --config ogen.yml --target internal/oas --package oas --clean ../openapi.yaml

migrate:
	@echo "TODO: implement migrations"
