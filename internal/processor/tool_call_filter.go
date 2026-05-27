package processor

import "context"

// Compile-time interface check.
var _ Processor = (*ToolCallFilter)(nil)

// ToolCallFilter blocks or allows tool calls based on policy lists. If
// AllowList is non-empty, only tools named in the list pass through. DenyList
// always blocks listed tools regardless of AllowList membership.
type ToolCallFilter struct {
	// AllowList, if non-empty, restricts allowed tools to only those named.
	AllowList []string
	// DenyList blocks these tools unconditionally.
	DenyList []string
}

// ID returns the processor identifier.
func (f *ToolCallFilter) ID() string { return "tool_call_filter" }

// ProcessInput passes through with ActionContinue (filtering happens on the
// output phase).
func (f *ToolCallFilter) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputStream filters tool_use and matching tool_result messages from
// the output stream.
func (f *ToolCallFilter) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return f.filterToolCalls(pctx), nil
}

// ProcessOutputResult filters tool_use and matching tool_result messages from
// the output result.
func (f *ToolCallFilter) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return f.filterToolCalls(pctx), nil
}

// ProcessAPIError passes through with ActionContinue (filtering is on the
// output phase).
func (f *ToolCallFilter) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// filterToolCalls removes blocked tool_use messages and their corresponding
// tool_result messages, returning updated messages and filter statistics in
// State.
func (f *ToolCallFilter) filterToolCalls(pctx ProcessorContext) ProcessorResult {
	allowSet := make(map[string]struct{}, len(f.AllowList))
	for _, name := range f.AllowList {
		allowSet[name] = struct{}{}
	}
	denySet := make(map[string]struct{}, len(f.DenyList))
	for _, name := range f.DenyList {
		denySet[name] = struct{}{}
	}

	// First pass: identify blocked tool_use IDs.
	blockedIDs := make(map[string]bool)
	allowed := 0
	blocked := 0
	blockedNames := make(map[string]bool)

	for _, msg := range pctx.Messages {
		if msg.Role != "tool_use" {
			continue
		}
		name, _ := msg.Meta["name"].(string)
		if f.isBlocked(name, allowSet, denySet) {
			id, _ := msg.Meta["id"].(string)
			blockedIDs[id] = true
			blockedNames[name] = true
			blocked++
		} else {
			allowed++
		}
	}

	// Second pass: keep messages that are not blocked tool_use and not
	// orphaned tool_result references.
	filtered := make([]Message, 0, len(pctx.Messages))
	for _, msg := range pctx.Messages {
		if msg.Role == "tool_use" {
			id, _ := msg.Meta["id"].(string)
			if blockedIDs[id] {
				continue
			}
		}
		if msg.Role == "tool_result" {
			tuid, _ := msg.Meta["tool_use_id"].(string)
			if blockedIDs[tuid] {
				continue
			}
		}
		filtered = append(filtered, msg)
	}

	// Build sorted blocked names for deterministic state.
	names := make([]string, 0, len(blockedNames))
	for n := range blockedNames {
		names = append(names, n)
	}

	state := map[string]any{
		"tools_allowed": allowed,
		"tools_blocked": blocked,
		"blocked_names": names,
		"filter_id":     f.ID(),
	}

	return ProcessorResult{
		Action:   ActionContinue,
		Messages: filtered,
		State:    state,
	}
}

// isBlocked returns true if the tool name should be blocked. AllowList takes
// precedence: when non-empty, only listed tools pass. DenyList then blocks
// within the allowed set.
func (f *ToolCallFilter) isBlocked(name string, allowSet, denySet map[string]struct{}) bool {
	if len(allowSet) > 0 {
		if _, ok := allowSet[name]; !ok {
			return true
		}
	}
	_, denied := denySet[name]
	return denied
}
