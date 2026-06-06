package agent

import (
	"context"
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// TestTierRouting — tier selection with complexity-based routing.
//
// These tests verify that TierRouter routes low-complexity prompts to the
// small model and high-complexity prompts to the large model, using
// ResolveWithComplexity (the same path the coordinator uses at runtime).
//
// Note: shell-level metadata for routing decisions is not reliably
// available in CI; these are Go-only tests.
// --------------------------------------------------------------------------

func TestTierRouting_LowComplexityRoutesToSmall(t *testing.T) {
	t.Parallel()

	// Two-tier router matching the default production configuration.
	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	// A short, simple prompt (500 tokens) should route to the small model.
	got := router.ResolveWithComplexity(500, ComplexitySimple)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"simple 500-token prompt should route to small model")

	// At exactly the tier boundary (4000 tokens), simple still fits small.
	got = router.ResolveWithComplexity(4000, ComplexitySimple)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"simple 4000-token prompt at tier boundary should route to small model")
}

func TestTierRouting_HighComplexityRoutesToLarge(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	// Complex prompts get a 4x token boost. 500*4=2000 still fits small.
	got := router.ResolveWithComplexity(500, ComplexityComplex)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"complex 500-token prompt (boosted to 2000) should still fit small model")

	// 1001 tokens * 4x boost = 4004, which exceeds the 4000 small tier.
	got = router.ResolveWithComplexity(1001, ComplexityComplex)
	require.Equal(t, config.SelectedModelTypeLarge, got,
		"complex 1001-token prompt (boosted to 4004) should route to large model")
}

func TestTierRouting_MediumComplexityIntermediate(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	// Medium gets a 2x boost. 2000*2=4000 still fits small.
	got := router.ResolveWithComplexity(2000, ComplexityMedium)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"medium 2000-token prompt (boosted to 4000) should fit small model")

	// 2001*2=4002 exceeds the small tier.
	got = router.ResolveWithComplexity(2001, ComplexityMedium)
	require.Equal(t, config.SelectedModelTypeLarge, got,
		"medium 2001-token prompt (boosted to 4002) should route to large model")
}

func TestTierRouting_ThreeTierWithComplexity(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: "nano"},
		{UpToTokens: 8000, ModelType: "medium"},
		{UpToTokens: 200000, ModelType: "heavy"},
	})

	tests := []struct {
		name       string
		tokens     int
		complexity ComplexityLevel
		want       config.SelectedModelType
	}{
		{
			name:       "simple 800 tokens fit nano",
			tokens:     800,
			complexity: ComplexitySimple,
			want:       "nano",
		},
		{
			name:       "simple 5000 tokens fit medium",
			tokens:     5000,
			complexity: ComplexitySimple,
			want:       "medium",
		},
		{
			name:       "simple 50000 tokens fit heavy",
			tokens:     50000,
			complexity: ComplexitySimple,
			want:       "heavy",
		},
		{
			name:       "complex 300 tokens boosted 4x=1200 -> medium",
			tokens:     300,
			complexity: ComplexityComplex,
			want:       "medium",
		},
		{
			name:       "complex 2500 boosted 4x=10000 -> heavy",
			tokens:     2500,
			complexity: ComplexityComplex,
			want:       "heavy",
		},
		{
			name:       "medium 600 boosted 2x=1200 -> medium",
			tokens:     600,
			complexity: ComplexityMedium,
			want:       "medium",
		},
		{
			name:       "medium 5000 boosted 2x=10000 -> heavy",
			tokens:     5000,
			complexity: ComplexityMedium,
			want:       "heavy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := router.ResolveWithComplexity(tt.tokens, tt.complexity)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTierRouting_WithPhaseAdjustment(t *testing.T) {
	t.Parallel()

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 4000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 400000, ModelType: config.SelectedModelTypeLarge},
	})

	// Planning phase gets 3x boost: 1400*3=4200 > 4000 → large.
	got := router.ResolveWithPhase(1400, PhasePlanning)
	require.Equal(t, config.SelectedModelTypeLarge, got,
		"planning phase boosts 1400 to 4200, should route to large model")

	// Editing phase gets 0.5x reduction: 6000/2=3000 → small.
	got = router.ResolveWithPhase(6000, PhaseEditing)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"editing phase reduces 6000 to 3000, should route to small model")

	// Reviewing is neutral: 3000 → 3000 → small.
	got = router.ResolveWithPhase(3000, PhaseReviewing)
	require.Equal(t, config.SelectedModelTypeSmall, got,
		"reviewing phase is neutral, 3000 stays 3000 → small model")
}

// --------------------------------------------------------------------------
// TestRoutingFallback — fallback behavior when the primary provider fails.
//
// These tests simulate the live fallback flow: tier resolution determines
// the primary model, and ExecuteWithFallback retries on the fallback chain
// when the primary returns a retryable error (429/5xx).
// --------------------------------------------------------------------------

func TestRoutingFallback_PrimaryFails429_SecondarySucceeds(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()

	// Simulate a request that resolves to the small tier.
	modelType := router.Resolve(500)
	require.Equal(t, config.SelectedModelTypeSmall, modelType)

	chain := router.FallbackChainForTokenCount(500)
	require.Equal(t, []string{"small-primary", "small-backup"}, chain)

	var modelsUsed []string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		modelsUsed = append(modelsUsed, model)
		if model == "small-primary" {
			return &fantasy.ProviderError{
				StatusCode: 429,
				Message:    "rate limited",
			}
		}
		return nil
	}, chain)
	require.NoError(t, err)
	require.Equal(t, []string{"small-primary", "small-backup"}, modelsUsed,
		"should try primary first, then succeed on backup")
}

