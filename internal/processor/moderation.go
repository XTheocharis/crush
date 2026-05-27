package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Compile-time interface check.
var _ Processor = (*ModerationProcessor)(nil)

// ModerationLLMClient is the interface used by ModerationProcessor to call an
// LLM for content moderation analysis.
type ModerationLLMClient interface {
	Complete(ctx context.Context, prompt, input string) (string, error)
}

// ModerationProcessor analyzes content for toxic or harmful material using an
// LLM. It sends content to the LLM with a moderation prompt, parses a JSON
// response containing a toxicity score and categories, and returns an
// appropriate action based on a configurable threshold.
type ModerationProcessor struct {
	llm       ModerationLLMClient
	threshold float64
}

// NewModerationProcessor creates a moderation processor with the given LLM
// client and threshold. The threshold must be between 0.0 and 1.0; values
// outside this range are clamped. The default threshold is 0.7.
func NewModerationProcessor(llm ModerationLLMClient, threshold float64) *ModerationProcessor {
	if math.IsNaN(threshold) || threshold < 0 {
		threshold = 0.7
	}
	if threshold > 1 {
		threshold = 1.0
	}
	return &ModerationProcessor{
		llm:       llm,
		threshold: threshold,
	}
}

// ID returns the unique processor identifier.
func (m *ModerationProcessor) ID() string {
	return "moderation"
}

// ProcessInput sends input content to the LLM for moderation analysis.
func (m *ModerationProcessor) ProcessInput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return m.moderate(ctx, pctx)
}

// ProcessOutputStream is a pass-through — stream output is not moderated.
func (m *ModerationProcessor) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessOutputResult is a pass-through — final output is not moderated.
func (m *ModerationProcessor) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// ProcessAPIError is a pass-through — errors are not moderated.
func (m *ModerationProcessor) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{
		Messages: pctx.Messages,
		Action:   ActionContinue,
	}, nil
}

// moderationResponse is the expected JSON structure returned by the LLM.
type moderationResponse struct {
	Score      float64  `json:"score"`
	Categories []string `json:"categories"`
	Filtered   string   `json:"filtered,omitempty"`
}

// moderate sends all message content to the LLM for moderation checking.
func (m *ModerationProcessor) moderate(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	// Build combined content from all messages for analysis.
	var parts []string
	for _, msg := range pctx.Messages {
		if msg.Content != "" {
			parts = append(parts, msg.Content)
		}
	}
	content := strings.Join(parts, "\n")

	// Empty input: nothing to moderate.
	if content == "" {
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"toxicity_score": float64(0),
				"action_taken":   "continue",
				"categories":     []string{},
			},
			Action: ActionContinue,
		}, nil
	}

	prompt := `Analyze the following text for toxic or harmful content. ` +
		`Consider: hate speech, harassment, violence, self-harm, sexual content, ` +
		`and illegal activities. ` +
		`Return a JSON object with: "score" (float 0.0-1.0, toxicity severity), ` +
		`"categories" (array of strings, the types of toxicity found, e.g. "hate_speech", "harassment", "violence"), ` +
		`"filtered" (string, the text with toxic words replaced by [FILTERED], only if score > 0). ` +
		`If the text is clean, return {"score":0.0,"categories":[],"filtered":""}.`

	resp, err := m.llm.Complete(ctx, prompt, content)
	if err != nil {
		// On LLM error, fall back to continue without blocking.
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"toxicity_score": float64(0),
				"action_taken":   "continue",
				"categories":     []string{},
				"llm_error":      err.Error(),
			},
			Action: ActionContinue,
		}, nil
	}

	var parsed moderationResponse
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		// Invalid JSON: fall back to continue.
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"toxicity_score": float64(0),
				"action_taken":   "continue",
				"categories":     []string{},
			},
			Action: ActionContinue,
		}, nil
	}

	// Normalize categories to non-nil slice.
	categories := parsed.Categories
	if categories == nil {
		categories = []string{}
	}

	score := parsed.Score

	// Determine action based on score and threshold.
	switch {
	case score > m.threshold:
		// Highly toxic: abort the chain.
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"toxicity_score": score,
				"action_taken":   "abort",
				"categories":     categories,
			},
			Action: ActionAbort,
			Error:  fmt.Errorf("content blocked: toxicity score %.2f exceeds threshold %.2f", score, m.threshold),
		}, nil

	case score > 0:
		// Moderate toxicity: filter and rewrite.
		filtered := m.rewriteMessages(pctx.Messages, parsed.Filtered)
		return ProcessorResult{
			Messages: filtered,
			State: map[string]any{
				"toxicity_score": score,
				"action_taken":   "rewrite",
				"categories":     categories,
			},
			Action: ActionRewrite,
		}, nil

	default:
		// Clean content.
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"toxicity_score": score,
				"action_taken":   "continue",
				"categories":     categories,
			},
			Action: ActionContinue,
		}, nil
	}
}

// rewriteMessages replaces message content with filtered content when
// available. For a single-message input the filtered text replaces the first
// message. For multi-message inputs each message is preserved as-is (the LLM
// filtered the concatenated content).
func (m *ModerationProcessor) rewriteMessages(msgs []Message, filtered string) []Message {
	if len(msgs) == 0 {
		return msgs
	}
	result := make([]Message, len(msgs))
	copy(result, msgs)

	if filtered != "" && len(msgs) == 1 {
		result[0] = Message{
			Role:    msgs[0].Role,
			Content: filtered,
			Meta:    msgs[0].Meta,
		}
	} else if filtered != "" {
		// Distribute the filtered text to the first non-empty message.
		for i, msg := range result {
			if msg.Content != "" {
				result[i] = Message{
					Role:    msg.Role,
					Content: filtered,
					Meta:    msg.Meta,
				}
				break
			}
		}
	}

	return result
}
