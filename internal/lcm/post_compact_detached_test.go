package lcm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPostCompactionHook_RunsWithDetachedContext verifies that PostCompactionHook
// completes when passed context.WithoutCancel(ctx) after the parent is canceled.
func TestPostCompactionHook_RunsWithDetachedContext(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	cm := mgr.(*compactionManager)
	ctx := context.Background()

	sessionID := "sess-detached-ctx"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	pc := &PreservedContext{
		SystemPromptContext: "system prompt to restore",
		ActiveFiles:         []string{"main.go"},
	}
	cm.PreserveContext(sessionID, pc)

	parentCtx, cancel := context.WithCancel(ctx)
	cancel()

	require.Error(t, parentCtx.Err())

	detachedCtx := context.WithoutCancel(parentCtx)

	mgr.PostCompactionHook(detachedCtx, sessionID)

	loaded := cm.preservedContextStore.Load(sessionID)
	require.Nil(t, loaded, "PostCompactionHook should have restored and cleared preserved context")
}

func TestPostCompactionHook_CanceledContextLogsWarnings(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-canceled-warn"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	mgr.PostCompactionHook(canceledCtx, sessionID)
}
