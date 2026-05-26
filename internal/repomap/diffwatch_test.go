//go:build treesitter
// +build treesitter

package repomap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiffWatcherParseDiffOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  map[string]struct{}
	}{
		{
			name:  "empty input",
			input: "",
			want:  map[string]struct{}{},
		},
		{
			name:  "single file",
			input: "foo.go",
			want:  map[string]struct{}{"foo.go": {}},
		},
		{
			name:  "multiple files",
			input: "a.go\nb.go\nc.go",
			want: map[string]struct{}{
				"a.go": {},
				"b.go": {},
				"c.go": {},
			},
		},
		{
			name:  "blank lines are skipped",
			input: "a.go\n\nb.go\n\n",
			want: map[string]struct{}{
				"a.go": {},
				"b.go": {},
			},
		},
		{
			name:  "whitespace is trimmed",
			input: "  a.go  \n  b.go\t\n",
			want: map[string]struct{}{
				"a.go": {},
				"b.go": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseDiffOutput([]byte(tt.input))
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDiffWatcherInvalidatesCaches(t *testing.T) {
	t.Parallel()

	renderCaches := NewSessionRenderCacheSet()
	sessionCaches := NewSessionCacheSet()

	rc := renderCaches.GetOrCreate("sess1")
	rc.Set("key1", "map-content", 100)
	sessionCaches.Store("sess1", "session-map", 200)

	var pollCount atomic.Int32
	mockCmd := func(_ context.Context, _ string) ([]byte, error) {
		pollCount.Add(1)
		return []byte("changed_file.go"), nil
	}

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:       t.TempDir(),
		Interval:      10 * time.Millisecond,
		RenderCaches:  renderCaches,
		SessionCaches: sessionCaches,
		GitDiffCmd:    mockCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dw.Start(ctx)

	require.Eventually(t, func() bool {
		return pollCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	dw.Stop()

	rc2 := renderCaches.GetOrCreate("sess1")
	_, _, ok := rc2.Get("key1")
	require.False(t, ok, "render cache entry should be invalidated")

	m, tok := sessionCaches.Load("sess1")
	require.False(t, m != "" || tok != 0, "session cache should be invalidated")
}

func TestDiffWatcherNoChangesSkipsInvalidation(t *testing.T) {
	t.Parallel()

	renderCaches := NewSessionRenderCacheSet()
	sessionCaches := NewSessionCacheSet()

	rc := renderCaches.GetOrCreate("sess1")
	rc.Set("key1", "map-content", 100)
	sessionCaches.Store("sess1", "session-map", 200)

	mockCmd := func(_ context.Context, _ string) ([]byte, error) {
		return []byte(""), nil
	}

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:       t.TempDir(),
		Interval:      10 * time.Millisecond,
		RenderCaches:  renderCaches,
		SessionCaches: sessionCaches,
		GitDiffCmd:    mockCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	dw.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	dw.Stop()

	m, tok, ok := rc.Get("key1")
	require.True(t, ok)
	require.Equal(t, "map-content", m)
	require.Equal(t, 100, tok)

	sm, stok := sessionCaches.Load("sess1")
	require.NotEmpty(t, sm)
	require.Equal(t, "session-map", sm)
	require.Equal(t, 200, stok)
}

func TestDiffWatcherGitErrorNoop(t *testing.T) {
	t.Parallel()

	renderCaches := NewSessionRenderCacheSet()
	sessionCaches := NewSessionCacheSet()

	rc := renderCaches.GetOrCreate("sess1")
	rc.Set("key1", "map-content", 100)

	mockCmd := func(_ context.Context, _ string) ([]byte, error) {
		return nil, errGitNotAvailable
	}

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:       t.TempDir(),
		Interval:      10 * time.Millisecond,
		RenderCaches:  renderCaches,
		SessionCaches: sessionCaches,
		GitDiffCmd:    mockCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	dw.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	dw.Stop()

	m, tok, ok := rc.Get("key1")
	require.True(t, ok)
	require.Equal(t, "map-content", m)
	require.Equal(t, 100, tok)
}

func TestDiffWatcherChangedFiles(t *testing.T) {
	t.Parallel()

	mockCmd := func(_ context.Context, _ string) ([]byte, error) {
		return []byte("a.go\nb.go"), nil
	}

	var pollCount atomic.Int32
	mockCmdTracking := func(ctx context.Context, dir string) ([]byte, error) {
		pollCount.Add(1)
		return mockCmd(ctx, dir)
	}

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:    t.TempDir(),
		Interval:   10 * time.Millisecond,
		GitDiffCmd: mockCmdTracking,
	})

	ctx, cancel := context.WithCancel(context.Background())
	dw.Start(ctx)

	require.Eventually(t, func() bool {
		return pollCount.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	dw.Stop()

	changed := dw.ChangedFiles()
	require.Len(t, changed, 2)
	changedMap := map[string]struct{}{}
	for _, f := range changed {
		changedMap[f] = struct{}{}
	}
	require.Contains(t, changedMap, "a.go")
	require.Contains(t, changedMap, "b.go")
}

func TestDiffWatcherStopWithoutStart(t *testing.T) {
	t.Parallel()

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir: t.TempDir(),
	})
	dw.Stop()
}

func TestDiffWatcherDoubleStart(t *testing.T) {
	t.Parallel()

	callCount := atomic.Int32{}
	mockCmd := func(_ context.Context, _ string) ([]byte, error) {
		callCount.Add(1)
		return []byte(""), nil
	}

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:    t.TempDir(),
		Interval:   10 * time.Millisecond,
		GitDiffCmd: mockCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dw.Start(ctx)
	dw.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	cancel()
	dw.Stop()

	require.LessOrEqual(t, callCount.Load(), int32(20), "double start should not spawn two goroutines")
}

func TestDiffWatcherDefaultInterval(t *testing.T) {
	t.Parallel()

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir: t.TempDir(),
	})
	require.Equal(t, 30*time.Second, dw.interval)
}

func TestDiffWatcherCustomInterval(t *testing.T) {
	t.Parallel()

	dw := NewDiffWatcher(DiffWatcherConfig{
		RootDir:  t.TempDir(),
		Interval: 5 * time.Second,
	})
	require.Equal(t, 5*time.Second, dw.interval)
}

var errGitNotAvailable = errors.New("git not available")
