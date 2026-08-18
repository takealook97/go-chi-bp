-- name: CreateWidget :one
INSERT INTO widgets (name)
VALUES ($1)
RETURNING id, name, created_at, updated_at;

-- name: GetWidget :one
SELECT id, name, created_at, updated_at
FROM widgets
WHERE id = $1;

-- name: ListWidgets :many
SELECT id, name, created_at, updated_at
FROM widgets
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: ListWidgetsAfter :many
-- Row comparison, not the equivalent OR form: it matches the leading columns of
-- widgets_created_at_id_idx so the planner can start the scan at the cursor.
-- The casts are required because sqlc otherwise infers both row-constructor
-- arguments from the first column and types cursor_id as a timestamp.
SELECT id, name, created_at, updated_at
FROM widgets
WHERE (created_at, id) < (sqlc.arg(cursor_created_at)::timestamptz, sqlc.arg(cursor_id)::bigint)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: DeleteWidget :execrows
DELETE FROM widgets
WHERE id = $1;
