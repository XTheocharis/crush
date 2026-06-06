package model

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/ui/chat"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Streaming response display tests
// ---------------------------------------------------------------------------

func TestAppendSessionMessage_AssistantStreaming(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	msg := message.Message{
		ID:   "stream-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Hello, world!"},
		},
	}

	cmd := u.appendSessionMessage(msg)
	if cmd != nil {
		_ = cmd()
	}

	item := u.chat.MessageItem("stream-1")
	require.NotNil(t, item, "streamed assistant message should be in chat by ID")

	_, ok := item.(chat.MessageItem)
	require.True(t, ok, "item must satisfy chat.MessageItem")
}

func TestAppendSessionMessage_AssistantWithToolCalls(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	msg := message.Message{
		ID:   "stream-2",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Let me look at that."},
			message.ToolCall{
				ID:   "tc-1",
				Name: "view",
				Input: `{"path":"/tmp/test.go"}`,
			},
		},
	}

	cmd := u.appendSessionMessage(msg)
	if cmd != nil {
		_ = cmd()
	}

	assistantItem := u.chat.MessageItem("stream-2")
	require.NotNil(t, assistantItem, "assistant message must be in chat")

	toolItem := u.chat.MessageItem("tc-1")
	require.NotNil(t, toolItem, "tool call item must be registered in chat")
}

func TestAppendSessionMessage_UserMessage(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	msg := message.Message{
		ID:   "user-1",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Fix the bug"},
		},
	}

	cmd := u.appendSessionMessage(msg)
	if cmd != nil {
		_ = cmd()
	}

	require.Equal(t, 1, u.chat.Len(), "chat should have exactly one message after append")
	require.NotNil(t, u.chat.MessageItem("user-1"))
}

func TestAppendSessionMessage_ToolResultUpdatesToolItem(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	assistantMsg := message.Message{
		ID:   "assistant-tool-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:   "tc-bash-1",
				Name: "bash",
				Input: `{"command":"echo hi"}`,
			},
		},
	}
	cmd := u.appendSessionMessage(assistantMsg)
	if cmd != nil {
		_ = cmd()
	}

	resultMsg := message.Message{
		ID:   "tool-result-1",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID: "tc-bash-1",
				Content:    "hi\n",
			},
		},
	}
	cmd = u.appendSessionMessage(resultMsg)
	if cmd != nil {
		_ = cmd()
	}

	toolItem := u.chat.MessageItem("tc-bash-1")
	require.NotNil(t, toolItem, "tool item must still exist after result update")

	_, ok := toolItem.(chat.ToolMessageItem)
	require.True(t, ok, "tool item must satisfy ToolMessageItem")
}

func TestUpdateSessionMessage_AssistantContentUpdate(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	msg1 := message.Message{
		ID:   "stream-update-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Hello"},
		},
	}
	cmd := u.appendSessionMessage(msg1)
	if cmd != nil {
		_ = cmd()
	}

	require.Equal(t, 1, u.chat.Len(), "should have one item after initial append")

	msg2 := message.Message{
		ID:   "stream-update-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "Hello, world! This is the full response."},
		},
	}
	cmd = u.updateSessionMessage(msg2)
	if cmd != nil {
		_ = cmd()
	}

	require.Equal(t, 1, u.chat.Len(), "updated message must not duplicate")

	item := u.chat.MessageItem("stream-update-1")
	require.NotNil(t, item)
}

func TestSetSessionMessages_BulkLoad(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	msgs := []message.Message{
		{
			ID:   "bulk-user-1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "First message"},
			},
		},
		{
			ID:   "bulk-assist-1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "First response"},
			},
		},
		{
			ID:   "bulk-user-2",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Second message"},
			},
		},
	}

	cmd := u.setSessionMessages(msgs)
	if cmd != nil {
		_ = cmd()
	}

	require.True(t, u.chat.Len() >= 3, "chat should contain at least 3 items after bulk load")
	require.NotNil(t, u.chat.MessageItem("bulk-user-1"))
	require.NotNil(t, u.chat.MessageItem("bulk-assist-1"))
	require.NotNil(t, u.chat.MessageItem("bulk-user-2"))
}

