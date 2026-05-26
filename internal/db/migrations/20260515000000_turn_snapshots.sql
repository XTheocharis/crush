-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS turn_snapshots (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_message_seq INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_turn_snapshots_session ON turn_snapshots(session_id);
CREATE INDEX IF NOT EXISTS idx_turn_snapshots_session_seq ON turn_snapshots(session_id, user_message_seq);

CREATE TABLE IF NOT EXISTS turn_snapshot_files (
    snapshot_id TEXT NOT NULL REFERENCES turn_snapshots(id) ON DELETE CASCADE,
    file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    version INTEGER NOT NULL,
    PRIMARY KEY (snapshot_id, path)
);

CREATE INDEX IF NOT EXISTS idx_turn_snapshot_files_snapshot ON turn_snapshot_files(snapshot_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_turn_snapshot_files_snapshot;
DROP TABLE IF EXISTS turn_snapshot_files;
DROP INDEX IF EXISTS idx_turn_snapshots_session_seq;
DROP INDEX IF EXISTS idx_turn_snapshots_session;
DROP TABLE IF EXISTS turn_snapshots;
-- +goose StatementEnd
