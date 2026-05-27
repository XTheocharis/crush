package processor

import (
	"context"
)

// TokenLimiter enforces a token budget on input messages using a simple
// chars-per-token heuristic (~4 chars/token). When the total exceeds the
// budget, oldest messages are removed first; if a single message still
// exceeds the budget its content is truncated.
type TokenLimiter struct {
	Budget int
}

func (tl *TokenLimiter) ID() string { return "token_limiter" }

func (tl *TokenLimiter) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	msgs := pctx.Messages
	totalBefore := countTokens(msgs)

	if totalBefore <= tl.Budget {
		return ProcessorResult{Action: ActionContinue, Messages: msgs}, nil
	}

	trimmed := msgs
	removed := 0
	for len(trimmed) > 1 && countTokens(trimmed) > tl.Budget {
		trimmed = trimmed[1:]
		removed++
	}

	if len(trimmed) == 1 && countTokens(trimmed) > tl.Budget {
		maxChars := max(tl.Budget*charsPerToken, 0)
		trimmed[0] = Message{
			Role:    trimmed[0].Role,
			Content: truncateString(trimmed[0].Content, maxChars),
			Meta:    trimmed[0].Meta,
		}
	}

	totalAfter := countTokens(trimmed)

	return ProcessorResult{
		Action:   ActionContinue,
		Messages: trimmed,
		State: map[string]any{
			"tokens_before":    totalBefore,
			"tokens_after":     totalAfter,
			"messages_removed": removed,
		},
	}, nil
}

func (tl *TokenLimiter) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

func (tl *TokenLimiter) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

func (tl *TokenLimiter) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

const charsPerToken = 4

func countTokens(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content) / charsPerToken
	}
	return total
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