// ---------------------------------------------------------------------------
// Permission dialog accept/deny tests
// ---------------------------------------------------------------------------

func TestPermissionsDialog_CreatedOnPermissionRequest(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()
	perm := permission.PermissionRequest{
		ID:         "perm-test-1",
		ToolCallID: "tc-perm-1",
		ToolName:   "bash",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm))

	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"permissions dialog should be open after creation")
}

func TestPermissionsDialog_ReplacedOnSecondRequest(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()

	perm1 := permission.PermissionRequest{
		ID:         "perm-replace-1",
		ToolCallID: "tc-1",
		ToolName:   "bash",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm1))
	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID))

	perm2 := permission.PermissionRequest{
		ID:         "perm-replace-2",
		ToolCallID: "tc-2",
		ToolName:   "edit",
	}
	u.dialog.CloseDialog(dialog.PermissionsID)
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm2))

	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"permissions dialog should be open after replacement")
}

func TestPermissionNotification_GrantClosesDialog(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()
	perm := permission.PermissionRequest{
		ID:         "perm-grant-1",
		ToolCallID: "tc-grant",
		ToolName:   "bash",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm))
	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID))

	u.handlePermissionNotification(permission.PermissionNotification{
		ToolCallID: "tc-grant",
		Granted:    true,
	})

	require.False(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"grant notification should close matching dialog")
}

func TestPermissionNotification_DenyClosesDialog(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()
	perm := permission.PermissionRequest{
		ID:         "perm-deny-1",
		ToolCallID: "tc-deny",
		ToolName:   "edit",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm))

	u.handlePermissionNotification(permission.PermissionNotification{
		ToolCallID: "tc-deny",
		Denied:     true,
	})

	require.False(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"deny notification should close matching dialog")
}

func TestPermissionNotification_PendingDoesNotClose(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()
	perm := permission.PermissionRequest{
		ID:         "perm-pending-1",
		ToolCallID: "tc-pending",
		ToolName:   "view",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm))

	u.handlePermissionNotification(permission.PermissionNotification{
		ToolCallID: "tc-pending",
	})

	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"pending notification must not close dialog")
}

func TestPermissionNotification_MismatchedToolCallIDDoesNotClose(t *testing.T) {
	t.Parallel()

	u := newTestUIForPermissions()
	perm := permission.PermissionRequest{
		ID:         "perm-mismatch-1",
		ToolCallID: "tc-original",
		ToolName:   "bash",
	}
	u.dialog.OpenDialog(dialog.NewPermissions(u.com, perm))

	u.handlePermissionNotification(permission.PermissionNotification{
		ToolCallID: "tc-different",
		Granted:    true,
	})

	require.True(t, u.dialog.ContainsDialog(dialog.PermissionsID),
		"notification for different tool call must not close dialog")
}

// ---------------------------------------------------------------------------
// Compaction pill and compact mode tests
// ---------------------------------------------------------------------------

func TestCompactionPill_HiddenWhenNotCompacting(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = false

	require.Empty(t, u.compactionPill(),
		"compaction pill must be empty when not compacting")
}

func TestCompactionPill_VisibleWhenCompacting(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = true
	u.lcmCompactingStart = time.Now()

	pill := u.compactionPill()
	require.NotEmpty(t, pill, "compaction pill must be non-empty when compacting")
	require.Contains(t, stripANSIInteraction(pill), "Compacting")
}

func TestCompactionStarted_SetsFlag(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	require.False(t, u.lcmCompacting, "should start as not compacting")

	cmd := u.handleCompactionStarted()
	require.True(t, u.lcmCompacting, "should be compacting after started")
	require.NotNil(t, cmd, "should return spinner tick command")
	require.False(t, u.lcmCompactingStart.IsZero(), "start time should be set")
}

func TestCompactionFinished_ClearsFlag(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = true
	u.lcmCompactingStart = time.Now()

	u.handleCompactionFinished()
	require.False(t, u.lcmCompacting, "should not be compacting after finished")
}

