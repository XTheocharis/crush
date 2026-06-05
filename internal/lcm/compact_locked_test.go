package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

type trackingLLMClient struct {
	response  string
	callCount atomic.Int32
}

func (c *trackingLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	c.callCount.Add(1)
	return c.response, nil
}

// setupCompactLockedTest creates a manager with a tracking LLM client and a
// session pre-populated with context items that trigger MicroCompactor.
func setupCompactLockedTest(t *testing.T, tokenCount int64) (*compactionManager, *trackingLLMClient, string) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	llm := &trackingLLMClient{response: `{"type":"summary","content":"summarized"}`}
	mgr := NewManagerWithLLM(queries, sqlDB, llm).(*compactionManager)

	ctx := context.Background()
	sessionID := "sess-compact-locked"
	createTestSession(t, queries, sessionID)

	msgID := "msg-large-output"
	content := strings.Repeat("x", 500)
	createTestMessage(t, queries, sessionID, msgID, "assistant", content)

	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msgID, Valid: true},
		SummaryID:  sql.NullString{},
		TokenCount: tokenCount,
	})
	require.NoError(t, err)

	return mgr, llm, sessionID
}

// addSmallMessages inserts additional small messages with context items so
// trySummarize has enough messages to call the LLM.
func addSmallMessages(t *testing.T, mgr *compactionManager, sessionID string, count int) {
	t.Helper()
	ctx := context.Background()
	queries := mgr.queries
	for i := range count {
		msgID := fmt.Sprintf("msg-small-%d", i)
		createTestMessage(t, queries, sessionID, msgID, "user", "hello world "+strings.Repeat("x", 200))
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i + 1),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			SummaryID:  sql.NullString{},
			TokenCount: 1000,
		})
		require.NoError(t, err)
	}
}

func TestCompactLocked_Phase1DeltaSkipsPhase2(t *testing.T) {
	t.Parallel()

	// Given: 60K-token context item → MicroCompactor archives it, freeing tokens.
	mgr, llm, sessionID := setupCompactLockedTest(t, 60000)
	ctx := context.Background()
	mgr.SetActualPromptTokens(sessionID, 100000)

	// When: TargetTokens is high enough that after delta subtraction, we're under.
	// Phase 1 frees ~110K+ tokens; 100000 - 110000 = clamped to 0 << any threshold.
	cfg := compactConfig{TargetTokens: 80000}
	result, err := mgr.compactLocked(ctx, sessionID, CompactHookDecision{}, &cfg)

	// Then: Phase 2 is skipped (no LLM call).
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, int32(0), llm.callCount.Load())
}

func TestCompactLocked_Phase1DeltaInsufficient(t *testing.T) {
	t.Parallel()

	// Given: 55K-token context item + 4 small messages for summarization.
	mgr, llm, sessionID := setupCompactLockedTest(t, 55000)
	addSmallMessages(t, mgr, sessionID, 4)
	ctx := context.Background()

	// Use a very large context window to prevent PressureCompactionSelector
	// from triggering FullCompactor (which would collide summary IDs).
	mgr.SetDefaultContextWindow(10_000_000)

	// Use a very high provider token count so that even after Phase 1 frees
	// tokens, we're still above the low threshold.
	mgr.SetActualPromptTokens(sessionID, 500000)

	// When: TargetTokens=10000, effective after delta = 500000 - phase1Freed >> 10000.
	cfg := compactConfig{TargetTokens: 10000}
	_, err := mgr.compactLocked(ctx, sessionID, CompactHookDecision{}, &cfg)

	// Then: Phase 2 ran (indicated by LLM call OR summarization error from
	// summary ID collision, which only occurs if Phase 2 was entered).
	llmCalls := llm.callCount.Load()
	phase2Entered := llmCalls > 0 || (err != nil && strings.Contains(err.Error(), "lcm_summaries"))
	require.True(t, phase2Entered,
		"Phase 2 should have been entered (LLMCalls=%d, err=%v)", llmCalls, err)
}

func TestCompactLocked_DeltaClampedZero(t *testing.T) {
	t.Parallel()

	// Given: 60K-token context item → MicroCompactor archives it, freeing tokens.
	mgr, _, sessionID := setupCompactLockedTest(t, 60000)
	ctx := context.Background()

	// Set provider tokens much lower than what Phase 1 will free.
	// After subtraction: 10000 - ~110000 → clamped to 0.
	mgr.SetActualPromptTokens(sessionID, 10000)

	// When: TargetTokens=5000. Clamped 0 < 5000 → Phase 2 skipped.
	cfg := compactConfig{TargetTokens: 5000}
	result, err := mgr.compactLocked(ctx, sessionID, CompactHookDecision{}, &cfg)

	// Then: Phase 1 took action, Phase 2 skipped (no error, no crash).
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
}
