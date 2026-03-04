package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CompactOnce_WithRealLLM(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	llm := testutil.NewLLMClient(t)
	mgr := NewManagerWithLLM(queries, sqlDB, llm).(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-integration-compact"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	// Insert realistic conversation messages.
	messages := []struct {
		role    string
		content string
	}{
		{"user", "I need to add a caching layer to the API endpoints. Currently every request hits the database directly and it's causing performance issues under load."},
		{"assistant", "I can help with that. Let me look at the current API structure. I see you have endpoints in internal/api/handlers.go that directly call the repository layer. I'll add a caching middleware using an in-memory cache with TTL-based expiration."},
		{"user", "Sounds good. Can you use Redis instead of in-memory? We need the cache to be shared across multiple instances."},
		{"assistant", "Good point for horizontal scaling. I've updated the implementation to use Redis. Changes: 1) Added internal/cache/redis.go with a Cache interface and Redis implementation. 2) Created internal/api/middleware/cache.go with response caching middleware. 3) Updated internal/api/router.go to apply the cache middleware to GET endpoints. The TTL is configurable via the config file."},
		{"user", "What about cache invalidation? We need to invalidate when data changes."},
		{"assistant", "I've added cache invalidation. When POST/PUT/DELETE requests succeed, the middleware invalidates related cache keys using a tag-based approach. Each resource type has a cache tag, and mutations clear all entries with that tag. Files modified: internal/cache/redis.go (added Tags and InvalidateByTag methods), internal/api/middleware/cache.go (added invalidation hooks)."},
		{"user", "Perfect. Can you add tests for the cache middleware?"},
	}

	for i, msg := range messages {
		msgID := fmt.Sprintf("int-msg-%d", i)
		createTestMessage(t, queries, sessionID, msgID, msg.role, msg.content)
		err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msgID, Valid: true},
			TokenCount: EstimateTokens(msg.content),
		})
		require.NoError(t, err)
	}

	tokensBefore, err := mgr.store.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Greater(t, tokensBefore, int64(0))

	// Force compaction with real LLM.
	result, err := mgr.CompactOnce(ctx, sessionID, true)
	require.NoError(t, err)
	require.True(t, result.ActionTaken, "compaction should have taken action")
	require.Equal(t, 1, result.Rounds)

	// Token count should have decreased.
	require.Less(t, result.TokenCount, tokensBefore,
		"token count should decrease after compaction")

	// Verify a summary was created in the context.
	entries, err := mgr.store.GetContextEntries(ctx, sessionID)
	require.NoError(t, err)

	var summaryEntry *ContextEntry
	for _, e := range entries {
		if e.ItemType == "summary" {
			summaryEntry = &e
			break
		}
	}
	require.NotNil(t, summaryEntry, "should have at least one summary after compaction")

	// Verify the summary is an actual LLM summary, not a truncated fallback.
	// Fallback just truncates raw message text; LLM summaries are structured.
	summary, err := queries.GetLcmSummary(ctx, summaryEntry.SummaryID)
	require.NoError(t, err)
	require.NotEmpty(t, summary.Content)

	// A real LLM summary should not start with the first message verbatim.
	require.False(t, strings.HasPrefix(summary.Content, messages[0].content),
		"summary should not be a verbatim copy of input messages")

	t.Logf("Compaction result: tokens %d -> %d", tokensBefore, result.TokenCount)
	t.Logf("Summary:\n%s", summary.Content)
}
