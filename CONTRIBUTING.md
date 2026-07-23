# Contributing

1. Read `docs/ARCHITECTURE.md` and `docs/CONVENTIONS.md`.
2. Update the OpenAPI contract for public API changes.
3. Add or update tests with the behavior change.
4. Regenerate sqlc output when schema or queries change.
5. Run `make check`.

Database behavior changes should also run `make test-integration` against a
dedicated local PostgreSQL database. CI always runs the PostgreSQL integration
path.

Pull requests must explain the user-visible behavior, data migration impact,
rollback plan, module or service-boundary impact, and any deliberate convention
exception. A proposal to extract a service must include the ADR evidence and
completion criteria from `docs/MICROSERVICES.md`.

Commit subjects use the English `type: imperative summary` format documented in
`docs/CONVENTIONS.md`.
