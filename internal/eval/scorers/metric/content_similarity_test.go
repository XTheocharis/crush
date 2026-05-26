package metric

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

func TestContentSimilarityIdentical(t *testing.T) {
	t.Parallel()
	s := NewContentSimilarityScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Before: "hello world foo bar", After: "hello world foo bar"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestContentSimilarityNoOverlap(t *testing.T) {
	t.Parallel()
	s := NewContentSimilarityScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Before: "alpha beta gamma", After: "delta epsilon zeta"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestContentSimilarityNoEdits(t *testing.T) {
	t.Parallel()
	s := NewContentSimilarityScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestContentSimilarityNameAndType(t *testing.T) {
	t.Parallel()
	s := NewContentSimilarityScorer(0.8)
	require.Equal(t, "ContentSimilarity", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}
