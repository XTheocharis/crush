package eval

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubScorer struct {
	name    string
	sType   ScorerType
	score   float64
	passed  bool
	explain string
	err     error
	delay   time.Duration
}

func (s *stubScorer) Name() string     { return s.name }
func (s *stubScorer) Type() ScorerType { return s.sType }
func (s *stubScorer) Score(_ context.Context, _ *EvalInput) (*ScoreResult, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.err != nil {
		return nil, s.err
	}
	return &ScoreResult{
		Score:       s.score,
		Passed:      s.passed,
		Explanation: s.explain,
		Details:     map[string]any{"stub": true},
	}, nil
}

func TestScorerTypeString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "metric", ScorerMetric.String())
	require.Equal(t, "llm_judge", ScorerLLMJudge.String())
}

func TestNewEvalHarnessEmpty(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	require.Empty(t, h.Scorers())
}

func TestRegisterScorer(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge})
	require.Equal(t, []string{"accuracy", "safety"}, h.Scorers())
}

func TestRegisterDuplicatePanics(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "dup", sType: ScorerMetric})
	require.Panics(t, func() {
		h.Register(&stubScorer{name: "dup", sType: ScorerLLMJudge})
	})
}

func TestRunNoScorers(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	input := &EvalInput{SessionID: "sess_1"}
	report, err := h.Run(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.Equal(t, "sess_1", report.SessionID)
	require.True(t, report.Passed)
	require.Equal(t, 0.0, report.OverallScore)
	require.Empty(t, report.Results)
}

func TestRunSingleScorer(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{
		name:    "accuracy",
		sType:   ScorerMetric,
		score:   0.85,
		passed:  true,
		explain: "85% accuracy",
	})

	input := &EvalInput{SessionID: "sess_1"}
	report, err := h.Run(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 0.85, report.OverallScore)
	require.True(t, report.Passed)
	require.Len(t, report.Results, 1)
	require.Equal(t, "accuracy", report.Results[0].Name)
	require.Equal(t, ScorerMetric, report.Results[0].Type)
	require.Equal(t, 0.85, report.Results[0].Result.Score)
	require.Equal(t, "85% accuracy", report.Results[0].Result.Explanation)
	require.True(t, report.Results[0].Result.Passed)
	require.Equal(t, "accuracy", report.Results[0].Result.ScorerName)
	require.True(t, report.Results[0].Result.Duration > 0)
}

func TestRunMultipleScorers(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "safety", sType: ScorerLLMJudge, score: 0.8, passed: true})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_2"})
	require.NoError(t, err)
	require.Len(t, report.Results, 2)
	require.InDelta(t, 0.85, report.OverallScore, 0.001)
	require.True(t, report.Passed)
}

func TestRunScorerFailure(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "accuracy", sType: ScorerMetric, score: 0.9, passed: true})
	h.Register(&stubScorer{name: "broken", sType: ScorerMetric, err: errors.New("timeout")})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_3"})
	require.NoError(t, err)
	require.False(t, report.Passed)
	require.Len(t, report.Results, 2)

	require.Equal(t, "accuracy", report.Results[0].Name)
	require.True(t, report.Results[0].Result.Passed)

	require.Equal(t, "broken", report.Results[1].Name)
	require.Equal(t, "timeout", report.Results[1].Result.Error)
	require.False(t, report.Results[1].Result.Passed)
}

func TestRunOverallScoreSkipsErrors(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "good", sType: ScorerMetric, score: 1.0, passed: true})
	h.Register(&stubScorer{name: "bad", sType: ScorerMetric, err: errors.New("fail")})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_4"})
	require.NoError(t, err)
	require.Equal(t, 1.0, report.OverallScore)
	require.False(t, report.Passed)
}

func TestRunRespectsScorerErrors(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "slow", sType: ScorerMetric, score: 0.5, passed: true, delay: 10 * time.Millisecond})
	h.Register(&stubScorer{name: "fail", sType: ScorerMetric, err: errors.New("scorer exploded")})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_5"})
	require.NoError(t, err)
	require.Len(t, report.Results, 2)
	require.Equal(t, "scorer exploded", report.Results[1].Result.Error)
	require.False(t, report.Passed)
}

func TestRunParallelExecution(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "a", sType: ScorerMetric, score: 0.9, passed: true, delay: 50 * time.Millisecond})
	h.Register(&stubScorer{name: "b", sType: ScorerMetric, score: 0.8, passed: true, delay: 50 * time.Millisecond})
	h.Register(&stubScorer{name: "c", sType: ScorerMetric, score: 0.7, passed: true, delay: 50 * time.Millisecond})

	start := time.Now()
	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_6"})
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Len(t, report.Results, 3)
	require.True(t, elapsed < 200*time.Millisecond, "scorers should run in parallel, took %v", elapsed)
}

