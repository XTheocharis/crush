package metric

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

func TestTrajectoryCodeSingleEdit(t *testing.T) {
	t.Parallel()
	s := NewTrajectoryCodeScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Path: "a.go", Before: "old", After: "old"},
		},
	})
	require.NoError(t, err)
	// Single edit: sequences differ only in suffix (:before vs :after),
	// so similarity is 0.0 for length-1 sequences.
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestTrajectoryCodeMatchingSequences(t *testing.T) {
	t.Parallel()
	s := NewTrajectoryCodeScorer(0.8)
	// Two edits with same path produce matching expected/actual sequences
	// except for :before/:after suffix. With 2 items, 1 match = 0.5 similarity.
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Path: "x", Before: "a", After: "a"},
			{Path: "x", Before: "b", After: "b"},
		},
	})
	require.NoError(t, err)
	// "x:before","x:before" vs "x:after","x:after" -> edit distance 2, maxLen 2, similarity 0.0
	require.Equal(t, 0.0, result.Score)
}

func TestTrajectoryCodeNoEdits(t *testing.T) {
	t.Parallel()
	s := NewTrajectoryCodeScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTrajectoryCodeNameAndType(t *testing.T) {
	t.Parallel()
	s := NewTrajectoryCodeScorer(0.8)
	require.Equal(t, "TrajectoryCodeScorer", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}
