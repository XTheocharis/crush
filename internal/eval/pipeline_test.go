package eval

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubPipelineScorer struct {
	name  string
	sType ScorerType

	preprocessErr error
	analysis      string
	score         float64
	reason        string
}

func (s *stubPipelineScorer) Name() string     { return s.name }
func (s *stubPipelineScorer) Type() ScorerType { return s.sType }
func (s *stubPipelineScorer) Score(_ context.Context, _ *EvalInput) (*ScoreResult, error) {
	return &ScoreResult{Score: s.score, Explanation: s.reason, Passed: s.score >= 0.5}, nil
}

func (s *stubPipelineScorer) Preprocess(_ context.Context, _ *EvalInput, pctx *PipelineContext) error {
	if s.preprocessErr != nil {
		return s.preprocessErr
	}
	pctx.Preprocessed = "preprocessed"
	return nil
}

func (s *stubPipelineScorer) Analyze(_ context.Context, pctx *PipelineContext) error {
	pctx.Analysis = s.analysis
	return nil
}

func (s *stubPipelineScorer) GenerateScore(_ context.Context, pctx *PipelineContext) (float64, error) {
	return s.score, nil
}

func (s *stubPipelineScorer) GenerateReason(_ context.Context, pctx *PipelineContext) (string, error) {
	return s.reason, nil
}

type stubMetricPipeline struct {
	name   string
	score  float64
	reason string
}

func (s *stubMetricPipeline) Name() string     { return s.name }
func (s *stubMetricPipeline) Type() ScorerType { return ScorerMetric }
func (s *stubMetricPipeline) Score(_ context.Context, _ *EvalInput) (*ScoreResult, error) {
	return &ScoreResult{Score: s.score, Explanation: s.reason, Passed: s.score >= 0.7}, nil
}

func (s *stubMetricPipeline) Preprocess(_ context.Context, _ *EvalInput, _ *PipelineContext) error {
	return nil
}

func (s *stubMetricPipeline) Analyze(_ context.Context, _ *PipelineContext) error {
	return nil
}

func (s *stubMetricPipeline) GenerateScore(_ context.Context, _ *PipelineContext) (float64, error) {
	return s.score, nil
}

func (s *stubMetricPipeline) GenerateReason(_ context.Context, _ *PipelineContext) (string, error) {
	return s.reason, nil
}

func TestPipelineScorerScore(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubPipelineScorer{
		name:     "test_pipeline",
		sType:    ScorerMastra,
		analysis: "analyzed",
		score:    0.85,
		reason:   "Highly relevant",
	})
	result, err := ps.Score(context.Background(), &EvalInput{SessionID: "p1"})
	require.NoError(t, err)
	require.Equal(t, 0.85, result.Score)
	require.Equal(t, "Highly relevant", result.Explanation)
	require.True(t, result.Passed)
}

func TestPipelineScorerNameAndType(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubPipelineScorer{
		name:  "my_scorer",
		sType: ScorerLLMJudge,
	})
	require.Equal(t, "my_scorer", ps.Name())
	require.Equal(t, ScorerLLMJudge, ps.Type())
}

func TestPipelineScorerPreprocessError(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubPipelineScorer{
		name:          "bad_preprocess",
		preprocessErr: fmt.Errorf("boom"),
	})
	result, err := ps.Score(context.Background(), &EvalInput{})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "preprocess")
}

func TestPipelineScoreClamped(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubPipelineScorer{
		name:  "overscore",
		score: 2.0,
	})
	result, err := ps.Score(context.Background(), &EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
}

func TestPipelineScoreClampedNegative(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubPipelineScorer{
		name:  "underscore",
		score: -1.0,
	})
	result, err := ps.Score(context.Background(), &EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
}

func TestAsPipelineDirect(t *testing.T) {
	t.Parallel()
	inner := &stubPipelineScorer{name: "direct", sType: ScorerMastra}
	require.NotNil(t, AsPipeline(inner))
	require.Equal(t, "direct", AsPipeline(inner).Name())
}

func TestAsPipelineWrapped(t *testing.T) {
	t.Parallel()
	inner := &stubPipelineScorer{name: "wrapped", sType: ScorerMastra}
	wrapped := NewPipelineScorer(inner)
	require.NotNil(t, AsPipeline(wrapped))
	require.Equal(t, "wrapped", AsPipeline(wrapped).Name())
}

func TestAsPipelinePlainScorer(t *testing.T) {
	t.Parallel()
	plain := &stubScorer{name: "plain", sType: ScorerMetric}
	require.Nil(t, AsPipeline(plain))
}

func TestMetricSkipsAnalyze(t *testing.T) {
	t.Parallel()
	ps := NewPipelineScorer(&stubMetricPipeline{
		name:   "metric_skip",
		score:  0.9,
		reason: "All good",
	})
	result, err := ps.Score(context.Background(), &EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.9, result.Score)
	require.Equal(t, "All good", result.Explanation)
}

func TestHarnessAutoWrapsPipeline(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	inner := &stubPipelineScorer{
		name:   "auto_wrapped",
		sType:  ScorerMastra,
		score:  0.75,
		reason: "Auto-wrapped pipeline scorer",
	}
	h.Register(inner)

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "wrap_test"})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	require.Equal(t, 0.75, report.Results[0].Result.Score)
	require.Equal(t, "Auto-wrapped pipeline scorer", report.Results[0].Result.Explanation)
	require.True(t, report.Passed)
}

func TestHarnessRegistersPipelineScorerDirectly(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	inner := &stubPipelineScorer{
		name:   "pre_wrapped",
		sType:  ScorerLLMJudge,
		score:  0.65,
		reason: "Pre-wrapped",
	}
	h.Register(NewPipelineScorer(inner))

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "pre_wrap"})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	require.Equal(t, 0.65, report.Results[0].Result.Score)
}

func TestHarnessPlainScorerStillWorks(t *testing.T) {
	t.Parallel()
	h := NewEvalHarness()
	h.Register(&stubScorer{name: "plain", sType: ScorerMetric, score: 0.9, passed: true})

	report, err := h.Run(context.Background(), &EvalInput{SessionID: "plain"})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	require.Equal(t, 0.9, report.Results[0].Result.Score)
}
