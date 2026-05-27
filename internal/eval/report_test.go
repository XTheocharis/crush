package eval

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReportApplyCriteriaAllPass(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-1",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: true}},
			{Name: "relevance", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.72, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.True(t, scored.OverallPassed)
	require.Equal(t, 2, scored.PassedCount)
	require.Equal(t, 0, scored.FailedCount)
	require.Equal(t, 2, scored.TotalScorers)

	require.Equal(t, 0.85, scored.ScorerScores[0].Score)
	require.True(t, scored.ScorerScores[0].Passed)
	require.Equal(t, 0.72, scored.ScorerScores[1].Score)
	require.True(t, scored.ScorerScores[1].Passed)
}

func TestReportApplyCriteriaMixedPassFail(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-2",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: true}},
			{Name: "coverage", Type: ScorerMetric, Result: &ScoreResult{Score: 0.45, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed)
	require.Equal(t, 1, scored.PassedCount)
	require.Equal(t, 1, scored.FailedCount)

	require.True(t, scored.ScorerScores[0].Passed)
	require.False(t, scored.ScorerScores[1].Passed)
}

func TestReportApplyCriteriaAllFail(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-3",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.3, Passed: false}},
			{Name: "coverage", Type: ScorerMetric, Result: &ScoreResult{Score: 0.4, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed)
	require.Equal(t, 0, scored.PassedCount)
	require.Equal(t, 2, scored.FailedCount)
}

func TestReportWeightedAggregation(t *testing.T) {
	t.Parallel()

	criteria := NewScoringCriteria()
	criteria.Set("heavy", Criterion{Threshold: 0.5, Weight: 3.0, ScorerType: ScorerMetric})
	criteria.Set("light", Criterion{Threshold: 0.5, Weight: 1.0, ScorerType: ScorerLLMJudge})

	rg := NewReportGenerator(criteria)
	report := &EvalReport{
		SessionID: "sess-4",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "heavy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.8, Passed: true}},
			{Name: "light", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.4, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.Equal(t, 0.7, scored.WeightedScore, "weighted: (0.8*3 + 0.4*1) / (3+1) = 0.7")
}

func TestReportWeightedAggregationEqualWeights(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-5",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "a", Type: ScorerMetric, Result: &ScoreResult{Score: 1.0, Passed: true}},
			{Name: "b", Type: ScorerMetric, Result: &ScoreResult{Score: 0.5, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.Equal(t, 0.75, scored.WeightedScore)
}

func TestReportThresholdChecking(t *testing.T) {
	t.Parallel()

	criteria := NewScoringCriteria()
	criteria.Set("strict", Criterion{Threshold: 0.9, Weight: 1.0, ScorerType: ScorerMetric})
	criteria.Set("lenient", Criterion{Threshold: 0.3, Weight: 1.0, ScorerType: ScorerMetric})

	rg := NewReportGenerator(criteria)
	report := &EvalReport{
		SessionID: "sess-6",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "strict", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: false}},
			{Name: "lenient", Type: ScorerMetric, Result: &ScoreResult{Score: 0.35, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed)

	for _, ss := range scored.ScorerScores {
		if ss.Name == "strict" {
			require.False(t, ss.Passed, "0.85 < 0.9 threshold")
			require.Equal(t, 0.9, ss.Threshold)
		}
		if ss.Name == "lenient" {
			require.True(t, ss.Passed, "0.35 >= 0.3 threshold")
			require.Equal(t, 0.3, ss.Threshold)
		}
	}
}

func TestReportDefaultThresholds(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-7",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "metric-scorer", Type: ScorerMetric, Result: &ScoreResult{Score: 0.69, Passed: false}},
			{Name: "llm-scorer", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.59, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed)

	for _, ss := range scored.ScorerScores {
		if ss.Name == "metric-scorer" {
			require.Equal(t, 0.7, ss.Threshold, "default metric threshold should be 0.7")
			require.False(t, ss.Passed, "0.69 < 0.7")
		}
		if ss.Name == "llm-scorer" {
			require.Equal(t, 0.6, ss.Threshold, "default LLM judge threshold should be 0.6")
			require.False(t, ss.Passed, "0.59 < 0.6")
		}
	}
}

func TestReportEdgeCaseJustAtThreshold(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-8",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "exact", Type: ScorerMetric, Result: &ScoreResult{Score: 0.7, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.True(t, scored.OverallPassed)
	require.True(t, scored.ScorerScores[0].Passed, "score exactly at threshold should pass")
}

func TestReportEmptyReport(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	scored := rg.ApplyCriteria(&EvalReport{})
	require.False(t, scored.OverallPassed)
	require.Empty(t, scored.ScorerScores)
	require.Equal(t, 0, scored.TotalScorers)
}

func TestReportNilReport(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	scored := rg.ApplyCriteria(nil)
	require.False(t, scored.OverallPassed)
	require.Empty(t, scored.ScorerScores)
}

func TestReportSingleScorer(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-9",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "only", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.True(t, scored.OverallPassed)
	require.Equal(t, 0.9, scored.WeightedScore)
	require.Equal(t, 1, scored.PassedCount)
	require.Equal(t, 0, scored.FailedCount)
}

func TestReportErrorEntries(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-10",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "good", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true}},
			{Name: "errored", Type: ScorerMetric, Result: &ScoreResult{Error: "timeout", Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed, "error entry should cause overall failure")
	require.Equal(t, 1, scored.PassedCount)
	require.Equal(t, 1, scored.FailedCount)

	for _, ss := range scored.ScorerScores {
		if ss.Name == "errored" {
			require.Equal(t, 0.0, ss.Score)
			require.Equal(t, "timeout", ss.Error)
			require.False(t, ss.Passed)
		}
	}
}

func TestReportNilResultEntry(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-11",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "nil-result", Type: ScorerMetric, Result: nil},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.False(t, scored.OverallPassed)
	require.Equal(t, 1, scored.FailedCount)

	for _, ss := range scored.ScorerScores {
		if ss.Name == "nil-result" {
			require.Equal(t, 0.0, ss.Score)
			require.Equal(t, "no result", ss.Error)
		}
	}
}

func TestReportToJSON(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-json",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	data, err := rg.ToJSON(scored)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var parsed ScoredReport
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Equal(t, "sess-json", parsed.SessionID)
	require.Equal(t, 1, parsed.TotalScorers)
	require.True(t, parsed.OverallPassed)
	require.Equal(t, 0.85, parsed.WeightedScore)
}

func TestReportToMarkdown(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-md",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: true}},
			{Name: "coverage", Type: ScorerMetric, Result: &ScoreResult{Score: 0.45, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	md := rg.ToMarkdown(scored)
	require.NotEmpty(t, md)
	require.Contains(t, md, "sess-md")
	require.Contains(t, md, "accuracy")
	require.Contains(t, md, "coverage")
	require.Contains(t, md, "0.850")
	require.Contains(t, md, "0.450")
	require.Contains(t, md, "FAIL")
}

func TestReportToMarkdownEmpty(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	md := rg.ToMarkdown(nil)
	require.Contains(t, md, "No scorer results")
}

func TestReportExecutiveSummary(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-summary",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.85, Passed: true}},
			{Name: "relevance", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.72, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	summary := rg.ExecutiveSummary(scored)
	require.Contains(t, summary, "PASSED")
	require.Contains(t, summary, "2/2")
}

func TestReportExecutiveSummaryFailed(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-summary-fail",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "accuracy", Type: ScorerMetric, Result: &ScoreResult{Score: 0.3, Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	summary := rg.ExecutiveSummary(scored)
	require.Contains(t, summary, "FAILED")
	require.Contains(t, summary, "0/1")
}

func TestReportExecutiveSummaryNil(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	summary := rg.ExecutiveSummary(nil)
	require.Contains(t, summary, "No scorers evaluated")
}

func TestReportScoresSortedByName(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-sorted",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "zebra", Type: ScorerMetric, Result: &ScoreResult{Score: 0.5, Passed: false}},
			{Name: "alpha", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true}},
			{Name: "middle", Type: ScorerLLMJudge, Result: &ScoreResult{Score: 0.7, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.Equal(t, "alpha", scored.ScorerScores[0].Name)
	require.Equal(t, "middle", scored.ScorerScores[1].Name)
	require.Equal(t, "zebra", scored.ScorerScores[2].Name)
}

func TestReportWeightedScoreWithErrorEntry(t *testing.T) {
	t.Parallel()

	criteria := NewScoringCriteria()
	criteria.Set("good", Criterion{Threshold: 0.5, Weight: 2.0, ScorerType: ScorerMetric})
	criteria.Set("bad", Criterion{Threshold: 0.5, Weight: 1.0, ScorerType: ScorerMetric})

	rg := NewReportGenerator(criteria)
	report := &EvalReport{
		SessionID: "sess-weighted-err",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "good", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true}},
			{Name: "bad", Type: ScorerMetric, Result: &ScoreResult{Error: "crashed", Passed: false}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.Equal(t, 0.6, scored.WeightedScore, "(0.9*2 + 0.0*1) / (2+1) = 0.6")
}

func TestReportDefaultCriteria(t *testing.T) {
	t.Parallel()

	scorers := []Scorer{
		&stubScorer{name: "metric1", sType: ScorerMetric},
		&stubScorer{name: "judge1", sType: ScorerLLMJudge},
	}

	criteria := DefaultCriteria(scorers)
	cr := criteria.Get("metric1", ScorerMetric)
	require.Equal(t, 0.7, cr.Threshold)
	require.Equal(t, 1.0, cr.Weight)

	cr = criteria.Get("judge1", ScorerLLMJudge)
	require.Equal(t, 0.6, cr.Threshold)
	require.Equal(t, 1.0, cr.Weight)
}

func TestReportToMarkdownPassIcon(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	report := &EvalReport{
		SessionID: "sess-icons",
		Timestamp: time.Now(),
		Results: []ScorerResultEntry{
			{Name: "passing", Type: ScorerMetric, Result: &ScoreResult{Score: 0.9, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	md := rg.ToMarkdown(scored)
	require.True(t, scored.OverallPassed)
	require.True(t, strings.Contains(md, "PASS"))
}

func TestReportTimestampFormatted(t *testing.T) {
	t.Parallel()

	rg := NewReportGenerator(nil)
	ts := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	report := &EvalReport{
		SessionID: "sess-ts",
		Timestamp: ts,
		Results: []ScorerResultEntry{
			{Name: "a", Type: ScorerMetric, Result: &ScoreResult{Score: 0.8, Passed: true}},
		},
	}

	scored := rg.ApplyCriteria(report)
	require.Contains(t, scored.Timestamp, "2025-03-15")
}
