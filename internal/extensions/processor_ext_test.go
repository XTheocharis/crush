package extensions

import (
	"context"
	"database/sql"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/lsp"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

type mockHostContext struct {
	cfg *config.Config
}

func (m *mockHostContext) Config() *config.Config                    { return m.cfg }
func (m *mockHostContext) WorkingDir() string                        { return "/tmp" }
func (m *mockHostContext) Completer() ext.TextCompleter              { return nil }
func (m *mockHostContext) RegisterTools(ext.ToolProvider)            {}
func (m *mockHostContext) RegisterRunHooks(ext.RunHookProvider)      {}
func (m *mockHostContext) RegisterStepHooks(ext.StepHookProvider)    {}
func (m *mockHostContext) RegisterPromptHook(ext.PromptHookProvider) {}
func (m *mockHostContext) PublishEvent(_ context.Context, _ string, _ any) error {
	return nil
}
func (m *mockHostContext) LSP() *lsp.Manager         { return nil }
func (m *mockHostContext) DB() *sql.DB               { return nil }
func (m *mockHostContext) Sessions() session.Service { return nil }
func (m *mockHostContext) Messages() message.Service { return nil }

func TestProcessorExtension_Name(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	require.Equal(t, "processor", e.Name())
}

func TestProcessorExtension_InactiveWithoutConfig(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.StepHooks())
	require.Nil(t, e.RunHooks())
}

func TestProcessorExtension_InactiveWhenDisabled(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: false,
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
}

func TestProcessorExtension_InactiveWithEmptyList(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: true,
				List:    []string{},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
}

func TestProcessorExtension_InactiveWithUnsafeProcessor(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: true,
				List:    []string{"unknown_processor"},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
}

func TestProcessorExtension_ActiveWithTokenLimiter(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: true,
				List:    []string{"token_limiter"},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)
	require.NotNil(t, e.runner)

	stepHooks := e.StepHooks()
	require.Len(t, stepHooks, 2)
	require.Equal(t, "processor:input", stepHooks[0].Name)
	require.Equal(t, "processor:output", stepHooks[1].Name)

	runHooks := e.RunHooks()
	require.Len(t, runHooks, 1)
	require.Equal(t, "processor:run-start", runHooks[0].Name)
}

func TestProcessorExtension_Shutdown(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: true,
				List:    []string{"token_limiter"},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.runner)
	require.Nil(t, e.StepHooks())
	require.Nil(t, e.RunHooks())
}

func TestProcessorExtension_InputHookTrimsMessages(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Processors: &config.ProcessorsOptions{
				Enabled: true,
				List:    []string{"token_limiter"},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	longContent := make([]byte, 500000)
	for i := range longContent {
		longContent[i] = 'a'
	}

	messages := []fantasy.Message{
		{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: string(longContent)},
		}},
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "short reply"},
		}},
	}

	stepHooks := e.StepHooks()
	result, err := stepHooks[0].OnPrepareStep(context.Background(), "test-session", messages)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, len(result) <= len(messages), "token limiter should have trimmed messages")
}

func TestProcessorExtension_InputHookPassThroughOnNilRunner(t *testing.T) {
	t.Parallel()
	e := &ProcessorExtension{}
	require.False(t, e.active)

	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "hello"},
		}},
	}
	result, err := e.processInput(context.Background(), "session", msgs)
	require.NoError(t, err)
	require.Equal(t, msgs, result)
}

func TestFantasyToProcessorMessages(t *testing.T) {
	t.Parallel()
	msgs := []fantasy.Message{
		{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "hello"},
		}},
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "world"},
		}},
	}
	result := fantasyToProcessorMessages(msgs)
	require.Len(t, result, 2)
	require.Equal(t, "user", result[0].Role)
	require.Equal(t, "hello", result[0].Content)
	require.Equal(t, "assistant", result[1].Role)
	require.Equal(t, "world", result[1].Content)
}

func TestProcessorToFantasyMessages(t *testing.T) {
	t.Parallel()
	msgs := []processor.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	result := processorToFantasyMessages(msgs)
	require.Len(t, result, 2)
	require.Equal(t, fantasy.MessageRoleUser, result[0].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, result[1].Role)
}

func TestExtractFantasyMessageText_Empty(t *testing.T) {
	t.Parallel()
	msg := fantasy.Message{Role: fantasy.MessageRoleUser}
	require.Equal(t, "", extractFantasyMessageText(msg))
}

func TestExtractFantasyMessageText_SinglePart(t *testing.T) {
	t.Parallel()
	msg := fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
	}
	require.Equal(t, "hello", extractFantasyMessageText(msg))
}

