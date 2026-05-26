-- +goose Up
-- +goose StatementBegin
ALTER TABLE session_operational_memory ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE session_operational_memory DROP COLUMN priority;
-- +goose StatementEnd
