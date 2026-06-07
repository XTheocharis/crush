package model

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/ui/chat"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/util"
)

// handleXrushRoutingUpdate handles fork-only message routing in the main Update
// loop. This includes rewind results, compaction events, edit message results,
// and delayed click handling.
func (m *UI) handleXrushRoutingUpdate(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case rewindResultMsg:
		var cmds []tea.Cmd
		cmds = append(cmds, m.loadSession(msg.session.ID))
		if msg.extractedText != "" {
			cmds = append(cmds, func() tea.Msg {
				return openEditorMsg{Text: msg.extractedText}
			})
		}
		return tea.Batch(cmds...)

	case CompactionStartedMsg:
		return m.handleCompactionStarted()

	case CompactionCompletedMsg:
		m.handleCompactionFinished()
		return nil

	case CompactionFailedMsg:
		m.handleCompactionFinished()
		return nil

	case editMessageResult:
		return m.handleEditMessageResult(msg)

	case commitEditResult:
		return m.handleCommitEditResult(msg)

	case DelayedClickMsg:
		if handled, cmd := m.chat.HandleDelayedClick(msg); handled {
			return cmd
		}
		return nil

	case RepoMapRefreshResultMsg:
		return m.handleRepoMapRefreshResult(msg)
	}

	return nil
}

// XRUSH: SetSessionID sets the current session ID used for message options.
func (m *Chat) SetSessionID(id string) {
	m.sessionID = id
}

// XRUSH: SelectedUserMessageItem returns the selected item if it is a user
// message, or nil otherwise.
func (m *Chat) SelectedUserMessageItem() *chat.UserMessageItem {
	if item, ok := m.list.SelectedItem().(*chat.UserMessageItem); ok {
		return item
	}
	return nil
}

// XRUSH: SelectedMessageItem returns the selected item if it is a user or
// assistant message, or nil otherwise.
func (m *Chat) SelectedMessageItem() (id string, seq int, ok bool) {
	switch item := m.list.SelectedItem().(type) {
	case *chat.UserMessageItem:
		return item.ID(), item.Seq(), true
	case *chat.AssistantMessageItem:
		return item.ID(), item.Seq(), true
	}
	return "", 0, false
}

// XRUSH: dispatchForkMessageOptions handles fork-specific message options
// dispatch on single-click of user messages.
func (m *Chat) dispatchXrushMessageOptions(selectedItem list.Item) (bool, tea.Cmd) {
	switch item := selectedItem.(type) {
	case *chat.UserMessageItem:
		if m.OnMessageOptions != nil {
			return true, m.OnMessageOptions(item.ID(), m.sessionID, item.Seq())
		}
	case *chat.AssistantMessageItem:
		if m.OnMessageOptions != nil {
			return true, m.OnMessageOptions(item.ID(), m.sessionID, item.Seq())
		}
	}
	return false, nil
}

func isXrushDialogAction(action dialog.Action) bool {
	switch action.(type) {
	case dialog.ActionOpenMessageOptions, dialog.ActionRewind, dialog.ActionFork, dialog.ActionEditMessage:
		return true
	}
	return false
}

// initXrushChatCallbacks wires fork-only callbacks on the Chat component.
func initXrushChatCallbacks(ch *Chat, com *common.Common) {
	ch.OnMessageOptions = func(messageID, sessionID string, seq int) tea.Cmd {
		return func() tea.Msg {
			return dialog.ActionOpenMessageOptions{MessageID: messageID, SessionID: sessionID, Seq: seq}
		}
	}
}

// handleXrushDialogMsg handles fork-only dialog action routing. This includes
// message options, rewind, fork, and edit message actions.
func (m *UI) handleXrushDialogMsg(action tea.Msg) tea.Cmd {
	switch msg := action.(type) {
	case dialog.ActionOpenMessageOptions:
		if m.com.Workspace.RewindService() == nil {
			return func() tea.Msg {
				return util.InfoMsg{Type: util.InfoTypeWarn, Msg: "Rewind is not available"}
			}
		}
		seq := msg.Seq
		messageID := msg.MessageID
		if seq == 0 && messageID == "" {
			if id, s, ok := m.chat.SelectedMessageItem(); ok {
				seq = s
				messageID = id
			}
			if seq == 0 && m.hasSession() {
				msgs, err := m.com.Workspace.ListMessages(context.Background(), msg.SessionID)
				if err == nil {
					for _, msg := range slices.Backward(msgs) {
						if msg.Role == message.User || msg.Role == message.Assistant {
							seq = msg.Seq
							messageID = msg.ID
							break
						}
					}
				}
			}
		}
		if seq == 0 && messageID == "" {
			return nil
		}
		m.dialog.OpenDialog(dialog.NewMessageOptions(m.com, msg.SessionID, seq, messageID))
		return nil

	case dialog.ActionRewind:
		m.dialog.CloseFrontDialog()
		return m.executeRewind(msg.SessionID, msg.Seq, msg.Mode)

	case dialog.ActionFork:
		m.dialog.CloseFrontDialog()
		return m.executeFork(msg.SessionID, msg.Seq)

	case dialog.ActionEditMessage:
		m.dialog.CloseFrontDialog()
		return m.executeEditMessage(msg.SessionID, msg.Seq, msg.MessageID)
	}

	return nil
}

// handleXrushKeyPress handles fork-only key bindings. This includes the message
// options key binding in the main chat focus state.
func (m *UI) handleXrushKeyPress(msg tea.KeyPressMsg) tea.Cmd {
	if m.hasSession() {
		if m.com.Workspace.RewindService() == nil {
			return func() tea.Msg {
				return util.InfoMsg{Type: util.InfoTypeWarn, Msg: "Rewind is not available"}
			}
		}
		if id, seq, ok := m.chat.SelectedMessageItem(); ok {
			if m.chat.OnMessageOptions != nil {
				return m.chat.OnMessageOptions(id, m.session.ID, seq)
			}
		}
	}
	return nil
}
