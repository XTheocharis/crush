package model

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/charmbracelet/crush/internal/ui/util"
)

type editMessageResult struct {
	Text            string
	ReloadSessionID string
}

type rewindResultMsg struct {
	session       *session.Session
	extractedText string
}

func (m *UI) executeRewind(sessionID string, seq int, mode rewind.RewindMode) tea.Cmd {
	return func() tea.Msg {
		svc := m.com.Workspace.RewindService()
		if svc == nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: "Rewind is not available"}
		}
		result, err := svc.Rewind(context.Background(), sessionID, seq, mode)
		if err != nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: fmt.Sprintf("Rewind failed: %v", err)}
		}
		return rewindResultMsg{
			session:       m.session,
			extractedText: result.ExtractedText,
		}
	}
}

func (m *UI) executeFork(sessionID string, seq int) tea.Cmd {
	return func() tea.Msg {
		svc := m.com.Workspace.RewindService()
		if svc == nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: "Rewind is not available"}
		}
		result, err := svc.Fork(context.Background(), sessionID, seq)
		if err != nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: fmt.Sprintf("Fork failed: %v", err)}
		}
		return dialog.ActionSelectSession{
			Session: session.Session{ID: result.NewSessionID, Title: result.NewSessionTitle},
		}
	}
}

func (m *UI) executeEditMessage(sessionID string, seq int, messageID string) tea.Cmd {
	return func() tea.Msg {
		svc := m.com.Workspace.RewindService()
		if svc == nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: "Rewind is not available"}
		}
		result, err := svc.EditMessage(context.Background(), sessionID, seq)
		if err != nil {
			return util.InfoMsg{Type: util.InfoTypeError, Msg: fmt.Sprintf("Edit failed: %v", err)}
		}
		return editMessageResult{Text: result.ExtractedText, ReloadSessionID: sessionID}
	}
}

func (m *UI) handleEditMessageResult(msg editMessageResult) tea.Cmd {
	var cmds []tea.Cmd
	if msg.ReloadSessionID != "" && m.session != nil {
		cmds = append(cmds, m.loadSession(msg.ReloadSessionID))
	}
	m.textarea.SetValue(msg.Text)
	m.textarea.CursorEnd()
	m.focus = uiFocusEditor
	cmds = append(cmds, m.textarea.Focus())
	m.chat.Blur()
	m.setState(m.state, uiFocusEditor)
	return tea.Batch(cmds...)
}
