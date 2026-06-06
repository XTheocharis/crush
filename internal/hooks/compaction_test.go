package hooks

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPreCompactHook_FiresWithCorrectPayload(t *testing.T) {
	t.Parallel()

	t.Run("runner executes PreCompact event hooks", func(t *testing.T) {
		t.Parallel()
		markerDir := t.TempDir()
		hookCfg := config.HookConfig{
			Command: `touch ` + markerDir + `/pre-compact-fired && echo '{"decision":"allow"}'`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess-compact-1", "lcm_compact", `{"session_id":"s1","token_count":50000}`)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, result.Decision)
		require.FileExists(t, markerDir+"/pre-compact-fired")
	})

	t.Run("PreCompact hook receives correct env vars", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `printf '{"decision":"allow","context":"%s %s"}' "$CRUSH_EVENT" "$CRUSH_TOOL_NAME"`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess-compact-1", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, result.Decision)
		require.Equal(t, "PreCompact lcm_compact", result.Context)
	})

	t.Run("PreCompact hook receives session ID in env", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `printf '{"decision":"allow","context":"%s"}' "$CRUSH_SESSION_ID"`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "my-session-42", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, "my-session-42", result.Context)
	})
}

func TestPostCompactHook_FiresWithCorrectPayload(t *testing.T) {
	t.Parallel()

	t.Run("runner executes PostCompact event hooks", func(t *testing.T) {
		t.Parallel()
		markerDir := t.TempDir()
		hookCfg := config.HookConfig{
			Command: `touch ` + markerDir + `/post-compact-fired && echo '{"decision":"allow"}'`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "sess-compact-2", "lcm_compact", `{"session_id":"s2","success":true,"rounds":2}`)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, result.Decision)
		require.FileExists(t, markerDir+"/post-compact-fired")
	})

	t.Run("PostCompact hook receives correct env vars", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `printf '{"decision":"allow","context":"%s %s"}' "$CRUSH_EVENT" "$CRUSH_TOOL_NAME"`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "sess-compact-2", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, "PostCompact lcm_compact", result.Context)
	})

	t.Run("PostCompact hook receives session ID in env", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `printf '{"decision":"allow","context":"%s"}' "$CRUSH_SESSION_ID"`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "my-session-99", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, "my-session-99", result.Context)
	})
}

func TestCompactionHook_PayloadMetadata(t *testing.T) {
	t.Parallel()

	t.Run("PreCompact stdin payload contains event and tool_input", func(t *testing.T) {
		t.Parallel()
		payload := BuildPayload(EventPreCompact, "sess-1", "/work", "lcm_compact", `{"session_id":"s1","token_count":50000}`)
		s := string(payload)
		require.Contains(t, s, `"event":"PreCompact"`)
		require.Contains(t, s, `"session_id":"sess-1"`)
		require.Contains(t, s, `"tool_name":"lcm_compact"`)
		require.Contains(t, s, `"token_count":50000`)
	})

	t.Run("PostCompact stdin payload contains event and tool_input", func(t *testing.T) {
		t.Parallel()
		payload := BuildPayload(EventPostCompact, "sess-2", "/work", "lcm_compact", `{"session_id":"s2","success":true,"rounds":3}`)
		s := string(payload)
		require.Contains(t, s, `"event":"PostCompact"`)
		require.Contains(t, s, `"session_id":"sess-2"`)
		require.Contains(t, s, `"tool_name":"lcm_compact"`)
		require.Contains(t, s, `"success":true`)
		require.Contains(t, s, `"rounds":3`)
	})

	t.Run("PreCompact env includes CWD and PROJECT_DIR", func(t *testing.T) {
		t.Parallel()
		env := BuildEnv(EventPreCompact, "lcm_compact", "sess-1", "/work", "/project", `{}`)
		envMap := make(map[string]string)
		for _, e := range env {
			parts := splitFirst(e, "=")
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		require.Equal(t, EventPreCompact, envMap["CRUSH_EVENT"])
		require.Equal(t, "lcm_compact", envMap["CRUSH_TOOL_NAME"])
		require.Equal(t, "/work", envMap["CRUSH_CWD"])
		require.Equal(t, "/project", envMap["CRUSH_PROJECT_DIR"])
	})
}

func TestCompactionHook_DenyAndHaltBehavior(t *testing.T) {
	t.Parallel()

	t.Run("PreCompact deny blocks via exit code 2", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo "compaction denied by policy" >&2; exit 2`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess-deny", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionDeny, result.Decision)
		require.Equal(t, "compaction denied by policy", result.Reason)
	})

	t.Run("PreCompact halt via exit code 49", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo "critical: halt compaction" >&2; exit 49`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess-halt", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.True(t, result.Halt)
		require.Equal(t, DecisionDeny, result.Decision)
		require.Equal(t, "critical: halt compaction", result.Reason)
	})

	t.Run("PreCompact deny via JSON output", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo '{"decision":"deny","reason":"over token budget"}'`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess-json-deny", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionDeny, result.Decision)
		require.Equal(t, "over token budget", result.Reason)
	})

	t.Run("PostCompact deny via exit code 2", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo "post-compact check failed" >&2; exit 2`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "sess-post-deny", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionDeny, result.Decision)
		require.Equal(t, "post-compact check failed", result.Reason)
	})

	t.Run("PostCompact halt via exit code 49", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `echo "post-compact halt" >&2; exit 49`,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "sess-post-halt", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.True(t, result.Halt)
		require.Equal(t, DecisionDeny, result.Decision)
	})
}

func TestCompactionHook_TimeoutBehavior(t *testing.T) {
	t.Parallel()

	t.Run("PreCompact hook timeout returns none", func(t *testing.T) {
		t.Parallel()
		hookCfg := config.HookConfig{
			Command: `sleep 10`,
			Timeout: 1,
		}
		r := NewRunner([]config.HookConfig{hookCfg}, t.TempDir(), t.TempDir())
		start := time.Now()
		result, err := r.Run(context.Background(), EventPreCompact, "sess-timeout", "lcm_compact", `{}`)
		elapsed := time.Since(start)
		require.NoError(t, err)
		require.Equal(t, DecisionNone, result.Decision)
		require.Less(t, elapsed, 5*time.Second)
	})
}

func TestCompactionHook_NoMatchingHooks(t *testing.T) {
	t.Parallel()

	t.Run("empty runner returns none for PreCompact", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(nil, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPreCompact, "sess", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionNone, result.Decision)
	})

	t.Run("empty runner returns none for PostCompact", func(t *testing.T) {
		t.Parallel()
		r := NewRunner(nil, t.TempDir(), t.TempDir())
		result, err := r.Run(context.Background(), EventPostCompact, "sess", "lcm_compact", `{}`)
		require.NoError(t, err)
		require.Equal(t, DecisionNone, result.Decision)
	})
}

func TestCompactionHook_EventNameNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"canonical PreCompact", "PreCompact"},
		{"lowercase precompact", "precompact"},
		{"snake_case pre_compact", "pre_compact"},
		{"canonical PostCompact", "PostCompact"},
		{"lowercase postcompact", "postcompact"},
		{"snake_case post_compact", "post_compact"},
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
			target := EventPreCompact
			if tt.input == "PostCompact" || tt.input == "postcompact" || tt.input == "post_compact" {
				target = EventPostCompact
			}
			require.Len(t, cfg.Hooks[target], 1)
		})
	}
}
