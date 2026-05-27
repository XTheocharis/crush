package hooks

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRunPostToolUse_OutputRewriting(t *testing.T) {
	t.Parallel()
	hookCfg := config.HookConfig{
		Command: `echo '{"modified_output":"sanitized output"}'`,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
	require.NoError(t, err)
	require.Equal(t, "sanitized output", result.UpdatedOutput)
}

func TestRunPostToolUse_NoModification(t *testing.T) {
	t.Parallel()
	hookCfg := config.HookConfig{
		Command: `echo ""`,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
	require.NoError(t, err)
	require.Empty(t, result.UpdatedOutput)
}

func TestRunPostToolUse_MultipleHooks(t *testing.T) {
	t.Parallel()
	hooks := []config.HookConfig{
		{Command: `echo '{"modified_output":"first rewrite"}'`},
		{Command: `echo '{"modified_output":"second rewrite"}'`},
	}
	r := NewRunner(hooks, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original", 50)
	require.NoError(t, err)
	require.Equal(t, "second rewrite", result.UpdatedOutput)
}

func TestRunPostToolUse_HookError(t *testing.T) {
	t.Parallel()
	hookCfg := config.HookConfig{
		Command: `exit 1`,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
	require.NoError(t, err)
	require.Empty(t, result.UpdatedOutput)
}

func TestRunPostToolUse_ExitCode2NonBlocking(t *testing.T) {
	t.Parallel()
	hookCfg := config.HookConfig{
		Command: `echo "blocked" >&2; exit 2`,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
	require.NoError(t, err)
	require.Empty(t, result.UpdatedOutput)
	require.NotEqual(t, DecisionDeny, result.Decision)
}

func TestRunPostToolUse_ExitCode49NonBlocking(t *testing.T) {
	t.Parallel()
	hookCfg := config.HookConfig{
		Command: `echo "halt" >&2; exit 49`,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
	require.NoError(t, err)
	require.Empty(t, result.UpdatedOutput)
	require.False(t, result.Halt)
}

func TestRunPostToolUse_NoHooks(t *testing.T) {
	t.Parallel()
	r := NewRunner(nil, t.TempDir(), t.TempDir())
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "output", 10)
	require.NoError(t, err)
	require.Equal(t, DecisionNone, result.Decision)
	require.Empty(t, result.UpdatedOutput)
}

func TestParsePostStdout(t *testing.T) {
	t.Parallel()

	t.Run("JSON with modified_output", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout(`{"modified_output":"replacement text"}`)
		require.Equal(t, "replacement text", result)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout("")
		require.Empty(t, result)
	})

	t.Run("whitespace only", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout("   \n\t  ")
		require.Empty(t, result)
	})

	t.Run("plain text as fallback", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout("plain text output")
		require.Equal(t, "plain text output", result)
	})

	t.Run("JSON without modified_output", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout(`{"decision":"allow"}`)
		require.Empty(t, result)
	})

	t.Run("JSON with empty modified_output", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout(`{"modified_output":""}`)
		require.Empty(t, result)
	})

	t.Run("invalid JSON treated as plain text", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout(`not json at all`)
		require.Equal(t, "not json at all", result)
	})

	t.Run("JSON with modified_output and extra fields", func(t *testing.T) {
		t.Parallel()
		result := parsePostStdout(`{"modified_output":"sanitized","extra":"ignored"}`)
		require.Equal(t, "sanitized", result)
	})
}

func TestBuildPostEnv(t *testing.T) {
	t.Parallel()
	env := BuildPostEnv(EventPostToolUse, "bash", "sess-1", "/work", "/project", `{"command":"ls"}`, "tool output here", 250)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := splitFirst(e, "=")
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	require.Equal(t, EventPostToolUse, envMap["CRUSH_EVENT"])
	require.Equal(t, "bash", envMap["CRUSH_TOOL_NAME"])
	require.Equal(t, "tool output here", envMap["CRUSH_TOOL_OUTPUT"])
	require.Equal(t, "250", envMap["CRUSH_TOOL_DURATION_MS"])
}

func TestBuildPostPayload(t *testing.T) {
	t.Parallel()
	payload := BuildPostPayload(EventPostToolUse, "sess-1", "/work", "bash", `{"command":"ls"}`, "output text", 500)
	s := string(payload)
	require.Contains(t, s, `"event":"`+EventPostToolUse+`"`)
	require.Contains(t, s, `"tool_name":"bash"`)
	require.Contains(t, s, `"tool_output":"output text"`)
	require.Contains(t, s, `"duration_ms":500`)
}

func TestAggregationUpdatedOutput(t *testing.T) {
	t.Parallel()

	t.Run("last-writer-wins", func(t *testing.T) {
		t.Parallel()
		agg := aggregate([]HookResult{
			{UpdatedOutput: "first"},
			{UpdatedOutput: "second"},
		}, `{}`)
		require.Equal(t, "second", agg.UpdatedOutput)
	})

	t.Run("no UpdatedOutput", func(t *testing.T) {
		t.Parallel()
		agg := aggregate([]HookResult{
			{Decision: DecisionAllow},
		}, `{}`)
		require.Empty(t, agg.UpdatedOutput)
	})

	t.Run("single UpdatedOutput", func(t *testing.T) {
		t.Parallel()
		agg := aggregate([]HookResult{
			{UpdatedOutput: "rewritten"},
		}, `{}`)
		require.Equal(t, "rewritten", agg.UpdatedOutput)
	})

	t.Run("mixed: some hooks provide output, some don't", func(t *testing.T) {
		t.Parallel()
		agg := aggregate([]HookResult{
			{Decision: DecisionAllow},
			{UpdatedOutput: "from second"},
			{Decision: DecisionNone},
		}, `{}`)
		require.Equal(t, "from second", agg.UpdatedOutput)
	})
}

func TestValidateHooksNormalizesPostToolUseEventNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"canonical", "PostToolUse"},
		{"lowercase", "posttooluse"},
		{"snake_case", "post_tool_use"},
		{"upper_snake", "POST_TOOL_USE"},
		{"mixed_case", "postToolUse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				Hooks: map[string][]config.HookConfig{
					tt.input: {
						{Command: "true"},
					},
				},
			}
			require.NoError(t, cfg.ValidateHooks())
			require.Len(t, cfg.Hooks[EventPostToolUse], 1)
		})
	}
}
