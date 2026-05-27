package metric

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/crush/internal/eval"
)

type TrajectoryCodeScorer struct {
	PassThreshold float64
}

func NewTrajectoryCodeScorer(threshold float64) *TrajectoryCodeScorer {
	return &TrajectoryCodeScorer{PassThreshold: threshold}
}

func (s *TrajectoryCodeScorer) Name() string          { return "TrajectoryCodeScorer" }
func (s *TrajectoryCodeScorer) Type() eval.ScorerType { return eval.ScorerMetric }

func (s *TrajectoryCodeScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if len(input.Edits) == 0 {
		return &eval.ScoreResult{
			Score:       1.0,
			Explanation: "No edits to evaluate trajectory.",
			Passed:      true,
			Details:     map[string]any{"similarity": 1.0},
		}, nil
	}

	expectedSeq := make([]string, len(input.Edits))
	actualSeq := make([]string, len(input.Edits))
	for i, edit := range input.Edits {
		expectedSeq[i] = edit.Path + ":before"
		actualSeq[i] = edit.Path + ":after"
	}

	similarity := sequenceSimilarity(expectedSeq, actualSeq)
	passed := similarity >= s.PassThreshold
	explanation := fmt.Sprintf("Trajectory similarity: %.1f%%", similarity*100)

	return &eval.ScoreResult{
		Score:       similarity,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"expected_length": len(expectedSeq),
			"actual_length":   len(actualSeq),
			"similarity":      similarity,
		},
	}, nil
}

func extractToolCalls(msgs []eval.Message, role string) []string {
	var calls []string
	for _, msg := range msgs {
		if msg.Role != role {
			continue
		}
		for _, line := range strings.Split(msg.Content, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "tool:") || strings.HasPrefix(line, "call:") {
				calls = append(calls, line)
			}
		}
	}
	return calls
}

func sequenceSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	dist := sequenceEditDistance(a, b)
	maxLen := math.Max(float64(len(a)), float64(len(b)))
	return 1.0 - float64(dist)/maxLen
}

func sequenceEditDistance(a, b []string) int {
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
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
