package extensions

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/skills"
)

var safeProcessorNames = map[string]struct{}{
	// Tier 0 — existing.
	"token_limiter":          {},
	"system_prompt_scrubber": {},
	// Tier 1 — zero deps, zero risk.
	"unicode_normalizer": {},
	"batch_parts":        {},
	// Tier 2 — safe with config.
	"pii_detector":      {},
	"message_selection": {},
	"tool_call_filter":  {},
	"tool_search":       {},
	"skills":            {},
	"skill_search":      {},
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
// cfg.Options.Processors.Enabled is nil (default) or explicitly true.
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
	if cfg.Options == nil || cfg.Options.Processors == nil || (cfg.Options.Processors.Enabled != nil && !*cfg.Options.Processors.Enabled) {
		e.active = false
		return nil
	}

	var procCfg config.ProcessorConfig
	if cfg.Options.Processors.Config != nil {
		procCfg = cfg.Options.Processors.Config
	}

	toolDefs := host.ToolDefs()
	skillDefs := host.SkillDefs()

	runner := buildProcessorRunner(cfg.Options.Processors.List, procCfg, host.Completer(), toolDefs, skillDefs)
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

func buildProcessorRunner(
	list []string,
	cfg config.ProcessorConfig,
	completer ext.TextCompleter,
	toolDefs []processor.ToolDef,
	skillDefs []processor.SkillDef,
) *processor.ProcessorRunner {
	enforceOrdering(list)

	var inputProcessors []processor.Processor
	var outputProcessors []processor.Processor

	for _, name := range list {
		if _, ok := safeProcessorNames[name]; !ok {
			slog.Debug("Skipping unknown or unsafe processor", "name", name)
			continue
		}
		switch name {
		// Tier 0 — existing processors.
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

		// Tier 1 — zero deps, zero risk.
		case "unicode_normalizer":
			inputProcessors = append(inputProcessors, &processor.UnicodeNormalizer{})
			outputProcessors = append(outputProcessors, &processor.UnicodeNormalizer{})
		case "batch_parts":
			outputProcessors = append(outputProcessors, &processor.BatchParts{})

		// Tier 2 — safe with config.
		case "pii_detector":
			pc := perProcessor(cfg, name)
			sensitivity := processor.SensitivityLow
			if pc != nil {
				if s, ok := pc["sensitivity"].(string); ok {
					sensitivity = processor.PIISensitivity(s)
				}
			}
			p := processor.NewPIIDetector(sensitivity, nil)
			inputProcessors = append(inputProcessors, p)
			outputProcessors = append(outputProcessors, p)
		case "message_selection":
			pc := perProcessor(cfg, name)
			ms := &processor.MessageSelection{}
			if pc != nil {
				if v, ok := pc["max_messages"].(float64); ok && v > 0 {
					ms.MaxMessages = int(v)
				}
				if s, ok := pc["strategy"].(string); ok {
					ms.Strategy = s
				}
			}
			inputProcessors = append(inputProcessors, ms)
		case "tool_call_filter":
			pc := perProcessor(cfg, name)
			tcf := &processor.ToolCallFilter{}
			if pc != nil {
				if raw, ok := pc["allow_list"].([]any); ok {
					for _, v := range raw {
						if s, ok := v.(string); ok {
							tcf.AllowList = append(tcf.AllowList, s)
						}
					}
				}
				if raw, ok := pc["deny_list"].([]any); ok {
					for _, v := range raw {
						if s, ok := v.(string); ok {
							tcf.DenyList = append(tcf.DenyList, s)
						}
					}
				}
			}
			outputProcessors = append(outputProcessors, tcf)
		case "tool_search":
			inputProcessors = append(inputProcessors, &processor.ToolSearch{Tools: toolDefs})
		case "skills":
			inputProcessors = append(inputProcessors, &processor.Skills{Skills: skillDefs})
		case "skill_search":
			inputProcessors = append(inputProcessors, &processor.SkillSearch{})
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

// enforceOrdering ensures "skills" appears before "skill_search" in the list.
// If both are present but out of order, it reorders them in place.
func enforceOrdering(list []string) {
	skillsIdx := -1
	skillSearchIdx := -1
	for i, name := range list {
		switch name {
		case "skills":
			skillsIdx = i
		case "skill_search":
			skillSearchIdx = i
		}
	}
	if skillsIdx >= 0 && skillSearchIdx >= 0 && skillsIdx > skillSearchIdx {
		list[skillsIdx], list[skillSearchIdx] = list[skillSearchIdx], list[skillsIdx]
	}
}

// LoadToolDefsFromMD scans a directory for .md files and returns each as a
// ToolDef with the filename (minus .md) as Name and content as Description.
func LoadToolDefsFromMD(toolsDir string) []processor.ToolDef {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil
	}
	var defs []processor.ToolDef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(toolsDir, entry.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		defs = append(defs, processor.ToolDef{
			Name:        name,
			Description: strings.TrimSpace(string(data)),
		})
	}
	return defs
}

// SkillDefsFromManager converts active skills from a skills.Manager into
// processor SkillDef structs.
func SkillDefsFromManager(m *skills.Manager) []processor.SkillDef {
	if m == nil {
		return nil
	}
	active := m.ActiveSkills()
	defs := make([]processor.SkillDef, 0, len(active))
	for _, s := range active {
		defs = append(defs, processor.SkillDef{
			Name:        s.Name,
			Description: s.Description,
			Content:     s.Instructions,
		})
	}
	return defs
}

// perProcessor extracts a named processor's config block from the full config.
func perProcessor(cfg config.ProcessorConfig, name string) config.ProcessorConfig {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg[name]
	if !ok {
		return nil
	}
	sub, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return config.ProcessorConfig(sub)
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
