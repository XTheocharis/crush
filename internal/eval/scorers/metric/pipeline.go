package metric

import (
	"context"

	"github.com/charmbracelet/crush/internal/eval"
)

// MetricPipelineAdapter wraps any metric Scorer to implement the
// eval.ScorerPipeline interface. Preprocess stores the input, Analyze is a
// no-op, GenerateScore delegates to the underlying Score method, and
// GenerateReason returns the explanation from the generated result.
type MetricPipelineAdapter struct {
	eval.Scorer
	result *eval.ScoreResult
}

// NewMetricPipeline wraps a metric Scorer as a ScorerPipeline.
func NewMetricPipeline(s eval.Scorer) eval.ScorerPipeline {
	return &MetricPipelineAdapter{Scorer: s}
}

func (m *MetricPipelineAdapter) Preprocess(_ context.Context, _ *eval.EvalInput, _ *eval.PipelineContext) error {
	return nil
}

func (m *MetricPipelineAdapter) Analyze(_ context.Context, _ *eval.PipelineContext) error {
	return nil
}

func (m *MetricPipelineAdapter) GenerateScore(ctx context.Context, pctx *eval.PipelineContext) (float64, error) {
	result, err := m.Score(ctx, pctx.Input)
	if err != nil {
		return 0, err
	}
	m.result = result
	return result.Score, nil
}

func (m *MetricPipelineAdapter) GenerateReason(_ context.Context, _ *eval.PipelineContext) (string, error) {
	if m.result == nil {
		return "", nil
	}
	return m.result.Explanation, nil
}
