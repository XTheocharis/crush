package wave5_test

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test 1: TierRouter loads tier definitions and resolves correctly
// ---------------------------------------------------------------------------

func TestTierRouter_LoadsTiers(t *testing.T) {
	t.Parallel()

	tiers := []config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge},
	}
	router := agent.NewTierRouter(tiers)

	// Within first tier → small.
	require.Equal(t, config.SelectedModelTypeSmall, router.Resolve(1000))
	require.Equal(t, config.SelectedModelTypeSmall, router.Resolve(4000))

	// Within second tier → large.
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(5000))
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(16000))

	// Beyond all tiers → fallback to largest tier.
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(99999))
}

func TestTierRouter_EmptyTiers_DefaultsToLarge(t *testing.T) {
	t.Parallel()

	router := agent.NewTierRouter(nil)
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(0))
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(100000))
}

func TestTierRouter_FromThreshold(t *testing.T) {
	t.Parallel()

	router := agent.NewTierRouterFromThreshold(4000)
	require.Equal(t, config.SelectedModelTypeSmall, router.Resolve(4000))
	require.Equal(t, config.SelectedModelTypeLarge, router.Resolve(4001))
}

func TestTierRouter_TiersReturnsSortedCopy(t *testing.T) {
	t.Parallel()

	tiers := []config.RoutingTier{
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
	}
	router := agent.NewTierRouter(tiers)
	got := router.Tiers()

	// Should be sorted ascending by UpToTokens.
	require.Equal(t, 4000, got[0].UpToTokens)
	require.Equal(t, 16000, got[1].UpToTokens)

	// Mutating the returned slice should not affect the router.
	got[0].UpToTokens = 9999
	require.Equal(t, 4000, router.Tiers()[0].UpToTokens)
}

func TestTierRouter_AgentSpecificTiers(t *testing.T) {
	t.Parallel()

	global := []config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge},
	}
	agentTiers := map[string][]config.RoutingTier{
		"task": {
			{UpToTokens: 2000, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: 8000, ModelType: config.SelectedModelTypeLarge},
		},
	}
	router := agent.NewTierRouterWithAgentTiers(global, agentTiers)

	// "task" agent uses its own tiers.
	require.Equal(t, config.SelectedModelTypeSmall, router.RouteForAgent("task", 2000))
	require.Equal(t, config.SelectedModelTypeLarge, router.RouteForAgent("task", 2001))

	// Unknown agent falls back to global.
	require.Equal(t, config.SelectedModelTypeSmall, router.RouteForAgent("unknown", 3000))
	require.Equal(t, config.SelectedModelTypeLarge, router.RouteForAgent("unknown", 5000))

	// Empty agent name uses global.
	require.Equal(t, config.SelectedModelTypeSmall, router.RouteForAgent("", 3000))
}

func TestTierRouter_ResolveByCharCount(t *testing.T) {
	t.Parallel()

	router := agent.NewTierRouterFromThreshold(4000) // threshold = 4000 tokens

	// 16000 chars / 4 chars-per-token = 4000 tokens → exactly at threshold → small.
	require.Equal(t, config.SelectedModelTypeSmall, router.ResolveByCharCount(16000))

	// 16004 chars / 4 = 4001 tokens → above threshold → large.
	require.Equal(t, config.SelectedModelTypeLarge, router.ResolveByCharCount(16004))
}

// ---------------------------------------------------------------------------
// Test 2: CostTracker tracks cumulative costs
// ---------------------------------------------------------------------------

func TestCostTracker_TracksCumulativeCosts(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(10.0)

	require.InDelta(t, 0.0, ct.TotalCost(), 0.001)
	require.InDelta(t, 10.0, ct.RemainingBudget(), 0.001)

	ct.RecordCost("model-a", 1000, 500, 1.5)
	require.InDelta(t, 1.5, ct.TotalCost(), 0.001)
	require.InDelta(t, 8.5, ct.RemainingBudget(), 0.001)

	ct.RecordCost("model-b", 2000, 1000, 3.0)
	require.InDelta(t, 4.5, ct.TotalCost(), 0.001)
	require.InDelta(t, 5.5, ct.RemainingBudget(), 0.001)
}

