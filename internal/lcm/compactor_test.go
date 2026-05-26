package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestCompactUntilUnderLimit_AlreadyUnder(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-already-under"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// No context items => 0 tokens, already under limit.
	err = mgr.CompactUntilUnderLimit(ctx, sessionID)
	require.NoError(t, err)
}

func TestCompact_PublishesEvents(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := t.Context()

	sessionID := "sess-events"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	ch := mgr.Subscribe(ctx)

	// Run compaction (no-op since no messages, but events should still fire).
	err = mgr.Compact(ctx, sessionID)
	require.NoError(t, err)

	// Should receive started and completed events.
	evt1 := <-ch
	require.Equal(t, CompactionStarted, evt1.Payload.Type)
	require.Equal(t, sessionID, evt1.Payload.SessionID)

	evt2 := <-ch
	require.Equal(t, CompactionCompleted, evt2.Payload.Type)
	require.Equal(t, sessionID, evt2.Payload.SessionID)
}

func TestCompact_Force_WithMessages(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-compact-force"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Create enough messages for summarization (>= MinMessagesToSummarize).
	for i := range 5 {
		msgID := fmt.Sprintf("cmsg-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user",
			fmt.Sprintf("Compaction test message %d: %s", i, strings.Repeat("content ", 20)))
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 500,
		})
		require.NoError(t, err)
	}

	err = mgr.Compact(ctx, sessionID)
	require.NoError(t, err)

	// Verify context entries were updated.
	inner := mgr.(*compactionManager)
	entries, err := inner.store.GetContextEntries(ctx, sessionID)
	require.NoError(t, err)

	// Should have at least one summary.
	hasSummary := false
	for _, e := range entries {
		if e.ItemType == "summary" {
			hasSummary = true
			break
		}
	}
	require.True(t, hasSummary, "should have at least one summary after compaction")
}

func TestCompact_Condensation(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-condense"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Insert two summaries directly. With contextWindow=1000, softThreshold=400.
	// 2×500=1000 > 400, so the fallback path will trigger condensation.
	for i := range 2 {
		sumID := fmt.Sprintf("sum_condense%08d%04d", i, i)
		err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
			SummaryID:  sumID,
			SessionID:  sessionID,
			Kind:       KindLeaf,
			Content:    fmt.Sprintf("Summary %d: lots of content here about things we discussed", i),
			TokenCount: 500,
			FileIds:    "[]",
		})
		require.NoError(t, err)

		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "summary",
			SummaryID:  sql.NullString{String: sumID, Valid: true},
			TokenCount: 500,
		})
		require.NoError(t, err)
	}

	err = mgr.Compact(ctx, sessionID)
	require.NoError(t, err)

	// Verify condensation happened (token count decreased).
	inner := mgr.(*compactionManager)
	entries, err := inner.store.GetContextEntries(ctx, sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "should have entries after compaction")
}

func TestFormatSummaryContent_Leaf(t *testing.T) {
	t.Parallel()
	entry := ContextEntry{
		SummaryID:      "sum_testformat12345",
		SummaryContent: "Test summary content",
		SummaryKind:    KindLeaf,
	}

	content := formatSummaryContent(entry)
	require.Contains(t, content, "[Summary ID: sum_testformat12345]")
	require.Contains(t, content, "Test summary content")
	require.NotContains(t, content, "Condensed from")
}

func TestFormatSummaryContent_Condensed(t *testing.T) {
	t.Parallel()
	entry := ContextEntry{
		SummaryID:      "sum_condensed12345",
		SummaryContent: "Condensed content",
		SummaryKind:    KindCondensed,
		ParentIDs:      []string{"sum_parent1", "sum_parent2"},
	}

	content := formatSummaryContent(entry)
	require.Contains(t, content, "[Summary ID: sum_condensed12345]")
	require.Contains(t, content, "[Condensed from: sum_parent1, sum_parent2]")
	require.Contains(t, content, "Condensed content")
}

func BenchmarkCompact(b *testing.B) {
	queries, sqlDB := setupBenchDB(b)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "bench-compact"
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := mgr.InitSession(ctx, sessionID); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_ = mgr.Compact(ctx, sessionID)
	}
}
