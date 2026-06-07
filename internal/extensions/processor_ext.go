package extensions

import (
	"context"
	"maps"
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
	"unicode_normalizer":     {},
	"workspace_instructions": {},
	"batch_parts":            {},
	// Tier 2 — safe with config.
	"pii_detector":      {},
	"message_selection": {},
	"tool_call_filter":  {},
	"tool_search":       {},
	"skills":            {},
	"skill_search":      {},
	"message_history":   {},
	// Tier 3 — LLM-dependent processors.
	"moderation":        {},
	"prompt_injection":  {},
	"language_detector": {},
}

const defaultTokenBudget = 200000

// defaultProcessors lists the processors that activate without any config.
var defaultProcessors = []string{"token_limiter", "system_prompt_scrubber", "pii_detector"}

type completerAdapter struct {
	fn ext.TextCompleter
}

func (a *completerAdapter) Complete(ctx context.Context, prompt, input string) (string, error) {
	return a.fn(ctx, prompt, input)
}

// TheProcessorExtension is the singleton processor extension instance
// registered at init. Follows the same pattern as TheLCMExtension.
var TheProcessorExtension = &ProcessorExtension{}

// ProcessorStateSnapshot is an immutable snapshot of the processor pipeline
// state at a point in time. Safe to read concurrently without locking.
type ProcessorStateSnapshot struct {
	// Active indicates whether the processor pipeline is enabled.
	Active bool
	// LastPhase is the most recent processor phase that was executed.
	// Empty string if no phase has run yet.
	LastPhase processor.ProcessorPhase
	// ProcessorNames lists the IDs of all configured processors.
	ProcessorNames []string
	// TokenBudget is the configured token budget (0 if unset).
	TokenBudget int
	// LastState holds the accumulated processor state from the last
	// pipeline execution. Nil if no execution has occurred.
	LastState map[string]any
}

// ProcessorExtension wires the processor pipeline into the extension host as
// both a StepHookProvider and RunHookProvider. Config-gated: only active when
// cfg.Options.Processors.Enabled is nil (default) or explicitly true.
type ProcessorExtension struct {
	mu         sync.RWMutex
	host       ext.HostContext
	runner     *processor.ProcessorRunner
	active     bool
	lastPhase  processor.ProcessorPhase
	lastState  map[string]any
	tokenBudget int
}

func (e *ProcessorExtension) Name() string { return "processor" }

func (e *ProcessorExtension) Init(_ context.Context, host ext.HostContext) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.host = host

	cfg := host.Config()
	if cfg.Options != nil && cfg.Options.Processors != nil && cfg.Options.Processors.Enabled != nil && !*cfg.Options.Processors.Enabled {
		e.active = false
		return nil
	}

	var list []string
	var procCfg config.ProcessorConfig
	if cfg.Options != nil && cfg.Options.Processors != nil {
		list = cfg.Options.Processors.List
		if cfg.Options.Processors.Config != nil {
			procCfg = cfg.Options.Processors.Config
		}
	}

	toolDefs := host.ToolDefs()
	skillDefs := host.SkillDefs()

	runner := buildProcessorRunner(list, procCfg, host.Completer(), toolDefs, skillDefs)
	if runner == nil {
		e.active = false
		return nil
	}

	e.runner = runner
	e.active = true
	e.tokenBudget = defaultTokenBudget
	if procCfg != nil {
		if pc, ok := procCfg["token_limiter"].(map[string]any); ok {
			if v, ok := pc["budget"].(float64); ok && v > 0 {
				e.tokenBudget = int(v)
			}
		}
	}
	return nil
}

func (e *ProcessorExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runner = nil
	e.active = false
	e.lastPhase = processor.ProcessorPhase(0)
	e.lastState = nil
	e.tokenBudget = 0
	return nil
}

// GetState returns an immutable snapshot of the current processor pipeline
// state. Safe to call from any goroutine; returns a zero-value snapshot if
// the extension has not been initialized.
func (e *ProcessorExtension) GetState() ProcessorStateSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var names []string
	if e.runner != nil {
		all := append(e.runner.InputProcessors, e.runner.OutputProcessors...)
		all = append(all, e.runner.ErrorProcessors...)
		names = make([]string, 0, len(all))
		for _, p := range all {
			names = append(names, p.ID())
		}
	}

	var stateCopy map[string]any
	if e.lastState != nil {
		stateCopy = make(map[string]any, len(e.lastState))
		maps.Copy(stateCopy, e.lastState)
	}

	return ProcessorStateSnapshot{
		Active:         e.active,
		LastPhase:      e.lastPhase,
		ProcessorNames: names,
		TokenBudget:    e.tokenBudget,
		LastState:      stateCopy,
	}
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

	e.mu.Lock()
	e.lastPhase = processor.InputPhase
	if result.State != nil {
		e.lastState = result.State
	}
	e.mu.Unlock()

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

