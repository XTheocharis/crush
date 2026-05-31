package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/stretchr/testify/require"
)

func TestSiblingError_NotificationSent(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	siblingInbox := mailbox.Register("sibling-1")
	_ = mailbox.Register("failer")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("boom")
	}, "", "failer")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)

	select {
	case msg := <-siblingInbox:
		require.Equal(t, "failer", msg.From)
		require.Equal(t, "sibling-1", msg.To)
		require.Equal(t, tools.MailboxMessageSiblingError, msg.Type)
		require.Contains(t, msg.Content, "boom")
	case <-time.After(2 * time.Second):
		t.Fatal("expected sibling error notification but got none")
	}
}

func TestSiblingError_MultipleSiblings(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	inbox1 := mailbox.Register("sibling-1")
	inbox2 := mailbox.Register("sibling-2")
	_ = mailbox.Register("failer")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "", "failer")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)

	received := []string{}
	for _, ch := range []<-chan MailboxMessage{inbox1, inbox2} {
		select {
		case msg := <-ch:
			require.Equal(t, tools.MailboxMessageSiblingError, msg.Type)
			require.Equal(t, "failer", msg.From)
			received = append(received, msg.To)
		case <-time.After(2 * time.Second):
			t.Fatal("expected sibling error notification")
		}
	}
	require.Len(t, received, 2)
}

func TestSiblingError_SiblingsCanContinue(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	siblingInbox := mailbox.Register("sibling-1")
	_ = mailbox.Register("failer")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	var siblingCompleted atomic.Bool
	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		_ = <-siblingInbox
		siblingCompleted.Store(true)
		return "done", nil
	}, "", "sibling-1")
	require.NoError(t, err)

	_, failErr := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "", "failer")
	require.NoError(t, failErr)

	_, _ = pc.WaitAll(context.Background())
	require.True(t, siblingCompleted.Load(), "sibling should have completed cooperatively")
}

func TestSiblingError_NoMailbox(t *testing.T) {
	t.Parallel()

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	defer pc.Shutdown()

	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "", "branch-1")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)
}

func TestSiblingError_NoBranchName(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	_ = mailbox.Register("some-agent")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	_, err := pc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)
}

func TestSiblingError_ErrorExcludesSelf(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	failerInbox := mailbox.Register("failer")
	_ = mailbox.Register("sibling-1")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, errors.New("fail")
	}, "", "failer")
	require.NoError(t, err)

	_, waitErr := pc.WaitAll(context.Background())
	require.Error(t, waitErr)

	select {
	case msg := <-failerInbox:
		t.Fatalf("failer should not receive its own error notification, got: %v", msg)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSiblingError_MailboxBroadcast(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	inbox1 := mailbox.Register("a")
	inbox2 := mailbox.Register("b")
	inbox3 := mailbox.Register("c")

	msg := MailboxMessage{
		From:      "c",
		Content:   "test broadcast",
		Type:      tools.MailboxMessageSiblingError,
		Timestamp: time.Now(),
	}
	errs := mailbox.Broadcast(msg, "c")
	require.Empty(t, errs)

	var received []string
	for _, ch := range []<-chan MailboxMessage{inbox1, inbox2} {
		select {
		case m := <-ch:
			require.Equal(t, "c", m.From)
			require.Equal(t, tools.MailboxMessageSiblingError, m.Type)
			received = append(received, m.To)
		case <-time.After(time.Second):
			t.Fatal("expected broadcast message")
		}
	}
	require.Equal(t, []string{"a", "b"}, received)

	select {
	case m := <-inbox3:
		t.Fatalf("excluded inbox should not receive, got: %v", m)
	default:
	}
}

func TestSiblingError_ErrorContent(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	siblingInbox := mailbox.Register("sibling")
	_ = mailbox.Register("failing-branch")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	testErr := errors.New("database connection refused")
	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		return nil, testErr
	}, "", "failing-branch")
	require.NoError(t, err)

	_, _ = pc.WaitAll(context.Background())

	select {
	case msg := <-siblingInbox:
		require.Equal(t, "failing-branch", msg.From)
		require.Contains(t, msg.Content, "failing-branch")
		require.Contains(t, msg.Content, "database connection refused")
		require.Equal(t, tools.MailboxMessageSiblingError, msg.Type)
		require.False(t, msg.Timestamp.IsZero())
	case <-time.After(2 * time.Second):
		t.Fatal("expected sibling error notification")
	}
}

func TestSiblingError_ConcurrentFailure(t *testing.T) {
	t.Parallel()

	mailbox := NewMailbox(64)
	inbox1 := mailbox.Register("worker-1")
	inbox2 := mailbox.Register("worker-2")
	_ = mailbox.Register("worker-3")

	pc := NewParallelController(ParallelControllerConfig{MaxConcurrent: 5})
	pc.SetMailbox(mailbox)
	defer pc.Shutdown()

	var started sync.WaitGroup
	started.Add(3)

	_, err := pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		started.Done()
		time.Sleep(50 * time.Millisecond)
		return nil, errors.New("worker-3 failed")
	}, "", "worker-3")
	require.NoError(t, err)

	_, err = pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		started.Done()
		time.Sleep(5 * time.Second)
		return "ok", nil
	}, "", "worker-1")
	require.NoError(t, err)

	_, err = pc.SubmitWithName(context.Background(), func(ctx context.Context) (any, error) {
		started.Done()
		time.Sleep(5 * time.Second)
		return "ok", nil
	}, "", "worker-2")
	require.NoError(t, err)

	started.Wait()

	_, _ = pc.WaitAll(context.Background())

	var receivedBy []string
	remaining := []*<-chan MailboxMessage{&inbox1, &inbox2}
	for _, ch := range remaining {
		select {
		case msg := <-*ch:
			require.Equal(t, tools.MailboxMessageSiblingError, msg.Type)
			receivedBy = append(receivedBy, msg.To)
		case <-time.After(2 * time.Second):
		}
	}

	if len(receivedBy) < 1 {
		t.Fatal("expected at least one sibling to receive error notification")
	}
}
