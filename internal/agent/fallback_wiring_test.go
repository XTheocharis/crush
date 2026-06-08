package agent

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFallback_RunWithFallback_NoTierRouter(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
		calls.Add(1)
		return agentResultWithText("ok"), nil
	})

	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
	}

	result, err := c.runWithFallback(
		context.Background(), agent, agent.model, config.ProviderConfig{},
		"session-1", "hello", nil, nil, 4096,
		nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), calls.Load())
}

func TestFallback_RunWithFallback_429RetriesWithFallback(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
		n := calls.Add(1)
		if n == 1 {
			return nil, &fantasy.ProviderError{
				Message:    "rate limited",
				StatusCode: http.StatusTooManyRequests,
			}
		}
		return agentResultWithText("fallback-ok"), nil
	})

	router := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    100000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"fallback-model-a"},
		},
	})

	ct := NewCostTracker(DefaultCostBudget)
	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
		tierRouter:     router,
		costTracker:    ct,
		metricsStore:   NewMetricsStore(),
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 3, CostPer1MOut: 15, DefaultMaxTokens: 4096},
		ModelCfg:   config.SelectedModel{Model: "primary-model", Provider: "test-provider"},
	}

	result, err := c.runWithFallback(
		context.Background(), agent, model, config.ProviderConfig{},
		"session-1", "hello", nil, nil, 4096,
		nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(2), calls.Load(), "should have retried after 429")
}

func TestFallback_RunWithFallback_500FallsBack(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
		n := calls.Add(1)
		if n == 1 {
			return nil, &fantasy.ProviderError{
				Message:    "internal server error",
				StatusCode: http.StatusInternalServerError,
			}
		}
		return agentResultWithText("recovered"), nil
	})

	router := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    100000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"fallback-model"},
		},
	})

	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
		tierRouter:     router,
		costTracker:    NewCostTracker(DefaultCostBudget),
		metricsStore:   NewMetricsStore(),
	}

	result, err := c.runWithFallback(
		context.Background(), agent, modelWithProvider("primary", "test-provider"),
		config.ProviderConfig{}, "s1", "prompt", nil, nil, 4096,
		nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(2), calls.Load())
}

func TestFallback_RunWithFallback_400ReturnsImmediately(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
		calls.Add(1)
		return nil, &fantasy.ProviderError{
			Message:    "bad request",
			StatusCode: http.StatusBadRequest,
		}
	})

	router := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    100000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"fallback-a", "fallback-b"},
		},
	})

	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
		tierRouter:     router,
		costTracker:    NewCostTracker(DefaultCostBudget),
		metricsStore:   NewMetricsStore(),
	}

	result, err := c.runWithFallback(
		context.Background(), agent, modelWithProvider("primary", "test-provider"),
		config.ProviderConfig{}, "s1", "prompt", nil, nil, 4096,
		nil, nil, nil, nil, nil,
	)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, int32(1), calls.Load(), "400 should not trigger fallback")
}

func TestFallback_RunWithFallback_NonInteractiveSubAgent(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		calls.Add(1)
		require.True(t, call.NonInteractive, "sub-agent call must be non-interactive")
		return agentResultWithText("ok"), nil
	})

	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
	}

	result, err := c.runWithFallback(
		context.Background(), agent, agent.model, config.ProviderConfig{},
		"session-1", "task", nil, nil, 4096,
		nil, nil, nil, nil, nil, true,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestFallback_ResolveChain_NilWhenNoRouter(t *testing.T) {
	t.Parallel()

	c := &coordinator{}
	require.Nil(t, c.resolveFallbackChain("model-a"))
}

func TestFallback_ResolveChain_EmptyWhenNoFallbackConfigured(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 100000, ModelType: config.SelectedModelTypeLarge},
	})
	c := &coordinator{tierRouter: router}
	require.Nil(t, c.resolveFallbackChain("model-a"))
}

func TestFallback_ResolveChain_PrependsPrimaryModel(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    100000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"fallback-a", "fallback-b"},
		},
	})
	c := &coordinator{tierRouter: router}
	chain := c.resolveFallbackChain("primary-model")
	require.Equal(t, []string{"primary-model", "fallback-a", "fallback-b"}, chain)
}

func TestFallback_RecordRateLimitIf429(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	c := &coordinator{rateLimitCoord: coord}

	err := &fantasy.ProviderError{
		Message:         "rate limited",
		StatusCode:      http.StatusTooManyRequests,
		ResponseHeaders: map[string]string{"retry-after-ms": "1000"},
	}
	c.recordRateLimitIf429("test-provider", err)

	ctx := context.Background()
	require.NoError(t, coord.WaitIfBackedOff(ctx, "test-provider"))
}

func TestFallback_RecordRateLimitIf429_NonRateLimitError(t *testing.T) {
	t.Parallel()

	coord := NewRateLimitCoordinator()
	c := &coordinator{rateLimitCoord: coord}

	c.recordRateLimitIf429("test-provider", &fantasy.ProviderError{
		Message:    "bad request",
		StatusCode: http.StatusBadRequest,
	})

	ctx := context.Background()
	require.NoError(t, coord.WaitIfBackedOff(ctx, "test-provider"))
}

func TestFallback_WithTierRouterOption(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 100000, ModelType: config.SelectedModelTypeLarge},
	})
	opt := WithTierRouter(router)

	c := &coordinator{}
	opt(c)
	require.NotNil(t, c.tierRouter)
}

func TestFallback_CostRecordedDuringRetry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	agent := newMockAgent("test-provider", 4096, func(_ context.Context, _ SessionAgentCall) (*fantasy.AgentResult, error) {
		n := calls.Add(1)
		if n == 1 {
			return &fantasy.AgentResult{
				TotalUsage: fantasy.Usage{InputTokens: 100, OutputTokens: 0},
			}, &fantasy.ProviderError{
				Message:    "rate limited",
				StatusCode: http.StatusTooManyRequests,
			}
		}
		return &fantasy.AgentResult{
			TotalUsage: fantasy.Usage{InputTokens: 200, OutputTokens: 50},
			Response:   fantasy.Response{Content: fantasy.ResponseContent{fantasy.TextContent{Text: "ok"}}},
		}, nil
	})

	router := NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    100000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"fallback"},
		},
	})

	ct := NewCostTracker(DefaultCostBudget)
	store := NewMetricsStore()
	c := &coordinator{
		rateLimitCoord: NewRateLimitCoordinator(),
		tierRouter:     router,
		costTracker:    ct,
		metricsStore:   store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 3, CostPer1MOut: 15, DefaultMaxTokens: 4096},
		ModelCfg:   config.SelectedModel{Model: "primary", Provider: "test-provider"},
	}

	result, err := c.runWithFallback(
		context.Background(), agent, model, config.ProviderConfig{},
		"s1", "prompt", nil, nil, 4096,
		nil, nil, nil, nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	c.recordCostFromResult(model, result, err)
	require.InDelta(t, 3.0/1e6*200+15.0/1e6*50, ct.TotalCost(), 0.0001)
}

func modelWithProvider(modelID, provider string) Model {
	return Model{
		CatwalkCfg: catwalk.Model{DefaultMaxTokens: 4096},
		ModelCfg:   config.SelectedModel{Model: modelID, Provider: provider},
	}
}
