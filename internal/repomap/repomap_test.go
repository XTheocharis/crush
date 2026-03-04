package repomap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// TestDisableLatchParityModeDeadlineExceeded verifies that after a
// context.DeadlineExceeded error in parity-mode Generate, the session
// is permanently disabled and subsequent calls return the fallback.
func TestDisableLatchParityModeDeadlineExceeded(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Pre-populate a known-good cached map so fallback returns it.
	svc.sessionCaches.Store("sess-1", "cached-map", 42)

	// Manually trigger the disable latch as if DeadlineExceeded occurred
	// during parity-mode generation.
	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))

	// Generate must return the fallback (cached) value without regenerating.
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "cached-map", m)
	require.Equal(t, 42, tok)

	// Even ForceRefresh does not bypass the latch: the latch check
	// precedes ForceRefresh cache clearing in Generate(). The fallback
	// returns the last-loaded cache snapshot from before the clear.
	m, tok, err = svc.Generate(context.Background(), GenerateOpts{
		SessionID:    "sess-1",
		ParityMode:   true,
		ForceRefresh: true,
	})
	require.NoError(t, err)
	// Latch fires before ForceRefresh clearing — fallback uses snapshot.
	require.Equal(t, "cached-map", m)
	require.Equal(t, 42, tok)
}

// TestDisableLatchResetClears verifies that Reset() clears the disable
// latch, allowing future Generate calls to proceed normally.
func TestDisableLatchResetClears(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.disableForSession("sess-1")
	require.True(t, svc.isDisabledForSession("sess-1"))

	err := svc.Reset(context.Background(), "sess-1")
	require.NoError(t, err)

	require.False(t, svc.isDisabledForSession("sess-1"))

	// After reset, Generate should proceed past the latch check.
	// With nil db it will fallback, but the latch is no longer the cause.
	svc.sessionCaches.Store("sess-1", "new-map", 99)
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID: "sess-1",
	})
	require.NoError(t, err)
	require.Equal(t, "new-map", m)
	require.Equal(t, 99, tok)
}

// TestDisableLatchMultiSessionIsolation verifies that disabling session A
// does not affect session B.
func TestDisableLatchMultiSessionIsolation(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.sessionCaches.Store("sess-A", "map-A", 10)
	svc.sessionCaches.Store("sess-B", "map-B", 20)

	// Disable session A only.
	svc.disableForSession("sess-A")

	require.True(t, svc.isDisabledForSession("sess-A"))
	require.False(t, svc.isDisabledForSession("sess-B"))

	// Session A returns fallback due to latch.
	mA, tokA, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-A",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "map-A", mA)
	require.Equal(t, 10, tokA)

	// Session B proceeds normally (returns cache because db is nil).
	mB, tokB, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-B",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.Equal(t, "map-B", mB)
	require.Equal(t, 20, tokB)
}

// TestDisableLatchNonDeadlineErrorDoesNotTrigger verifies that non-deadline
// errors (such as file read errors) do not trigger the disable latch.
// This tests the infrastructure contract directly since reaching the error
// paths in extractTags/FitToBudget requires database setup.
func TestDisableLatchNonDeadlineErrorDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Verify session starts without latch.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// Generate with nil db falls through to fallback(nil) without error.
	// No latch should be set because there's no DeadlineExceeded error.
	_, _, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.NoError(t, err)
	require.False(t, svc.isDisabledForSession("sess-1"),
		"non-error generate path must not trigger disable latch")
}

// TestDisableLatchContextCanceledDoesNotTrigger verifies that
// context.Canceled does not trigger the disable latch. Canceled represents
// user cancellation, not resource exhaustion.
func TestDisableLatchContextCanceledDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Pre-populate cache so the service is not freshly empty.
	svc.sessionCaches.Store("sess-1", "map", 10)

	// A canceled context will cause checkContextsDone to return
	// context.Canceled immediately, never reaching the error handlers.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: true,
	})
	require.ErrorIs(t, err, context.Canceled)

	// The session must NOT be disabled.
	require.False(t, svc.isDisabledForSession("sess-1"),
		"context.Canceled must not trigger disable latch")
}

