-- +goose Up
-- +goose StatementBegin
-- Create repo_map_file_cache table
CREATE TABLE IF NOT EXISTS repo_map_file_cache (
    repo_key TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    mtime INTEGER NOT NULL,
    language TEXT NOT NULL DEFAULT '',
    tag_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo_key, rel_path)
);

-- Create repo_map_tags table
CREATE TABLE IF NOT EXISTS repo_map_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_key TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('def', 'ref')),
    node_type TEXT NOT NULL DEFAULT '',
    line INTEGER NOT NULL,
    language TEXT NOT NULL,
    FOREIGN KEY (repo_key, rel_path) REFERENCES repo_map_file_cache(repo_key, rel_path) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_path ON repo_map_tags(repo_key, rel_path);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_name ON repo_map_tags(repo_key, name);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_kind_name ON repo_map_tags(repo_key, kind, name);

-- Create repo_map_session_rankings table
CREATE TABLE IF NOT EXISTS repo_map_session_rankings (
    repo_key TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    rank REAL NOT NULL,
    PRIMARY KEY (repo_key, session_id, rel_path)
);
CREATE INDEX IF NOT EXISTS idx_rmsr_repo_session ON repo_map_session_rankings(repo_key, session_id);

-- Create repo_map_session_read_only table
CREATE TABLE IF NOT EXISTS repo_map_session_read_only (
    repo_key TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    PRIMARY KEY (repo_key, session_id, rel_path)
);
CREATE INDEX IF NOT EXISTS idx_rmsro_repo_session ON repo_map_session_read_only(repo_key, session_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_rmsro_repo_session;
DROP INDEX IF EXISTS idx_rmsr_repo_session;
DROP INDEX IF EXISTS idx_rmt_repo_kind_name;
DROP INDEX IF EXISTS idx_rmt_repo_name;
DROP INDEX IF EXISTS idx_rmt_repo_path;
DROP TABLE IF EXISTS repo_map_session_read_only;
DROP TABLE IF EXISTS repo_map_session_rankings;
DROP TABLE IF EXISTS repo_map_tags;
DROP TABLE IF EXISTS repo_map_file_cache;
-- +goose StatementEnd