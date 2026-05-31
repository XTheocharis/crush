package extensions

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
)

// DiagGateExtension wraps the diagnostic gate as a ToolProvider and
// StepHookProvider. The gate compares pre/post-edit LSP diagnostics to detect
// newly introduced errors. It contributes the lsp_diagnostics tool and
// optionally hooks into step lifecycle for automatic baseline capture and
// comparison.
type DiagGateExtension struct {
	mu       sync.RWMutex
	host     ext.HostContext
	gate     *tools.DiagnosticGate
	active   bool
	tools    []fantasy.AgentTool
	toolName string
}

func (e *DiagGateExtension) Name() string { return "diag-gate" }

func (e *DiagGateExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	lspManager := host.LSP()
	if lspManager == nil {
		e.active = false
		return nil
	}

	e.gate = tools.NewDiagnosticGate(lspManager, tools.WithSeverityFilter(
		tools.ParseSeverityFilter(severityFilterFromConfig(host.Config())),
	))
	e.tools = []fantasy.AgentTool{tools.NewDiagnosticsTool(lspManager)}
	e.toolName = tools.DiagnosticsToolName
	e.active = true
	return nil
}

func (e *DiagGateExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gate = nil
	e.tools = nil
	e.toolName = ""
	e.active = false
	return nil
}

func (e *DiagGateExtension) Tools(_ context.Context) ([]fantasy.AgentTool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil, nil
	}
	return append([]fantasy.AgentTool{}, e.tools...), nil
}

func (e *DiagGateExtension) ToolNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.active {
		return nil
	}
	return []string{e.toolName}
}

// StepHooks returns hooks that capture a diagnostic baseline before each step
// and log a comparison after. This provides automatic error-introduction
// detection without requiring explicit tool calls.
func (e *DiagGateExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "diag-gate-capture",
			OnPrepareStep: func(ctx context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
				e.mu.RLock()
				gate := e.gate
				e.mu.RUnlock()
				if gate == nil {
					return messages, nil
				}

				filePaths := extractFilePathsFromMessages(messages)
				if len(filePaths) > 0 {
					gate.CaptureBaseline(ctx, filePaths)
				}
				return messages, nil
			},
			OnStepFinish: func(ctx context.Context, _ string, step fantasy.StepResult) error {
				e.mu.RLock()
				gate := e.gate
				e.mu.RUnlock()
				if gate == nil {
					return nil
				}

				filePaths := extractFilePathsFromStep(step)
				if len(filePaths) > 0 {
					result := gate.Compare(ctx, filePaths)
					if !result.Pass {
						slog.Warn("Diagnostic gate detected new errors",
							"message", result.Message(),
							"newErrors", len(result.NewErrors),
						)
					}
				}
				return nil
			},
		},
	}
}

func extractFilePathsFromMessages(messages []fantasy.Message) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, msg := range messages {
		for _, part := range msg.Content {
			for _, p := range filePathsFromPart(part) {
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

func extractFilePathsFromStep(step fantasy.StepResult) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, msg := range step.Messages {
		for _, part := range msg.Content {
			for _, p := range filePathsFromPart(part) {
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

// editTools lists tool names whose inputs likely contain a file_path field.
var editTools = map[string]bool{
	"edit": true, "multiedit": true, "write": true,
}

func filePathsFromPart(part fantasy.MessagePart) []string {
	if part.GetType() != fantasy.ContentTypeToolCall {
		return nil
	}
	call, ok := fantasy.AsContentType[fantasy.ToolCallPart](part)
	if !ok || !editTools[call.ToolName] {
		return nil
	}
	return parseFilePathsFromJSON(call.Input)
}

func parseFilePathsFromJSON(input string) []string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return nil
	}
	var paths []string
	for _, key := range []string{"file_path", "path"} {
		if v, ok := raw[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				paths = append(paths, s)
			}
		}
	}
	return paths
}

func severityFilterFromConfig(cfg *config.Config) string {
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		return ""
	}
	return cfg.Options.Validation.SeverityFilter
}

var (
	_ ext.Extension        = (*DiagGateExtension)(nil)
	_ ext.ToolProvider     = (*DiagGateExtension)(nil)
	_ ext.StepHookProvider = (*DiagGateExtension)(nil)
)