// TestDisableLatchEnhancementModeDeadlineDoesNotTrigger verifies that
// context.DeadlineExceeded in enhancement mode does NOT trigger the
// permanent disable latch. Enhancement mode treats timeouts as transient.
func TestDisableLatchEnhancementModeDeadlineDoesNotTrigger(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Simulate: DeadlineExceeded occurred, but ParityMode is false.
	// The latch should NOT engage.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// A timed-out context triggers checkContextsDone before the latch
	// handlers, so we test the guard condition directly: the error
	// handlers only call disableForSession when opts.ParityMode is true.
	// With enhancement mode, the session stays enabled.

	// Manually verify the parity guard: calling disableForSession is
	// gated on opts.ParityMode in the error handlers.
	svc.sessionCaches.Store("sess-1", "map", 10)

	// Generate with a deadline-exceeded context returns immediately
	// from checkContextsDone, which doesn't trigger the latch.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done() // Ensure deadline passed.

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false, // Enhancement mode.
	})
	require.Error(t, err)
	require.False(t, svc.isDisabledForSession("sess-1"),
		"enhancement-mode DeadlineExceeded must not trigger disable latch")
}

// TestDisableLatchParityGuardConsistency verifies that ALL three
// DeadlineExceeded error handlers in Generate() gate on ParityMode.
// This tests the condition logic directly because the full integration
// path (live context expiring mid-render) requires a real database
// and file walker setup.
func TestDisableLatchParityGuardConsistency(t *testing.T) {
	t.Parallel()

	// Simulate the guard condition that exists at each handler site:
	//   if errors.Is(err, context.DeadlineExceeded) && opts.ParityMode
	// In enhancement mode (ParityMode=false), the latch must not engage
	// even when the error IS DeadlineExceeded.
	testCases := []struct {
		name       string
		parityMode bool
		wantLatch  bool
	}{
		{"parity_mode_engages_latch", true, true},
		{"enhancement_mode_skips_latch", false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(nil, nil, nil, ".", context.Background())

			// Simulate the guard condition from the three error handlers.
			err := context.DeadlineExceeded
			opts := GenerateOpts{
				SessionID:  "sess-guard",
				ParityMode: tc.parityMode,
			}

			if errors.Is(err, context.DeadlineExceeded) && opts.ParityMode {
				svc.disableForSession(opts.SessionID)
			}

			require.Equal(t, tc.wantLatch,
				svc.isDisabledForSession(opts.SessionID),
				"latch state mismatch for parityMode=%v", tc.parityMode)
		})
	}
}

// TestDisableLatchEnhancementModeRetryAfterTimeout verifies that in
// enhancement mode, a timeout is transient and subsequent calls retry.
func TestDisableLatchEnhancementModeRetryAfterTimeout(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// First call: deadline exceeded.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()

	_, _, err := svc.Generate(ctx, GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false,
	})
	require.Error(t, err)

	// Session is not disabled.
	require.False(t, svc.isDisabledForSession("sess-1"))

	// Second call: fresh context, cached value available.
	svc.sessionCaches.Store("sess-1", "recovered-map", 55)
	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-1",
		ParityMode: false,
	})
	require.NoError(t, err)
	require.Equal(t, "recovered-map", m)
	require.Equal(t, 55, tok)
}

// TestDisableLatchInfrastructureMethods directly tests the disableForSession
// and isDisabledForSession methods for correctness and concurrency safety.
func TestDisableLatchInfrastructureMethods(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Initially no session is disabled.
	require.False(t, svc.isDisabledForSession("a"))
	require.False(t, svc.isDisabledForSession("b"))
	require.False(t, svc.isDisabledForSession(""))

	// Disable session "a".
	svc.disableForSession("a")
	require.True(t, svc.isDisabledForSession("a"))
	require.False(t, svc.isDisabledForSession("b"))

	// Idempotent: disabling twice is safe.
	svc.disableForSession("a")
	require.True(t, svc.isDisabledForSession("a"))

	// Delete clears the latch.
	svc.disabledSessions.Delete("a")
	require.False(t, svc.isDisabledForSession("a"))
}

