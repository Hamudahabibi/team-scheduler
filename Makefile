.PHONY: run build test test-integration cover lint fmt up down experiment

DATABASE_URL ?= postgres://scheduler:scheduler@localhost:5432/scheduler?sslmode=disable
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

# Интеграционные тесты на PostgreSQL (make up). Каждый тест — во временной схеме.
test-integration:
	TEST_DATABASE_URL=$(DATABASE_URL) go test -race -count=1 -tags=integration ./internal/storage/...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	$(GOLANGCI_LINT) run --build-tags=integration ./...

fmt:
	gofmt -w .

experiment:
	go run ./cmd/experiment -seeds 20 -teams 2,4,6,8,10 -limit 2s

up:
	docker compose up -d --build

down:
	docker compose down
