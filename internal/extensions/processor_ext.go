package extensions

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/processor"
)

var safeProcessorNames = map[string]struct{}{
	"token_limiter":          {},
	"system_prompt_scrubber": {},
}

const defaultTokenBudget = 200000

type completerAdapter struct {
	fn ext.TextCompleter
}

func (a *completerAdapter) Complete(ctx context.Context, prompt, input string) (string, error) {
	return a.fn(ctx, prompt, input)
}

// ProcessorExtension wires the processor pipeline into the extension host as
// both a StepHookProvider and RunHookProvider. Config-gated: only active when
// cfg.Options.Processors.Enabled is true.
type ProcessorExtension struct {
	mu     sync.RWMutex
	host   ext.HostContext
	runner *processor.ProcessorRunner
	active bool
}

func (e *ProcessorExtension) Name() string { return "processor" }

func (e *ProcessorExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	cfg := host.Config()
	if cfg.Options == nil || cfg.Options.Processors == nil || !cfg.Options.Processors.Enabled {
		e.active = false
		return nil
	}

	runner := buildProcessorRunner(cfg.Options.Processors.List, host.Completer())
	if runner == nil {
		e.active = false
		return nil
	}

	e.runner = runner
	e.active = true
	return nil
}

func (e *ProcessorExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runner = nil
	e.active = false
	return nil
}

func (e *ProcessorExtension) StepHooks() []ext.StepHook {
	e.mu.RLock()
	active := e.active
	e.mu.RUnlock()
	if !active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "processor:input",
			OnPrepareStep: func(ctx context.Context, sessionID string, messages []fantasy.Message) ([]fantasy.Message, error) {
				return e.processInput(ctx, sessionID, messages)
			},
		},
		{
			Name: "processor:output",
			OnStepFinish: func(ctx context.Context, sessionID string, step fantasy.StepResult) error {
				e.processOutput(ctx, sessionID, step)
				return nil
			},
		},
	}
}

func (e *ProcessorExtension) RunHooks() []ext.RunHook {
	e.mu.RLock()
	active := e.active
	e.mu.RUnlock()
	if !active {
		return nil
	}
	return []ext.RunHook{
		{
			Name: "processor:run-start",
			OnRunStart: func(ctx context.Context, sessionID string, userMessage string) error {
				return e.processRunStart(ctx, sessionID, userMessage)
			},
			OnRunEnd: func(_ context.Context, _ string, _ *fantasy.AgentResult, _ error) error {
				return nil
			},
		},
	}
}

