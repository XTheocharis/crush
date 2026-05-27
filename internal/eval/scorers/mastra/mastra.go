// Package mastra provides a 4-step pipeline scorer (preprocess → analyze →
// generateScore → generateReason) inspired by the Mastra evaluation framework.
// Each step is a StepHandler, allowing pure functions or LLM calls.
package mastra

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strings"
	"text/template"

	"github.com/charmbracelet/crush/internal/eval"
	"github.com/charmbracelet/crush/internal/eval/scorers/judge"
)

// ScorerStep identifies a pipeline step.
type ScorerStep int

const (
	StepPreprocess ScorerStep = iota
	StepAnalyze
	StepGenerateScore
	StepGenerateReason
)

// StepInput is the input to a single pipeline step.
type StepInput struct {
	EvalInput      *eval.EvalInput
	PreviousOutput string
	Metadata       map[string]any
}

// StepOutput is the output of a single pipeline step.
type StepOutput struct {
	Result   string
	Score    float64
	Metadata map[string]any
}

// StepHandler processes one step of the pipeline.
type StepHandler interface {
	Handle(ctx context.Context, input *StepInput) (*StepOutput, error)
}

// StepHandlerFunc adapts a function to the StepHandler interface.
type StepHandlerFunc func(ctx context.Context, input *StepInput) (*StepOutput, error)

func (f StepHandlerFunc) Handle(ctx context.Context, input *StepInput) (*StepOutput, error) {
	return f(ctx, input)
}

// MastraScorer implements eval.Scorer with a 4-step pipeline.
type MastraScorer struct {
	scorerName string
	threshold  float64
	client     judge.LLMClient
	criteria   string
	promptTmpl *template.Template
	steps      [4]StepHandler
}

// NewMastraScorer creates a MastraScorer with default pipeline steps.
// The analyze step calls the LLM; the other steps are pure functions.
func NewMastraScorer(name string, client judge.LLMClient, threshold float64, criteria, promptBody string) *MastraScorer {
	tmpl := template.Must(template.New(name).Parse(fmt.Sprintf(
		`You are an expert evaluator assessing: %s.

%s

{{if .PreviousOutput}}Preprocessed input:
{{.PreviousOutput}}{{end}}

Respond ONLY with a JSON object: {"score": <0.0-1.0>, "explanation": "<brief explanation>"}`,
		criteria, promptBody,
	)))

	s := &MastraScorer{
		scorerName: name,
		threshold:  threshold,
		client:     client,
		criteria:   criteria,
		promptTmpl: tmpl,
	}
	s.steps = [4]StepHandler{
		&defaultPreprocessHandler{},
		&defaultAnalyzeHandler{client: client, tmpl: tmpl},
		&defaultGenerateScoreHandler{},
		&defaultGenerateReasonHandler{},
	}
	return s
}

func (s *MastraScorer) Name() string          { return s.scorerName }
func (s *MastraScorer) Type() eval.ScorerType { return eval.ScorerMastra }

func (s *MastraScorer) Score(ctx context.Context, input *eval.EvalInput) (*eval.ScoreResult, error) {
	stepInput := &StepInput{EvalInput: input}
	var lastOutput *StepOutput
	var scoreOutput *StepOutput
	var err error

	for i, handler := range s.steps {
		lastOutput, err = handler.Handle(ctx, stepInput)
		if err != nil {
			return nil, fmt.Errorf("mastra %s step %d: %w", s.scorerName, i, err)
		}
		if i == int(StepGenerateScore) {
			scoreOutput = lastOutput
		}
		stepInput = &StepInput{
			EvalInput:      input,
			PreviousOutput: lastOutput.Result,
			Metadata:       lastOutput.Metadata,
		}
	}

	score := scoreOutput.Score
	score = math.Max(0, math.Min(1, score))
	reason := lastOutput.Result

	return &eval.ScoreResult{
		Score:       score,
		Explanation: reason,
		Passed:      score >= s.threshold,
		Details: map[string]any{
			"criteria": s.criteria,
		},
	}, nil
}

// defaultPreprocessHandler normalizes input for downstream steps.
type defaultPreprocessHandler struct{}

func (h *defaultPreprocessHandler) Handle(_ context.Context, input *StepInput) (*StepOutput, error) {
	var b strings.Builder
	for _, msg := range input.EvalInput.Conversation {
		fmt.Fprintf(&b, "[%s]: %s\n", msg.Role, msg.Content)
	}
	for _, edit := range input.EvalInput.Edits {
		fmt.Fprintf(&b, "--- %s (before)\n%s\n+++ %s (after)\n%s\n",
			edit.Path, edit.Before, edit.Path, edit.After)
	}
	for name, content := range input.EvalInput.Files {
		fmt.Fprintf(&b, "=== %s ===\n%s\n", name, content)
	}
	if input.EvalInput.TestResults != nil {
		tr := input.EvalInput.TestResults
		fmt.Fprintf(&b, "Tests: %d/%d passed\n%s\n", tr.Passed, tr.Total, tr.Output)
	}
	return &StepOutput{Result: b.String()}, nil
}

type analyzePromptData struct {
	PreviousOutput string
}

// defaultAnalyzeHandler calls the LLM to analyze the preprocessed input.
type defaultAnalyzeHandler struct {
	client judge.LLMClient
	tmpl   *template.Template
}

func (h *defaultAnalyzeHandler) Handle(ctx context.Context, input *StepInput) (*StepOutput, error) {
	data := analyzePromptData{PreviousOutput: input.PreviousOutput}
	var buf strings.Builder
	if err := h.tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	resp, err := h.client.Complete(ctx, buf.String())
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}
	return &StepOutput{Result: resp, Metadata: map[string]any{"raw_response": resp}}, nil
}

// defaultGenerateScoreHandler extracts a numeric score from the analysis.
type defaultGenerateScoreHandler struct{}

func (h *defaultGenerateScoreHandler) Handle(_ context.Context, input *StepInput) (*StepOutput, error) {
	score := extractScore(input.PreviousOutput)
	meta := make(map[string]any)
	maps.Copy(meta, input.Metadata)
	meta["score"] = score
	return &StepOutput{Result: input.PreviousOutput, Score: score, Metadata: meta}, nil
}

// defaultGenerateReasonHandler formats the final explanation.
type defaultGenerateReasonHandler struct{}

func (h *defaultGenerateReasonHandler) Handle(_ context.Context, input *StepInput) (*StepOutput, error) {
	reason := extractExplanation(input.PreviousOutput)
	if reason == "" {
		if score, ok := input.Metadata["score"].(float64); ok {
			reason = fmt.Sprintf("Score: %.2f", score)
		} else {
			reason = "No explanation available"
		}
	}
	score, _ := input.Metadata["score"].(float64)
	return &StepOutput{Result: reason, Score: score, Metadata: input.Metadata}, nil
}

func extractScore(raw string) float64 {
	cleaned := extractJSON(raw)
	var resp struct {
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return 0.5
	}
	return resp.Score
}

func extractExplanation(raw string) string {
	cleaned := extractJSON(raw)
	var resp struct {
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return ""
	}
	return resp.Explanation
}

func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}
