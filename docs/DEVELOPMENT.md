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

If port `5432` is already in use, change both `POSTGRES_PORT` and the port in
`DATABASE_URL` in `.env` before starting PostgreSQL.

## Database workflow

1. Add a forward and backward Goose migration under `db/migrations`.
2. Update SQL in `db/queries`.
3. Run `make sqlc`.
4. Add repository and service tests.
5. Run `make check`.

Never edit files under `internal/database/sqlc` manually.

## Testing

The default test command runs unit tests without requiring external services:

```sh
make test
```

Repository integration tests require an isolated PostgreSQL database. They
create a random schema, apply the real Goose migration, execute the generated
sqlc queries, roll the migration back, and remove the schema.

```sh
make db-up
make test-integration
```

Use a dedicated non-production database. The integration test never logs the
connection URL, but database driver errors can contain server metadata. CI
provides PostgreSQL and sets `TEST_DATABASE_URL`, so `make check` exercises the
integration path there. Without that variable, the integration test skips and
the remaining local checks still run.

`make check` remains the merge gate. It verifies formatting, generated sqlc
output, tidy module files, OpenAPI quality, vet and lint findings, reachable
dependency vulnerabilities, race-tested behavior, and the production binary
build. CI additionally builds the production container image. The check prints
an explicit note when `TEST_DATABASE_URL` is absent and PostgreSQL integration
tests were therefore skipped. CI always sets the variable.

Generate a browsable local coverage report with `make cover`. The command writes
`coverage.out` and `coverage.html`; both files are ignored and removed by
`make clean`. The merge gate runs `make cover-check` and requires at least 80%
statement coverage across maintained internal packages. Generated sqlc code and
the composition root are excluded from that aggregate.

The HTTP test suite also compares every OpenAPI path and method with the routes
registered by Chi. This catches undocumented endpoints and documented endpoints
that are not wired into the application. Endpoint tests remain responsible for
verifying response status codes and payload behavior.

## Adding a module

Create `internal/<capability>` as a vertical slice. A typical module contains:

```text
internal/orders/
  model.go       Domain and application data
  service.go     Use cases and consumed interfaces
  repository.go  PostgreSQL adapter, when required
  handler.go     HTTP transport adapter
  *_test.go
```

Small modules should stay small. Do not create empty `domain`, `usecase`,
`ports`, and `adapters` directories to imitate an architecture diagram.

The depguard rules in `.golangci.yml` mechanically keep router, database,
generated SQL, and environment packages out of business model and service
files. Extend those rules when adding a new technical adapter or module layout.

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
