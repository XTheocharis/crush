package metric

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/crush/internal/eval"
)

// TestPassRateScorer analyzes file content for test function patterns and also
// uses TestResults when available.
type TestPassRateScorer struct {
	PassThreshold float64
}

// NewTestPassRateScorer creates a scorer that passes when the test pass rate
// meets or exceeds the threshold (0.0-1.0).
func NewTestPassRateScorer(threshold float64) *TestPassRateScorer {
	return &TestPassRateScorer{PassThreshold: threshold}
}

func (s *TestPassRateScorer) Name() string          { return "test_pass_rate" }
func (s *TestPassRateScorer) Type() eval.ScorerType { return eval.ScorerMetric }

var (
	testFuncRe    = regexp.MustCompile(`(?m)^func\s+Test\w+\s*\(`)
	testingTRe    = regexp.MustCompile(`\*testing\.T`)
	assertRe      = regexp.MustCompile(`(?:require\.|assert\.|Expect|Should|So\()`)
	tableDrivenRe = regexp.MustCompile(`t\.Run\(`)
)

func (s *TestPassRateScorer) Score(_ context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	if input.TestResults != nil && input.TestResults.Total > 0 {
		return scoreFromTestResults(input, s.PassThreshold)
	}

	testFileCount, totalTestFuncs, patternScore := analyzeTestPatterns(input)

	if testFileCount == 0 {
		return &eval.ScoreResult{
			Score:       0.0,
			Explanation: "No test results available and no test patterns detected.",
			Passed:      false,
			Details:     map[string]any{"total": 0, "passed": 0, "rate": 0.0, "source": "none"},
		}, nil
	}

	passed := patternScore >= s.PassThreshold
	explanation := fmt.Sprintf(
		"Detected %d test functions across %d files (pattern score %.1f%%)",
		totalTestFuncs, testFileCount, patternScore*100,
	)

	return &eval.ScoreResult{
		Score:       patternScore,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"test_files":    testFileCount,
			"test_funcs":    totalTestFuncs,
			"pattern_score": patternScore,
			"source":        "content_analysis",
		},
	}, nil
}

func scoreFromTestResults(input *eval.EvalInput, threshold float64) (*eval.ScoreResult, error) {
	rate := float64(input.TestResults.Passed) / float64(input.TestResults.Total)
	passed := rate >= threshold
	explanation := fmt.Sprintf(
		"%d/%d tests passed (%.1f%%)",
		input.TestResults.Passed, input.TestResults.Total, rate*100,
	)

	return &eval.ScoreResult{
		Score:       rate,
		Explanation: explanation,
		Passed:      passed,
		Details: map[string]any{
			"total":  input.TestResults.Total,
			"passed": input.TestResults.Passed,
			"failed": input.TestResults.Failed,
			"rate":   rate,
			"source": "test_results",
		},
	}, nil
}

func analyzeTestPatterns(input *eval.EvalInput) (testFileCount int, totalFuncs int, score float64) {
	sources := make(map[string]string)
	for _, edit := range input.Edits {
		if len(edit.After) > 0 {
			sources[edit.Path] = edit.After
		}
	}
	for path, content := range input.Files {
		sources[path] = content
	}

	totalIndicators := 0
	for _, content := range sources {
		funcs := len(testFuncRe.FindAllString(content, -1))
		if funcs == 0 {
			continue
		}
		testFileCount++
		totalFuncs += funcs
		totalIndicators += funcs

		if testingTRe.MatchString(content) {
			totalIndicators++
		}
		if assertRe.MatchString(content) {
			totalIndicators++
		}
		if tableDrivenRe.MatchString(content) {
			totalIndicators++
		}
	}

	if testFileCount == 0 {
		return 0, 0, 0.0
	}

	qualityBonus := 0.0
	if totalFuncs > 0 {
		qualityBonus = float64(strings.Count(fmt.Sprintf("%d", totalIndicators), "")) * 0.02
		if qualityBonus > 0.3 {
			qualityBonus = 0.3
		}
	}

	perFunc := float64(totalIndicators) / float64(totalFuncs)
	score = min(1.0, perFunc/3.0+qualityBonus)
	if score < 0.3 {
		score = 0.3
	}

	return testFileCount, totalFuncs, score
}