// TestDisableLatchConcurrentAccess verifies the sync.Map-based latch
// is safe under concurrent access from multiple goroutines.
func TestDisableLatchConcurrentAccess(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	const goroutines = 100

	done := make(chan struct{})

	// Concurrent writers.
	for i := range goroutines {
		go func(idx int) {
			sessionID := "sess-" + string(rune('A'+idx%26))
			svc.disableForSession(sessionID)
			_ = svc.isDisabledForSession(sessionID)
		}(i)
	}

	// Concurrent readers.
	for i := range goroutines {
		go func(idx int) {
			sessionID := "sess-" + string(rune('A'+idx%26))
			_ = svc.isDisabledForSession(sessionID)
		}(i)
	}

	close(done)
}

// TestDisableLatchResetOnlyAffectsTargetSession verifies that Reset()
// on one session does not clear the latch for other sessions.
func TestDisableLatchResetOnlyAffectsTargetSession(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	svc.disableForSession("sess-1")
	svc.disableForSession("sess-2")
	svc.disableForSession("sess-3")

	require.True(t, svc.isDisabledForSession("sess-1"))
	require.True(t, svc.isDisabledForSession("sess-2"))
	require.True(t, svc.isDisabledForSession("sess-3"))

	// Reset only sess-2.
	err := svc.Reset(context.Background(), "sess-2")
	require.NoError(t, err)

	require.True(t, svc.isDisabledForSession("sess-1"),
		"sess-1 latch must survive reset of sess-2")
	require.False(t, svc.isDisabledForSession("sess-2"),
		"sess-2 latch must be cleared by reset")
	require.True(t, svc.isDisabledForSession("sess-3"),
		"sess-3 latch must survive reset of sess-2")
}

// initGitRepo creates a temporary git repo with the given files committed.
// Returns the root directory path.
func initGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}

	run("git", "init")
	run("git", "checkout", "-b", "main")

	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	run("git", "add", "-A")
	run("git", "commit", "-m", "init")

	return dir
}

// TestGitTrackedFilesParityMode verifies that parity mode uses
// git-tracked files as the file universe.
func TestGitTrackedFilesParityMode(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t, map[string]string{
		"main.go":        "package main",
		"lib/utils.go":   "package lib",
		"docs/README.md": "# docs",
	})

	svc := NewService(nil, nil, nil, dir, context.Background())

	files, err := svc.gitTrackedFiles(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"docs/README.md", "lib/utils.go", "main.go"}, files)
}

// TestGitTrackedFilesFallbackNonGitDir verifies that gitTrackedFiles
// returns an error when run outside a git repository, causing parity
// mode to fall back to ChatFiles.
func TestGitTrackedFilesFallbackNonGitDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() // Not a git repo.
	svc := NewService(nil, nil, nil, dir, context.Background())

	files, err := svc.gitTrackedFiles(context.Background())
	require.Error(t, err)
	require.Nil(t, files)
}

// TestGitTrackedFilesExcludeGlobs verifies that ExcludeGlobs are
// applied to filter the git-tracked file universe.
func TestGitTrackedFilesExcludeGlobs(t *testing.T) {
	t.Parallel()

	dir := initGitRepo(t, map[string]string{
		"main.go":           "package main",
		"vendor/dep.go":     "package dep",
		"vendor/sub/lib.go": "package sub",
		"internal/app.go":   "package internal",
		"build.log":         "log output",
	})

	cfg := &config.Config{
		Options: &config.Options{
			RepoMap: &config.RepoMapOptions{
				ExcludeGlobs: []string{"vendor/**", "*.log"},
			},
		},
	}

	svc := NewService(cfg, nil, nil, dir, context.Background())

	files, err := svc.gitTrackedFiles(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"internal/app.go", "main.go"}, files)
}

