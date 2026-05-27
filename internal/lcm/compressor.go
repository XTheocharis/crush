package lcm

import (
	"context"
	"fmt"
	"math"
)

// CompressedOutput holds the result of a compression operation.
type CompressedOutput struct {
	// Content is the compressed text.
	Content string
	// Ratio is the estimated compression ratio (0, 1]. A value of 1.0 means
	// no compression occurred.
	Ratio float64
	// Strategy is the name of the strategy that produced this output.
	Strategy string
}

// CompressionStrategy defines the interface for LLM-based compression
// strategies. Each strategy applies a different semantic transformation to
// the input text using the LLMClient.
type CompressionStrategy interface {
	// Name returns the human-readable name of the strategy.
	Name() string
	// Compress applies the strategy to the input text using the LLM.
	Compress(ctx context.Context, input string) (*CompressedOutput, error)
	// EstimateRatio returns the expected compression ratio for this strategy.
	// The value is always in the range (0, 1].
	EstimateRatio() float64
}

// ---------------------------------------------------------------------------
// RangeCompression
// ---------------------------------------------------------------------------

// RangeCompression extracts structured key-value ranges from tool output or
// other structured text (e.g., "lines 1-10: function foo"). It is most
// effective on large, line-oriented outputs such as file listings or grep
// results.
type RangeCompression struct {
	llm   LLMClient
	ratio float64
}

// NewRangeCompression creates a RangeCompression strategy with the given LLM
// client.
func NewRangeCompression(llm LLMClient) *RangeCompression {
	return &RangeCompression{llm: llm, ratio: 0.3}
}

// Name implements CompressionStrategy.
func (s *RangeCompression) Name() string { return "range" }

// EstimateRatio implements CompressionStrategy.
func (s *RangeCompression) EstimateRatio() float64 { return s.ratio }

// Compress implements CompressionStrategy.
func (s *RangeCompression) Compress(ctx context.Context, input string) (*CompressedOutput, error) {
	if input == "" {
		return &CompressedOutput{Content: "", Ratio: 1.0, Strategy: s.Name()}, nil
	}
	result, err := s.llm.Complete(ctx, rangeCompressionSystemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("range compression: %w", err)
	}
	ratio := compressionRatio(input, result)
	return &CompressedOutput{Content: result, Ratio: ratio, Strategy: s.Name()}, nil
}

const rangeCompressionSystemPrompt = `You are a structured-output compressor. Given structured or line-oriented text, extract key-value ranges that preserve the essential information.

For each logical section, produce a summary line in the form:
  lines N-M: <brief description>

Rules:
- Preserve all file paths, function names, and identifiers.
- Omit boilerplate, blank lines, and repetitive formatting.
- Keep descriptions concise (one line per range).
- If the input is already compact, return it with minimal changes.`

// ---------------------------------------------------------------------------
// MessageCompression
// ---------------------------------------------------------------------------

// MessageCompression compresses conversation messages while preserving
// semantic meaning. It focuses on retaining decisions, technical details, and
// action items while removing filler and verbosity.
type MessageCompression struct {
	llm   LLMClient
	ratio float64
}

// NewMessageCompression creates a MessageCompression strategy with the given
// LLM client.
func NewMessageCompression(llm LLMClient) *MessageCompression {
	return &MessageCompression{llm: llm, ratio: 0.4}
}

// Name implements CompressionStrategy.
func (s *MessageCompression) Name() string { return "message" }

// EstimateRatio implements CompressionStrategy.
func (s *MessageCompression) EstimateRatio() float64 { return s.ratio }

// Compress implements CompressionStrategy.
func (s *MessageCompression) Compress(ctx context.Context, input string) (*CompressedOutput, error) {
	if input == "" {
		return &CompressedOutput{Content: "", Ratio: 1.0, Strategy: s.Name()}, nil
	}
	result, err := s.llm.Complete(ctx, messageCompressionSystemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("message compression: %w", err)
	}
	ratio := compressionRatio(input, result)
	return &CompressedOutput{Content: result, Ratio: ratio, Strategy: s.Name()}, nil
}

const messageCompressionSystemPrompt = `You are a conversation compressor. Compress the provided messages while preserving all semantic content.

Retain:
- Decisions made and their rationale.
- Technical details: file paths, function names, variable names, error messages.
- Action items and next steps.
- Questions asked and answers given.

Remove:
- Filler words, hedging language, and pleasantries.
- Repeated information.
- Verbose explanations when a brief summary suffices.

Output plain compressed text. Do not add commentary.`

// ---------------------------------------------------------------------------
// DedupCompression
// ---------------------------------------------------------------------------

// DedupCompression removes duplicate information across messages or
// summaries. It identifies repeated content and keeps only the most complete
// or most recent occurrence.
type DedupCompression struct {
	llm   LLMClient
	ratio float64
}

// NewDedupCompression creates a DedupCompression strategy with the given LLM
// client.
func NewDedupCompression(llm LLMClient) *DedupCompression {
	return &DedupCompression{llm: llm, ratio: 0.5}
}

// Name implements CompressionStrategy.
func (s *DedupCompression) Name() string { return "dedup" }

// EstimateRatio implements CompressionStrategy.
func (s *DedupCompression) EstimateRatio() float64 { return s.ratio }

