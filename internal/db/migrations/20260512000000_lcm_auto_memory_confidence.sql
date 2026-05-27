-- +goose Up
-- +goose StatementBegin
ALTER TABLE lcm_auto_memory ADD COLUMN confidence REAL NOT NULL DEFAULT 0.0;
ALTER TABLE lcm_auto_memory ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite doesn't support DROP COLUMN before 3.35.0, but we use a version
-- that supports it. For safety, we recreate the table without these columns.
CREATE TABLE lcm_auto_memory_old (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    memory_type TEXT NOT NULL CHECK(memory_type IN ('fact', 'decision', 'preference', 'lesson')),
    content TEXT NOT NULL DEFAULT '',
    source_message_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

INSERT INTO lcm_auto_memory_old (id, session_id, memory_type, content, source_message_ids, created_at)
SELECT id, session_id, memory_type, content, source_message_ids, created_at
FROM lcm_auto_memory;

DROP TABLE lcm_auto_memory;
ALTER TABLE lcm_auto_memory_old RENAME TO lcm_auto_memory;

CREATE INDEX IF NOT EXISTS idx_lcm_auto_memory_session ON lcm_auto_memory(session_id);
-- +goose StatementEnd
