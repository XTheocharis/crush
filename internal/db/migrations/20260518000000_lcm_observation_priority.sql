-- +goose Up
-- +goose StatementBegin

-- Add priority column to lcm_observation_buffer for priority-based ordering
-- of observations. High-priority observations sort first when reflecting.
ALTER TABLE lcm_observation_buffer ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('high', 'medium', 'low'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- SQLite does not support DROP COLUMN for tables with CHECK constraints,
-- so rebuild the table without the priority column.
CREATE TABLE lcm_observation_buffer_new (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    buffer_type TEXT NOT NULL CHECK(buffer_type IN ('observation', 'insight', 'summary')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

INSERT INTO lcm_observation_buffer_new (id, session_id, buffer_type, content, token_count, created_at)
SELECT id, session_id, buffer_type, content, token_count, created_at
FROM lcm_observation_buffer;

DROP TABLE lcm_observation_buffer;

ALTER TABLE lcm_observation_buffer_new RENAME TO lcm_observation_buffer;

CREATE INDEX idx_lcm_observation_buffer_session ON lcm_observation_buffer(session_id);

-- +goose StatementEnd