func TestRoutingFallback_PrimaryFails500_SecondarySucceeds(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()

	modelType := router.Resolve(2500)
	require.Equal(t, config.SelectedModelTypeLarge, modelType)

	chain := router.FallbackChainForTokenCount(2500)
	require.Equal(t, []string{"large-primary", "large-backup-1", "large-backup-2"}, chain)

	var modelsUsed []string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		modelsUsed = append(modelsUsed, model)
		if model == "large-primary" {
			return &fantasy.ProviderError{
				StatusCode: 500,
				Message:    "internal server error",
			}
		}
		return nil
	}, chain)
	require.NoError(t, err)
	require.Equal(t, []string{"large-primary", "large-backup-1"}, modelsUsed,
		"should try primary first, then succeed on first backup")
}

func TestRoutingFallback_AllFail_ReturnsLastError(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()

	chain := router.FallbackChainForTokenCount(500)
	lastErr := &fantasy.ProviderError{
		StatusCode: 503,
		Message:    "service unavailable",
	}

	var modelsUsed []string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		modelsUsed = append(modelsUsed, model)
		return lastErr
	}, chain)
	require.Error(t, err)
	require.Equal(t, []string{"small-primary", "small-backup"}, modelsUsed)
	require.Equal(t, lastErr, err, "should return the last error from the chain")
}

func TestRoutingFallback_NonRetryableError_StopsImmediately(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()
	chain := router.FallbackChainForTokenCount(500)

	var modelsUsed []string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		modelsUsed = append(modelsUsed, model)
		return &fantasy.ProviderError{
			StatusCode: 400,
			Message:    "bad request",
		}
	}, chain)
	require.Error(t, err)
	require.Equal(t, []string{"small-primary"}, modelsUsed,
		"400 error should not trigger fallback, only primary attempted")
}

func TestRoutingFallback_ComplexityBoostChangesTierThenFails(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()

	// 1000 tokens with simple complexity → small tier.
	modelType := router.ResolveWithComplexity(1000, ComplexitySimple)
	require.Equal(t, config.SelectedModelTypeSmall, modelType)

	// Same 1000 tokens with complex complexity (4x boost = 4000) → still small.
	// But with a 3-tier router, this would cross into a higher tier.
	// Use a three-tier router to demonstrate the complexity-driven tier shift.
	threeTierRouter := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall, FallbackChain: []string{"s-primary", "s-backup"}},
		{UpToTokens: 8000, ModelType: config.SelectedModelTypeLarge, FallbackChain: []string{"l-primary", "l-backup"}},
		{UpToTokens: 200000, ModelType: "ultra", FallbackChain: []string{"u-primary", "u-backup"}},
	})

	// 500 tokens, simple → small (500 <= 1000).
	simpleResult := threeTierRouter.ResolveWithComplexity(500, ComplexitySimple)
	require.Equal(t, config.SelectedModelTypeSmall, simpleResult)

	// 500 tokens, complex → 500*4=2000, which is > 1000 → large.
	complexResult := threeTierRouter.ResolveWithComplexity(500, ComplexityComplex)
	require.Equal(t, config.SelectedModelTypeLarge, complexResult)

	// Verify the fallback chain changes accordingly.
	simpleChain := threeTierRouter.FallbackChainForTokenCount(500)
	require.Equal(t, []string{"s-primary", "s-backup"}, simpleChain)

	complexChain := threeTierRouter.FallbackChainForTokenCount(2000)
	require.Equal(t, []string{"l-primary", "l-backup"}, complexChain)
}

func TestRoutingFallback_IntegrationResolveThenExecute(t *testing.T) {
	t.Parallel()

	// End-to-end integration: resolve tier, get fallback chain, execute with
	// primary succeeding on first try.
	router := newTestRouterWithFallback()

	modelType := router.Resolve(500)
	chain := router.FallbackChainForTokenCount(500)

	var executedModel string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		executedModel = model
		return nil
	}, chain)
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeSmall, modelType)
	require.Equal(t, "small-primary", executedModel)
}

func TestRoutingFallback_Wrapped429TriggersFallback(t *testing.T) {
	t.Parallel()

	router := newTestRouterWithFallback()
	chain := router.FallbackChainForTokenCount(2500)

	var modelsUsed []string
	err := ExecuteWithFallback(context.Background(), func(model string) error {
		modelsUsed = append(modelsUsed, model)
		if model == "large-primary" {
			return fmt.Errorf("provider failure: %w", &fantasy.ProviderError{
				StatusCode: 429,
				Message:    "rate limited",
			})
		}
		return nil
	}, chain)
	require.NoError(t, err)
	require.Equal(t, []string{"large-primary", "large-backup-1"}, modelsUsed,
		"wrapped 429 error should still trigger fallback")
}

// newTestRouterWithFallback creates a TierRouter with both small and large
// tiers, each carrying a fallback chain. This mirrors a realistic production
// configuration.
func newTestRouterWithFallback() *TierRouter {
	return NewTierRouter([]config.RoutingTier{
		{
			UpToTokens:    1000,
			ModelType:     config.SelectedModelTypeSmall,
			FallbackChain: []string{"small-primary", "small-backup"},
		},
		{
			UpToTokens:    400000,
			ModelType:     config.SelectedModelTypeLarge,
			FallbackChain: []string{"large-primary", "large-backup-1", "large-backup-2"},
		},
	})
}
