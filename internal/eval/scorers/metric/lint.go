package metric

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/crush/internal/eval"
)

// LintScoreScorer analyzes file content for common lint issues: hard tabs,
// long lines, trailing whitespace, missing final newline, and mixed
// indentation. Fewer issues produce a higher score.
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
	allContent := make(map[string]string)
	for _, edit := range input.Edits {
		if len(edit.After) > 0 {
			allContent[edit.Path] = edit.After
		}
	}
	for path, content := range input.Files {
		allContent[path] = content
	}

	issues := 0
	issueBreakdown := make(map[string]int)
	for _, content := range allContent {
		fileIssues := lintContent(content)
		for category, count := range fileIssues {
			issues += count
			issueBreakdown[category] += count
		}
	}

	if s.MaxWarnings <= 0 {
		return &eval.ScoreResult{
			Score:       0.0,
			Explanation: "MaxWarnings not configured.",
			Passed:      false,
			Details:     map[string]any{"warnings": issues},
		}, nil
	}

	score := math.Max(0, 1.0-float64(issues)/float64(s.MaxWarnings))
	passed := score >= s.PassThreshold
	explanation := fmt.Sprintf("%d lint warnings across %d files (score %.2f)", issues, len(allContent), score)

	details := map[string]any{
		"warnings":  issues,
		"max":       s.MaxWarnings,
		"files":     len(allContent),
		"breakdown": issueBreakdown,
	}

	return &eval.ScoreResult{
		Score:       score,
		Explanation: explanation,
		Passed:      passed,
		Details:     details,
	}, nil
}

func lintContent(content string) map[string]int {
	issues := make(map[string]int)
	lines := strings.Split(content, "\n")

	prevIndentWasTabs := false
	prevIndentWasSpaces := false

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		leading := leadingWhitespace(line)

		if strings.Contains(leading, "\t") {
			issues["hard_tabs"]++
			prevIndentWasTabs = true
		} else if len(leading) > 0 {
			prevIndentWasSpaces = true
		}

		if prevIndentWasTabs && prevIndentWasSpaces {
			issues["mixed_indent"]++
		}

		lineLen := utf8.RuneCountInString(line)
		if lineLen > 120 {
			issues["long_lines"]++
		}

		if len(line) > 0 && line[len(line)-1] == ' ' {
			issues["trailing_whitespace"]++
		}
	}

	if len(content) > 0 && content[len(content)-1] != '\n' {
		issues["missing_newline"]++
	}

	return issues
}

func leadingWhitespace(line string) string {
	var b strings.Builder
	for _, r := range line {
		if r == ' ' || r == '\t' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}
