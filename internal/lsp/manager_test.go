package lsp

import (
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestUnavailableBackoff(t *testing.T) { // XRUSH: rewritten for exponential backoff
	t.Parallel()

	base := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	now := base

	manager := &Manager{
		unavailable: csync.NewMap[string, *serverRetryState](), // XRUSH: changed type
		now:         func() time.Time { return now },
		backoff:     DefaultBackoff(), // XRUSH: added backoff
	}

	require.False(t, manager.recentlyUnavailable("gopls"))

	manager.markUnavailable("gopls")
	require.True(t, manager.recentlyUnavailable("gopls"))

	now = now.Add(2 * time.Second)
	require.False(t, manager.recentlyUnavailable("gopls"))

	manager.markUnavailable("gopls")
	require.True(t, manager.recentlyUnavailable("gopls"))

	now = now.Add(3 * time.Second)
	require.False(t, manager.recentlyUnavailable("gopls"))

	manager.markUnavailable("gopls")
	manager.clearUnavailable("gopls")
	require.False(t, manager.recentlyUnavailable("gopls"))
}
