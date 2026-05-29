package agent

import (
	"sort"

	"github.com/charmbracelet/crush/internal/config"
)

// TierRouter routes prompts to model types using a sorted list of
// RoutingTier thresholds. It supports N tiers with linear-scan resolution
// and falls back to the largest tier's model type for token counts exceeding
// all thresholds.
type TierRouter struct {
	tiers      []config.RoutingTier
	agentTiers map[string][]config.RoutingTier
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
	if len(r.tiers) == 0 {
		return config.SelectedModelTypeLarge
	}

	result := r.tiers[len(r.tiers)-1].ModelType
	for _, tier := range r.tiers {
		if tokenCount <= tier.UpToTokens {
			result = tier.ModelType
			break
		}
	}
	return result
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

// Tiers returns a copy of the sorted tiers.
func (r *TierRouter) Tiers() []config.RoutingTier {
	out := make([]config.RoutingTier, len(r.tiers))
	copy(out, r.tiers)
	return out
}
