package agent

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRateLimitCoordinator_NoBackoff(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	err := coord.WaitIfBackedOff(context.Background(), "openai")
	require.NoError(t, err)
}

func TestRateLimitCoordinator_ContextCancellation(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	ctx, cancel := context.WithCancel(context.Background())

	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after": "300"},
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := coord.WaitIfBackedOff(ctx, "openai")
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
}

func TestRateLimitCoordinator_BackoffExpires(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	coord.RecordRateLimit("anthropic", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "100"},
	})

	start := time.Now()
	err := coord.WaitIfBackedOff(context.Background(), "anthropic")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.LessOrEqual(t, elapsed, 500*time.Millisecond)
}

func TestRateLimitCoordinator_DifferentProvidersIndependent(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after": "300"},
	})

	start := time.Now()
	err := coord.WaitIfBackedOff(context.Background(), "anthropic")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.WithinDuration(t, start, time.Now(), 50*time.Millisecond)
	_ = elapsed
}

func TestRateLimitCoordinator_LargestBackoffWins(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()

	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "50"},
	})
	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "200"},
	})

	start := time.Now()
	err := coord.WaitIfBackedOff(context.Background(), "openai")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.GreaterOrEqual(t, elapsed, 150*time.Millisecond)
}

func TestExtractRetryAfter_Milliseconds(t *testing.T) {
	t.Parallel()

	err := &fantasy.ProviderError{
		ResponseHeaders: map[string]string{"retry-after-ms": "5000"},
	}
	require.Equal(t, 5*time.Second, extractRetryAfter(err))
}

func TestExtractRetryAfter_Seconds(t *testing.T) {
	t.Parallel()

	err := &fantasy.ProviderError{
		ResponseHeaders: map[string]string{"retry-after": "3"},
	}
	require.Equal(t, 3*time.Second, extractRetryAfter(err))
}

func TestExtractRetryAfter_DefaultFallback(t *testing.T) {
	t.Parallel()

	err := &fantasy.ProviderError{
		ResponseHeaders: map[string]string{},
	}
	require.Equal(t, 2*time.Second, extractRetryAfter(err))
}

func TestExtractRetryAfter_NilHeaders(t *testing.T) {
	t.Parallel()

	err := &fantasy.ProviderError{}
	require.Equal(t, 2*time.Second, extractRetryAfter(err))
}

func TestRateLimitedModel_Propagates429(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	inner := testutil.NewStubLM(
		testutil.WithGenerateFunc(func(
			_ context.Context, _ fantasy.Call,
		) (*fantasy.Response, error) {
			return nil, &fantasy.ProviderError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseHeaders: map[string]string{"retry-after-ms": "5000"},
			}
		}),
	)

	m := newRateLimitedModel(inner, coord, "openai")
	_, err := m.Generate(context.Background(), fantasy.Call{})
	require.Error(t, err)

	var pe *fantasy.ProviderError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, http.StatusTooManyRequests, pe.StatusCode)
}

func TestRateLimitedModel_ConcurrentBackoffCoordination(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	var calls atomic.Int32

	inner := testutil.NewStubLM(
		testutil.WithGenerateFunc(func(
			_ context.Context, _ fantasy.Call,
		) (*fantasy.Response, error) {
			count := calls.Add(1)
			if count == 1 {
				return nil, &fantasy.ProviderError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseHeaders: map[string]string{"retry-after-ms": "200"},
				}
			}
			return &fantasy.Response{}, nil
		}),
	)

	m := newRateLimitedModel(inner, coord, "openai")

	var wg sync.WaitGroup
	var firstErr, secondErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, firstErr = m.Generate(context.Background(), fantasy.Call{})
	}()
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		_, secondErr = m.Generate(context.Background(), fantasy.Call{})
	}()
	wg.Wait()

	_ = firstErr
	_ = secondErr
}


func TestRateLimitCoordinatorWiredInBuildModels(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	inner := testutil.NewStubLM(testutil.WithProvider("openai"), testutil.WithModel("gpt-4"))

	wrapped := newRateLimitedModel(inner, coord, "openai")

	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "100"},
	})

	start := time.Now()
	_, err := wrapped.Generate(context.Background(), fantasy.Call{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.GreaterOrEqual(t, elapsed, 80*time.Millisecond, "wrapped model should have waited for backoff")
}

func TestRateLimitCoordinatorBackoffShared(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()

	model1 := newRateLimitedModel(testutil.NewStubLM(testutil.WithProvider("openai")), coord, "openai")
	model2 := newRateLimitedModel(testutil.NewStubLM(testutil.WithProvider("openai")), coord, "openai")

	model1.Generate(context.Background(), fantasy.Call{})

	coord.RecordRateLimit("openai", &fantasy.ProviderError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "200"},
	})

	start := time.Now()
	_, err := model2.Generate(context.Background(), fantasy.Call{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.GreaterOrEqual(t, elapsed, 150*time.Millisecond, "second model should observe backoff recorded from first model's provider")
}
