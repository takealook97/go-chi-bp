SHELL := /bin/sh

include tools.mk

ifneq (,$(wildcard .env))
include .env
export
endif

HOST_OS := $(shell go env GOHOSTOS)
HOST_ARCH := $(shell go env GOHOSTARCH)
BIN_DIR := $(CURDIR)/bin/$(HOST_OS)-$(HOST_ARCH)
API_BIN := $(BIN_DIR)/api
SQLC := $(BIN_DIR)/sqlc
GOOSE := $(BIN_DIR)/goose
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
VACUUM := $(BIN_DIR)/vacuum
GOVULNCHECK := $(BIN_DIR)/govulncheck
OAPI_CODEGEN := $(BIN_DIR)/oapi-codegen
CHECK_TOOLS := $(SQLC) $(GOLANGCI_LINT) $(VACUUM) $(GOVULNCHECK) $(OAPI_CODEGEN)

COVERAGE_MIN := 80.0
MAINTAINED_PACKAGES = $(shell go list ./internal/... | grep -Ev '/(apigen|dbgen|testkit)(/|$$)')

.DEFAULT_GOAL := help

.PHONY: help tools check-tools hooks run build docker-build test test-race test-integration test-integration-check cover cover-check fmt fmt-check lint vet vuln \
	tidy-check openapi openapi-check sqlc sqlc-check check clean db-up db-down migrate-up migrate-down migrate-status

help: ## Show available commands.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: $(CHECK_TOOLS) $(GOOSE) ## Install all pinned development tools locally.

check-tools: $(CHECK_TOOLS) ## Install the pinned tools required by make check.

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

$(SQLC): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(SQLC_INSTALL)

$(GOOSE): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(GOOSE_INSTALL)

$(GOLANGCI_LINT): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(GOLANGCI_LINT_INSTALL)

$(VACUUM): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(VACUUM_INSTALL)

$(GOVULNCHECK): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(GOVULNCHECK_INSTALL)

$(OAPI_CODEGEN): | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install $(OAPI_CODEGEN_INSTALL)

hooks: ## Enable repository-managed Git hooks.
	git config core.hooksPath .githooks

run: ## Run the API with the current environment.
	go run ./cmd/api

build: ## Build the API binary.
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -o $(API_BIN) ./cmd/api

docker-build: ## Verify the production container image builds.
	docker build --tag go-chi-bp:check .

test: ## Run external-service-free tests.
	go test -count=1 ./...

test-race: ## Run all tests with the race detector.
	go test -race ./...

test-integration: ## Run PostgreSQL integration tests against TEST_DATABASE_URL.
	@test -n "$(TEST_DATABASE_URL)" || { echo "TEST_DATABASE_URL is required"; exit 1; }
	go test -race -count=1 -tags=integration -run Integration ./internal/...

test-integration-check: ## Run integration tests when PostgreSQL is configured.
	@if test -n "$(TEST_DATABASE_URL)"; then $(MAKE) test-integration; else echo "NOTE: PostgreSQL integration tests were skipped because TEST_DATABASE_URL is not set."; fi

cover: ## Generate an HTML coverage report for maintained application packages.
	go test -coverprofile=coverage.out $(MAINTAINED_PACKAGES)
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -n 1

cover-check: cover ## Enforce the minimum maintained-package coverage.
	@go tool cover -func=coverage.out | awk -v minimum=$(COVERAGE_MIN) '/^total:/ { value=$$3; sub(/%/, "", value); if (value + 0 < minimum) { printf "Coverage %.1f%% is below %.1f%%.\n", value, minimum; exit 1 } }'

fmt: $(GOLANGCI_LINT) ## Format Go source files.
	$(GOLANGCI_LINT) fmt

fmt-check: $(GOLANGCI_LINT) ## Verify configured formatting without modifying files.
	$(GOLANGCI_LINT) fmt --diff

lint: $(GOLANGCI_LINT) ## Run the configured linters.
	$(GOLANGCI_LINT) run

vet: ## Run go vet.
	go vet ./...

vuln: $(GOVULNCHECK) ## Check reachable code for known Go vulnerabilities.
	$(GOVULNCHECK) ./...

tidy-check: ## Verify go.mod and go.sum are tidy.
	go mod tidy -diff

openapi: $(OAPI_CODEGEN) ## Generate HTTP contract types from OpenAPI.
	$(OAPI_CODEGEN) --config api/oapi-codegen.yaml -o internal/httpapi/apigen/types.gen.go api/openapi.yaml

openapi-check: $(VACUUM) $(OAPI_CODEGEN) ## Validate OpenAPI and verify generated contract types.
	$(VACUUM) lint -d api/openapi.yaml
	@temp_file="$$(mktemp)"; trap 'rm -f "$$temp_file"' EXIT; \
		$(OAPI_CODEGEN) --config api/oapi-codegen.yaml -o "$$temp_file" api/openapi.yaml; \
		diff -u "$$temp_file" internal/httpapi/apigen/types.gen.go

sqlc: $(SQLC) ## Generate type-safe database code.
	$(SQLC) generate

sqlc-check: $(SQLC) ## Verify generated database code is current.
	@temp_dir="$$(mktemp -d)"; trap 'rm -rf "$$temp_dir"' EXIT; \
		cp -R db "$$temp_dir/db"; \
		cp sqlc.yaml "$$temp_dir/sqlc.yaml"; \
		cd "$$temp_dir" && $(SQLC) generate; \
		temp_outputs="$$(cd "$$temp_dir" && find internal -type d -name dbgen | sort)"; \
		repo_outputs="$$(find internal -type d -name dbgen | sort)"; \
		test "$$temp_outputs" = "$$repo_outputs" || { echo "sqlc output directories differ"; echo "generated: $$temp_outputs"; echo "repository: $$repo_outputs"; exit 1; }; \
		for output in $$temp_outputs; do diff -ru "$$temp_dir/$$output" "$(CURDIR)/$$output"; done

check: fmt-check tidy-check sqlc-check openapi-check vet lint vuln test-race test-integration-check cover-check build ## Run all required local and CI checks.

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
