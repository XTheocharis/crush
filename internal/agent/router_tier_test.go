package agent

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTierRouterResolve_ExactBoundary(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(1000))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(4000))
}

func TestTierRouterResolve_BetweenTiers(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(500))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(2500))
}

func TestTierRouterResolve_AboveAllTiers(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(10000))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(999999))
}

func TestTierRouterResolve_BackwardCompat(t *testing.T) {
	t.Parallel()

	threshold := 4000
	r := NewTierRouterFromThreshold(threshold)

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(4000))
	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(1))
	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(0))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(4001))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(100000))
}

func TestTierRouterResolve_EmptyTiers(t *testing.T) {
	t.Parallel()

	r := NewTierRouter(nil)
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(0))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(10000))
}

func TestTierRouterResolve_SortsInput(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 8000, ModelType: config.SelectedModelTypeLarge},
		{UpToTokens: 2000, ModelType: config.SelectedModelTypeSmall},
	})

	tiers := r.Tiers()
	require.Equal(t, 2000, tiers[0].UpToTokens)
	require.Equal(t, 8000, tiers[1].UpToTokens)

	require.Equal(t, config.SelectedModelTypeSmall, r.Resolve(2000))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(5000))
	require.Equal(t, config.SelectedModelTypeLarge, r.Resolve(8000))
}

func TestTierRouterResolve_ThreeTiers(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: "tier-1"},
		{UpToTokens: 4000, ModelType: "tier-2"},
		{UpToTokens: 16000, ModelType: "tier-3"},
	})

	require.Equal(t, config.SelectedModelType("tier-1"), r.Resolve(500))
	require.Equal(t, config.SelectedModelType("tier-1"), r.Resolve(1000))
	require.Equal(t, config.SelectedModelType("tier-2"), r.Resolve(1001))
	require.Equal(t, config.SelectedModelType("tier-2"), r.Resolve(4000))
	require.Equal(t, config.SelectedModelType("tier-3"), r.Resolve(4001))
	require.Equal(t, config.SelectedModelType("tier-3"), r.Resolve(16000))
	require.Equal(t, config.SelectedModelType("tier-3"), r.Resolve(100000))
}

func TestTierRouterResolveByCharCount(t *testing.T) {
	t.Parallel()

	r := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 250, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeLarge},
	})

	require.Equal(t, config.SelectedModelTypeSmall, r.ResolveByCharCount(1000))
	require.Equal(t, config.SelectedModelTypeLarge, r.ResolveByCharCount(4000))
	require.Equal(t, config.SelectedModelTypeLarge, r.ResolveByCharCount(50000))
}
