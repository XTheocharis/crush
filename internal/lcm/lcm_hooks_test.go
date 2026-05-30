package lcm

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/stretchr/testify/require"
)

func newHookRunner(t *testing.T, event, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			event: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[event], t.TempDir(), t.TempDir())
}

func TestLCMHooks_PreCompactFires(t *testing.T) {
	t.Parallel()
	queries, db := setupTestDB(t)

	sessionID := "sess-hooks-pre"
	createTestSession(t, queries, sessionID)
	mgr := NewManager(queries, db)
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	// Add messages to push over soft threshold.
	for i := range 20 {
		createTestMessage(t, queries, sessionID, "msg-pre-"+string(rune('a'+i)), "user",
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit. "+time.Now().String())
	}
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	// Hook that writes a marker file to prove it fired.
	markerDir := t.TempDir()
	runner := newHookRunner(t, hooks.EventPreCompact,
		`echo '{"decision":"allow"}' > /dev/stdout && touch `+markerDir+"/pre-fired")
	mgr.SetHookRunners(runner, nil)

	err := mgr.Compact(context.Background(), sessionID)
	require.NoError(t, err)

	// The marker file proves the PreCompact hook fired.
	require.FileExists(t, markerDir+"/pre-fired")
}

func TestLCMHooks_PostCompactFires(t *testing.T) {
	t.Parallel()
	queries, db := setupTestDB(t)

	sessionID := "sess-hooks-post"
	createTestSession(t, queries, sessionID)
	mgr := NewManager(queries, db)
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	for i := range 20 {
		createTestMessage(t, queries, sessionID, "msg-post-"+string(rune('a'+i)), "user",
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit. "+time.Now().String())
	}
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	markerDir := t.TempDir()
	runner := newHookRunner(t, hooks.EventPostCompact,
		`touch `+markerDir+"/post-fired")
	mgr.SetHookRunners(nil, runner)

	err := mgr.Compact(context.Background(), sessionID)
	require.NoError(t, err)

	require.FileExists(t, markerDir+"/post-fired")
}

func TestLCMHooks_PreCompactDeny_SkipsCompaction(t *testing.T) {
	t.Parallel()
	queries, db := setupTestDB(t)

	sessionID := "sess-hooks-deny"
	createTestSession(t, queries, sessionID)
	mgr := NewManager(queries, db)
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	// Add messages so there's content.
	for i := range 20 {
		createTestMessage(t, queries, sessionID, "msg-deny-"+string(rune('a'+i)), "user",
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit. "+time.Now().String())
	}
	require.NoError(t, mgr.InitSession(context.Background(), sessionID))

	countBefore, err := mgr.GetContextTokenCount(context.Background(), sessionID)
	require.NoError(t, err)
	require.Greater(t, countBefore, int64(0))

	// Hook that denies compaction.
	runner := newHookRunner(t, hooks.EventPreCompact,
		`echo "compaction blocked by policy" >&2; exit 2`)
	mgr.SetHookRunners(runner, nil)

	err = mgr.Compact(context.Background(), sessionID)
	require.NoError(t, err)

	// Token count should be unchanged — compaction was skipped.
	countAfter, err := mgr.GetContextTokenCount(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, countBefore, countAfter, "compaction should be skipped when hook denies")
}

func TestLCMHooks_PreCompactForceFull(t *testing.T) {
	t.Parallel()

	// Test the CompactHookDecision logic for ForceFull.
	runner := newHookRunner(t, hooks.EventPreCompact,
		`echo '{"decision":"allow","updated_input":{"force_full":true}}'`)

	input := CompactHookInput{
		SessionID:     "test",
		TokenCount:    50000,
		SoftThreshold: 40000,
		HardLimit:     60000,
		OverSoft:      true,
	}

	decision := runPreCompactHooks(context.Background(), runner, "test", input)
	require.False(t, decision.Skip, "hook should not deny")
	require.True(t, decision.ForceFull, "hook should request force-full")
}

func TestLCMHooks_NilRunner_Proceeds(t *testing.T) {
	t.Parallel()

	input := CompactHookInput{SessionID: "test", TokenCount: 50000}
	decision := runPreCompactHooks(context.Background(), nil, "test", input)
	require.False(t, decision.Skip)
	require.False(t, decision.ForceFull)
}

func TestLCMHooks_PreCompactAllow_Proceeds(t *testing.T) {
	t.Parallel()

	runner := newHookRunner(t, hooks.EventPreCompact,
		`echo '{"decision":"allow"}'`)

	input := CompactHookInput{
		SessionID:     "test",
		TokenCount:    50000,
		SoftThreshold: 40000,
	}
	decision := runPreCompactHooks(context.Background(), runner, "test", input)
	require.False(t, decision.Skip)
	require.False(t, decision.ForceFull)
}

func TestLCMHooks_BuildPreCompactInput(t *testing.T) {
	t.Parallel()
	t.Run("over soft threshold", func(t *testing.T) {
		t.Parallel()
		input := buildPreCompactInput("s1", 50000, Budget{
			SoftThreshold: 40000,
			HardLimit:     60000,
		}, false)
		require.Equal(t, "s1", input.SessionID)
		require.Equal(t, int64(50000), input.TokenCount)
		require.True(t, input.OverSoft)
		require.False(t, input.OverHard)
		require.False(t, input.Blocking)
	})
	t.Run("over hard limit", func(t *testing.T) {
		t.Parallel()
		input := buildPreCompactInput("s2", 70000, Budget{
			SoftThreshold: 40000,
			HardLimit:     60000,
		}, true)
		require.True(t, input.OverSoft)
		require.True(t, input.OverHard)
		require.True(t, input.Blocking)
	})
}

func TestLCMHooks_BuildPostCompactOutput(t *testing.T) {
	t.Parallel()
	start := time.Now()
	result := CompactionResult{Rounds: 2, ActionTaken: true, TokenCount: 30000}
	output := buildPostCompactOutput("s1", result, false, start)
	require.Equal(t, "s1", output.SessionID)
	require.True(t, output.Success)
	require.Equal(t, 2, output.Rounds)
	require.Equal(t, int64(30000), output.TokenCountAfter)
	require.False(t, output.Blocking)
	require.GreaterOrEqual(t, output.DurationMs, int64(0))
}

func TestPostCompactHookResultsProcessed(t *testing.T) {
	t.Parallel()

	output := CompactHookOutput{
		SessionID:       "s1",
		Success:         true,
		Rounds:          2,
		TokenCountAfter: 30000,
		Blocking:        false,
		DurationMs:      150,
	}

	t.Run("nil runner returns empty decision", func(t *testing.T) {
		t.Parallel()
		decision := runPostCompactHooks(context.Background(), nil, "s1", output)
		require.False(t, decision.Halt)
		require.Empty(t, decision.Reason)
	})

	t.Run("allow decision returns clean decision", func(t *testing.T) {
		t.Parallel()
		runner := newHookRunner(t, hooks.EventPostCompact,
			`echo '{"decision":"allow"}'`)
		decision := runPostCompactHooks(context.Background(), runner, "s1", output)
		require.False(t, decision.Halt)
		require.Empty(t, decision.Reason)
	})

	t.Run("deny decision returns reason", func(t *testing.T) {
		t.Parallel()
		runner := newHookRunner(t, hooks.EventPostCompact,
			`echo '{"decision":"deny","reason":"compaction exceeded policy threshold"}'`)
		decision := runPostCompactHooks(context.Background(), runner, "s1", output)
		require.False(t, decision.Halt)
		require.Equal(t, "compaction exceeded policy threshold", decision.Reason)
	})

	t.Run("halt decision is surfaced", func(t *testing.T) {
		t.Parallel()
		runner := newHookRunner(t, hooks.EventPostCompact,
			`echo '{"decision":"deny"}'; echo "critical post-condition failed" >&2; exit 49`)
		decision := runPostCompactHooks(context.Background(), runner, "s1", output)
		require.True(t, decision.Halt)
		require.Equal(t, "critical post-condition failed", decision.Reason)
	})
}
