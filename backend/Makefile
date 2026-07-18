.PHONY: build dev test lint migrate

build:
	go build -o bin/server main.go

dev:
	go run main.go

test:
	go test -race -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

migrate:
	@echo "TODO: implement migrations"
