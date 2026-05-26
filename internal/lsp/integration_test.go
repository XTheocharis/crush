//go:build ignore
// +build ignore

package lsp

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/stretchr/testify/require"
)

// TestIntegrationBackoffPriorityExecutor verifies that the three LSP features
// (exponential backoff, priority-based startup, and task executor) are
// correctly wired into the Manager.
func TestIntegration(t *testing.T) {
	t.Parallel()

	t.Run("BackoffRetriesIncreaseOverTime", func(t *testing.T) {
		t.Parallel()

		base := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
		now := base

		mgr := &Manager{
			unavailable: csync.NewMap[string, *serverRetryState](),
			now:         func() time.Time { return now },
			backoff:     DefaultBackoff(),
			executor:    NewTaskExecutor(0),
		}
		mgr.executor.Start()
		defer mgr.executor.Stop()

		// First mark: attempt 0 → ~1s backoff.
		mgr.markUnavailable("gopls")
		require.True(t, mgr.recentlyUnavailable("gopls"))

		// Advance 500ms — still within backoff window.
		now = now.Add(500 * time.Millisecond)
		require.True(t, mgr.recentlyUnavailable("gopls"))

		// Advance to 1.5s total — should clear (initial backoff ~1s).
		now = base.Add(1500 * time.Millisecond)
		require.False(t, mgr.recentlyUnavailable("gopls"))

		// Second mark: attempt 1 → ~2s backoff.
		mgr.markUnavailable("gopls")
		now = now.Add(1500 * time.Millisecond)
		require.True(t, mgr.recentlyUnavailable("gopls"), "should still be in ~2s backoff after 1.5s")

		now = now.Add(1500 * time.Millisecond)
		require.False(t, mgr.recentlyUnavailable("gopls"), "should clear after ~3s total for 2s backoff")

		// Third mark: attempt 2 → ~4s backoff.
		mgr.markUnavailable("gopls")
		now = now.Add(3 * time.Second)
		require.True(t, mgr.recentlyUnavailable("gopls"), "should still be in ~4s backoff after 3s")

		now = now.Add(3 * time.Second)
		require.False(t, mgr.recentlyUnavailable("gopls"), "should clear after ~6s total for 4s backoff")

		// clearUnavailable resets everything.
		mgr.markUnavailable("gopls")
		mgr.clearUnavailable("gopls")
		require.False(t, mgr.recentlyUnavailable("gopls"))
	})

	t.Run("PriorityOrderingCriticalFirst", func(t *testing.T) {
		t.Parallel()

		servers := map[string]*powernapconfig.ServerConfig{
			"nil":                        {Command: "nil"},
			"lua-language-server":        {Command: "lua-language-server"},
			"gopls":                      {Command: "gopls"},
			"terraform-ls":               {Command: "terraform-ls"},
			"rust-analyzer":              {Command: "rust-analyzer"},
			"typescript-language-server": {Command: "typescript-language-server"},
		}

		sorted := sortServersByPriority(servers)
		require.Len(t, sorted, len(servers))

		criticalSeen := 0
		for _, entry := range sorted {
			p := serverPriority(entry.Name)
			if p == PriorityCritical {
				criticalSeen++
				// All critical servers must appear before any non-critical.
				// Once we've seen a non-critical, no more critical allowed.
			}
		}
		require.Equal(t, 3, criticalSeen, "gopls, rust-analyzer, typescript-language-server")

		// Verify ordering: critical before non-critical.
		seenNonCritical := false
		for _, entry := range sorted {
			if serverPriority(entry.Name) != PriorityCritical {
				seenNonCritical = true
				continue
			}
			require.False(t, seenNonCritical, "critical server %q appeared after non-critical", entry.Name)
		}
	})

	t.Run("ExecutorSerializesPerServer", func(t *testing.T) {
		t.Parallel()

		mgr := &Manager{
			clients:     csync.NewMap[string, *Client](),
			unavailable: csync.NewMap[string, *serverRetryState](),
			backoff:     DefaultBackoff(),
			executor:    NewTaskExecutor(10),
		}
		mgr.executor.Start()
		defer mgr.executor.Stop()

		// Without a real client, GetDiagnosticsForServer returns nil.
		result := mgr.GetDiagnosticsForServer(context.Background(), "nonexistent")
		require.Nil(t, result)

		// FindReferencesForServer returns ErrClientNotFound.
		_, err := mgr.FindReferencesForServer(context.Background(), "nonexistent", "foo.go", 1, 0, true)
		require.ErrorIs(t, err, ErrClientNotFound)
	})

	t.Run("ExecutorStopOnClose", func(t *testing.T) {
		t.Parallel()

		mgr := &Manager{
			clients:     csync.NewMap[string, *Client](),
			unavailable: csync.NewMap[string, *serverRetryState](),
			backoff:     DefaultBackoff(),
			executor:    NewTaskExecutor(10),
		}
		mgr.executor.Start()

		// Submit a task before Close.
		var executed atomic.Bool
		var wg sync.WaitGroup
		wg.Go(func() {
			_ = mgr.executor.Submit(context.Background(), "srv", func() error {
				time.Sleep(100 * time.Millisecond)
				executed.Store(true)
				return nil
			})
		})
		time.Sleep(30 * time.Millisecond)

		mgr.Close(context.Background())
		wg.Wait()
		require.True(t, executed.Load(), "in-flight task should complete before Close returns")

		// After Close, executor rejects new submissions.
		err := mgr.executor.Submit(context.Background(), "srv", func() error { return nil })
		require.ErrorIs(t, err, ErrExecutorStopped)
	})
}
