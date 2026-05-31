package tools

import (
	"context"
	"os"
	"path/filepath"
	"sync"
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

// mockSnapshotter records calls to CaptureSnapshot for test assertions.
type mockSnapshotter struct {
	mu    sync.Mutex
	calls []captureCall
	err   error
}

type captureCall struct {
	sessionID string
	seq       int
}

func (m *mockSnapshotter) CaptureSnapshot(_ context.Context, sessionID string, seq int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, captureCall{sessionID: sessionID, seq: seq})
	return nil
}

func (m *mockSnapshotter) getCalls() []captureCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]captureCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestRollbackManager_Persistence_NoSnapshotter(t *testing.T) {
	t.Parallel()

	rm := NewRollbackManager()
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("hello"), 0o644))

	snap, err := rm.Capture([]string{fileA})
	require.NoError(t, err)
	require.NotNil(t, snap)

	// History is empty when no snapshotter is configured.
	require.Empty(t, rm.History())
}

func TestRollbackManager_Persistence_WithSnapshotter(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-1")

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("world"), 0o644))

	snap, err := rm.Capture([]string{fileA, fileB})
	require.NoError(t, err)
	require.Len(t, snap.Files, 2)

	// Snapshotter was called.
	calls := mock.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "session-1", calls[0].sessionID)

	// History was recorded.
	history := rm.History()
	require.Len(t, history, 1)
	require.Equal(t, "file capture", history[0].Reason)
	require.Len(t, history[0].Files, 2)
	require.Contains(t, history[0].Files, fileA)
	require.Contains(t, history[0].Files, fileB)
	require.False(t, history[0].CreatedAt.IsZero())
}

func TestRollbackManager_Persistence_EmptyCapture(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-2")

	snap, err := rm.Capture(nil)
	require.NoError(t, err)
	require.Empty(t, snap.Files)

	// Snapshotter was still called.
	calls := mock.getCalls()
	require.Len(t, calls, 1)

	history := rm.History()
	require.Len(t, history, 1)
	require.Equal(t, "empty capture", history[0].Reason)
}

func TestRollbackManager_Persistence_SnapshotterError(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{err: context.DeadlineExceeded}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-3")

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("data"), 0o644))

	// Capture succeeds even when persistence fails.
	snap, err := rm.Capture([]string{fileA})
	require.NoError(t, err)
	require.Len(t, snap.Files, 1)
	require.Equal(t, "data", snap.Files[0].Content)

	// History is empty because persistence failed.
	require.Empty(t, rm.History())
}

func TestRollbackManager_Persistence_MultipleCaptures(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-4")

	dir := t.TempDir()

	for range 3 {
		fp := filepath.Join(dir, "file.txt")
		require.NoError(t, os.WriteFile(fp, []byte("content"), 0o644))
		_, err := rm.Capture([]string{fp})
		require.NoError(t, err)
	}

	require.Len(t, mock.getCalls(), 3)
	require.Len(t, rm.History(), 3)
}

func TestRollbackManager_Persistence_SetSnapshotterNil(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-5")

	dir := t.TempDir()
	fp := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o644))

	_, err := rm.Capture([]string{fp})
	require.NoError(t, err)
	require.Len(t, rm.History(), 1)

	// Setting to nil disables persistence.
	rm.SetSnapshotter(nil, "")
	_, err = rm.Capture([]string{fp})
	require.NoError(t, err)
	require.Len(t, mock.getCalls(), 1) // No additional call.
	require.Len(t, rm.History(), 1)    // No additional entry.
}

func TestRollbackManager_Persistence_RollbackStillWorks(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-6")

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fileA, []byte("original"), 0o644))

	snap, err := rm.Capture([]string{fileA})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(fileA, []byte("modified"), 0o644))

	// Restore still works with persistence enabled.
	require.NoError(t, rm.Restore(snap))

	content, err := os.ReadFile(fileA)
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
}

func TestRollbackManager_Persistence_HistoryIsCopy(t *testing.T) {
	t.Parallel()

	mock := &mockSnapshotter{}
	rm := NewRollbackManager()
	rm.SetSnapshotter(mock, "session-7")

	dir := t.TempDir()
	fp := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o644))

	_, err := rm.Capture([]string{fp})
	require.NoError(t, err)

	h1 := rm.History()
	h2 := rm.History()
	// Two calls return distinct slices.
	require.Equal(t, h1, h2)
	require.NotSame(t, &h1[0], &h2[0])
}
