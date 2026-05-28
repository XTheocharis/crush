-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN submitted_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN sent_to_llm_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN first_token_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN completed_at INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN completed_at;
ALTER TABLE messages DROP COLUMN first_token_at;
ALTER TABLE messages DROP COLUMN sent_to_llm_at;
ALTER TABLE messages DROP COLUMN submitted_at;
-- +goose StatementEnd
