package agent

import (
	"strings"

	"github.com/charmbracelet/crush/internal/message"
)

// ComplexityLevel represents the estimated complexity of a conversation or
// task. Higher complexity signals should route to higher-tier models.
type ComplexityLevel int

const (
	// ComplexitySimple indicates a single, focused task such as a small
	// single-file edit or a short question.
	ComplexitySimple ComplexityLevel = iota

	// ComplexityMedium indicates a moderate task such as editing a few files
	// or answering a multi-part question.
	ComplexityMedium

	// ComplexityComplex indicates a demanding task such as a multi-file
	// refactor, architectural decision, or a conversation with many tool
	// calls and planning steps.
	ComplexityComplex
)

// String returns a human-readable name for the complexity level.
func (c ComplexityLevel) String() string {
	switch c {
	case ComplexitySimple:
		return "simple"
	case ComplexityMedium:
		return "medium"
	case ComplexityComplex:
		return "complex"
	default:
		return "unknown"
	}
}

// NumericPriority returns a float64 priority score for the complexity level,
// following the project convention of float64 numeric priorities (e.g.
// PriorityCritical=0.9, PriorityHigh=0.7).
func (c ComplexityLevel) NumericPriority() float64 {
	switch c {
	case ComplexitySimple:
		return 0.15
	case ComplexityMedium:
		return 0.4
	case ComplexityComplex:
		return 0.7
	default:
		return 0.15
	}
}

// ComplexitySignals holds the heuristic signals used to classify task
// complexity. Each field contributes to the overall complexity score.
type ComplexitySignals struct {
	ToolCallCount     int
	TokenCount        int
	HasPlanningTools  bool
	DistinctToolCount int
}

// ClassifyComplexity analyzes a conversation's messages and returns an
// estimated complexity level using heuristic signals. It examines the number
// of tool calls, the token count, whether planning tools were used, and the
// variety of tools invoked.
func ClassifyComplexity(messages []message.Message) ComplexityLevel {
	signals := extractSignals(messages)
	return classifyFromSignals(signals)
}

// ClassifyComplexityFromSignals classifies complexity from pre-extracted
// signals, useful when signals are already available from the routing path.
func ClassifyComplexityFromSignals(signals ComplexitySignals) ComplexityLevel {
	return classifyFromSignals(signals)
}

// extractSignals walks the conversation and collects heuristic signals.
func extractSignals(messages []message.Message) ComplexitySignals {
	var signals ComplexitySignals
	seenTools := make(map[string]struct{})
	totalChars := 0

	for i := range messages {
		msg := &messages[i]
		if msg.Role == message.Assistant {
			for _, tc := range msg.ToolCalls() {
				signals.ToolCallCount++
				seenTools[tc.Name] = struct{}{}
				if isPlanningTool(tc.Name) {
					signals.HasPlanningTools = true
				}
			}
		}
		for _, part := range msg.Parts {
			if tc, ok := part.(message.TextContent); ok {
				totalChars += len(tc.Text)
			}
		}
	}

	signals.TokenCount = (totalChars + charsPerToken - 1) / charsPerToken
	signals.DistinctToolCount = len(seenTools)
	return signals
}

// Scoring thresholds for classifyFromSignals:
//
//	Planning tools:  present → Complex (early return)
//	Tool call count: 0-2 → +0.0,  3-6 → +0.2,  7+ → +0.4
//	Token count:     <4k → +0.0,  4k-16k → +0.2,  >16k → +0.3
//	Distinct tools:  1-2 → +0.0,  3-4 → +0.1,  5+ → +0.15
//	Final: score <0.15 → Simple, <0.45 → Medium, ≥0.45 → Complex
func classifyFromSignals(s ComplexitySignals) ComplexityLevel {
	score := 0.0

	switch {
	case s.ToolCallCount >= 7:
		score += 0.4
	case s.ToolCallCount >= 3:
		score += 0.2
	}

	switch {
	case s.TokenCount > 16000:
		score += 0.3
	case s.TokenCount > 4000:
		score += 0.2
	}

	if s.HasPlanningTools {
		return ComplexityComplex
	}

	switch {
	case s.DistinctToolCount >= 5:
		score += 0.15
	case s.DistinctToolCount >= 3:
		score += 0.1
	}

	switch {
	case score >= 0.45:
		return ComplexityComplex
	case score >= 0.15:
		return ComplexityMedium
	default:
		return ComplexitySimple
	}
}

// planningToolPrefixes lists tool name prefixes that indicate planning or
// orchestration activity, which are strong signals of task complexity.
var planningToolPrefixes = []string{
	"architect",
	"operator",
	"parallel",
	"swarm",
	"team_",
	"plan",
}

// isPlanningTool returns true when the tool name suggests planning or
// orchestration activity.
func isPlanningTool(toolName string) bool {
	lower := strings.ToLower(toolName)
	for _, prefix := range planningToolPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
