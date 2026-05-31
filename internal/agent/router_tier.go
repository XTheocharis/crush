package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
)

// maxFallbackAttempts is the maximum number of total attempts (primary +
// fallbacks) that ExecuteWithFallback will make before giving up.
const maxFallbackAttempts = 3

// TierRouter routes prompts to model types using a sorted list of
// RoutingTier thresholds. It supports N tiers with linear-scan resolution
// and falls back to the largest tier's model type for token counts exceeding
// all thresholds.
type TierRouter struct {
	tiers       []config.RoutingTier
	agentTiers  map[string][]config.RoutingTier
	costTracker *CostTracker
	metrics     *MetricsStore
}

// NewTierRouter creates a TierRouter from the given tiers. The tiers are
// sorted ascending by UpToTokens. If tiers is empty, the router returns
// config.SelectedModelTypeLarge for all inputs.
func NewTierRouter(tiers []config.RoutingTier) *TierRouter {
	sorted := make([]config.RoutingTier, len(tiers))
	copy(sorted, tiers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpToTokens < sorted[j].UpToTokens
	})
	return &TierRouter{tiers: sorted}
}

// NewTierRouterWithAgentTiers creates a TierRouter with per-agent tier
// overrides. Each agent's tiers are sorted ascending by UpToTokens. When
// RouteForAgent is called, agent-specific tiers are checked first; if the
// agent has no specific tiers, the global table is used.
func NewTierRouterWithAgentTiers(
	globalTiers []config.RoutingTier,
	agentTiers map[string][]config.RoutingTier,
) *TierRouter {
	r := NewTierRouter(globalTiers)
	sorted := make(map[string][]config.RoutingTier, len(agentTiers))
	for name, tiers := range agentTiers {
		s := make([]config.RoutingTier, len(tiers))
		copy(s, tiers)
		sort.Slice(s, func(i, j int) bool {
			return s[i].UpToTokens < s[j].UpToTokens
		})
		sorted[name] = s
	}
	r.agentTiers = sorted
	return r
}

// SetCostTracker attaches a CostTracker to the router for cost-aware tier
// selection. When set, Resolve and RouteForAgent consult the tracker before
// returning: if the tracker signals ForceLowTier the lowest-cost tier is
// returned; if it signals ShouldDowngrade the result is downgraded to the
// lowest tier.
func (r *TierRouter) SetCostTracker(ct *CostTracker) {
	r.costTracker = ct
}

// lowestModelType returns the model type of the first (lowest-token) tier,
// falling back to SelectedModelTypeSmall.
func (r *TierRouter) lowestModelType() config.SelectedModelType {
	if len(r.tiers) > 0 {
		return r.tiers[0].ModelType
	}
	return config.SelectedModelTypeSmall
}

// NewTierRouterFromThreshold creates a 2-tier router that matches the old
// binary threshold behavior: token counts at or below the limit route to
// SelectedModelTypeSmall; above routes to SelectedModelTypeLarge.
func NewTierRouterFromThreshold(threshold int) *TierRouter {
	return &TierRouter{
		tiers: []config.RoutingTier{
			{UpToTokens: threshold, ModelType: config.SelectedModelTypeSmall},
			{UpToTokens: threshold * 100, ModelType: config.SelectedModelTypeLarge},
		},
	}
}

// Resolve returns the SelectedModelType for the given token count using a
// linear scan of the sorted tiers. The scan finds the highest tier whose
// UpToTokens is >= tokenCount. If tokenCount exceeds all tier thresholds,
// the largest tier's ModelType is returned (fallback behavior).
func (r *TierRouter) Resolve(tokenCount int) config.SelectedModelType {
	return r.ResolveWithComplexity(tokenCount, ComplexitySimple)
}