func TestCostTracker_PerModelTracking(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(100.0)
	ct.RecordCost("model-a", 100, 50, 2.0)
	ct.RecordCost("model-b", 200, 100, 3.0)
	ct.RecordCost("model-a", 100, 50, 1.5)

	require.InDelta(t, 3.5, ct.CostForModel("model-a"), 0.001)
	require.InDelta(t, 3.0, ct.CostForModel("model-b"), 0.001)
	require.InDelta(t, 0.0, ct.CostForModel("unknown"), 0.001)
}

func TestCostTracker_IgnoresNonPositiveCost(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(100.0)
	ct.RecordCost("model-a", 100, 50, 0)
	ct.RecordCost("model-b", 100, 50, -1.0)
	require.InDelta(t, 0.0, ct.TotalCost(), 0.001)
}

func TestCostTracker_DowngradeThreshold(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(10.0)

	// Below 80% → no downgrade.
	ct.RecordCost("m", 100, 50, 7.9)
	require.False(t, ct.ShouldDowngrade())

	// At 80% → downgrade.
	ct.RecordCost("m", 100, 50, 0.1)
	require.True(t, ct.ShouldDowngrade())
}

func TestCostTracker_ForceLowTierThreshold(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(10.0)

	ct.RecordCost("m", 100, 50, 9.4)
	require.False(t, ct.ForceLowTier())

	ct.RecordCost("m", 100, 50, 0.1)
	require.True(t, ct.ForceLowTier())
}

func TestCostTracker_UnlimitedBudget(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(0) // unlimited

	ct.RecordCost("m", 100, 50, 999999.0)
	require.False(t, ct.ShouldDowngrade())
	require.False(t, ct.ForceLowTier())
	require.InDelta(t, 999999.0, ct.TotalCost(), 0.001)
}

func TestCostTracker_ResolveWithCost(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(10.0)
	lowest := config.SelectedModelTypeSmall

	// Under thresholds → returns base.
	require.Equal(t, config.SelectedModelTypeLarge,
		ct.ResolveWithCost(config.SelectedModelTypeLarge, lowest))

	// Force 95% threshold.
	ct.RecordCost("m", 100, 50, 9.5)
	require.Equal(t, config.SelectedModelTypeSmall,
		ct.ResolveWithCost(config.SelectedModelTypeLarge, lowest))

	// When base == lowest, downgrade returns base (no-op).
	ct2 := agent.NewCostTracker(10.0)
	ct2.RecordCost("m", 100, 50, 8.5)
	require.Equal(t, config.SelectedModelTypeSmall,
		ct2.ResolveWithCost(config.SelectedModelTypeSmall, config.SelectedModelTypeSmall))
}

func TestCostTracker_RemainingBudgetNeverNegative(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(5.0)
	ct.RecordCost("m", 100, 50, 10.0)
	require.InDelta(t, 0.0, ct.RemainingBudget(), 0.001)
}

func TestCostTracker_BudgetReturnsConfiguredValue(t *testing.T) {
	t.Parallel()

	ct := agent.NewCostTracker(42.0)
	require.InDelta(t, 42.0, ct.Budget(), 0.001)
}

// ---------------------------------------------------------------------------
// Test 3: ModelMetrics records latency and token counts
// ---------------------------------------------------------------------------

func TestModelMetrics_RecordsLatencyAndTokens(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()

	store.Record("gpt-4o", 500*time.Millisecond, true, 1000, 500, 0.03)
	store.Record("gpt-4o", 300*time.Millisecond, true, 800, 400, 0.02)
	store.Record("gpt-4o", 200*time.Millisecond, false, 200, 0, 0.00)

	m := store.Get("gpt-4o")
	require.NotNil(t, m)
	require.Equal(t, int64(3), m.RequestCount())
	require.Equal(t, int64(2), m.SuccessCount())
	require.Equal(t, int64(1), m.FailureCount())
	require.Equal(t, 1000*time.Millisecond, m.TotalLatency())
	require.Equal(t, int64(2000), m.TotalInputTokens())
	require.Equal(t, int64(900), m.TotalOutputTokens())
	require.InDelta(t, 0.05, m.TotalCost(), 0.001)
}

