package metric

import (
	"context"
	"fmt"
	"math"

	"github.com/charmbracelet/crush/internal/eval"
)

// LintScoreScorer scores based on lint warning count. Fewer warnings produce
// a higher score. Warnings are read from EvalInput.Details["lint_warnings"].
type LintScoreScorer struct {
	MaxWarnings   int
	PassThreshold float64
}

// NewLintScoreScorer creates a scorer that normalizes against maxWarnings and
// passes when the score meets the threshold (0.0-1.0).
func NewLintScoreScorer(maxWarnings int, threshold float64) *LintScoreScorer {
	return &LintScoreScorer{MaxWarnings: maxWarnings, PassThreshold: threshold}
}

func (s *LintScoreScorer) Name() string          { return "lint_score" }
func (s *LintScoreScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *LintScoreScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	warnings := 0
	for _, edit := range input.Edits {
		warnings += lintWarningsFromContent(edit.After)
	}

	if s.MaxWarnings <= 0 {
		return &eval.ScoreResult{
			Score:       0.0,
			Explanation: "MaxWarnings not configured.",
			Passed:      false,
			Details:     map[string]any{"warnings": warnings},
		}, nil
	}

	score := math.Max(0, 1.0-float64(warnings)/float64(s.MaxWarnings))
	passed := score >= s.PassThreshold
	explanation := fmt.Sprintf("%d lint warnings (score %.2f)", warnings, score)

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details:     map[string]any{"warnings": warnings, "max": s.MaxWarnings},
	}, nil
}

// lintWarningsFromContent counts heuristic lint indicators in source content.
func lintWarningsFromContent(content string) int {
	count := 0
	for i := 0; i < len(content); i++ {
		switch {
		case content[i] == '\t':
			count++
		case i+1 < len(content) && content[i] == ' ' && content[i+1] == ' ' &&
			(i+2 >= len(content) || content[i+2] != ' '):
			count++
		}
	}
	return count
}