// processOutput invokes the output stream and output result processor phases.
// OutputStreamPhase fires for each streaming chunk, enabling real-time
// processing (e.g. system prompt leak detection). OutputResultPhase fires
// once the full response is assembled.
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

	// OutputStreamPhase: streaming chunk processing (e.g. PII redaction,
	// system prompt scrubbing on partial output).
	streamCtx := processor.ProcessorContext{
		Phase:        processor.OutputStreamPhase,
		OutputStream: text,
		Messages:     pmsgs,
		State:        make(map[string]any),
		Metadata:     make(map[string]any),
	}

	_, err := runner.Execute(ctx, processor.OutputStreamPhase, streamCtx)
	if err != nil {
		slog.Debug("Processor output stream phase failed", "error", err)
	}

	e.mu.Lock()
	e.lastPhase = processor.OutputStreamPhase
	e.mu.Unlock()

	// OutputResultPhase: final assembled output processing.
	pctx := processor.ProcessorContext{
		Phase:        processor.OutputResultPhase,
		OutputResult: text,
		Messages:     pmsgs,
		State:        make(map[string]any),
		Metadata:     make(map[string]any),
	}

	result, err := runner.Execute(ctx, processor.OutputResultPhase, pctx)
	if err != nil {
		slog.Debug("Processor output result phase failed", "error", err)
	}

	e.mu.Lock()
	e.lastPhase = processor.OutputResultPhase
	if result.State != nil {
		e.lastState = result.State
	}
	e.mu.Unlock()
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
	if len(list) == 0 {
		list = defaultProcessors
	}
	enforceOrdering(list)

	var inputProcessors []processor.Processor
	var outputProcessors []processor.Processor

	for _, raw := range list {
		name := normalizeProcessorName(raw)
		if _, ok := safeProcessorNames[name]; !ok {
			slog.Debug("Skipping unknown or unsafe processor", "name", raw, "normalized", name)
			continue
		}
		switch name {
		// Tier 0 — existing processors.
		case "token_limiter":
			budget := defaultTokenBudget
			pc := perProcessor(cfg, name)
			if pc != nil {
				if v, ok := pc["budget"].(float64); ok && v > 0 {
					budget = int(v)
				}
			}
			inputProcessors = append(inputProcessors, &processor.TokenLimiter{
				Budget: budget,
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
		case "workspace_instructions":
			inputProcessors = append(inputProcessors, &processor.WorkspaceInstructions{})
		case "message_history":
			mh := &processor.MessageHistory{Store: &processor.InMemoryStore{}}
			inputProcessors = append(inputProcessors, mh)
			outputProcessors = append(outputProcessors, mh)
		case "moderation":
			if completer == nil {
				slog.Debug("Skipping moderation: no text completer available")
				continue
			}
			threshold := 0.7
			pc := perProcessor(cfg, name)
			if pc != nil {
				if v, ok := pc["threshold"].(float64); ok && v > 0 && v <= 1 {
					threshold = v
				}
			}
			inputProcessors = append(inputProcessors, processor.NewModerationProcessor(&completerAdapter{fn: completer}, threshold))
		case "prompt_injection":
			if completer == nil {
				slog.Debug("Skipping prompt_injection: no text completer available")
				continue
			}
			inputProcessors = append(inputProcessors, processor.NewPromptInjectionDetector(&completerAdapter{fn: completer}))
		case "language_detector":
			if completer == nil {
				slog.Debug("Skipping language_detector: no text completer available")
				continue
			}
			inputProcessors = append(inputProcessors, processor.NewLanguageDetector(&completerAdapter{fn: completer}))
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

// normalizeProcessorName converts PascalCase, camelCase, and kebab-case to
// snake_case so that user config is resilient against style mismatches.
func normalizeProcessorName(name string) string {
	if !strings.ContainsFunc(name, func(r rune) bool {
		return r >= 'A' && r <= 'Z'
	}) {
		return strings.ReplaceAll(name, "-", "_")
	}
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 'a' - 'A')
		} else if r == '-' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
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
