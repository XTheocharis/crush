package processor

import (
	"context"
	"encoding/json"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*LanguageDetector)(nil)

// LanguageLLMClient is the interface used by LanguageDetector to call an LLM
// for language detection. It mirrors MockLLMClient.Complete.
type LanguageLLMClient interface {
	Complete(ctx context.Context, prompt, input string) (string, error)
}

// LanguageDetector detects the language of user messages using an LLM and tags
// each message with the detected ISO 639-1 language code. Detection runs only
// during the input phase; all other phases pass through unchanged.
type LanguageDetector struct {
	llm LanguageLLMClient
}

// languageResult is the JSON structure expected from the LLM response.
type languageResult struct {
	Language     string   `json:"language"`
	Confidence   float64  `json:"confidence"`
	Alternatives []string `json:"alternatives"`
}

// NewLanguageDetector creates a LanguageDetector with the given LLM client.
func NewLanguageDetector(llm LanguageLLMClient) *LanguageDetector {
	return &LanguageDetector{llm: llm}
}

// ID returns the unique processor identifier.
func (d *LanguageDetector) ID() string {
	return "language_detector"
}

// ProcessInput detects the language of the first user message content by
// sending it to the LLM, then tags every message Meta with the detected
// language code. If no user content is found, the LLM call is skipped.
func (d *LanguageDetector) ProcessInput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	// Find the first user message with non-empty content.
	var input string
	for _, msg := range pctx.Messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			input = msg.Content
			break
		}
	}

	state := map[string]any{
		"detected_language": "",
		"confidence":        0.0,
		"alternatives":      []string{},
	}

	if input == "" {
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	prompt := `Detect the language of the following text. ` +
		`Return a JSON object with three fields: ` +
		`"language" (ISO 639-1 code, e.g. "en", "es", "zh", "fr", "de"), ` +
		`"confidence" (float between 0 and 1), ` +
		`"alternatives" (array of alternative ISO 639-1 language codes). ` +
		`Return ONLY the JSON object, no other text.`

	resp, err := d.llm.Complete(ctx, prompt, input)
	if err != nil {
		// On LLM error, pass through without tagging.
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	var parsed languageResult
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		// On invalid JSON, pass through without tagging.
		return ProcessorResult{
			Messages: pctx.Messages,
			State:    state,
			Action:   ActionContinue,
		}, nil
	}

	state["detected_language"] = parsed.Language
	state["confidence"] = parsed.Confidence
	if parsed.Alternatives == nil {
		parsed.Alternatives = []string{}
	}
	state["alternatives"] = parsed.Alternatives

	// Tag each message Meta with the detected language code.
	tagged := make([]Message, len(pctx.Messages))
	for i, msg := range pctx.Messages {
		meta := msg.Meta
		if meta == nil {
			meta = make(map[string]any)
		}
		meta["detected_language"] = parsed.Language
		tagged[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
			Meta:    meta,
		}
	}

	return ProcessorResult{
		Messages: tagged,
		State:    state,
		Action:   ActionContinue,
	}, nil
}

// ProcessOutputStream is a pass-through for streaming output.
func (d *LanguageDetector) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessOutputResult is a pass-through for the final output.
func (d *LanguageDetector) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessAPIError is a pass-through for API errors.
func (d *LanguageDetector) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}
