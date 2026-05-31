//go:build treesitter

package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

// buildContentWithLines returns a string where each line has enough words to
// place anchors predictably. Lines are 0-indexed in the returned string.
// Each generated line has 5 words so that with interval=5, one anchor lands
// per line.
func buildContentWithLines(numLines int) string {
	var b strings.Builder
	for i := 0; i < numLines; i++ {
		// 5 words per line → at interval=5, anchor lands on each line's 4th
		// word (0-indexed word 4, 9, 14, ...).
		b.WriteString("word word word word word\n")
	}
	return b.String()
}

func TestASTAnchorBridgeSymbolRanges(t *testing.T) {
	t.Parallel()

	// 40 lines, interval=5 → anchor every 5th word. Since each line has 5
	// words, anchors land at word indices 4, 9, 14, 19... which map to
	// lines 0, 1, 2, 3... With interval=5, anchors appear on every line.
	content := buildContentWithLines(40)
	bridge := NewASTAnchorBridge(nil)

	sym := treesitter.SymbolInfo{
		Name:    "MyFunc",
		Kind:    "function",
		Line:    10,
		EndLine: 30,
	}

	results, err := bridge.MapSymbolsToAnchors(
		context.Background(),
		"test.go",
		content,
		[]treesitter.SymbolInfo{sym},
		5,
	)
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	require.Equal(t, "MyFunc", r.Symbol.Name)
	require.Equal(t, 10, r.StartAnchor.LineNum)
	require.Equal(t, 30, r.EndAnchor.LineNum)
}

func TestASTAnchorBridgeSymbolRangesMultipleSymbols(t *testing.T) {
	t.Parallel()

	content := buildContentWithLines(60)
	bridge := NewASTAnchorBridge(nil)

	symbols := []treesitter.SymbolInfo{
		{Name: "FuncA", Kind: "function", Line: 5, EndLine: 15},
		{Name: "FuncB", Kind: "function", Line: 20, EndLine: 40},
		{Name: "FuncC", Kind: "function", Line: 50, EndLine: 55},
	}

	results, err := bridge.MapSymbolsToAnchors(
		context.Background(),
		"test.go",
		content,
		symbols,
		5,
	)
	require.NoError(t, err)
	require.Len(t, results, 3)

	require.Equal(t, "FuncA", results[0].Symbol.Name)
	require.Equal(t, 5, results[0].StartAnchor.LineNum)
	require.Equal(t, 15, results[0].EndAnchor.LineNum)

	require.Equal(t, "FuncB", results[1].Symbol.Name)
	require.Equal(t, 20, results[1].StartAnchor.LineNum)
	require.Equal(t, 40, results[1].EndAnchor.LineNum)

	require.Equal(t, "FuncC", results[2].Symbol.Name)
	require.Equal(t, 50, results[2].StartAnchor.LineNum)
	require.Equal(t, 55, results[2].EndAnchor.LineNum)
}

func TestASTAnchorBridgeSymbolRangesNoAnchorsBeforeSymbol(t *testing.T) {
	t.Parallel()

	// Content with anchors only at later lines (interval is very large so
	// few anchors exist). Symbol starts before any anchor.
	content := "word word word word word\n" + // line 0
		"word word word word word\n" // line 1
	// Only 10 words total. With interval=100, no anchor is created at all.

	bridge := NewASTAnchorBridge(nil)

	sym := treesitter.SymbolInfo{
		Name:    "EarlyFunc",
		Kind:    "function",
		Line:    0,
		EndLine: 1,
	}

	results, err := bridge.MapSymbolsToAnchors(
		context.Background(),
		"test.go",
		content,
		[]treesitter.SymbolInfo{sym},
		100,
	)
	require.NoError(t, err)
	require.Empty(t, results, "symbol before any anchor should be skipped")
}

func TestASTAnchorBridgeSymbolRangesEmpty(t *testing.T) {
	t.Parallel()

	bridge := NewASTAnchorBridge(nil)
	content := buildContentWithLines(10)

	results, err := bridge.MapSymbolsToAnchors(
		context.Background(),
		"test.go",
		content,
		nil,
		5,
	)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestBatchProcessorMaxFilesCap(t *testing.T) {
	t.Parallel()

	store := NewMapContentStore(nil)
	bp := NewBatchProcessor(store, nil, 0)

	ops := make([]EditOp, MaxBatchFiles+1)
	for i := range ops {
		ops[i] = EditOp{
			FilePath:   fmt.Sprintf("file_%d.go", i),
			OldContent: "old",
			NewContent: "new",
		}
	}

	_, err := bp.Apply(ops)
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch exceeds maximum of 50 files")
}

func TestBatchProcessorMaxFilesCapExactBoundary(t *testing.T) {
	t.Parallel()

	t.Run("at_cap_succeeds", func(t *testing.T) {
		t.Parallel()

		initial := make(map[string]string, MaxBatchFiles)
		ops := make([]EditOp, MaxBatchFiles)
		for i := 0; i < MaxBatchFiles; i++ {
			path := fmt.Sprintf("file_%d.go", i)
			initial[path] = "old content"
			ops[i] = EditOp{
				FilePath:   path,
				OldContent: "old",
				NewContent: "new",
			}
		}

		store := NewMapContentStore(initial)
		bp := NewBatchProcessor(store, nil, 0)
		result, err := bp.Apply(ops)
		require.NoError(t, err)
		require.True(t, result.OverallSuccess)
	})

	t.Run("over_cap_fails", func(t *testing.T) {
		t.Parallel()

		ops := make([]EditOp, MaxBatchFiles+1)
		for i := range ops {
			ops[i] = EditOp{
				FilePath:   fmt.Sprintf("file_%d.go", i),
				OldContent: "old",
				NewContent: "new",
			}
		}

		store := NewMapContentStore(nil)
		bp := NewBatchProcessor(store, nil, 0)
		_, err := bp.Apply(ops)
		require.Error(t, err)
		require.Contains(t, err.Error(), "batch exceeds maximum of 50 files")
	})
}

func TestBatchProcessorPreEditDiagnostics(t *testing.T) {
	t.Parallel()

	mockDiags := func(_ context.Context, filePath string) []string {
		if filePath == "a.go" {
			return []string{"unused variable", "type mismatch"}
		}
		return nil
	}

	initial := map[string]string{
		"a.go": "package p",
		"b.go": "package q",
	}

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "package p", NewContent: "package main"},
		{FilePath: "b.go", OldContent: "package q", NewContent: "package main"},
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0).WithDiagnosticsCapture(mockDiags)
	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.NotNil(t, result.BaselineDiagnostics)
	require.Len(t, result.BaselineDiagnostics.Diags["a.go"], 2)
	_, hasB := result.BaselineDiagnostics.Diags["b.go"]
	require.False(t, hasB)
}

