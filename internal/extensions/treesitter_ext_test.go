//go:build treesitter

package extensions

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/rewind"
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
	require.Equal(t, "func new()", infos[0].newContent)
	require.Equal(t, "func old()", infos[0].editSpec.OldString)
	require.Equal(t, "func new()", infos[0].editSpec.NewString)
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

func TestParseEditInfoFromJSON_ReadsFullFileFromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/test.go"

	fullContent := `package main

func old() {}
func helper() {}
`
	require.NoError(t, os.WriteFile(filePath, []byte(fullContent), 0o644))

	input := map[string]string{
		"file_path":  filePath,
		"old_string": "func old()",
		"new_string": "func new()",
	}
	inputJSON, _ := json.Marshal(input)

	info, ok := parseEditInfoFromJSON(string(inputJSON))
	require.True(t, ok)
	require.Equal(t, filePath, info.filePath)
	require.Equal(t, "", info.preEditContent, "parseEditInfoFromJSON no longer reads from disk")
	require.Equal(t, "func new()", info.newContent)
	require.Equal(t, "func old()", info.editSpec.OldString)
	require.Equal(t, "func new()", info.editSpec.NewString)
}

func TestParseEditInfoFromJSON_MissingFileReturnsEmptyOldContent(t *testing.T) {
	input := map[string]string{
		"file_path":  "/nonexistent/path/test.go",
		"old_string": "func old()",
		"new_string": "func new()",
	}
	inputJSON, _ := json.Marshal(input)

	info, ok := parseEditInfoFromJSON(string(inputJSON))
	require.True(t, ok)
	require.Equal(t, "", info.preEditContent)
	require.Equal(t, "func old()", info.editSpec.OldString)
	require.Equal(t, "func new()", info.editSpec.NewString)
}

// ---------------------------------------------------------------------------
// Integration tests: full validation lifecycle through hook closures
// ---------------------------------------------------------------------------

// helperInitActiveExtension creates an initialised, active TreesitterExtension.
func helperInitActiveExtension(t *testing.T) *TreesitterExtension {
	t.Helper()
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)
	return e
}

func TestTreesitterExtension_FullValidationLifecycle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/lifecycle.go"

	validGo := `package main

func hello() string { return "hello" }
`
	require.NoError(t, os.WriteFile(filePath, []byte(validGo), 0o644))

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	require.Len(t, hooks, 1)

	// Step 1: OnPrepareStep — captures snapshot of the file.
	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			&fantasy.TextPart{Text: "let me edit the file"},
		}},
	}
	prepared, err := hooks[0].OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.Equal(t, msgs, prepared, "no pending warning, messages unchanged")

	// Step 2: OnStepFinish — non-edit step, no validation triggered.
	textStep := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.TextPart{Text: "just thinking"},
			}},
		},
	}
	require.NoError(t, hooks[0].OnStepFinish(context.Background(), "s1", textStep))
	require.False(t, e.criticalFail, "no edit tools, no critical failure")

	// Step 3: StopCondition — should be false.
	require.False(t, hooks[0].StopCondition(context.Background(), nil))

	// Verify file untouched.
	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, validGo, string(got))
}

func TestTreesitterExtension_ValidationFailure_WithBadSyntax(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/badsyntax.go"

	validGo := `package main

func greet() string { return "hi" }
func farewell() string { return "bye" }
`
	require.NoError(t, os.WriteFile(filePath, []byte(validGo), 0o644))

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	require.Len(t, hooks, 1)

	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			&fantasy.TextPart{Text: "editing"},
		}},
	}
	_, err := hooks[0].OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)

	// SymbolConsistency failure (duplicate symbol definitions).
	inputJSON, _ := json.Marshal(map[string]string{
		"file_path":  filePath,
		"old_string": `func farewell() string { return "bye" }`,
		"new_string": `func greet() string { return "bye" }`,
	})
	editStep := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "edit", Input: string(inputJSON)},
			}},
		},
	}

	require.NoError(t, hooks[0].OnStepFinish(context.Background(), "s1", editStep))

	e.mu.RLock()
	critical := e.criticalFail
	warning := e.pendingWarning
	e.mu.RUnlock()
	require.True(t, critical, "criticalFail should be set for duplicate symbol edit")
	require.True(t, hooks[0].StopCondition(context.Background(), nil),
		"StopCondition should return true after critical fail")
	require.NotEmpty(t, warning, "pendingWarning should be set")
	require.Contains(t, warning, "tree-sitter validation detected errors")

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, validGo, string(got),
		"file should be rolled back to pre-edit snapshot")
}