func TestRunAllScorersFail(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "a", sType: ScorerMetric, score: 0.3, passed: false})
	h.Register(&stubScorer{name: "b", sType: ScorerLLMJudge, score: 0.2, passed: false})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_7"})
	require.NoError(t, err)
	require.False(t, report.Passed)
	require.Len(t, report.Results, 2)
}

func TestRunPreservesResultOrder(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "z-scorer", sType: ScorerMetric, score: 0.9, passed: true, delay: 10 * time.Millisecond})
	h.Register(&stubScorer{name: "a-scorer", sType: ScorerMetric, score: 0.8, passed: true, delay: 30 * time.Millisecond})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_8"})
	require.NoError(t, err)
	require.Equal(t, "z-scorer", report.Results[0].Name)
	require.Equal(t, "a-scorer", report.Results[1].Name)
}

func TestScoreResultDetails(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "detail", sType: ScorerMetric, score: 1.0, passed: true})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_9"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"stub": true}, report.Results[0].Result.Details)
}

func TestEvalInputFields(t *testing.T) {
	t.Parallel()
	input := &EvalInput{
		SessionID: "sess_10",
		Conversation: []Message{
			{Role: "user", Content: "fix the bug"},
			{Role: "assistant", Content: "I fixed it"},
		},
		Edits: []FileEdit{
			{Path: "main.go", Before: "old", After: "new"},
		},
		TestResults: &TestResult{Total: 5, Passed: 4, Failed: 1, Output: "FAIL"},
		Files:       map[string]string{"main.go": "package main"},
	}

	require.Equal(t, "sess_10", input.SessionID)
	require.Len(t, input.Conversation, 2)
	require.Len(t, input.Edits, 1)
	require.Equal(t, "main.go", input.Edits[0].Path)
	require.NotNil(t, input.TestResults)
	require.Equal(t, 1, input.TestResults.Failed)
	require.Equal(t, "package main", input.Files["main.go"])
}

func TestAggregateScore(t *testing.T) {
	t.Parallel()
	report := &EvalReport{
		Results: []ScorerResultEntry{
			{Name: "a", Result: &ScoreResult{Score: 0.8, Passed: true}},
			{Name: "b", Result: &ScoreResult{Score: 0.6, Passed: true}},
		},
	}
	require.InDelta(t, 0.7, AggregateScore(report), 0.001)
}

func TestAggregateScoreNil(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0.0, AggregateScore(nil))
	require.Equal(t, 0.0, AggregateScore(&EvalReport{}))
}

func TestAggregateScoreSkipsErrors(t *testing.T) {
	t.Parallel()
	report := &EvalReport{
		Results: []ScorerResultEntry{
			{Name: "a", Result: &ScoreResult{Score: 0.9, Passed: true}},
			{Name: "b", Result: &ScoreResult{Score: 0.0, Passed: false, Error: "crashed"}},
		},
	}
	require.Equal(t, 0.9, AggregateScore(report))
}

func TestPassedScorers(t *testing.T) {
	t.Parallel()
	report := &EvalReport{
		Results: []ScorerResultEntry{
			{Name: "accuracy", Result: &ScoreResult{Passed: true}},
			{Name: "safety", Result: &ScoreResult{Passed: false}},
			{Name: "style", Result: &ScoreResult{Passed: true}},
		},
	}
	require.Equal(t, []string{"accuracy", "style"}, PassedScorers(report))
}

func TestFailedScorers(t *testing.T) {
	t.Parallel()
	report := &EvalReport{
		Results: []ScorerResultEntry{
			{Name: "accuracy", Result: &ScoreResult{Passed: true}},
			{Name: "safety", Result: &ScoreResult{Passed: false}},
			{Name: "broken", Result: &ScoreResult{Passed: false, Error: "timeout"}},
		},
	}
	require.Equal(t, []string{"broken", "safety"}, FailedScorers(report))
}

func TestPassedScorersNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, PassedScorers(nil))
	require.Nil(t, FailedScorers(nil))
}

func TestConcurrentRegistration(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			h.Register(&stubScorer{
				name:  fmt.Sprintf("scorer_%d", idx),
				sType: ScorerMetric,
			})
		}(i)
	}
	wg.Wait()
	require.Len(t, h.Scorers(), 10)
}

func TestEvalReportTimestamp(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "ts", sType: ScorerMetric, score: 1.0, passed: true})

	before := time.Now()
	report, err := h.Run(context.Background(), &EvalInput{SessionID: "sess_ts"})
	require.NoError(t, err)
	require.True(t, report.Timestamp.After(before) || report.Timestamp.Equal(before))
	require.True(t, report.Duration > 0)
}
