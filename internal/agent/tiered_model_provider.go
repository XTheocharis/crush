package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
)

// TieredModelProvider creates and caches separate Model instances per tier.
// Each tier corresponds to a SelectedModelType (e.g. "small", "large") and
// resolves to a different model + provider combination from the config.
//
// When no tiers are configured (provider is nil), the caller should use the
// default single-model path for backward compatibility.
type TieredModelProvider struct {
	cfg     *config.ConfigStore
	coord   modelBuilder
	models  map[config.SelectedModelType]Model
	mu      sync.RWMutex
	cache   map[string]Model
	rlCoord *RateLimitCoordinator
}

// modelBuilder is extracted from coordinator to allow test doubles that
// create LanguageModel instances without the full coordinator dependency
// tree.
type modelBuilder interface {
	// BuildModel creates a fully-wired Model (including rate-limit wrapping)
	// from the given provider config and selected model.
	BuildModel(ctx context.Context, providerCfg config.ProviderConfig, sel config.SelectedModel, isSubAgent bool) (Model, error)
}

// NewTieredModelProvider creates a TieredModelProvider that serves Model
// instances per tier. The models map must contain at least the model types
// referenced by the tier router. Unrecognised tier keys return an error from
// ModelForTier.
func NewTieredModelProvider(
	cfg *config.ConfigStore,
	coord modelBuilder,
	models map[config.SelectedModelType]Model,
	rlCoord *RateLimitCoordinator,
) *TieredModelProvider {
	return &TieredModelProvider{
		cfg:     cfg,
		coord:   coord,
		models:  models,
		cache:   make(map[string]Model),
		rlCoord: rlCoord,
	}
}

// ModelForTier returns the cached Model for the given tier key. If the model
// has not been built yet, it is created from the config, cached, and returned.
// The tier key is a SelectedModelType string (e.g. "small", "large").
func (p *TieredModelProvider) ModelForTier(ctx context.Context, tierKey config.SelectedModelType) (Model, error) {
	cacheKey := p.cacheKeyForTier(tierKey)

	p.mu.RLock()
	if m, ok := p.cache[cacheKey]; ok {
		p.mu.RUnlock()
		return m, nil
	}
	p.mu.RUnlock()

	if m, ok := p.models[tierKey]; ok {
		p.mu.Lock()
		if cached, ok := p.cache[cacheKey]; ok {
			p.mu.Unlock()
			return cached, nil
		}
		p.cache[cacheKey] = m
		p.mu.Unlock()
		return m, nil
	}

	modelCfg, ok := p.cfg.Config().Models[tierKey]
	if !ok {
		return Model{}, fmt.Errorf("tiered model provider: no model configured for tier %q", tierKey)
	}

	providerCfg, ok := p.cfg.Config().Providers.Get(modelCfg.Provider)
	if !ok {
		return Model{}, fmt.Errorf("tiered model provider: provider %q not configured for tier %q", modelCfg.Provider, tierKey)
	}

	m, err := p.coord.BuildModel(ctx, providerCfg, modelCfg, false)
	if err != nil {
		return Model{}, fmt.Errorf("tiered model provider: build model for tier %q: %w", tierKey, err)
	}

	p.mu.Lock()
	if cached, ok := p.cache[cacheKey]; ok {
		p.mu.Unlock()
		return cached, nil
	}
	p.cache[cacheKey] = m
	p.mu.Unlock()

	return m, nil
}

// HasTier reports whether a model is available for the given tier key.
func (p *TieredModelProvider) HasTier(tierKey config.SelectedModelType) bool {
	if _, ok := p.models[tierKey]; ok {
		return true
	}
	_, ok := p.cfg.Config().Models[tierKey]
	return ok
}

// Tiers returns the set of tier keys that have models configured.
func (p *TieredModelProvider) Tiers() []config.SelectedModelType {
	p.mu.RLock()
	defer p.mu.RUnlock()

	seen := make(map[config.SelectedModelType]bool)
	for k := range p.models {
		seen[k] = true
	}
	for k := range p.cfg.Config().Models {
		seen[k] = true
	}

	result := make([]config.SelectedModelType, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	return result
}

// cacheKeyForTier produces a stable cache key for the given tier. If the tier
// is configured in the config, the key is "provider:model". Otherwise it falls
// back to the tier string itself.
func (p *TieredModelProvider) cacheKeyForTier(tierKey config.SelectedModelType) string {
	if modelCfg, ok := p.cfg.Config().Models[tierKey]; ok {
		return modelCfg.Provider + ":" + modelCfg.Model
	}
	return string(tierKey)
}

// coordinatorModelBuilder adapts the coordinator to the modelBuilder interface.
type coordinatorModelBuilder struct {
	coord *coordinator
}

// BuildModel creates a Model by delegating to the coordinator's provider
// construction and LanguageModel creation.
func (b coordinatorModelBuilder) BuildModel(ctx context.Context, providerCfg config.ProviderConfig, sel config.SelectedModel, isSubAgent bool) (Model, error) {
	provider, err := b.coord.buildProvider(providerCfg, sel, isSubAgent)
	if err != nil {
		return Model{}, err
	}

	var catwalkModel *catwalk.Model
	for _, m := range providerCfg.Models {
		if m.ID == sel.Model {
			catwalkModel = &m
			break
		}
	}
	if catwalkModel == nil {
		return Model{}, fmt.Errorf("model %q not found in provider %q", sel.Model, sel.Provider)
	}

	lm, err := provider.LanguageModel(ctx, sel.Model)
	if err != nil {
		return Model{}, err
	}

	if b.coord.rateLimitCoord != nil {
		lm = newRateLimitedModel(lm, b.coord.rateLimitCoord, sel.Provider)
	}

	return Model{
		Model:      lm,
		CatwalkCfg: *catwalkModel,
		ModelCfg:   sel,
		FlatRate:   providerCfg.FlatRate,
	}, nil
}

// ResolveTieredModel resolves the appropriate Model for a given step using the
// TierRouter and TieredModelProvider. When tieredProvider is nil, returns the
// defaultModel unchanged (backward compatible).
func ResolveTieredModel(
	ctx context.Context,
	tierRouter *TierRouter,
	tieredProvider *TieredModelProvider,
	tokenCount int,
	defaultModel Model,
) (Model, error) {
	if tieredProvider == nil || tierRouter == nil {
		return defaultModel, nil
	}

	tierKey := tierRouter.Resolve(tokenCount)

	if !tieredProvider.HasTier(tierKey) {
		slog.Debug("TieredModelProvider: tier not configured, using default model",
			"tier", tierKey,
		)
		return defaultModel, nil
	}

	model, err := tieredProvider.ModelForTier(ctx, tierKey)
	if err != nil {
		slog.Warn("TieredModelProvider: failed to resolve tier model, using default",
			"tier", tierKey,
			"error", err,
		)
		return defaultModel, nil
	}

	return model, nil
}

var _ fantasy.LanguageModel = (Model{}).Model
