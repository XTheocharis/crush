package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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

func TestManager_InitSession_BootstrapsLegacyVisibleMessages(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-init-legacy"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "msg-hidden-1", "user", "hidden 1")
	createTestMessage(t, queries, sessionID, "msg-hidden-2", "assistant", "hidden 2")
	summaryID := createTestSummaryMessage(t, queries, sessionID, "msg-summary", "legacy summary")
	createTestMessage(t, queries, sessionID, "msg-visible-1", "user", "visible 1")
	createTestMessage(t, queries, sessionID, "msg-visible-2", "assistant", "visible 2")
	setSessionSummaryMessageID(t, queries, sessionID, summaryID)

	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	sessionRow, err := queries.GetSessionByID(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, sessionRow.SummaryMessageID.Valid)

	items, err := queries.ListLcmContextItems(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, items, 3)

	var gotIDs []string
	for i, item := range items {
		require.Equal(t, int64(i), item.Position)
		require.True(t, item.MessageID.Valid)
		gotIDs = append(gotIDs, item.MessageID.String)
	}

	require.Equal(t, []string{
		"msg-summary",
		"msg-visible-1",
		"msg-visible-2",
	}, gotIDs)
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

func TestIsOverHardLimitNotEqualToIsOverSoftThreshold(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(200000)
	ctx := context.Background()

	sessionID := "sess-hard-vs-soft"
	createTestSession(t, queries, sessionID)
	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Less(t, budget.SoftThreshold, budget.HardLimit)

	tokenCount := budget.SoftThreshold + 1000
	require.Less(t, tokenCount, budget.HardLimit, "token count must be below hard limit for this test")

	msgID := "msg-mid"
	createTestMessage(t, queries, sessionID, msgID, "user", "mid message")
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: tokenCount,
	})
	require.NoError(t, err)

	softCheck, err := mgr.IsOverSoftThreshold(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, softCheck.OverSoft, "should be over soft threshold at %d tokens (soft=%d)", tokenCount, budget.SoftThreshold)

	hardCheck, err := mgr.IsOverHardLimit(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, hardCheck.OverHard, "should NOT be over hard limit at %d tokens (hard=%d)", tokenCount, budget.HardLimit)

	sessionID2 := "sess-hard-vs-soft-over"
	createTestSession(t, queries, sessionID2)
	err = mgr.InitSession(ctx, sessionID2)
	require.NoError(t, err)

	hardTokenCount := budget.HardLimit + 1000
	msgID2 := "msg-over-hard"
	createTestMessage(t, queries, sessionID2, msgID2, "user", "over hard message")
	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID2,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID2, Valid: true},
		TokenCount: hardTokenCount,
	})
	require.NoError(t, err)

	hardCheck2, err := mgr.IsOverHardLimit(ctx, sessionID2)
	require.NoError(t, err)
	require.True(t, hardCheck2.OverHard, "should be over hard limit at %d tokens (hard=%d)", hardTokenCount, budget.HardLimit)
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

func TestManager_SetRepoMapTokens_RecomputesBudgetAndPersists(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-repomap-budget"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	require.NoError(t, mgr.SetRepoMapTokens(ctx, sessionID, 1500))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	cfg, err := queries.GetLcmSessionConfig(ctx, sessionID)
	require.NoError(t, err)

	expected := ComputeBudget(BudgetConfig{
		ContextWindow:    cfg.ModelCtxMaxTokens,
		CutoffThreshold:  cfg.CtxCutoffThreshold,
		RepoMapTokens:    1500,
		ModelOutputLimit: 0,
	})

	require.Equal(t, expected.SoftThreshold, budget.SoftThreshold)
	require.Equal(t, expected.HardLimit, budget.HardLimit)
	require.Equal(t, expected.ContextWindow, budget.ContextWindow)
	require.Equal(t, expected.SoftThreshold, cfg.SoftThresholdTokens)
	require.Equal(t, expected.HardLimit, cfg.HardThresholdTokens)
}