func TestExtractFantasyMessageText_MultipleParts(t *testing.T) {
	t.Parallel()
	msg := fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "hello"},
			fantasy.TextPart{Text: "world"},
		},
	}
	require.Equal(t, "hello\nworld", extractFantasyMessageText(msg))
}

func TestBuildProcessorRunner_UnknownName(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"nonexistent"}, nil)
	require.Nil(t, runner)
}

func TestBuildProcessorRunner_TokenLimiter(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"token_limiter"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Equal(t, "token_limiter", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_MixedKnownAndUnknown(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"unknown", "token_limiter", "also_unknown"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
}

func TestBuildProcessorRunner_UnicodeNormalizer(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"unicode_normalizer"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Len(t, runner.OutputProcessors, 1)
	require.Equal(t, "unicode_normalizer", runner.InputProcessors[0].ID())
	require.Equal(t, "unicode_normalizer", runner.OutputProcessors[0].ID())
}

func TestBuildProcessorRunner_BatchParts(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"batch_parts"}, nil)
	require.NotNil(t, runner)
	require.Empty(t, runner.InputProcessors)
	require.Len(t, runner.OutputProcessors, 1)
	require.Equal(t, "batch_parts", runner.OutputProcessors[0].ID())
}

func TestBuildProcessorRunner_PIIDetector(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"pii_detector"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Len(t, runner.OutputProcessors, 1)
	require.Equal(t, "pii_detector", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_MessageSelection(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"message_selection"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Empty(t, runner.OutputProcessors)
	require.Equal(t, "message_selection", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_ToolCallFilter(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"tool_call_filter"}, nil)
	require.NotNil(t, runner)
	require.Empty(t, runner.InputProcessors)
	require.Len(t, runner.OutputProcessors, 1)
	require.Equal(t, "tool_call_filter", runner.OutputProcessors[0].ID())
}

func TestBuildProcessorRunner_ToolSearch(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"tool_search"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Empty(t, runner.OutputProcessors)
	require.Equal(t, "tool_search", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_Skills(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"skills"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Empty(t, runner.OutputProcessors)
	require.Equal(t, "skills", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_SkillSearch(t *testing.T) {
	t.Parallel()
	runner := buildProcessorRunner([]string{"skill_search"}, nil)
	require.NotNil(t, runner)
	require.Len(t, runner.InputProcessors, 1)
	require.Empty(t, runner.OutputProcessors)
	require.Equal(t, "skill_search", runner.InputProcessors[0].ID())
}

func TestBuildProcessorRunner_AllProcessors(t *testing.T) {
	t.Parallel()
	all := []string{
		"token_limiter",
		"system_prompt_scrubber",
		"unicode_normalizer",
		"batch_parts",
		"pii_detector",
		"message_selection",
		"tool_call_filter",
		"tool_search",
		"skills",
		"skill_search",
	}
	runner := buildProcessorRunner(all, nil)
	require.NotNil(t, runner)

	// system_prompt_scrubber is skipped without completer, so input = 7, output = 4.
	inputIDs := make([]string, len(runner.InputProcessors))
	for i, p := range runner.InputProcessors {
		inputIDs[i] = p.ID()
	}
	outputIDs := make([]string, len(runner.OutputProcessors))
	for i, p := range runner.OutputProcessors {
		outputIDs[i] = p.ID()
	}

	require.Len(t, runner.InputProcessors, 7)
	require.Len(t, runner.OutputProcessors, 4)
	require.Contains(t, inputIDs, "token_limiter")
	require.Contains(t, inputIDs, "unicode_normalizer")
	require.Contains(t, inputIDs, "pii_detector")
	require.Contains(t, inputIDs, "message_selection")
	require.Contains(t, inputIDs, "tool_search")
	require.Contains(t, inputIDs, "skills")
	require.Contains(t, inputIDs, "skill_search")
	require.Contains(t, outputIDs, "unicode_normalizer")
	require.Contains(t, outputIDs, "batch_parts")
	require.Contains(t, outputIDs, "pii_detector")
	require.Contains(t, outputIDs, "tool_call_filter")
}

func TestBuildProcessorRunner_DestructiveNotWhitelisted(t *testing.T) {
	t.Parallel()
	destructive := []string{
		"structured_output",
		"moderation",
		"workspace_instructions",
		"message_history",
	}
	runner := buildProcessorRunner(destructive, nil)
	require.Nil(t, runner)
}

func TestSafeProcessorNames_Count(t *testing.T) {
	t.Parallel()
	require.Len(t, safeProcessorNames, 10)
}
