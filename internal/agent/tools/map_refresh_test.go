package tools

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestMapRefreshToolCreation(t *testing.T) {
	t.Parallel()

	tool := NewMapRefreshTool(nil, nil)
	require.NotNil(t, tool)
	require.Equal(t, MapRefreshToolName, tool.Info().Name)
}

func TestMapRefreshToolRequiresSession(t *testing.T) {
	t.Parallel()

	tool := NewMapRefreshTool(func(ctx context.Context, sessionID string) error {
		return nil
	}, func(ctx context.Context, sessionID string) error {
		return nil
	})

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "test", Input: `{"sync":true}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "session ID is required")
}

func TestMapRefreshToolSyncPath(t *testing.T) {
	t.Parallel()

	called := false
	tool := NewMapRefreshTool(func(ctx context.Context, sessionID string) error {
		called = true
		require.Equal(t, "sess-1", sessionID)
		return nil
	}, func(ctx context.Context, sessionID string) error {
		return nil
	})

	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess-1")
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "test", Input: `{"sync":true}`})
	require.NoError(t, err)
	require.True(t, called)
	require.Contains(t, resp.Content, "refreshed")
}

func TestMapRefreshToolAsyncPath(t *testing.T) {
	t.Parallel()

	called := false
	tool := NewMapRefreshTool(func(ctx context.Context, sessionID string) error {
		return nil
	}, func(ctx context.Context, sessionID string) error {
		called = true
		require.Equal(t, "sess-2", sessionID)
		return nil
	})

	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess-2")
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "test", Input: `{}`})
	require.NoError(t, err)
	require.True(t, called)
	require.Contains(t, resp.Content, "scheduled")
}

func TestMapRefreshToolUnavailable(t *testing.T) {
	t.Parallel()

	tool := NewMapRefreshTool(nil, nil)
	ctx := context.WithValue(t.Context(), SessionIDContextKey, "sess-3")

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "test", Input: `{"sync":true}`})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "not available")
}
