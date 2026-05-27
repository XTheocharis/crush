-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS scorer_results (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    scorer_name TEXT NOT NULL,
    scorer_type TEXT NOT NULL DEFAULT 'metric' CHECK(scorer_type IN ('metric', 'llm_judge')),
    score REAL NOT NULL DEFAULT 0.0,
    passed INTEGER NOT NULL DEFAULT 0 CHECK(passed IN (0, 1)),
    explanation TEXT NOT NULL DEFAULT '',
    input_hash TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_msg TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_scorer_results_run_id ON scorer_results (run_id);
CREATE INDEX IF NOT EXISTS idx_scorer_results_scorer_name ON scorer_results (scorer_name);
CREATE INDEX IF NOT EXISTS idx_scorer_results_input_hash ON scorer_results (input_hash);
CREATE INDEX IF NOT EXISTS idx_scorer_results_created_at ON scorer_results (created_at);

CREATE TABLE IF NOT EXISTS eval_runs (
    run_id TEXT PRIMARY KEY,
    dataset_path TEXT NOT NULL DEFAULT '',
    scorer_filter TEXT NOT NULL DEFAULT '',
    total_examples INTEGER NOT NULL DEFAULT 0,
    overall_score REAL NOT NULL DEFAULT 0.0,
    overall_passed INTEGER NOT NULL DEFAULT 0 CHECK(overall_passed IN (0, 1)),
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS scorer_results;
DROP TABLE IF EXISTS eval_runs;
-- +goose StatementEnd
