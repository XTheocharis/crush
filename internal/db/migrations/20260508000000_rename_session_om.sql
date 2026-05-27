-- +goose Up
-- +goose StatementBegin
ALTER TABLE session_om RENAME TO session_operational_memory;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE session_operational_memory RENAME TO session_om;
-- +goose StatementEnd
