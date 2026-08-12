# Architecture

## Default shape

This repository starts as a modular monolith. A module is extracted into a
separate service only when independent deployment, scaling, ownership, security,
or availability requirements justify the operational cost.

```text
cmd/api                 Process entry point and signal handling
internal/app            Composition root and application lifecycle
internal/httpapi        HTTP router and cross-cutting HTTP behavior
internal/httpapi/apigen Generated public-contract boundary types
internal/platform       Technical adapters shared by modules
internal/<capability>    Transport- and persistence-independent use cases
internal/<capability>/<capability>http
                        Capability-owned HTTP adapter
internal/<capability>/<capability>postgres
                        Capability-owned PostgreSQL adapter
internal/<capability>/<capability>postgres/dbgen
                        Module-owned generated database access code
internal/testkit         Reusable integration-test harnesses
db/migrations           Versioned database schema changes
db/queries/<capability> SQL owned by one capability and consumed by sqlc
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

Go packages are architecture boundaries. HTTP and PostgreSQL adapters live in
child packages rather than beside business services in the same package. This
keeps dependency direction compiler-enforceable and lets an adapter be replaced
without coupling business behavior to its dependencies.

## Module rules

Each business capability owns its behavior, data access, and public interfaces.

- Modules communicate through explicit Go interfaces or application services.
- A module never queries another module's tables directly.
- A module never imports another module's adapter implementation.
- Cyclic package dependencies are architecture failures, not refactoring tasks to
  postpone.
- Shared business models are avoided. Share stable primitives only when their
  semantics are genuinely identical.

The repository enforces the most important business-layer import restrictions
with depguard. When a module introduces a new adapter package or file layout,
update `.golangci.yml` in the same change so the documented boundary remains
machine-checkable.

## Database ownership

PostgreSQL is shared at the deployment level in the default monolith, but table
ownership remains module-specific. Cross-module joins are prohibited in command
paths. Purpose-built read models may join data when explicitly documented and
kept read-only.

All SQL is visible and version controlled. sqlc generates typed access code
inside the owning module; it does not define domain boundaries. Generated
structs must not leak through the service or HTTP API layers.

## Transactions

Transactions start at the application use-case boundary, not inside HTTP
handlers. A repository method must not commit a transaction that its caller
expects to coordinate with other writes.

When a use case spans multiple repositories, define a capability-owned
transaction port that exposes only that capability's repositories. Implement it
in the PostgreSQL adapter with pgx. Do not pass pgx transactions through business
types, hide transactions in context values, or add a domain-agnostic service
locator.

Keep transactions short and never hold them open while calling an external
service.

## HTTP contract

`api/openapi.yaml` is the public contract. Handlers implement it; database models
do not define it. Generated OpenAPI types remain at the HTTP boundary and are
mapped explicitly to application models. Breaking changes require a new API
version or a documented migration path.

## Application and test harnesses

`internal/app` assembles replaceable capability ports into one in-process HTTP
application. `cmd/api` owns only process concerns. Tests can build the same
application with fakes without environment variables, sockets, or PostgreSQL.

`internal/testkit` contains reusable infrastructure test harnesses. The
PostgreSQL harness creates an isolated schema, applies migrations through Goose,
and cleans up through `testing.T.Cleanup`. Production code must never import a
test harness package.

## Extraction criteria

Do not extract a module merely because it is large. Consider extraction only
when at least one hard requirement exists and the data ownership boundary is
already clean:

- a separate team owns and deploys it;
- its traffic or resource profile scales independently;
- it has a distinct security or compliance boundary;
- it needs a materially different availability target;
- its release cadence conflicts with the rest of the application.

Extraction is a staged operational change, not a package move. The capability
must have clean application and data ownership boundaries, contract tests,
measurable service objectives, and an identified operator before a network seam
is introduced. Follow the [Microservice Evolution Guide](MICROSERVICES.md) for
the required ADR, data migration, traffic cutover, reliability, security, and
rollback process.

Until an extraction is approved, do not add remote-call abstractions, messaging,
service discovery, distributed tracing vendors, or deployment units for
hypothetical services.

Architecture decisions that change these boundaries are recorded under
`docs/adr/` using the repository template.
