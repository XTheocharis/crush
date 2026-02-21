package model

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/lcm"
)

// CompactionStartedMsg is sent when LCM compaction begins.
type CompactionStartedMsg struct {
	SessionID string
}

// CompactionCompletedMsg is sent when LCM compaction finishes.
type CompactionCompletedMsg struct {
	SessionID string
	Rounds    int
}

// CompactionFailedMsg is sent when LCM compaction fails.
type CompactionFailedMsg struct {
	SessionID string
	Err       string
}

// CompactionEventToMsg converts a pubsub CompactionEvent to the appropriate tea.Msg.
func CompactionEventToMsg(event lcm.CompactionEvent) tea.Msg {
	switch event.Type {
	case lcm.CompactionStarted:
		return CompactionStartedMsg{SessionID: event.SessionID}
	case lcm.CompactionCompleted:
		return CompactionCompletedMsg{SessionID: event.SessionID, Rounds: event.Rounds}
	case lcm.CompactionFailed:
		return CompactionFailedMsg{SessionID: event.SessionID, Err: event.Error}
	default:
		return nil
	}
}

// compactionPill renders a compact status indicator for LCM compaction.
// When compacting is true, shows "⟳ Compacting (Xs)" with elapsed time.
// Returns "" when not compacting.
//
// NOTE: This function references fields that will be added to the UI model in task #26:
// - compacting (bool): tracks whether LCM compaction is in progress
// - compactingStart (time.Time): records when compaction began
//
// These fields should be added to the UI struct in ui.go:
//
//	compacting      bool      // tracks whether LCM compaction is in progress
//	compactingStart time.Time // records when compaction began (for elapsed time display)
func (m *UI) compactionPill() string {
	if !m.lcmCompacting {
		return ""
	}

	t := m.com.Styles
	elapsed := time.Since(m.lcmCompactingStart).Seconds()
	label := t.Base.Render("⟳ Compacting")
	duration := t.Muted.Render(fmt.Sprintf("(%.0fs)", elapsed))
	content := fmt.Sprintf("%s %s", label, duration)

	return pillStyle(false, false, t).Render(content)
}
