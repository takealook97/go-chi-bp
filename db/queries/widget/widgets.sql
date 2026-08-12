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
SELECT id, name, created_at, updated_at
FROM widgets
WHERE created_at < sqlc.arg(cursor_created_at)
   OR (created_at = sqlc.arg(cursor_created_at) AND id < sqlc.arg(cursor_id))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: DeleteWidget :execrows
DELETE FROM widgets
WHERE id = $1;
