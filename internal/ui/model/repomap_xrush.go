package model

import (
	"context"
	"log/slog"

	tea "charm.land/bubbletea/v2"
)

// RepoMapRefreshResultMsg carries the result of an asynchronous repo map
// refresh triggered from the command palette.
type RepoMapRefreshResultMsg struct {
	SessionID string
	Err       error
}

// executeRepoMapRefresh creates a tea.Cmd that triggers an asynchronous repo
// map refresh via the workspace bridge.
func (m *UI) executeRepoMapRefresh(sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := m.com.Workspace.RepoMapRefresh(context.Background(), sessionID)
		return RepoMapRefreshResultMsg{
			SessionID: sessionID,
			Err:       err,
		}
	}
}

// handleRepoMapRefreshResult processes the result of a repo map refresh.
func (m *UI) handleRepoMapRefreshResult(msg RepoMapRefreshResultMsg) tea.Cmd {
	if msg.Err != nil {
		slog.Error("Repo map refresh failed", "session_id", msg.SessionID, "error", msg.Err)
	}
	return nil
}
