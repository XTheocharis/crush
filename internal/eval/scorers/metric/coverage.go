package metric

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/eval"
)

// CoverageScoreScorer scores based on test coverage percentage.
// Coverage is read from EvalInput.Details["coverage_pct"].
type CoverageScoreScorer struct {
	PassThreshold float64
}

// NewCoverageScoreScorer creates a scorer that passes when coverage meets the
// threshold (0.0-1.0).
func NewCoverageScoreScorer(threshold float64) *CoverageScoreScorer {
	return &CoverageScoreScorer{PassThreshold: threshold}
}

func (s *CoverageScoreScorer) Name() string          { return "coverage_score" }
func (s *CoverageScoreScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *CoverageScoreScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if input.TestResults == nil || input.TestResults.Total == 0 {
		return &eval.ScoreResult{
			Score:       0.0,
			Explanation: "No test results available to estimate coverage.",
			Passed:      false,
			Details:     map[string]any{"coverage_pct": 0.0},
		}, nil
	}

	coverage := float64(input.TestResults.Passed) / float64(input.TestResults.Total)
	passed := coverage >= s.PassThreshold
	explanation := fmt.Sprintf("Estimated coverage: %.1f%%", coverage*100)

	return &eval.ScoreResult{
		Score:       coverage,
		Explanation: explanation,
		Passed:      passed,
		Details:     map[string]any{"coverage_pct": coverage * 100},
	}, nil
}
