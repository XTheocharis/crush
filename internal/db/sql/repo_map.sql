-- name: UpsertRepoMapFileCache :exec
INSERT INTO repo_map_file_cache (repo_key, rel_path, mtime, language, tag_count)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(repo_key, rel_path) DO UPDATE SET mtime = excluded.mtime, language = excluded.language, tag_count = excluded.tag_count;

-- name: GetRepoMapFileCache :many
SELECT repo_key, rel_path, mtime, language, tag_count
FROM repo_map_file_cache
WHERE repo_key = ?;

-- name: GetRepoMapFileCacheByPath :one
SELECT repo_key, rel_path, mtime, language, tag_count
FROM repo_map_file_cache
WHERE repo_key = ? AND rel_path = ?;

-- name: InsertRepoMapTag :exec
INSERT INTO repo_map_tags (repo_key, rel_path, name, kind, node_type, line, language)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: DeleteRepoMapTagsByPath :exec
DELETE FROM repo_map_tags
WHERE repo_key = ? AND rel_path = ?;

-- name: ListRepoMapTags :many
SELECT repo_key, rel_path, name, kind, node_type, line, language
FROM repo_map_tags
WHERE repo_key = ?;

-- name: ListRepoMapDefsByName :many
SELECT repo_key, rel_path, name, node_type, line, language
FROM repo_map_tags
WHERE repo_key = ? AND kind = 'def' AND name = ?;

-- name: UpsertSessionRanking :exec
INSERT INTO repo_map_session_rankings (repo_key, session_id, rel_path, rank)
VALUES (?, ?, ?, ?)
ON CONFLICT(repo_key, session_id, rel_path) DO UPDATE SET rank = excluded.rank;

-- name: ListSessionRankings :many
SELECT repo_key, session_id, rel_path, rank
FROM repo_map_session_rankings
WHERE repo_key = ? AND session_id = ?
ORDER BY rank DESC;

-- name: DeleteSessionRankings :exec
DELETE FROM repo_map_session_rankings
WHERE repo_key = ? AND session_id = ?;

-- name: UpsertSessionReadOnlyPath :exec
INSERT INTO repo_map_session_read_only (repo_key, session_id, rel_path)
VALUES (?, ?, ?)
ON CONFLICT(repo_key, session_id, rel_path) DO NOTHING;

-- name: ListSessionReadOnlyPaths :many
SELECT rel_path
FROM repo_map_session_read_only
WHERE repo_key = ? AND session_id = ?
ORDER BY rel_path;

-- name: DeleteSessionReadOnlyPaths :exec
DELETE FROM repo_map_session_read_only
WHERE repo_key = ? AND session_id = ?;

-- name: DeleteRepoMapFileCache :exec
DELETE FROM repo_map_file_cache
WHERE repo_key = ? AND rel_path = ?;
