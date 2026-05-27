package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/stretchr/testify/require"
)

type mockLLM struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockLLM) Complete(_ context.Context, prompt string) (string, error) {
	m.called = true
	m.prompt = prompt
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func mockResp(score float64, explanation string) string {
	b, _ := json.Marshal(JudgeResponse{Score: score, Explanation: explanation})
	return string(b)
}

func mockRespWrapped(score float64, explanation string) string {
	b, _ := json.Marshal(JudgeResponse{Score: score, Explanation: explanation})
	return "```json\n" + string(b) + "\n```"
}

func allScorers(client LLMClient) []eval.Scorer {
	return []eval.Scorer{
		NewCodeQualityScorer(client, 0.6),
		NewCorrectnessScorer(client, 0.6),
		NewCompletenessScorer(client, 0.6),
		NewClarityScorer(client, 0.6),
		NewSafetyScorer(client, 0.6),
		NewPerformanceScorer(client, 0.6),
		NewMaintainabilityScorer(client, 0.6),
		NewErrorHandlingScorer(client, 0.6),
		NewDocumentationScorer(client, 0.6),
		NewConventionsScorer(client, 0.6),
		NewTestingQualityScorer(client, 0.6),
		NewEdgeCasesScorer(client, 0.6),
	}
}

var scorerNames = []string{
	"code_quality", "correctness", "completeness", "clarity",
	"safety", "performance", "maintainability", "error_handling",
	"documentation", "conventions", "testing_quality", "edge_cases",
}

var scorerCriteria = []string{
	"Code Quality", "Correctness", "Completeness", "Clarity",
	"Safety", "Performance", "Maintainability", "Error Handling",
	"Documentation", "Conventions", "Testing Quality", "Edge Cases",
}

func TestAllScorerNames(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.0, "ok")}
	scorers := allScorers(m)
	require.Len(t, scorers, 12)
	for i, name := range scorerNames {
		require.Equal(t, name, scorers[i].Name())
		require.Equal(t, eval.ScorerLLMJudge, scorers[i].Type())
	}
}

func TestScoreFromJSON(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(0.85, "Good quality code.")}
	s := NewCodeQualityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"main.go": "package main"},
	})
	require.NoError(t, err)
	require.Equal(t, 0.85, result.Score)
	require.Equal(t, "Good quality code.", result.Explanation)
	require.True(t, result.Passed)
	require.Equal(t, eval.ScorerLLMJudge, s.Type())
}

func TestScoreFromWrappedJSON(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockRespWrapped(0.75, "Decent code.")}
	s := NewCorrectnessScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.75, result.Score)
	require.Equal(t, "Decent code.", result.Explanation)
	require.True(t, result.Passed)
}

func TestScoreBelowThreshold(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(0.4, "Poor quality.")}
	s := NewCodeQualityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.4, result.Score)
	require.False(t, result.Passed)
}

func TestScoreClampedToRange(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.5, "Over score")}
	s := NewClarityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 1.0, result.Score)
}

func TestScoreClampedNegative(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(-0.5, "Under score")}
	s := NewClarityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.0, result.Score)
}

func TestLLMError(t *testing.T) {
	t.Parallel()
	m := &mockLLM{err: fmt.Errorf("timeout")}
	s := NewSafetyScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.5, result.Score)
	require.Contains(t, result.Explanation, "LLM call failed")
	require.NotEmpty(t, result.Error)
}

func TestNonJSONResponse(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: "This code looks good to me!"}
	s := NewPerformanceScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.5, result.Score)
	require.Contains(t, result.Explanation, "Failed to parse")
}

func TestPartialJSONResponse(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: `Here's my eval: {"score": 0.9, "explanation": "Looks great!"} Hope that helps!`}
	s := NewMaintainabilityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.Equal(t, 0.9, result.Score)
	require.Equal(t, "Looks great!", result.Explanation)
}

func TestExtractJSONFromCodeBlock(t *testing.T) {
	t.Parallel()
	input := "```json\n{\"score\": 0.7, \"explanation\": \"test\"}\n```"
	result := extractJSON(input)
	require.Contains(t, result, `"score"`)
}

func TestExtractJSONFromBareBlock(t *testing.T) {
	t.Parallel()
	input := "```\n{\"score\": 0.7, \"explanation\": \"test\"}\n```"
	result := extractJSON(input)
	require.Contains(t, result, `"score"`)
}

func TestExtractJSONBare(t *testing.T) {
	t.Parallel()
	input := `{"score": 0.7, "explanation": "test"}`
	result := extractJSON(input)
	require.Equal(t, input, result)
}

func TestExtractJSONNoObject(t *testing.T) {
	t.Parallel()
	result := extractJSON("no json here")
	require.Equal(t, "no json here", result)
}

func TestParseJudgeResponseValid(t *testing.T) {
	t.Parallel()
	resp := parseJudgeResponse(`{"score": 0.8, "explanation": "good"}`)
	require.Equal(t, 0.8, resp.Score)
	require.Equal(t, "good", resp.Explanation)
}

func TestParseJudgeResponseInvalid(t *testing.T) {
	t.Parallel()
	resp := parseJudgeResponse("not json")
	require.Equal(t, 0.5, resp.Score)
}

func TestPromptContainsCriteria(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.0, "ok")}
	s := NewCodeQualityScorer(m, 0.6)
	_, _ = s.Score(context.Background(), &eval.EvalInput{
		Files: map[string]string{"a.go": "package main"},
	})
	require.True(t, m.called)
	require.Contains(t, m.prompt, "Code Quality")
	require.Contains(t, m.prompt, "Respond ONLY with a JSON object")
}

