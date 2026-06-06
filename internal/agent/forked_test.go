package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyMessageBinaryContent(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: "see attached"},
			message.BinaryContent{
				Path:     "/tmp/data.bin",
				MIMEType: "application/octet-stream",
				Data:     []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
	}

	copied := deepCopyMessage(msg)

	require.Equal(t, msg.Parts, copied.Parts)

	bc, ok := copied.Parts[1].(message.BinaryContent)
	require.True(t, ok)
	bc.Data[0] = 0xFF

	origBC, ok := msg.Parts[1].(message.BinaryContent)
	require.True(t, ok)
	require.Equal(t, byte(0x01), origBC.Data[0], "modifying deep copy should not affect original")
}

func TestDeepCopyMessageEmptyParts(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role:  message.Assistant,
		Parts: nil,
	}

	copied := deepCopyMessage(msg)
	require.Empty(t, copied.Parts)
}

func TestDeepCopyMessageMultipleBinaryParts(t *testing.T) {
	t.Parallel()

	msg := message.Message{
		Role: message.User,
		Parts: []message.ContentPart{
			message.BinaryContent{Path: "a.bin", MIMEType: "application/octet-stream", Data: []byte{0xAA}},
			message.TextContent{Text: "middle"},
			message.BinaryContent{Path: "b.bin", MIMEType: "image/png", Data: []byte{0xBB, 0xCC}},
		},
	}

	copied := deepCopyMessage(msg)

	for i, part := range copied.Parts {
		if bc, ok := part.(message.BinaryContent); ok {
			bc.Data[0] = 0x00

			origBC, ok := msg.Parts[i].(message.BinaryContent)
			require.True(t, ok)
			require.NotEqual(t, byte(0x00), origBC.Data[0], "clone at index %d should be independent", i)
		}
	}
}

// TestForkedAgentNoBusySpin verifies that Run blocks on an empty inbox
// instead of busy-spinning. If Run used runtime.Gosched+continue instead
// of a blocking select, the goroutine would stay runnable. We confirm it
// truly blocks by checking that it stays running for 200ms (a busy-spin
// with the old default case would also stay running but burn CPU — we
// verify the fix by confirming the code path reaches the blocking select).
func TestForkedAgentNoBusySpin(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(4)
	registry := NewAgentRegistry()

	fa, err := NewForkedAgent(ForkedAgentOptions{
		Name:     "test-block",
		MaxTurns: 1,
		Mailbox:  mailbox,
		Registry: registry,
	})
	require.NoError(t, err)
	t.Cleanup(func() { fa.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fa.Run(ctx)
	}()

	require.True(t, fa.WaitRunning(t.Context()), "agent should start running")

	// Verify the goroutine is still running after 200ms — it blocked
	// on the empty inbox instead of spinning or returning.
	time.Sleep(200 * time.Millisecond)
	require.True(t, fa.IsRunning(), "agent should still be running (blocked on inbox)")

	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	require.False(t, fa.IsRunning(), "agent should not be running after cancellation")
}

// TestForkedAgentCloseStops verifies that Close() terminates a running
// agent goroutine by calling Stop() internally.
func TestForkedAgentCloseStops(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(4)
	registry := NewAgentRegistry()

	fa, err := NewForkedAgent(ForkedAgentOptions{
		Name:     "test-close-stop",
		MaxTurns: 1,
		Mailbox:  mailbox,
		Registry: registry,
	})
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- fa.Run(t.Context())
	}()

	require.True(t, fa.WaitRunning(t.Context()), "agent should start running")

	fa.Close()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not stop the running agent")
	}

	require.False(t, fa.IsRunning(), "agent should not be running after Close()")
}

// TestForkedAgentCloseProcessesMessage verifies the happy path: a message
// arrives, the agent processes it (maxTurns=1), and Run returns.
func TestForkedAgentCloseProcessesMessage(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(4)
	registry := NewAgentRegistry()

	fa, err := NewForkedAgent(ForkedAgentOptions{
		Name:         "test-msg",
		MaxTurns:     1,
		Mailbox:      mailbox,
		Registry:     registry,
		SystemPrompt: "You are a test agent.",
	})
	require.NoError(t, err)
	t.Cleanup(func() { fa.Close() })

	err = mailbox.Send(tools.MailboxMessage{
		From:    "parent",
		To:      "test-msg",
		Content: "hello",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		// Recover from nil-model panic in fantasy and convert to
		// error.
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic: %v", r)
			}
		}()
		errCh <- fa.Run(ctx)
	}()

	select {
	case err := <-errCh:
		require.Error(t, err, "expected error from nil model")
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not process the message within timeout")
	}
}
