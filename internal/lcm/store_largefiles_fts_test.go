package lcm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLargeFilesFTS_InsertTriggerPopulates(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-insert"
	createTestSession(t, queries, sessionID)

	content := "The quick brown fox jumps over the lazy dog."
	fileID, err := store.InsertLargeTextContent(ctx, sessionID, content, "/path/to/fox.txt")
	require.NoError(t, err)
	require.NotEmpty(t, fileID)

	results, err := store.SearchLargeFiles(ctx, sessionID, "quick brown", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, fileID, results[0].FileID)
	require.Equal(t, "/path/to/fox.txt", results[0].Path)
	require.Contains(t, results[0].Snippet, "quick")
}

func TestLargeFilesFTS_SearchReturnsRankedResults(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-rank"
	createTestSession(t, queries, sessionID)

	fileID1, err := store.InsertLargeTextContent(ctx, sessionID,
		"database connection error timeout retry", "/logs/db.log")
	require.NoError(t, err)

	fileID2, err := store.InsertLargeTextContent(ctx, sessionID,
		"network error timeout websocket disconnect", "/logs/net.log")
	require.NoError(t, err)

	fileID3, err := store.InsertLargeTextContent(ctx, sessionID,
		"application started successfully on port 8080", "/logs/app.log")
	require.NoError(t, err)

	results, err := store.SearchLargeFiles(ctx, sessionID, "error timeout", 10)
	require.NoError(t, err)
	require.Len(t, results, 2)

	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.FileID] = true
	}
	require.True(t, ids[fileID1], "db.log should match")
	require.True(t, ids[fileID2], "net.log should match")
	require.False(t, ids[fileID3], "app.log should not match")

	for _, r := range results {
		require.NotEmpty(t, r.Snippet)
	}
}

func TestLargeFilesFTS_SessionScoping(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	session1 := "sess-fts-scope1"
	session2 := "sess-fts-scope2"
	createTestSession(t, queries, session1)
	createTestSession(t, queries, session2)

	_, err := store.InsertLargeTextContent(ctx, session1,
		"secret project alpha details", "/docs/alpha.txt")
	require.NoError(t, err)

	_, err = store.InsertLargeTextContent(ctx, session2,
		"secret project beta details", "/docs/beta.txt")
	require.NoError(t, err)

	results1, err := store.SearchLargeFiles(ctx, session1, "secret project", 10)
	require.NoError(t, err)
	require.Len(t, results1, 1)
	require.Equal(t, "/docs/alpha.txt", results1[0].Path)

	results2, err := store.SearchLargeFiles(ctx, session2, "secret project", 10)
	require.NoError(t, err)
	require.Len(t, results2, 1)
	require.Equal(t, "/docs/beta.txt", results2[0].Path)
}

func TestLargeFilesFTS_DeleteTriggerRemoves(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-delete"
	createTestSession(t, queries, sessionID)

	content := "temporary data that will be deleted soon"
	fileID, err := store.InsertLargeTextContent(ctx, sessionID, content, "/tmp/data.txt")
	require.NoError(t, err)

	results, err := store.SearchLargeFiles(ctx, sessionID, "temporary data", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	_, err = sqlDB.ExecContext(ctx, "DELETE FROM lcm_large_files WHERE file_id = ?", fileID)
	require.NoError(t, err)

	results, err = store.SearchLargeFiles(ctx, sessionID, "temporary data", 10)
	require.NoError(t, err)
	require.Len(t, results, 0)
}

func TestLargeFilesFTS_EmptySearchReturnsEmpty(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-empty"
	createTestSession(t, queries, sessionID)

	results, err := store.SearchLargeFiles(ctx, sessionID, "nonexistent content", 10)
	require.NoError(t, err)
	require.Len(t, results, 0)
}

func TestLargeFilesFTS_UpdateTriggerSyncs(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-update"
	createTestSession(t, queries, sessionID)

	fileID, err := store.InsertLargeTextContent(ctx, sessionID,
		"original content about birds", "/docs/nature.txt")
	require.NoError(t, err)

	results, err := store.SearchLargeFiles(ctx, sessionID, "birds", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	_, err = sqlDB.ExecContext(ctx,
		"UPDATE lcm_large_files SET content = ? WHERE file_id = ?",
		"updated content about marine mammals", fileID)
	require.NoError(t, err)

	results, err = store.SearchLargeFiles(ctx, sessionID, "birds", 10)
	require.NoError(t, err)
	require.Len(t, results, 0, "old content should no longer match")

	results, err = store.SearchLargeFiles(ctx, sessionID, "marine mammals", 10)
	require.NoError(t, err)
	require.Len(t, results, 1, "new content should match")
	require.Equal(t, fileID, results[0].FileID)
}

func TestLargeFilesFTS_LargeContentSearch(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fts-large"
	createTestSession(t, queries, sessionID)

	content := strings.Repeat("Lorem ipsum dolor sit amet. ", 1000) +
		"UNIQUE_KEYWORD_FINDME " +
		strings.Repeat(" consectetur adipiscing elit.", 1000)

	fileID, err := store.InsertLargeTextContent(ctx, sessionID, content, "/data/big.txt")
	require.NoError(t, err)

	results, err := store.SearchLargeFiles(ctx, sessionID, "UNIQUE_KEYWORD_FINDME", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, fileID, results[0].FileID)
	require.Contains(t, results[0].Snippet, "UNIQUE_KEYWORD_FINDME")
}
