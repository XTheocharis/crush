package lcm

import (
	"context"
	"database/sql"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func seedLineageData(t *testing.T, q *db.Queries, rawDB *sql.DB) {
	t.Helper()
	ctx := context.Background()

	createTestSession(t, q, "sess_lin")

	insertSummary := func(id, kind, content string, tokens int64) {
		_, err := rawDB.ExecContext(ctx,
			`INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids) VALUES (?, ?, ?, ?, ?, '[]')`,
			id, "sess_lin", kind, content, tokens)
		require.NoError(t, err)
	}
	insertParent := func(child, parent string, ord int) {
		require.NoError(t, q.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
			SummaryID: child, ParentSummaryID: parent, Ord: int64(ord),
		}))
	}

	// Build a DAG:
	//
	//   leaf_a  leaf_b  leaf_c
	//     \      /        |
	//     cond_1       cond_2
	//        \          /
	//         cond_top
	//
	insertSummary("lin_leaf_a", KindLeaf, "leaf A content", 10)
	insertSummary("lin_leaf_b", KindLeaf, "leaf B content", 15)
	insertSummary("lin_leaf_c", KindLeaf, "leaf C content", 20)
	insertSummary("lin_cond_1", KindCondensed, "condensed 1", 8)
	insertSummary("lin_cond_2", KindCondensed, "condensed 2", 6)
	insertSummary("lin_cond_top", KindCondensed, "top condensation", 4)

	insertParent("lin_cond_1", "lin_leaf_a", 0)
	insertParent("lin_cond_1", "lin_leaf_b", 1)
	insertParent("lin_cond_2", "lin_leaf_c", 0)
	insertParent("lin_cond_top", "lin_cond_1", 0)
	insertParent("lin_cond_top", "lin_cond_2", 1)
}

func TestQueryByLineage(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedLineageData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("ancestors_from_top", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_cond_top", LineageAncestors, 10)
		require.NoError(t, err)

		ids := nodeIDs(nodes)
		require.Contains(t, ids, "lin_cond_top")
		require.Contains(t, ids, "lin_cond_1")
		require.Contains(t, ids, "lin_cond_2")
		require.Contains(t, ids, "lin_leaf_a")
		require.Contains(t, ids, "lin_leaf_b")
		require.Contains(t, ids, "lin_leaf_c")

		root := findNode(nodes, "lin_cond_top")
		require.Equal(t, 0, root.Depth)
		require.Empty(t, root.ParentID)
	})

	t.Run("descendants_from_leaf_a", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_leaf_a", LineageDescendants, 10)
		require.NoError(t, err)

		ids := nodeIDs(nodes)
		require.Contains(t, ids, "lin_leaf_a")
		require.Contains(t, ids, "lin_cond_1")
		require.Contains(t, ids, "lin_cond_top")
		require.NotContains(t, ids, "lin_leaf_b")
		require.NotContains(t, ids, "lin_leaf_c")
		require.NotContains(t, ids, "lin_cond_2")
	})

	t.Run("descendants_from_leaf_c", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_leaf_c", LineageDescendants, 10)
		require.NoError(t, err)

		ids := nodeIDs(nodes)
		require.Contains(t, ids, "lin_leaf_c")
		require.Contains(t, ids, "lin_cond_2")
		require.Contains(t, ids, "lin_cond_top")
		require.NotContains(t, ids, "lin_cond_1")
	})

	t.Run("both_directions_from_cond_1", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_cond_1", LineageBoth, 10)
		require.NoError(t, err)

		ids := nodeIDs(nodes)
		// Ancestors: cond_1 -> leaf_a, leaf_b
		require.Contains(t, ids, "lin_cond_1")
		require.Contains(t, ids, "lin_leaf_a")
		require.Contains(t, ids, "lin_leaf_b")
		// Descendants: cond_1 -> cond_top
		require.Contains(t, ids, "lin_cond_top")
		// cond_2 is NOT reachable from cond_1 in either direction.
		require.NotContains(t, ids, "lin_cond_2")
		require.NotContains(t, ids, "lin_leaf_c")
	})

	t.Run("max_depth_1_ancestors", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_cond_top", LineageAncestors, 1)
		require.NoError(t, err)

		ids := nodeIDs(nodes)
		require.Contains(t, ids, "lin_cond_top")
		require.Contains(t, ids, "lin_cond_1")
		require.Contains(t, ids, "lin_cond_2")
		// Leaves are at depth 2, so excluded by maxDepth=1.
		require.NotContains(t, ids, "lin_leaf_a")
		require.NotContains(t, ids, "lin_leaf_b")
		require.NotContains(t, ids, "lin_leaf_c")
	})

	t.Run("nonexistent_summary", func(t *testing.T) {
		t.Parallel()

		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_nonexistent", LineageAncestors, 10)
		require.NoError(t, err)
		require.Empty(t, nodes)
	})

	t.Run("wrong_session", func(t *testing.T) {
		t.Parallel()

		createTestSession(t, q, "sess_other")
		nodes, err := store.QueryByLineage(t.Context(), "sess_other", "lin_cond_top", LineageAncestors, 10)
		require.NoError(t, err)
		require.Empty(t, nodes)
	})

	t.Run("default_max_depth", func(t *testing.T) {
		t.Parallel()

		// maxDepth=0 should default to 10.
		nodes, err := store.QueryByLineage(t.Context(), "sess_lin", "lin_cond_top", LineageAncestors, 0)
		require.NoError(t, err)
		require.Len(t, nodes, 6)
	})
}

func TestLineageFormatted(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedLineageData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("ancestors_formatted", func(t *testing.T) {
		t.Parallel()

		result, err := store.Lineage(t.Context(), "sess_lin", "lin_cond_top", LineageAncestors, 10)
		require.NoError(t, err)
		require.Contains(t, result, "lin_cond_top")
		require.Contains(t, result, "direction=ancestors")
	})

	t.Run("no_lineage", func(t *testing.T) {
		t.Parallel()

		result, err := store.Lineage(t.Context(), "sess_lin", "lin_nonexistent", LineageBoth, 10)
		require.NoError(t, err)
		require.Contains(t, result, "No lineage found")
	})
}

func nodeIDs(nodes []LineageNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.SummaryID
	}
	return ids
}

func findNode(nodes []LineageNode, id string) LineageNode {
	for _, n := range nodes {
		if n.SummaryID == id {
			return n
		}
	}
	return LineageNode{}
}
