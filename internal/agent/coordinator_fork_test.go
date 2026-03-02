package agent

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/stretchr/testify/require"
)

// mockSessionAgent mirrors the upstream mock in coordinator_test.go.
// After merging upstream, remove this definition and add
// SetPrepareStepHooks to the upstream mock instead.
type mockSessionAgent struct {
	model     Model
	runFunc   func(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error)
	cancelled []string
}

func (m *mockSessionAgent) Run(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
	return m.runFunc(ctx, call)
}

func (m *mockSessionAgent) Model() Model                                  { return m.model }
func (m *mockSessionAgent) SetModels(large, small Model)                  {}
func (m *mockSessionAgent) SetTools(tools []fantasy.AgentTool)            {}
func (m *mockSessionAgent) SetSystemPrompt(systemPrompt string)           {}
func (m *mockSessionAgent) SetPrepareStepHooks(hooks []PrepareStepHook)   {}
func (m *mockSessionAgent) Cancel(sessionID string)                       { m.cancelled = append(m.cancelled, sessionID) }
func (m *mockSessionAgent) CancelAll()                                    {}
func (m *mockSessionAgent) IsSessionBusy(sessionID string) bool           { return false }
func (m *mockSessionAgent) IsBusy() bool                                  { return false }
func (m *mockSessionAgent) QueuedPrompts(sessionID string) int            { return 0 }
func (m *mockSessionAgent) QueuedPromptsList(sessionID string) []string   { return nil }
func (m *mockSessionAgent) ClearQueue(sessionID string)                   {}
func (m *mockSessionAgent) Summarize(context.Context, string, fantasy.ProviderOptions) error {
	return nil
}

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
	cfgDisabled.Options.DisabledTools = []string{"map_refresh", "agent"}
	cfgDisabled.SetupAgents()

	cDisabled := &coordinator{
		cfg:         cfgDisabled,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: permission.NewPermissionService(env.workingDir, true, nil),
		history:     env.history,
		filetracker: *env.filetracker,
	}

	coderTools, err := cDisabled.buildTools(t.Context(), cfgDisabled.Agents[config.AgentCoder])
	require.NoError(t, err)
	require.NotContains(t, toolNames(coderTools), tools.MapRefreshToolName)

	cfgEnabled, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfgEnabled.Options.DisabledTools = []string{"agent"}
	cfgEnabled.SetupAgents()

	cEnabled := &coordinator{
		cfg:         cfgEnabled,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: permission.NewPermissionService(env.workingDir, true, nil),
		history:     env.history,
		filetracker: *env.filetracker,
	}

	coderTools, err = cEnabled.buildTools(t.Context(), cfgEnabled.Agents[config.AgentCoder])
	require.NoError(t, err)
	require.Contains(t, toolNames(coderTools), tools.MapRefreshToolName)

	taskTools, err := cEnabled.buildTools(t.Context(), cfgEnabled.Agents[config.AgentTask])
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
