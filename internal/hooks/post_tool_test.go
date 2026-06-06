package hooks

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/shell"
	"github.com/stretchr/testify/require"
)

func TestPostToolUse_UpdatedInputDenyHaltAllowAggregation(t *testing.T) {
	t.Parallel()

	t.Run("deny from hook is non-blocking for PostToolUse", func(t *testing.T) {
		t.Parallel()
		// PostToolUse hooks exit 2 does not produce DecisionDeny — the tool
		// already ran. Verify the aggregate reflects non-blocking behavior.
		hookCfg := config.HookConfig{
			Command: `echo "still blocked" >&2; exit 2`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
		require.NoError(t, err)
		// PostToolUse exit code 2 is non-blocking: decision stays none and
		// output is preserved (not rewritten).
		require.NotEqual(t, DecisionDeny, result.Decision)
		require.Empty(t, result.UpdatedOutput)
	})

	t.Run("halt from hook is non-blocking for PostToolUse", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo "halt attempt" >&2; exit 49`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
		require.NoError(t, err)
		// PostToolUse exit code 49 is non-blocking: no halt flag.
		require.False(t, result.Halt)
		require.Empty(t, result.UpdatedOutput)
	})

	t.Run("allow with output rewriting via modified_output", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo '{"modified_output":"sanitized"}'`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
		require.NoError(t, err)
		require.Equal(t, "sanitized", result.UpdatedOutput)
	})

	t.Run("multiple hooks: last writer wins for output", func(t *testing.T) {
		t.Parallel()
		hooks := []config.HookConfig{
			{Command: `echo '{"modified_output":"first"}'`},
			{Command: `echo '{"modified_output":"second"}'`},
			{Command: `echo '{"decision":"allow"}'`},
		}
		r := NewRunner(hooks, t.TempDir(), t.TempDir())
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original", 50)
		require.NoError(t, err)
		require.Equal(t, "second", result.UpdatedOutput)
	})
}

func TestPostToolUse_NonZeroExitPreservesOriginalOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
	}{
		{"exit 1", `exit 1`},
		{"exit 2", `echo "blocked" >&2; exit 2`},
		{"exit 49", `echo "halt" >&2; exit 49`},
		{"segv-like exit 139", `sh -c 'kill -11 $$'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hookCfg := config.HookConfig{
				Command: tt.command,
			}
			r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
			result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
			require.NoError(t, err)
			// Non-zero exit must not rewrite output.
			require.Empty(t, result.UpdatedOutput, "non-zero exit should not rewrite output")
		})
	}
}

func TestPostToolUse_TimeoutBehavior(t *testing.T) {
	t.Parallel()

	t.Run("hook timeout returns no rewrite", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `sleep 10`,
			Timeout: 1,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		start := time.Now()
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original output", 100)
		elapsed := time.Since(start)
		require.NoError(t, err)
		require.Empty(t, result.UpdatedOutput)
		require.Less(t, elapsed, 5*time.Second, "timeout should return promptly")
	})
}

func TestPostToolUse_AbandonRaceSafety(t *testing.T) {
	origRunShell := runShell
	t.Cleanup(func() { runShell = origRunShell })

	var wg sync.WaitGroup
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		wg.Wait()
	})

	runShell = func(_ context.Context, opts shell.RunOptions) error {
		wg.Add(1)
		defer wg.Done()
		_, _ = io.WriteString(opts.Stdout, "before\n")
		select {
		case <-time.After(5 * time.Second):
		case <-release:
		}
		_, _ = io.WriteString(opts.Stdout, "after\n")
		return nil
	}

	hookCfg := config.HookConfig{
		Command: "# irrelevant; runShell is stubbed",
		Timeout: 1,
	}
	r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())

	start := time.Now()
	result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original", 100)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Empty(t, result.UpdatedOutput, "abandoned hook must not produce output")
	require.Less(t, elapsed, 3500*time.Millisecond)
}

func TestPostToolUse_EnvAndPayloadMetadata(t *testing.T) {
	t.Parallel()

	t.Run("PostPayload contains all required fields", func(t *testing.T) {
		t.Parallel()
		payload := BuildPostPayload(EventPostToolUse, "sess-42", "/work", "bash", `{"command":"ls"}`, "output text", 500)
		s := string(payload)
		require.Contains(t, s, `"event":"PostToolUse"`)
		require.Contains(t, s, `"session_id":"sess-42"`)
		require.Contains(t, s, `"cwd":"/work"`)
		require.Contains(t, s, `"tool_name":"bash"`)
		require.Contains(t, s, `"tool_output":"output text"`)
		require.Contains(t, s, `"duration_ms":500`)
		require.Contains(t, s, `"tool_input":{"command":"ls"}`)
	})

	t.Run("PostEnv contains all required variables", func(t *testing.T) {
		t.Parallel()
		env := BuildPostEnv(EventPostToolUse, "bash", "sess-42", "/work", "/project", `{"command":"ls","file_path":"/tmp/f.txt"}`, "output", 250)
		envMap := make(map[string]string)
		for _, e := range env {
			parts := splitFirst(e, "=")
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		require.Equal(t, EventPostToolUse, envMap["CRUSH_EVENT"])
		require.Equal(t, "bash", envMap["CRUSH_TOOL_NAME"])
		require.Equal(t, "sess-42", envMap["CRUSH_SESSION_ID"])
		require.Equal(t, "/work", envMap["CRUSH_CWD"])
		require.Equal(t, "/project", envMap["CRUSH_PROJECT_DIR"])
		require.Equal(t, "output", envMap["CRUSH_TOOL_OUTPUT"])
		require.Equal(t, "250", envMap["CRUSH_TOOL_DURATION_MS"])
		require.Equal(t, "ls", envMap["CRUSH_TOOL_INPUT_COMMAND"])
		require.Equal(t, "/tmp/f.txt", envMap["CRUSH_TOOL_INPUT_FILE_PATH"])
	})

	t.Run("hook receives tool name via env", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `printf '{"modified_output":"saw:%s"}' "$CRUSH_TOOL_NAME"`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "view", `{}`, "original", 10)
		require.NoError(t, err)
		require.Equal(t, "saw:view", result.UpdatedOutput)
	})
}

func TestPostToolUse_MatcherFiltering(t *testing.T) {
	t.Parallel()

	t.Run("matcher filters PostToolUse hooks", func(t *testing.T) {
		t.Parallel()
		hooks := []config.HookConfig{
			{Command: `echo '{"modified_output":"bash-rewrite"}'`, Matcher: "^bash$"},
		}
		r := NewRunner(hooks, t.TempDir(), t.TempDir())

		// Matches.
		result, err := r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "bash", `{}`, "original", 10)
		require.NoError(t, err)
		require.Equal(t, "bash-rewrite", result.UpdatedOutput)

		// Does not match.
		result, err = r.RunPostToolUse(context.Background(), EventPostToolUse, "sess", "edit", `{}`, "original", 10)
		require.NoError(t, err)
		require.Empty(t, result.UpdatedOutput)
	})
}
