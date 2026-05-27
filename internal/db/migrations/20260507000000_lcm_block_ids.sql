-- +goose Up
-- +goose StatementBegin
-- Add block_id and original_content columns for lossless expansion.
ALTER TABLE lcm_summaries ADD COLUMN block_id TEXT NOT NULL DEFAULT '';
ALTER TABLE lcm_summaries ADD COLUMN original_content TEXT NOT NULL DEFAULT '';

-- Index for fast block_id lookups.
CREATE INDEX IF NOT EXISTS idx_lcm_block_id ON lcm_summaries(session_id, block_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_lcm_block_id;

-- SQLite does not support DROP COLUMN before 3.35.0, and existing prod
-- SQLite may be older. Recreate the table without the new columns.

PRAGMA foreign_keys = OFF;

CREATE TABLE lcm_summaries_old (
    summary_id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('leaf', 'condensed', 'observation', 'auto_memory', 'session', 'repo', 'archive_stub')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    file_ids TEXT NOT NULL DEFAULT '[]',
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

INSERT INTO lcm_summaries_old (summary_id, session_id, kind, content, token_count, file_ids, metadata, created_at)
SELECT summary_id, session_id, kind, content, token_count, file_ids, metadata, created_at
FROM lcm_summaries;

DROP TABLE IF EXISTS lcm_summaries_fts;
DROP INDEX IF EXISTS idx_lcm_summaries_session;
DROP TABLE lcm_summaries;
ALTER TABLE lcm_summaries_old RENAME TO lcm_summaries;

CREATE INDEX IF NOT EXISTS idx_lcm_summaries_session ON lcm_summaries(session_id);

CREATE VIRTUAL TABLE IF NOT EXISTS lcm_summaries_fts USING fts5(
    content,
    content='lcm_summaries',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

PRAGMA foreign_keys = ON;
-- +goose StatementEnd
