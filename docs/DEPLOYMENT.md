# Deployment

This repository ships two images built from one tree: the API and a migration job
that applies pending schema changes and exits. Nothing here selects a hosting
platform. It describes the order the two must run in and what each step is
allowed to assume, because that order is the part a platform cannot supply.

## Order of operations

1. Build both images from the same commit.
2. Run the migration job to completion against the target database.
3. Deploy the API only after the job exits successfully.

The application never migrates on startup. A replica that migrated as it booted
would tie a schema change to whichever instance started first, run the same
change concurrently on every other replica, and surface a failed migration as a
crash loop rather than as a failed deployment step.

Because migrations run before the new code does, every migration must be
compatible with the API version that is still serving traffic. Adding a nullable
column, a table, or an index is safe. Renaming or dropping anything is not, and
is split across releases: add the new shape, deploy code that writes both, move
the data, deploy code that reads the new shape, and only then drop the old one in
a later release.

## Images

```sh
docker build --tag registry.example.com/app:$TAG .
docker build --target migrate --tag registry.example.com/app-migrate:$TAG .
```

The default target is the API. The `migrate` target produces the job image, which
carries the migrations embedded in its binary rather than as files beside it.
An image pair built from two different trees is a schema the running code does
not expect; embedding removes one of the two ways that happens.

Both images run as `nonroot` and contain a single static binary. The job image
exposes no port and serves nothing.

## Running the job

The job reads `DATABASE_URL` and nothing else the API needs, so an HTTP or CORS
value that fails validation cannot fail a schema deployment or be reported as
one. It exits non-zero when a migration fails, which is what stops the rollout;
the platform is responsible for treating that exit status as fatal.

| Variable | Required | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | yes | Connection string for a role allowed to change the schema |
| `MIGRATE_TIMEOUT` | no | How long one run may take, `5m` by default |

Give the job a role with the privileges its migrations need. The API's own role
does not need them, and separating the two keeps a compromised application from
altering the schema it runs on.

A migration that waits on a lock held by the running application blocks every
query behind it. Bound that wait rather than the whole run when a change touches
a busy table:

```sh
DATABASE_URL='postgres://migrator@db/app?options=-c%20lock_timeout%3D5s'
```

Raise `MIGRATE_TIMEOUT` for a change that rewrites a large table. The default
cancels a run that outlives it, and for a migration Goose runs in a transaction —
every migration in this repository today — a cancelled run is one PostgreSQL
rolls back rather than one that half-applied, because Goose records a version
only after that transaction commits. A migration marked `-- +goose NO
TRANSACTION` has no such guarantee; see below.

## Failure and retry

Re-running the job is safe and is the normal response to an infrastructure
failure. Goose applies only the versions the database has not recorded, so a
retry after a network drop resumes rather than repeats.

A migration that failed on its own SQL is a different case. Its version is not
recorded, the versions before it are, and the fix is a new migration or a
correction to the unapplied one. Deploying the API on top of a partially applied
set is the thing to avoid; failing the deployment on the job's exit status is
what prevents it.

## Non-transactional migrations

Both statements above assume the migration ran inside a transaction, which is
Goose's default and is true of every migration here. A migration marked
`-- +goose NO TRANSACTION` breaks that assumption: its statements commit as they
execute, so a cancelled or failed run leaves the database in whatever state the
last completed statement produced, while Goose records no version for it. A
retry then re-runs statements that already took effect.

The reason to reach for it is a statement PostgreSQL refuses to run inside a
transaction, `CREATE INDEX CONCURRENTLY` being the usual one. That case shows the
cost exactly: a failed concurrent build leaves an INVALID index behind, which the
retry cannot create over and which serves no query until someone drops it.

A migration that opts out of the transaction carries its own recovery plan in a
comment at the top of the file: what a partial run leaves behind, and the
statement that returns the database to a state where the migration can be run
again. Prefer statements that are safe to repeat — `DROP INDEX IF EXISTS` before
the concurrent create, rather than a plan that requires someone to notice.

## Rollback

Prefer rolling forward. Down migrations are written and tested here, and
`make migrate-down` runs them locally, but a down migration that drops a column
destroys the data written since the deployment, and no rollback restores it.

When the schema change was additive, roll back the API alone and leave the schema
in place; the new shape is unused until code reads it again. When it was not, the
expand-and-contract sequence above is what makes a rollback possible at all,
because each release is compatible with the one before it.

## Local rehearsal

```sh
make db-up
make migrate-run     # runs cmd/migrate, the same code path as the job
make migrate-status  # shows what the database has recorded
```

`make migrate-up` runs the pinned Goose CLI against `db/migrations` instead. It
is the convenient loop while writing a migration; `make migrate-run` is the one
that rehearses the deployment.

To rehearse the deployed artifacts rather than the source, build both images and
run them:

```sh
make docker-build docker-build-migrate
DATABASE_URL='postgres://postgres:postgres@localhost:5432/app?sslmode=disable' \
  make docker-smoke
```

That applies the migrations from the job image, starts the API image against the
schema it applied, and writes and reads a row back. On Docker Desktop the
containers run inside a VM, so the API needs a published port and the database
is reached by a different name:

```sh
DOCKER_NETWORK=bridge \
DATABASE_URL='postgres://postgres:postgres@host.docker.internal:5432/app?sslmode=disable' \
  make docker-smoke
```
