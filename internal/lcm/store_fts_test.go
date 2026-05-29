package lcm

import (
	"context"
	"database/sql"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func seedFTSData(t *testing.T, q *db.Queries, rawDB *sql.DB) {
	t.Helper()
	ctx := context.Background()

	createTestSession(t, q, "sess_fts_ranked")

	insertSummary := func(id, content string, tokens int) {
		_, err := rawDB.ExecContext(ctx,
			`INSERT INTO lcm_summaries (summary_id, session_id, kind, content, token_count, file_ids) VALUES (?, ?, ?, ?, ?, '[]')`,
			id, "sess_fts_ranked", KindLeaf, content, tokens)
		require.NoError(t, err)
	}

	insertSummary("sum_r_01", "The quick brown fox jumps over the lazy dog", 10)
	insertSummary("sum_r_02", "Authentication and authorization via JWT tokens for secure API access", 15)
	insertSummary("sum_r_03", "Database schema migration for user authentication tables", 12)
	insertSummary("sum_r_04", "Payment processing integration with stripe gateway", 20)
	insertSummary("sum_r_05", "Authentication system overview including JWT, OAuth2, and session tokens", 18)
	insertSummary("sum_r_06", "Unicode test: café résumé naïveüber", 5)

	// Content-synced FTS5 tables require explicit rebuild after direct inserts.
	_, err := rawDB.ExecContext(ctx, `INSERT INTO lcm_summaries_fts(lcm_summaries_fts) VALUES('rebuild')`)
	require.NoError(t, err)
}

func TestFTSRankedSearchReturnsResults(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "authentication", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "should find results for 'authentication'")

	for _, r := range results {
		require.NotEmpty(t, r.SummaryID, "each result should have a summary ID")
	}
}

func TestFTSRankedSearchSortedByRelevance(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "authentication", 10)
	require.NoError(t, err)
	require.Len(t, results, 3, "should find exactly 3 summaries mentioning authentication")

	// bm25() returns negative floats — more negative = more relevant.
	// Results should be sorted by rank ascending (most negative first).
	for i := 1; i < len(results); i++ {
		require.LessOrEqual(t, results[i-1].Rank, results[i].Rank,
			"results should be sorted by rank (most negative first)")
	}
}

func TestFTSRankedSearchSnippetHasHighlights(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "authentication", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// At least one snippet should contain the highlight markers.
	hasHighlight := false
	for _, r := range results {
		if containsMarker(r.Snippet, ">>>") && containsMarker(r.Snippet, "<<<") {
			hasHighlight = true
			break
		}
	}
	require.True(t, hasHighlight, "at least one snippet should contain >>> and <<< highlight markers")
}

func TestFTSRankedSearchEmptyResults(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "zzzznonexistent", 10)
	require.NoError(t, err)
	require.Empty(t, results, "should return empty slice for no matches")
}

func TestFTSRankedSearchNonASCII(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "café", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "unicode61 tokenizer should handle non-ASCII content")

	found := false
	for _, r := range results {
		if r.SummaryID == "sum_r_06" {
			found = true
			break
		}
	}
	require.True(t, found, "should find the unicode test summary by 'café'")
}

func TestFTSRankedSearchLimit(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	results, err := store.SearchSummariesRanked(t.Context(), "authentication", 2)
	require.NoError(t, err)
	require.Len(t, results, 2, "should respect the limit parameter")
}

func TestFTSRankedSearchDefaultLimit(t *testing.T) {
	t.Parallel()

	q, rawDB := setupTestDB(t)
	seedFTSData(t, q, rawDB)
	store := newStore(q, rawDB)

	// limit <= 0 should default to 10.
	results, err := store.SearchSummariesRanked(t.Context(), "authentication", 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

func containsMarker(s, marker string) bool {
	for i := 0; i+len(marker) <= len(s); i++ {
		if s[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}