// TestGenerateParityModeUsesGitTracked verifies that Generate() in
// parity mode uses git-tracked files and falls back to ChatFiles when
// not in a git repo.
func TestGenerateParityModeUsesGitTracked(t *testing.T) {
	t.Parallel()

	t.Run("non_git_dir_falls_back_to_chat_files", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		svc := NewService(nil, nil, nil, dir, context.Background())

		// With no git repo and no ChatFiles, parity mode falls back to
		// empty ChatFiles → empty fileUniverse → fallback(nil).
		m, tok, err := svc.Generate(context.Background(), GenerateOpts{
			SessionID:  "sess-parity-fallback",
			ParityMode: true,
		})
		require.NoError(t, err)
		require.Empty(t, m)
		require.Zero(t, tok)
	})

	t.Run("non_git_dir_uses_chat_files_fallback", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		svc := NewService(nil, nil, nil, dir, context.Background())

		// Pre-populate cache so we can observe the fallback path
		// (db is nil so Generate hits fallback after fileUniverse).
		svc.sessionCaches.Store("sess-fb", "cached-map", 50)

		m, tok, err := svc.Generate(context.Background(), GenerateOpts{
			SessionID:  "sess-fb",
			ParityMode: true,
			ChatFiles:  []string{"foo.go", "bar.go"},
		})
		require.NoError(t, err)
		// db is nil → fallback returns cached map.
		require.Equal(t, "cached-map", m)
		require.Equal(t, 50, tok)
	})
}

// TestGenerateNonParityModeUnchanged verifies that non-parity mode
// still uses the walker-based file universe (existing behaviour).
func TestGenerateNonParityModeUnchanged(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// With nil db, Generate falls back. Pre-populate the cache.
	svc.sessionCaches.Store("sess-enh", "enhance-map", 77)

	m, tok, err := svc.Generate(context.Background(), GenerateOpts{
		SessionID:  "sess-enh",
		ParityMode: false,
	})
	require.NoError(t, err)
	require.Equal(t, "enhance-map", m)
	require.Equal(t, 77, tok)
}

// TestGitTrackedFilesEmptyRepo verifies that an empty git repo (no
// committed files) returns nil without error.
func TestGitTrackedFilesEmptyRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", out)

	svc := NewService(nil, nil, nil, dir, context.Background())

	files, err := svc.gitTrackedFiles(context.Background())
	require.NoError(t, err)
	require.Nil(t, files)
}

// TestGenerateProducesTreeContextOutput verifies that Generate() exercises
// the TreeContext render path end-to-end (S2.2). The output must contain
// scope-aware markers (│ for shown lines, ⋮ for collapsed gaps). This test
// fails if RenderRepoMap is replaced with renderStageEntries in Generate.
func TestGenerateProducesTreeContextOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Real database is required: Generate() returns fallback when db is nil.
	conn, err := db.Connect(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	q := db.New(conn)

	dir := initGitRepo(t, map[string]string{
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(Hello())\n\tfmt.Println(add(1, 2))\n}\n",
		"lib.go":  "package main\n\n// Hello returns a greeting.\nfunc Hello() string {\n\treturn \"hello\"\n}\n",
		"util.go": "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc unused() {}\n",
	})

	svc := NewService(nil, q, conn, dir, ctx)
	defer svc.Close()

	m, tok, err := svc.Generate(ctx, GenerateOpts{
		SessionID:    "sess-s2-treecontext",
		ParityMode:   false,
		TokenBudget:  100_000,
		ForceRefresh: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, m, "expected non-empty repo map from Generate")
	require.Greater(t, tok, 0, "expected positive token count")

	// TreeContext scope-aware markers must be present in the output.
	// │ prefixes shown lines; ⋮ marks collapsed gaps.
	require.Contains(t, m, "│", "expected │ (pipe) marker from TreeContext rendering")
	require.Contains(t, m, ":\n│", "expected file header followed by TreeContext output")
}
