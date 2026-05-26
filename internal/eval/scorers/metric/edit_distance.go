package metric

import (
	"context"
	"fmt"
	"math"

	"github.com/charmbracelet/crush/internal/eval"
)

// EditDistanceScorer scores based on the normalized Levenshtein distance
// between FileEdit.Before and After for each edit. An exact match scores 1.0.
type EditDistanceScorer struct {
	ScorerName    string
	PassThreshold float64
}

// NewEditDistanceScorer creates a scorer that passes when the normalized
// similarity meets the threshold (0.0-1.0).
func NewEditDistanceScorer(threshold float64) *EditDistanceScorer {
	return &EditDistanceScorer{ScorerName: "edit_distance", PassThreshold: threshold}
}

// NewNamedEditDistanceScorer creates an edit distance scorer with a custom name.
func NewNamedEditDistanceScorer(name string, threshold float64) *EditDistanceScorer {
	return &EditDistanceScorer{ScorerName: name, PassThreshold: threshold}
}

func (s *EditDistanceScorer) Name() string          { return s.ScorerName }
func (s *EditDistanceScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *EditDistanceScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Edits) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No edits to evaluate.",
			Passed:      true,
			Details:     map[string]any{"edits": 0, "avg_similarity": 1.0},
		}, nil
	}

	totalSimilarity := 0.0
	for _, edit := range input.Edits {
		totalSimilarity += normalizedSimilarity(edit.Before, edit.After)
	}

	avgSimilarity := totalSimilarity / float64(len(input.Edits))
	passed := avgSimilarity >= s.PassThreshold
	explanation := fmt.Sprintf(
		"Average similarity across %d edits: %.2f",
		len(input.Edits), avgSimilarity,
	)

	return &eval.ScoreResult{
		Score:       avgSimilarity,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"edits":          len(input.Edits),
			"avg_similarity": avgSimilarity,
		},
	}, nil
}

// normalizedSimilarity returns 1.0 minus the normalized Levenshtein distance.
func normalizedSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 {
		return 0.0
	}
	if len(b) == 0 {
		return 0.0
	}

	dist := levenshtein(a, b)
	maxLen := math.Max(float64(len(a)), float64(len(b)))
	return 1.0 - float64(dist)/maxLen
}

// levenshtein computes the Levenshtein edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}