func TestModelMetrics_SuccessRate(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	store.Record("m", 100*time.Millisecond, true, 10, 10, 0)
	store.Record("m", 100*time.Millisecond, true, 10, 10, 0)
	store.Record("m", 100*time.Millisecond, false, 10, 10, 0)

	m := store.Get("m")
	require.InDelta(t, 2.0/3.0, m.SuccessRate(), 0.001)
}

func TestModelMetrics_AvgLatency(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	store.Record("m", 200*time.Millisecond, true, 0, 0, 0)
	store.Record("m", 400*time.Millisecond, true, 0, 0, 0)

	m := store.Get("m")
	require.Equal(t, 300*time.Millisecond, m.AvgLatency())
}

func TestModelMetrics_TokenEfficiency(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	store.Record("m", 100*time.Millisecond, true, 1000, 1000, 0)

	m := store.Get("m")
	require.InDelta(t, 0.5, m.TokenEfficiency(), 0.001)
}

func TestModelMetrics_ZeroRequestEdgeCases(t *testing.T) {
	t.Parallel()

	m := &agent.ModelMetrics{}
	require.Equal(t, float64(0), m.SuccessRate())
	require.Equal(t, time.Duration(0), m.AvgLatency())
	require.Equal(t, float64(0), m.TokenEfficiency())
}

func TestModelMetrics_IgnoresEmptyModel(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	store.Record("", 100*time.Millisecond, true, 10, 10, 0)
	require.Nil(t, store.Get(""))
}

func TestModelMetrics_GetAll(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	store.Record("a", 100*time.Millisecond, true, 0, 0, 0)
	store.Record("b", 200*time.Millisecond, true, 0, 0, 0)

	all := store.GetAll()
	require.Len(t, all, 2)
	require.Contains(t, all, "a")
	require.Contains(t, all, "b")
}

func TestModelMetrics_GetMissingReturnsNil(t *testing.T) {
	t.Parallel()

	store := agent.NewMetricsStore()
	require.Nil(t, store.Get("nonexistent"))
}

// ---------------------------------------------------------------------------
// Test 4: FallbackChain selects fallback model when primary fails
// ---------------------------------------------------------------------------

func TestFallback_PrimarySucceeds(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		return nil
	}

	err := agent.ExecuteWithFallback(context.Background(), fn, []string{"primary", "fallback"})
	require.NoError(t, err)
	require.Equal(t, []string{"primary"}, called)
}

func TestFallback_429TriggersFallback(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		if model == "primary" {
			return &fantasy.ProviderError{Message: "rate limited", StatusCode: 429}
		}
		return nil
	}

	err := agent.ExecuteWithFallback(context.Background(), fn, []string{"primary", "fallback"})
	require.NoError(t, err)
	require.Equal(t, []string{"primary", "fallback"}, called)
}

func TestFallback_500TriggersFallback(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		if model == "primary" {
			return &fantasy.ProviderError{Message: "server error", StatusCode: 500}
		}
		return nil
	}

	err := agent.ExecuteWithFallback(context.Background(), fn, []string{"primary", "fallback"})
	require.NoError(t, err)
	require.Equal(t, []string{"primary", "fallback"}, called)
}

func TestFallback_400DoesNotTriggerFallback(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		return &fantasy.ProviderError{Message: "bad request", StatusCode: 400}
	}

	err := agent.ExecuteWithFallback(context.Background(), fn, []string{"primary", "fallback"})
	require.Error(t, err)
	require.Equal(t, []string{"primary"}, called)
}

func TestFallback_AllModelsFail(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		return &fantasy.ProviderError{Message: "fail", StatusCode: 429}
	}

	err := agent.ExecuteWithFallback(context.Background(), fn,
		[]string{"a", "b", "c"})
	require.Error(t, err)
	require.Equal(t, []string{"a", "b", "c"}, called)
}

func TestFallback_EmptyChain(t *testing.T) {
	t.Parallel()

	err := agent.ExecuteWithFallback(context.Background(),
		func(string) error { return nil }, []string{})
	require.Error(t, err)
}

