# AI Agent Rules

These rules are mandatory for AI-assisted changes in this repository.

1. Read `docs/ARCHITECTURE.md` and `docs/CONVENTIONS.md` before editing.
2. Keep all repository content in English.
3. Preserve the modular-monolith boundaries and dependency direction.
4. Do not add an ORM, service locator, global mutable state, or automatic runtime
   migrations.
5. Do not add Redis, authentication, messaging, or vendor SDKs without an
   explicit requirement.
6. Update OpenAPI, migrations, SQL, generated code, tests, and docs as one atomic
   change when the contract requires them.
7. Never edit generated sqlc files manually.
8. Never place secrets or real credentials in source, tests, examples, logs, or
   tool output.
9. Prefer standard-library packages and justify every new dependency.
10. Run `make check` and report any check that could not be executed.

