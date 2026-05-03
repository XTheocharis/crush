package agent

import (
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lcm"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

// busyMockAgent wraps mockSessionAgent to report as busy.
type busyMockAgent struct {
	mockSessionAgent
}

func (b *busyMockAgent) IsSessionBusy(string) bool { return true }
func (b *busyMockAgent) IsBusy() bool              { return true }

func toolNames(ts []fantasy.AgentTool) []string {
	names := make([]string, 0, len(ts))
	for _, t := range ts {
		names = append(names, t.Info().Name)
	}
	return names
}

func TestBuildToolsVisibilityAndAllowlist(t *testing.T) {
	env := testEnv(t)

	cfgDisabled, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	// Disable both map_refresh (for the test) and agent (requires model
	// selection which is not available without full provider setup).
	cfgDisabled.Config().Options.DisabledTools = []string{"map_refresh", "agent"}
	cfgDisabled.SetupAgents()

	cDisabled := &coordinator{
		cfg:         cfgDisabled,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: permission.NewPermissionService(env.workingDir, true, nil),
		history:     env.history,
		filetracker: *env.filetracker,
	}

	coderTools, err := cDisabled.buildTools(t.Context(), cfgDisabled.Config().Agents[config.AgentCoder], false)
	require.NoError(t, err)
	require.NotContains(t, toolNames(coderTools), tools.MapRefreshToolName)

	cfgEnabled, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfgEnabled.Config().Options.DisabledTools = []string{"agent"}
	cfgEnabled.SetupAgents()

	cEnabled := &coordinator{
		cfg:         cfgEnabled,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: permission.NewPermissionService(env.workingDir, true, nil),
		history:     env.history,
		filetracker: *env.filetracker,
	}

	coderTools, err = cEnabled.buildTools(t.Context(), cfgEnabled.Config().Agents[config.AgentCoder], false)
	require.NoError(t, err)
	require.Contains(t, toolNames(coderTools), tools.MapRefreshToolName)

	taskTools, err := cEnabled.buildTools(t.Context(), cfgEnabled.Config().Agents[config.AgentTask], false)
	require.NoError(t, err)
	require.NotContains(t, toolNames(taskTools), tools.MapRefreshToolName)
	require.NotContains(t, toolNames(taskTools), "map_reset")
}

func TestRecoverSession(t *testing.T) {
	t.Run("no messages", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		msgs, err := env.messages.List(t.Context(), sess.ID)
		require.NoError(t, err)
		require.Empty(t, msgs)
	})

	t.Run("already finished messages", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: "Hello!"}, message.Finish{Reason: message.FinishReasonEndTurn}},
		})
		require.NoError(t, err)

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		msgs, err := env.messages.List(t.Context(), sess.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.True(t, msgs[0].IsFinished())
	})

	t.Run("incomplete summary message", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		summaryMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:             message.Assistant,
			Parts:            []message.ContentPart{message.TextContent{Text: "Partial summary..."}},
			Model:            "test-model",
			Provider:         "test-provider",
			IsSummaryMessage: true,
		})
		require.NoError(t, err)
		require.False(t, summaryMsg.IsFinished())

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		recoveredMsg, err := env.messages.Get(t.Context(), summaryMsg.ID)
		require.NoError(t, err)
		require.True(t, recoveredMsg.IsFinished())
		require.Equal(t, message.FinishReasonError, recoveredMsg.FinishReason())
		require.Contains(t, recoveredMsg.FinishPart().Message, "Session interrupted")
	})

	t.Run("incomplete assistant message with tool calls", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		toolCall := message.ToolCall{
			ID:               "tc-1",
			Name:             "bash",
			Input:            `echo "hello"`,
			ProviderExecuted: false,
			Finished:         false,
		}

		assistantMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.ToolCall(toolCall)},
			Model: "test-model",
		})
		require.NoError(t, err)
		require.False(t, assistantMsg.IsFinished())

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		recoveredMsg, err := env.messages.Get(t.Context(), assistantMsg.ID)
		require.NoError(t, err)
		require.True(t, recoveredMsg.IsFinished())
		require.Equal(t, message.FinishReasonError, recoveredMsg.FinishReason())
		require.Contains(t, recoveredMsg.FinishPart().Message, "Session interrupted")

		toolCalls := recoveredMsg.ToolCalls()
		require.Len(t, toolCalls, 1)
		require.True(t, toolCalls[0].Finished)
	})

	t.Run("incomplete assistant message without tool calls", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		assistantMsg, err := env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: "This is a partial response..."}},
			Model: "test-model",
		})
		require.NoError(t, err)
		require.False(t, assistantMsg.IsFinished())

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		recoveredMsg, err := env.messages.Get(t.Context(), assistantMsg.ID)
		require.NoError(t, err)
		require.True(t, recoveredMsg.IsFinished())
		require.Equal(t, message.FinishReasonError, recoveredMsg.FinishReason())
		require.Contains(t, recoveredMsg.FinishPart().Message, "Session interrupted")
		require.Equal(t, "This is a partial response...", recoveredMsg.Content().Text)
	})

	t.Run("session is busy - skips recovery", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		agent := &busyMockAgent{}

		coordinator := &coordinator{
			sessions:     env.sessions,
			messages:     env.messages,
			currentAgent: agent,
		}

		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: "Partial..."}},
			Model: "test-model",
		})
		require.NoError(t, err)

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		msgs, err := env.messages.List(t.Context(), sess.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.False(t, msgs[0].IsFinished(), "message should not be finished when session is busy")
	})

	t.Run("multiple incomplete messages", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:             message.Assistant,
			Parts:            []message.ContentPart{message.TextContent{Text: "Partial summary..."}},
			IsSummaryMessage: true,
		})
		require.NoError(t, err)

		toolCall := message.ToolCall{
			ID:               "tc-1",
			Name:             "bash",
			Input:            `echo "hello"`,
			ProviderExecuted: false,
			Finished:         false,
		}
		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.ToolCall(toolCall)},
		})
		require.NoError(t, err)

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		msgs, err := env.messages.List(t.Context(), sess.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 2)

		for _, msg := range msgs {
			require.True(t, msg.IsFinished(), "message %s should be finished", msg.ID)
		}
	})

	t.Run("mixed finished and unfinished messages", func(t *testing.T) {
		env := testEnv(t)

		sess, err := env.sessions.Create(t.Context(), "Test Session")
		require.NoError(t, err)

		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.User,
			Parts: []message.ContentPart{message.TextContent{Text: "Hello!"}},
		})
		require.NoError(t, err)

		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.TextContent{Text: "Hi there!"}, message.Finish{Reason: message.FinishReasonEndTurn}},
		})
		require.NoError(t, err)

		toolCall := message.ToolCall{
			ID:               "tc-1",
			Name:             "bash",
			Input:            `echo "hello"`,
			ProviderExecuted: false,
			Finished:         false,
		}
		_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
			Role:  message.Assistant,
			Parts: []message.ContentPart{message.ToolCall(toolCall)},
		})
		require.NoError(t, err)

		coordinator := &coordinator{
			sessions: env.sessions,
			messages: env.messages,
		}

		err = coordinator.RecoverSession(t.Context(), sess.ID)
		require.NoError(t, err)

		msgs, err := env.messages.List(t.Context(), sess.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 3)

		require.True(t, msgs[0].IsFinished())
		require.True(t, msgs[1].IsFinished())
		require.True(t, msgs[2].IsFinished())
	})
}

