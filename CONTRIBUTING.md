# Contributing

1. Read `docs/ARCHITECTURE.md` and `docs/CONVENTIONS.md`.
2. Update the OpenAPI contract for public API changes.
3. Add or update tests with the behavior change.
4. Regenerate sqlc output when schema or queries change.
5. Run `make check`.

Pull requests must explain the user-visible behavior, data migration impact,
rollback plan, and any deliberate convention exception.

Commit subjects use the English `type: imperative summary` format documented in
`docs/CONVENTIONS.md`.
