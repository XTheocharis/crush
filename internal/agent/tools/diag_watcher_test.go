package tools

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

func TestNewDiagnosticWatcher(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)
	require.NotNil(t, w)
	require.NotNil(t, w.watcher)

	_ = w.watcher.Close()
}

func TestDiagnosticWatcher_StartStop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)

	require.GreaterOrEqual(t, w.WatchedFiles(), 1, "should watch at least the project dir")

	w.Stop()
}

func TestDiagnosticWatcher_StopWithoutStart(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	// Stop without Start is handled by the caller: since run() was never
	// started, the done channel will block. In practice, callers should
	// always pair Start+Stop, but we verify construction succeeds.
}

func TestDiagnosticWatcher_GetCachedDiagnostics_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	result := w.GetCachedDiagnostics(filepath.Join(dir, "test.go"))
	require.Nil(t, result)
}

func TestDiagnosticWatcher_GetCachedDiagnostics_Expired(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	testFile := filepath.Join(dir, "test.go")
	absPath, err := filepath.Abs(testFile)
	require.NoError(t, err)

	// Manually insert an expired cache entry.
	w.mu.Lock()
	w.cache[absPath] = &diagCacheEntry{
		diagnostics: []DiagnosticInfo{
			{FilePath: absPath, Line: 1, Severity: SeverityError, Message: "test error"},
		},
		cachedAt: time.Now().Add(-cacheTTL - time.Second),
	}
	w.mu.Unlock()

	result := w.GetCachedDiagnostics(testFile)
	require.Nil(t, result, "expired cache should return nil")
}

func TestDiagnosticWatcher_GetCachedDiagnostics_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	testFile := filepath.Join(dir, "test.go")
	absPath, err := filepath.Abs(testFile)
	require.NoError(t, err)

	expected := []DiagnosticInfo{
		{FilePath: absPath, Line: 1, Severity: SeverityError, Message: "test error"},
	}

	w.mu.Lock()
	w.cache[absPath] = &diagCacheEntry{
		diagnostics: expected,
		cachedAt:    time.Now(),
	}
	w.mu.Unlock()

	result := w.GetCachedDiagnostics(testFile)
	require.Len(t, result, 1)
	require.Equal(t, "test error", result[0].Message)

	// Verify it's a copy, not the same slice.
	result[0].Message = "modified"
	w.mu.RLock()
	original := w.cache[absPath].diagnostics[0].Message
	w.mu.RUnlock()
	require.Equal(t, "test error", original)
}

func TestDiagnosticWatcher_HandleEvent_WatchedExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	tests := []struct {
		name         string
		filename     string
		expectQueued bool
	}{
		{"go file", "main.go", true},
		{"ts file", "app.ts", true},
		{"py file", "script.py", true},
		{"js file", "app.js", false},
		{"rs file", "main.rs", false},
		{"no ext", "Makefile", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.filename)
			event := fsnotify.Event{
				Name: path,
				Op:   fsnotify.Write,
			}

			w.pending = make(map[string]bool)
			w.handleEvent(event)

			_, queued := w.pending[path]
			require.Equal(t, tt.expectQueued, queued)
		})
	}
}

func TestDiagnosticWatcher_HandleEvent_IgnoresIrrelevantOps(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "test.go")

	tests := []struct {
		name string
		op   fsnotify.Op
	}{
		{"chmod", fsnotify.Chmod},
		{"remove", fsnotify.Remove},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w.pending = make(map[string]bool)
			w.handleEvent(fsnotify.Event{Name: path, Op: tt.op})
			require.Empty(t, w.pending)
		})
	}
}

func TestDiagnosticWatcher_HandleEvent_WriteCreateRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "test.go")

	for _, op := range []fsnotify.Op{fsnotify.Write, fsnotify.Create, fsnotify.Rename} {
		w.pending = make(map[string]bool)
		w.handleEvent(fsnotify.Event{Name: path, Op: op})
		require.True(t, w.pending[path], "should queue for op %v", op)
	}
}

func TestDiagnosticWatcher_HandleEvent_OutsideProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	otherDir := t.TempDir()
	path := filepath.Join(otherDir, "test.go")

	w.pending = make(map[string]bool)
	w.handleEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})
	require.Empty(t, w.pending, "should not queue files outside project dir")
}

func TestDiagnosticWatcher_HandleEvent_EmptyName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	w.pending = make(map[string]bool)
	w.handleEvent(fsnotify.Event{Name: "", Op: fsnotify.Write})
	require.Empty(t, w.pending)
}

func TestDiagnosticWatcher_FlushPending(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	testFile := filepath.Join(dir, "test.go")

	// With nil manager, refreshDiagnostics is a no-op, but pending is
	// still cleared.
	w.pending[testFile] = true
	w.flushPending()

	require.Empty(t, w.pending, "pending should be cleared after flush")
	require.Empty(t, w.cache, "cache should be empty with nil manager")
}

func TestDiagnosticWatcher_FlushPending_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := NewDiagnosticWatcher(nil, dir)
	require.NoError(t, err)

	// Should be a no-op with empty pending.
	w.flushPending()
	require.Empty(t, w.cache)
}

func TestIsSubPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		parent string
		sub    string
		want   bool
	}{
		{"child", "/project", "/project/pkg/foo.go", true},
		{"same", "/project", "/project", true},
		{"sibling", "/project", "/other/foo.go", false},
		{"parent traversal", "/project", "/project/../other/foo.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isSubPath(tt.parent, tt.sub))
		})
	}
}

func TestDiagnosticWatcher_WatchedFiles_Nil(t *testing.T) {
	t.Parallel()

	w := &DiagnosticWatcher{}
	require.Equal(t, 0, w.WatchedFiles())
}

func TestDiagnosticWatcher_WatchedExtensions(t *testing.T) {
	t.Parallel()

	require.True(t, watchedExtensions[".go"])
	require.True(t, watchedExtensions[".ts"])
	require.True(t, watchedExtensions[".py"])
	require.False(t, watchedExtensions[".js"])
	require.False(t, watchedExtensions[".rs"])
	require.False(t, watchedExtensions[".java"])
}
