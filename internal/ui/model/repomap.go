package model

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/ui/util"
)

// handleRepoMapCommand checks if commandID is a repo-map control command and
// handles it. Returns (handled, cmd). When handled is true the caller should
// close the dialog and break; cmd may be non-nil with a status/error message.
func (m *UI) handleRepoMapCommand(commandID string) (bool, tea.Cmd) {
	if !isRepoMapCommand(commandID) {
		return false, nil
	}
	if !m.hasSession() {
		return true, util.ReportWarn("Start a session before running repo map controls.")
	}
	// Try RunRepoMapControl first — it handles refresh and reset.
	handled, statusMsg, err := m.com.App.RunRepoMapControl(context.Background(), commandID, m.session.ID)
	if handled {
		if err != nil {
			return true, util.ReportError(err)
		}
		if statusMsg != "" {
			return true, util.ReportInfo(statusMsg)
		}
		return true, nil
	}
	// Safety net: block unhandled reset commands from being sent to the model.
	if app.IsRepoMapResetCommand(commandID) {
		return true, util.ReportWarn("map_reset is command-only and not available for direct model invocation.")
	}
	return false, nil
}

func isRepoMapCommand(commandID string) bool {
	return strings.HasPrefix(commandID, "project:map-") || app.IsRepoMapResetCommand(commandID)
}
