-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS repo_map_imports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    import_path TEXT NOT NULL,
    category TEXT NOT NULL,
    repo_key TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_repo_map_imports_path ON repo_map_imports(path, repo_key);
CREATE INDEX IF NOT EXISTS idx_repo_map_imports_repo_key ON repo_map_imports(repo_key);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_repo_map_imports_repo_key;
DROP INDEX IF EXISTS idx_repo_map_imports_path;
DROP TABLE IF EXISTS repo_map_imports;
-- +goose StatementEnd
