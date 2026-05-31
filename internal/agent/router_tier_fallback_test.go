package agent

import (
	"context"
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFallback_429TriggersFallback(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		if model == "primary" {
			return &fantasy.ProviderError{
				Message:    "rate limited",
				StatusCode: 429,
			}
		}
		return nil
	}

	chain := []string{"primary", "fallback-a", "fallback-b"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestFallback_500TriggersFallback(t *testing.T) {
	t.Parallel()

	fn := func(model string) error {
		if model == "primary" {
			return &fantasy.ProviderError{
				Message:    "internal server error",
				StatusCode: 500,
			}
		}
		return nil
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
}

func TestFallback_400ReturnsImmediately(t *testing.T) {
	t.Parallel()

	callCount := 0
	badRequestErr := &fantasy.ProviderError{
		Message:    "invalid request",
		StatusCode: 400,
	}
	fn := func(model string) error {
		callCount++
		return badRequestErr
	}

	chain := []string{"primary", "fallback-a", "fallback-b"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.Error(t, err)
	require.Equal(t, 1, callCount)

	var providerErr *fantasy.ProviderError
	require.ErrorAs(t, err, &providerErr)
	require.Equal(t, 400, providerErr.StatusCode)
}

func TestFallback_ChainExhaustedReturnsLastError(t *testing.T) {
	t.Parallel()

	lastErr := &fantasy.ProviderError{
		Message:    "still rate limited",
		StatusCode: 429,
	}
	callCount := 0
	fn := func(model string) error {
		callCount++
		return lastErr
	}

	chain := []string{"primary", "fallback-a", "fallback-b"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.Error(t, err)
	require.Equal(t, 3, callCount)
	require.Equal(t, lastErr, err)
}

func TestFallback_MaxThreeAttempts(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		return &fantasy.ProviderError{
			Message:    "rate limited",
			StatusCode: 429,
		}
	}

	// Chain has 5 models but maxFallbackAttempts is 3.
	chain := []string{"m1", "m2", "m3", "m4", "m5"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.Error(t, err)
	require.Equal(t, 3, callCount)
}

func TestFallback_EmptyChainReturnsError(t *testing.T) {
	t.Parallel()

	err := ExecuteWithFallback(context.Background(), func(model string) error {
		return nil
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty fallback chain")
}

func TestFallback_TimeoutTriggersFallback(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		if model == "primary" {
			return context.DeadlineExceeded
		}
		return nil
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestFallback_CancelledContextNotRetried(t *testing.T) {
	t.Parallel()

	callCount := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn := func(model string) error {
		callCount++
		return ctx.Err()
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(ctx, fn, chain)
	require.Error(t, err)
	require.Equal(t, 1, callCount)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFallback_GenericErrorNotRetried(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		return fmt.Errorf("some random error")
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.Error(t, err)
	require.Equal(t, 1, callCount)
}

func TestFallback_FirstModelSucceeds(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		return nil
	}

	chain := []string{"primary", "fallback-a", "fallback-b"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
	require.Equal(t, 1, callCount)
}

func TestFallback_502TriggersFallback(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		if callCount < 2 {
			return &fantasy.ProviderError{
				Message:    "bad gateway",
				StatusCode: 502,
			}
		}
		return nil
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestFallback_408TriggersFallback(t *testing.T) {
	t.Parallel()

	fn := func(model string) error {
		if model == "primary" {
			return &fantasy.ProviderError{
				Message:    "request timeout",
				StatusCode: 408,
			}
		}
		return nil
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
}

func TestIsRetryableProviderError_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name: "429 is retryable",
			err: &fantasy.ProviderError{
				StatusCode: 429,
				Message:    "rate limited",
			},
			retryable: true,
		},
		{
			name: "500 is retryable",
			err: &fantasy.ProviderError{
				StatusCode: 500,
				Message:    "internal server error",
			},
			retryable: true,
		},
		{
			name: "502 is retryable",
			err: &fantasy.ProviderError{
				StatusCode: 502,
				Message:    "bad gateway",
			},
			retryable: true,
		},
		{
			name: "503 is retryable",
			err: &fantasy.ProviderError{
				StatusCode: 503,
				Message:    "service unavailable",
			},
			retryable: true,
		},
		{
			name: "400 is not retryable",
			err: &fantasy.ProviderError{
				StatusCode: 400,
				Message:    "bad request",
			},
			retryable: false,
		},
		{
			name: "401 is not retryable",
			err: &fantasy.ProviderError{
				StatusCode: 401,
				Message:    "unauthorized",
			},
			retryable: false,
		},
		{
			name: "403 is not retryable",
			err: &fantasy.ProviderError{
				StatusCode: 403,
				Message:    "forbidden",
			},
			retryable: false,
		},
		{
			name:      "deadline exceeded is retryable",
			err:       context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "cancelled context is not retryable",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "generic error is not retryable",
			err:       fmt.Errorf("something broke"),
			retryable: false,
		},
		{
			name:      "nil error is not retryable",
			err:       nil,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isRetryableProviderError(tt.err)
			require.Equal(t, tt.retryable, result)
		})
	}
}

func TestFallbackChainForTokenCount(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall, FallbackChain: []string{"small-fallback"}},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeLarge, FallbackChain: []string{"large-fallback-1", "large-fallback-2"}},
	})

	// Below first tier.
	require.Equal(t, []string{"small-fallback"}, r.FallbackChainForTokenCount(500))
	// At first tier boundary.
	require.Equal(t, []string{"small-fallback"}, r.FallbackChainForTokenCount(1000))
	// Between tiers — matches second tier.
	require.Equal(t, []string{"large-fallback-1", "large-fallback-2"}, r.FallbackChainForTokenCount(2500))
	// Above all tiers — last tier's chain.
	require.Equal(t, []string{"large-fallback-1", "large-fallback-2"}, r.FallbackChainForTokenCount(10000))
}

func TestFallbackChainForTokenCount_NoChainConfigured(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Nil(t, r.FallbackChainForTokenCount(500))
	require.Nil(t, r.FallbackChainForTokenCount(5000))
}

func TestFallbackChainForTokenCount_EmptyTiers(t *testing.T) {
	t.Parallel()

	r := NewTierRouter(nil)
	require.Nil(t, r.FallbackChainForTokenCount(500))
}

func TestFallbackChainForAgent(t *testing.T) {
	t.Parallel()

	globalTiers := []config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall, FallbackChain: []string{"global-small"}},
		{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge, FallbackChain: []string{"global-large"}},
	}

	agentTiers := map[string][]config.RoutingTier{
		"heavy": {
			{UpToTokens: 5000, ModelType: config.SelectedModelType("medium"), FallbackChain: []string{"heavy-medium"}},
			{UpToTokens: 50000, ModelType: config.SelectedModelType("ultra"), FallbackChain: []string{"heavy-ultra"}},
		},
	}

	r := NewTierRouterWithAgentTiers(globalTiers, agentTiers)

	// Agent-specific tiers.
	require.Equal(t, []string{"heavy-medium"}, r.FallbackChainForAgent("heavy", 3000))
	require.Equal(t, []string{"heavy-ultra"}, r.FallbackChainForAgent("heavy", 10000))

	// Unknown agent falls back to global.
	require.Equal(t, []string{"global-small"}, r.FallbackChainForAgent("unknown", 500))

	// Empty agent name falls back to global.
	require.Equal(t, []string{"global-small"}, r.FallbackChainForAgent("", 500))
}

func TestFallback_WrappedProviderError(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		if model == "primary" {
			return fmt.Errorf("wrapped: %w", &fantasy.ProviderError{
				Message:    "rate limited",
				StatusCode: 429,
			})
		}
		return nil
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}

func TestFallback_Wrapped400NotRetried(t *testing.T) {
	t.Parallel()

	callCount := 0
	fn := func(model string) error {
		callCount++
		return fmt.Errorf("wrapped: %w", &fantasy.ProviderError{
			Message:    "bad request",
			StatusCode: 400,
		})
	}

	chain := []string{"primary", "fallback"}
	err := ExecuteWithFallback(context.Background(), fn, chain)
	require.Error(t, err)
	require.Equal(t, 1, callCount)
}

func TestIsRetryableProviderError_WrappedError(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("outer: %w", &fantasy.ProviderError{
		StatusCode: 429,
		Message:    "rate limited",
	})
	require.True(t, isRetryableProviderError(wrapped))

	wrapped400 := fmt.Errorf("outer: %w", &fantasy.ProviderError{
		StatusCode: 400,
		Message:    "bad request",
	})
	require.False(t, isRetryableProviderError(wrapped400))
}

func TestFallback_IntegrationWithTierRouter(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:   1000,
			ModelType:    config.SelectedModelTypeSmall,
			FallbackChain: []string{"small-primary", "small-backup"},
		},
		{
			UpToTokens:   10000,
			ModelType:    config.SelectedModelTypeLarge,
			FallbackChain: []string{"large-primary", "large-backup"},
		},
	})

	// Resolve tier and use its fallback chain.
	modelType := r.Resolve(500)
	require.Equal(t, config.SelectedModelTypeSmall, modelType)

	chain := r.FallbackChainForTokenCount(500)
	require.Equal(t, []string{"small-primary", "small-backup"}, chain)

	callCount := 0
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		callCount++
		if model == "small-primary" {
			return &fantasy.ProviderError{StatusCode: 429, Message: "rate limited"}
		}
		return nil
	}, chain)
	require.NoError(t, err)
	require.Equal(t, 2, callCount)
}
