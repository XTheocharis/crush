-- name: GetMessage :one
SELECT *
FROM messages
WHERE id = ? LIMIT 1;

-- name: ListMessagesBySession :many
SELECT *
FROM messages
WHERE session_id = ?
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO messages (
    id,
    session_id,
    role,
    parts,
    model,
    provider,
    is_summary_message,
    seq,
    created_at,
    updated_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(session_id),
    sqlc.arg(role),
    sqlc.arg(parts),
    sqlc.arg(model),
    sqlc.arg(provider),
    sqlc.arg(is_summary_message),
    (SELECT COALESCE(MAX(m.seq), 0) + 1 FROM messages m WHERE m.session_id = sqlc.arg(session_id)),
    strftime('%s', 'now'),
    strftime('%s', 'now')
)
RETURNING *;

-- name: UpdateMessage :exec
UPDATE messages
SET
    parts = ?,
    finished_at = ?,
    updated_at = strftime('%s', 'now')
WHERE id = ?;


-- name: DeleteMessage :exec
DELETE FROM messages
WHERE id = ?;

-- name: DeleteSessionMessages :exec
DELETE FROM messages
WHERE session_id = ?;

-- name: ListUserMessagesBySession :many
SELECT *
FROM messages
WHERE session_id = ? AND role = 'user'
ORDER BY created_at DESC;

-- name: ListAllUserMessages :many
SELECT *
FROM messages
WHERE role = 'user'
ORDER BY created_at DESC;