func TestBatchProcessorPreEditDiagnosticsUnavailable(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "package p",
	}

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "package p", NewContent: "package main"},
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0)
	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.NotNil(t, result.BaselineDiagnostics)
	require.Empty(t, result.BaselineDiagnostics.Diags)
}

func TestDetectOverlapsOverlappingEditsSameFile(t *testing.T) {
	t.Parallel()

	content := "aaa BBBB ccc"
	initial := map[string]string{"f.go": content}

	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "f.go", OldContent: "aaa BBBB", NewContent: "xxx"},
		{FilePath: "f.go", OldContent: "BBBB ccc", NewContent: "yyy"},
	}

	err := detectOverlaps(ops, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch overlap:")
	require.Contains(t, err.Error(), "overlap in f.go")
}

func TestDetectOverlapsInsertAtSamePositionFlagged(t *testing.T) {
	t.Parallel()

	content := "AAAAAA"
	initial := map[string]string{"f.go": content}

	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "f.go", OldContent: "AAA", NewContent: "xxx"},
		{FilePath: "f.go", OldContent: "AAAAAA", NewContent: "yyy"},
	}

	err := detectOverlaps(ops, store)
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch overlap:")
}

func TestDetectOverlapsNonOverlappingSameFile(t *testing.T) {
	t.Parallel()

	content := "aaa bbb ccc"
	initial := map[string]string{"f.go": content}

	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "f.go", OldContent: "aaa", NewContent: "xxx"},
		{FilePath: "f.go", OldContent: "ccc", NewContent: "yyy"},
	}

	err := detectOverlaps(ops, store)
	require.NoError(t, err)
}

func TestDetectOverlapsDifferentFilesNoConflict(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "same content",
		"b.go": "same content",
	}

	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "a.go", OldContent: "same content", NewContent: "xxx"},
		{FilePath: "b.go", OldContent: "same content", NewContent: "yyy"},
	}

	err := detectOverlaps(ops, store)
	require.NoError(t, err)
}

func TestDetectOverlapsSingleOpNoConflict(t *testing.T) {
	t.Parallel()

	initial := map[string]string{"f.go": "hello world"}
	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "f.go", OldContent: "hello", NewContent: "goodbye"},
	}

	err := detectOverlaps(ops, store)
	require.NoError(t, err)
}

func TestDetectOverlapsFileNotFoundSkipped(t *testing.T) {
	t.Parallel()

	initial := map[string]string{"a.go": "hello world"}
	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "a.go", OldContent: "hello", NewContent: "goodbye"},
		{FilePath: "missing.go", OldContent: "hello", NewContent: "goodbye"},
	}

	err := detectOverlaps(ops, store)
	require.NoError(t, err)
}

func TestDetectOverlapsOldContentNotFoundSkipped(t *testing.T) {
	t.Parallel()

	initial := map[string]string{"f.go": "hello world"}
	store := NewMapContentStore(initial)
	ops := []EditOp{
		{FilePath: "f.go", OldContent: "hello", NewContent: "goodbye"},
		{FilePath: "f.go", OldContent: "nonexistent", NewContent: "xxx"},
	}

	err := detectOverlaps(ops, store)
	require.NoError(t, err)
}

func TestBatchProcessorOverlapRejected(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"f.go": "aaa BBBB ccc",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0)

	ops := []EditOp{
		{FilePath: "f.go", OldContent: "aaa BBBB", NewContent: "xxx"},
		{FilePath: "f.go", OldContent: "BBBB ccc", NewContent: "yyy"},
	}

	_, err := bp.Apply(ops)
	require.Error(t, err)
	require.Contains(t, err.Error(), "batch overlap:")

	// Verify file was not modified.
	current, _ := store.Get("f.go")
	require.Equal(t, "aaa BBBB ccc", current)
}
