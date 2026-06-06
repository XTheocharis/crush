//go:build treesitter

package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// alwaysFailStage is a ValidationStage that always reports StatusFail.
type alwaysFailStage struct{}

func (s *alwaysFailStage) Name() string { return "always_fail" }
func (s *alwaysFailStage) Execute(_ context.Context, _ *ValidationInput) (*StageResult, error) {
	return &StageResult{StageName: "always_fail", Status: StatusFail, Message: "forced failure"}, nil
}
func (s *alwaysFailStage) CanSkip() bool { return false }

func TestValidationHandler_CaptureBaseline_NilGate_NoPanic(t *testing.T) {
	t.Parallel()
	vh := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: true})
	require.NotPanics(t, func() {
		vh.CaptureBaseline(context.Background(), []string{"/tmp/test.go"})
	})
}

func TestValidationHandler_CaptureBaseline_Disabled_NoPanic(t *testing.T) {
	t.Parallel()
	vh := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: false})
	require.NotPanics(t, func() {
		vh.CaptureBaseline(context.Background(), []string{"/tmp/test.go"})
	})
}

func TestDiagnosticsStage_EmptyBaseline_TreatsAllAsAdded(t *testing.T) {
	t.Parallel()

	// BUG REPRODUCTION: When CaptureBaseline is never called before
	// DiagnosticsStage.Execute, the gate's baseline is an empty map.
	// Any pre-existing LSP diagnostics will appear as "Added" in the diff,
	// triggering false rollback.
	//
	// This test demonstrates the core mechanism: computeDiff with an empty
	// baseline treats all post-state diagnostics as "Added".
	//
	// Expected (correct): pre-existing diagnostics should be in Unchanged.
	// Actual (buggy): pre-existing diagnostics end up in Added.

	preExisting := DiagnosticInfo{
		FilePath:  "test.go",
		Line:      10,
		Character: 0,
		Severity:  SeverityError,
		Message:   "pre-existing error from LSP",
	}

	// Simulate: gate created but CaptureBaseline never called → empty baseline.
	emptyBaseline := map[diagnosticKey]DiagnosticInfo{}

	// Simulate: post-edit state still has the same pre-existing diagnostic
	// (the edit was benign and didn't change diagnostics).
	postMap := map[diagnosticKey]DiagnosticInfo{
		preExisting.Key(): preExisting,
	}

	diff := computeDiff(emptyBaseline, postMap)

	// This assertion FAILS — demonstrating the bug.
	// With an empty baseline, the pre-existing error appears as "Added"
	// instead of "Unchanged". In production, this causes DiagnosticsStage
	// to report StatusFail, which triggers false rollback.
	require.Empty(t, diff.Added,
		"BUG: pre-existing diagnostic appears as 'Added' when baseline is empty; "+
			"CaptureBaseline was never called before DiagnosticsStage.Execute")
	require.Len(t, diff.Unchanged, 1,
		"pre-existing diagnostic should be unchanged, not added")
}

func TestValidationHandler_ValidateEdit_FailureTriggersRollback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "rollback_test.go")
	require.NoError(t, os.WriteFile(filePath, []byte("original content"), 0o644))

	vh := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: true})
	vh.ReplaceStages([]ValidationStage{&alwaysFailStage{}})

	snap, err := vh.CaptureSnapshot([]string{filePath})
	require.NoError(t, err)
	require.NotNil(t, snap)

	require.NoError(t, os.WriteFile(filePath, []byte("modified content"), 0o644))

	result, err := vh.ValidateEdit(
		context.Background(),
		snap,
		filePath,
		"original content",
		"modified content",
		EditSpec{OldString: "original", NewString: "modified"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.RolledBack, "validation failure should trigger rollback")
	require.Equal(t, "original content", result.FinalContent)

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "original content", string(data), "file should be restored to original")
}

func TestValidationHandler_ValidateEdit_Disabled_NoRollback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "disabled_test.go")
	require.NoError(t, os.WriteFile(filePath, []byte("original"), 0o644))

	vh := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: false})

	snap, err := vh.CaptureSnapshot([]string{filePath})
	require.NoError(t, err)
	require.Nil(t, snap, "disabled handler should return nil snapshot")

	require.NoError(t, os.WriteFile(filePath, []byte("modified"), 0o644))

	result, err := vh.ValidateEdit(
		context.Background(),
		nil,
		filePath,
		"original",
		"modified",
		EditSpec{},
	)
	require.NoError(t, err)
	require.Nil(t, result, "disabled handler should return nil result")

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "modified", string(data), "disabled handler should not rollback")
}

func TestValidationHandler_CaptureSnapshot_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "snap.go")
	require.NoError(t, os.WriteFile(filePath, []byte("snap content"), 0o644))

	vh := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: true})

	snap, err := vh.CaptureSnapshot([]string{filePath})
	require.NoError(t, err)
	require.NotNil(t, snap)
	require.Len(t, snap.Files, 1)
	require.Equal(t, "snap content", snap.Files[0].Content)
}

func TestValidationHandler_Enabled(t *testing.T) {
	t.Parallel()

	enabled := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: true})
	require.True(t, enabled.Enabled())

	disabled := NewValidationHandler(nil, nil, ValidationHandlerConfig{Enabled: false})
	require.False(t, disabled.Enabled())
}

func TestCountFailedStages(t *testing.T) {
	t.Parallel()

	pr := &PipelineResult{
		StageResults: []StageResult{
			{StageName: "pass1", Status: StatusPass},
			{StageName: "fail1", Status: StatusFail},
			{StageName: "pass2", Status: StatusPass},
			{StageName: "err1", Status: StatusError},
			{StageName: "skip1", Status: StatusSkip},
		},
	}
	require.Equal(t, 2, countFailedStages(pr))
}

func TestCountFailedStages_AllPass(t *testing.T) {
	t.Parallel()

	pr := &PipelineResult{
		StageResults: []StageResult{
			{StageName: "a", Status: StatusPass},
			{StageName: "b", Status: StatusPass},
		},
	}
	require.Equal(t, 0, countFailedStages(pr))
}
