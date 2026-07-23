SHELL := /bin/sh

ifneq (,$(wildcard .env))
include .env
export
endif

BIN_DIR := $(CURDIR)/bin
API_BIN := $(BIN_DIR)/api
SQLC := $(BIN_DIR)/sqlc
GOOSE := $(BIN_DIR)/goose
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
VACUUM := $(BIN_DIR)/vacuum

SQLC_VERSION := v1.31.1
GOOSE_VERSION := v3.27.2
GOLANGCI_LINT_VERSION := v2.12.2
VACUUM_VERSION := v0.29.9

.DEFAULT_GOAL := help

.PHONY: help tools hooks run build test test-race test-integration fmt fmt-check lint vet openapi-check sqlc sqlc-check \
	check clean db-up db-down migrate-up migrate-down migrate-status

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: $(SQLC) $(GOOSE) $(GOLANGCI_LINT) $(VACUUM) ## Install pinned development tools locally.

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

$(SQLC): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

$(GOOSE): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

$(GOLANGCI_LINT): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(VACUUM): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/daveshanley/vacuum@$(VACUUM_VERSION)

hooks: ## Enable repository-managed Git hooks.
	git config core.hooksPath .githooks

run: ## Run the API with the current environment.
	go run ./cmd/api

build: ## Build the API binary.
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -o $(API_BIN) ./cmd/api

test: ## Run all unit tests.
	go test ./...

test-race: ## Run all tests with the race detector.
	go test -race ./...

test-integration: ## Run PostgreSQL integration tests against TEST_DATABASE_URL.
	@test -n "$(TEST_DATABASE_URL)" || { echo "TEST_DATABASE_URL is required"; exit 1; }
	go test -race -count=1 -run Integration ./internal/platform/database ./internal/widget

fmt: $(GOLANGCI_LINT) ## Format Go source files.
	$(GOLANGCI_LINT) fmt

fmt-check: ## Verify gofmt formatting without modifying files.
	@files="$$(gofmt -l .)"; test -z "$$files" || { echo "Unformatted files:"; echo "$$files"; exit 1; }

lint: $(GOLANGCI_LINT) ## Run the configured linters.
	$(GOLANGCI_LINT) run

vet: ## Run go vet.
	go vet ./...

openapi-check: $(VACUUM) ## Validate and lint the OpenAPI contract.
	$(VACUUM) lint -d api/openapi.yaml

sqlc: $(SQLC) ## Generate type-safe database code.
	$(SQLC) generate

sqlc-check: $(SQLC) ## Verify generated database code is current.
	$(SQLC) generate
	git diff --exit-code -- internal/database/sqlc

check: fmt-check sqlc-check openapi-check vet lint test-race build ## Run all required local and CI checks.

clean: ## Remove local build and tool artifacts.
	rm -rf $(BIN_DIR) coverage.out coverage.html

db-up: ## Start local PostgreSQL.
	docker compose up -d postgres

db-down: ## Stop local PostgreSQL.
	docker compose down

migrate-up: $(GOOSE) ## Apply all pending database migrations.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down: $(GOOSE) ## Roll back one database migration.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

migrate-status: $(GOOSE) ## Show database migration status.
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" status
