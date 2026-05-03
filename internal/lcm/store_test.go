package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestStore_InsertLargeTextContent(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-large-text"
	createTestSession(t, queries, sessionID)

	content := strings.Repeat("Hello world! ", 1000)
	fileID, err := store.InsertLargeTextContent(ctx, sessionID, content, "/path/to/file.txt")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(fileID, FileIDPrefix), "file ID should start with file_")

	// Verify we can retrieve it.
	retrieved, err := store.GetLargeFileContent(ctx, fileID, sessionID, 0)
	require.NoError(t, err)
	require.Equal(t, content, retrieved)
}

func TestStore_GetLargeFileContent_Truncation(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-truncate"
	createTestSession(t, queries, sessionID)

	content := strings.Repeat("x", 5000)
	fileID, err := store.InsertLargeTextContent(ctx, sessionID, content, "big.txt")
	require.NoError(t, err)

	truncated, err := store.GetLargeFileContent(ctx, fileID, sessionID, 100)
	require.NoError(t, err)
	require.Len(t, truncated, 100)
}

func TestStore_LargeFileExists(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-exists"
	createTestSession(t, queries, sessionID)

	// Non-existent file.
	exists, err := store.LargeFileExists(ctx, "file_0000000000000000", sessionID)
	require.NoError(t, err)
	require.False(t, exists)

	// Insert and check.
	fileID, err := store.InsertLargeTextContent(ctx, sessionID, "data", "file.txt")
	require.NoError(t, err)

	exists, err = store.LargeFileExists(ctx, fileID, sessionID)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestStore_InsertLeafSummary_And_GetContextEntries(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-leaf"
	createTestSession(t, queries, sessionID)

	// Create messages.
	for i := range 3 {
		msgID := fmt.Sprintf("msg-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user", fmt.Sprintf("Message content %d", i))
	}

	// Insert context items for messages.
	for i := range 3 {
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: fmt.Sprintf("msg-%d", i), Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	// Insert a leaf summary.
	summaryID, _ := GenerateSummaryID(sessionID)
	messageIDs := []string{"msg-0", "msg-1"}
	err := store.InsertLeafSummary(ctx, queries, sessionID, summaryID,
		"Summary of messages 0 and 1", 50, []string{}, messageIDs)
	require.NoError(t, err)

	// Replace positions.
	err = store.ReplacePositionsWithSummary(ctx, queries, sessionID, summaryID, 0, 50, messageIDs)
	require.NoError(t, err)

	// Get context entries.
	entries, err := store.GetContextEntries(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 2, "should have summary + remaining message")

	// First should be the summary.
	require.Equal(t, "summary", entries[0].ItemType)
	require.Equal(t, summaryID, entries[0].SummaryID)
	require.Equal(t, "Summary of messages 0 and 1", entries[0].SummaryContent)
	require.Equal(t, KindLeaf, entries[0].SummaryKind)

	// Second should be msg-2.
	require.Equal(t, "message", entries[1].ItemType)
	require.Equal(t, "msg-2", entries[1].MessageID)
}

func TestStore_InsertLeafSummaryAtomically(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-atomic"
	createTestSession(t, queries, sessionID)

	// Create messages.
	for i := range 4 {
		msgID := fmt.Sprintf("msg-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user", fmt.Sprintf("Atomic message %d", i))
	}

	// Insert context items.
	for i := range 4 {
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: fmt.Sprintf("msg-%d", i), Valid: true},
			TokenCount: 200,
		})
		require.NoError(t, err)
	}

	summaryID, _ := GenerateSummaryID(sessionID)
	messageIDs := []string{"msg-0", "msg-1", "msg-2"}

	err := store.InsertLeafSummaryAtomically(
		ctx, sessionID, summaryID, "Atomic summary", 75,
		[]string{}, messageIDs, 0, messageIDs,
	)
	require.NoError(t, err)

	// Verify context now has summary + msg-3.
	entries, err := store.GetContextEntries(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "summary", entries[0].ItemType)
	require.Equal(t, "message", entries[1].ItemType)
	require.Equal(t, "msg-3", entries[1].MessageID)
}

func TestStore_GetContextTokenCount(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-token-count"
	createTestSession(t, queries, sessionID)

	// Empty session should return 0.
	count, err := store.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	// Insert messages with known token counts.
	for i := range 3 {
		msgID := fmt.Sprintf("msg-tc-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user", "token content")

		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		require.NoError(t, err)
	}

	count, err = store.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(300), count)
}

func TestStore_GetMessages(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-get-msgs"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "m1", "user", "Hello")
	createTestMessage(t, queries, sessionID, "m2", "assistant", "Hi there")

	msgs, err := store.GetMessages(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, "Hello", msgs[0].Content)
	require.Equal(t, "Hi there", msgs[1].Content)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "assistant", msgs[1].Role)
}

func TestStore_GetMessageCount(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-msg-count"
	createTestSession(t, queries, sessionID)

	count, err := store.GetMessageCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	createTestMessage(t, queries, sessionID, "mc-1", "user", "one")
	createTestMessage(t, queries, sessionID, "mc-2", "user", "two")

	count, err = store.GetMessageCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestStore_GetSummaryMessageIDs(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-sum-msgs"
	createTestSession(t, queries, sessionID)

	// Create messages.
	createTestMessage(t, queries, sessionID, "sm-1", "user", "first")
	createTestMessage(t, queries, sessionID, "sm-2", "user", "second")

	// Insert a summary linking to those messages.
	summaryID := "sum_test1234567890"
	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "test summary",
		TokenCount: 10,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: summaryID,
		MessageID: "sm-1",
		Ord:       0,
	})
	require.NoError(t, err)
	err = queries.InsertLcmSummaryMessage(ctx, db.InsertLcmSummaryMessageParams{
		SummaryID: summaryID,
		MessageID: "sm-2",
		Ord:       1,
	})
	require.NoError(t, err)

	ids, err := store.GetSummaryMessageIDs(ctx, summaryID)
	require.NoError(t, err)
	require.Equal(t, []string{"sm-1", "sm-2"}, ids)
}

