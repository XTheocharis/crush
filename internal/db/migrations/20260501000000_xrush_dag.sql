-- +goose Up
-- +goose StatementBegin

-- Extend lcm_summaries with expanded kind values and a metadata column.
-- SQLite does not support ALTER COLUMN, so we recreate the table.

-- Disable foreign keys during table swap to avoid cascading side-effects.
PRAGMA foreign_keys = OFF;

CREATE TABLE lcm_summaries_new (
    summary_id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('leaf', 'condensed', 'observation', 'auto_memory', 'session', 'repo', 'archive_stub')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    file_ids TEXT NOT NULL DEFAULT '[]',
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- Copy existing data, defaulting metadata to empty JSON object.
INSERT INTO lcm_summaries_new (summary_id, session_id, kind, content, token_count, file_ids, metadata, created_at)
SELECT summary_id, session_id, kind, content, token_count, file_ids, '{}', created_at
FROM lcm_summaries;

-- Drop dependent objects before swapping.
DROP TABLE IF EXISTS lcm_summaries_fts;
DROP INDEX IF EXISTS idx_lcm_summaries_session;

-- Swap tables.
DROP TABLE lcm_summaries;
ALTER TABLE lcm_summaries_new RENAME TO lcm_summaries;

-- Recreate index on swapped table.
CREATE INDEX IF NOT EXISTS idx_lcm_summaries_session ON lcm_summaries(session_id);

-- Recreate FTS5 virtual table for summaries.
CREATE VIRTUAL TABLE IF NOT EXISTS lcm_summaries_fts USING fts5(
    content,
    content='lcm_summaries',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

-- Re-enable foreign keys.
PRAGMA foreign_keys = ON;

-- Create lcm_reversible_state table for storing original messages
-- before compression, enabling reversible decompression.
CREATE TABLE IF NOT EXISTS lcm_reversible_state (
    id TEXT NOT NULL PRIMARY KEY,
    summary_id TEXT NOT NULL REFERENCES lcm_summaries(summary_id) ON DELETE CASCADE,
    original_messages TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_lcm_reversible_state_summary ON lcm_reversible_state(summary_id);

-- Create lcm_observation_buffer table for accumulating per-session
-- observations, insights, and mini-summaries before consolidation.
CREATE TABLE IF NOT EXISTS lcm_observation_buffer (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    buffer_type TEXT NOT NULL CHECK(buffer_type IN ('observation', 'insight', 'summary')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_lcm_observation_buffer_session ON lcm_observation_buffer(session_id);

-- Create lcm_auto_memory table for storing automatically extracted
-- facts, decisions, preferences, and lessons from conversations.
CREATE TABLE IF NOT EXISTS lcm_auto_memory (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    memory_type TEXT NOT NULL CHECK(memory_type IN ('fact', 'decision', 'preference', 'lesson')),
    content TEXT NOT NULL DEFAULT '',
    source_message_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_lcm_auto_memory_session ON lcm_auto_memory(session_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Reverse the DAG extensions: drop new tables and restore original lcm_summaries schema.

-- Drop new tables and indexes.
DROP INDEX IF EXISTS idx_lcm_auto_memory_session;
DROP TABLE IF EXISTS lcm_auto_memory;

DROP INDEX IF EXISTS idx_lcm_observation_buffer_session;
DROP TABLE IF EXISTS lcm_observation_buffer;

DROP INDEX IF EXISTS idx_lcm_reversible_state_summary;
DROP TABLE IF EXISTS lcm_reversible_state;

-- Restore original lcm_summaries schema (without metadata, limited kind).
PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS lcm_summaries_fts;
DROP INDEX IF EXISTS idx_lcm_summaries_session;

CREATE TABLE lcm_summaries_old (
    summary_id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK(kind IN ('leaf', 'condensed')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    file_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

-- Copy only rows that match the original kind constraint.
INSERT INTO lcm_summaries_old (summary_id, session_id, kind, content, token_count, file_ids, created_at)
SELECT summary_id, session_id, kind, content, token_count, file_ids, created_at
FROM lcm_summaries
WHERE kind IN ('leaf', 'condensed');

DROP TABLE lcm_summaries;
ALTER TABLE lcm_summaries_old RENAME TO lcm_summaries;

CREATE INDEX IF NOT EXISTS idx_lcm_summaries_session ON lcm_summaries(session_id);

-- Recreate FTS5 virtual table for summaries.
CREATE VIRTUAL TABLE IF NOT EXISTS lcm_summaries_fts USING fts5(
    content,
    content='lcm_summaries',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

PRAGMA foreign_keys = ON;
-- +goose StatementEnd
