//go:build treesitter
// +build treesitter

package repomap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunKeyFromContext(t *testing.T) {
	t.Parallel()

	baseCtx := context.Background()
	_, ok := RunInjectionKeyFromContext(baseCtx)
	require.False(t, ok)

	ctx := WithRunInjectionKey(baseCtx, RunInjectionKey{
		RootUserMessageID: "msg-1",
		QueueGeneration:   2,
	})
	key, ok := RunInjectionKeyFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "msg-1", key.RootUserMessageID)
	require.EqualValues(t, 2, key.QueueGeneration)
}

func TestShouldInjectGuardOneInjectionPerRun(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	runKey := RunInjectionKey{
		RootUserMessageID: "root-user-msg",
		QueueGeneration:   0,
	}

	require.True(t, svc.ShouldInject("session-1", runKey))
	require.False(t, svc.ShouldInject("session-1", runKey))
}

func TestShouldInjectGuardQueueGenerationAllowsNextRun(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	first := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}
	second := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 1}

	require.True(t, svc.ShouldInject("session-1", first))
	require.True(t, svc.ShouldInject("session-1", second))
	require.False(t, svc.ShouldInject("session-1", second))
}

func TestShouldInjectGuardResetClearsRunState(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	runKey := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}

	require.True(t, svc.ShouldInject("session-1", runKey))
	require.False(t, svc.ShouldInject("session-1", runKey))

	require.NoError(t, svc.Reset(context.Background(), "session-1"))
	require.True(t, svc.ShouldInject("session-1", runKey))
}

func TestShouldInjectGuardConcurrentSingleWinner(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	runKey := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}

	const goroutines = 32
	var trueCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			if svc.ShouldInject("session-1", runKey) {
				trueCount.Add(1)
			}
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, trueCount.Load())
}

func TestClearInjectionReAllowsInjection(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	runKey := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}

	require.True(t, svc.ShouldInject("session-1", runKey))
	require.False(t, svc.ShouldInject("session-1", runKey))

	svc.ClearInjection("session-1", runKey)
	require.True(t, svc.ShouldInject("session-1", runKey))
	require.False(t, svc.ShouldInject("session-1", runKey))
}

func TestClearInjectionIsScopedToRunKey(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	first := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}
	second := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 1}

	require.True(t, svc.ShouldInject("session-1", first))
	require.True(t, svc.ShouldInject("session-1", second))

	svc.ClearInjection("session-1", first)
	require.True(t, svc.ShouldInject("session-1", first))
	require.False(t, svc.ShouldInject("session-1", second))
}

func TestClearInjectionNoopOnEmptyInputs(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())
	runKey := RunInjectionKey{RootUserMessageID: "root-user-msg", QueueGeneration: 0}

	require.True(t, svc.ShouldInject("session-1", runKey))

	svc.ClearInjection("", runKey)
	svc.ClearInjection("session-1", RunInjectionKey{})
	svc.ClearInjection("nonexistent", RunInjectionKey{RootUserMessageID: "other", QueueGeneration: 0})

	require.False(t, svc.ShouldInject("session-1", runKey))
}

func TestInjectionFlowWithDedup(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// First injection with msg-1 should succeed.
	key1 := RunInjectionKey{RootUserMessageID: "msg-1"}
	require.True(t, svc.ShouldInject("session-1", key1), "first inject should return true")

	// Same key again should be deduped.
	require.False(t, svc.ShouldInject("session-1", key1), "dedup: same key should return false")

	// New message ID (simulating a new run) should inject again.
	key2 := RunInjectionKey{RootUserMessageID: "msg-2"}
	require.True(t, svc.ShouldInject("session-1", key2), "new run key should return true")

	// ClearInjection allows re-injection for that key.
	svc.ClearInjection("session-1", key2)
	require.True(t, svc.ShouldInject("session-1", key2), "after clear, should re-inject")
	require.False(t, svc.ShouldInject("session-1", key2), "dedup after re-inject")
}

func TestShouldInjectReturnsFalseWithoutKey(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil, nil, ".", context.Background())

	// Empty RootUserMessageID should return false.
	require.False(t, svc.ShouldInject("session-1", RunInjectionKey{}), "empty RootUserMessageID should return false")

	// Empty sessionID should return false.
	require.False(t, svc.ShouldInject("", RunInjectionKey{RootUserMessageID: "msg-1"}), "empty sessionID should return false")
}
