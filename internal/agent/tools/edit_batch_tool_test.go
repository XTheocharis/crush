package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertBatchOps_StringReplacement(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", OldContent: "old_a", NewContent: "new_a"},
		{FilePath: "b.go", OldContent: "old_b", NewContent: "new_b"},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	require.Equal(t, "a.go", ops[0].FilePath)
	require.Equal(t, "old_a", ops[0].OldContent)
	require.Equal(t, "new_a", ops[0].NewContent)
	require.Equal(t, "b.go", ops[1].FilePath)
}

func TestConvertBatchOps_InsertBefore(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{
			FilePath:   "a.go",
			Op:         "insert_before",
			AnchorHash: "a1b2c3d4",
			Content:    "inserted code",
		},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, "a.go", ops[0].FilePath)
	require.Equal(t, "inserted code", ops[0].NewContent)
}

func TestConvertBatchOps_InsertAfter(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{
			FilePath:   "b.go",
			Op:         "insert_after",
			AnchorHash: "deadbeef",
			Content:    "after code",
		},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, "b.go", ops[0].FilePath)
}

func TestConvertBatchOps_ReplaceRange(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{
			FilePath:  "a.go",
			Op:        "replace_range",
			StartHash: "aaaa0000",
			EndHash:   "bbbb0000",
			Content:   "replacement",
		},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, "replacement", ops[0].NewContent)
}

func TestConvertBatchOps_DeleteRange(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{
			FilePath:  "a.go",
			Op:        "delete_range",
			StartHash: "aaaa0000",
			EndHash:   "bbbb0000",
		},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, "", ops[0].OldContent)
	require.Equal(t, "", ops[0].NewContent)
}

func TestConvertBatchOps_NoOpTypeButHasOldNewContent(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", OldContent: "old", NewContent: "new"},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.Equal(t, "old", ops[0].OldContent)
	require.Equal(t, "new", ops[0].NewContent)
}

func TestConvertBatchOps_ErrorNoOpNoContent(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must specify op type or old_content/new_content")
}

func TestConvertBatchOps_ErrorInsertWithoutAnchorHash(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "insert_before", Content: "code"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "anchor_hash required")
}

func TestConvertBatchOps_ErrorInsertAfterWithoutAnchorHash(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "insert_after", Content: "code"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "anchor_hash required for insert_after")
}

func TestConvertBatchOps_ErrorInsertWithoutContent(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "insert_before", AnchorHash: "a1b2c3d4"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "content is required")
}

func TestConvertBatchOps_ErrorReplaceRangeMissingEndHash(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "replace_range", StartHash: "aaaa0000"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start_hash and end_hash required for replace_range")
}

func TestConvertBatchOps_ErrorReplaceRangeMissingStartHash(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "replace_range", EndHash: "bbbb0000"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start_hash and end_hash required for replace_range")
}

func TestConvertBatchOps_ErrorDeleteRangeMissingHashes(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "delete_range", StartHash: "aaaa"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start_hash and end_hash required for delete_range")
}

func TestConvertBatchOps_ErrorUnknownOp(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", Op: "teleport"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown operation")
}

func TestConvertBatchOps_MixedOps(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", OldContent: "old", NewContent: "new"},
		{FilePath: "b.go", Op: "insert_before", AnchorHash: "a1b2c3d4", Content: "inserted"},
		{FilePath: "c.go", Op: "delete_range", StartHash: "s1", EndHash: "e1"},
	}

	ops, err := convertBatchOps(params)
	require.NoError(t, err)
	require.Len(t, ops, 3)

	require.Equal(t, "a.go", ops[0].FilePath)
	require.Equal(t, "old", ops[0].OldContent)

	require.Equal(t, "b.go", ops[1].FilePath)
	require.Equal(t, "inserted", ops[1].NewContent)

	require.Equal(t, "c.go", ops[2].FilePath)
	require.Equal(t, "", ops[2].OldContent)
}

