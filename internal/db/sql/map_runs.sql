-- name: InsertMapRun :exec
INSERT INTO lcm_map_runs (run_id, session_id, input_path, output_path, schema_json, tool_type)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetMapRun :one
SELECT * FROM lcm_map_runs WHERE run_id = ?;

-- name: GetMapRunItems :many
SELECT * FROM lcm_map_items WHERE run_id = ? ORDER BY created_at ASC;

-- name: UpdateMapRunStatus :exec
UPDATE lcm_map_runs SET status = ?, updated_at = strftime('%s', 'now') WHERE run_id = ?;