func TestTreesitterExtension_ValidationPass_WithGoodEdit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/goodedit.go"

	originalGo := `package main

func greet() string { return "hi" }
`
	require.NoError(t, os.WriteFile(filePath, []byte(originalGo), 0o644))

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	require.Len(t, hooks, 1)

	// OnPrepareStep — captures snapshot.
	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			&fantasy.TextPart{Text: "editing"},
		}},
	}
	_, err := hooks[0].OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)

	// Build edit step that produces valid Go.
	inputJSON, _ := json.Marshal(map[string]string{
		"file_path":  filePath,
		"old_string": `func greet() string { return "hi" }`,
		"new_string": `func greet() string { return "hello" }`,
	})
	editStep := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "edit", Input: string(inputJSON)},
			}},
		},
	}

	require.NoError(t, hooks[0].OnStepFinish(context.Background(), "s1", editStep))

	// Validation should pass — no critical fail, no stop.
	e.mu.RLock()
	critical := e.criticalFail
	warning := e.pendingWarning
	e.mu.RUnlock()
	require.False(t, critical, "criticalFail should remain false for valid Go edit")
	require.False(t, hooks[0].StopCondition(context.Background(), nil),
		"StopCondition should return false when validation passes")
	require.Empty(t, warning, "no pending warning for passing edit")
}

func TestTreesitterCriticalFailRecovery(t *testing.T) {
	t.Parallel()

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	hook := hooks[0]

	e.mu.Lock()
	e.criticalFail = true
	e.mu.Unlock()

	require.True(t, hook.StopCondition(context.Background(), nil),
		"StopCondition should return true when criticalFail is set")

	_, err := hook.OnPrepareStep(context.Background(), "session", nil)
	require.NoError(t, err)

	require.False(t, hook.StopCondition(context.Background(), nil),
		"criticalFail should reset at the start of OnPrepareStep")

	e.mu.Lock()
	e.criticalFail = true
	e.mu.Unlock()
	require.True(t, hook.StopCondition(context.Background(), nil))

	_, err = hook.OnPrepareStep(context.Background(), "session", nil)
	require.NoError(t, err)
	require.False(t, hook.StopCondition(context.Background(), nil),
		"criticalFail should reset again on second OnPrepareStep")
}

func TestTreesitterCriticalFailRecovery_AfterValidationFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/fail_recovery.go"

	validGo := `package main

func greet() string { return "hi" }
func farewell() string { return "bye" }
`
	require.NoError(t, os.WriteFile(filePath, []byte(validGo), 0o644))

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	hook := hooks[0]

	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			&fantasy.TextPart{Text: "editing"},
		}},
	}
	_, err := hook.OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)

	inputJSON, _ := json.Marshal(map[string]string{
		"file_path":  filePath,
		"old_string": `func farewell() string { return "bye" }`,
		"new_string": `func greet() string { return "bye" }`,
	})
	editStep := fantasy.StepResult{
		Messages: []fantasy.Message{
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
				&fantasy.ToolCallPart{ToolName: "edit", Input: string(inputJSON)},
			}},
		},
	}
	require.NoError(t, hook.OnStepFinish(context.Background(), "s1", editStep))
	require.True(t, hook.StopCondition(context.Background(), nil),
		"StopCondition should be true after validation failure")

	_, err = hook.OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)
	require.False(t, hook.StopCondition(context.Background(), nil),
		"criticalFail should reset on next OnPrepareStep, allowing agent to recover")
}

