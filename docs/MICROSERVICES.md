# Microservice Evolution Guide

## Position

The modular monolith is the default deployment model. Microservices are an
operational trade-off, not an architectural maturity level. Extract a capability
only when a measured deployment, scaling, ownership, security, availability, or
release-cadence requirement is more expensive than operating another service.

The existing module boundary is the extraction seam:

```text
HTTP -> application service -> repository interface -> module-owned tables
```

Extraction changes the transport and deployment topology. It must not move an
unclear boundary into a network call.

## Evidence required before extraction

Record the decision in an Architecture Decision Record (ADR). The ADR must name:

- the capability and team that will own it;
- the hard requirement that the monolith cannot meet economically;
- its traffic, latency, availability, and recovery objectives;
- its data owner and every current consumer of that data;
- the synchronous and asynchronous contracts it will expose;
- the deployment, rollback, and incident-response owners;
- the additional infrastructure and recurring operational cost;
- the reason a module-level change inside the monolith is insufficient.

Code size, a preference for a technology, and a desire to use distributed
systems are not sufficient reasons.

## Preconditions

Do not start extraction until all of the following are true:

1. The capability has a stable name and cohesive business responsibility.
2. Other modules use an application service or explicit interface rather than
   querying its tables or importing its adapters.
3. The capability owns its schema changes and has no command-path cross-module
   joins.
4. Its public behavior has contract and integration tests.
5. Logs, metrics, traces, dashboards, and alerts can identify the capability.
6. An owner can deploy, operate, secure, and support the new service.

If a precondition is false, fix the modular boundary first. That work is useful
even if extraction is later canceled.

## Target shape

Start with the smallest topology that satisfies the requirement:

```text
client
  |
  v
edge or existing API
  | versioned HTTP contract
  v
extracted capability service ----> capability-owned PostgreSQL database
  |
  +---- telemetry, deployment, alerts, and runbook
```

Keep the existing API as the compatibility edge when clients cannot migrate in
one release. The edge may delegate requests temporarily, but it must not acquire
business rules that belong to the extracted capability.

Do not introduce a gateway, service mesh, event broker, cache, or orchestration
platform merely because the first service is extracted. Add each capability only
for an explicit requirement and document the choice in an ADR.

## Extraction sequence

### 1. Measure and decide

Capture the current traffic, latency, error rate, deployment frequency, database
load, and incident history. Define a successful extraction with measurable exit
criteria. Approve the ADR and assign operational ownership.

### 2. Harden the in-process seam

- Remove direct table access from other modules.
- Replace shared business structs with stable contract types at the boundary.
- Keep the consuming interface small and owned by the consumer.
- Add contract tests for callers and provider behavior.
- Identify operations that require idempotency before they cross the network.

Run the hardened boundary in-process first. This separates boundary defects from
distributed-system defects.

### 3. Define the network contract

Use a versioned OpenAPI contract for synchronous HTTP calls. Define:

- request, response, and stable error schemas;
- authentication and authorization requirements when explicitly required;
- deadlines, payload limits, and pagination bounds;
- idempotency semantics for retried commands;
- compatibility and deprecation windows;
- health endpoints that distinguish liveness from dependency readiness.

Generate clients only when generation materially reduces drift. Generated types
must remain at the adapter boundary and must not become shared domain models.

### 4. Separate data ownership

The extracted service becomes the only writer of its data. Do not leave a shared
database as the permanent integration API.

Use an expand, copy, verify, cut over, and contract sequence:

1. **Expand:** prepare the new schema without breaking the monolith.
2. **Copy:** backfill data with a restartable, observable process.
3. **Verify:** compare counts and business invariants, not only row checksums.
4. **Cut over:** move reads and writes through one owner using a controlled
   deployment or feature flag.
5. **Contract:** remove obsolete tables and compatibility code only after the
   rollback window closes.

