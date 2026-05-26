package model

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/stretchr/testify/require"
)

type repoMapTestWorkspace struct {
	testWorkspace
	mu           sync.Mutex
	refreshCalls []string
	refreshErr   error
}

func (w *repoMapTestWorkspace) RepoMapRefresh(_ context.Context, sessionID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.refreshCalls = append(w.refreshCalls, sessionID)
	return w.refreshErr
}

func (w *repoMapTestWorkspace) getRefreshCalls() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]string, len(w.refreshCalls))
	copy(result, w.refreshCalls)
	return result
}

func newRepoMapTestUI(t *testing.T) (*UI, *repoMapTestWorkspace) {
	t.Helper()
	ws := &repoMapTestWorkspace{}
	return &UI{
		com: &common.Common{
			Workspace: ws,
		},
	}, ws
}

func TestRepoMapRefreshCmd(t *testing.T) {
	t.Parallel()

	t.Run("triggers refresh via workspace", func(t *testing.T) {
		t.Parallel()

		ui, ws := newRepoMapTestUI(t)
		cmd := ui.executeRepoMapRefresh("test-session-1")
		msg := cmd()
		result, ok := msg.(RepoMapRefreshResultMsg)
		require.True(t, ok, "expected RepoMapRefreshResultMsg, got %T", msg)
		require.Equal(t, "test-session-1", result.SessionID)
		require.NoError(t, result.Err)
		require.Equal(t, []string{"test-session-1"}, ws.getRefreshCalls())
	})

	t.Run("returns error from workspace", func(t *testing.T) {
		t.Parallel()

		ui, ws := newRepoMapTestUI(t)
		ws.refreshErr = errors.New("refresh failed")
		cmd := ui.executeRepoMapRefresh("test-session-2")
		msg := cmd()
		result, ok := msg.(RepoMapRefreshResultMsg)
		require.True(t, ok)
		require.Equal(t, "test-session-2", result.SessionID)
		require.Error(t, result.Err)
		require.EqualError(t, result.Err, "refresh failed")
	})
}

func TestRepoMapRefreshRouting(t *testing.T) {
	t.Parallel()

	t.Run("RepoMapRefreshResultMsg routes through handleXrushRoutingUpdate", func(t *testing.T) {
		t.Parallel()

		ui, _ := newRepoMapTestUI(t)
		msg := RepoMapRefreshResultMsg{SessionID: "sess-1", Err: nil}
		cmd := ui.handleXrushRoutingUpdate(msg)
		require.Nil(t, cmd)
	})

	t.Run("RepoMapRefreshResultMsg with error routes without panic", func(t *testing.T) {
		t.Parallel()

		ui, _ := newRepoMapTestUI(t)
		msg := RepoMapRefreshResultMsg{SessionID: "sess-2", Err: errors.New("boom")}
		cmd := ui.handleXrushRoutingUpdate(msg)
		require.Nil(t, cmd)
	})
}

func TestRepoMapRefreshResultHandler(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns nil cmd", func(t *testing.T) {
		t.Parallel()

		ui, _ := newRepoMapTestUI(t)
		cmd := ui.handleRepoMapRefreshResult(RepoMapRefreshResultMsg{
			SessionID: "sess-ok",
			Err:       nil,
		})
		require.Nil(t, cmd)
	})

	t.Run("non-nil error returns nil cmd", func(t *testing.T) {
		t.Parallel()

		ui, _ := newRepoMapTestUI(t)
		cmd := ui.handleRepoMapRefreshResult(RepoMapRefreshResultMsg{
			SessionID: "sess-fail",
			Err:       errors.New("something went wrong"),
		})
		require.Nil(t, cmd)
	})
}

func TestRepoMapRefreshUnrecognizedCommand(t *testing.T) {
	t.Parallel()

	t.Run("non-repomap message returns nil from routing", func(t *testing.T) {
		t.Parallel()

		ui, _ := newRepoMapTestUI(t)
		cmd := ui.handleXrushRoutingUpdate("not-a-repomap-message")
		require.Nil(t, cmd)
	})
}