// ResolveWithComplexity returns the SelectedModelType for the given token
// count and complexity level. When the complexity is Complex, the effective
// token count is boosted so that the router prefers higher-tier models. When
// complexity is ComplexitySimple (the zero value), behavior is identical to
// Resolve for backward compatibility.
func (r *TierRouter) ResolveWithComplexity(
	tokenCount int,
	complexity ComplexityLevel,
) config.SelectedModelType {
	if len(r.tiers) == 0 {
		return config.SelectedModelTypeLarge
	}

	effectiveTokenCount := applyComplexityBoost(tokenCount, complexity)

	result := r.tiers[len(r.tiers)-1].ModelType
	for _, tier := range r.tiers {
		if effectiveTokenCount <= tier.UpToTokens {
			result = tier.ModelType
			break
		}
	}

	if r.costTracker != nil {
		result = r.costTracker.ResolveWithCost(result, r.lowestModelType())
	}
	return result
}

// applyComplexityBoost adjusts the effective token count based on complexity.
// Complex tasks get a 4x multiplier to push them into higher tiers, Medium
// tasks get a 2x multiplier, and Simple tasks are unchanged.
func applyComplexityBoost(tokenCount int, complexity ComplexityLevel) int {
	switch complexity {
	case ComplexityComplex:
		return tokenCount * 4
	case ComplexityMedium:
		return tokenCount * 2
	default:
		return tokenCount
	}
}

// ResolveWithPhase returns the SelectedModelType for the given token count
// adjusted by the agent's current phase. Planning-heavy conversations get a
// token boost to prefer architect-capable (higher-tier) models. Editing-heavy
// conversations get a token reduction to prefer fast editor (lower-tier)
// models. PhaseReviewing is neutral.
func (r *TierRouter) ResolveWithPhase(
	tokenCount int,
	phase AgentPhase,
) config.SelectedModelType {
	if len(r.tiers) == 0 {
		return config.SelectedModelTypeLarge
	}

	effectiveTokenCount := applyPhaseMultiplier(tokenCount, phase)

	result := r.tiers[len(r.tiers)-1].ModelType
	for _, tier := range r.tiers {
		if effectiveTokenCount <= tier.UpToTokens {
			result = tier.ModelType
			break
		}
	}

	if r.costTracker != nil {
		result = r.costTracker.ResolveWithCost(result, r.lowestModelType())
	}
	return result
}

// ResolveWithComplexityAndPhase returns the SelectedModelType for the given
// token count adjusted by both complexity and phase. Complexity boost is
// applied first, then phase adjustment. This combines both routing signals
// for maximum accuracy.
func (r *TierRouter) ResolveWithComplexityAndPhase(
	tokenCount int,
	complexity ComplexityLevel,
	phase AgentPhase,
) config.SelectedModelType {
	if len(r.tiers) == 0 {
		return config.SelectedModelTypeLarge
	}

	boosted := applyComplexityBoost(tokenCount, complexity)
	effectiveTokenCount := applyPhaseMultiplier(boosted, phase)

	result := r.tiers[len(r.tiers)-1].ModelType
	for _, tier := range r.tiers {
		if effectiveTokenCount <= tier.UpToTokens {
			result = tier.ModelType
			break
		}
	}

	if r.costTracker != nil {
		result = r.costTracker.ResolveWithCost(result, r.lowestModelType())
	}
	return result
}

// applyPhaseMultiplier adjusts the effective token count based on the agent
// phase. Planning conversations get a 3x boost to prefer higher-tier
// (architect-capable) models. Editing conversations get a 0.5x reduction to
// prefer lower-tier (fast editor) models. Reviewing is neutral (1x).
func applyPhaseMultiplier(tokenCount int, phase AgentPhase) int {
	switch phase {
	case PhasePlanning:
		return tokenCount * 3
	case PhaseEditing:
		return tokenCount / 2
	default:
		return tokenCount
	}
}

