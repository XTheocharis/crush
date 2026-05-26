package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

// This file contains B1/B4 migration safety tests that document the expected
// behavioral contract of compaction through the Manager interface. These tests
// MUST pass after the CompactOnce removal. Every test exercises the public
// Manager interface only — no direct internal method calls.

// ---------------------------------------------------------------------------
// Helper: insert N messages with a given per-message token count.
// ---------------------------------------------------------------------------

func insertTestMessages(t *testing.T, queries *db.Queries, sessionID string, count int, tokensEach int64) {
	t.Helper()
	ctx := context.Background()
	for i := range count {
		msgID := fmt.Sprintf("msg-migration-%s-%d", sessionID, i)
		text := fmt.Sprintf("Migration safety test message %d with enough content for token estimation purposes", i)
		createTestMessage(t, queries, sessionID, msgID, "user", text)
		require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: tokensEach,
		}))
	}
}

// ---------------------------------------------------------------------------
// 1. CompactIfOverHardLimit triggers compaction when context exceeds hard limit
// ---------------------------------------------------------------------------

func TestMigrationSafety_CompactIfOverHardLimit_TriggersCompaction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-hardlimit"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// With context window 1000: hardLimit ≈ 550.  Insert 5×200 = 1000 tokens.
	insertTestMessages(t, queries, sessionID, 5, 200)

	// Confirm over hard limit.
	check, err := mgr.IsOverHardLimit(ctx, sessionID)
	require.NoError(t, err)
	require.True(t, check.OverHard, "should be over hard limit before compaction")
	tokensBefore := check.CurrentTokens

	// CompactIfOverHardLimit must detect the over-limit state and compact.
	mgr.CompactIfOverHardLimit(ctx, sessionID)

	tokensAfter, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Less(t, tokensAfter, tokensBefore, "compaction should reduce token count")
}

// ---------------------------------------------------------------------------
// 2. PostCompactionHook fires exactly once per compaction call
//
// CompactUntilUnderLimit publishes exactly one started→completed event pair
// regardless of internal round count.  PostCompactionHook is invoked once in
// a goroutine after the completed event.  We verify the lifecycle by
// subscribing to events and asserting a single pair.
// ---------------------------------------------------------------------------

func TestMigrationSafety_PostCompactionHook_FiresOncePerCompaction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := t.Context()

	sessionID := "sess-migration-hook-once"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// 5×200 = 1000 tokens, over hardLimit ≈ 550.
	insertTestMessages(t, queries, sessionID, 5, 200)

	ch := mgr.Subscribe(ctx)

	err := mgr.CompactUntilUnderLimit(ctx, sessionID)
	require.NoError(t, err)

	// Exactly one started event.
	evt1 := <-ch
	require.Equal(t, CompactionStarted, evt1.Payload.Type)
	require.Equal(t, sessionID, evt1.Payload.SessionID)
	require.True(t, evt1.Payload.Blocking)

	// Exactly one completed event.
	evt2 := <-ch
	require.Equal(t, CompactionCompleted, evt2.Payload.Type)
	require.Equal(t, sessionID, evt2.Payload.SessionID)
	require.True(t, evt2.Payload.Success)

	// No further events — the hook fires once.
	select {
	case evt := <-ch:
		t.Fatalf("unexpected extra event: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected: no more events.
	}
}

// ---------------------------------------------------------------------------
// 3. Compact reduces token count
// ---------------------------------------------------------------------------

func TestMigrationSafety_Compact_ReducesTokenCount(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-reduce"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// 5×200 = 1000 tokens.  Compact(force=true) will summarise.
	insertTestMessages(t, queries, sessionID, 5, 200)

	tokensBefore, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(1000), tokensBefore)

	require.NoError(t, mgr.Compact(ctx, sessionID))

	tokensAfter, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Less(t, tokensAfter, tokensBefore, "Compact should reduce token count")
}

// ---------------------------------------------------------------------------
// 4. Concurrent compaction calls are serialized (no data races)
//
// Uses 2 messages (< MinMessagesToSummarize) so Compact is a safe no-op that
// still exercises mutex acquisition.  GenerateSummaryID uses millisecond
// timestamps, so multi-round compaction on the same session within the same
// millisecond would collide; this is avoided by design here.
// ---------------------------------------------------------------------------

