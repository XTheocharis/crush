-- +goose Up
-- +goose StatementBegin

-- Add 'critical' priority level to lcm_observation_buffer for the highest-priority observations.
-- SQLite does not support ALTER TABLE ... ALTER CONSTRAINT, so rebuild the table.
CREATE TABLE lcm_observation_buffer_new (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    buffer_type TEXT NOT NULL CHECK(buffer_type IN ('observation', 'insight', 'summary')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('critical', 'high', 'medium', 'low', 'info')),
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

INSERT INTO lcm_observation_buffer_new (id, session_id, buffer_type, content, token_count, priority, created_at)
SELECT id, session_id, buffer_type, content, token_count, priority, created_at
FROM lcm_observation_buffer;

DROP TABLE lcm_observation_buffer;

ALTER TABLE lcm_observation_buffer_new RENAME TO lcm_observation_buffer;

CREATE INDEX idx_lcm_observation_buffer_session ON lcm_observation_buffer(session_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Revert to the 4-level priority constraint (no critical).
CREATE TABLE lcm_observation_buffer_downgrade (
    id TEXT NOT NULL PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    buffer_type TEXT NOT NULL CHECK(buffer_type IN ('observation', 'insight', 'summary')),
    content TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    priority TEXT NOT NULL DEFAULT 'medium' CHECK(priority IN ('high', 'medium', 'low', 'info')),
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

INSERT INTO lcm_observation_buffer_downgrade (id, session_id, buffer_type, content, token_count, priority, created_at)
SELECT id, session_id, buffer_type, content, token_count,
    CASE WHEN priority = 'critical' THEN 'high' ELSE priority END,
    created_at
FROM lcm_observation_buffer;

DROP TABLE lcm_observation_buffer;

ALTER TABLE lcm_observation_buffer_downgrade RENAME TO lcm_observation_buffer;

CREATE INDEX idx_lcm_observation_buffer_session ON lcm_observation_buffer(session_id);

-- +goose StatementEnd
