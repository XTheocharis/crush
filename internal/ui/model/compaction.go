package model

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/common"
)

type CompactionStartedMsg struct {
	SessionID string
}

type CompactionCompletedMsg struct {
	SessionID string
	Rounds    int
}

type CompactionFailedMsg struct {
	SessionID string
	Err       string
}

func newLCMSpinner(com *common.Common) spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(com.Styles.Pills.TodoSpinner),
	)
}

func (m *UI) handleCompactionStarted() tea.Cmd {
	m.lcmCompacting = true
	m.lcmCompactingStart = time.Now()
	m.renderPills()
	return m.lcmSpinner.Tick
}

func (m *UI) handleCompactionFinished() {
	m.lcmCompacting = false
	m.renderPills()
}

func (m *UI) updateLCMSpinner(msg tea.Msg) tea.Cmd {
	if !m.lcmCompacting {
		return nil
	}
	var cmd tea.Cmd
	m.lcmSpinner, cmd = m.lcmSpinner.Update(msg)
	if cmd != nil {
		m.renderPills()
	}
	return cmd
}

func (m *UI) compactionPill() string {
	if !m.lcmCompacting {
		return ""
	}

	t := m.com.Styles
	elapsed := time.Since(m.lcmCompactingStart).Seconds()
	label := t.Pills.Base.Render("⟳ Compacting")
	duration := t.Pills.Base.Render(fmt.Sprintf("(%.0fs)", elapsed))
	content := fmt.Sprintf("%s %s", label, duration)

	return pillStyle(false, false, t).Render(content)
}