func TestTreesitterPreEditSnapshot(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := tmpDir + "/snapshot_test.go"

	preEditContent := `package main

func old() {}
`
	postEditContent := `package main

func new() {}
`
	require.NoError(t, os.WriteFile(filePath, []byte(preEditContent), 0o644))

	e := helperInitActiveExtension(t)
	hooks := e.StepHooks()
	hook := hooks[0]

	editInputJSON, _ := json.Marshal(map[string]string{
		"file_path":  filePath,
		"old_string": "func old()",
		"new_string": "func new()",
	})
	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			&fantasy.ToolCallPart{ToolName: "edit", Input: string(editInputJSON)},
		}},
	}
	_, err := hook.OnPrepareStep(context.Background(), "s1", msgs)
	require.NoError(t, err)

	e.mu.RLock()
	snap := e.pendingSnapshots[filePath]
	e.mu.RUnlock()
	require.NotNil(t, snap, "pendingSnapshots should contain entry for file")
	require.Equal(t, preEditContent, snapshotContent(snap, filePath),
		"snapshot should hold pre-edit content")

	require.NoError(t, os.WriteFile(filePath, []byte(postEditContent), 0o644))

	e.mu.RLock()
	snapAfter := e.pendingSnapshots[filePath]
	e.mu.RUnlock()
	require.Equal(t, preEditContent, snapshotContent(snapAfter, filePath),
		"snapshot should still hold pre-edit content after disk is overwritten")
}

func TestSnapshotContent_NilSnapshot(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", snapshotContent(nil, "/some/file.go"))
}

func TestSnapshotContent_FileNotInSnapshot(t *testing.T) {
	t.Parallel()
	snap := &tools.Snapshot{
		Files: []tools.FileSnapshot{
			{FilePath: "/other/file.go", Content: "package other"},
		},
	}
	require.Equal(t, "", snapshotContent(snap, "/some/other.go"))
}

func TestSnapshotContent_ExtractsCorrectFile(t *testing.T) {
	t.Parallel()
	snap := &tools.Snapshot{
		Files: []tools.FileSnapshot{
			{FilePath: "/a.go", Content: "package a"},
			{FilePath: "/b.go", Content: "package b"},
		},
	}
	require.Equal(t, "package a", snapshotContent(snap, "/a.go"))
	require.Equal(t, "package b", snapshotContent(snap, "/b.go"))
}

type mockRewindHostContext struct {
	mockHostContext
	rewindSvc rewind.Service
}

func (m *mockRewindHostContext) RewindService() rewind.Service { return m.rewindSvc }

type mockSnapshotter struct {
	captured bool
}

func (m *mockSnapshotter) CaptureSnapshot(_ context.Context, _ string, _ int) error {
	m.captured = true
	return nil
}

func (m *mockSnapshotter) GetSnapshotAtOrBeforeSeq(_ context.Context, _ string, _ int) (*rewind.TurnSnapshot, error) {
	return nil, nil
}

func (m *mockSnapshotter) GetSnapshotFiles(_ context.Context, _ string) ([]rewind.SnapshotFile, error) {
	return nil, nil
}

func (m *mockSnapshotter) DeleteSnapshotsAfterSeq(_ context.Context, _ string, _ int) error {
	return nil
}
func (m *mockSnapshotter) CleanupOldSnapshots(_ context.Context, _ string) error { return nil }
func (m *mockSnapshotter) Rewind(_ context.Context, _ string, _ int, _ rewind.RewindMode) (*rewind.RewindResult, error) {
	return nil, nil
}

func (m *mockSnapshotter) Fork(_ context.Context, _ string, _ int) (*rewind.ForkResult, error) {
	return nil, nil
}

func (m *mockSnapshotter) ExtractMessageText(_ context.Context, _ string, _ int) (*rewind.EditResult, error) {
	return nil, nil
}

func (m *mockSnapshotter) UpdateMessageText(_ context.Context, _ string, _ int, _ string) error {
	return nil
}

func TestTreesitterInit_SetsSnapshotter(t *testing.T) {
	t.Parallel()

	snap := &mockSnapshotter{}
	host := &mockRewindHostContext{
		mockHostContext: mockHostContext{cfg: &config.Config{
			Options: &config.Options{
				Validation: &config.ValidationOptions{Enabled: true},
			},
		}},
		rewindSvc: snap,
	}

	e := &TreesitterExtension{}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)
	require.True(t, e.snapshotterSet, "snapshotterSet should be true after Init with RewindService")
	require.NotNil(t, e.rewindService, "rewindService should be stored")
}
