-- Turn Snapshot CRUD

-- name: CreateTurnSnapshot :one
INSERT INTO turn_snapshots (
    id,
    session_id,
    user_message_id,
    user_message_seq,
    created_at
) VALUES (
    ?, ?, ?, ?, strftime('%s', 'now')
)
RETURNING *;

-- name: GetTurnSnapshot :one
SELECT * FROM turn_snapshots
WHERE id = ? LIMIT 1;

-- name: GetTurnSnapshotByMessage :one
SELECT * FROM turn_snapshots
WHERE session_id = ? AND user_message_seq = ? LIMIT 1;

-- name: ListTurnSnapshotsBySession :many
SELECT * FROM turn_snapshots
WHERE session_id = ?
ORDER BY user_message_seq ASC;

-- name: GetLatestTurnSnapshot :one
SELECT * FROM turn_snapshots
WHERE session_id = ?
ORDER BY user_message_seq DESC
LIMIT 1;

-- name: GetTurnSnapshotAtOrBeforeSeq :one
SELECT * FROM turn_snapshots
WHERE session_id = ? AND user_message_seq <= ?
ORDER BY user_message_seq DESC
LIMIT 1;

-- name: DeleteTurnSnapshot :exec
DELETE FROM turn_snapshots
WHERE id = ?;

-- name: DeleteSessionTurnSnapshots :exec
DELETE FROM turn_snapshots
WHERE session_id = ?;

-- name: DeleteSnapshotsAfterSeq :exec
DELETE FROM turn_snapshots
WHERE session_id = ? AND user_message_seq > ?;

-- name: CountTurnSnapshots :one
SELECT COUNT(*) FROM turn_snapshots
WHERE session_id = ?;

-- name: DeleteOldTurnSnapshots :execrows
DELETE FROM turn_snapshots
WHERE id IN (
    SELECT id FROM turn_snapshots AS ts_inner
    WHERE ts_inner.session_id = ?
    ORDER BY ts_inner.created_at ASC
    LIMIT (
        SELECT COUNT(*) - ? FROM turn_snapshots AS ts_count WHERE ts_count.session_id = ?
    )
);

-- Snapshot file bridge

-- name: AddSnapshotFile :exec
INSERT INTO turn_snapshot_files (snapshot_id, file_id, path, version)
VALUES (?, ?, ?, ?)
ON CONFLICT(snapshot_id, path) DO UPDATE SET
    file_id = excluded.file_id,
    version = excluded.version;

-- name: ListSnapshotFiles :many
SELECT tsf.*, f.content
FROM turn_snapshot_files tsf
JOIN files f ON tsf.file_id = f.id
WHERE tsf.snapshot_id = ?;

-- name: DeleteSnapshotFiles :exec
DELETE FROM turn_snapshot_files
WHERE snapshot_id = ?;

-- Message operations for undo/rewind

-- name: DeleteMessagesAfterSeq :exec
DELETE FROM messages
WHERE session_id = ? AND seq > ?;

-- name: GetLatestUserMessage :one
SELECT * FROM messages
WHERE session_id = ? AND role = 'user'
ORDER BY seq DESC
LIMIT 1;

-- name: GetMessageBySessionAndSeq :one
SELECT * FROM messages
WHERE session_id = ? AND seq = ? LIMIT 1;

-- Fork operations

-- name: CloneSessionMessages :exec
INSERT INTO messages (
    id,
    session_id,
    role,
    parts,
    seq,
    model,
    provider,
    is_summary_message,
    token_count,
    created_at,
    updated_at
)
SELECT
    ? || m.id,
    ?,
    m.role,
    m.parts,
    (SELECT COALESCE(MAX(seq), 0) FROM messages sub WHERE sub.session_id = ?) + ROW_NUMBER() OVER (ORDER BY m.seq ASC),
    m.model,
    m.provider,
    m.is_summary_message,
    m.token_count,
    m.created_at,
    m.updated_at
FROM messages m
WHERE m.session_id = ?
ORDER BY m.seq ASC;

-- name: CloneSessionFiles :exec
INSERT INTO files (
    id,
    session_id,
    path,
    content,
    version,
    created_at,
    updated_at
)
SELECT
    ? || f.id,
    ?,
    f.path,
    f.content,
    f.version,
    f.created_at,
    f.updated_at
FROM files f
WHERE f.session_id = ?;
