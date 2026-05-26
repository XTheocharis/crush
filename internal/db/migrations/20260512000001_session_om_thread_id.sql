-- +goose Up
-- +goose StatementBegin
CREATE TABLE session_operational_memory_new (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    thread_id  TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    priority   TEXT NOT NULL DEFAULT 'medium',
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (session_id, thread_id, key)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO session_operational_memory_new (session_id, thread_id, key, value, priority, updated_at)
SELECT session_id, '', key, value, priority, updated_at
FROM session_operational_memory;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE session_operational_memory;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE session_operational_memory_new RENAME TO session_operational_memory;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE session_operational_memory_old (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    priority   TEXT NOT NULL DEFAULT 'medium',
    updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
    PRIMARY KEY (session_id, key)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO session_operational_memory_old (session_id, key, value, priority, updated_at)
SELECT session_id, key, value, priority, updated_at
FROM session_operational_memory
WHERE thread_id = '';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE session_operational_memory;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE session_operational_memory_old RENAME TO session_operational_memory;
-- +goose StatementEnd