func TestPromptIncludesConversation(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.0, "ok")}
	s := NewCorrectnessScorer(m, 0.6)
	_, _ = s.Score(context.Background(), &eval.EvalInput{
		Conversation: []eval.Message{
			{Role: "user", Content: "Fix the bug"},
			{Role: "assistant", Content: "I fixed it"},
		},
	})
	require.Contains(t, m.prompt, "[user]: Fix the bug")
	require.Contains(t, m.prompt, "[assistant]: I fixed it")
}

func TestPromptIncludesEdits(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.0, "ok")}
	s := NewCorrectnessScorer(m, 0.6)
	_, _ = s.Score(context.Background(), &eval.EvalInput{
		Edits: []eval.FileEdit{
			{Path: "main.go", Before: "old", After: "new"},
		},
	})
	require.Contains(t, m.prompt, "main.go (before)")
	require.Contains(t, m.prompt, "main.go (after)")
}

func TestPromptIncludesTestResults(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(1.0, "ok")}
	s := NewTestingQualityScorer(m, 0.6)
	_, _ = s.Score(context.Background(), &eval.EvalInput{
		TestResults: &eval.TestResult{Total: 10, Passed: 8, Failed: 2, Output: "FAIL: TestX"},
	})
	require.Contains(t, m.prompt, "Total: 10")
	require.Contains(t, m.prompt, "FAIL: TestX")
}

func TestAllScorersInHarness(t *testing.T) {
	t.Parallel()
	h := eval.NewEvalHarness()
	scorers := []eval.Scorer{
		NewCodeQualityScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewCorrectnessScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewCompletenessScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewClarityScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewSafetyScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewPerformanceScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewMaintainabilityScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewErrorHandlingScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewDocumentationScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewConventionsScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewTestingQualityScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
		NewEdgeCasesScorer(&mockLLM{response: mockResp(0.8, "Good")}, 0.6),
	}
	for _, s := range scorers {
		h.Register(s)
	}

	report, err := h.Run(context.Background(), &eval.EvalInput{
		SessionID: "judge_test",
		Files:     map[string]string{"main.go": "package main"},
	})
	require.NoError(t, err)
	require.Len(t, report.Results, 12)
	require.True(t, report.OverallScore > 0)
	require.True(t, report.Passed)
}

func TestEachScorerProducesUniquePrompt(t *testing.T) {
	t.Parallel()
	prompts := make(map[string]string)
	for i, name := range scorerNames {
		m := &mockLLM{response: mockResp(1.0, "ok")}
		scorers := allScorers(m)
		_, _ = scorers[i].Score(context.Background(), &eval.EvalInput{
			Files: map[string]string{"a.go": "package main"},
		})
		require.True(t, m.called, "scorer %s was not called", name)
		prompts[name] = m.prompt
	}

	for i, name := range scorerNames {
		require.Contains(t, prompts[name], scorerCriteria[i],
			"prompt for %s should contain criteria %q", name, scorerCriteria[i])
	}

	unique := make(map[string]bool)
	for _, p := range prompts {
		firstLine := strings.Split(p, "\n")[0]
		unique[firstLine] = true
	}
	require.GreaterOrEqual(t, len(unique), 10,
		"scorers should have diverse prompt templates")
}

func TestFormatConversation(t *testing.T) {
	t.Parallel()
	result := formatConversation([]eval.Message{
		{Role: "user", Content: "hello"},
	})
	require.Contains(t, result, "[user]: hello")
}

func TestFormatConversationEmpty(t *testing.T) {
	t.Parallel()
	require.Empty(t, formatConversation(nil))
}

func TestFormatEdits(t *testing.T) {
	t.Parallel()
	result := formatEdits([]eval.FileEdit{
		{Path: "a.go", Before: "old", After: "new"},
	})
	require.Contains(t, result, "a.go (before)")
	require.Contains(t, result, "old")
	require.Contains(t, result, "a.go (after)")
	require.Contains(t, result, "new")
}

func TestFormatEditsEmpty(t *testing.T) {
	t.Parallel()
	require.Empty(t, formatEdits(nil))
}

func TestFormatFiles(t *testing.T) {
	t.Parallel()
	result := formatFiles(map[string]string{"a.go": "package main"})
	require.Contains(t, result, "=== a.go ===")
	require.Contains(t, result, "package main")
}

func TestFormatFilesEmpty(t *testing.T) {
	t.Parallel()
	require.Empty(t, formatFiles(nil))
}

func TestFormatTestResults(t *testing.T) {
	t.Parallel()
	result := formatTestResults(&eval.TestResult{Total: 5, Passed: 3, Failed: 2, Output: "ok"})
	require.Contains(t, result, "Total: 5")
	require.Contains(t, result, "Passed: 3")
}

func TestFormatTestResultsNil(t *testing.T) {
	t.Parallel()
	require.Empty(t, formatTestResults(nil))
}

func TestDefaultThreshold(t *testing.T) {
	t.Parallel()
	m := &mockLLM{response: mockResp(0.59, "just below")}
	s := NewCodeQualityScorer(m, 0.6)
	result, err := s.Score(context.Background(), &eval.EvalInput{})
	require.NoError(t, err)
	require.False(t, result.Passed)
}
