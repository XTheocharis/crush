//go:build treesitter

package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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
