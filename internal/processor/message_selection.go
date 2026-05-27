package processor

import (
	"context"
)

// Compile-time interface check.
var _ Processor = (*MessageSelection)(nil)

// MessageSelection filters messages to fit within a count budget. It supports
// two strategies: "recency" keeps the most recent messages, while "relevance"
// prioritizes user and assistant messages and drops tool messages first.
type MessageSelection struct {
	// MaxMessages is the maximum number of messages to keep. Zero means no
	// filtering is applied.
	MaxMessages int
	// Strategy controls how messages are selected: "recency" or "relevance".
	Strategy string
}

// ID returns the processor identifier.
func (ms *MessageSelection) ID() string {
	return "message_selection"
}

// ProcessInput selects messages based on the configured budget and strategy.
// When MaxMessages is zero or the message count fits within budget, it returns
// the messages unchanged. Otherwise it applies the selection strategy and
// records stats in the result State.
func (ms *MessageSelection) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	msgs := pctx.Messages
	before := len(msgs)

	// No filtering needed.
	if ms.MaxMessages == 0 || before <= ms.MaxMessages {
		return ProcessorResult{Action: ActionContinue, Messages: msgs}, nil
	}

	// Find index of the first user message for preservation.
	firstUserIdx := -1
	for i, m := range msgs {
		if m.Role == "user" {
			firstUserIdx = i
			break
		}
	}

	var selected []Message
	var removedRoles map[string]int

	switch ms.Strategy {
	case "relevance":
		selected, removedRoles = ms.selectByRelevance(msgs, firstUserIdx)
	default:
		// Default to recency strategy.
		selected, removedRoles = ms.selectByRecency(msgs, firstUserIdx)
	}

	return ProcessorResult{
		Action:   ActionContinue,
		Messages: selected,
		State: map[string]any{
			"messages_before": before,
			"messages_after":  len(selected),
			"strategy":        ms.effectiveStrategy(),
			"removed_roles":   removedRoles,
		},
	}, nil
}

// ProcessOutputStream passes through with ActionContinue.
func (ms *MessageSelection) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputResult passes through with ActionContinue.
func (ms *MessageSelection) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessAPIError passes through with ActionContinue.
func (ms *MessageSelection) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// effectiveStrategy returns the strategy name used for state reporting.
func (ms *MessageSelection) effectiveStrategy() string {
	if ms.Strategy == "relevance" {
		return "relevance"
	}
	return "recency"
}

// selectByRecency keeps the last MaxMessages messages, always preserving the
// first user message if it would otherwise be dropped.
func (ms *MessageSelection) selectByRecency(msgs []Message, firstUserIdx int) ([]Message, map[string]int) {
	removedRoles := make(map[string]int)
	budget := ms.MaxMessages

	// Start with the most recent messages.
	selected := make([]Message, 0, budget)
	start := len(msgs) - budget
	if start < 0 {
		start = 0
	}
	selected = append(selected, msgs[start:]...)

	hasUserInWindow := false
	for _, m := range selected {
		if m.Role == "user" {
			hasUserInWindow = true
			break
		}
	}
	if firstUserIdx >= 0 && firstUserIdx < start && !hasUserInWindow {
		dropped := selected[0]
		removedRoles[dropped.Role]++
		selected = selected[1:]
		selected = append([]Message{msgs[firstUserIdx]}, selected...)
	}

	// Count removed messages by role.
	for i := range start {
		if firstUserIdx >= 0 && i == firstUserIdx {
			continue // This one was preserved.
		}
		removedRoles[msgs[i].Role]++
	}

	return selected, removedRoles
}

// selectByRelevance prioritizes user messages, then assistant messages, then
// tool results. Tool-use messages are dropped first.
func (ms *MessageSelection) selectByRelevance(msgs []Message, firstUserIdx int) ([]Message, map[string]int) {
	removedRoles := make(map[string]int)
	budget := ms.MaxMessages

	// Priority order for dropping: tool_use first, then tool_result, then
	// assistant, then user (user is never dropped except when all are user).
	rolePriority := map[string]int{
		"tool_use":    0, // Drop first.
		"tool_result": 1,
		"assistant":   2,
		"user":        3, // Drop last.
	}

	// Build list of removable indices sorted by priority (ascending = drop
	// first).
	type idxEntry struct {
		idx      int
		priority int
	}

	removable := make([]idxEntry, 0, len(msgs))
	for i, m := range msgs {
		// Never remove the first user message.
		if i == firstUserIdx {
			continue
		}
		p, ok := rolePriority[m.Role]
		if !ok {
			p = 0 // Unknown roles get highest drop priority.
		}
		removable = append(removable, idxEntry{idx: i, priority: p})
	}

	// Sort removable by priority ascending (drop low priority first), then by
	// index ascending (older messages first within same priority).
	for i := 1; i < len(removable); i++ {
		for j := i; j > 0; j-- {
			a, b := removable[j-1], removable[j]
			if a.priority > b.priority || (a.priority == b.priority && a.idx > b.idx) {
				removable[j-1], removable[j] = removable[j], removable[j-1]
			}
		}
	}

	toRemove := len(msgs) - budget
	removeSet := make(map[int]bool, toRemove)
	for i := 0; i < toRemove && i < len(removable); i++ {
		removeSet[removable[i].idx] = true
		removedRoles[msgs[removable[i].idx].Role]++
	}

	// Build the selected slice preserving original order.
	selected := make([]Message, 0, budget)
	for i, m := range msgs {
		if !removeSet[i] {
			selected = append(selected, m)
		}
	}

	return selected, removedRoles
}
