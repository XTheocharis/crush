package dialog

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/sahilm/fuzzy"
)

const (
	MessageOptionsID           = "message-options"
	messageOptionsDialogWidth  = 46
	messageOptionsDialogHeight = 14
)

// MessageOptions is a dialog that shows message-level actions like rewind,
// fork, and edit.
type MessageOptions struct {
	com       *common.Common
	help      help.Model
	list      *list.FilterableList
	input     textinput.Model
	sessionID string
	seq       int
	messageID string

	keyMap struct {
		Select   key.Binding
		Next     key.Binding
		Previous key.Binding
		UpDown   key.Binding
		Close    key.Binding
	}
}

// MessageOptionItem represents a single action option in the message options
// dialog.
type MessageOptionItem struct {
	*list.Versioned
	label   string
	action  Action
	t       *styles.Styles
	m       fuzzy.Match
	cache   map[int]string
	focused bool
}

var (
	_ Dialog   = (*MessageOptions)(nil)
	_ ListItem = (*MessageOptionItem)(nil)
)

// NewMessageOptions creates a new message options dialog.
func NewMessageOptions(com *common.Common, sessionID string, seq int, messageID string) *MessageOptions {
	m := &MessageOptions{
		com:       com,
		sessionID: sessionID,
		seq:       seq,
		messageID: messageID,
	}

	help := help.New()
	help.Styles = com.Styles.DialogHelpStyles()
	m.help = help

	m.list = list.NewFilterableList()
	m.list.Focus()

	m.input = textinput.New()
	m.input.SetVirtualCursor(false)
	m.input.Placeholder = "Type to filter"
	m.input.SetStyles(com.Styles.TextInput)
	m.input.Focus()

	m.keyMap.Select = key.NewBinding(
		key.WithKeys("enter", "ctrl+y"),
		key.WithHelp("enter", "confirm"),
	)
	m.keyMap.Next = key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next item"),
	)
	m.keyMap.Previous = key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "previous item"),
	)
	m.keyMap.UpDown = key.NewBinding(
		key.WithKeys("up", "down"),
		key.WithHelp("↑/↓", "choose"),
	)
	m.keyMap.Close = CloseKey

	m.setOptions()
	return m
}

func (m *MessageOptions) ID() string {
	return MessageOptionsID
}

// HandleMsg implements [Dialog].
func (m *MessageOptions) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, m.keyMap.Previous):
			m.list.Focus()
			if m.list.IsSelectedFirst() {
				m.list.SelectLast()
				m.list.ScrollToBottom()
				break
			}
			m.list.SelectPrev()
			m.list.ScrollToSelected()
		case key.Matches(msg, m.keyMap.Next):
			m.list.Focus()
			if m.list.IsSelectedLast() {
				m.list.SelectFirst()
				m.list.ScrollToTop()
				break
			}
			m.list.SelectNext()
			m.list.ScrollToSelected()
		case key.Matches(msg, m.keyMap.Select):
			selectedItem := m.list.SelectedItem()
			if selectedItem == nil {
				break
			}
			optItem, ok := selectedItem.(*MessageOptionItem)
			if !ok {
				break
			}
			return optItem.action
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			value := m.input.Value()
			m.list.SetFilter(value)
			m.list.ScrollToTop()
			m.list.SetSelected(0)
			return ActionCmd{cmd}
		}
	}
	return nil
}

