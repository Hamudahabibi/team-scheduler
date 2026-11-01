.PHONY: run build test test-integration cover lint fmt up down experiment

DATABASE_URL ?= postgres://scheduler:scheduler@localhost:5432/scheduler?sslmode=disable

run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

# Интеграционные тесты хранилища, нужна запущенная PostgreSQL (make up).
test-integration:
	TEST_DATABASE_URL=$(DATABASE_URL) go test -race -tags=integration ./internal/storage/...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

fmt:
	gofmt -w .

up:
	docker compose up -d --build

down:
	docker compose down
