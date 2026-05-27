package processor

import "context"

// Compile-time interface check.
var _ Processor = (*BatchParts)(nil)

// BatchParts merges adjacent messages with the same Role into a single message,
// concatenating their content with newlines and merging Meta maps (later values
// win). This is pure computation — no LLM calls are required.
type BatchParts struct{}

func (BatchParts) ID() string { return "batch_parts" }

// ProcessInput passes messages through unchanged.
func (BatchParts) ProcessInput(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Messages: pctx.Messages, Action: ActionContinue}, nil
}

// ProcessOutputStream merges adjacent same-role messages and records batch
// stats in State.
func (BatchParts) ProcessOutputStream(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	merged, merges := batchMessages(pctx.Messages)
	return ProcessorResult{
		Messages: merged,
		Action:   ActionContinue,
		State: map[string]any{
			"messages_before":  len(pctx.Messages),
			"messages_after":   len(merged),
			"merges_performed": merges,
		},
	}, nil
}

// ProcessOutputResult merges adjacent same-role messages and records batch
// stats in State.
func (BatchParts) ProcessOutputResult(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	merged, merges := batchMessages(pctx.Messages)
	return ProcessorResult{
		Messages: merged,
		Action:   ActionContinue,
		State: map[string]any{
			"messages_before":  len(pctx.Messages),
			"messages_after":   len(merged),
			"merges_performed": merges,
		},
	}, nil
}

// ProcessAPIError passes messages through unchanged.
func (BatchParts) ProcessAPIError(_ context.Context, pctx ProcessorContext) (ProcessorResult, error) {
	return ProcessorResult{Messages: pctx.Messages, Action: ActionContinue}, nil
}

// batchMessages merges adjacent messages with the same Role. Content is joined
// with a newline. Meta maps are merged with later values winning. Returns the
// merged slice and the number of merges performed.
func batchMessages(msgs []Message) ([]Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}

	out := make([]Message, 0, len(msgs))
	merges := 0

	cur := Message{
		Role:    msgs[0].Role,
		Content: msgs[0].Content,
	}
	if len(msgs[0].Meta) > 0 {
		cur.Meta = make(map[string]any, len(msgs[0].Meta))
		for k, v := range msgs[0].Meta {
			cur.Meta[k] = v
		}
	}

	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == cur.Role {
			cur.Content = cur.Content + "\n" + msgs[i].Content
			if len(msgs[i].Meta) > 0 {
				if cur.Meta == nil {
					cur.Meta = make(map[string]any)
				}
				for k, v := range msgs[i].Meta {
					cur.Meta[k] = v
				}
			}
			merges++
		} else {
			out = append(out, cur)
			cur = Message{
				Role:    msgs[i].Role,
				Content: msgs[i].Content,
			}
			if len(msgs[i].Meta) > 0 {
				cur.Meta = make(map[string]any, len(msgs[i].Meta))
				for k, v := range msgs[i].Meta {
					cur.Meta[k] = v
				}
			}
		}
	}
	out = append(out, cur)

	return out, merges
}
