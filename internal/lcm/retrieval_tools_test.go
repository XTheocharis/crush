package lcm

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestRetrievalTool_Creation(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)

	tools := []struct {
		name string
		tool fantasy.AgentTool
		want string
	}{
		{"bindle", newBindleTool(store), "lcm_bindle"},
		{"ancestry", newAncestryTool(store), "lcm_ancestry"},
		{"dolt", newDoltTool(store), "lcm_dolt"},
		{"archive", newArchiveTool(store), "lcm_archive"},
		{"sprig", newSprigTool(store), "lcm_sprig"},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			info := tc.tool.Info()
			require.Equal(t, tc.want, info.Name)
			require.NotEmpty(t, info.Description)
		})
	}
}

func TestRetrievalTool_MissingParams(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	store := newStore(q, rawDB)
	ctx := context.Background()

	t.Run("bindle_missing_summary_id", func(t *testing.T) {
		t.Parallel()

		tool := newBindleTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_bindle", Input: `{}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "summary_id is required")
	})

	t.Run("ancestry_missing_summary_id", func(t *testing.T) {
		t.Parallel()

		tool := newAncestryTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_ancestry", Input: `{}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "summary_id is required")
	})

	t.Run("dolt_missing_session_id", func(t *testing.T) {
		t.Parallel()

		tool := newDoltTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_dolt", Input: `{}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "session_id is required")
	})

	t.Run("archive_missing_session_id", func(t *testing.T) {
		t.Parallel()

		tool := newArchiveTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_archive", Input: `{"pattern":"test"}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "session_id is required")
	})

	t.Run("archive_missing_pattern", func(t *testing.T) {
		t.Parallel()

		tool := newArchiveTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_archive", Input: `{"session_id":"s1"}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "pattern is required")
	})

	t.Run("sprig_missing_session_id", func(t *testing.T) {
		t.Parallel()

		tool := newSprigTool(store)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_sprig", Input: `{}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "session_id is required")
	})
}

func TestRetrievalTool_StoreCallthrough(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)
	ctx := context.Background()

	t.Run("bindle_found", func(t *testing.T) {
		t.Parallel()

		tool := newBindleTool(store)
		input, _ := json.Marshal(bindleParams{SummaryID: "sum_alpha_1"})
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_bindle", Input: string(input)})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.Contains(t, resp.Content, "Summary ID: sum_alpha_1")
		require.Contains(t, resp.Content, "JWT tokens")
	})

	t.Run("bindle_not_found", func(t *testing.T) {
		t.Parallel()

		tool := newBindleTool(store)
		input, _ := json.Marshal(bindleParams{SummaryID: "nonexistent"})
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_bindle", Input: string(input)})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.Contains(t, resp.Content, "Summary not found")
	})

	t.Run("ancestry_found", func(t *testing.T) {
		t.Parallel()

		tool := newAncestryTool(store)
		input, _ := json.Marshal(ancestryParams{SummaryID: "sum_alpha_cond"})
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_ancestry", Input: string(input)})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.Contains(t, resp.Content, "Ancestry chain")
	})

	t.Run("dolt_found", func(t *testing.T) {
		t.Parallel()

		tool := newDoltTool(store)
		input, _ := json.Marshal(doltParams{SessionID: "sess_alpha"})
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_dolt", Input: string(input)})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.Contains(t, resp.Content, "Summaries for session sess_alpha")
	})

	t.Run("sprig_found", func(t *testing.T) {
		t.Parallel()

		tool := newSprigTool(store)
		input, _ := json.Marshal(sprigParams{SessionID: "sess_alpha"})
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "1", Name: "lcm_sprig", Input: string(input)})
		require.NoError(t, err)
		require.False(t, resp.IsError)
		require.Contains(t, resp.Content, "Latest summary for session sess_alpha")
	})
}
