//go:build treesitter

package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/treesitter"
)

// TreesitterExtension wires the tree-sitter validation pipeline as
// post-edit infrastructure. Only compiled when the "treesitter" build tag
// is set.
type TreesitterExtension struct {
	mu             sync.RWMutex
	host           ext.HostContext
	handler        *tools.ValidationHandler
	active         bool
	pendingWarning string
	criticalFail   bool
}

func (e *TreesitterExtension) Name() string { return "treesitter-validation" }

func (e *TreesitterExtension) Init(_ context.Context, host ext.HostContext) error {
	e.host = host

	cfg := host.Config()
	if cfg == nil || cfg.Options == nil || cfg.Options.Validation == nil {
		e.active = false
		return nil
	}

	vcfg := cfg.Options.Validation
	handlerCfg := tools.ValidationHandlerConfig{
		Enabled: vcfg.Enabled,
		AutoFix: vcfg.AutoFix,
	}

	// Create the tree-sitter parser. Stages 5-7 (ParseCheck,
	// SymbolConsistency, ImportConsistency) skip gracefully when nil.
	var parser interface{}
	if vcfg.Enabled {
		parser = treesitter.NewParser()
		if parser == nil {
			slog.Warn("TreesitterExtension: NewParser returned nil, parser-dependent stages will be skipped")
		}
	}

	var diagGate *tools.DiagnosticGate
	if lspMgr := host.LSP(); lspMgr != nil {
		diagGate = tools.NewDiagnosticGate(lspMgr, tools.WithSeverityFilter(
			tools.ParseSeverityFilter(vcfg.SeverityFilter),
		))
	}

	e.handler = tools.NewValidationHandler(parser, diagGate, handlerCfg)
	tools.InitSymbolParser(parser)
	e.active = true
	return nil
}

func (e *TreesitterExtension) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	tools.InitSymbolParser(nil)
	e.handler = nil
	e.active = false
	e.pendingWarning = ""
	e.criticalFail = false
	return nil
}

// Handler returns the wired ValidationHandler for use as post-edit
// infrastructure by the coordinator.
func (e *TreesitterExtension) Handler() *tools.ValidationHandler {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.handler
}

// StepHooks returns hooks that run tree-sitter validation after edit tool
// calls. If the validation pipeline detects syntax errors that break the
// file, the extension injects an XML warning into the next step's messages
// and signals early termination via StopCondition.
func (e *TreesitterExtension) StepHooks() []ext.StepHook {
	if !e.active {
		return nil
	}
	return []ext.StepHook{
		{
			Name: "treesitter-validation",
			OnPrepareStep: func(_ context.Context, _ string, messages []fantasy.Message) ([]fantasy.Message, error) {
				e.mu.Lock()
				warning := e.pendingWarning
				e.pendingWarning = ""
				e.mu.Unlock()

				if warning == "" {
					return messages, nil
				}

				warningText := fmt.Sprintf(`<treesitter-validation-warning>%s</treesitter-validation-warning>`, warning)
				warningMsg := fantasy.Message{
					Role: fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{
						&fantasy.TextPart{Text: warningText},
					},
				}
				return append([]fantasy.Message{warningMsg}, messages...), nil
			},
			OnStepFinish: func(ctx context.Context, _ string, step fantasy.StepResult) error {
				e.mu.Lock()
				handler := e.handler
				e.mu.Unlock()
				if handler == nil || !handler.Enabled() {
					return nil
				}

				editInfos := extractEditInfoFromStep(step)
				if len(editInfos) == 0 {
					return nil
				}

				for _, info := range editInfos {
					snapshot, _ := handler.CaptureSnapshot([]string{info.filePath})
					result, err := handler.ValidateEdit(ctx, snapshot, info.filePath, info.oldContent, info.newContent, info.editSpec)
					if err != nil {
						slog.Debug("TreesitterExtension: ValidateEdit error",
							"file", info.filePath,
							"error", err,
						)
						continue
					}
					if result == nil {
						continue
					}

					if result.PipelineResult != nil && result.PipelineResult.OverallStatus == tools.StatusFail {
						e.mu.Lock()
						e.criticalFail = true
						e.mu.Unlock()

						warning := formatPipelineWarning(result)
						e.mu.Lock()
						e.pendingWarning = warning
						e.mu.Unlock()

						slog.Warn("TreesitterExtension: validation failed",
							"file", info.filePath,
							"rolledBack", result.RolledBack,
						)
					}
				}
				return nil
			},
			StopCondition: func(_ context.Context, _ []fantasy.StepResult) bool {
				e.mu.RLock()
				defer e.mu.RUnlock()
				return e.criticalFail
			},
		},
	}
}

type editInfo struct {
	filePath   string
	oldContent string
	newContent string
	editSpec   tools.EditSpec
}

func extractEditInfoFromStep(step fantasy.StepResult) []editInfo {
	var infos []editInfo
	for _, msg := range step.Messages {
		for _, part := range msg.Content {
			if part.GetType() != fantasy.ContentTypeToolCall {
				continue
			}
			call, ok := fantasy.AsContentType[fantasy.ToolCallPart](part)
			if !ok || !validationEditTools[call.ToolName] {
				continue
			}
			info, ok := parseEditInfoFromJSON(call.Input)
			if ok {
				infos = append(infos, info)
			}
		}
	}
	return infos
}

var validationEditTools = map[string]bool{
	"edit":       true,
	"multiedit":  true,
	"batch_edit": true,
	"write":      true,
}

func parseEditInfoFromJSON(input string) (editInfo, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return editInfo{}, false
	}

	var info editInfo
	for _, key := range []string{"file_path", "path"} {
		if v, ok := raw[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				info.filePath = s
				break
			}
		}
	}
	if info.filePath == "" {
		return editInfo{}, false
	}

	if v, ok := raw["old_string"]; ok {
		_ = json.Unmarshal(v, &info.oldContent)
	}
	if v, ok := raw["new_string"]; ok {
		_ = json.Unmarshal(v, &info.newContent)
	}
	if v, ok := raw["content"]; ok {
		_ = json.Unmarshal(v, &info.newContent)
	}

	info.editSpec = tools.EditSpec{
		OldString:  info.oldContent,
		NewString:  info.newContent,
		ReplaceAll: false,
	}

	return info, true
}

func formatPipelineWarning(result *tools.ValidationHandlerResult) string {
	if result.PipelineResult == nil {
		return ""
	}
	var failed []string
	for _, sr := range result.PipelineResult.StageResults {
		if sr.Status == tools.StatusFail || sr.Status == tools.StatusError {
			failed = append(failed, fmt.Sprintf("%s: %s", sr.StageName, sr.Message))
		}
	}
	msg := "tree-sitter validation detected errors after edit"
	if result.RolledBack {
		msg = "tree-sitter validation detected errors after edit, file has been rolled back"
	}
	if len(failed) > 0 {
		msg += fmt.Sprintf(" (failed stages: %v)", failed)
	}
	return msg
}

var (
	_ ext.Extension        = (*TreesitterExtension)(nil)
	_ ext.StepHookProvider = (*TreesitterExtension)(nil)
)
