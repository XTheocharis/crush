package lcm

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderTokensOverrideContextCount(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-provider"
	createTestSession(t, queries, sessionID)

	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	// Without provider tokens, GetContextTokenCount returns the
	// lcm_context_items sum (0 for a fresh session).
	count, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	// Simulate a provider reporting 150K prompt tokens.
	mgr.SetActualPromptTokens(sessionID, 150_000)

	count, err = mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(150_000), count)

	// A new message adds ~500 estimated tokens.
	mgr.AddPendingItemTokens(sessionID, 500)

	count, err = mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(150_500), count)

	// Next provider report resets the delta.
	mgr.SetActualPromptTokens(sessionID, 155_000)

	count, err = mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, int64(155_000), count)
}

func TestProviderTokensConcurrentPendingAdds(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-concurrent"
	createTestSession(t, queries, sessionID)

	err := mgr.InitSession(ctx, sessionID)
	require.NoError(t, err)

	mgr.SetActualPromptTokens(sessionID, 100_000)

	// Concurrently add pending tokens from multiple goroutines.
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.AddPendingItemTokens(sessionID, int64(i+1))
		}()
	}
	wg.Wait()

	count, err := mgr.GetContextTokenCount(ctx, sessionID)
	require.NoError(t, err)

	// Expected: 100_000 + sum(1..100) = 100_000 + 5050 = 105_050
	require.Equal(t, int64(105_050), count)
}
