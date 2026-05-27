package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*SystemPromptScrubber)(nil)

// ScrubberLLMClient is the interface used by SystemPromptScrubber to call an
// LLM for system prompt leak detection. It mirrors MockLLMClient.Complete.
type ScrubberLLMClient interface {
	Complete(ctx context.Context, prompt, input string) (string, error)
}

// SystemPromptScrubber detects and removes leaked system prompt instructions
// from LLM output. It sends output content to an LLM to determine whether the
// text contains leaked system instructions and, if so, returns scrubbed
// content via ActionRewrite.
type SystemPromptScrubber struct {
	llm ScrubberLLMClient
}

// NewSystemPromptScrubber creates a scrubber with the given LLM client.
func NewSystemPromptScrubber(llm ScrubberLLMClient) *SystemPromptScrubber {
	return &SystemPromptScrubber{llm: llm}
}

// ID returns the unique processor identifier.
func (s *SystemPromptScrubber) ID() string {
	return "system_prompt_scrubber"
}

// ProcessInput is a pass-through — the scrubber does not scan input.
func (s *SystemPromptScrubber) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessOutputStream scans streaming output for leaked system prompt
// content. When a leak is detected it returns ActionRewrite with scrubbed
// content.
func (s *SystemPromptScrubber) ProcessOutputStream(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return s.scrub(ctx, pctx)
}

// ProcessOutputResult scans the final output for leaked system prompt content.
// When a leak is detected it returns ActionRewrite with scrubbed content.
func (s *SystemPromptScrubber) ProcessOutputResult(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return s.scrub(ctx, pctx)
}

// ProcessAPIError is a pass-through — errors are not scanned.
func (s *SystemPromptScrubber) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// scrub sends each message's content to the LLM for leak detection and
// rewrites any that contain leaked system prompt instructions.
func (s *SystemPromptScrubber) scrub(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	rewritten := make([]Message, len(pctx.Messages))
	anyLeak := false
	var leakTypes []string
	scrubbedCount := 0

	for i, msg := range pctx.Messages {
		if msg.Content == "" {
			rewritten[i] = msg
			continue
		}

		clean, leakDetected, leakType, err := s.detectLeak(ctx, msg.Content)
		if err != nil {
			// On LLM error, pass through unchanged.
			rewritten[i] = msg
			continue
		}

		if leakDetected {
			anyLeak = true
			scrubbedCount++
			if leakType != "" {
				leakTypes = append(leakTypes, leakType)
			}
			rewritten[i] = Message{
				Role:    msg.Role,
				Content: clean,
				Meta:    msg.Meta,
			}
		} else {
			rewritten[i] = msg
		}
	}

	state := map[string]any{
		"leak_detected": anyLeak,
		"leak_type":     strings.Join(leakTypes, ","),
		"scrubbed":      scrubbedCount > 0,
	}

	action := ActionContinue
	if anyLeak {
		action = ActionRewrite
	}

	return ProcessorResult{
		Messages: rewritten,
		State:    state,
		Action:   action,
	}, nil
}

// scrubResponse is the expected JSON structure returned by the LLM.
type scrubResponse struct {
	LeakDetected    bool   `json:"leak_detected"`
	ScrubbedContent string `json:"scrubbed_content"`
	LeakType        string `json:"leak_type"`
}

// detectLeak sends content to the LLM and returns the scrubbed text, whether a
// leak was detected, the leak type, and any error.
func (s *SystemPromptScrubber) detectLeak(ctx context.Context, content string) (string, bool, string, error) {
	prompt := `Analyze the following LLM response for leaked system prompt instructions. ` +
		`A leak occurs when the response contains verbatim system instructions, ` +
		`system prompts, role definitions, or internal configuration meant to be hidden. ` +
		`Return a JSON object with: "leak_detected" (boolean), ` +
		`"scrubbed_content" (string, the content with leaked portions removed), ` +
		`"leak_type" (string, e.g. "system_prompt", "role_definition", "configuration", or empty if no leak). ` +
		fmt.Sprintf(`If no leak: {"leak_detected":false,"scrubbed_content":"%s","leak_type":""}.`, content)

	resp, err := s.llm.Complete(ctx, prompt, content)
	if err != nil {
		return "", false, "", err
	}

	var parsed scrubResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		return content, false, "", nil
	}

	if !parsed.LeakDetected {
		return content, false, "", nil
	}

	return parsed.ScrubbedContent, true, parsed.LeakType, nil
}