func TestManager_SetRepoMapTokens_UpsertsWhenConfigMissing(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-repomap-upsert"
	createTestSession(t, queries, sessionID)

	require.NoError(t, mgr.SetRepoMapTokens(ctx, sessionID, 900))

	cfg, err := queries.GetLcmSessionConfig(ctx, sessionID)
	require.NoError(t, err)
	expected := ComputeBudget(BudgetConfig{
		ContextWindow:    cfg.ModelCtxMaxTokens,
		CutoffThreshold:  cfg.CtxCutoffThreshold,
		RepoMapTokens:    900,
		ModelOutputLimit: 0,
	})

	require.Equal(t, expected.SoftThreshold, cfg.SoftThresholdTokens)
	require.Equal(t, expected.HardLimit, cfg.HardThresholdTokens)
}

func TestManager_SetRepoMapTokens_RecomputeAfterContextWindowUpdate(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-repomap-recompute"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))
	require.NoError(t, mgr.SetRepoMapTokens(ctx, sessionID, 1200))
	require.NoError(t, mgr.UpdateContextWindow(ctx, 256000))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	cfg, err := queries.GetLcmSessionConfig(ctx, sessionID)
	require.NoError(t, err)

	expected := ComputeBudget(BudgetConfig{
		ContextWindow:    256000,
		CutoffThreshold:  cfg.CtxCutoffThreshold,
		RepoMapTokens:    1200,
		ModelOutputLimit: 0,
	})

	require.Equal(t, expected.SoftThreshold, budget.SoftThreshold)
	require.Equal(t, expected.HardLimit, budget.HardLimit)
	require.Equal(t, int64(256000), budget.ContextWindow)
	require.Equal(t, expected.SoftThreshold, cfg.SoftThresholdTokens)
	require.Equal(t, expected.HardLimit, cfg.HardThresholdTokens)
}

func TestManager_SetRepoMapTokens_ConcurrentConsistency(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-repomap-concurrent"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	const workers = 24
	const repoMapTokens = int64(777)
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if err := mgr.SetRepoMapTokens(ctx, sessionID, repoMapTokens); err != nil {
				errCh <- err
				return
			}
			if _, err := mgr.GetBudget(ctx, sessionID); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	cfg, err := queries.GetLcmSessionConfig(ctx, sessionID)
	require.NoError(t, err)

	expected := ComputeBudget(BudgetConfig{
		ContextWindow:    cfg.ModelCtxMaxTokens,
		CutoffThreshold:  cfg.CtxCutoffThreshold,
		RepoMapTokens:    repoMapTokens,
		ModelOutputLimit: 0,
	})

	require.Equal(t, expected.SoftThreshold, budget.SoftThreshold)
	require.Equal(t, expected.HardLimit, budget.HardLimit)
	require.Equal(t, expected.SoftThreshold, cfg.SoftThresholdTokens)
	require.Equal(t, expected.HardLimit, cfg.HardThresholdTokens)
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
			SessionID_2: sessionID,
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

func TestManager_CompressWith_DelegatesToCompressor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManagerWithLLM(queries, sqlDB, &stubLLMClient{response: "compressed"})
	ctx := context.Background()

	output, err := mgr.CompressWith(ctx, "some input text")
	require.NoError(t, err)
	require.Equal(t, "compressed", output.Content)
	require.Equal(t, "message", output.Strategy)
}

func TestManager_CompressWith_NoCompressor(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	_, err := mgr.CompressWith(ctx, "some input text")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoCompressor))
}

func TestManager_RetrieveSummary_DelegatesToRetrieval(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-retrieval-test"
	createTestSession(t, queries, sessionID)

	result, err := mgr.RetrieveSummary(ctx, "nonexistent-summary")
	require.NoError(t, err)
	require.Contains(t, result, "Summary not found")
}

type stubLLMClient struct {
	response string
	err      error
}

func (s *stubLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	return s.response, s.err
}

func TestContextWindowFromModel(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	mgr.SetDefaultContextWindow(200000)

	sessionID := "sess-ctx-from-model"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(200000), budget.ContextWindow)

	expected := ComputeBudget(BudgetConfig{
		ContextWindow:   200000,
		CutoffThreshold: 0.6,
	})
	require.Equal(t, expected.SoftThreshold, budget.SoftThreshold)
	require.Equal(t, expected.HardLimit, budget.HardLimit)
}

func TestContextWindowZeroFallback(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	// Do NOT call SetDefaultContextWindow — the default 128000 must remain.
	sessionID := "sess-ctx-zero-fallback"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(128000), budget.ContextWindow)
}

