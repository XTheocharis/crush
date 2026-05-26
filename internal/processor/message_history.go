package processor

import "context"

// Compile-time interface check.
var _ Processor = (*MessageHistory)(nil)

// MessageStore persists messages to storage. Production implementations back
// this with SQLite; tests use InMemoryStore.
type MessageStore interface {
	// SaveMessages persists the given messages.
	SaveMessages(ctx context.Context, messages []Message) error
	// LoadMessages retrieves all previously saved messages.
	LoadMessages(ctx context.Context) ([]Message, error)
}

// InMemoryStore is a simple slice-backed MessageStore for testing.
type InMemoryStore struct {
	Messages []Message
}

// SaveMessages replaces the store contents with the provided slice.
func (s *InMemoryStore) SaveMessages(_ context.Context, messages []Message) error {
	s.Messages = make([]Message, len(messages))
	copy(s.Messages, messages)
	return nil
}

// LoadMessages returns the current store contents.
func (s *InMemoryStore) LoadMessages(_ context.Context) ([]Message, error) {
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out, nil
}

// MessageHistory records input and output messages via a MessageStore. It acts
// as an input/output processor — saving messages on the way in and on the way
// out — and passes them through unchanged.
type MessageHistory struct {
	Store MessageStore
}

// ID returns the processor identifier.
func (h *MessageHistory) ID() string { return "message_history" }

// ProcessInput saves the current input messages to the store and returns them
// unchanged with ActionContinue.
func (h *MessageHistory) ProcessInput(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	if err := h.Store.SaveMessages(ctx, pctx.Messages); err != nil {
		return ProcessorResult{}, err
	}
	return ProcessorResult{
		Action:   ActionContinue,
		Messages: pctx.Messages,
		State: map[string]any{
			"input_messages_saved": len(pctx.Messages),
			"history_action":       "save_input",
		},
	}, nil
}

// ProcessOutputStream passes through with ActionContinue. Message history is
// recorded during the input and output result phases only.
func (h *MessageHistory) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}

// ProcessOutputResult saves the output messages to the store and returns them
// unchanged with ActionContinue.
func (h *MessageHistory) ProcessOutputResult(ctx context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	if err := h.Store.SaveMessages(ctx, pctx.Messages); err != nil {
		return ProcessorResult{}, err
	}
	return ProcessorResult{
		Action:   ActionContinue,
		Messages: pctx.Messages,
		State: map[string]any{
			"output_messages_saved": len(pctx.Messages),
			"history_action":        "save_output",
		},
	}, nil
}

// ProcessAPIError passes through with ActionContinue.
func (h *MessageHistory) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Action: ActionContinue, Messages: pctx.Messages}, nil
}
