-- +goose Up
-- +goose StatementBegin
-- Add seq and token_count columns to messages table
ALTER TABLE messages ADD COLUMN seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN token_count INTEGER NOT NULL DEFAULT 0;

-- Backfill seq using ROW_NUMBER()
UPDATE messages SET seq = (
    SELECT rn FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at) AS rn
        FROM messages
    ) t WHERE t.id = messages.id
);

-- Create unique index AFTER backfill
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_session_seq ON messages(session_id, seq);

-- Create lcm_session_config table
CREATE TABLE IF NOT EXISTS lcm_session_config (
    session_id TEXT NOT NULL PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL DEFAULT '',
    model_ctx_max_tokens INTEGER NOT NULL DEFAULT 128000,
    ctx_cutoff_threshold REAL NOT NULL DEFAULT 0.6,
    soft_threshold_tokens INTEGER NOT NULL DEFAULT 0,
    hard_threshold_tokens INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- Create lcm_summaries table
CREATE TABLE IF NOT EXISTS lcm_summaries (
    summary_id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('leaf', 'condensed')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    file_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_lcm_summaries_session ON lcm_summaries(session_id);

-- Create FTS5 virtual table for summaries
CREATE VIRTUAL TABLE IF NOT EXISTS lcm_summaries_fts USING fts5(
    content,
    content='lcm_summaries',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

-- Create lcm_summary_messages table
CREATE TABLE IF NOT EXISTS lcm_summary_messages (
    summary_id TEXT NOT NULL REFERENCES lcm_summaries(summary_id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    ord INTEGER NOT NULL,
    PRIMARY KEY (summary_id, ord),
    UNIQUE (summary_id, message_id)
);

-- Create lcm_summary_parents table
CREATE TABLE IF NOT EXISTS lcm_summary_parents (
    summary_id        TEXT NOT NULL REFERENCES lcm_summaries(summary_id) ON DELETE CASCADE,
    parent_summary_id TEXT NOT NULL REFERENCES lcm_summaries(summary_id) ON DELETE CASCADE,
    ord               INTEGER NOT NULL,
    PRIMARY KEY (summary_id, ord),
    UNIQUE (summary_id, parent_summary_id)
);

-- Create lcm_context_items table
CREATE TABLE IF NOT EXISTS lcm_context_items (
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    item_type   TEXT NOT NULL CHECK(item_type IN ('message', 'summary')),
    message_id  TEXT REFERENCES messages(id) ON DELETE CASCADE,
    summary_id  TEXT REFERENCES lcm_summaries(summary_id) ON DELETE CASCADE,
    token_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, position),
    CONSTRAINT ctx_item_exactly_one_ref CHECK (
        (item_type = 'message' AND message_id IS NOT NULL AND summary_id IS NULL) OR
        (item_type = 'summary' AND summary_id IS NOT NULL AND message_id IS NULL)
    )
);

-- Create lcm_large_files table
CREATE TABLE IF NOT EXISTS lcm_large_files (
    file_id            TEXT PRIMARY KEY,
    session_id         TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    original_path      TEXT NOT NULL,
    content            TEXT,
    token_count        INTEGER NOT NULL DEFAULT 0,
    exploration_summary TEXT,
    explorer_used      TEXT,
    created_at         INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_lcm_large_files_session ON lcm_large_files(session_id);

-- Create map run tables
CREATE TABLE IF NOT EXISTS lcm_map_runs (
    run_id      TEXT NOT NULL PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'RUNNING' CHECK(status IN ('RUNNING', 'DONE', 'FAILED')),
    input_path  TEXT NOT NULL,
    output_path TEXT NOT NULL,
    schema_json TEXT NOT NULL DEFAULT '{}',
    created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE TABLE IF NOT EXISTS lcm_map_items (
    item_id    TEXT NOT NULL PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES lcm_map_runs(run_id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING', 'RUNNING', 'DONE', 'FAILED')),
    input_json TEXT NOT NULL,
    output_json TEXT,
    error_msg  TEXT,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- Create contentless FTS5 table for messages
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    content,
    content='',
    tokenize='porter unicode61'
);

-- Create INSERT trigger for messages_fts
CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts(rowid, content)
    VALUES (NEW.rowid, (
        SELECT COALESCE(group_concat(c, ' '), '') FROM (
            SELECT json_extract(je.value, '$.content') AS c
            FROM json_each(NEW.parts) AS je
            WHERE json_extract(je.value, '$.type') = 'text'
            ORDER BY je.key
        )
    ));
END;

-- Create UPDATE trigger for messages_fts
CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE OF parts ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.rowid, (
        SELECT COALESCE(group_concat(c, ' '), '') FROM (
            SELECT json_extract(je.value, '$.content') AS c
            FROM json_each(OLD.parts) AS je
            WHERE json_extract(je.value, '$.type') = 'text'
            ORDER BY je.key
        )
    ));
    INSERT INTO messages_fts(rowid, content)
    VALUES (NEW.rowid, (
        SELECT COALESCE(group_concat(c, ' '), '') FROM (
            SELECT json_extract(je.value, '$.content') AS c
            FROM json_each(NEW.parts) AS je
            WHERE json_extract(je.value, '$.type') = 'text'
            ORDER BY je.key
        )
    ));
END;

-- Create DELETE trigger for messages_fts
CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, content)
    VALUES ('delete', OLD.rowid, (
        SELECT COALESCE(group_concat(c, ' '), '') FROM (
            SELECT json_extract(je.value, '$.content') AS c
            FROM json_each(OLD.parts) AS je
            WHERE json_extract(je.value, '$.type') = 'text'
            ORDER BY je.key
        )
    ));
END;

-- Backfill messages_fts for existing messages
INSERT INTO messages_fts(rowid, content)
SELECT m.rowid, (
    SELECT COALESCE(group_concat(c, ' '), '') FROM (
        SELECT json_extract(je.value, '$.content') AS c
        FROM json_each(m.parts) AS je
        WHERE json_extract(je.value, '$.type') = 'text'
        ORDER BY je.key
    )
) FROM messages m;

-- Replace pre-existing updated_at triggers with recursion-safe versions.
-- The originals (from 20250424200609_initial.sql) fire AFTER UPDATE and
-- UPDATE the same table unconditionally, which causes infinite recursion
-- when PRAGMA recursive_triggers = ON (required for FTS5 cleanup triggers
-- to fire during ON DELETE CASCADE). Using UPDATE OF <data columns> ensures
-- the trigger's own UPDATE of updated_at does not re-fire it.
DROP TRIGGER IF EXISTS update_messages_updated_at;
CREATE TRIGGER IF NOT EXISTS update_messages_updated_at
AFTER UPDATE OF session_id, role, parts, model, finished_at, provider, is_summary_message, seq, token_count ON messages
BEGIN
    UPDATE messages SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS update_sessions_updated_at;
CREATE TRIGGER IF NOT EXISTS update_sessions_updated_at
AFTER UPDATE OF parent_session_id, title, message_count, prompt_tokens, completion_tokens, cost, summary_message_id, todos ON sessions
BEGIN
    UPDATE sessions SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS update_files_updated_at;
CREATE TRIGGER IF NOT EXISTS update_files_updated_at
AFTER UPDATE OF session_id, path, content, version ON files
BEGIN
    UPDATE files SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore original updated_at triggers (without WHEN guard).
DROP TRIGGER IF EXISTS update_files_updated_at;
CREATE TRIGGER IF NOT EXISTS update_files_updated_at
AFTER UPDATE ON files
BEGIN
    UPDATE files SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS update_sessions_updated_at;
CREATE TRIGGER IF NOT EXISTS update_sessions_updated_at
AFTER UPDATE ON sessions
BEGIN
    UPDATE sessions SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS update_messages_updated_at;
CREATE TRIGGER IF NOT EXISTS update_messages_updated_at
AFTER UPDATE ON messages
BEGIN
    UPDATE messages SET updated_at = strftime('%s', 'now')
    WHERE id = NEW.id;
END;

-- Drop FTS triggers
DROP TRIGGER IF EXISTS messages_fts_delete;
DROP TRIGGER IF EXISTS messages_fts_update;
DROP TRIGGER IF EXISTS messages_fts_insert;

-- Drop FTS tables
DROP TABLE IF EXISTS messages_fts;

-- Drop map tables
DROP TABLE IF EXISTS lcm_map_items;
DROP TABLE IF EXISTS lcm_map_runs;

-- Drop large files table
DROP TABLE IF EXISTS lcm_large_files;

-- Drop context items table
DROP TABLE IF EXISTS lcm_context_items;

-- Drop summary parent table
DROP TABLE IF EXISTS lcm_summary_parents;

-- Drop summary messages table
DROP TABLE IF EXISTS lcm_summary_messages;

-- Drop FTS virtual table for summaries
DROP TABLE IF EXISTS lcm_summaries_fts;

-- Drop summaries table
DROP TABLE IF EXISTS lcm_summaries;

-- Drop session config table
DROP TABLE IF EXISTS lcm_session_config;

-- Drop index and columns from messages
DROP INDEX IF EXISTS idx_messages_session_seq;
ALTER TABLE messages DROP COLUMN token_count;
ALTER TABLE messages DROP COLUMN seq;
-- +goose StatementEnd
