package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvalRunner_LoadDataset(t *testing.T) {
	t.Parallel()

	dataset := &Dataset{
		Name: "test-dataset",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "basic test",
				Input: &EvalInput{
					SessionID: "sess_1",
					Files:     map[string]string{"main.go": "package main"},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.json")
	data, err := json.MarshalIndent(dataset, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := LoadDataset(path)
	require.NoError(t, err)
	require.Equal(t, "test-dataset", loaded.Name)
	require.Len(t, loaded.Examples, 1)
	require.Equal(t, "ex_1", loaded.Examples[0].ID)
	require.Equal(t, "sess_1", loaded.Examples[0].Input.SessionID)
}

func TestEvalRunner_LoadDatasetNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadDataset("/nonexistent/path.json")
	require.Error(t, err)
}

func TestEvalRunner_LoadDatasetInvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))

	_, err := LoadDataset(path)
	require.Error(t, err)
}

func TestEvalRunner_LoadDatasetEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	data, _ := json.Marshal(&Dataset{Name: "empty"})
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, err := LoadDataset(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no examples")
}

func TestEvalRunner_RunBasic(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge, score: 0.8, passed: true})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "basic",
		Examples: []DatasetExample{
			{
				ID:   "ex_1",
				Name: "test 1",
				Input: &EvalInput{
					SessionID: "sess_1",
					Files:     map[string]string{"main.go": "package main"},
				},
			},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/test.json", "")
	require.NoError(t, err)
	require.NotEmpty(t, outcome.RunID)
	require.Equal(t, "/data/test.json", outcome.DatasetPath)
	require.Len(t, outcome.Results, 1)
	require.InDelta(t, 0.85, outcome.OverallScore, 0.01)
	require.True(t, outcome.Passed)
	require.True(t, outcome.Duration > 0)
}

func TestEvalRunner_RunMultipleExamples(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.8, passed: true})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "multi",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "first", Input: &EvalInput{SessionID: "s1"}},
			{ID: "ex_2", Name: "second", Input: &EvalInput{SessionID: "s2"}},
			{ID: "ex_3", Name: "third", Input: &EvalInput{SessionID: "s3"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/multi.json", "")
	require.NoError(t, err)
	require.Len(t, outcome.Results, 3)
	require.InDelta(t, 0.8, outcome.OverallScore, 0.01)
	require.True(t, outcome.Passed)
}

func TestEvalRunner_RunWithFailure(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.3, passed: false})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "fail-ds",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "fail test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/fail.json", "")
	require.NoError(t, err)
	require.False(t, outcome.Passed)
	require.InDelta(t, 0.3, outcome.OverallScore, 0.01)
}

func TestEvalRunner_RunNilDataset(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	runner := NewEvalRunner(h, nil)

	_, err := runner.Run(context.Background(), nil, "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "dataset is nil")
}

func TestEvalRunner_RunNilInput(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "nil-input",
		Examples: []DatasetExample{
			{ID: "ex_nil", Name: "nil input", Input: nil},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/nil.json", "")
	require.NoError(t, err)
	require.Len(t, outcome.Results, 1)
	require.Equal(t, "nil input", outcome.Results[0].Error)
	require.False(t, outcome.Passed)
}

func TestEvalRunner_RunWithScorerFilter(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge, score: 0.5, passed: false})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "filtered",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "filtered test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/filt.json", "accuracy")
	require.NoError(t, err)
	require.Len(t, outcome.Results, 1)
	require.Len(t, outcome.Results[0].Report.Results, 1)
	require.Equal(t, "accuracy", outcome.Results[0].Report.Results[0].Name)
	require.True(t, outcome.Passed)
}

func TestEvalRunner_RunWithScorerFilterNonexistent(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "no-filter-match",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/nf.json", "nonexistent")
	require.NoError(t, err)
	require.Len(t, outcome.Results, 1)
	require.Empty(t, outcome.Results[0].Report.Results)
	require.True(t, outcome.Passed)
}

func TestEvalRunner_RunWithStorage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	storage := NewScorerStorage(db)

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.95, passed: true})

	runner := NewEvalRunner(h, storage)

	dataset := &Dataset{
		Name: "stored",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "stored test", Input: &EvalInput{SessionID: "s1", Files: map[string]string{"a.go": "code"}}},
		},
	}

	outcome, err := runner.Run(ctx, dataset, "/data/stored.json", "")
	require.NoError(t, err)
	require.True(t, outcome.Passed)

	run, err := storage.GetRun(ctx, outcome.RunID)
	require.NoError(t, err)
	require.Equal(t, "/data/stored.json", run.DatasetPath)
	require.Equal(t, 1, run.TotalExamples)
	require.True(t, run.OverallPassed)
}

func TestEvalRunner_RunWithScorerError(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "broken", sType: ScorerMetric, err: fmt.Errorf("scorer crashed")})
	h.Register(&stubScorer{name: "good", sType: ScorerMetric, score: 0.9, passed: true})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "error-ds",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "error test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/error.json", "")
	require.NoError(t, err)
	require.False(t, outcome.Passed)
	require.Len(t, outcome.Results, 1)
	require.Len(t, outcome.Results[0].Report.Results, 2)
}

func TestEvalRunner_RunResultInputHash(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})

	runner := NewEvalRunner(h, nil)

	input := &EvalInput{SessionID: "s1", Files: map[string]string{"a.go": "package main"}}
	dataset := &Dataset{
		Name: "hash-ds",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "hash test", Input: input},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/hash.json", "")
	require.NoError(t, err)
	require.Equal(t, HashInput(input), outcome.Results[0].InputHash)
}

func TestEvalRunner_RegisteredScorers(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "a", sType: ScorerMetric, score: 1.0, passed: true})
	h.Register(&stubScorer{name: "b", sType: ScorerLLMJudge, score: 1.0, passed: true})

	scorers := h.RegisteredScorers()
	require.Len(t, scorers, 2)
	require.Equal(t, "a", scorers[0].Name())
	require.Equal(t, "b", scorers[1].Name())
}

func TestEvalRunner_MixedPassFail(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "good", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "bad", sType: ScorerMetric, score: 0.3, passed: false})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "mixed",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "mixed test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/mixed.json", "")
	require.NoError(t, err)
	require.False(t, outcome.Passed)
	require.InDelta(t, 0.6, outcome.OverallScore, 0.01)
}

func TestEvalRunner_DurationTracked(t *testing.T) {
	t.Parallel()

	h := NewEvalHarness()
	h.Register(&stubScorer{name: "slow", sType: ScorerMetric, score: 0.9, passed: true, delay: 20 * time.Millisecond})

	runner := NewEvalRunner(h, nil)

	dataset := &Dataset{
		Name: "slow-ds",
		Examples: []DatasetExample{
			{ID: "ex_1", Name: "slow test", Input: &EvalInput{SessionID: "s1"}},
		},
	}

	outcome, err := runner.Run(context.Background(), dataset, "/data/slow.json", "")
	require.NoError(t, err)
	require.True(t, outcome.Duration >= 15*time.Millisecond)
}
