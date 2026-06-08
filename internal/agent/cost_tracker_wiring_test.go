package agent

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCostTracker_ComputeUsageCost(t *testing.T) {
	t.Parallel()

	model := Model{
		CatwalkCfg: catwalk.Model{
			CostPer1MIn:  10,
			CostPer1MOut: 20,
		},
		ModelCfg: config.SelectedModel{Model: "gpt-4"},
	}

	usage := fantasy.Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	cost := computeUsageCost(model, usage)
	expected := 10.0/1e6*1000 + 20.0/1e6*500
	require.InDelta(t, expected, cost, 0.0001)
}

func TestCostTracker_ComputeUsageCostWithCache(t *testing.T) {
	t.Parallel()

	model := Model{
		CatwalkCfg: catwalk.Model{
			CostPer1MIn:        10,
			CostPer1MOut:       20,
			CostPer1MInCached:  5,
			CostPer1MOutCached: 10,
		},
		ModelCfg: config.SelectedModel{Model: "claude"},
	}

	usage := fantasy.Usage{
		InputTokens:         1000,
		OutputTokens:        500,
		CacheCreationTokens: 200,
		CacheReadTokens:     300,
	}

	cost := computeUsageCost(model, usage)
	expected := 5.0/1e6*200 + 10.0/1e6*300 + 10.0/1e6*1000 + 20.0/1e6*500
	require.InDelta(t, expected, cost, 0.0001)
}

func TestCostTracker_RecordCostFromResult(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{
			CostPer1MIn:  3,
			CostPer1MOut: 15,
		},
		ModelCfg: config.SelectedModel{Model: "claude-sonnet-4"},
	}

	result := &fantasy.AgentResult{
		TotalUsage: fantasy.Usage{
			InputTokens:  1000,
			OutputTokens: 500,
		},
		Response: fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: "hello"},
			},
		},
	}

	c.recordCostFromResult(model, result, nil)

	expectedCost := 3.0/1e6*1000 + 15.0/1e6*500
	require.InDelta(t, expectedCost, ct.TotalCost(), 0.0001)
	require.InDelta(t, expectedCost, ct.CostForModel("claude-sonnet-4"), 0.0001)

	m := store.Get("claude-sonnet-4")
	require.NotNil(t, m)
	require.Equal(t, int64(1), m.RequestCount())
	require.Equal(t, int64(1), m.SuccessCount())
	require.Equal(t, int64(1000), m.TotalInputTokens())
	require.Equal(t, int64(500), m.TotalOutputTokens())
	require.InDelta(t, expectedCost, m.TotalCost(), 0.0001)
}

func TestCostTracker_RecordCostFromResult_NilResult(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{ModelCfg: config.SelectedModel{Model: "test"}}
	c.recordCostFromResult(model, nil, nil)

	require.Equal(t, 0.0, ct.TotalCost())
	require.Empty(t, store.GetAll())
}

func TestCostTracker_RecordCostFromResult_FailedRequest(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 3, CostPer1MOut: 15},
		ModelCfg:   config.SelectedModel{Model: "gpt-4"},
	}

	result := &fantasy.AgentResult{
		TotalUsage: fantasy.Usage{InputTokens: 100, OutputTokens: 50},
	}

	c.recordCostFromResult(model, result, errModelProviderNotConfigured)

	m := store.Get("gpt-4")
	require.NotNil(t, m)
	require.Equal(t, int64(1), m.FailureCount())
	require.Equal(t, int64(0), m.SuccessCount())
}

func TestCostTracker_RecordCostFromResult_FlatRate(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 3, CostPer1MOut: 15},
		ModelCfg:   config.SelectedModel{Model: "flat-model"},
		FlatRate:   true,
	}

	result := &fantasy.AgentResult{
		TotalUsage: fantasy.Usage{InputTokens: 1000, OutputTokens: 500},
	}

	c.recordCostFromResult(model, result, nil)

	require.Equal(t, 0.0, ct.TotalCost())
	m := store.Get("flat-model")
	require.NotNil(t, m)
	require.Equal(t, 0.0, m.TotalCost())
}

func TestCostTracker_WithCostTrackerOption(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(42.0)
	opt := WithCostTracker(ct)

	c := &coordinator{}
	opt(c)

	require.Equal(t, 42.0, c.costTracker.Budget())
}

func TestCostTracker_WithMetricsStoreOption(t *testing.T) {
	t.Parallel()

	store := NewMetricsStore()
	opt := WithMetricsStore(store)

	c := &coordinator{}
	opt(c)

	require.NotNil(t, c.metricsStore)
}

func TestCostTracker_MultipleResponsesAccumulate(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 10, CostPer1MOut: 20},
		ModelCfg:   config.SelectedModel{Model: "gpt-4"},
	}

	for i := 0; i < 5; i++ {
		result := &fantasy.AgentResult{
			TotalUsage: fantasy.Usage{InputTokens: 100, OutputTokens: 50},
		}
		c.recordCostFromResult(model, result, nil)
	}

	stepCost := 10.0/1e6*100 + 20.0/1e6*50
	require.InDelta(t, stepCost*5, ct.TotalCost(), 0.0001)
	require.InDelta(t, stepCost*5, ct.CostForModel("gpt-4"), 0.0001)

	m := store.Get("gpt-4")
	require.Equal(t, int64(5), m.RequestCount())
	require.Equal(t, int64(5), m.SuccessCount())
}

func TestCostTracker_ComputeUsageCost_ZeroPricing(t *testing.T) {
	t.Parallel()

	model := Model{
		CatwalkCfg: catwalk.Model{},
		ModelCfg:   config.SelectedModel{Model: "free-model"},
	}

	usage := fantasy.Usage{InputTokens: 1000, OutputTokens: 500}
	cost := computeUsageCost(model, usage)
	require.Equal(t, 0.0, cost)
}

func TestCostTracker_DowngradeAfterCosts(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	store := NewMetricsStore()

	c := &coordinator{
		costTracker:  ct,
		metricsStore: store,
	}

	model := Model{
		CatwalkCfg: catwalk.Model{CostPer1MIn: 10000, CostPer1MOut: 20000},
		ModelCfg:   config.SelectedModel{Model: "expensive"},
	}

	result := &fantasy.AgentResult{
		TotalUsage: fantasy.Usage{InputTokens: 100, OutputTokens: 50},
	}
	c.recordCostFromResult(model, result, nil)

	require.True(t, ct.ShouldDowngrade(),
		"expected downgrade after recording cost %.4f with budget 1.0", ct.TotalCost())
}
