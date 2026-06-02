package extensions

import (
	"context"
	"database/sql"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

type resourceMockHost struct {
	cfg *config.Config
}

func (m *resourceMockHost) Config() *config.Config                    { return m.cfg }
func (m *resourceMockHost) WorkingDir() string                        { return "/tmp" }
func (m *resourceMockHost) Completer() ext.TextCompleter              { return nil }
func (m *resourceMockHost) RegisterTools(ext.ToolProvider)            {}
func (m *resourceMockHost) RegisterRunHooks(ext.RunHookProvider)      {}
func (m *resourceMockHost) RegisterStepHooks(ext.StepHookProvider)    {}
func (m *resourceMockHost) RegisterPromptHook(ext.PromptHookProvider) {}
func (m *resourceMockHost) PublishEvent(_ context.Context, _ string, _ any) error {
	return nil
}
func (m *resourceMockHost) LSP() *lsp.Manager               { return nil }
func (m *resourceMockHost) DB() *sql.DB                     { return nil }
func (m *resourceMockHost) Sessions() session.Service       { return nil }
func (m *resourceMockHost) Messages() message.Service       { return nil }
func (m *resourceMockHost) RewindService() rewind.Service   { return nil }
func (m *resourceMockHost) ToolDefs() []processor.ToolDef   { return nil }
func (m *resourceMockHost) SkillDefs() []processor.SkillDef { return nil }

func TestResourceLimitsExtension_NameAndLifecycle(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	require.Equal(t, "resource-limits", e.Name())

	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.False(t, e.active)
}

func TestResourceLimitsExtension_StepHooksInactive(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	require.Nil(t, e.StepHooks())
}

func TestResourceLimitsExtension_OnStepFinishTracking(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	require.Equal(t, "resource-limits-check", hooks[0].Name)
	require.NotNil(t, hooks[0].OnStepFinish)

	onFinish := hooks[0].OnStepFinish

	step := fantasy.StepResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: "Hello world"},
			},
		},
	}

	require.Equal(t, int32(0), e.usage.StepsTaken.Load())

	err = onFinish(context.Background(), "s1", step)
	require.NoError(t, err)
	require.Equal(t, int32(1), e.usage.StepsTaken.Load())

	err = onFinish(context.Background(), "s1", step)
	require.NoError(t, err)
	require.Equal(t, int32(2), e.usage.StepsTaken.Load())

	require.True(t, e.usage.TokensUsed.Load() > 0, "tokens should have been tracked")
}

func TestResourceLimitsExtension_OnStepFinishEmptyContent(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	hooks := e.StepHooks()
	onFinish := hooks[0].OnStepFinish

	step := fantasy.StepResult{}
	err = onFinish(context.Background(), "s1", step)
	require.NoError(t, err)
	require.Equal(t, int32(1), e.usage.StepsTaken.Load())
	require.Equal(t, int64(0), e.usage.TokensUsed.Load(), "no text means no tokens")
}

func TestResourceLimitsExtension_OnStepFinishAfterShutdown(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	hooks := e.StepHooks()
	onFinish := hooks[0].OnStepFinish

	err = e.Shutdown(context.Background())
	require.NoError(t, err)

	step := fantasy.StepResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: "should be ignored"},
			},
		},
	}
	err = onFinish(context.Background(), "s1", step)
	require.NoError(t, err)
}

func TestResourceLimitsEnforcementHalts(t *testing.T) {
	t.Parallel()

	profile := config.Config{}
	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &profile}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	hooks := e.StepHooks()
	require.Len(t, hooks, 1)
	require.NotNil(t, hooks[0].StopCondition)

	t.Run("hard_step_limit", func(t *testing.T) {
		t.Parallel()

		ext := &ResourceLimitsExtension{}
		h := &resourceMockHost{cfg: &config.Config{}}
		require.NoError(t, ext.Init(context.Background(), h))
		t.Cleanup(func() { _ = ext.Shutdown(context.Background()) })

		hk := ext.StepHooks()[0]

		limits := ext.limits
		for range int(limits.MaxSteps.Hard) + 1 {
			require.NoError(t, hk.OnStepFinish(context.Background(), "s1", fantasy.StepResult{}))
		}

		require.True(t, hk.StopCondition(context.Background(), nil),
			"should stop when hard step limit exceeded")
	})

	t.Run("hard_token_limit", func(t *testing.T) {
		t.Parallel()

		ext := &ResourceLimitsExtension{}
		h := &resourceMockHost{cfg: &config.Config{}}
		require.NoError(t, ext.Init(context.Background(), h))
		t.Cleanup(func() { _ = ext.Shutdown(context.Background()) })

		hk := ext.StepHooks()[0]

		limits := ext.limits
		bigText := makeTextExceedingTokens(int(limits.MaxTokens.Hard))
		step := fantasy.StepResult{
			Response: fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: bigText},
				},
			},
		}
		require.NoError(t, hk.OnStepFinish(context.Background(), "s1", step))

		require.True(t, hk.StopCondition(context.Background(), nil),
			"should stop when hard token limit exceeded")
	})
}

func TestResourceLimitsWarningWithinBounds(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	hooks := e.StepHooks()
	require.Len(t, hooks, 1)

	hk := hooks[0]

	step := fantasy.StepResult{
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: "small step content"},
			},
		},
	}

	require.NoError(t, hk.OnStepFinish(context.Background(), "s1", step))

	require.False(t, hk.StopCondition(context.Background(), nil),
		"should not stop when within all limits")
}

func TestResourceLimitsSoftLimitWarnOnly(t *testing.T) {
	t.Parallel()

	e := &ResourceLimitsExtension{}
	host := &resourceMockHost{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Shutdown(context.Background()) })

	hooks := e.StepHooks()
	hk := hooks[0]

	limits := e.limits
	for range int(limits.MaxSteps.Soft) + 1 {
		require.NoError(t, hk.OnStepFinish(context.Background(), "s1", fantasy.StepResult{}))
	}

	require.False(t, hk.StopCondition(context.Background(), nil),
		"soft limit should warn only, not stop")
}

// makeTextExceedingTokens generates a string long enough to exceed the given
// token count using the default chars-per-token ratio.
func makeTextExceedingTokens(tokens int) string {
	chars := tokens * agent.DefaultCharsPerToken
	result := make([]byte, chars)
	for i := range result {
		result[i] = 'a'
	}
	return string(result)
}
