package metric

import (
	"context"
	"fmt"
	"math"

	"github.com/charmbracelet/crush/internal/eval"
)

// TypeCheckScoreScorer scores based on type-check error count.
// Zero errors yields 1.0. Errors are read from EvalInput.Details["type_errors"].
type TypeCheckScoreScorer struct {
	MaxErrors     int
	PassThreshold float64
}

// NewTypeCheckScoreScorer creates a scorer that normalizes against maxErrors
// and passes when the score meets the threshold (0.0-1.0).
func NewTypeCheckScoreScorer(maxErrors int, threshold float64) *TypeCheckScoreScorer {
	return &TypeCheckScoreScorer{MaxErrors: maxErrors, PassThreshold: threshold}
}

func (s *TypeCheckScoreScorer) Name() string          { return "typecheck_score" }
func (s *TypeCheckScoreScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *TypeCheckScoreScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	errors := 0
	for _, content := range input.Files {
		errors += countTypeErrors(content)
	}

	if s.MaxErrors <= 0 {
		score := 1.0
		if errors > 0 {
			score = 0.0
		}
		explanation := fmt.Sprintf("%d type errors", errors)
		return &eval.ScoreResult{
			Score:       score,
			Explanation: explanation,
			Passed:      errors == 0,
			Details:     map[string]any{"type_errors": errors},
		}, nil
	}

	score := math.Max(0, 1.0-float64(errors)/float64(s.MaxErrors))
	passed := score >= s.PassThreshold
	explanation := fmt.Sprintf("%d type errors (score %.2f)", errors, score)

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details:     map[string]any{"type_errors": errors, "max_errors": s.MaxErrors},
	}, nil
}

// countTypeErrors counts heuristic type-error indicators in source content.
func countTypeErrors(content string) int {
	count := 0
	prefix := "cannot use "
	for i := 0; i+len(prefix) <= len(content); i++ {
		if content[i:i+len(prefix)] == prefix {
			count++
			i += len(prefix) - 1
		}
	}
	return count
}
