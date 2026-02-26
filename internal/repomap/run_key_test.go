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
