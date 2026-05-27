package metric

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/eval"
)

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "it": true, "that": true, "this": true, "was": true,
	"are": true, "be": true, "has": true, "had": true, "have": true,
	"will": true, "would": true, "could": true, "should": true,
	"not": true, "no": true, "do": true, "if": true, "as": true,
	"can": true, "its": true, "been": true, "also": true,
}

type KeywordCoverageScorer struct {
	PassThreshold float64
}

func NewKeywordCoverageScorer(threshold float64) *KeywordCoverageScorer {
	return &KeywordCoverageScorer{PassThreshold: threshold}
}

func (s *KeywordCoverageScorer) Name() string          { return "KeywordCoverage" }
func (s *KeywordCoverageScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *KeywordCoverageScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Edits) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No edits to evaluate.",
			Passed:      true,
			Details:     map[string]any{"keywords": 0, "matched": 0, "coverage": 1.0},
		}, nil
	}

	var expectedText, actualText strings.Builder
	for _, edit := range input.Edits {
		expectedText.WriteString(edit.Before)
		expectedText.WriteString(" ")
		actualText.WriteString(edit.After)
		actualText.WriteString(" ")
	}

	keywords := extractKeywords(expectedText.String())
	if len(keywords) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No keywords found in reference text.",
			Passed:      true,
			Details:     map[string]any{"keywords": 0, "matched": 0, "coverage": 1.0},
		}, nil
	}

	actualTokens := tokenSet(actualText.String())
	matched := 0
	for _, kw := range keywords {
		if actualTokens[kw] {
			matched++
		}
	}

	coverage := float64(matched) / float64(len(keywords))
	passed := coverage >= s.PassThreshold
	explanation := fmt.Sprintf("Keyword coverage: %d/%d (%.1f%%)",
		matched, len(keywords), coverage*100)

	return &eval.ScoreResult{
		Score:       coverage,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"keywords": len(keywords),
			"matched":  matched,
			"coverage": coverage,
		},
	}, nil
}

func extractKeywords(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool)
	var keywords []string
	for _, f := range fields {
		cleaned := strings.Trim(f, ".,;:!?()[]{}\"'")
		if cleaned != "" && !stopwords[cleaned] && !seen[cleaned] {
			seen[cleaned] = true
			keywords = append(keywords, cleaned)
		}
	}
	return keywords
}

func tokenSet(text string) map[string]bool {
	fields := strings.Fields(strings.ToLower(text))
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		cleaned := strings.Trim(f, ".,;:!?()[]{}\"'")
		if cleaned != "" {
			set[cleaned] = true
		}
	}
	return set
}
