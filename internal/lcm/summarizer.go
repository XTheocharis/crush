package lcm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// CompressionLevel represents the escalation level for summarization or
// condensation. Higher values produce more aggressive compression.
type CompressionLevel int

const (
	LevelNormal        CompressionLevel = iota // 10/10 detail — full summarization
	LevelExtractive                            // 8/10 detail — extract key sentences
	LevelAggressive                            // 6/10 detail — aggressive compression
	LevelSkeleton                              // 4/10 detail — headers and key terms only
	LevelDeterministic                         // truncation fallback, no LLM call
)

func (l CompressionLevel) String() string {
	switch l {
	case LevelNormal:
		return "normal"
	case LevelExtractive:
		return "extractive"
	case LevelAggressive:
		return "aggressive"
	case LevelSkeleton:
		return "skeleton"
	case LevelDeterministic:
		return "deterministic"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

func summarizePrompt(lvl CompressionLevel) string {
	switch lvl {
	case LevelExtractive:
		return extractiveSummarizeSystemPrompt
	case LevelAggressive:
		return aggressiveSummarizeSystemPrompt
	case LevelSkeleton:
		return skeletonSummarizeSystemPrompt
	default:
		return normalSummarizeSystemPrompt
	}
}

func condensePrompt(lvl CompressionLevel) string {
	switch lvl {
	case LevelExtractive:
		return extractiveCondenseSystemPrompt
	case LevelAggressive:
		return aggressiveCondenseSystemPrompt
	case LevelSkeleton:
		return skeletonCondenseSystemPrompt
	default:
		return normalCondenseSystemPrompt
	}
}

// LLMClient is the interface for calling an LLM.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Summarizer handles LLM-based summarization and condensation.
type Summarizer struct {
	mu  sync.RWMutex
	llm LLMClient
}

// NewSummarizer creates a new Summarizer with the given LLM client.
func NewSummarizer(llm LLMClient) *Summarizer {
	return &Summarizer{llm: llm}
}

// SetLLM updates the LLM client used for summarization and condensation.
func (s *Summarizer) SetLLM(llm LLMClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llm = llm
}

func (s *Summarizer) llmClient() LLMClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.llm
}

// Summarize generates a summary of a set of messages using 5-level escalation.
// Returns the summary text and estimated token count.
func (s *Summarizer) Summarize(ctx context.Context, input SummaryInput) (string, int64, error) {
	llm := s.llmClient()
	if llm == nil {
		fb := s.fallbackSummarize(input)
		return fb, EstimateTokens(fb), nil
	}

	userPrompt := formatMessagesForSummary(input.Messages)
	inputTokens := EstimateTokens(userPrompt)

	result := ""
	tokens := int64(0)

	for lvl := CompressionLevel(0); lvl < LevelDeterministic; lvl++ {
		prompt := summarizePrompt(lvl)
		resp, err := llm.Complete(ctx, prompt, userPrompt)
		if err != nil {
			return "", 0, fmt.Errorf("%s summarization: %w", lvl, err)
		}
		result = resp
		tokens = EstimateTokens(result)
		if tokens < inputTokens {
			return result, tokens, nil
		}
	}

	// Level 4: Deterministic fallback.
	input.SummaryText = result
	result = s.fallbackSummarize(input)
	tokens = EstimateTokens(result)
	return result, tokens, nil
}

// Condense generates a condensed summary from multiple summaries using 5-level
// escalation.
func (s *Summarizer) Condense(ctx context.Context, summaries []ContextEntry) (string, int64, error) {
	userPrompt := formatSummariesForCondensation(summaries)

	llm := s.llmClient()
	if llm == nil {
		fb := truncateToMaxChars(userPrompt)
		return fb, EstimateTokens(fb), nil
	}

	inputTokens := EstimateTokens(userPrompt)

	result := ""
	tokens := int64(0)

	for lvl := CompressionLevel(0); lvl < LevelDeterministic; lvl++ {
		prompt := condensePrompt(lvl)
		resp, err := llm.Complete(ctx, prompt, userPrompt)
		if err != nil {
			return "", 0, fmt.Errorf("%s condensation: %w", lvl, err)
		}
		result = resp
		tokens = EstimateTokens(result)
		if tokens < inputTokens {
			return result, tokens, nil
		}
	}

	// Level 4: Deterministic fallback.
	result = truncateToMaxChars(result)
	tokens = EstimateTokens(result)
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

const normalSummarizeSystemPrompt = `You are a conversation summarizer. Create a concise, accurate summary of the provided messages that preserves all important technical details, decisions, and context. The summary will replace the original messages in the conversation history.

Format your response as plain text. Include:
- Key decisions made
- Important technical details
- Context needed to continue the work
- File paths, function names, and other specific references`

const extractiveSummarizeSystemPrompt = `You are a conversation summarizer. Extract the key sentences from the provided messages while preserving document structure. Target 8/10 detail level — keep the most important sentences verbatim, removing filler and repetition.

Always preserve file paths, function names, and specific references.`

const aggressiveSummarizeSystemPrompt = `You are a conversation summarizer. Create an extremely concise summary of the provided messages. Focus only on the most critical information. Be very brief.

Always preserve file paths and function names mentioned in the messages.`

const skeletonSummarizeSystemPrompt = `You are a conversation summarizer. Reduce the provided messages to essential headers and key terms only. Target 4/10 detail level — output section headers and the most important nouns, verbs, file paths, and identifiers. Omit all prose.

Format as a structured outline.`

const normalCondenseSystemPrompt = `You are a conversation summarizer. Condense the following summaries into a single, concise summary. Preserve all important technical details, decisions, and context.

Format your response as plain text. Include:
- Key decisions made
- Important technical details
- Context needed to continue the work
- File paths, function names, and other specific references`

const extractiveCondenseSystemPrompt = `You are a conversation summarizer. Extract the key sentences from the following summaries while preserving structure. Target 8/10 detail level — keep the most important sentences verbatim, removing filler and repetition.

Always preserve file paths, function names, and specific references.`

const aggressiveCondenseSystemPrompt = `You are a conversation summarizer. Condense the following summaries into an extremely brief summary. Focus only on the most critical information.

Always preserve file paths and function names mentioned in the summaries.`

const skeletonCondenseSystemPrompt = `You are a conversation summarizer. Reduce the following summaries to essential headers and key terms only. Target 4/10 detail level — output section headers and the most important nouns, verbs, file paths, and identifiers. Omit all prose.

Format as a structured outline.`

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
