# Pinned development tool versions.
#
# Keep these values separate from the main Makefile so CI's binary cache is
# invalidated only when the toolchain changes.
SQLC_VERSION := v1.31.1
SQLC_INSTALL := github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
GOOSE_VERSION := v3.27.2
GOOSE_INSTALL := github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT_INSTALL := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
VACUUM_VERSION := v0.29.9
VACUUM_INSTALL := github.com/daveshanley/vacuum@$(VACUUM_VERSION)
GOVULNCHECK_VERSION := v1.6.0
GOVULNCHECK_INSTALL := golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
OAPI_CODEGEN_VERSION := v2.8.0
OAPI_CODEGEN_INSTALL := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