Avoid application-level dual writes and distributed transactions. If a temporary
change feed or replication mechanism is required, define its authority, lag,
reconciliation process, and removal date in the ADR.

### 5. Cut over traffic gradually

Deploy the new service before routing production calls to it. Validate readiness,
capacity, telemetry, and rollback in the target environment. Shift traffic in
small increments and compare behavior with the established baseline.

During the transition:

- propagate request and trace context;
- set a caller deadline shorter than its inbound deadline;
- retry only idempotent operations and use bounded backoff with jitter;
- cap connection pools and concurrent work;
- preserve the client-facing error contract at the compatibility edge;
- stop the rollout automatically when agreed error or latency thresholds fail.

### 6. Introduce asynchronous communication only when required

Messaging is appropriate when producers must be decoupled from consumer
availability, several consumers need the same business fact, or an asynchronous
workflow is part of the product requirement. It is not a prerequisite for MSA.

When messaging becomes an explicit requirement:

- publish immutable business events rather than database-row notifications;
- version schemas and define compatibility rules;
- use a transactional outbox to couple a state change with publication;
- make consumers idempotent and track processed message identifiers when needed;
- define ordering, retention, retry, poison-message, and replay behavior;
- monitor consumer lag and provide reconciliation tooling.

Do not promise exactly-once delivery. Design for at-least-once delivery and
observable recovery.

### 7. Remove the old path

After the rollback window and verification period:

- remove the in-process implementation and temporary delegation;
- revoke the monolith's access to the extracted data;
- delete compatibility flags and migration jobs;
- update architecture diagrams, ownership records, runbooks, and threat models;
- record actual cost and outcome against the ADR's success criteria.

## Reliability rules for service calls

- Every outbound call has an explicit timeout and propagates cancellation.
- Retry budgets are bounded and retries never multiply across layers.
- Commands are idempotent before automatic retries are enabled.
- Readiness fails only for dependencies required to serve the instance's traffic.
- Liveness never calls another service or database.
- A downstream failure is translated into a stable caller-facing error; vendor
  or internal response bodies are never forwarded directly.
- Circuit breakers, hedging, and load shedding are added from observed failure
  modes, not as speculative infrastructure.

## Security changes after extraction

The trust boundary changes when an in-process call becomes a network call.
Before production cutover:

- authenticate workload identity and authorize each operation;
- encrypt traffic in transit at the platform boundary;
- use separate least-privilege database and deployment credentials;
- prevent direct network access to internal service ports where possible;
- define secret rotation and certificate rotation ownership;
- review logs, traces, and events for sensitive data exposure;
- update the threat model and incident-response runbook.

This repository intentionally contains no authentication or vendor SDK. Select
and implement those only when the deployment environment and requirement are
known.

## Repository strategy

Keep the first extraction in the same repository when shared review, atomic
contract changes, and one CI system reduce risk. Separate repositories only when
independent ownership and release governance justify the coordination cost.

Whether the code is in one repository or several:

- do not publish a shared business-model package;
- share only stable protocol primitives or generated boundary clients;
- version contracts independently from deployment artifacts;
- keep each service's migrations, tests, container, and runbook with that service;
- make local development possible without starting every service.

## Extraction completion checklist

- [ ] ADR contains measurable motivation, owners, cost, and rollback strategy.
- [ ] Module boundary is clean and covered by contract tests.
- [ ] API or event schemas are versioned and compatibility-tested.
- [ ] One service is the only writer for the extracted data.
- [ ] Backfill and reconciliation are restartable and observable.
- [ ] Timeouts, idempotency, retry limits, and capacity limits are verified.
- [ ] Dashboards, alerts, traces, and runbooks exist before traffic cutover.
- [ ] Workload identity, authorization, network policy, and secret rotation are
      reviewed.
- [ ] Rollout and rollback have been exercised in the target environment.
- [ ] Temporary code, data access, and infrastructure have explicit removal
      criteria.
