# Conventions

These rules are defaults. Exceptions require a short Architecture Decision Record
that explains the concrete trade-off.

## Language

- All source code, identifiers, comments, logs, documentation, commit messages,
  API fields, and database identifiers are written in English.
- Prefer clear names over abbreviations. Common protocol terms such as `ID`,
  `HTTP`, `URL`, and `DB` are acceptable.
- Use American English consistently.

## Go

- Run `gofmt`; generated code is never edited manually.
- Package names are short, lowercase, singular nouns.
- Export only what another package needs.
- Accept `context.Context` as the first parameter for I/O-bound operations.
- Return errors; do not log and return the same error at lower layers.
- Wrap errors with operation context using `%w`.
- Compare sentinel errors with `errors.Is` and typed errors with `errors.As`.
- Avoid `panic` outside unrecoverable startup failures.
- Avoid global mutable state and package-level service locators.
- Define interfaces in the consuming package and keep them small.
- Do not create interfaces solely for hypothetical future implementations.
- Constructors validate mandatory dependencies and return concrete types unless
  callers need an interface.
- Prefer table-driven tests and `t.Parallel()` where shared state permits it.

## HTTP

- Public endpoints are defined in `api/openapi.yaml` before or with code changes.
- Use nouns for resources and HTTP methods for actions.
- Use `/v1/...` for public application endpoints.
- Use camelCase JSON fields and snake_case database identifiers.
- Reject unknown JSON fields and limit request body sizes.
- Require an explicit JSON media type for JSON request bodies.
- Keep transport validation on OpenAPI-generated boundary types and business
  validation in application services.
- Return one JSON value per response and a consistent error envelope.
- Do not expose internal error strings, SQL, stack traces, or vendor responses.
- Set explicit server and outbound-client timeouts.
- Pass the request context to every downstream operation.
- Trust forwarded client IP values only through an explicitly configured proxy
  topology.
- Liveness checks test the process only. Readiness checks test required
  dependencies with strict timeouts.

## SQL and PostgreSQL

- Use lowercase snake_case identifiers and plural table names.
- Every schema change is a reviewed, reversible migration when practical.
- Never run automatic schema migration during application startup.
- Always list selected columns; do not use `SELECT *` in application queries.
- Use parameterized queries only.
- Add indexes from observed access patterns, not speculation.
- Make ordering explicit when result order matters.
- Keep pagination bounded. Prefer cursor pagination for large or mutable datasets.
- Store timestamps as `timestamptz` in UTC and expose RFC 3339 timestamps.
- Use database constraints for invariants the database can enforce.
- Do not use ORM lifecycle hooks or hide writes behind model methods.

## Logging and observability

- Use structured `slog` fields, not formatted log sentences.
- Log through the request context: the logging handler reads the request ID
  from it. Naming the request ID in a call duplicates the field. Add operation
  names yourself.
- Never log credentials, tokens, cookies, full payment data, or personal data.
- Log an error once at the boundary responsible for handling it.
- Metrics and traces must use low-cardinality attributes.

## Testing

- Test business rules at the service boundary and transport behavior at the HTTP
  boundary.
- Every public endpoint covers its success response and stable error mappings.
- Database integration tests apply real migrations and execute generated queries
  against PostgreSQL; repository mocks do not replace this verification.
- Integration tests isolate their data and must not depend on execution order.
- Contract linting and implementation tests serve different purposes; passing an
  OpenAPI linter does not prove that handlers implement the contract.
- Tests may skip an external integration only when its documented environment
  variable is absent. CI must provide required integrations.
- Prefer behavioral assertions over implementation details and keep test failure
  messages free of credentials and sensitive configuration.

## Configuration and secrets

- Configuration comes from environment variables and is validated at startup.
- Defaults are allowed only for safe local-development values.
- Production secrets are never committed, logged, embedded in images, or placed
  in example files.
- Adding a variable requires updating `.env.example` and documentation.

## Git and review

- Commit subjects must be written in English as `type: imperative summary`.
- Allowed types are `feat`, `fix`, `refactor`, `docs`, `test`, `build`, `ci`,
  `chore`, `perf`, `security`, and `revert`.
- The summary starts with a lowercase imperative verb, has no trailing period,
  and stays within 72 characters including the type.
- Examples: `feat: add widget creation endpoint`, `fix: reject blank names`, and
  `docs: clarify transaction ownership`.
- Keep changes focused and independently reviewable.
- Commit generated sqlc output with the schema and queries that produced it.
- Do not combine behavior changes with unrelated formatting or renaming.
- Every bug fix includes a regression test when feasible.
- `make check` must pass before merge.
