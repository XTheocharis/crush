package eval

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StoredScorerResult represents a persisted scorer result row.
type StoredScorerResult struct {
	ID          string
	RunID       string
	ScorerName  string
	ScorerType  string
	Score       float64
	Passed      bool
	Explanation string
	InputHash   string
	DetailsJSON string
	DurationMs  int64
	ErrorMsg    string
	CreatedAt   int64
}

// StoredEvalRun represents a persisted eval run row.
type StoredEvalRun struct {
	RunID         string
	DatasetPath   string
	ScorerFilter  string
	TotalExamples int
	OverallScore  float64
	OverallPassed bool
	DurationMs    int64
	CreatedAt     int64
}

// ScorerStorage persists scorer results and eval runs to SQLite.
type ScorerStorage struct {
	db *sql.DB
}

// NewScorerStorage creates a new ScorerStorage backed by the given database.
func NewScorerStorage(db *sql.DB) *ScorerStorage {
	return &ScorerStorage{db: db}
}

// SaveResult persists a single ScoreResult from an EvalReport entry.
func (s *ScorerStorage) SaveResult(ctx context.Context, runID string, entry ScorerResultEntry, inputHash string) error {
	if entry.Result == nil {
		return nil
	}

	result := entry.Result
	detailsJSON := "{}"
	if result.Details != nil {
		if b, err := json.Marshal(result.Details); err == nil {
			detailsJSON = string(b)
		}
	}

	passed := 0
	if result.Passed {
		passed = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scorer_results (
			id, run_id, scorer_name, scorer_type, score, passed,
			explanation, input_hash, details_json, duration_ms, error_msg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(),
		runID,
		entry.Name,
		entry.Type.String(),
		result.Score,
		passed,
		result.Explanation,
		inputHash,
		detailsJSON,
		result.Duration.Milliseconds(),
		result.Error,
	)
	return err
}

// SaveReport persists all results from an EvalReport under a single run ID.
func (s *ScorerStorage) SaveReport(ctx context.Context, report *EvalReport, inputHash string) error {
	if report == nil {
		return nil
	}

	for _, entry := range report.Results {
		if err := s.SaveResult(ctx, report.SessionID, entry, inputHash); err != nil {
			return fmt.Errorf("save result for scorer %q: %w", entry.Name, err)
		}
	}
	return nil
}

// SaveRun persists eval run metadata.
func (s *ScorerStorage) SaveRun(ctx context.Context, run *StoredEvalRun) error {
	if run.RunID == "" {
		run.RunID = uuid.NewString()
	}

	passed := 0
	if run.OverallPassed {
		passed = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_runs (
			run_id, dataset_path, scorer_filter, total_examples,
			overall_score, overall_passed, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.RunID,
		run.DatasetPath,
		run.ScorerFilter,
		run.TotalExamples,
		run.OverallScore,
		passed,
		run.DurationMs,
	)
	return err
}

// GetResultsByRun retrieves all scorer results for a given run ID.
func (s *ScorerStorage) GetResultsByRun(ctx context.Context, runID string) ([]StoredScorerResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, scorer_name, scorer_type, score, passed,
			explanation, input_hash, details_json, duration_ms, error_msg, created_at
		FROM scorer_results
		WHERE run_id = ?
		ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanResults(rows)
}

// GetResultsByScorer retrieves all results for a given scorer name.
func (s *ScorerStorage) GetResultsByScorer(ctx context.Context, scorerName string) ([]StoredScorerResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, scorer_name, scorer_type, score, passed,
			explanation, input_hash, details_json, duration_ms, error_msg, created_at
		FROM scorer_results
		WHERE scorer_name = ?
		ORDER BY created_at DESC`, scorerName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanResults(rows)
}

// GetRun retrieves a single eval run by ID.
func (s *ScorerStorage) GetRun(ctx context.Context, runID string) (*StoredEvalRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, dataset_path, scorer_filter, total_examples,
			overall_score, overall_passed, duration_ms, created_at
		FROM eval_runs
		WHERE run_id = ?`, runID)

	var run StoredEvalRun
	var passed int
	if err := row.Scan(
		&run.RunID, &run.DatasetPath, &run.ScorerFilter,
		&run.TotalExamples, &run.OverallScore, &passed,
		&run.DurationMs, &run.CreatedAt,
	); err != nil {
		return nil, err
	}
	run.OverallPassed = passed == 1
	return &run, nil
}

// ListRuns returns all eval runs ordered by creation time (newest first).
func (s *ScorerStorage) ListRuns(ctx context.Context) ([]StoredEvalRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, dataset_path, scorer_filter, total_examples,
			overall_score, overall_passed, duration_ms, created_at
		FROM eval_runs
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []StoredEvalRun
	for rows.Next() {
		var run StoredEvalRun
		var passed int
		if err := rows.Scan(
			&run.RunID, &run.DatasetPath, &run.ScorerFilter,
			&run.TotalExamples, &run.OverallScore, &passed,
			&run.DurationMs, &run.CreatedAt,
		); err != nil {
			return nil, err
		}
		run.OverallPassed = passed == 1
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// CompareRuns returns results from two runs for side-by-side comparison.
func (s *ScorerStorage) CompareRuns(ctx context.Context, runID1, runID2 string) (map[string][]StoredScorerResult, error) {
	r1, err := s.GetResultsByRun(ctx, runID1)
	if err != nil {
		return nil, fmt.Errorf("get results for run %s: %w", runID1, err)
	}
	r2, err := s.GetResultsByRun(ctx, runID2)
	if err != nil {
		return nil, fmt.Errorf("get results for run %s: %w", runID2, err)
	}
	return map[string][]StoredScorerResult{runID1: r1, runID2: r2}, nil
}

// HashInput computes a SHA-256 hash of an EvalInput for deduplication.
func HashInput(input *EvalInput) string {
	data, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// OpenTestDB creates a SQLite database in the given directory with
// migrations applied. Intended for tests only.
func OpenTestDB(ctx context.Context, dir string) (*sql.DB, error) {
	dbPath := dir + "/test.db"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open test db: %w", err)
	}

	pragmas := map[string]string{
		"foreign_keys": "ON",
		"journal_mode": "WAL",
	}

	for name, value := range pragmas {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA %s = %s", name, value)); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %s: %w", name, err)
		}
	}

	if err := applyMigrations(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS scorer_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			scorer_name TEXT NOT NULL,
			scorer_type TEXT NOT NULL DEFAULT 'metric' CHECK(scorer_type IN ('metric', 'llm_judge', 'mastra')),
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
	`)
	return err
}

func scanResults(rows *sql.Rows) ([]StoredScorerResult, error) {
	var results []StoredScorerResult
	for rows.Next() {
		var r StoredScorerResult
		var passed int
		if err := rows.Scan(
			&r.ID, &r.RunID, &r.ScorerName, &r.ScorerType,
			&r.Score, &passed, &r.Explanation, &r.InputHash,
			&r.DetailsJSON, &r.DurationMs, &r.ErrorMsg, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.Passed = passed == 1
		results = append(results, r)
	}
	return results, rows.Err()
}

// DurationFromMs converts milliseconds to a time.Duration.
func DurationFromMs(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
