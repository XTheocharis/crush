package metric

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/eval"
)

// TestPassRateScorer scores based on the percentage of tests passing.
type TestPassRateScorer struct {
	PassThreshold float64
}

// NewTestPassRateScorer creates a scorer that passes when the test pass rate
// meets or exceeds the threshold (0.0-1.0).
func NewTestPassRateScorer(threshold float64) *TestPassRateScorer {
	return &TestPassRateScorer{PassThreshold: threshold}
}

func (s *TestPassRateScorer) Name() string          { return "test_pass_rate" }
func (s *TestPassRateScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *TestPassRateScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if input.TestResults == nil || input.TestResults.Total == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No test results available, defaulting to perfect score.",
			Passed:      true,
			Details:     map[string]any{"total": 0, "passed": 0, "rate": 1.0},
		}, nil
	}

	rate := float64(input.TestResults.Passed) / float64(input.TestResults.Total)
	passed := rate >= s.PassThreshold
	explanation := fmt.Sprintf(
		"%d/%d tests passed (%.1f%%)",
		input.TestResults.Passed, input.TestResults.Total, rate*100,
	)

	return &eval.ScoreResult{
		Score:       rate,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"total":  input.TestResults.Total,
			"passed": input.TestResults.Passed,
			"failed": input.TestResults.Failed,
			"rate":   rate,
		},
	}, nil
}
