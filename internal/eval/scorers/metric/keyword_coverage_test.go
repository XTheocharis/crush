package metric

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

func TestKeywordCoverageAllMatch(t *testing.T) {
	t.Parallel()
	s := NewKeywordCoverageScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Before: "implement authentication handler", After: "implement authentication handler"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestKeywordCoveragePartialMatch(t *testing.T) {
	t.Parallel()
	s := NewKeywordCoverageScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Before: "implement authentication handler", After: "authentication module"},
		},
	})
	require.NoError(t, err)
	require.Greater(t, result.Score, 0.0)
	require.Less(t, result.Score, 1.0)
}

func TestKeywordCoverageNoEdits(t *testing.T) {
	t.Parallel()
	s := NewKeywordCoverageScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestKeywordCoverageNameAndType(t *testing.T) {
	t.Parallel()
	s := NewKeywordCoverageScorer(0.8)
	require.Equal(t, "KeywordCoverage", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}