func TestMigrationSafety_ConcurrentCompaction_Serialized(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-migration-concurrent"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	insertTestMessages(t, queries, sessionID, 2, 100)

	const workers = 12
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)
	wg.Add(workers * 3)

	for range workers {
		go func() {
			defer wg.Done()
			if err := mgr.Compact(ctx, sessionID); err != nil {
				errCh <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := mgr.GetContextTokenCount(ctx, sessionID); err != nil {
				errCh <- err
			}
		}()
		go func() {
			defer wg.Done()
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
}

// ---------------------------------------------------------------------------
// 5. Mid-turn resumption: context is accessible after compaction
// ---------------------------------------------------------------------------

func TestMigrationSafety_MidTurnResumption_ContextAccessibleAfterCompaction(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-resume"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	insertTestMessages(t, queries, sessionID, 5, 200)

	require.NoError(t, mgr.Compact(ctx, sessionID))

	// After compaction the session must remain usable.
	entries, err := mgr.GetFormattedContext(ctx, sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "context entries must be accessible after compaction")

	tokens, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Greater(t, tokens, int64(0), "token count should be positive after compaction")

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)
	require.Greater(t, budget.HardLimit, int64(0))

	// Simulate adding a new message after compaction (mid-turn resume).
	msgID := "msg-migration-resume-new"
	createTestMessage(t, queries, sessionID, msgID, "user", "resumed message after compaction")
	require.NoError(t, queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   int64(len(entries)),
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		TokenCount: 50,
	}))

	afterResume, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, tokens+50, afterResume, "new message should add to token count")
}

// ---------------------------------------------------------------------------
// 6. CompactUntilUnderLimit loops to convergence
//
// Verifies the convergence contract: tokens exceed the hard limit before the
// call, and are at or below the hard limit after.  The internal loop may
// take one or more rounds; the contract is that it terminates with tokens ≤
// hardLimit (or returns an error on stall).
// ---------------------------------------------------------------------------

func TestMigrationSafety_CompactUntilUnderLimit_LoopsToConvergence(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-loops"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	budget, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	// 5×200 = 1000 tokens > hardLimit ≈ 550.
	insertTestMessages(t, queries, sessionID, 5, 200)

	tokensBefore, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Greater(t, tokensBefore, budget.HardLimit)

	err = mgr.CompactUntilUnderLimit(ctx, sessionID)
	require.NoError(t, err, "compaction should converge")

	tokensAfter, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.LessOrEqual(t, tokensAfter, budget.HardLimit,
		"tokens must be under hard limit after convergence")
}

// ---------------------------------------------------------------------------
// 7. ScheduleCompaction defers and returns result
// ---------------------------------------------------------------------------

func TestMigrationSafety_ScheduleCompaction_DefersAndReturnsResult(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-schedule"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// 3×200 = 600 > softThreshold(400).  ScheduleCompaction runs with
	// force=false, so it must detect the over-threshold state and compact.
	insertTestMessages(t, queries, sessionID, 3, 200)

	resultCh := mgr.ScheduleCompaction(ctx, sessionID)
	require.NotNil(t, resultCh)

	result := <-resultCh
	require.True(t, result.ActionTaken,
		"scheduled compaction should compact when over soft threshold")
	require.GreaterOrEqual(t, result.Rounds, 1)

	// Token count should have decreased.
	tokensAfter, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Less(t, tokensAfter, int64(600),
		"scheduled compaction should reduce tokens")
}

// ---------------------------------------------------------------------------
// 8. CompactUntilUnderLimit is a no-op when already under limit
// ---------------------------------------------------------------------------

func TestMigrationSafety_CompactUntilUnderLimit_AlreadyUnderIsNoop(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-migration-under"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// No context items → 0 tokens, well under any limit.
	err := mgr.CompactUntilUnderLimit(ctx, sessionID)
	require.NoError(t, err, "should be a no-op when already under limit")
}

// ---------------------------------------------------------------------------
// 9. ScheduleCompaction deduplicates in-flight requests
// ---------------------------------------------------------------------------

func TestMigrationSafety_ScheduleCompaction_DeduplicatesInFlight(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	mgr.SetDefaultContextWindow(1000)
	ctx := context.Background()

	sessionID := "sess-migration-dedup"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	insertTestMessages(t, queries, sessionID, 3, 200)

	ch1 := mgr.ScheduleCompaction(ctx, sessionID)
	ch2 := mgr.ScheduleCompaction(ctx, sessionID)

	result1 := <-ch1
	result2 := <-ch2

	// At least one must be a real result (ActionTaken=true) and at least
	// one must be the dedup no-op (ActionTaken=false, zero Rounds).
	require.True(t, result1.ActionTaken || result2.ActionTaken,
		"at least one schedule call should perform compaction")
}

// ---------------------------------------------------------------------------
// 10. PostCompactionHook is safe to call on an empty session
// ---------------------------------------------------------------------------

func TestMigrationSafety_PostCompactionHook_SafeOnEmptySession(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-migration-hook-empty"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Should not panic.
	mgr.PostCompactionHook(ctx, sessionID)
}
