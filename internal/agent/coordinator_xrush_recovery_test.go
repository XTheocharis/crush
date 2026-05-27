package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestRecoverSession_CleanSession(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	sess, err := env.sessions.Create(t.Context(), "test-clean")
	require.NoError(t, err)

	finishedMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "hello"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	got, err := env.messages.Get(t.Context(), finishedMsg.ID)
	require.NoError(t, err)
	require.True(t, got.IsFinished())
	require.Equal(t, message.FinishReasonEndTurn, got.FinishReason())
}

func TestRecoverSession_InterruptedThinking(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	sess, err := env.sessions.Create(t.Context(), "test-thinking")
	require.NoError(t, err)

	interruptedMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "I was thinking...", StartedAt: 1000},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	got, err := env.messages.Get(t.Context(), interruptedMsg.ID)
	require.NoError(t, err)
	require.True(t, got.IsFinished())
	require.Equal(t, message.FinishReasonError, got.FinishReason())

	reasoning := got.ReasoningContent()
	require.NotZero(t, reasoning.FinishedAt, "FinishThinking should set FinishedAt")
}

func TestRecoverSession_InterruptedToolCall(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	sess, err := env.sessions.Create(t.Context(), "test-toolcall")
	require.NoError(t, err)

	interruptedMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc_123",
				Name:     "bash",
				Input:    `{"command":"ls"}`,
				Finished: false,
			},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	got, err := env.messages.Get(t.Context(), interruptedMsg.ID)
	require.NoError(t, err)
	require.True(t, got.IsFinished())
	require.Equal(t, message.FinishReasonError, got.FinishReason())

	toolCalls := got.ToolCalls()
	require.Len(t, toolCalls, 1)
	require.True(t, toolCalls[0].Finished, "unfinished tool call should be marked finished")
	require.Equal(t, "tc_123", toolCalls[0].ID)
}

func TestRecoverSession_MultipleInterrupted(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	sess, err := env.sessions.Create(t.Context(), "test-multi")
	require.NoError(t, err)

	msg1, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "thinking 1", StartedAt: 1000},
		},
	})
	require.NoError(t, err)

	msg2, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "partial response"},
			message.Finish{Reason: message.FinishReasonEndTurn},
		},
	})
	require.NoError(t, err)

	msg3, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc_456",
				Name:     "edit",
				Input:    `{"path":"foo.go"}`,
				Finished: false,
			},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	got1, err := env.messages.Get(t.Context(), msg1.ID)
	require.NoError(t, err)
	require.True(t, got1.IsFinished())
	require.Equal(t, message.FinishReasonError, got1.FinishReason())

	got2, err := env.messages.Get(t.Context(), msg2.ID)
	require.NoError(t, err)
	require.True(t, got2.IsFinished())
	require.Equal(t, message.FinishReasonEndTurn, got2.FinishReason(), "already-finished message should not change")

	got3, err := env.messages.Get(t.Context(), msg3.ID)
	require.NoError(t, err)
	require.True(t, got3.IsFinished())
	require.Equal(t, message.FinishReasonError, got3.FinishReason())
	require.True(t, got3.ToolCalls()[0].Finished)
}

func TestRecoverSession_PartialToolResults(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	sess, err := env.sessions.Create(t.Context(), "test-partial-results")
	require.NoError(t, err)

	interruptedMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ToolCall{
				ID:       "tc_789",
				Name:     "bash",
				Input:    `{"command":"echo hi"}`,
				Finished: true,
			},
			message.ToolCall{
				ID:       "tc_790",
				Name:     "edit",
				Input:    `{"path":"bar.go"}`,
				Finished: false,
			},
			message.ToolResult{
				ToolCallID: "tc_789",
				Content:    "hi\n",
			},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	got, err := env.messages.Get(t.Context(), interruptedMsg.ID)
	require.NoError(t, err)
	require.True(t, got.IsFinished())
	require.Equal(t, message.FinishReasonError, got.FinishReason())

	toolCalls := got.ToolCalls()
	require.Len(t, toolCalls, 2)
	require.True(t, toolCalls[0].Finished, "already finished tool call should remain finished")
	require.True(t, toolCalls[1].Finished, "unfinished tool call should be marked finished")
	require.Equal(t, "tc_790", toolCalls[1].ID)

	toolResults := got.ToolResults()
	require.Len(t, toolResults, 1, "existing tool results should be preserved")
	require.Equal(t, "tc_789", toolResults[0].ToolCallID)
}

func TestRecoverSession_BusySession(t *testing.T) {
	t.Parallel()
	env := testEnv(t)

	busyAgent := &mockSessionAgent{
		busySessions: map[string]bool{"sess-busy": true},
	}
	c := &coordinator{
		messages:     env.messages,
		currentAgent: busyAgent,
	}

	sess, err := env.sessions.Create(t.Context(), "test-busy")
	require.NoError(t, err)

	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "thinking", StartedAt: 1000},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), "sess-busy")
	require.NoError(t, err)

	msgs, err := env.messages.List(t.Context(), sess.ID)
	require.NoError(t, err)
	require.False(t, msgs[0].IsFinished(), "busy session should not be recovered")
}

func TestRecoverSession_NonexistentSession(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: &mockSessionAgent{},
	}

	err := c.RecoverSession(t.Context(), "nonexistent-session-id")
	require.NoError(t, err)
}

func TestRecoverSession_NilCurrentAgent(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	c := &coordinator{
		messages:     env.messages,
		currentAgent: nil,
	}

	sess, err := env.sessions.Create(t.Context(), "test-nil-agent")
	require.NoError(t, err)

	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.ReasoningContent{Thinking: "thinking", StartedAt: 1000},
		},
	})
	require.NoError(t, err)

	err = c.RecoverSession(t.Context(), sess.ID)
	require.NoError(t, err)

	msgs, err := env.messages.List(t.Context(), sess.ID)
	require.NoError(t, err)
	require.True(t, msgs[0].IsFinished(), "nil agent should still recover messages")
}
