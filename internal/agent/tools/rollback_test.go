package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRollbackManager_CaptureEmptyPaths(t *testing.T) {
	t.Parallel()

	rm := NewRollbackManager()
	snap, err := rm.Capture(nil)
	require.NoError(t, err)
	require.Empty(t, snap.Files)
	require.False(t, snap.CapturedAt.IsZero())

	snap, err = rm.Capture([]string{})
	require.NoError(t, err)
	require.Empty(t, snap.Files)
}

func TestRollbackManager_CaptureExistingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")

	require.NoError(t, os.WriteFile(fileA, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("world"), 0o644))

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{fileA, fileB})
	require.NoError(t, err)
	require.Len(t, snap.Files, 2)
	require.Equal(t, "hello", snap.Files[0].Content)
	require.Equal(t, "world", snap.Files[1].Content)
	require.False(t, snap.Files[0].ModTime.IsZero())
	require.False(t, snap.Files[1].ModTime.IsZero())
}

func TestRollbackManager_CaptureNonExistentFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "nonexistent.txt")

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{missing})
	require.NoError(t, err)
	require.Len(t, snap.Files, 1)
	require.Equal(t, "", snap.Files[0].Content)
	require.True(t, snap.Files[0].ModTime.IsZero())
}

func TestRollbackManager_FullRollback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")

	require.NoError(t, os.WriteFile(fileA, []byte("original-a"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("original-b"), 0o644))

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{fileA, fileB})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(fileA, []byte("modified-a"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("modified-b"), 0o644))

	require.NoError(t, rm.Restore(snap))

	contentA, err := os.ReadFile(fileA)
	require.NoError(t, err)
	require.Equal(t, "original-a", string(contentA))

	contentB, err := os.ReadFile(fileB)
	require.NoError(t, err)
	require.Equal(t, "original-b", string(contentB))
}

func TestRollbackManager_RestoreNilSnapshot(t *testing.T) {
	t.Parallel()

	rm := NewRollbackManager()
	require.Error(t, rm.Restore(nil))
}

func TestRollbackManager_RestorePartialNilSnapshot(t *testing.T) {
	t.Parallel()

	rm := NewRollbackManager()
	require.Error(t, rm.RestorePartial(nil, []string{"foo.txt"}))
}

func TestRollbackManager_RollbackRemovesNewlyCreatedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createdLater := filepath.Join(dir, "new.txt")

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{createdLater})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(createdLater, []byte("unexpected"), 0o644))

	require.NoError(t, rm.Restore(snap))

	_, err = os.Stat(createdLater)
	require.True(t, os.IsNotExist(err), "newly created file should be removed on rollback")
}

func TestRollbackManager_PartialRollback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	fileC := filepath.Join(dir, "c.txt")

	require.NoError(t, os.WriteFile(fileA, []byte("orig-a"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("orig-b"), 0o644))
	require.NoError(t, os.WriteFile(fileC, []byte("orig-c"), 0o644))

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{fileA, fileB, fileC})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(fileA, []byte("mod-a"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("mod-b"), 0o644))
	require.NoError(t, os.WriteFile(fileC, []byte("mod-c"), 0o644))

	require.NoError(t, rm.RestorePartial(snap, []string{fileA, fileC}))

	contentA, err := os.ReadFile(fileA)
	require.NoError(t, err)
	require.Equal(t, "orig-a", string(contentA))

	contentB, err := os.ReadFile(fileB)
	require.NoError(t, err)
	require.Equal(t, "mod-b", string(contentB), "file B should NOT be rolled back")

	contentC, err := os.ReadFile(fileC)
	require.NoError(t, err)
	require.Equal(t, "orig-c", string(contentC))
}

func TestRollbackManager_PartialRollback_FileNotInSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("content"), 0o644))

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{fileA})
	require.NoError(t, err)

	err = rm.RestorePartial(snap, []string{fileA, filepath.Join(dir, "missing.txt")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "files not in snapshot")
}

func TestRollbackManager_SnapshotIntegrity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("original"), 0o644))

	rm := NewRollbackManager()

	snap, err := rm.Capture([]string{fileA})
	require.NoError(t, err)
	require.Equal(t, "original", snap.Files[0].Content)

	require.NoError(t, os.WriteFile(fileA, []byte("modified"), 0o644))

	require.NoError(t, rm.Restore(snap))

	content, err := os.ReadFile(fileA)
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
}

func TestRollbackManager_CaptureStatError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(filePath, 0o755))

	rm := NewRollbackManager()
	_, err := rm.Capture([]string{filePath})
	require.Error(t, err)
}

