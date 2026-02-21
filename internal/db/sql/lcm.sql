-- name: UpdateMessageTokenCount :exec
UPDATE messages SET token_count = ? WHERE id = ?;

-- name: ListMessagesBySessionSeq :many
SELECT * FROM messages WHERE session_id = ? ORDER BY seq ASC;

-- name: ListMessagesInSeqRange :many
SELECT * FROM messages WHERE session_id = ? AND seq >= ? AND seq <= ? ORDER BY seq ASC;

-- name: ClearSessionSummaryMessageID :exec
UPDATE sessions SET summary_message_id = NULL WHERE id = ?;

-- LCM Session Config
-- name: UpsertLcmSessionConfig :exec
INSERT INTO lcm_session_config (session_id, model_name, model_ctx_max_tokens, ctx_cutoff_threshold, soft_threshold_tokens, hard_threshold_tokens)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO NOTHING;

-- name: GetLcmSessionConfig :one
SELECT * FROM lcm_session_config WHERE session_id = ?;

-- name: UpdateLcmSessionConfig :exec
UPDATE lcm_session_config SET
    model_name = ?,
    model_ctx_max_tokens = ?,
    ctx_cutoff_threshold = ?,
    soft_threshold_tokens = ?,
    hard_threshold_tokens = ?,
    updated_at = strftime('%s', 'now')
WHERE session_id = ?;

-- LCM Summaries
-- name: InsertLcmSummary :exec
INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetLcmSummary :one
SELECT * FROM lcm_summaries WHERE summary_id = ?;

-- name: ListLcmSummariesBySession :many
SELECT * FROM lcm_summaries WHERE session_id = ? ORDER BY created_at ASC;

-- name: DeleteLcmSummary :exec
DELETE FROM lcm_summaries WHERE summary_id = ?;

-- name: SearchLcmSummaries :many
SELECT summary_id, kind FROM lcm_summaries
WHERE rowid IN (
    SELECT lcm_summaries_fts.rowid FROM lcm_summaries_fts WHERE lcm_summaries_fts.content MATCH ?
)
AND session_id = ?;

-- LCM Summary Messages
-- name: InsertLcmSummaryMessage :exec
INSERT INTO lcm_summary_messages (summary_id, message_id, ord)
VALUES (?, ?, ?);

-- name: ListLcmSummaryMessages :many
SELECT * FROM lcm_summary_messages WHERE summary_id = ? ORDER BY ord ASC;

-- name: DeleteLcmSummaryMessages :exec
DELETE FROM lcm_summary_messages WHERE summary_id = ?;

-- LCM Summary Parents
-- name: InsertLcmSummaryParent :exec
INSERT INTO lcm_summary_parents (summary_id, parent_summary_id, ord)
VALUES (?, ?, ?);

-- name: ListLcmSummaryParents :many
SELECT * FROM lcm_summary_parents WHERE summary_id = ? ORDER BY ord ASC;

-- name: DeleteLcmSummaryParents :exec
DELETE FROM lcm_summary_parents WHERE summary_id = ?;

-- LCM Context Items
-- name: InsertLcmContextItem :exec
INSERT INTO lcm_context_items (session_id, position, item_type, message_id, summary_id, token_count)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListLcmContextItems :many
SELECT * FROM lcm_context_items WHERE session_id = ? ORDER BY position ASC;

-- name: DeleteAllLcmContextItems :exec
DELETE FROM lcm_context_items WHERE session_id = ?;

-- name: GetLcmContextTokenCount :one
SELECT COALESCE(SUM(token_count), 0) AS total FROM lcm_context_items WHERE session_id = ?;

-- LCM Large Files
-- name: InsertLcmLargeFile :exec
INSERT INTO lcm_large_files (file_id, session_id, original_path, content, token_count, exploration_summary, explorer_used)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(file_id) DO NOTHING;

-- name: GetLcmLargeFile :one
SELECT * FROM lcm_large_files WHERE file_id = ?;

-- name: ListLcmLargeFilesBySession :many
SELECT * FROM lcm_large_files WHERE session_id = ? ORDER BY created_at ASC;

-- name: UpdateLcmLargeFileExploration :exec
UPDATE lcm_large_files SET exploration_summary = ?, explorer_used = ? WHERE file_id = ?;

-- LCM Map Runs
-- name: InsertLcmMapRun :exec
INSERT INTO lcm_map_runs (run_id, session_id, input_path, output_path, schema_json)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateLcmMapRunStatus :exec
UPDATE lcm_map_runs SET status = ?, updated_at = strftime('%s', 'now') WHERE run_id = ?;

-- LCM Map Items
-- name: InsertLcmMapItem :exec
INSERT INTO lcm_map_items (item_id, run_id, input_json) VALUES (?, ?, ?);

-- name: UpdateLcmMapItem :exec
UPDATE lcm_map_items SET status = ?, output_json = ?, error_msg = ?, updated_at = strftime('%s', 'now') WHERE item_id = ?;
