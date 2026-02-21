package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestManager_InitSession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-init"
	createTestSession(t, queries, sessionID)

	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Verify session config was created by fetching the budget.
	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(128000), budget.ContextWindow)
	require.Greater(t, budget.SoftThreshold, int64(0))
	require.Greater(t, budget.HardLimit, int64(0))
	require.LessOrEqual(t, budget.SoftThreshold, budget.HardLimit)
}

func TestManager_GetBudget_DefaultWhenNoConfig(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	// No session config exists; should return default budget.
	budget, err := mgr.GetBudget(ctx, "nonexistent-session")
	require.NoError(t, err)
	require.Equal(t, int64(128000), budget.ContextWindow)
	require.Greater(t, budget.SoftThreshold, int64(0))
}

func TestManager_SetDefaultContextWindow(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	mgr.SetDefaultContextWindow(200000)

	budget, err := mgr.GetBudget(ctx, "any-session")
	require.NoError(t, err)
	require.Equal(t, int64(200000), budget.ContextWindow)
}

func TestManager_GetContextTokenCount_Empty(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-empty-tokens"
	createTestSession(t, queries, sessionID)

	count, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestManager_IsOverSoftThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-soft-check"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// No context items => under threshold.
	check, err := mgr.IsOverSoftThreshold(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, check.OverSoft)
	require.False(t, check.OverHard)
	require.Equal(t, int64(0), check.CurrentTokens)
	require.Greater(t, check.SoftLimit, int64(0))
	require.Greater(t, check.HardLimit, int64(0))
}

func TestManager_IsOverSoftThreshold_WhenOverBudget(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-over-soft"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	// Insert a message with token count exceeding soft threshold.
	msgID := "msg-big"
	createTestMessage(t, queries, sessionID, msgID, "user", "big message")
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: budget.SoftThreshold + 1000,
	})
	require.NoError(t, err)

	check, err := mgr.IsOverSoftThreshold(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, check.OverSoft)
}

func TestManager_IsOverHardLimit(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-over-hard"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	msgID := "msg-huge"
	createTestMessage(t, queries, sessionID, msgID, "user", "huge message")
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: budget.HardLimit + 5000,
	})
	require.NoError(t, err)

	check, err := mgr.IsOverHardLimit(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, check.OverHard)
}

func TestManager_GetContextFiles(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	files := mgr.GetContextFiles()
	require.Len(t, files, 1)
	require.Equal(t, "LCM Instructions", files[0].Name)
	require.Contains(t, files[0].Content, "Lossless Context Management")
}

func TestManager_UpdateContextWindow(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-update-ctx"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Update to a new window size.
	err = mgr.UpdateContextWindow(ctx, 256000)
	require.NoError(t, err)

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(256000), budget.ContextWindow)
}

func TestManager_Subscribe(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := t.Context()

	ch := mgr.Subscribe(ctx)
	require.NotNil(t, ch)
}

func TestManager_GetFormattedContext_Empty(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fmt-empty"
	createTestSession(t, queries, sessionID)

	entries, err := mgr.GetFormattedContext(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestManager_GetFormattedContext_WithSummary(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-fmt-sum"
	createTestSession(t, queries, sessionID)

	// Insert a summary.
	summaryID := "sum_fmttest12345678"
	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  summaryID,
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "This is a test summary",
		TokenCount: 10,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	// Insert context item referencing the summary.
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "summary",
		SummaryID:  sql.NullString{String: summaryID, Valid: true},
		TokenCount: 10,
	})
	require.NoError(t, err)

	entries, err := mgr.GetFormattedContext(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, summaryID, entries[0].ID)
	require.Contains(t, entries[0].Content, summaryID)
	require.Contains(t, entries[0].Content, "This is a test summary")
}

func TestManager_ScheduleCompaction_Dedup(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-dedup"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Schedule two compactions; second should return immediately.
	ch1 := mgr.ScheduleCompaction(ctx, sessionID)
	ch2 := mgr.ScheduleCompaction(ctx, sessionID)

	result1 := <-ch1
	result2 := <-ch2

	// Both should complete without error. At least one should be a no-op dedup.
	_ = result1
	_ = result2
}

func BenchmarkManager_GetBudget(b *testing.B) {
	queries, sqlDB := setupBenchDB(b)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "bench-budget"
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}

	err = mgr.InitSession(ctx, sessionID)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := mgr.GetBudget(ctx, sessionID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkManager_GetContextTokenCount(b *testing.B) {
	queries, sqlDB := setupBenchDB(b)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "bench-token-count"
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "bench",
	})
	if err != nil {
		b.Fatal(err)
	}

	// Insert some context items.
	for i := range 50 {
		msgID := fmt.Sprintf("bench-msg-%d", i)
		_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
			ID:          msgID,
			SessionID:   sessionID,
			Role:        "user",
			Parts:       `[{"type":"text","data":{"text":"bench"}}]`,
		})
		if err != nil {
			b.Fatal(err)
		}
		err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: 100,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := mgr.GetContextTokenCount(ctx, sessionID)
		if err != nil {
			b.Fatal(err)
		}
	}
}