func TestRollbackManager_ModTimeRestored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "timed.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))

	rm := NewRollbackManager()
	snap, err := rm.Capture([]string{filePath})
	require.NoError(t, err)
	capturedModTime := snap.Files[0].ModTime

	require.NoError(t, os.Chtimes(filePath, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))
	require.NoError(t, os.WriteFile(filePath, []byte("modified"), 0o644))

	require.NoError(t, rm.Restore(snap))

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "content", string(content))

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, capturedModTime, info.ModTime(), "ModTime should be restored to captured value")
}

func TestDiffDiagnostics_NewErrors(t *testing.T) {
	t.Parallel()

	before := map[string][]DiagnosticInfo{
		"main.go": {
			{FilePath: "main.go", Line: 10, Message: "unused variable", Severity: SeverityWarning},
		},
	}
	after := map[string][]DiagnosticInfo{
		"main.go": {
			{FilePath: "main.go", Line: 10, Message: "unused variable", Severity: SeverityWarning},
			{FilePath: "main.go", Line: 25, Message: "undefined: x", Severity: SeverityError},
		},
		"util.go": {
			{FilePath: "util.go", Line: 5, Message: "type mismatch", Severity: SeverityError},
		},
	}

	delta := DiffDiagnostics(before, after)
	require.Len(t, delta.NewErrors, 2)
	require.Empty(t, delta.ResolvedErrors)
	require.Equal(t, 2, delta.NetChange)
}

func TestDiffDiagnostics_ResolvedErrors(t *testing.T) {
	t.Parallel()

	before := map[string][]DiagnosticInfo{
		"main.go": {
			{FilePath: "main.go", Line: 10, Message: "unused variable", Severity: SeverityWarning},
			{FilePath: "main.go", Line: 25, Message: "undefined: x", Severity: SeverityError},
		},
	}
	after := map[string][]DiagnosticInfo{
		"main.go": {
			{FilePath: "main.go", Line: 10, Message: "unused variable", Severity: SeverityWarning},
		},
	}

	delta := DiffDiagnostics(before, after)
	require.Empty(t, delta.NewErrors)
	require.Len(t, delta.ResolvedErrors, 1)
	require.Equal(t, "undefined: x", delta.ResolvedErrors[0].Message)
	require.Equal(t, -1, delta.NetChange)
}

func TestDiffDiagnostics_NoChange(t *testing.T) {
	t.Parallel()

	snapshot := map[string][]DiagnosticInfo{
		"main.go": {
			{FilePath: "main.go", Line: 10, Message: "unused variable", Severity: SeverityWarning},
		},
	}

	delta := DiffDiagnostics(snapshot, snapshot)
	require.Empty(t, delta.NewErrors)
	require.Empty(t, delta.ResolvedErrors)
	require.Equal(t, 0, delta.NetChange)
}

func TestDiffDiagnostics_NetChange(t *testing.T) {
	t.Parallel()

	before := map[string][]DiagnosticInfo{
		"a.go": {
			{FilePath: "a.go", Line: 1, Message: "err1", Severity: SeverityError},
			{FilePath: "a.go", Line: 2, Message: "err2", Severity: SeverityError},
			{FilePath: "a.go", Line: 3, Message: "err3", Severity: SeverityError},
		},
	}
	after := map[string][]DiagnosticInfo{
		"a.go": {
			{FilePath: "a.go", Line: 1, Message: "err1", Severity: SeverityError},
			{FilePath: "a.go", Line: 10, Message: "new_err", Severity: SeverityError},
		},
	}

	delta := DiffDiagnostics(before, after)
	require.Len(t, delta.NewErrors, 1)
	require.Len(t, delta.ResolvedErrors, 2)
	require.Equal(t, -1, delta.NetChange)
}

func TestShouldRollback_NewErrors(t *testing.T) {
	t.Parallel()

	delta := &DiagnosticDelta{
		NewErrors:      []DiagnosticInfo{{FilePath: "main.go", Line: 1, Message: "bad", Severity: SeverityError}},
		ResolvedErrors: nil,
		NetChange:      1,
	}
	require.True(t, ShouldRollback(delta))
}

func TestShouldRollback_NoNewErrors(t *testing.T) {
	t.Parallel()

	delta := &DiagnosticDelta{
		NewErrors:      nil,
		ResolvedErrors: []DiagnosticInfo{{FilePath: "main.go", Line: 1, Message: "fixed", Severity: SeverityError}},
		NetChange:      -1,
	}
	require.False(t, ShouldRollback(delta))
}
