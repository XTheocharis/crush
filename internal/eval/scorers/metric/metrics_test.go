package metric

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

func TestTestPassRatePerfect(t *testing.T) {
	t.Parallel()
	s := NewTestPassRateScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 10, Passed: 10, Failed: 0},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
	require.Equal(t, 10, result.Details["total"])
}

func TestTestPassRatePartial(t *testing.T) {
	t.Parallel()
	s := NewTestPassRateScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 10, Passed: 7, Failed: 3},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.7, result.Score, 0.001)
	require.False(t, result.Passed)
}

func TestTestPassRateNil(t *testing.T) {
	t.Parallel()
	s := NewTestPassRateScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTestPassRateZeroTests(t *testing.T) {
	t.Parallel()
	s := NewTestPassRateScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 0, Passed: 0},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTestPassRateNameAndType(t *testing.T) {
	t.Parallel()
	s := NewTestPassRateScorer(1.0)
	require.Equal(t, "test_pass_rate", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestLintScoreNoWarnings(t *testing.T) {
	t.Parallel()
	s := NewLintScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{After: "package main\nfunc main() {}\n"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestLintScoreWithWarnings(t *testing.T) {
	t.Parallel()
	s := NewLintScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{After: "\t\tindented\twith\ttabs"}},
	})
	require.NoError(t, err)
	require.Less(t, result.Score, 1.0)
}

func TestLintScoreNoEdits(t *testing.T) {
	t.Parallel()
	s := NewLintScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestLintScoreMaxZero(t *testing.T) {
	t.Parallel()
	s := NewLintScoreScorer(0, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{After: "code"}},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestLintScoreNameAndType(t *testing.T) {
	t.Parallel()
	s := NewLintScoreScorer(10, 0.8)
	require.Equal(t, "lint_score", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestBuildSuccess(t *testing.T) {
	t.Parallel()
	s := NewBuildSuccessScorer()
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{After: "package main"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestBuildSuccessFromFiles(t *testing.T) {
	t.Parallel()
	s := NewBuildSuccessScorer()
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"main.go": "package main"},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestBuildSuccessEmpty(t *testing.T) {
	t.Parallel()
	s := NewBuildSuccessScorer()
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestBuildSuccessNameAndType(t *testing.T) {
	t.Parallel()
	s := NewBuildSuccessScorer()
	require.Equal(t, "build_success", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestCoveragePerfect(t *testing.T) {
	t.Parallel()
	s := NewCoverageScoreScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 10, Passed: 10},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestCoveragePartial(t *testing.T) {
	t.Parallel()
	s := NewCoverageScoreScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 100, Passed: 75},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.75, result.Score, 0.001)
	require.False(t, result.Passed)
}

func TestCoverageNil(t *testing.T) {
	t.Parallel()
	s := NewCoverageScoreScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestCoverageZeroTests(t *testing.T) {
	t.Parallel()
	s := NewCoverageScoreScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 0},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestCoverageNameAndType(t *testing.T) {
	t.Parallel()
	s := NewCoverageScoreScorer(0.8)
	require.Equal(t, "coverage_score", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestEditDistanceExactMatch(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{Before: "hello", After: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestEditDistanceCompleteChange(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{Before: "aaa", After: "bbb"}},
	})
	require.NoError(t, err)
	require.Less(t, result.Score, 1.0)
}

func TestEditDistanceNoEdits(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestEditDistanceEmptyBefore(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{Before: "", After: "new file"}},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
}

func TestEditDistanceEmptyAfter(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{{Before: "deleted", After: ""}},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
}

func TestEditDistanceMultipleEdits(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.5)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Before: "same", After: "same"},
			{Before: "aaa", After: "bbb"},
		},
	})
	require.NoError(t, err)
	require.InDelta(t, 0.5, result.Score, 0.01)
}

func TestEditDistanceNameAndType(t *testing.T) {
	t.Parallel()
	s := NewEditDistanceScorer(0.8)
	require.Equal(t, "edit_distance", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestSyntaxValid(t *testing.T) {
	t.Parallel()
	s := NewSyntaxValidityScorer(1.0)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{
			"a.go": "package main\nfunc main() {\n\tprintln(\"hi\")\n}",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestSyntaxInvalid(t *testing.T) {
	t.Parallel()
	s := NewSyntaxValidityScorer(1.0)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{
			"a.go": "func main( {",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestSyntaxNoFiles(t *testing.T) {
	t.Parallel()
	s := NewSyntaxValidityScorer(1.0)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestSyntaxMixed(t *testing.T) {
	t.Parallel()
	s := NewSyntaxValidityScorer(0.5)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{
			"good.go": "package main\nfunc main() {}",
			"bad.go":  "func main( {",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0.5, result.Score)
	require.True(t, result.Passed)
}

func TestSyntaxBrackets(t *testing.T) {
	t.Parallel()
	require.True(t, isBracketBalanced("([]){}"))
	require.False(t, isBracketBalanced("([)]"))
	require.True(t, isBracketBalanced(""))
	require.False(t, isBracketBalanced("("))
	require.False(t, isBracketBalanced("}"))
}

func TestSyntaxNameAndType(t *testing.T) {
	t.Parallel()
	s := NewSyntaxValidityScorer(1.0)
	require.Equal(t, "syntax_validity", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestTypeCheckNoErrors(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"a.go": "package main\nfunc main() {}"},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTypeCheckWithErrors(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{
			"a.go": "cannot use x as int cannot use y as string",
		},
	})
	require.NoError(t, err)
	require.Less(t, result.Score, 1.0)
}

func TestTypeCheckNoFiles(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(10, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTypeCheckMaxZero(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(0, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"a.go": "package main"},
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
	require.True(t, result.Passed)
}

func TestTypeCheckMaxZeroWithErrors(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(0, 0.8)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"a.go": "cannot use x as int"},
	})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
	require.False(t, result.Passed)
}

func TestTypeNameAndType(t *testing.T) {
	t.Parallel()
	s := NewTypeCheckScoreScorer(10, 0.8)
	require.Equal(t, "typecheck_score", s.Name())
	require.Equal(t, eval.ScorerMetric, s.Type())
}

func TestLevenshtein(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, levenshtein("", ""))
	require.Equal(t, 3, levenshtein("", "abc"))
	require.Equal(t, 3, levenshtein("abc", ""))
	require.Equal(t, 0, levenshtein("same", "same"))
	require.Equal(t, 1, levenshtein("cat", "car"))
	require.Equal(t, 3, levenshtein("kitten", "sitting"))
}

func TestNormalizedSimilarity(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1.0, normalizedSimilarity("same", "same"))
	require.Equal(t, 0.0, normalizedSimilarity("", "abc"))
	require.Equal(t, 0.0, normalizedSimilarity("abc", ""))
}

func TestAllScorersInHarness(t *testing.T) {
	t.Parallel()
	h := eval.NewEvalHarness()
	h.Register(NewTestPassRateScorer(0.8))
	h.Register(NewLintScoreScorer(10, 0.8))
	h.Register(NewBuildSuccessScorer())
	h.Register(NewCoverageScoreScorer(0.8))
	h.Register(NewEditDistanceScorer(0.8))
	h.Register(NewSyntaxValidityScorer(1.0))
	h.Register(NewTypeCheckScoreScorer(10, 0.8))

	input := &eval.EvalInput{
		SessionID:   "integration_test",
		TestResults: &eval.TestResult{Total: 10, Passed: 9, Failed: 1},
		Edits: []eval.FileEdit{
			{Path: "main.go", Before: "package main", After: "package main\nfunc main() {}"},
		},
		Files: map[string]string{
			"main.go": "package main\nfunc main() {}",
		},
	}

	report, err := h.Run(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, report.Results, 7)
	require.True(t, report.OverallScore > 0)
}
