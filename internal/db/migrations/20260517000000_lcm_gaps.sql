-- +goose Up
-- +goose StatementBegin

-- Create lcm_content_replacements table for tracking content replacement state
CREATE TABLE IF NOT EXISTS lcm_content_replacements (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id             TEXT    NOT NULL,
    position               INTEGER NOT NULL,
    message_id             TEXT,
    file_id                TEXT,
    state                  TEXT    NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'restored', 'superseded', 'pinned')),
    round                  INTEGER NOT NULL DEFAULT 0,
    original_token_count   INTEGER NOT NULL DEFAULT 0,
    replacement_token_count INTEGER NOT NULL DEFAULT 0,
    created_at             INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at             INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    FOREIGN KEY (session_id, position) REFERENCES lcm_context_items(session_id, position) ON DELETE CASCADE
);

-- Indexes for lcm_content_replacements
CREATE INDEX IF NOT EXISTS idx_cr_session_state ON lcm_content_replacements(session_id, state);
CREATE INDEX IF NOT EXISTS idx_cr_session_round ON lcm_content_replacements(session_id, round);
CREATE INDEX IF NOT EXISTS idx_cr_file_id ON lcm_content_replacements(file_id) WHERE file_id IS NOT NULL;

-- Index for messages session+created_at lookups
CREATE INDEX IF NOT EXISTS idx_messages_session_created ON messages(session_id, created_at);

-- Create FTS5 virtual table for large files full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS lcm_large_files_fts USING fts5(
    content,
    content='lcm_large_files',
    content_rowid='rowid',
    tokenize='porter unicode61 remove_diacritics 2'
);

-- Sync trigger: INSERT on lcm_large_files
CREATE TRIGGER IF NOT EXISTS lcm_large_files_fts_insert AFTER INSERT ON lcm_large_files BEGIN
    INSERT INTO lcm_large_files_fts(rowid, content)
    VALUES (NEW.rowid, NEW.content);
END;

-- Sync trigger: UPDATE on lcm_large_files
CREATE TRIGGER IF NOT EXISTS lcm_large_files_fts_update AFTER UPDATE OF content ON lcm_large_files BEGIN
    INSERT INTO lcm_large_files_fts(lcm_large_files_fts, rowid, content)
    VALUES ('delete', OLD.rowid, OLD.content);
    INSERT INTO lcm_large_files_fts(rowid, content)
    VALUES (NEW.rowid, NEW.content);
END;

-- Sync trigger: DELETE on lcm_large_files
CREATE TRIGGER IF NOT EXISTS lcm_large_files_fts_delete AFTER DELETE ON lcm_large_files BEGIN
    INSERT INTO lcm_large_files_fts(lcm_large_files_fts, rowid, content)
    VALUES ('delete', OLD.rowid, OLD.content);
END;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop FTS sync triggers (reverse order)
DROP TRIGGER IF EXISTS lcm_large_files_fts_delete;
DROP TRIGGER IF EXISTS lcm_large_files_fts_update;
DROP TRIGGER IF EXISTS lcm_large_files_fts_insert;

-- Drop FTS virtual table
DROP TABLE IF EXISTS lcm_large_files_fts;

-- Drop indexes
DROP INDEX IF EXISTS idx_messages_session_created;
DROP INDEX IF EXISTS idx_cr_file_id;
DROP INDEX IF EXISTS idx_cr_session_round;
DROP INDEX IF EXISTS idx_cr_session_state;

-- Drop content replacements table
DROP TABLE IF EXISTS lcm_content_replacements;

-- +goose StatementEnd
