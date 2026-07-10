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

## Dependency updates

Update dependencies deliberately, review release notes, then run:

```sh
go mod tidy
make check
```

Do not use floating versions in CI or Docker images.
