package lcm

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/agent/tools/types"
)

func TestCompactTool_Creation(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)
	tool := newCompactTool(mgr)

	info := tool.Info()
	require.Equal(t, "lcm_compact", info.Name)
	require.NotEmpty(t, info.Description)
}

func TestCompactTool_NilManager(t *testing.T) {
	t.Parallel()

	tool := newCompactTool(nil)
	ctx := context.Background()

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "1",
		Name:  "lcm_compact",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "LCM manager is not available")
}

func TestCompactTool_MissingSessionID(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)
	tool := newCompactTool(mgr)
	ctx := context.Background()

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "1",
		Name:  "lcm_compact",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "Session ID not found in context")
}

func TestCompactTool_InvalidPressure(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)
	tool := newCompactTool(mgr)

	ctx := context.WithValue(context.Background(), types.SessionIDContextKey, "test-session")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "1",
		Name:  "lcm_compact",
		Input: `{"pressure": "urgent"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "Invalid pressure")
}

func TestCompactTool_ValidPressures(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)

	createTestSession(t, q, "session-press")

	for _, pressure := range []string{"low", "medium", "high", ""} {
		t.Run("pressure_"+pressure, func(t *testing.T) {
			t.Parallel()

			tool := newCompactTool(mgr)
			ctx := context.WithValue(context.Background(), types.SessionIDContextKey, "session-press")
			input := "{}"
			if pressure != "" {
				input = `{"pressure": "` + pressure + `"}`
			}
			resp, err := tool.Run(ctx, fantasy.ToolCall{
				ID:    "1",
				Name:  "lcm_compact",
				Input: input,
			})
			require.NoError(t, err)
			require.False(t, resp.IsError, "unexpected error response: %s", resp.Content)
			require.Contains(t, resp.Content, "Compaction completed")
		})
	}
}

func TestCompactTool_WithTargetTokens(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)

	createTestSession(t, q, "session-target")

	tool := newCompactTool(mgr)
	ctx := context.WithValue(context.Background(), types.SessionIDContextKey, "session-target")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "1",
		Name:  "lcm_compact",
		Input: `{"target_tokens": 5000}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error response: %s", resp.Content)
	require.Contains(t, resp.Content, "Compaction completed")
	require.Contains(t, resp.Content, "Tokens before")
	require.Contains(t, resp.Content, "Tokens after")
}

func TestCompactTool_OutputFormat(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	mgr := NewManager(q, rawDB)

	createTestSession(t, q, "session-fmt")

	tool := newCompactTool(mgr)
	ctx := context.WithValue(context.Background(), types.SessionIDContextKey, "session-fmt")
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "1",
		Name:  "lcm_compact",
		Input: `{}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Tokens before:")
	require.Contains(t, resp.Content, "Tokens after:")
	require.Contains(t, resp.Content, "Soft threshold:")
	require.Contains(t, resp.Content, "Hard limit:")
	require.Contains(t, resp.Content, "Status:")
}
