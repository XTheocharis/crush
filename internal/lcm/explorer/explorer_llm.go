package explorer

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// llmTruncateMax is the maximum character count for content sent to O19a
// single-call LLM summarization.
const llmTruncateMax = 50_000

// llmTruncateHead is the number of leading characters kept when content exceeds
// llmTruncateMax. The remaining budget goes to the tail.
const llmTruncateHead = 40_000

// LLMClient is the interface for calling an LLM. It mirrors lcm.LLMClient but
// is declared here to avoid a circular import between the explorer and lcm
// packages.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// AgentFunc is a callback that runs an agent-based exploration. The caller
// provides the file path and a combined prompt (system + language-specific
// instructions). The function returns the agent's summary text.
type AgentFunc func(ctx context.Context, path, systemPrompt, userPrompt string) (string, error)

// NewRegistryWithLLM creates a registry with all built-in explorers and an LLM
// client for enhanced summarization. If agentFn is non-nil, agent-based
// exploration (tier 3) is available.
func NewRegistryWithLLM(llm LLMClient, agentFn AgentFunc) *Registry {
	r := NewRegistry()
	r.llm = llm
	r.agentFn = agentFn
	return r
}

// generateLLMSummary produces a summary via a single-call LLM request (tier 2,
// O19a). Content is truncated to llmTruncateMax characters by keeping the
// first llmTruncateHead characters and the last (llmTruncateMax -
// llmTruncateHead) characters.
func generateLLMSummary(ctx context.Context, llm LLMClient, path string, content []byte) (string, error) {
	text := truncateForLLM(string(content))
	userPrompt := fmt.Sprintf("File: %s\n\n%s", path, text)

	result, err := llm.Complete(ctx, llmSummarySystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM summary for %s: %w", filepath.Base(path), err)
	}
	return result, nil
}

// generateAgentSummary produces a summary via an agent sub-session (tier 3,
// O19b). A language-specific prompt is selected and the agent is asked to read
// and analyze the file.
func generateAgentSummary(ctx context.Context, agentFn AgentFunc, path, language string) (string, error) {
	langPrompt := getLanguagePrompt(language)
	userPrompt := fmt.Sprintf("Analyze the file at: %s\n\n%s", path, langPrompt)

	result, err := agentFn(ctx, path, exploreSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("agent summary for %s: %w", filepath.Base(path), err)
	}
	return result, nil
}

// truncateForLLM truncates content to llmTruncateMax characters. If the
// content exceeds the limit, the first llmTruncateHead characters and last
// (llmTruncateMax - llmTruncateHead) characters are kept with a marker
// between them.
func truncateForLLM(content string) string {
	runes := []rune(content)
	if len(runes) <= llmTruncateMax {
		return content
	}
	tailLen := llmTruncateMax - llmTruncateHead
	head := string(runes[:llmTruncateHead])
	tail := string(runes[len(runes)-tailLen:])
	return head + "\n\n...[TRUNCATED]...\n\n" + tail
}

// detectLanguage returns the language identifier for a file based on its
// extension and content (shebang). Returns an empty string if unknown.
func detectLanguage(path string, content []byte) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if lang, ok := TEXT_EXTENSIONS[ext]; ok {
		return lang
	}
	// Check for shebang-based detection.
	if lang := detectShebang(content); lang != "" {
		return lang
	}
	return ""
}

// exploreLLMEnhanced implements the three-tier dispatch logic. It is called by
// Registry.Explore after the static explorer has produced a base result.
//
// Tiers:
//  1. Template only — no LLM or agent available; returns the static result as-is.
//  2. O19a single-call LLM — LLM available, no agent function; sends truncated
//     content to the LLM for a richer summary.
//  3. O19b agent-based — agent function available; spawns a sub-agent that can
//     read the file and produce a language-aware summary.
//
// Python exception: Python files skip tier 2 entirely; they go straight from
// tier 1 to tier 3 (if available) because regex-based static exploration
// captures enough structure, while an agent can do significantly better than a
// single-call summary for Python's dynamic nature.
func exploreLLMEnhanced(
	ctx context.Context,
	llm LLMClient,
	agentFn AgentFunc,
	input ExploreInput,
	staticResult ExploreResult,
) ExploreResult {
	language := detectLanguage(input.Path, input.Content)
	isPython := language == "python"

	// Tier 3: agent-based exploration (highest priority when available).
	if agentFn != nil && input.SessionID != "" {
		summary, err := generateAgentSummary(ctx, agentFn, input.Path, language)
		if err != nil {
			slog.Warn("Agent exploration failed, falling back",
				"path", input.Path,
				"error", err,
			)
			// Fall through to tier 2 or tier 1.
		} else {
			return ExploreResult{
				Summary:       summary,
				ExplorerUsed:  staticResult.ExplorerUsed + "+agent",
				TokenEstimate: estimateTokens(summary),
			}
		}
	}

	// Python exception: skip tier 2 (O19a) entirely.
	if isPython {
		return staticResult
	}

	// Tier 2: single-call LLM summary.
	if llm != nil {
		summary, err := generateLLMSummary(ctx, llm, input.Path, input.Content)
		if err != nil {
			slog.Warn("LLM exploration failed, falling back to static",
				"path", input.Path,
				"error", err,
			)
			return staticResult
		}
		return ExploreResult{
			Summary:       summary,
			ExplorerUsed:  staticResult.ExplorerUsed + "+llm",
			TokenEstimate: estimateTokens(summary),
		}
	}

	// Tier 1: static template (fallback).
	return staticResult
}
