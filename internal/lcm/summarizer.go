package lcm

import (
	"context"
	"fmt"
	"strings"
)

// LLMClient is the interface for calling an LLM.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Summarizer handles LLM-based summarization and condensation.
type Summarizer struct {
	llm LLMClient
}

// NewSummarizer creates a new Summarizer with the given LLM client.
func NewSummarizer(llm LLMClient) *Summarizer {
	return &Summarizer{llm: llm}
}

// Summarize generates a summary of a set of messages.
// Returns the summary text and estimated token count.
func (s *Summarizer) Summarize(ctx context.Context, input SummaryInput) (string, int64, error) {
	if s.llm == nil {
		fb := s.fallbackSummarize(input)
		return fb, EstimateTokens(fb), nil
	}

	// Level 1: Normal summarization.
	userPrompt := formatMessagesForSummary(input.Messages)
	result, err := s.llm.Complete(ctx, normalSummarizeSystemPrompt, userPrompt)
	if err != nil {
		return "", 0, fmt.Errorf("normal summarization: %w", err)
	}
	tokens := EstimateTokens(result)
	inputTokens := EstimateTokens(userPrompt)

	// Level 2: Aggressive summarization if result too large.
	if tokens >= inputTokens {
		result, err = s.llm.Complete(ctx, aggressiveSummarizeSystemPrompt, userPrompt)
		if err != nil {
			return "", 0, fmt.Errorf("aggressive summarization: %w", err)
		}
		tokens = EstimateTokens(result)
	}

	// Level 3: Deterministic fallback.
	if tokens >= inputTokens {
		input.SummaryText = result
		result = s.fallbackSummarize(input)
		tokens = EstimateTokens(result)
	}

	return result, tokens, nil
}

// Condense generates a condensed summary from multiple summaries.
// Uses the same three-level escalation as Summarize.
func (s *Summarizer) Condense(ctx context.Context, summaries []ContextEntry) (string, int64, error) {
	userPrompt := formatSummariesForCondensation(summaries)

	if s.llm == nil {
		fb := truncateToMaxChars(userPrompt)
		return fb, EstimateTokens(fb), nil
	}

	// Level 1: Normal condensation.
	result, err := s.llm.Complete(ctx, normalCondenseSystemPrompt, userPrompt)
	if err != nil {
		return "", 0, fmt.Errorf("normal condensation: %w", err)
	}
	tokens := EstimateTokens(result)
	inputTokens := EstimateTokens(userPrompt)

	// Level 2: Aggressive condensation.
	if tokens >= inputTokens {
		result, err = s.llm.Complete(ctx, aggressiveCondenseSystemPrompt, userPrompt)
		if err != nil {
			return "", 0, fmt.Errorf("aggressive condensation: %w", err)
		}
		tokens = EstimateTokens(result)
	}

	// Level 3: Deterministic fallback.
	if tokens >= inputTokens {
		result = truncateToMaxChars(result)
		tokens = EstimateTokens(result)
	}

	return result, tokens, nil
}

func (s *Summarizer) fallbackSummarize(input SummaryInput) string {
	text := input.SummaryText
	if text == "" && len(input.Messages) > 0 {
		var sb strings.Builder
		for _, m := range input.Messages {
			sb.WriteString(m.Content)
			sb.WriteString("\n")
		}
		text = sb.String()
	}
	return truncateToMaxChars(text)
}

// truncateToMaxChars truncates text to FallbackMaxChars.
func truncateToMaxChars(text string) string {
	if len(text) > FallbackMaxChars {
		text = text[:FallbackMaxChars]
	}
	return text
}

// Summary prompt templates.
const normalSummarizeSystemPrompt = `You are a conversation summarizer. Create a concise, accurate summary of the provided messages that preserves all important technical details, decisions, and context. The summary will replace the original messages in the conversation history.

Format your response as plain text. Include:
- Key decisions made
- Important technical details
- Context needed to continue the work
- File paths, function names, and other specific references`

const aggressiveSummarizeSystemPrompt = `You are a conversation summarizer. Create an extremely concise summary of the provided messages. Focus only on the most critical information. Be very brief.`

const normalCondenseSystemPrompt = `You are a conversation summarizer. Condense the following summaries into a single, concise summary. Preserve all important technical details, decisions, and context.

Format your response as plain text. Include:
- Key decisions made
- Important technical details
- Context needed to continue the work
- File paths, function names, and other specific references`

const aggressiveCondenseSystemPrompt = `You are a conversation summarizer. Condense the following summaries into an extremely brief summary. Focus only on the most critical information.`

func formatMessagesForSummary(messages []MessageForSummary) string {
	var sb strings.Builder
	sb.WriteString("<messages>\n")
	for _, m := range messages {
		fmt.Fprintf(&sb, "--- Message (seq: %d, role: %s) ---\n%s\n\n", m.Seq, m.Role, m.Content)
	}
	sb.WriteString("</messages>")
	return sb.String()
}

func formatSummariesForCondensation(summaries []ContextEntry) string {
	var sb strings.Builder
	sb.WriteString("<summaries>\n")
	for _, s := range summaries {
		fmt.Fprintf(&sb, "--- Summary (id: %s, kind: %s) ---\n%s\n\n", s.SummaryID, s.SummaryKind, s.SummaryContent)
	}
	sb.WriteString("</summaries>")
	return sb.String()
}