// RouteForAgent resolves the model type for a given agent name and token
// count. If the agent has specific tiers configured, those are used;
// otherwise the global table is consulted via Resolve.
func (r *TierRouter) RouteForAgent(
	agentName string,
	tokenCount int,
) config.SelectedModelType {
	if agentName == "" {
		return r.Resolve(tokenCount)
	}
	if tiers, ok := r.agentTiers[agentName]; ok {
		result := tiers[len(tiers)-1].ModelType
		for _, tier := range tiers {
			if tokenCount <= tier.UpToTokens {
				result = tier.ModelType
				break
			}
		}
		if r.costTracker != nil {
			result = r.costTracker.ResolveWithCost(result, r.lowestModelType())
		}
		return result
	}
	return r.Resolve(tokenCount)
}

// ResolveByCharCount converts a character count to an estimated token count
// using ceiling division, then delegates to Resolve.
func (r *TierRouter) ResolveByCharCount(charCount int) config.SelectedModelType {
	tokenCount := (charCount + charsPerToken - 1) / charsPerToken
	return r.Resolve(tokenCount)
}

// SetMetricsStore sets the metrics store used to record per-model performance.
func (r *TierRouter) SetMetricsStore(store *MetricsStore) {
	r.metrics = store
}

// Tiers returns a copy of the sorted tiers.
func (r *TierRouter) Tiers() []config.RoutingTier {
	out := make([]config.RoutingTier, len(r.tiers))
	copy(out, r.tiers)
	return out
}

// ExecuteWithFallback executes fn with each model in the chain until one
// succeeds or the chain is exhausted. The chain should contain the primary
// model as the first element followed by fallback models.
//
// Retryable errors (429, 5xx, timeout) trigger fallback to the next model.
// Non-retryable errors (400 user errors) return immediately.
// At most maxFallbackAttempts total attempts are made.
func ExecuteWithFallback(
	ctx context.Context,
	fn func(model string) error,
	chain []string,
) error {
	if len(chain) == 0 {
		return fmt.Errorf("empty fallback chain")
	}

	attempts := min(len(chain), maxFallbackAttempts)
	var lastErr error
	for i := range attempts {
		model := chain[i]
		err := fn(model)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryableProviderError(err) {
			return err
		}

		if i < attempts-1 {
			slog.Warn("Falling back from model due to retryable error",
				"from_model", model,
				"to_model", chain[i+1],
				"attempt", i+1,
				"error", err,
			)
		}
	}

	return lastErr
}

// isRetryableProviderError reports whether the error is a retryable provider
// error (429, 5xx, timeout, or unexpected EOF). Non-provider errors (e.g.,
// context cancellation) are not considered retryable.
func isRetryableProviderError(err error) bool {
	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.IsRetryable() {
			return true
		}
		// 400-level errors other than 429/408/409 are not retryable.
		// IsRetryable already handles 429, 408, 409, and 5xx.
		return false
	}
	// Non-ProviderError: could be context cancellation or timeout.
	// Only retry on deadline exceeded or generic timeout errors.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

// FallbackChainForTokenCount resolves the fallback chain for a given token
// count. It returns the FallbackChain from the matching tier, or nil if no
// fallback chain is configured.
func (r *TierRouter) FallbackChainForTokenCount(tokenCount int) []string {
	if len(r.tiers) == 0 {
		return nil
	}
	for _, tier := range r.tiers {
		if tokenCount <= tier.UpToTokens {
			return tier.FallbackChain
		}
	}
	// Token count exceeds all tiers; use last tier's chain.
	return r.tiers[len(r.tiers)-1].FallbackChain
}

// FallbackChainForAgent resolves the fallback chain for a given agent name
// and token count. Agent-specific tiers are checked first; global tiers are
// the fallback.
func (r *TierRouter) FallbackChainForAgent(
	agentName string,
	tokenCount int,
) []string {
	if agentName == "" {
		return r.FallbackChainForTokenCount(tokenCount)
	}
	if tiers, ok := r.agentTiers[agentName]; ok {
		for _, tier := range tiers {
			if tokenCount <= tier.UpToTokens {
				return tier.FallbackChain
			}
		}
		return tiers[len(tiers)-1].FallbackChain
	}
	return r.FallbackChainForTokenCount(tokenCount)
}
