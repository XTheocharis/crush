package dialog

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

const MessageOptionsID = "message-options"

type option struct {
	label  string
	action Action
}

// MessageOptions is a dialog that shows message-level actions like rewind,
// fork, and edit. Only rewind-related options are shown when the rewind
// service is available.
type MessageOptions struct {
	com       *common.Common
	sessionID string
	seq       int
	messageID string
	selected  int
	keyMap    struct {
		UpDown key.Binding
		Enter  key.Binding
		Close  key.Binding
	}
}

var _ Dialog = (*MessageOptions)(nil)

func NewMessageOptions(com *common.Common, sessionID string, seq int, messageID string) *MessageOptions {
	m := &MessageOptions{
		com:       com,
		sessionID: sessionID,
		seq:       seq,
		messageID: messageID,
		selected:  0,
	}
	m.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "navigate"),
	)
	m.keyMap.Enter = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	)
	m.keyMap.Close = CloseKey
	return m
}

func (*MessageOptions) ID() string {
	return MessageOptionsID
}

func (m *MessageOptions) options() []option {
	return []option{
		{label: "Rewind (code only)", action: ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindCodeOnly}},
		{label: "Rewind (convo only)", action: ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindConvoOnly}},
		{label: "Rewind (both)", action: ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindBoth}},
		{label: "Edit & resubmit", action: ActionEditMessage{SessionID: m.sessionID, Seq: m.seq, MessageID: m.messageID}},
		{label: "Fork from here", action: ActionFork{SessionID: m.sessionID, Seq: m.seq}},
		{label: "Cancel", action: ActionClose{}},
	}
}

func (m *MessageOptions) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, m.keyMap.UpDown):
			opts := m.options()
			switch msg.String() {
			case "up":
				m.selected--
				if m.selected < 0 {
					m.selected = len(opts) - 1
				}
			case "down":
				m.selected++
				if m.selected >= len(opts) {
					m.selected = 0
				}
			}
		case key.Matches(msg, m.keyMap.Enter):
			opts := m.options()
			if m.selected >= 0 && m.selected < len(opts) {
				return opts[m.selected].action
			}
		}
	}
	return nil
}

func (m *MessageOptions) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	opts := m.options()
	normalStyle := m.com.Styles.Dialog.NormalItem
	selectedStyle := m.com.Styles.Dialog.SelectedItem

	lines := make([]string, len(opts))
	for i, opt := range opts {
		if i == m.selected {
			lines[i] = selectedStyle.Render(opt.label)
		} else {
			lines[i] = normalStyle.Render(opt.label)
		}
	}

	contentStyle := m.com.Styles.Dialog.Quit.Content
	content := contentStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			lines...,
		),
	)

	view := m.com.Styles.Dialog.Quit.Frame.Render(content)
	DrawCenter(scr, area, view)
	return nil
}