func (e *ProcessorExtension) processInput(ctx context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
	e.mu.RLock()
	runner := e.runner
	e.mu.RUnlock()
	if runner == nil {
		return messages, nil
	}

	pmsgs := fantasyToProcessorMessages(messages)
	pctx := processor.ProcessorContext{
		Phase:    processor.InputPhase,
		Messages: pmsgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	result, err := runner.Execute(ctx, processor.InputPhase, pctx)
	if err != nil {
		slog.Debug("Processor input phase failed", "error", err)
		return messages, nil
	}

	// Preserve original fantasy messages to keep rich content (tool
	// calls, tool results, reasoning) that the flat processor.Message
	// type cannot represent. Do NOT use processorToFantasyMessages here
	// — it strips all non-text parts.
	if len(result.Messages) == len(messages) {
		return messages, nil
	}

	removed := len(messages) - len(result.Messages)
	if removed > 0 {
		if removed >= len(messages) {
			removed = len(messages) - 1
		}
		return messages[removed:], nil
	}

	return messages, nil
}

func (e *ProcessorExtension) processOutput(ctx context.Context, _ string, step fantasy.StepResult) {
	e.mu.RLock()
	runner := e.runner
	e.mu.RUnlock()
	if runner == nil {
		return
	}

	text := extractStepText(step)
	if text == "" {
		return
	}

	pmsgs := []processor.Message{
		{Role: "assistant", Content: text},
	}
	pctx := processor.ProcessorContext{
		Phase:        processor.OutputResultPhase,
		OutputResult: text,
		Messages:     pmsgs,
		State:        make(map[string]any),
		Metadata:     make(map[string]any),
	}

	_, err := runner.Execute(ctx, processor.OutputResultPhase, pctx)
	if err != nil {
		slog.Debug("Processor output phase failed", "error", err)
	}
}

func (e *ProcessorExtension) processRunStart(ctx context.Context, _ string, userMessage string) error {
	e.mu.RLock()
	runner := e.runner
	e.mu.RUnlock()
	if runner == nil {
		return nil
	}

	pmsgs := []processor.Message{
		{Role: "user", Content: userMessage},
	}
	pctx := processor.ProcessorContext{
		Phase:    processor.InputPhase,
		Input:    userMessage,
		Messages: pmsgs,
		State:    make(map[string]any),
		Metadata: make(map[string]any),
	}

	_, err := runner.Execute(ctx, processor.InputPhase, pctx)
	if err != nil {
		slog.Debug("Processor run-start phase failed", "error", err)
	}
	return nil
}

func buildProcessorRunner(list []string, completer ext.TextCompleter) *processor.ProcessorRunner {
	var inputProcessors []processor.Processor
	var outputProcessors []processor.Processor

	for _, name := range list {
		if _, ok := safeProcessorNames[name]; !ok {
			slog.Debug("Skipping unknown or unsafe processor", "name", name)
			continue
		}
		switch name {
		case "token_limiter":
			inputProcessors = append(inputProcessors, &processor.TokenLimiter{
				Budget: defaultTokenBudget,
			})
		case "system_prompt_scrubber":
			if completer == nil {
				slog.Debug("Skipping system_prompt_scrubber: no text completer available")
				continue
			}
			outputProcessors = append(outputProcessors, processor.NewSystemPromptScrubber(&completerAdapter{fn: completer}))
		}
	}

	if len(inputProcessors) == 0 && len(outputProcessors) == 0 {
		return nil
	}

	opts := []processor.RunnerOption{}
	if len(inputProcessors) > 0 {
		opts = append(opts, processor.WithInputProcessors(inputProcessors...))
	}
	if len(outputProcessors) > 0 {
		opts = append(opts, processor.WithOutputProcessors(outputProcessors...))
	}

	return processor.NewRunner(opts...)
}

// ---------------------------------------------------------------------------
// Message conversion helpers
// ---------------------------------------------------------------------------

func fantasyToProcessorMessages(msgs []fantasy.Message) []processor.Message {
	result := make([]processor.Message, 0, len(msgs))
	for _, msg := range msgs {
		text := extractFantasyMessageText(msg)
		meta := make(map[string]any)
		result = append(result, processor.Message{
			Role:    string(msg.Role),
			Content: text,
			Meta:    meta,
		})
	}
	return result
}

func processorToFantasyMessages(msgs []processor.Message) []fantasy.Message {
	result := make([]fantasy.Message, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, fantasy.Message{
			Role: fantasy.MessageRole(msg.Role),
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: msg.Content},
			},
		})
	}
	return result
}

func extractFantasyMessageText(msg fantasy.Message) string {
	var texts []string
	for _, part := range msg.Content {
		if tp, ok := fantasy.AsContentType[fantasy.TextPart](part); ok {
			texts = append(texts, tp.Text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	if len(texts) == 1 {
		return texts[0]
	}
	var combined strings.Builder
	combined.WriteString(texts[0])
	for _, t := range texts[1:] {
		combined.WriteString("\n" + t)
	}
	return combined.String()
}

func extractStepText(step fantasy.StepResult) string {
	if len(step.Messages) == 0 {
		return ""
	}
	// Use the last assistant message as the step output.
	for _, v := range slices.Backward(step.Messages) {
		if v.Role == fantasy.MessageRoleAssistant {
			return extractFantasyMessageText(v)
		}
	}
	return extractFantasyMessageText(step.Messages[len(step.Messages)-1])
}

var (
	_ ext.Extension        = (*ProcessorExtension)(nil)
	_ ext.StepHookProvider = (*ProcessorExtension)(nil)
	_ ext.RunHookProvider  = (*ProcessorExtension)(nil)
)
