-- +goose Up
-- +goose StatementBegin
ALTER TABLE lcm_map_runs ADD COLUMN tool_type TEXT NOT NULL DEFAULT 'agentic_map' CHECK(tool_type IN ('agentic_map', 'llm_map'));
CREATE INDEX IF NOT EXISTS idx_lcm_map_runs_tool_type ON lcm_map_runs(tool_type);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_lcm_map_runs_tool_type;
ALTER TABLE lcm_map_runs DROP COLUMN tool_type;
-- +goose StatementEnd