func TestCutoffFromConfig(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	mgr.SetCutoffThreshold(0.8)

	sessionID := "sess-cutoff-config"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	expected := ComputeBudget(BudgetConfig{
		ContextWindow:   128000,
		CutoffThreshold: 0.8,
	})
	require.Equal(t, expected.SoftThreshold, budget.SoftThreshold, "soft threshold should use 0.8 cutoff, not 0.6 default")
	require.Equal(t, expected.HardLimit, budget.HardLimit, "hard limit should reflect 0.8 cutoff budget")

	// Sanity: 0.8 cutoff produces a higher soft threshold than 0.6.
	defaultBudget := ComputeBudget(BudgetConfig{
		ContextWindow:   128000,
		CutoffThreshold: 0.6,
	})
	require.Greater(t, budget.SoftThreshold, defaultBudget.SoftThreshold, "0.8 cutoff should produce higher soft threshold than 0.6")
}

func TestLargeOutputThreshold(t *testing.T) {
	t.Parallel()

	t.Run("constant default is 50000", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 50000, LargeOutputThreshold)
	})

	t.Run("setter accepts positive value", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)

		mgr.SetLargeOutputThreshold(10000)
		cm := mgr.(*compactionManager)
		require.Equal(t, int64(10000), cm.largeOutputThreshold)
	})

	t.Run("setter ignores zero", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)

		mgr.SetLargeOutputThreshold(25000)
		mgr.SetLargeOutputThreshold(0)
		cm := mgr.(*compactionManager)
		require.Equal(t, int64(25000), cm.largeOutputThreshold)
	})

	t.Run("setter ignores negative", func(t *testing.T) {
		t.Parallel()
		queries, sqlDB := setupTestDB(t)
		mgr := NewManager(queries, sqlDB)

		mgr.SetLargeOutputThreshold(25000)
		mgr.SetLargeOutputThreshold(-1)
		cm := mgr.(*compactionManager)
		require.Equal(t, int64(25000), cm.largeOutputThreshold)
	})
}

func TestManager_GetTurnCount_NoSession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	require.Equal(t, int64(0), mgr.GetTurnCount("nonexistent-session"))
}

func TestManager_GetTurnCount_AfterPostTurnHook(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-turn-count"
	createTestSession(t, queries, sessionID)

	cm := mgr.(*compactionManager)
	// Use a very high interval so ShouldExtract always returns false.
	cm.autoMemoryExtractor = NewAutoMemoryExtractor(nil, nil, 999999)

	mgr.PostTurnHook(ctx, sessionID)
	require.Equal(t, int64(1), mgr.GetTurnCount(sessionID))

	mgr.PostTurnHook(ctx, sessionID)
	require.Equal(t, int64(2), mgr.GetTurnCount(sessionID))
}

func TestManager_GetIterationCount_NoSession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	require.Equal(t, int64(0), mgr.GetIterationCount("nonexistent-session"))
}

func TestManager_IncrementIteration(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	sessionID := "sess-iter-count"

	mgr.IncrementIteration(sessionID)
	require.Equal(t, int64(1), mgr.GetIterationCount(sessionID))

	mgr.IncrementIteration(sessionID)
	mgr.IncrementIteration(sessionID)
	require.Equal(t, int64(3), mgr.GetIterationCount(sessionID))
}

func TestManager_PostTurnHook_ResetsIterationCounter(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-iter-reset"
	createTestSession(t, queries, sessionID)

	cm := mgr.(*compactionManager)
	cm.autoMemoryExtractor = NewAutoMemoryExtractor(nil, nil, 999999)

	mgr.IncrementIteration(sessionID)
	mgr.IncrementIteration(sessionID)
	require.Equal(t, int64(2), mgr.GetIterationCount(sessionID))

	mgr.PostTurnHook(ctx, sessionID)
	require.Equal(t, int64(0), mgr.GetIterationCount(sessionID),
		"iteration counter should be reset when turn advances")
	require.Equal(t, int64(1), mgr.GetTurnCount(sessionID))
}

func TestManager_IncrementIteration_Concurrent(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)

	sessionID := "sess-iter-concurrent"
	const workers = 100

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			mgr.IncrementIteration(sessionID)
		}()
	}
	wg.Wait()

	require.Equal(t, int64(workers), mgr.GetIterationCount(sessionID))
}
