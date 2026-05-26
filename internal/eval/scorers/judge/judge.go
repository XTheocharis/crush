// Package judge provides LLM-as-judge scorers that evaluate agent output
// by delegating scoring to an LLM. Each scorer formats a domain-specific
// prompt, calls the LLM, and parses the structured JSON response.
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"text/template"

	"github.com/charmbracelet/crush/internal/eval"
)

// LLMClient is the interface for calling an LLM to produce a judgment.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// JudgeResponse is the expected JSON structure from the LLM.
type JudgeResponse struct {
	Score       float64 `json:"score"`
	Explanation string  `json:"explanation"`
}

type judgePromptData struct {
	Conversation string
	Edits        string
	Files        string
	TestResults  string
	Criteria     string
}

const responseFormat = `Respond ONLY with a JSON object: {"score": <0.0-1.0>, "explanation": "<brief explanation>"}`

// NewLLMJudgeScorer creates an LLM judge scorer with a custom name and
// evaluation criteria. This is the exported version for use by external
// packages that need spec-aligned scorer names.
func NewLLMJudgeScorer(name string, client LLMClient, threshold float64, criteria, promptBody string) *LLMJudgeScorer {
	return newLLMJudgeScorer(name, client, threshold, criteria, promptBody)
}

// LLMJudgeScorer is a base scorer that delegates evaluation to an LLM.
type LLMJudgeScorer struct {
	scorerName string
	client     LLMClient
	threshold  float64
	criteria   string
	promptTmpl *template.Template
}

// newLLMJudgeScorer creates a base LLM judge scorer with the given prompt template.
func newLLMJudgeScorer(name string, client LLMClient, threshold float64, criteria, promptBody string) *LLMJudgeScorer {
	fullPrompt := fmt.Sprintf(`You are an expert code evaluator assessing: %s.

%s

{{if .Conversation}}Conversation:
{{.Conversation}}{{end}}

{{if .Edits}}Code changes:
{{.Edits}}{{end}}

{{if .Files}}Files:
{{.Files}}{{end}}

{{if .TestResults}}Test results:
{{.TestResults}}{{end}}

%s`, criteria, promptBody, responseFormat)

	tmpl := template.Must(template.New(name).Parse(fullPrompt))

	return &LLMJudgeScorer{
		scorerName: name,
		client:     client,
		threshold:  threshold,
		criteria:   criteria,
		promptTmpl: tmpl,
	}
}

func (s *LLMJudgeScorer) Name() string          { return s.scorerName }
func (s *LLMJudgeScorer) Type() eval.ScorerType { return eval.ScorerLLMJudge }

func (s *LLMJudgeScorer) Score(ctx context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	data := judgePromptData{
		Conversation: formatConversation(input.Conversation),
		Edits:        formatEdits(input.Edits),
		Files:        formatFiles(input.Files),
		TestResults:  formatTestResults(input.TestResults),
	}

	var buf strings.Builder
	if err := s.promptTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("judge: failed to render prompt for %s: %w", s.scorerName, err)
	}

	resp, err := s.client.Complete(ctx, buf.String())
	if err != nil {
		return &eval.ScoreResult{
			Score:       0.5,
			Explanation: fmt.Sprintf("LLM call failed: %v", err),
			Passed:      false,
			Error:       err.Error(),
		}, nil
	}

	judgeResp := parseJudgeResponse(resp)
	score := math.Max(0, math.Min(1, judgeResp.Score))

	return &eval.ScoreResult{
		Score:       score,
		Explanation: judgeResp.Explanation,
		Passed:      score >= s.threshold,
		Details: map[string]any{
			"criteria":     s.criteria,
			"raw_response": resp,
		},
	}, nil
}

// parseJudgeResponse extracts JSON from the LLM response, handling markdown
// code block wrapping and falling back to score 0.5 on parse failure.
func parseJudgeResponse(raw string) JudgeResponse {
	cleaned := extractJSON(raw)

	var resp JudgeResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return JudgeResponse{Score: 0.5, Explanation: "Failed to parse LLM response."}
	}
	return resp
}

// extractJSON strips markdown code fences and extracts the JSON object.
func extractJSON(raw string) string {
	re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
	matches := re.FindStringSubmatch(raw)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}

	return raw
}

func formatConversation(msgs []eval.Message) string {
	if len(msgs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s]: %s\n", m.Role, m.Content)
	}
	return b.String()
}

func formatEdits(edits []eval.FileEdit) string {
	if len(edits) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range edits {
		fmt.Fprintf(&b, "--- %s (before)\n%s\n+++ %s (after)\n%s\n", e.Path, e.Before, e.Path, e.After)
	}
	return b.String()
}

func formatFiles(files map[string]string) string {
	if len(files) == 0 {
		return ""
	}
	var b strings.Builder
	for name, content := range files {
		fmt.Fprintf(&b, "=== %s ===\n%s\n", name, content)
	}
	return b.String()
}

func formatTestResults(tr *eval.TestResult) string {
	if tr == nil {
		return ""
	}
	return fmt.Sprintf("Total: %d, Passed: %d, Failed: %d\nOutput:\n%s", tr.Total, tr.Passed, tr.Failed, tr.Output)
}
