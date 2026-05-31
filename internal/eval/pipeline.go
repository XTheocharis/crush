package eval

import (
	"context"
	"fmt"
	"math"
)

// PipelineContext carries state between pipeline stages.
type PipelineContext struct {
	Input        *EvalInput
	Preprocessed string
	Analysis     string
	Metadata     map[string]any
}

// ScorerPipeline is a 4-stage scoring interface: Preprocess → Analyze →
// GenerateScore → GenerateReason. Scorers implement this to decompose their
// logic into discrete steps. Implementations must also satisfy Scorer for
// Name/Type. Metric scorers can use no-op implementations for unneeded stages.
type ScorerPipeline interface {
	Scorer

	Preprocess(ctx context.Context, input *EvalInput, pctx *PipelineContext) error
	Analyze(ctx context.Context, pctx *PipelineContext) error
	GenerateScore(ctx context.Context, pctx *PipelineContext) (float64, error)
	GenerateReason(ctx context.Context, pctx *PipelineContext) (string, error)
}

// PipelineScorer adapts a ScorerPipeline to the flat Scorer interface by
// orchestrating the 4-stage pipeline and building a ScoreResult.
type PipelineScorer struct {
	impl ScorerPipeline
}

// NewPipelineScorer wraps a ScorerPipeline so it can be registered with
// EvalHarness.
func NewPipelineScorer(impl ScorerPipeline) *PipelineScorer {
	return &PipelineScorer{impl: impl}
}

func (ps *PipelineScorer) Name() string     { return ps.impl.Name() }
func (ps *PipelineScorer) Type() ScorerType { return ps.impl.Type() }
func (ps *PipelineScorer) Score(ctx context.Context, input *EvalInput) (*ScoreResult, error) {
	pctx := &PipelineContext{
		Input:    input,
		Metadata: make(map[string]any),
	}

	if err := ps.impl.Preprocess(ctx, input, pctx); err != nil {
		return nil, fmt.Errorf("pipeline %s preprocess: %w", ps.impl.Name(), err)
	}

	if err := ps.impl.Analyze(ctx, pctx); err != nil {
		return nil, fmt.Errorf("pipeline %s analyze: %w", ps.impl.Name(), err)
	}

	score, err := ps.impl.GenerateScore(ctx, pctx)
	if err != nil {
		return nil, fmt.Errorf("pipeline %s generate_score: %w", ps.impl.Name(), err)
	}
	score = math.Max(0, math.Min(1, score))

	reason, err := ps.impl.GenerateReason(ctx, pctx)
	if err != nil {
		return nil, fmt.Errorf("pipeline %s generate_reason: %w", ps.impl.Name(), err)
	}

	return &ScoreResult{
		Score:       score,
		Explanation: reason,
		Passed:      score >= 0.5,
		Details:     pctx.Metadata,
	}, nil
}

// AsPipeline unwraps a ScorerPipeline from a Scorer (direct or PipelineScorer
// wrapper). Returns nil if the scorer does not implement ScorerPipeline.
func AsPipeline(s Scorer) ScorerPipeline {
	if p, ok := s.(ScorerPipeline); ok {
		return p
	}
	if ps, ok := s.(*PipelineScorer); ok {
		return ps.impl
	}
	return nil
}
