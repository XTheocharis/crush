package lcm

import (
	"context"
	"database/sql"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func seedRetrievalData(t *testing.T, q *db.Queries, rawDB *sql.DB) {
	t.Helper()
	ctx := context.Background()

	createTestSession(t, q, "sess_alpha")
	createTestSession(t, q, "sess_beta")

	_, err := rawDB.ExecContext(ctx, `INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids, created_at) VALUES (?, ?, ?, ?, ?, ?, 100)`,
		"sum_alpha_1", "sess_alpha", KindLeaf, "Implemented user authentication with JWT tokens", 50, `["file_auth_1"]`)
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx, `INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids, created_at) VALUES (?, ?, ?, ?, ?, ?, 200)`,
		"sum_alpha_2", "sess_alpha", KindLeaf, "Added database migration for user table", 40, "[]")
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx, `INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids, created_at) VALUES (?, ?, ?, ?, ?, ?, 300)`,
		"sum_alpha_cond", "sess_alpha", KindCondensed, "Authentication system: JWT tokens, user table, migration", 30, "[]")
	require.NoError(t, err)

	require.NoError(t, q.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
		SummaryID:       "sum_alpha_cond",
		ParentSummaryID: "sum_alpha_1",
		Ord:             0,
	}))
	require.NoError(t, q.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
		SummaryID:       "sum_alpha_cond",
		ParentSummaryID: "sum_alpha_2",
		Ord:             1,
	}))

	_, err = rawDB.ExecContext(ctx, `INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids, created_at) VALUES (?, ?, ?, ?, ?, ?, 100)`,
		"sum_beta_1", "sess_beta", KindLeaf, "Implemented payment processing module", 60, "[]")
	require.NoError(t, err)
}

func TestBindle(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		result, err := store.Bindle(t.Context(), "sum_alpha_1")
		require.NoError(t, err)
		require.Contains(t, result, "Summary ID: sum_alpha_1")
		require.Contains(t, result, "Kind: leaf")
		require.Contains(t, result, "Tokens: 50")
		require.Contains(t, result, "JWT tokens")
		require.Contains(t, result, "file_auth_1")
	})

	t.Run("condensed_with_parents", func(t *testing.T) {
		t.Parallel()

		result, err := store.Bindle(t.Context(), "sum_alpha_cond")
		require.NoError(t, err)
		require.Contains(t, result, "Kind: condensed")
		require.Contains(t, result, "Parents: sum_alpha_1, sum_alpha_2")
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		result, err := store.Bindle(t.Context(), "sum_nonexistent")
		require.NoError(t, err)
		require.Contains(t, result, "Summary not found: sum_nonexistent")
	})
}

func TestAncestry(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("condensed_chain", func(t *testing.T) {
		t.Parallel()

		result, err := store.Ancestry(t.Context(), "sum_alpha_cond")
		require.NoError(t, err)
		require.Contains(t, result, "sum_alpha_cond")
		require.Contains(t, result, "sum_alpha_1")
		require.Contains(t, result, "sum_alpha_2")
		require.Contains(t, result, "3 levels")
	})

	t.Run("leaf_no_parents", func(t *testing.T) {
		t.Parallel()

		result, err := store.Ancestry(t.Context(), "sum_alpha_1")
		require.NoError(t, err)
		require.Contains(t, result, "1 levels")
		require.Contains(t, result, "sum_alpha_1")
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		result, err := store.Ancestry(t.Context(), "sum_nonexistent")
		require.NoError(t, err)
		require.Contains(t, result, "Summary not found: sum_nonexistent")
	})
}

