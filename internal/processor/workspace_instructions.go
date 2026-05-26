package processor

import "context"

// Compile-time interface check.
var _ Processor = (*WorkspaceInstructions)(nil)

// WorkspaceInstructions injects workspace-specific instructions (e.g. AGENTS.md,
// CRUSH.md content) into the message context as a system message. This is a
// pure injection processor with no LLM dependency.
type WorkspaceInstructions struct {
	// Instructions holds the workspace instructions content to inject.
	Instructions string
}

// ID returns the processor identifier.
func (w *WorkspaceInstructions) ID() string {
	return "workspace_instructions"
}

// ProcessInput prepends workspace instructions as a system message when the
// Instructions field is non-empty. When empty it acts as a no-op.
func (w *WorkspaceInstructions) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	if w.Instructions == "" {
		return ProcessorResult{
			Messages: pctx.Messages,
			State: map[string]any{
				"instructions_length": 0,
				"injected":            false,
			},
			Action: ActionContinue,
		}, nil
	}

	systemMsg := Message{Role: "system", Content: w.Instructions}
	msgs := make([]Message, 0, len(pctx.Messages)+1)
	msgs = append(msgs, systemMsg)
	msgs = append(msgs, pctx.Messages...)

	return ProcessorResult{
		Messages: msgs,
		State: map[string]any{
			"instructions_length": len(w.Instructions),
			"injected":            true,
		},
		Action: ActionContinue,
	}, nil
}

// ProcessOutputStream passes through with ActionContinue.
func (w *WorkspaceInstructions) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Messages: pctx.Messages, Action: ActionContinue}, nil
}

// ProcessOutputResult passes through with ActionContinue.
func (w *WorkspaceInstructions) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Messages: pctx.Messages, Action: ActionContinue}, nil
}

// ProcessAPIError passes through with ActionContinue.
func (w *WorkspaceInstructions) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Messages: pctx.Messages, Action: ActionContinue}, nil
}
