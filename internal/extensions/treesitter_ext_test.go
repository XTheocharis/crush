//go:build treesitter

package extensions

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

func TestTreesitterExtension_ValidationHandlerWired(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{
				Enabled: true,
			},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	handler := e.Handler()
	require.NotNil(t, handler, "Handler() must return non-nil ValidationHandler when validation is enabled")
	require.True(t, handler.Enabled())
}

func TestTreesitterExtension_ValidationHandlerInactiveWithoutConfig(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.Handler())
}

func TestTreesitterExtension_ValidationHandlerDisabledConfig(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{
				Enabled: false,
			},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	handler := e.Handler()
	require.NotNil(t, handler, "Handler() should be non-nil even when validation disabled (inert handler)")
	require.False(t, handler.Enabled())
}

func TestTreesitterExtension_ShutdownClearsHandler(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.NotNil(t, e.Handler())

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.Nil(t, e.Handler())
	require.False(t, e.active)
}

func TestTreesitterExtension_ToolProviderRemoved(t *testing.T) {
	_ = ext.Extension(&TreesitterExtension{})
}

func TestTreesitterExtension_Name(t *testing.T) {
	e := &TreesitterExtension{}
	require.Equal(t, "treesitter-validation", e.Name())
}

func TestTreesitterExtension_StepHooks_NilWhenInactive(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.StepHooks())
}

func TestTreesitterExtension_StepHooks_ReturnsHooksWhenActive(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	require.Equal(t, "treesitter-validation", hooks[0].Name)
	require.NotNil(t, hooks[0].OnPrepareStep)
	require.NotNil(t, hooks[0].OnStepFinish)
	require.NotNil(t, hooks[0].StopCondition)
}

func TestTreesitterExtension_StepHooks_OnPrepareStep_NoWarning(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.StepHooks()
	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{&fantasy.TextPart{Text: "hello"}}},
	}
	result, err := hooks[0].OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Equal(t, msgs, result, "messages unchanged when no pending warning")
}

func TestTreesitterExtension_StepHooks_OnStepFinish_NoEditTools(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.StepHooks()
	step := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.TextPart{Text: "no tools here"},
			}},
		},
	}
	err = hooks[0].OnStepFinish(context.Background(), "s1", step)
	require.NoError(t, err)
	require.False(t, e.criticalFail)
}

func TestTreesitterExtension_StepHooks_StopCondition_DefaultFalse(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.StepHooks()
	require.False(t, hooks[0].StopCondition(context.Background(), nil))
}

func TestTreesitterExtension_StepHooks_ImplementsInterface(t *testing.T) {
	_ = ext.StepHookProvider(&TreesitterExtension{})
}

func TestExtractEditInfoFromStep_EditTool(t *testing.T) {
	input := map[string]string{
		"file_path":  "/tmp/test.go",
		"old_string": "func old()",
		"new_string": "func new()",
	}
	inputJSON, _ := json.Marshal(input)

	step := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "edit", Input: string(inputJSON)},
			}},
		},
	}

	infos := extractEditInfoFromStep(step)
	require.Len(t, infos, 1)
	require.Equal(t, "/tmp/test.go", infos[0].filePath)
	require.Equal(t, "func old()", infos[0].oldContent)
	require.Equal(t, "func new()", infos[0].newContent)
}

func TestExtractEditInfoFromStep_WriteTool(t *testing.T) {
	input := map[string]string{
		"file_path": "/tmp/new.go",
		"content":   "package main",
	}
	inputJSON, _ := json.Marshal(input)

	step := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "write", Input: string(inputJSON)},
			}},
		},
	}

	infos := extractEditInfoFromStep(step)
	require.Len(t, infos, 1)
	require.Equal(t, "/tmp/new.go", infos[0].filePath)
	require.Equal(t, "package main", infos[0].newContent)
}

func TestExtractEditInfoFromStep_NonEditTool(t *testing.T) {
	input := map[string]string{
		"file_path": "/tmp/test.go",
	}
	inputJSON, _ := json.Marshal(input)

	step := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "view", Input: string(inputJSON)},
			}},
		},
	}

	infos := extractEditInfoFromStep(step)
	require.Empty(t, infos)
}

func TestFormatPipelineWarning(t *testing.T) {
	result := &tools.ValidationHandlerResult{
		PipelineResult: &tools.PipelineResult{
			OverallStatus: tools.StatusFail,
			StageResults: []tools.StageResult{
				{StageName: "parse_check", Status: tools.StatusFail, Message: "syntax error"},
			},
		},
	}
	warning := formatPipelineWarning(result)
	require.Contains(t, warning, "tree-sitter validation detected errors")
	require.Contains(t, warning, "parse_check")
}

func TestFormatPipelineWarning_WithRollback(t *testing.T) {
	result := &tools.ValidationHandlerResult{
		PipelineResult: &tools.PipelineResult{
			OverallStatus: tools.StatusFail,
			StageResults: []tools.StageResult{
				{StageName: "parse_check", Status: tools.StatusFail, Message: "syntax error"},
			},
		},
		RolledBack: true,
	}
	warning := formatPipelineWarning(result)
	require.Contains(t, warning, "rolled back")
}
