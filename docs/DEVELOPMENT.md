# Development

## Local environment

Copy `.env.example` to `.env`, start PostgreSQL, migrate the schema, and run the
API:

```sh
cp .env.example .env
make tools
make hooks
make db-up
make migrate-up
make run
```

The application reads `.env` only through the Makefile. The binary itself reads
the process environment and therefore behaves the same in containers and
production.

Development tool versions are pinned in `tools.mk`. Keeping that manifest
separate ensures CI reuses compiled tool binaries when application build targets
change, while still invalidating the cache whenever a tool version changes.

If port `5432` is already in use, change both `POSTGRES_PORT` and the port in
`DATABASE_URL` in `.env` before starting PostgreSQL.

## Database workflow

1. Add a forward and backward Goose migration under `db/migrations`.
2. Update capability-owned SQL in `db/queries/<capability>`.
3. Run `make sqlc`.
4. Add repository and service tests.
5. Run `make check`.

Never edit files under a module's generated `dbgen` directory manually.

## HTTP contract workflow

1. Update `api/openapi.yaml`.
2. Run `make openapi` to regenerate boundary request and response types.
3. Implement or update the HTTP adapter and behavioral tests.
4. Run `make openapi-check` to lint the contract and detect generated-code
   drift.

Never edit `internal/httpapi/apigen/types.gen.go` manually. Generated types stay
at the HTTP boundary and must not become domain or database models.

## Testing

The default test command runs deterministic unit and in-process HTTP tests. It
never enables integration build tags or requires external services:

```sh
make test
```

Repository integration tests use the `integration` build tag and require an
isolated PostgreSQL database. The shared `internal/testkit/postgrestest` harness
creates a random schema, applies migrations through the real Goose provider,
executes generated sqlc queries, rolls migrations back, and removes the schema.
Goose is the only added test-harness dependency because reimplementing its SQL
migration grammar would make the test behave differently from deployments.

```sh
make db-up
make test-integration
```

Use a dedicated non-production database. The integration test never logs the
connection URL, but database driver errors can contain server metadata. CI
provides PostgreSQL and sets `TEST_DATABASE_URL`, so `make check` exercises the
integration path there. Without that variable, the integration test skips and
the remaining local checks still run.

`make check` remains the merge gate. When `TEST_DATABASE_URL` is set it invokes
`make test-integration` in addition to the external-service-free test suite. It
verifies formatting, generated sqlc and
OpenAPI output, tidy module files, OpenAPI quality, vet and lint findings, reachable
dependency vulnerabilities, race-tested behavior, and the production binary
build. CI additionally builds the production container image. The check prints
an explicit note when `TEST_DATABASE_URL` is absent and PostgreSQL integration
tests were therefore skipped. CI always sets the variable.

Generate a browsable local coverage report with `make cover`. The command writes
`coverage.out` and `coverage.html`; both files are ignored and removed by
`make clean`. The merge gate runs `make cover-check` and requires at least 80%
statement coverage across the measured internal packages. Generated sqlc code and
test harness packages are excluded from that aggregate.

Packages whose tests need PostgreSQL are excluded too when `TEST_DATABASE_URL` is
absent, and the command names the ones it dropped. Counting them would mix
measured statements with statements no run could have executed, which leaves the
same threshold meaning something different depending on the machine. CI always
sets the variable, so the merge gate always measures everything.

The HTTP test suite also compares every OpenAPI path and method with the routes
registered by Chi. This catches undocumented endpoints and documented endpoints
that are not wired into the application. Endpoint tests remain responsible for
verifying response status codes and payload behavior.

## Adding a module

Create `internal/<capability>` as a vertical slice. A typical module contains:

```text
internal/order/
  model.go          Domain and application data
  service.go        Use cases and consumed interfaces
  orderhttp/        HTTP transport adapter
  orderpostgres/    PostgreSQL adapter
    dbgen/           Generated database access code
  *_test.go

db/queries/order/   SQL owned by the order capability
```

Small modules should stay small. Create child adapter packages only when they
contain real adapter code; do not create empty `domain`, `usecase`, `ports`, and
`adapters` directories to imitate an architecture diagram.

The depguard rules in `.golangci.yml` mechanically keep router, database,
generated SQL, and environment packages out of business model and service
files. They match capabilities by layout rather than by name, so a module placed
as `internal/<capability>` with `<capability>http` and `<capability>postgres`
adapters inherits those bans without any configuration change.

Two bans cannot be written that way, because each names one capability's own
PostgreSQL adapter and depguard matches a banned package by prefix:

```yaml
business-layer:
  deny:
    - pkg: <module>/internal/order/orderpostgres/dbgen
http-adapter:
  deny:
    - pkg: <module>/internal/order/orderpostgres
```

Add both when a module gains a PostgreSQL adapter. `internal/app` tests that
every capability restates them and that they still carry the module path, so a
module added without them, or a module path renamed without updating the lint
configuration, fails `make check` instead of linting green with no boundary
enforcement.

Schema changes reach production through the migration job described in the
[Deployment guide](DEPLOYMENT.md), never through application startup.

Before turning a module into a service, keep it in-process and follow the
[Microservice Evolution Guide](MICROSERVICES.md). Boundary hardening and contract
tests must precede any network or database split.

## Dependency updates

Update dependencies deliberately, review release notes, then run:

```sh
go mod tidy
make check
```

Do not use floating versions in CI or Docker images.