func TestConvertBatchOps_ErrorIncludesIndex(t *testing.T) {
	t.Parallel()

	params := []BatchEditOpParams{
		{FilePath: "a.go", OldContent: "old", NewContent: "new"},
		{FilePath: "b.go"},
	}

	_, err := convertBatchOps(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "op 1:")
}

func TestBatchProcessor_AtomicSuccess(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "package main\nfunc main() {}\n",
		"b.go": "package util\nfunc helper() {}\n",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0)

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "package main", NewContent: "package edited"},
		{FilePath: "b.go", OldContent: "func helper", NewContent: "func Helper"},
	}

	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.False(t, result.RolledBack)
	require.Len(t, result.PerOpResults, 2)
	require.True(t, result.PerOpResults[0].Success)
	require.True(t, result.PerOpResults[1].Success)

	content, ok := store.Get("a.go")
	require.True(t, ok)
	require.Contains(t, content, "package edited")

	content, ok = store.Get("b.go")
	require.True(t, ok)
	require.Contains(t, content, "func Helper")
}

func TestBatchProcessor_PartialFailureRollback(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "original_a",
		"b.go": "original_b",
		"c.go": "original_c",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0)

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "original_a", NewContent: "modified_a"},
		{FilePath: "b.go", OldContent: "nonexistent", NewContent: "modified_b"},
	}

	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.False(t, result.OverallSuccess)
	require.True(t, result.RolledBack)
	require.True(t, result.PerOpResults[0].Success)
	require.False(t, result.PerOpResults[1].Success)

	content, ok := store.Get("a.go")
	require.True(t, ok)
	require.Equal(t, "original_a", content, "file a should be rolled back")

	content, ok = store.Get("b.go")
	require.True(t, ok)
	require.Equal(t, "original_b", content, "file b should be unchanged")

	content, ok = store.Get("c.go")
	require.True(t, ok)
	require.Equal(t, "original_c", content, "file c should be unchanged")
}

func TestBatchProcessor_FileNotFoundRollback(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "original_a",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 0)

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "original_a", NewContent: "modified_a"},
		{FilePath: "missing.go", OldContent: "x", NewContent: "y"},
	}

	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.False(t, result.OverallSuccess)
	require.True(t, result.RolledBack)

	content, _ := store.Get("a.go")
	require.Equal(t, "original_a", content, "a.go should be rolled back when missing.go fails")
}

func TestBatchProcessor_EmptyOps(t *testing.T) {
	t.Parallel()

	store := NewMapContentStore(nil)
	bp := NewBatchProcessor(store, nil, 0)

	result, err := bp.Apply(nil)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.False(t, result.RolledBack)
	require.Nil(t, result.PerOpResults)
}

func TestBatchProcessor_AnchorMapsRebuilt(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "word word word word unique0\n",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 5)

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "unique0", NewContent: "EDITED"},
	}

	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.NotNil(t, result.AnchorMaps)
	require.Contains(t, result.AnchorMaps, "a.go")
	require.NotEmpty(t, result.AnchorMaps["a.go"].Anchors)
}

func TestBatchProcessor_AllOpsSucceed_ThenAnchorMapsHashRestoration(t *testing.T) {
	t.Parallel()

	initial := map[string]string{
		"a.go": "word word word word start\nword word word word end\n",
		"b.go": "word word word word alpha\nword word word word beta\n",
	}

	store := NewMapContentStore(initial)
	bp := NewBatchProcessor(store, nil, 5)

	ops := []EditOp{
		{FilePath: "a.go", OldContent: "start", NewContent: "START"},
		{FilePath: "b.go", OldContent: "alpha", NewContent: "ALPHA"},
	}

	result, err := bp.Apply(ops)
	require.NoError(t, err)
	require.True(t, result.OverallSuccess)
	require.Len(t, result.AnchorMaps, 2)

	for fp, am := range result.AnchorMaps {
		require.NotEmpty(t, am.Anchors, "anchor map for %s should have anchors", fp)
	}
}
