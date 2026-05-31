-- name: InsertMessagePart :one
INSERT INTO message_parts (
    part_id,
    message_id,
    session_id,
    part_type,
    part_index,
    content_json
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMessagePartsByMessageID :many
SELECT *
FROM message_parts
WHERE message_id = ?
ORDER BY part_index ASC;

-- name: GetMessagePartsBySessionAndType :many
SELECT *
FROM message_parts
WHERE session_id = ? AND part_type = ?
ORDER BY created_at ASC;

-- name: DeleteMessagePartsByMessageID :exec
DELETE FROM message_parts
WHERE message_id = ?;

-- name: CountMessagePartsBySession :one
SELECT COUNT(*)
FROM message_parts
WHERE session_id = ?;
