package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEvalPipelineRunCreatesRunRow verifies that a full runner execution
// persists an eval_runs row to the database with correct metadata.
func TestEvalPipelineRunCreatesRunRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	storage := NewScorerStorage(db)
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})

	runner := NewEvalRunner(h, storage)
	dataset := &Dataset{
		Name: "pipeline-row-test",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "row check",
				Input: &EvalInput{
					SessionID: "s_pipeline_row",
					Files:     map[string]string{"main.go": "package main"},
				},
			},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/pipeline-row.json", "")
	require.NoError(t, err)
	require.NotEmpty(t, outcome.RunID)

	// Verify eval_runs row exists with correct metadata.
	run, err := storage.GetRun(ctx, outcome.RunID)
	require.NoError(t, err)
	require.Equal(t, "/data/pipeline-row.json", run.DatasetPath)
	require.Equal(t, 1, run.TotalExamples)
	require.True(t, run.OverallPassed)
	require.InDelta(t, 0.9, run.OverallScore, 0.01)
}

// TestEvalPipelineScorerResultsCountMatchesScorers verifies that the number
// of scorer_results rows equals the number of registered scorers.
func TestEvalPipelineScorerResultsCountMatchesScorers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	storage := NewScorerStorage(db)
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge, score: 0.8, passed: true})
	h.Register(&stubScorer{name: "style", sType: ScorerMastra, score: 0.7, passed: true})

	runner := NewEvalRunner(h, storage)
	dataset := &Dataset{
		Name: "count-test",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "count check",
				Input: &EvalInput{
					SessionID: "s_count",
					Files:     map[string]string{"a.go": "package main"},
				},
			},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/count.json", "")
	require.NoError(t, err)

	// Verify scorer_results row count matches scorer count.
	results, err := storage.GetResultsByRun(ctx, outcome.Results[0].Report.SessionID)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

// TestEvalPipelineFailingScorerRepresentedCorrectly verifies that a scorer
// returning an error is persisted with its error message and Passed=false
// rather than being silently omitted.
func TestEvalPipelineFailingScorerRepresentedCorrectly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	storage := NewScorerStorage(db)
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "good", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "broken", sType: ScorerMetric, err: fmt.Errorf("timeout after 30s")})

	runner := NewEvalRunner(h, storage)
	dataset := &Dataset{
		Name: "fail-test",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "fail check",
				Input: &EvalInput{
					SessionID: "s_fail",
					Files:     map[string]string{"a.go": "package main"},
				},
			},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/fail.json", "")
	require.NoError(t, err)
	require.False(t, outcome.Passed)

	// Verify both scorer_results rows exist (failing scorer not omitted).
	results, err := storage.GetResultsByRun(ctx, outcome.Results[0].Report.SessionID)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Find the failing result and verify its error representation.
	var found bool
	for _, r := range results {
		if r.ScorerName == "broken" {
			found = true
			require.False(t, r.Passed)
			require.Equal(t, "timeout after 30s", r.ErrorMsg)
		}
	}
	require.True(t, found, "failing scorer 'broken' must be present in results")
}

// TestEvalPipelineScorerListingIncludesExpectedScorers verifies that
// registering a set of XRUSH-relevant scorers produces the correct listing.
func TestEvalPipelineScorerListingIncludesExpectedScorers(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()

	expectedScorers := []struct {
		name  string
		sType ScorerType
	}{
		{"build_success", ScorerMetric},
		{"test_pass_rate", ScorerMetric},
		{"lint_score", ScorerMetric},
		{"syntax_validity", ScorerMetric},
		{"code_quality", ScorerLLMJudge},
	}

	for _, s := range expectedScorers {
		h.Register(&stubScorer{name: s.name, sType: s.sType, score: 0.8, passed: true})
	}

	names := h.Scorers()
	require.Len(t, names, len(expectedScorers))

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	for _, s := range expectedScorers {
		require.True(t, nameSet[s.name], "expected scorer %q in listing", s.name)
	}
}

// TestEvalPipelineEndToEndWithDatasetFile verifies a complete end-to-end
// pipeline: write dataset to disk, load it, run through harness with storage,
// and verify both eval_runs and scorer_results are populated correctly.
func TestEvalPipelineEndToEndWithDatasetFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Write a tiny dataset to disk.
	dataset := &Dataset{
		Name: "e2e-pipeline",
		Examples: []DatasetExample{
			{
				ID:   "e2e_1",
				Name: "end to end",
				Input: &EvalInput{
					SessionID: "s_e2e",
					Conversation: []Message{
						{Role: "user", Content: "fix bug"},
						{Role: "assistant", Content: "done"},
					},
					Edits: []FileEdit{
						{Path: "main.go", Before: "old", After: "new"},
					},
					Files: map[string]string{"main.go": "package main"},
				},
			},
		},
	}

	dir := t.TempDir()
	dsPath := filepath.Join(dir, "e2e-dataset.json")
	data, err := json.MarshalIndent(dataset, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dsPath, data, 0o644))

	// Load dataset from disk.
	loaded, err := LoadDataset(dsPath)
	require.NoError(t, err)
	require.Equal(t, "e2e-pipeline", loaded.Name)

	// Run through harness with storage.
	storage := NewScorerStorage(db)
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.95, passed: true})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge, score: 0.88, passed: true})

	runner := NewEvalRunner(h, storage)
	outcome, err := runner.Run(ctx, loaded, dsPath, "")
	require.NoError(t, err)
	require.True(t, outcome.Passed)
	require.Len(t, outcome.Results, 1)

	// Verify eval_runs row.
	run, err := storage.GetRun(ctx, outcome.RunID)
	require.NoError(t, err)
	require.Equal(t, dsPath, run.DatasetPath)
	require.Equal(t, 1, run.TotalExamples)

	// Verify scorer_results rows.
	results, err := storage.GetResultsByRun(ctx, outcome.Results[0].Report.SessionID)
	require.NoError(t, err)
	require.Len(t, results, 2)

	scorerNames := make(map[string]bool, len(results))
	for _, r := range results {
		scorerNames[r.ScorerName] = true
		require.True(t, r.Passed)
	}
	require.True(t, scorerNames["accuracy"])
	require.True(t, scorerNames["safety"])
}

// TestEvalPipelinePartialScorerResult verifies that a scorer returning a low
// (failing) score is persisted correctly with Passed=false and its score.
func TestEvalPipelinePartialScorerResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	storage := NewScorerStorage(db)
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "passing", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "partial", sType: ScorerMetric, score: 0.3, passed: false})

	runner := NewEvalRunner(h, storage)
	dataset := &Dataset{
		Name: "partial-test",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "partial check",
				Input: &EvalInput{
					SessionID: "s_partial",
					Files:     map[string]string{"a.go": "code"},
				},
			},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/partial.json", "")
	require.NoError(t, err)
	require.False(t, outcome.Passed)

	results, err := storage.GetResultsByRun(ctx, outcome.Results[0].Report.SessionID)
	require.NoError(t, err)
	require.Len(t, results, 2)

	for _, r := range results {
		if r.ScorerName == "partial" {
			require.False(t, r.Passed, "partial scorer should have Passed=false")
			require.InDelta(t, 0.3, r.Score, 0.001, "partial scorer should retain its score")
			require.Empty(t, r.ErrorMsg, "partial scorer should have no error (just low score)")
		}
	}
}
