package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupTestStorage(t *testing.T) (*ScorerStorage, context.Context) {
	t.Helper()
	t.Parallel()

	ctx := context.Background()
	db, err := OpenTestDB(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return NewScorerStorage(db), ctx
}

func TestScorerStorage_SaveAndGetResult(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entry := ScorerResultEntry{
		Name: "accuracy",
		Type: ScorerMetric,
		Result: &ScoreResult{
			Score:       0.92,
			Passed:      true,
			Explanation: "High accuracy",
			Details:     map[string]any{"f1": 0.91},
			Duration:    DurationFromMs(150),
			ScorerName:  "accuracy",
		},
	}

	inputHash := "abc123"
	err := storage.SaveResult(ctx, "run_1", entry, inputHash)
	require.NoError(t, err)

	results, err := storage.GetResultsByRun(ctx, "run_1")
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	require.Equal(t, "accuracy", r.ScorerName)
	require.Equal(t, "metric", r.ScorerType)
	require.InDelta(t, 0.92, r.Score, 0.001)
	require.True(t, r.Passed)
	require.Equal(t, "High accuracy", r.Explanation)
	require.Equal(t, inputHash, r.InputHash)
	require.Equal(t, int64(150), r.DurationMs)
	require.Equal(t, "", r.ErrorMsg)
}

func TestScorerStorage_SaveResultWithError(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entry := ScorerResultEntry{
		Name: "broken",
		Type: ScorerMetric,
		Result: &ScoreResult{
			Score:      0.0,
			Passed:     false,
			Error:      "timeout after 30s",
			Duration:   DurationFromMs(30000),
			ScorerName: "broken",
		},
	}

	err := storage.SaveResult(ctx, "run_err", entry, "hash_err")
	require.NoError(t, err)

	results, err := storage.GetResultsByRun(ctx, "run_err")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "timeout after 30s", results[0].ErrorMsg)
	require.False(t, results[0].Passed)
}