// Compress implements CompressionStrategy.
func (s *DedupCompression) Compress(ctx context.Context, input string) (*CompressedOutput, error) {
	if input == "" {
		return &CompressedOutput{Content: "", Ratio: 1.0, Strategy: s.Name()}, nil
	}
	result, err := s.llm.Complete(ctx, dedupCompressionSystemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("dedup compression: %w", err)
	}
	ratio := compressionRatio(input, result)
	return &CompressedOutput{Content: result, Ratio: ratio, Strategy: s.Name()}, nil
}

const dedupCompressionSystemPrompt = `You are a deduplication compressor. Given text that may contain repeated information, produce a version with duplicates removed.

Rules:
- Identify repeated facts, descriptions, or statements.
- Keep the most complete or most recent version of each unique piece of information.
- Preserve all file paths, function names, and identifiers.
- Maintain chronological order where applicable.
- Output plain text with duplicates removed. Do not add commentary.`

// ---------------------------------------------------------------------------
// PurgeErrorsCompression
// ---------------------------------------------------------------------------

// PurgeErrorsCompression removes error output and debugging trails that have
// been resolved. It retains the resolution but drops the noisy error stack
// traces and intermediate failed attempts.
type PurgeErrorsCompression struct {
	llm   LLMClient
	ratio float64
}

// NewPurgeErrorsCompression creates a PurgeErrorsCompression strategy with
// the given LLM client.
func NewPurgeErrorsCompression(llm LLMClient) *PurgeErrorsCompression {
	return &PurgeErrorsCompression{llm: llm, ratio: 0.6}
}

// Name implements CompressionStrategy.
func (s *PurgeErrorsCompression) Name() string { return "purge_errors" }

// EstimateRatio implements CompressionStrategy.
func (s *PurgeErrorsCompression) EstimateRatio() float64 { return s.ratio }

// Compress implements CompressionStrategy.
func (s *PurgeErrorsCompression) Compress(ctx context.Context, input string) (*CompressedOutput, error) {
	if input == "" {
		return &CompressedOutput{Content: "", Ratio: 1.0, Strategy: s.Name()}, nil
	}
	result, err := s.llm.Complete(ctx, purgeErrorsCompressionSystemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("purge-errors compression: %w", err)
	}
	ratio := compressionRatio(input, result)
	return &CompressedOutput{Content: result, Ratio: ratio, Strategy: s.Name()}, nil
}

const purgeErrorsCompressionSystemPrompt = `You are an error-output purger. Given conversation text that may contain error messages, stack traces, and debugging output, produce a cleaned version.

Rules:
- Remove resolved error messages, stack traces, and debugging noise.
- Keep a brief note of what the error was and how it was resolved (e.g., "Fixed: missing import in foo.go").
- Preserve all successful actions, decisions, and context.
- Do not remove errors that appear to be unresolved.
- Output plain cleaned text. Do not add commentary.`

// ---------------------------------------------------------------------------
// ContextLimits
// ---------------------------------------------------------------------------

// ContextLimits describes the token budget constraints for a model's context
// window. It is used by GraduatedPressureSystem to select compression
// strategies as the window fills.
type ContextLimits struct {
	// MaxTokens is the model-specific maximum context window size.
	MaxTokens int
	// ReserveTokens is the number of tokens reserved for system prompts and
	// model responses. These tokens are not available for context.
	ReserveTokens int
	// SummaryBudget is the maximum number of tokens allowed for a single
	// summary produced during compaction.
	SummaryBudget int
}

// AvailableTokens returns the number of tokens available for context after
// subtracting the reserve.
func (cl ContextLimits) AvailableTokens() int {
	avail := cl.MaxTokens - cl.ReserveTokens
	if avail < 0 {
		return 0
	}
	return avail
}

// EffectiveBudget returns the usable token budget, preferring SummaryBudget
// when set. Falls back to AvailableTokens when SummaryBudget is zero.
func (cl ContextLimits) EffectiveBudget() int {
	if cl.SummaryBudget > 0 {
		return cl.SummaryBudget
	}
	return cl.AvailableTokens()
}

// PressureThreshold returns the token count at the given percentage of
// AvailableTokens. The percentage is in [0, 100].
func (cl ContextLimits) PressureThreshold(pct float64) int64 {
	return int64(float64(cl.AvailableTokens()) * pct / 100.0)
}

// DefaultContextLimits returns a ContextLimits with sensible defaults for a
// model with the given max context window. ReserveTokens is set to 20% of
// maxTokens, and SummaryBudget to 25% of the available tokens.
func DefaultContextLimits(maxTokens int) ContextLimits {
	reserve := maxTokens / 5
	available := maxTokens - reserve
	return ContextLimits{
		MaxTokens:     maxTokens,
		ReserveTokens: reserve,
		SummaryBudget: available / 4,
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// compressionRatio computes the ratio of output length to input length,
// clamped to (0, 1].
func compressionRatio(input, output string) float64 {
	if len(input) == 0 {
		return 1.0
	}
	r := float64(len(output)) / float64(len(input))
	// Clamp to (0, 1].
	return math.Min(1.0, math.Max(math.SmallestNonzeroFloat64, r))
}
