package metric

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/eval"
)

// SyntaxValidityScorer scores based on bracket balance in Files.
// Perfectly balanced brackets yield 1.0.
type SyntaxValidityScorer struct {
	PassThreshold float64
}

// NewSyntaxValidityScorer creates a scorer that passes when the syntax
// validity meets the threshold (0.0-1.0).
func NewSyntaxValidityScorer(threshold float64) *SyntaxValidityScorer {
	return &SyntaxValidityScorer{PassThreshold: threshold}
}

func (s *SyntaxValidityScorer) Name() string          { return "syntax_validity" }
func (s *SyntaxValidityScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *SyntaxValidityScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Files) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No files to validate.",
			Passed:      true,
			Details:     map[string]any{"files": 0, "balanced": true},
		}, nil
	}

	balanced := 0
	unbalanced := 0
	for _, content := range input.Files {
		if isBracketBalanced(content) {
			balanced++
		} else {
			unbalanced++
		}
	}

	total := balanced + unbalanced
	score := float64(balanced) / float64(total)
	passed := score >= s.PassThreshold
	explanation := fmt.Sprintf(
		"%d/%d files have balanced brackets",
		balanced, total,
	)

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"files":      total,
			"balanced":   balanced,
			"unbalanced": unbalanced,
		},
	}, nil
}

// isBracketBalanced checks that parentheses, brackets, and braces are balanced.
// It ignores brackets inside string literals.
func isBracketBalanced(content string) bool {
	stack := make([]rune, 0, 32)
	inString := false
	var stringDelim rune

	for i, ch := range content {
		switch {
		case inString:
			if ch == '\\' && i+1 < len(content) {
				continue
			}
			if ch == stringDelim {
				inString = false
			}
		case ch == '"' || ch == '\'' || ch == '`':
			inString = true
			stringDelim = ch
		case ch == '(' || ch == '[' || ch == '{':
			stack = append(stack, ch)
		case ch == ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return false
			}
			stack = stack[:len(stack)-1]
		case ch == ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return false
			}
			stack = stack[:len(stack)-1]
		case ch == '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}

	return len(stack) == 0 && !inString
}
