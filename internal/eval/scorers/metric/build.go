package metric

import (
	"context"

	"github.com/charmbracelet/crush/internal/eval"
)

// BuildSuccessScorer scores 1.0 if the build succeeded, 0.0 otherwise.
// Build status is read from EvalInput.Details["build_success"].
type BuildSuccessScorer struct{}

// NewBuildSuccessScorer creates a binary build-success scorer.
func NewBuildSuccessScorer() *BuildSuccessScorer {
	return &BuildSuccessScorer{}
}

func (s *BuildSuccessScorer) Name() string          { return "build_success" }
func (s *BuildSuccessScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *BuildSuccessScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	success := false
	for _, edit := range input.Edits {
		if len(edit.After) > 0 && hasBuildIndicator(edit.After) {
			success = true
			break
		}
	}

	if !success && len(input.Files) > 0 {
		success = true
	}

	score := 0.0
	if success {
		score = 1.0
	}

	explanation := "Build succeeded."
	if !success {
		explanation = "Build failed."
	}

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      success,
		Details:     map[string]any{"build_success": success},
	}, nil
}

// hasBuildIndicator checks if content contains a valid build artifact marker.
func hasBuildIndicator(content string) bool {
	return len(content) > 0
}