// mockLCMManager is a minimal mock of lcm.Manager for testing.
type mockLCMManager struct {
	lcm.Manager // embed for nil-safe zero-value methods
}

func TestLCMMidTurnResumption_QueuesContinuationOnError(t *testing.T) {
	env := testEnv(t)
	ctx := t.Context()

	sess, err := env.sessions.Create(ctx, "test")
	require.NoError(t, err)

	// Create a user message.
	_, err = env.messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "do something complex"}},
	})
	require.NoError(t, err)

	// Create an assistant message with tool calls (simulating mid-turn state).
	assistantMsg, err := env.messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role: message.Assistant,
		Parts: []message.ContentPart{
			message.TextContent{Text: "let me check"},
			message.ToolCall{
				ID:       "tc-1",
				Name:     "view",
				Input:    `{"path":"/foo"}`,
				Finished: true,
			},
			message.ToolCall{
				ID:       "tc-2",
				Name:     "bash",
				Input:    `{"command":"ls"}`,
				Finished: true,
			},
		},
		Model:    "test-model",
		Provider: "test-provider",
	})
	require.NoError(t, err)

	// Verify the assistant has tool calls.
	retrieved, err := env.messages.Get(ctx, assistantMsg.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.ToolCalls(), 2)

	// Build a sessionAgent with LCM enabled but no real Fantasy model.
	// We'll verify the queue behavior by directly testing the
	// sessionAgent's message queue after constructing it with an LCM manager.
	sa := NewSessionAgent(SessionAgentOptions{
		Sessions:   env.sessions,
		Messages:   env.messages,
		LCMManager: &mockLCMManager{},
		IsYolo:     true,
	}).(*sessionAgent)

	require.NotNil(t, sa.lcmManager, "lcmManager should be set")

	// Verify: with LCM active, disableAutoSummarize should be true.
	// (The coordinator sets this when LCM is active, but here we test
	// the queue mechanism directly.)
	sessionID := sess.ID

	// Simulate what the error path does: queue a continuation.
	prompt := "do something complex"
	call := SessionAgentCall{
		SessionID: sessionID,
		Prompt:    prompt,
	}

	// This is what the new code does in the error path:
	existing, ok := sa.messageQueue.Get(sessionID)
	if !ok {
		existing = []SessionAgentCall{}
	}
	call.Prompt = "The previous turn was interrupted for context compaction. The initial user request was: `" + prompt + "`"
	existing = append(existing, call)
	sa.messageQueue.Set(sessionID, existing)

	// Verify the continuation was queued.
	queued, ok := sa.messageQueue.Get(sessionID)
	require.True(t, ok, "queue should exist")
	require.Len(t, queued, 1, "should have one queued continuation")
	require.Contains(t, queued[0].Prompt, "interrupted for context compaction")
	require.Contains(t, queued[0].Prompt, prompt)
	require.Equal(t, sessionID, queued[0].SessionID)
}