func TestCompactionRoundTrip(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	require.Empty(t, u.compactionPill())

	_ = u.handleCompactionStarted()
	require.True(t, u.lcmCompacting)
	require.NotEmpty(t, u.compactionPill())

	u.handleCompactionFinished()
	require.False(t, u.lcmCompacting)
	require.Empty(t, u.compactionPill())
}

func TestPillsAreaHeight_IncludesCompaction(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.session = &session.Session{ID: "s-compaction"}
	u.lcmCompacting = true

	height := u.pillsAreaHeight()
	require.Greater(t, height, 0,
		"pills area height must be > 0 when compacting")
}

// ---------------------------------------------------------------------------
// Compact mode tests
// ---------------------------------------------------------------------------

func TestUpdateLayoutAndSize_ForceCompactMode(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.width = 200
	u.height = 60

	require.False(t, u.isCompact, "should start non-compact at large size")
	u.updateLayoutAndSize()
	require.False(t, u.isCompact, "should stay non-compact without forcing")

	u.forceCompactMode = true
	u.updateLayoutAndSize()
	require.True(t, u.isCompact, "should be compact when forced")

	u.forceCompactMode = false
	u.updateLayoutAndSize()
	require.False(t, u.isCompact, "should return to non-compact when un-forced at large size")
}

func TestUpdateLayoutAndSize_AutoCompactOnSmallTerminal(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	u.width = 80
	u.height = 60
	u.updateLayoutAndSize()
	require.True(t, u.isCompact, "should auto-compact when width < breakpoint")

	u.width = 200
	u.height = 20
	u.updateLayoutAndSize()
	require.True(t, u.isCompact, "should auto-compact when height < breakpoint")

	u.width = 200
	u.height = 60
	u.updateLayoutAndSize()
	require.False(t, u.isCompact, "should be non-compact when both dimensions large enough")
}

// ---------------------------------------------------------------------------
// Message action tests
// ---------------------------------------------------------------------------

func TestMessageOptionsDispatch_UserMessage(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	var callbackCalled bool
	var capturedID, capturedSessionID string
	var capturedSeq int

	u.chat.OnMessageOptions = func(messageID, sessionID string, seq int) tea.Cmd {
		callbackCalled = true
		capturedID = messageID
		capturedSessionID = sessionID
		capturedSeq = seq
		return nil
	}
	u.chat.SetSessionID("session-opts-1")

	msg := &message.Message{
		ID:   "user-opts-1",
		Role: message.User,
		Seq:  5,
		Parts: []message.ContentPart{
			message.TextContent{Text: "test"},
		},
	}
	item := chat.NewUserMessageItem(u.com.Styles, msg, nil)
	u.chat.SetMessages(item)
	u.chat.SetSelected(0)

	handled, _ := u.chat.dispatchXrushMessageOptions(u.chat.list.SelectedItem())
	require.True(t, handled, "dispatch should report handled for user message")
	require.True(t, callbackCalled, "OnMessageOptions callback should be invoked")
	require.Equal(t, "user-opts-1", capturedID)
	require.Equal(t, "session-opts-1", capturedSessionID)
	require.Equal(t, 5, capturedSeq)
}

func TestMessageOptionsDispatch_AssistantMessage(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	var callbackCalled bool

	u.chat.OnMessageOptions = func(_, _ string, _ int) tea.Cmd {
		callbackCalled = true
		return nil
	}
	u.chat.SetSessionID("session-opts-2")

	msg := &message.Message{
		ID:   "assist-opts-1",
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "response text"},
		},
	}
	item := chat.NewAssistantMessageItem(u.com.Styles, msg)
	u.chat.SetMessages(item)
	u.chat.SetSelected(0)

	handled, _ := u.chat.dispatchXrushMessageOptions(u.chat.list.SelectedItem())
	require.True(t, handled, "dispatch should report handled for assistant message")
	require.True(t, callbackCalled)
}

func TestMessageOptionsDispatch_ToolItemNotHandled(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	var callbackCalled bool

	u.chat.OnMessageOptions = func(_, _ string, _ int) tea.Cmd {
		callbackCalled = true
		return nil
	}

	item := testMessageItem{id: "tool-no-opts", text: "tool output"}
	u.chat.SetMessages(item)
	u.chat.SetSelected(0)

	handled, _ := u.chat.dispatchXrushMessageOptions(u.chat.list.SelectedItem())
	require.False(t, handled, "dispatch should NOT handle non-user/assistant items")
	require.False(t, callbackCalled)
}