func TestFallback_MaxThreeAttempts(t *testing.T) {
	t.Parallel()

	called := []string{}
	fn := func(model string) error {
		called = append(called, model)
		return &fantasy.ProviderError{Message: "fail", StatusCode: 429}
	}

	// Chain has 5 models, but only 3 attempts should be made.
	_ = agent.ExecuteWithFallback(context.Background(), fn,
		[]string{"a", "b", "c", "d", "e"})
	require.Equal(t, []string{"a", "b", "c"}, called)
}

func TestFallbackChainForTokenCount(t *testing.T) {
	t.Parallel()

	tiers := []config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall,
			FallbackChain: []string{"small-fallback"}},
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge,
			FallbackChain: []string{"large-fallback-1", "large-fallback-2"}},
	}
	router := agent.NewTierRouter(tiers)

	require.Equal(t, []string{"small-fallback"}, router.FallbackChainForTokenCount(4000))
	require.Equal(t, []string{"large-fallback-1", "large-fallback-2"}, router.FallbackChainForTokenCount(5000))
}

// ---------------------------------------------------------------------------
// Test 5: ClassifyComplexity returns correct complexity level
// ---------------------------------------------------------------------------

func TestClassifyComplexityFromSignals_Simple(t *testing.T) {
	t.Parallel()

	// Minimal signals → simple.
	require.Equal(t, agent.ComplexitySimple,
		agent.ClassifyComplexityFromSignals(agent.ComplexitySignals{
			ToolCallCount:     1,
			TokenCount:        100,
			DistinctToolCount: 1,
		}))
}

func TestClassifyComplexityFromSignals_Medium(t *testing.T) {
	t.Parallel()

	// Moderate tool usage + moderate tokens → medium.
	require.Equal(t, agent.ComplexityMedium,
		agent.ClassifyComplexityFromSignals(agent.ComplexitySignals{
			ToolCallCount:     4,
			TokenCount:        5000,
			DistinctToolCount: 2,
		}))
}

func TestClassifyComplexityFromSignals_Complex_HighToolCount(t *testing.T) {
	t.Parallel()

	require.Equal(t, agent.ComplexityComplex,
		agent.ClassifyComplexityFromSignals(agent.ComplexitySignals{
			ToolCallCount:     8,
			TokenCount:        20000,
			DistinctToolCount: 6,
		}))
}

func TestClassifyComplexityFromSignals_Complex_PlanningTools(t *testing.T) {
	t.Parallel()

	// Even with low other signals, planning tools → complex.
	require.Equal(t, agent.ComplexityComplex,
		agent.ClassifyComplexityFromSignals(agent.ComplexitySignals{
			ToolCallCount:     1,
			TokenCount:        100,
			HasPlanningTools:  true,
			DistinctToolCount: 1,
		}))
}

func TestComplexityLevel_String(t *testing.T) {
	t.Parallel()

	require.Equal(t, "simple", agent.ComplexitySimple.String())
	require.Equal(t, "medium", agent.ComplexityMedium.String())
	require.Equal(t, "complex", agent.ComplexityComplex.String())
}

func TestComplexityLevel_NumericPriority(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.15, agent.ComplexitySimple.NumericPriority(), 0.001)
	require.InDelta(t, 0.4, agent.ComplexityMedium.NumericPriority(), 0.001)
	require.InDelta(t, 0.7, agent.ComplexityComplex.NumericPriority(), 0.001)
}

func TestTierRouter_ComplexityBoost(t *testing.T) {
	t.Parallel()

	tiers := []config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge},
	}
	router := agent.NewTierRouter(tiers)

	// 1000 tokens, simple → fits in small tier.
	require.Equal(t, config.SelectedModelTypeSmall,
		router.ResolveWithComplexity(1000, agent.ComplexitySimple))

	// 1000 tokens, complex → 4x boost = 4000 → fits in small tier (at boundary).
	require.Equal(t, config.SelectedModelTypeSmall,
		router.ResolveWithComplexity(1000, agent.ComplexityComplex))

	// 1100 tokens, complex → 4x boost = 4400 → exceeds 4000 → large.
	require.Equal(t, config.SelectedModelTypeLarge,
		router.ResolveWithComplexity(1100, agent.ComplexityComplex))
}

