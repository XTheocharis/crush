package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCostTracker_RecordAndTotal(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(10.0)
	require.Equal(t, 0.0, ct.TotalCost())

	ct.RecordCost("gpt-4", 1000, 500, 0.05)
	require.InDelta(t, 0.05, ct.TotalCost(), 0.0001)

	ct.RecordCost("gpt-4", 2000, 1000, 0.10)
	require.InDelta(t, 0.15, ct.TotalCost(), 0.0001)

	ct.RecordCost("claude", 500, 200, 0.03)
	require.InDelta(t, 0.18, ct.TotalCost(), 0.0001)
}

func TestCostTracker_RecordZeroCostIgnored(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(10.0)
	ct.RecordCost("gpt-4", 100, 50, 0)
	ct.RecordCost("gpt-4", 100, 50, -1)
	require.Equal(t, 0.0, ct.TotalCost())
}

func TestCostTracker_RemainingBudget(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	require.InDelta(t, 1.0, ct.RemainingBudget(), 0.0001)

	ct.RecordCost("gpt-4", 100, 50, 0.6)
	require.InDelta(t, 0.4, ct.RemainingBudget(), 0.0001)
}

func TestCostTracker_RemainingBudgetUnlimited(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(0)
	require.Positive(t, ct.RemainingBudget())

	ct.RecordCost("gpt-4", 100, 50, 999.0)
	require.Positive(t, ct.RemainingBudget())
}

func TestCostTracker_RemainingBudgetClampedToZero(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(0.5)
	ct.RecordCost("gpt-4", 100, 50, 1.0)
	require.Equal(t, 0.0, ct.RemainingBudget())
}

func TestCostTracker_ShouldDowngrade(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	require.False(t, ct.ShouldDowngrade())

	ct.RecordCost("gpt-4", 100, 50, 0.79)
	require.False(t, ct.ShouldDowngrade())

	ct.RecordCost("gpt-4", 100, 50, 0.01)
	require.True(t, ct.ShouldDowngrade())
}

func TestCostTracker_ShouldDowngradeUnlimited(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(0)
	ct.RecordCost("gpt-4", 100, 50, 9999.0)
	require.False(t, ct.ShouldDowngrade())
}

func TestCostTracker_ForceLowTier(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	require.False(t, ct.ForceLowTier())

	ct.RecordCost("gpt-4", 100, 50, 0.94)
	require.False(t, ct.ForceLowTier())

	ct.RecordCost("gpt-4", 100, 50, 0.01)
	require.True(t, ct.ForceLowTier())
}

func TestCostTracker_ForceLowTierUnlimited(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(0)
	ct.RecordCost("gpt-4", 100, 50, 9999.0)
	require.False(t, ct.ForceLowTier())
}

func TestCostTracker_CostForModel(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(10.0)
	ct.RecordCost("gpt-4", 100, 50, 0.05)
	ct.RecordCost("gpt-4", 200, 100, 0.10)
	ct.RecordCost("claude", 100, 50, 0.03)

	require.InDelta(t, 0.15, ct.CostForModel("gpt-4"), 0.0001)
	require.InDelta(t, 0.03, ct.CostForModel("claude"), 0.0001)
	require.Equal(t, 0.0, ct.CostForModel("unknown"))
}

func TestCostTracker_Budget(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(5.0)
	require.Equal(t, 5.0, ct.Budget())
}

func TestCostTracker_ResolveWithCost_NoBudget(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(0)
	result := ct.ResolveWithCost(config.SelectedModelTypeLarge, config.SelectedModelTypeSmall)
	require.Equal(t, config.SelectedModelTypeLarge, result)
}

func TestCostTracker_ResolveWithCost_Downgrade(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	ct.RecordCost("gpt-4", 100, 50, 0.85)
	require.True(t, ct.ShouldDowngrade())
	require.False(t, ct.ForceLowTier())

	result := ct.ResolveWithCost(config.SelectedModelTypeLarge, config.SelectedModelTypeSmall)
	require.Equal(t, config.SelectedModelTypeSmall, result)
}

func TestCostTracker_ResolveWithCost_DowngradeAlreadyLowest(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	ct.RecordCost("gpt-4", 100, 50, 0.85)

	result := ct.ResolveWithCost(config.SelectedModelTypeSmall, config.SelectedModelTypeSmall)
	require.Equal(t, config.SelectedModelTypeSmall, result)
}

func TestCostTracker_ResolveWithCost_ForceLowTier(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(1.0)
	ct.RecordCost("gpt-4", 100, 50, 0.96)
	require.True(t, ct.ForceLowTier())

	result := ct.ResolveWithCost(config.SelectedModelTypeLarge, config.SelectedModelTypeSmall)
	require.Equal(t, config.SelectedModelTypeSmall, result)
}

func TestCostRouting_WithTierRouter(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge},
	})

	ct := NewCostTracker(1.0)
	r.SetCostTracker(ct)

	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(5000))
	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(500))

	ct.RecordCost("gpt-4", 100, 50, 0.85)
	require.True(t, ct.ShouldDowngrade())

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(5000))

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(500))
}

func TestCostRouting_ForceLowTierAll(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge},
	})

	ct := NewCostTracker(1.0)
	r.SetCostTracker(ct)

	ct.RecordCost("gpt-4", 100, 50, 0.96)
	require.True(t, ct.ForceLowTier())

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(5000))
	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(500))
}

func TestCostRouting_RouteForAgent(t *testing.T) {
	t.Parallel()

	r := NewTierRouterWithAgentTiers(
		[]config.RoutingTier{
			{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge},
		},
		map[string][]config.RoutingTier{
			"heavy": {
				{UpToTokens: 5000, ModelType: config.SelectedModelType("medium")},
				{UpToTokens: 50000, ModelType: config.SelectedModelType("ultra")},
			},
		},
	)

	ct := NewCostTracker(1.0)
	r.SetCostTracker(ct)

	require.Equal(t, config.SelectedModelType("ultra"), r.RouteForAgent("heavy", 10000))

	ct.RecordCost("gpt-4", 100, 50, 0.85)
	require.True(t, ct.ShouldDowngrade())

	require.Equal(t, config.SelectedModelTypeSmall, r.RouteForAgent("heavy", 10000))
}

func TestCostRouting_NoTrackerUnchanged(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(5000))
	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(500))
}

func TestCostRouting_ConcurrentRecord(t *testing.T) {
	t.Parallel()

	ct := NewCostTracker(100.0)

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			ct.RecordCost("gpt-4", 100, 50, 0.01)
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	require.InDelta(t, 1.0, ct.TotalCost(), 0.0001)
}
