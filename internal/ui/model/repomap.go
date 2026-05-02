package model

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/util"
)

func isRepoMapResetCommand(commandID string) bool {
	return strings.TrimSpace(commandID) == "project:map-reset"
}

func (m *UI) handleRepoMapCommand(commandID string) (bool, tea.Cmd) {
	if !isRepoMapCommand(commandID) {
		return false, nil
	}
	if !m.hasSession() {
		return true, util.ReportWarn("Start a session before running repo map controls.")
	}
	handled, statusMsg, err := m.com.Workspace.RunRepoMapControl(context.Background(), commandID, m.session.ID)
	if handled {
		if err != nil {
			return true, util.ReportError(err)
		}
		if statusMsg != "" {
			return true, util.ReportInfo(statusMsg)
		}
		return true, nil
	}
	if isRepoMapResetCommand(commandID) {
		return true, util.ReportWarn("map_reset is command-only and not available for direct model invocation.")
	}
	return false, nil
}

func isRepoMapCommand(commandID string) bool {
	return strings.HasPrefix(commandID, "project:map-") || isRepoMapResetCommand(commandID)
}