func TestTierRouter_PhaseMultiplier(t *testing.T) {
	t.Parallel()

	tiers := []config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 16000, ModelType: config.SelectedModelTypeLarge},
	}
	router := agent.NewTierRouter(tiers)

	// Planning phase: 3x boost → 1500*3 = 4500 > 4000 → large.
	require.Equal(t, config.SelectedModelTypeLarge,
		router.ResolveWithPhase(1500, agent.PhasePlanning))

	// Editing phase: 0.5x → 1500/2 = 750 → small.
	require.Equal(t, config.SelectedModelTypeSmall,
		router.ResolveWithPhase(1500, agent.PhaseEditing))

	// Reviewing phase: neutral → 1500 → small.
	require.Equal(t, config.SelectedModelTypeSmall,
		router.ResolveWithPhase(1500, agent.PhaseReviewing))
}

// ---------------------------------------------------------------------------
// Test 6: Deprecated ModelRouter still works as runtime fallback
// ---------------------------------------------------------------------------

func TestModelRouter_DefaultThreshold(t *testing.T) {
	t.Parallel()

	router := agent.NewModelRouter()

	// At or below default limit → editor.
	require.Equal(t, config.RoleEditor, router.RouteByTokenCount(4000))
	require.Equal(t, config.RoleEditor, router.RouteByTokenCount(0))

	// Above default limit → architect.
	require.Equal(t, config.RoleArchitect, router.RouteByTokenCount(4001))
}

func TestModelRouter_CustomThreshold(t *testing.T) {
	t.Parallel()

	router := agent.NewModelRouterWithLimit(8000)
	require.Equal(t, config.RoleEditor, router.RouteByTokenCount(8000))
	require.Equal(t, config.RoleArchitect, router.RouteByTokenCount(8001))
}

func TestModelRouter_ZeroLimitFallsBackToDefault(t *testing.T) {
	t.Parallel()

	router := &agent.ModelRouter{SmallModelTokenLimit: 0}
	require.Equal(t, config.RoleEditor, router.RouteByTokenCount(4000))
	require.Equal(t, config.RoleArchitect, router.RouteByTokenCount(4001))
}

func TestModelRouter_RouteByCharCount(t *testing.T) {
	t.Parallel()

	router := agent.NewModelRouterWithLimit(4000)

	// 16000 chars / 4 = 4000 tokens → editor (at limit).
	require.Equal(t, config.RoleEditor, router.RouteByCharCount(16000))

	// 16004 chars / 4 = 4001 tokens → architect.
	require.Equal(t, config.RoleArchitect, router.RouteByCharCount(16004))
}

// ---------------------------------------------------------------------------
// Bonus: ResourceLimit / ResourceUsage
// ---------------------------------------------------------------------------

func TestResourceLimit_Exceeded(t *testing.T) {
	t.Parallel()

	lim := agent.ResourceLimit{Soft: 8, Hard: 10}
	require.False(t, lim.Exceeded(5))
	require.False(t, lim.Exceeded(9))
	require.True(t, lim.Exceeded(10))
	require.True(t, lim.Exceeded(20))
}

func TestResourceLimit_Approaching(t *testing.T) {
	t.Parallel()

	// Soft explicitly set.
	lim := agent.ResourceLimit{Soft: 8, Hard: 10}
	require.False(t, lim.Approaching(7))
	require.True(t, lim.Approaching(8))
	require.True(t, lim.Approaching(9))

	// Soft = 0 → defaults to 80% of Hard.
	limAuto := agent.ResourceLimit{Hard: 100}
	require.False(t, limAuto.Approaching(79))
	require.True(t, limAuto.Approaching(80))
}

func TestResourceUsage_AddTokensAndSteps(t *testing.T) {
	t.Parallel()

	u := agent.NewResourceUsage()
	u.AddTokens("Hello world!") // 12 chars / 4 = 3 tokens.
	u.AddTokens("More text")   // 9 chars / 4 = 3 tokens (ceiling).
	u.AddStep()
	u.AddStep()

	snap := u.Snapshot()
	require.Equal(t, int64(6), snap.TokensUsed)
	require.Equal(t, int32(2), snap.StepsTaken)
}
