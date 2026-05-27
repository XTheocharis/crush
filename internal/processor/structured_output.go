package processor

import (
	"context"
	"encoding/json"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*StructuredOutput)(nil)

// StructuredLLMClient is the interface used by StructuredOutput to call an LLM
// for converting unstructured text into structured JSON.
type StructuredLLMClient interface {
	Complete(ctx context.Context, prompt, input string) (string, error)
}

// StructuredOutputConfig holds optional configuration for the StructuredOutput
// processor.
type StructuredOutputConfig struct {
	// Schema is an optional JSON schema hint passed to the LLM so it knows the
	// expected shape of the output.
	Schema string
}

// StructuredOutput is an output-only processor that sends assistant message
// content to an LLM and asks it to structure the content as JSON. If the LLM
// determines the content can be structured, the processor rewrites the message
// with the JSON output (ActionRewrite). Otherwise it passes through unchanged
// (ActionContinue).
type StructuredOutput struct {
	client StructuredLLMClient
	config StructuredOutputConfig
}

// NewStructuredOutput creates a new StructuredOutput processor with the given
// LLM client and optional configuration.
func NewStructuredOutput(client StructuredLLMClient, config StructuredOutputConfig) *StructuredOutput {
	return &StructuredOutput{
		client: client,
		config: config,
	}
}

// ID returns the unique processor identifier.
func (s *StructuredOutput) ID() string {
	return "structured_output"
}

// ProcessInput is a pass-through — this is an output-only processor.
func (s *StructuredOutput) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessOutputStream sends streaming output to the LLM for structuring.
func (s *StructuredOutput) ProcessOutputStream(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return s.processOutput(ctx, pctx)
}

// ProcessOutputResult sends the final output to the LLM for structuring.
func (s *StructuredOutput) ProcessOutputResult(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return s.processOutput(ctx, pctx)
}

// ProcessAPIError is a pass-through — errors are not structured.
func (s *StructuredOutput) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// structuredResponse is the JSON shape we expect the LLM to return.
type structuredResponse struct {
	Structured bool   `json:"structured"`
	JSONOutput string `json:"json_output"`
	Schema     string `json:"schema"`
}

// processOutput is the shared implementation for OutputStream and OutputResult
// phases. It collects assistant message content, sends it to the LLM, and
// rewrites if the LLM returns structured JSON.
func (s *StructuredOutput) processOutput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	// Collect all assistant message content.
	var parts []string
	for _, msg := range pctx.Messages {
		if msg.Role == "assistant" && msg.Content != "" {
			parts = append(parts, msg.Content)
		}
	}
	combined := strings.Join(parts, "\n")

	state := map[string]any{
		"structured":    false,
		"schema":        "",
		"output_length": len(combined),
	}

	if combined == "" {
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	prompt := `Analyze the following text and determine if it can be meaningfully converted to structured JSON. ` +
		`Return a JSON object with exactly three fields: ` +
		`"structured" (boolean, true if the content can be structured), ` +
		`"json_output" (string, the structured JSON representation if structured is true, otherwise empty string), ` +
		`"schema" (string, a brief description of the schema used if structured is true, otherwise empty string). ` +
		`If the content is already valid JSON or does not benefit from structuring, set structured to false.`

	if s.config.Schema != "" {
		prompt += ` The target schema should conform to: ` + s.config.Schema
	}

	resp, err := s.client.Complete(ctx, prompt, combined)
	if err != nil {
		// On LLM error, fall back to continue without structuring.
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	var parsed structuredResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		// Invalid JSON from LLM, fall back to continue.
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	state["schema"] = parsed.Schema

	if !parsed.Structured || parsed.JSONOutput == "" {
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	// Rewrite assistant messages with the structured JSON content.
	rewritten := make([]Message, len(pctx.Messages))
	for i, msg := range pctx.Messages {
		if msg.Role == "assistant" && msg.Content != "" {
			rewritten[i] = Message{
				Role:    msg.Role,
				Content: parsed.JSONOutput,
				Meta:    msg.Meta,
			}
		} else {
			rewritten[i] = msg
		}
	}

	state["structured"] = true

	return ProcessorResult{
		Messages: rewritten,
		State:    state,
		Action:   ActionRewrite,
	}, nil
}