func TestChatSelectionNavigation(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	msgs := []chat.MessageItem{}
	for i := range 5 {
		msg := &message.Message{
			ID:   "nav-msg-" + string(rune('0'+i)),
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "message " + string(rune('0' + i))},
			},
		}
		msgs = append(msgs, chat.NewUserMessageItem(u.com.Styles, msg, nil))
	}
	u.chat.SetMessages(msgs...)

	u.chat.SelectLast()
	lastIdx := u.chat.list.Selected()
	require.Equal(t, 4, lastIdx, "should select last item")

	u.chat.SelectPrev()
	require.Equal(t, 3, u.chat.list.Selected(), "should move to previous item")

	u.chat.SelectFirst()
	require.Equal(t, 0, u.chat.list.Selected(), "should select first item")

	u.chat.SelectNext()
	require.Equal(t, 1, u.chat.list.Selected(), "should move to next item")
}

func TestRemoveMessage_ByID(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	msg := &message.Message{
		ID:   "remove-me",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "temporary"},
		},
	}
	item := chat.NewUserMessageItem(u.com.Styles, msg, nil)
	u.chat.SetMessages(item)

	require.Equal(t, 1, u.chat.Len())
	require.NotNil(t, u.chat.MessageItem("remove-me"))

	u.chat.RemoveMessage("remove-me")

	require.Equal(t, 0, u.chat.Len(), "chat should be empty after removal")
	require.Nil(t, u.chat.MessageItem("remove-me"), "item should no longer be found")
}

func TestRemoveMessage_NonexistentID(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	msg := &message.Message{
		ID:   "keep-me",
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "stays"},
		},
	}
	item := chat.NewUserMessageItem(u.com.Styles, msg, nil)
	u.chat.SetMessages(item)

	u.chat.RemoveMessage("does-not-exist")

	require.Equal(t, 1, u.chat.Len(), "removing nonexistent ID should be a no-op")
}

// ---------------------------------------------------------------------------
// Compaction message routing tests
// ---------------------------------------------------------------------------

func TestXrushRouting_CompactionStartedMsg(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	require.False(t, u.lcmCompacting)

	cmd := u.handleXrushRoutingUpdate(CompactionStartedMsg{SessionID: "s-1"})
	require.True(t, u.lcmCompacting, "routing should set compacting flag")
	require.NotNil(t, cmd, "should return spinner tick command")
}

func TestXrushRouting_CompactionCompletedMsg(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = true

	cmd := u.handleXrushRoutingUpdate(CompactionCompletedMsg{SessionID: "s-1", Rounds: 2})
	require.False(t, u.lcmCompacting, "routing should clear compacting flag")
	require.Nil(t, cmd, "should return nil after compaction finished")
}

func TestXrushRouting_CompactionFailedMsg(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = true

	cmd := u.handleXrushRoutingUpdate(CompactionFailedMsg{SessionID: "s-1", Err: "OOM"})
	require.False(t, u.lcmCompacting, "failed compaction should also clear flag")
	require.Nil(t, cmd)
}

func TestXrushRouting_UnrecognizedMessage(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	cmd := u.handleXrushRoutingUpdate("some-random-string")
	require.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// LCM spinner update tests
// ---------------------------------------------------------------------------

func TestUpdateLCMSpinner_SkippedWhenNotCompacting(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = false
	u.lcmSpinner = spinner.New()

	cmd := u.updateLCMSpinner(spinner.TickMsg{})
	require.Nil(t, cmd, "should skip spinner update when not compacting")
}

func TestUpdateLCMSpinner_UpdatesWhenCompacting(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.lcmCompacting = true
	u.lcmSpinner = spinner.New(spinner.WithSpinner(spinner.MiniDot))

	_ = u.updateLCMSpinner(spinner.TickMsg{})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func stripANSIInteraction(s string) string {
	var b strings.Builder
	esc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z' {
				esc = false
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
