#!/bin/sh
# Run the production images against a real database and check that they work.
#
# Building an image proves it compiles and that its files exist. It does not
# prove the binary starts: a distroless image carries no dynamic loader, so a
# build that stops being static, an entrypoint that names a path the copy did
# not produce, or a binary the nonroot user cannot execute all build cleanly and
# die on first run. This runs both artifacts the way a deployment does.

set -eu

API_IMAGE="${API_IMAGE:-go-chi-bp:check}"
MIGRATE_IMAGE="${MIGRATE_IMAGE:-go-chi-bp-migrate:check}"
API_PORT="${API_PORT:-8080}"
# Host networking lets the containers reach a database published on the host,
# which is how CI's service container is exposed. Docker Desktop needs a
# different mode, so it stays overridable.
DOCKER_NETWORK="${DOCKER_NETWORK:-host}"
CONTAINER_NAME="go-chi-bp-smoke-api"
READY_ATTEMPTS="${READY_ATTEMPTS:-30}"

if [ -z "${DATABASE_URL:-}" ]; then
	echo "DATABASE_URL is required" >&2
	exit 1
fi

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		echo "--- api container logs ---" >&2
		docker logs "$CONTAINER_NAME" >&2 2>&1 || true
	fi
	docker rm --force "$CONTAINER_NAME" >/dev/null 2>&1 || true
	exit "$status"
}
trap cleanup EXIT INT TERM

echo "==> migration job applies the schema"
docker run --rm --network "$DOCKER_NETWORK" \
	--env DATABASE_URL="$DATABASE_URL" \
	"$MIGRATE_IMAGE"

echo "==> migration job refuses to run without a database URL"
# A deployment gate is only a gate if the job fails when it cannot do its work.
if docker run --rm --network "$DOCKER_NETWORK" "$MIGRATE_IMAGE" >/dev/null 2>&1; then
	echo "the migration job exited 0 with no DATABASE_URL set" >&2
	exit 1
fi

echo "==> api starts and reports ready"
docker rm --force "$CONTAINER_NAME" >/dev/null 2>&1 || true
docker run --detach --name "$CONTAINER_NAME" --network "$DOCKER_NETWORK" \
	--env DATABASE_URL="$DATABASE_URL" \
	--env HTTP_ADDR=":$API_PORT" \
	--env SHUTDOWN_DRAIN_DELAY=0s \
	"$API_IMAGE" >/dev/null

attempt=1
while [ "$attempt" -le "$READY_ATTEMPTS" ]; do
	if curl --fail --silent --show-error "http://localhost:$API_PORT/health/ready" >/dev/null 2>&1; then
		break
	fi
	if [ "$attempt" -eq "$READY_ATTEMPTS" ]; then
		echo "the api never reported ready after $READY_ATTEMPTS attempts" >&2
		exit 1
	fi
	attempt=$((attempt + 1))
	sleep 1
done

echo "==> liveness answers"
curl --fail --silent --show-error "http://localhost:$API_PORT/health/live" >/dev/null

# Readiness only proves the pool answered a ping. Writing and reading a row
# proves the schema the migration job applied is the one this binary expects.
echo "==> a write reaches the migrated schema"
created="$(curl --fail --silent --show-error --request POST \
	--header 'Content-Type: application/json' \
	--data '{"name":"container smoke"}' \
	"http://localhost:$API_PORT/v1/widgets")"
echo "$created" | grep -q 'container smoke' || {
	echo "the created widget was not returned: $created" >&2
	exit 1
}

echo "==> the row is listed back"
curl --fail --silent --show-error "http://localhost:$API_PORT/v1/widgets?limit=1" |
	grep -q 'container smoke' || {
	echo "the created widget was not listed" >&2
	exit 1
}

echo "container smoke test passed"
