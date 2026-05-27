package metric

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/eval"
)

type ContentSimilarityScorer struct {
	PassThreshold float64
}

func NewContentSimilarityScorer(threshold float64) *ContentSimilarityScorer {
	return &ContentSimilarityScorer{PassThreshold: threshold}
}

func (s *ContentSimilarityScorer) Name() string          { return "ContentSimilarity" }
func (s *ContentSimilarityScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *ContentSimilarityScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Edits) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No edits to evaluate.",
			Passed:      true,
			Details:     map[string]any{"jaccard": 1.0},
		}, nil
	}

	var expectedText, actualText string
	for _, edit := range input.Edits {
		expectedText += edit.Before + " "
		actualText += edit.After + " "
	}

	setA := tokenSet(expectedText)
	setB := tokenSet(actualText)

	if len(setA) == 0 && len(setB) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "Both texts are empty.",
			Passed:      true,
			Details:     map[string]any{"jaccard": 1.0, "intersection": 0, "union": 0},
		}, nil
	}

	intersection := 0
	for token := range setA {
		if setB[token] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	jaccard := float64(intersection) / float64(union)
	passed := jaccard >= s.PassThreshold
	explanation := fmt.Sprintf("Jaccard similarity: %d/%d (%.1f%%)",
		intersection, union, jaccard*100)

	return &eval.ScoreResult{
		Score:       jaccard,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"jaccard":      jaccard,
			"intersection": intersection,
			"union":        union,
		},
	}, nil
}