func TestDolt(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("session_with_summaries", func(t *testing.T) {
		t.Parallel()

		result, err := store.Dolt(t.Context(), "sess_alpha")
		require.NoError(t, err)
		require.Contains(t, result, "3 total")
		require.Contains(t, result, "sum_alpha_1")
		require.Contains(t, result, "sum_alpha_2")
		require.Contains(t, result, "sum_alpha_cond")
	})

	t.Run("empty_session", func(t *testing.T) {
		t.Parallel()

		createTestSession(t, q, "sess_empty")
		result, err := store.Dolt(t.Context(), "sess_empty")
		require.NoError(t, err)
		require.Contains(t, result, "No summaries found for session: sess_empty")
	})
}

func TestArchive(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("match_found", func(t *testing.T) {
		t.Parallel()

		result, err := store.Archive(t.Context(), "sess_alpha", "authentication")
		require.NoError(t, err)
		require.Contains(t, result, "authentication")
	})

	t.Run("no_match", func(t *testing.T) {
		t.Parallel()

		result, err := store.Archive(t.Context(), "sess_alpha", "nonexistent_topic")
		require.NoError(t, err)
		require.Contains(t, result, "No summaries matching")
	})
}

func TestSprig(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	t.Run("returns_latest", func(t *testing.T) {
		t.Parallel()

		result, err := store.Sprig(t.Context(), "sess_alpha")
		require.NoError(t, err)
		require.Contains(t, result, "Latest summary for session sess_alpha")
		require.Contains(t, result, "sum_alpha_cond")
		require.Contains(t, result, "Authentication system")
	})

	t.Run("no_summaries", func(t *testing.T) {
		t.Parallel()

		createTestSession(t, q, "sess_empty_sprig")
		result, err := store.Sprig(t.Context(), "sess_empty_sprig")
		require.NoError(t, err)
		require.Contains(t, result, "No summaries found for session: sess_empty_sprig")
	})
}

func TestAncestryDeepChain(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	ctx := context.Background()
	store := newStore(q, rawDB)

	createTestSession(t, q, "sess_deep")

	// Create a chain: leaf -> condensed_1 -> condensed_2
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: "sum_d0", SessionID: "sess_deep",
		Kind: KindLeaf, Content: "base layer", TokenCount: 10, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: "sum_d1", SessionID: "sess_deep",
		Kind: KindCondensed, Content: "first condensation", TokenCount: 8, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
		SummaryID: "sum_d1", ParentSummaryID: "sum_d0", Ord: 0,
	}))
	require.NoError(t, q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID: "sum_d2", SessionID: "sess_deep",
		Kind: KindCondensed, Content: "second condensation", TokenCount: 5, FileIds: "[]",
	}))
	require.NoError(t, q.InsertLcmSummaryParent(ctx, db.InsertLcmSummaryParentParams{
		SummaryID: "sum_d2", ParentSummaryID: "sum_d1", Ord: 0,
	}))

	result, err := store.Ancestry(ctx, "sum_d2")
	require.NoError(t, err)
	require.Contains(t, result, "3 levels")
	require.Contains(t, result, "sum_d2")
	require.Contains(t, result, "sum_d1")
	require.Contains(t, result, "sum_d0")
}

func TestBindleCrossSession(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	// Can retrieve a summary from a different session (no session scoping here).
	result, err := store.Bindle(t.Context(), "sum_beta_1")
	require.NoError(t, err)
	require.Contains(t, result, "Summary ID: sum_beta_1")
	require.Contains(t, result, "payment processing")
}

func TestDoltOtherSession(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedRetrievalData(t, q, rawDB)
	store := newStore(q, rawDB)

	result, err := store.Dolt(t.Context(), "sess_beta")
	require.NoError(t, err)
	require.Contains(t, result, "1 total")
	require.Contains(t, result, "sum_beta_1")
}

func TestSprigDBError(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	rawDB.Close()

	store := newStore(q, rawDB)
	_, err := store.Sprig(t.Context(), "sess_any")
	require.Error(t, err)
}

func TestAncestryDBError(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	rawDB.Close()

	store := newStore(q, rawDB)
	_, err := store.Ancestry(t.Context(), "sum_any")
	require.Error(t, err)
}
