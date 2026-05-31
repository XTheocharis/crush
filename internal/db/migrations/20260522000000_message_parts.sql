-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS message_parts (
    part_id      TEXT    NOT NULL PRIMARY KEY,
    message_id   TEXT    NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id   TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    part_type    TEXT    NOT NULL CHECK(part_type IN ('text', 'reasoning', 'tool_call', 'tool_result', 'finish', 'image_url', 'binary')),
    part_index   INTEGER NOT NULL,
    content_json TEXT    NOT NULL,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_message_parts_message_id ON message_parts(message_id, part_index);
CREATE INDEX IF NOT EXISTS idx_message_parts_session_type ON message_parts(session_id, part_type);
CREATE INDEX IF NOT EXISTS idx_message_parts_type ON message_parts(part_type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS message_parts;
-- +goose StatementEnd
