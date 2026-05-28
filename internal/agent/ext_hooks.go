package agent

import (
	"context"
	"fmt"
	"log/slog"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
)

// agentHookMediator wraps an ExtensionHost and provides typed methods for
// each extension hook invocation site. All methods handle a nil host
// gracefully by returning early.
type agentHookMediator struct {
	host *ext.ExtensionHost
}

// invokeRunStart calls OnRunStart on every registered RunHook.
func (m agentHookMediator) invokeRunStart(ctx context.Context, sessionID, prompt string) {
	if m.host == nil {
		return
	}
	for _, hook := range m.host.RunHooks() {
		if err := safeCall("OnRunStart:"+hook.Name, func() error {
			return hook.OnRunStart(ctx, sessionID, prompt)
		}); err != nil {
			slog.Warn("Extension run hook failed", "hook", hook.Name, "error", err)
		}
	}
}

// invokePreparePrompt calls OnPreparePrompt and SystemPromptModifier on the
// registered PromptHook. It mutates prepared.Messages in place.
func (m agentHookMediator) invokePreparePrompt(ctx context.Context, sessionID string, prepared *fantasy.PrepareStepResult) {
	if m.host == nil {
		return
	}
	hook := m.host.GetPromptHook()
	if hook == nil {
		return
	}
	if hook.OnPreparePrompt != nil {
		_ = safeCall("OnPreparePrompt:"+hook.Name, func() error {
			var hookErr error
			prepared.Messages, hookErr = hook.OnPreparePrompt(ctx, sessionID, prepared.Messages)
			return hookErr
		})
	}
	if hook.SystemPromptModifier != nil {
		for i, msg := range prepared.Messages {
			if msg.Role == fantasy.MessageRoleSystem {
				_ = safeCall("SystemPromptModifier:"+hook.Name, func() error {
					var original string
					if tp, ok := fantasy.AsMessagePart[fantasy.TextPart](msg.Content[0]); ok {
						original = tp.Text
					}
					modified, modErr := hook.SystemPromptModifier(ctx, sessionID, original)
					if modErr != nil {
						return modErr
					}
					prepared.Messages[i].Content = []fantasy.MessagePart{fantasy.TextPart{Text: modified}}
					return nil
				})
				break
			}
		}
	}
}

// invokePrepareStep calls OnPrepareStep on every registered StepHook.
func (m agentHookMediator) invokePrepareStep(ctx context.Context, sessionID string, prepared *fantasy.PrepareStepResult) {
	if m.host == nil {
		return
	}
	for _, hook := range m.host.StepHooks() {
		if hook.OnPrepareStep != nil {
			_ = safeCall("OnPrepareStep:"+hook.Name, func() error {
				var stepErr error
				prepared.Messages, stepErr = hook.OnPrepareStep(ctx, sessionID, prepared.Messages)
				return stepErr
			})
		}
	}
}

// invokeStepFinish calls OnStepFinish on every registered StepHook.
func (m agentHookMediator) invokeStepFinish(ctx context.Context, sessionID string, stepResult fantasy.StepResult) {
	if m.host == nil {
		return
	}
	for _, hook := range m.host.StepHooks() {
		if hook.OnStepFinish != nil {
			if err := safeCall("OnStepFinish:"+hook.Name, func() error {
				return hook.OnStepFinish(ctx, sessionID, stepResult)
			}); err != nil {
				slog.Warn("Extension step finish hook failed", "hook", hook.Name, "error", err)
			}
		}
	}
}

// checkStopCondition evaluates StopCondition on every registered StepHook.
// Returns true if any hook signals a stop.
func (m agentHookMediator) checkStopCondition(ctx context.Context, steps []fantasy.StepResult) bool {
	if m.host == nil {
		return false
	}
	for _, hook := range m.host.StepHooks() {
		if hook.StopCondition != nil {
			var shouldStop bool
			hookErr := safeCall("StopCondition:"+hook.Name, func() error {
				shouldStop = hook.StopCondition(ctx, steps)
				return nil
			})
			if hookErr != nil {
				slog.Warn("Extension stop condition hook errored", "hook", hook.Name, "error", hookErr)
				continue
			}
			if shouldStop {
				return true
			}
		}
	}
	return false
}

// getRoutedModelType returns the model type selected by the model router
// extension's most recent routing decision. Returns
// config.SelectedModelTypeLarge if no router is available or no routing
// has occurred.
func (m agentHookMediator) getRoutedModelType() config.SelectedModelType {
	if m.host == nil {
		return config.SelectedModelTypeLarge
	}
	e := m.host.ExtensionByName("model_router")
	if e == nil {
		return config.SelectedModelTypeLarge
	}
	type lastModelGetter interface {
		LastRoutedModel() config.SelectedModelType
	}
	getter, ok := e.(lastModelGetter)
	if !ok {
		return config.SelectedModelTypeLarge
	}
	mt := getter.LastRoutedModel()
	if mt == "" {
		return config.SelectedModelTypeLarge
	}
	return mt
}

// invokeRunEnd calls OnRunEnd on every registered RunHook.
func (m agentHookMediator) invokeRunEnd(ctx context.Context, sessionID string, result *fantasy.AgentResult, err error) {
	if m.host == nil {
		return
	}
	for _, hook := range m.host.RunHooks() {
		if err := safeCall("OnRunEnd:"+hook.Name, func() error {
			return hook.OnRunEnd(ctx, sessionID, result, err)
		}); err != nil {
			slog.Warn("Extension run-end hook failed", "hook", hook.Name, "error", err)
		}
	}
}

// safeCall executes fn with panic recovery, logging the panic and returning
// it as an error.
func safeCall(name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Extension hook panicked", "hook", name, "panic", r)
			err = fmt.Errorf("extension hook %s panicked: %v", name, r)
		}
	}()
	return fn()
}
