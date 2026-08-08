.PHONY: help run build test test-integration bench tidy lint fmt swagger docker-up docker-down migrate-up migrate-down

APP_NAME := go_template
BIN := bin/api
GOBIN := $(shell go env GOPATH)/bin

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

run: ## Run the API locally
	go run ./cmd/api

build: ## Build the API binary
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) ./cmd/api

test: ## Run unit tests with race detector and coverage
	go test ./... -race -covermode=atomic -coverprofile=coverage.out

test-integration: ## Run end-to-end integration tests (needs a reachable, migrated DB)
	RATE_LIMIT_ENABLED=false go test -tags=integration -count=1 ./test/integration/...

bench: ## Run benchmarks
	go test ./... -run=^$$ -bench=. -benchmem

tidy: ## Tidy go modules
	go mod tidy

fmt: ## Format code
	go fmt ./...

lint: ## Run golangci-lint (falls back to go vet)
	@$(GOBIN)/golangci-lint run ./... 2>/dev/null || go vet ./...

swagger: ## Regenerate OpenAPI docs from annotations
	$(GOBIN)/swag init -g internal/app/app.go -o docs --parseInternal --parseDepth 1

docker-up: ## Start the full stack with docker compose
	docker compose up --build

docker-down: ## Stop the stack
	docker compose down -v

migrate-up: ## Apply all pending migrations
	go run ./cmd/migrate up

migrate-down: ## Roll back one migration
	go run ./cmd/migrate down 1