func TestStore_GetLargeFilesBySession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-large-files"
	createTestSession(t, queries, sessionID)

	// Insert two files.
	_, err := store.InsertLargeTextContent(ctx, sessionID, "content1", "a.txt")
	require.NoError(t, err)
	_, err = store.InsertLargeTextContent(ctx, sessionID, "content2", "b.txt")
	require.NoError(t, err)

	files, err := store.GetLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestExtractFileIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "no file IDs",
			content:  "just some text",
			expected: nil,
		},
		{
			name:     "large file stored pattern",
			content:  "[Large File Stored: file_a3f9c2d1e8b4f7a2]",
			expected: []string{"file_a3f9c2d1e8b4f7a2"},
		},
		{
			name:     "large user text stored pattern",
			content:  "[Large User Text Stored: file_1234567890abcdef]",
			expected: []string{"file_1234567890abcdef"},
		},
		{
			name:     "LCM file ID pattern",
			content:  "LCM File ID: file_abcdef1234567890",
			expected: []string{"file_abcdef1234567890"},
		},
		{
			name:     "multiple patterns",
			content:  "[Large File Stored: file_aaaa000000000000] and LCM File ID: file_bbbb000000000000",
			expected: []string{"file_aaaa000000000000", "file_bbbb000000000000"},
		},
		{
			name:     "large tool output",
			content:  "[Large Tool Output Stored: file_a3f9c2d1e8b4f7a2]",
			expected: []string{"file_a3f9c2d1e8b4f7a2"},
		},
		{
			name:     "deduplication",
			content:  "[Large File Stored: file_aaaa000000000000] [Large File Stored: file_aaaa000000000000]",
			expected: []string{"file_aaaa000000000000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractFileIDs(tt.content)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestStore_GetMessagePartsBatch(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-batch"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "batch-1", "user", "msg one")
	createTestMessage(t, queries, sessionID, "batch-2", "user", "msg two")

	result, err := store.GetMessagePartsBatch(ctx, []string{"batch-1", "batch-2"})
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Contains(t, result, "batch-1")
	require.Contains(t, result, "batch-2")
}

func TestStore_GetMessagePartsBatch_Empty(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	result, err := store.GetMessagePartsBatch(ctx, []string{})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestStore_GetAncestorSessionIDs(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	// Create parent and child sessions.
	createTestSession(t, queries, "parent-sess")
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:              "child-sess",
		ParentSessionID: sql.NullString{String: "parent-sess", Valid: true},
		Title:           "child session",
	})
	require.NoError(t, err)

	ancestors, err := store.GetAncestorSessionIDs(ctx, "child-sess")
	require.NoError(t, err)
	require.Contains(t, ancestors, "child-sess")
	require.Contains(t, ancestors, "parent-sess")
}

func TestStore_LargeFileAccess_CrossSession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	// Create parent session and store a file.
	createTestSession(t, queries, "file-parent")
	fileID, err := store.InsertLargeTextContent(ctx, "file-parent", "secret data", "secret.txt")
	require.NoError(t, err)

	// Create child session.
	_, err = queries.CreateSession(ctx, db.CreateSessionParams{
		ID:              "file-child",
		ParentSessionID: sql.NullString{String: "file-parent", Valid: true},
		Title:           "child",
	})
	require.NoError(t, err)

	// Child should be able to access parent's file.
	content, err := store.GetLargeFileContent(ctx, fileID, "file-child", 0)
	require.NoError(t, err)
	require.Equal(t, "secret data", content)

	// Unrelated session should not.
	createTestSession(t, queries, "unrelated-sess")
	_, err = store.GetLargeFileContent(ctx, fileID, "unrelated-sess", 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not belong to session")
}

func TestStore_ExtractTextFromParts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "text part",
			input:    `[{"type":"text","data":{"text":"Hello world"}}]`,
			expected: "Hello world",
		},
		{
			name:     "tool result part",
			input:    `[{"type":"tool_result","data":{"content":"result output"}}]`,
			expected: "result output",
		},
		{
			name:     "multiple parts",
			input:    `[{"type":"text","data":{"text":"line1"}},{"type":"text","data":{"text":"line2"}}]`,
			expected: "line1\nline2",
		},
		{
			name:     "invalid JSON",
			input:    `not valid json`,
			expected: "",
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractTextFromParts(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func BenchmarkStore_InsertAndRetrieve(b *testing.B) {
	queries, sqlDB := setupBenchDB(b)
	store := newStore(queries, sqlDB)
	ctx := context.Background()

	sessionID := "bench-session"
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}

	content := strings.Repeat("bench content ", 100)

	for b.Loop() {
		_, err := store.InsertLargeTextContent(ctx, sessionID, content, "bench.txt")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// setupBenchDB is like setupTestDB but uses b.TempDir().
func setupBenchDB(b *testing.B) (*db.Queries, *sql.DB) {
	b.Helper()
	initGoose()

	tmpDir := b.TempDir()
	dbPath := fmt.Sprintf("file:%s/bench.db?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", tmpDir)

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { sqlDB.Close() })

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		b.Fatal(err)
	}

	return db.New(sqlDB), sqlDB
}