func TestScorerStorage_SaveResultNilResult(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entry := ScorerResultEntry{Name: "nil_scorer", Type: ScorerMetric, Result: nil}
	err := storage.SaveResult(ctx, "run_nil", entry, "hash_nil")
	require.NoError(t, err)

	results, err := storage.GetResultsByRun(ctx, "run_nil")
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestScorerStorage_SaveReport(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	report := &EvalReport{
		SessionID: "run_report",
		Results: []ScorerResultEntry{
			{
				Name: "accuracy",
				Type: ScorerMetric,
				Result: &ScoreResult{
					Score:       0.85,
					Passed:      true,
					Explanation: "Good accuracy",
					ScorerName:  "accuracy",
				},
			},
			{
				Name: "safety",
				Type: ScorerLLMJudge,
				Result: &ScoreResult{
					Score:       0.6,
					Passed:      true,
					Explanation: "Acceptable safety",
					ScorerName:  "safety",
				},
			},
		},
	}

	err := storage.SaveReport(ctx, report, "hash_report")
	require.NoError(t, err)

	results, err := storage.GetResultsByRun(ctx, "run_report")
	require.NoError(t, err)
	require.Len(t, results, 2)
}

func TestScorerStorage_SaveReportNil(t *testing.T) {
	storage, ctx := setupTestStorage(t)
	err := storage.SaveReport(ctx, nil, "hash")
	require.NoError(t, err)
}

func TestScorerStorage_GetResultsByRunEmpty(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	results, err := storage.GetResultsByRun(ctx, "nonexistent")
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestScorerStorage_GetResultsByScorer(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entry1 := ScorerResultEntry{
		Name: "accuracy", Type: ScorerMetric,
		Result: &ScoreResult{Score: 0.9, Passed: true, ScorerName: "accuracy"},
	}
	entry2 := ScorerResultEntry{
		Name: "accuracy", Type: ScorerMetric,
		Result: &ScoreResult{Score: 0.8, Passed: true, ScorerName: "accuracy"},
	}

	require.NoError(t, storage.SaveResult(ctx, "run_a", entry1, "h1"))
	require.NoError(t, storage.SaveResult(ctx, "run_b", entry2, "h2"))

	results, err := storage.GetResultsByScorer(ctx, "accuracy")
	require.NoError(t, err)
	require.Len(t, results, 2)
}

func TestScorerStorage_SaveAndGetRun(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	run := &StoredEvalRun{
		RunID:         "run_meta_1",
		DatasetPath:   "/data/test.json",
		ScorerFilter:  "accuracy",
		TotalExamples: 10,
		OverallScore:  0.87,
		OverallPassed: true,
		DurationMs:    5000,
	}

	err := storage.SaveRun(ctx, run)
	require.NoError(t, err)

	got, err := storage.GetRun(ctx, "run_meta_1")
	require.NoError(t, err)
	require.Equal(t, "run_meta_1", got.RunID)
	require.Equal(t, "/data/test.json", got.DatasetPath)
	require.Equal(t, "accuracy", got.ScorerFilter)
	require.Equal(t, 10, got.TotalExamples)
	require.InDelta(t, 0.87, got.OverallScore, 0.001)
	require.True(t, got.OverallPassed)
	require.Equal(t, int64(5000), got.DurationMs)
}

func TestScorerStorage_SaveRunGeneratesID(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	run := &StoredEvalRun{
		DatasetPath:   "/data/test.json",
		TotalExamples: 5,
	}

	err := storage.SaveRun(ctx, run)
	require.NoError(t, err)
	require.NotEmpty(t, run.RunID)
}

func TestScorerStorage_GetRunNotFound(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	_, err := storage.GetRun(ctx, "nonexistent")
	require.Error(t, err)
}

func TestScorerStorage_ListRuns(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	run1 := &StoredEvalRun{RunID: "list_1", DatasetPath: "a.json", TotalExamples: 3, OverallScore: 0.9, OverallPassed: true}
	run2 := &StoredEvalRun{RunID: "list_2", DatasetPath: "b.json", TotalExamples: 5, OverallScore: 0.7, OverallPassed: false}

	require.NoError(t, storage.SaveRun(ctx, run1))
	require.NoError(t, storage.SaveRun(ctx, run2))

	runs, err := storage.ListRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	ids := make(map[string]bool, len(runs))
	for _, r := range runs {
		ids[r.RunID] = true
	}
	require.True(t, ids["list_1"])
	require.True(t, ids["list_2"])
}

func TestScorerStorage_ListRunsEmpty(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	runs, err := storage.ListRuns(ctx)
	require.NoError(t, err)
	require.Empty(t, runs)
}

func TestScorerStorage_CompareRuns(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entry1 := ScorerResultEntry{
		Name: "accuracy", Type: ScorerMetric,
		Result: &ScoreResult{Score: 0.9, Passed: true, ScorerName: "accuracy"},
	}
	entry2 := ScorerResultEntry{
		Name: "accuracy", Type: ScorerMetric,
		Result: &ScoreResult{Score: 0.7, Passed: true, ScorerName: "accuracy"},
	}

	require.NoError(t, storage.SaveResult(ctx, "cmp_1", entry1, "h1"))
	require.NoError(t, storage.SaveResult(ctx, "cmp_2", entry2, "h2"))

	comparison, err := storage.CompareRuns(ctx, "cmp_1", "cmp_2")
	require.NoError(t, err)
	require.Len(t, comparison["cmp_1"], 1)
	require.Len(t, comparison["cmp_2"], 1)
	require.InDelta(t, 0.9, comparison["cmp_1"][0].Score, 0.001)
	require.InDelta(t, 0.7, comparison["cmp_2"][0].Score, 0.001)
}

func TestHashInput(t *testing.T) {
	t.Parallel()

	input1 := &EvalInput{SessionID: "s1", Files: map[string]string{"a.go": "package main"}}
	input2 := &EvalInput{SessionID: "s1", Files: map[string]string{"a.go": "package main"}}
	input3 := &EvalInput{SessionID: "s2", Files: map[string]string{"a.go": "package main"}}

	h1 := HashInput(input1)
	h2 := HashInput(input2)
	h3 := HashInput(input3)

	require.NotEmpty(t, h1)
	require.Equal(t, h1, h2)
	require.NotEqual(t, h1, h3)
}

func TestScorerStorage_MultipleResultsPerRun(t *testing.T) {
	storage, ctx := setupTestStorage(t)

	entries := []ScorerResultEntry{
		{Name: "a", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true, ScorerName: "a"}},
		{Name: "b", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.8, Passed: true, ScorerName: "b"}},
		{Name: "c", Type: ScorerMetric, Result: &ScoreResult{Score: 0.5, Passed: false, ScorerName: "c"}},
	}

	for _, e := range entries {
		require.NoError(t, storage.SaveResult(ctx, "multi_run", e, "hash_multi"))
	}

	results, err := storage.GetResultsByRun(ctx, "multi_run")
	require.NoError(t, err)
	require.Len(t, results, 3)
}