// Draw implements [Dialog].
func (m *MessageOptions) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := m.com.Styles
	width := max(0, min(messageOptionsDialogWidth, area.Dx()))
	height := max(0, min(messageOptionsDialogHeight, area.Dy()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()
	heightOffset := t.Dialog.Title.GetVerticalFrameSize() + titleContentHeight +
		t.Dialog.InputPrompt.GetVerticalFrameSize() + inputContentHeight +
		t.Dialog.HelpView.GetVerticalFrameSize() +
		t.Dialog.View.GetVerticalFrameSize()

	m.input.SetWidth(innerWidth - t.Dialog.InputPrompt.GetHorizontalFrameSize() - 1)
	m.list.SetSize(innerWidth, height-heightOffset)
	m.help.SetWidth(innerWidth)

	rc := NewRenderContext(t, width)
	rc.Title = "Message Options"
	inputView := t.Dialog.InputPrompt.Render(m.input.View())
	rc.AddPart(inputView)

	visibleCount := len(m.list.FilteredItems())
	if m.list.Height() >= visibleCount {
		m.list.ScrollToTop()
	} else {
		m.list.ScrollToSelected()
	}

	listView := t.Dialog.List.Height(m.list.Height()).Render(m.list.Render())
	rc.AddPart(listView)
	rc.Help = m.help.View(m)

	view := rc.Render()

	cur := InputCursor(t, m.input.Cursor())
	DrawCenterCursor(scr, area, view, cur)
	return cur
}

// ShortHelp implements [help.KeyMap].
func (m *MessageOptions) ShortHelp() []key.Binding {
	return []key.Binding{
		m.keyMap.UpDown,
		m.keyMap.Select,
		m.keyMap.Close,
	}
}

// FullHelp implements [help.KeyMap].
func (m *MessageOptions) FullHelp() [][]key.Binding {
	slice := []key.Binding{
		m.keyMap.Select,
		m.keyMap.Next,
		m.keyMap.Previous,
		m.keyMap.Close,
	}
	result := [][]key.Binding{}
	for i := 0; i < len(slice); i += 4 {
		end := min(i+4, len(slice))
		result = append(result, slice[i:end])
	}
	return result
}

func (m *MessageOptions) setOptions() {
	items := []struct {
		label  string
		action Action
	}{
		{"Rewind (code only)", ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindCodeOnly}},
		{"Rewind (convo only)", ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindConvoOnly}},
		{"Rewind (both)", ActionRewind{SessionID: m.sessionID, Seq: m.seq, Mode: rewind.RewindBoth}},
		{"Edit message", ActionEditMessage{SessionID: m.sessionID, Seq: m.seq, MessageID: m.messageID}},
		{"Fork from here", ActionFork{SessionID: m.sessionID, Seq: m.seq}},
		{"Cancel", ActionClose{}},
	}

	listItems := make([]list.FilterableItem, len(items))
	for i, opt := range items {
		listItems[i] = &MessageOptionItem{
			Versioned: list.NewVersioned(),
			label:     opt.label,
			action:    opt.action,
			t:         m.com.Styles,
		}
	}
	m.list.SetItems(listItems...)
	m.list.SetSelected(0)
	m.list.ScrollToTop()
}

// Finished implements list.Item.
func (r *MessageOptionItem) Finished() bool {
	return true
}

// Filter implements list.FilterableItem.
func (r *MessageOptionItem) Filter() string {
	return r.label
}

// ID implements ListItem.
func (r *MessageOptionItem) ID() string {
	return r.label
}

// SetFocused implements list.Focusable.
func (r *MessageOptionItem) SetFocused(focused bool) {
	if r.focused == focused {
		return
	}
	r.cache = nil
	r.focused = focused
	if r.Versioned != nil {
		r.Bump()
	}
}

// SetMatch implements list.MatchSettable.
func (r *MessageOptionItem) SetMatch(m fuzzy.Match) {
	if sameFuzzyMatch(r.m, m) {
		return
	}
	r.cache = nil
	r.m = m
	if r.Versioned != nil {
		r.Bump()
	}
}

// Render implements list.Item.
func (r *MessageOptionItem) Render(width int) string {
	styles := ListItemStyles{
		ItemBlurred:     r.t.Dialog.NormalItem,
		ItemFocused:     r.t.Dialog.SelectedItem,
		InfoTextBlurred: r.t.Dialog.ListItem.InfoBlurred,
		InfoTextFocused: r.t.Dialog.ListItem.InfoFocused,
	}
	return renderItem(styles, r.label, "", r.focused, width, r.cache, &r.m)
}