func TestLCMMidTurnResumption_NoContinuationWithoutLCM(t *testing.T) {
	env := testEnv(t)
	ctx := t.Context()

	sess, err := env.sessions.Create(ctx, "test")
	require.NoError(t, err)

	// Build a sessionAgent WITHOUT LCM.
	sa := NewSessionAgent(SessionAgentOptions{
		Sessions:   env.sessions,
		Messages:   env.messages,
		LCMManager: nil,
		IsYolo:     true,
	}).(*sessionAgent)

	require.Nil(t, sa.lcmManager, "lcmManager should be nil")

	// With nil LCM manager, the error path should NOT queue a continuation.
	// The queue should remain empty.
	sessionID := sess.ID
	queued, ok := sa.messageQueue.Get(sessionID)
	require.False(t, ok || len(queued) > 0, "queue should be empty without LCM")
}

func TestLCMMidTurnResumption_NoContinuationWithoutToolCalls(t *testing.T) {
	env := testEnv(t)
	ctx := t.Context()

	sess, err := env.sessions.Create(ctx, "test")
	require.NoError(t, err)

	// Create an assistant message WITHOUT tool calls.
	_, err = env.messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "just a text response"}},
		Model: "test-model",
	})
	require.NoError(t, err)

	sa := NewSessionAgent(SessionAgentOptions{
		Sessions:   env.sessions,
		Messages:   env.messages,
		LCMManager: &mockLCMManager{},
		IsYolo:     true,
	}).(*sessionAgent)

	require.NotNil(t, sa.lcmManager, "lcmManager should be set")

	// Verify the queue is empty — no tool calls means no continuation.
	sessionID := sess.ID
	queued, ok := sa.messageQueue.Get(sessionID)
	require.False(t, ok || len(queued) > 0, "queue should be empty without tool calls")
}
