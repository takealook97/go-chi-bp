# Architecture

## Default shape

This repository starts as a modular monolith. A module is extracted into a
separate service only when independent deployment, scaling, ownership, security,
or availability requirements justify the operational cost.

```text
cmd/api                 Composition root
internal/httpapi        HTTP router and cross-cutting HTTP behavior
internal/platform       Technical adapters shared by modules
internal/<capability>   Vertical business module
internal/database/sqlc  Generated database access code
db/migrations           Versioned database schema changes
db/queries              SQL consumed by sqlc
api                     Public OpenAPI contract
```

## Dependency direction

```text
HTTP handler -> application service -> repository interface
                                         ^
                                         |
                               PostgreSQL repository
```

The composition root constructs concrete adapters and injects them inward.
Business services must not import Chi, pgx, sqlc-generated packages, environment
packages, or another module's HTTP handler.

## Module rules

Each business capability owns its behavior, data access, and public interfaces.

- Modules communicate through explicit Go interfaces or application services.
- A module never queries another module's tables directly.
- A module never imports another module's adapter implementation.
- Cyclic package dependencies are architecture failures, not refactoring tasks to
  postpone.
- Shared business models are avoided. Share stable primitives only when their
  semantics are genuinely identical.

## Database ownership

PostgreSQL is shared at the deployment level in the default monolith, but table
ownership remains module-specific. Cross-module joins are prohibited in command
paths. Purpose-built read models may join data when explicitly documented and
kept read-only.

All SQL is visible and version controlled. sqlc generates typed access code; it
does not define domain boundaries. Generated structs must not leak through the
service or HTTP API layers.

## Transactions

Transactions start at the application use-case boundary, not inside HTTP
handlers. A repository method must not commit a transaction that its caller
expects to coordinate with other writes.

Keep transactions short and never hold them open while calling an external
service.

## HTTP contract

`api/openapi.yaml` is the public contract. Handlers implement it; database models
do not define it. Breaking changes require a new API version or a documented
migration path.

## Extraction criteria

Do not extract a module merely because it is large. Consider extraction only
when at least one hard requirement exists and the data ownership boundary is
already clean:

- a separate team owns and deploys it;
- its traffic or resource profile scales independently;
- it has a distinct security or compliance boundary;
- it needs a materially different availability target;
- its release cadence conflicts with the rest of the application.

