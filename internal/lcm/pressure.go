package lcm

import (
	"context"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// PressureTier
// ---------------------------------------------------------------------------

// PressureTier represents the severity of context window pressure. Higher
// tiers trigger progressively more aggressive compaction strategies.
type PressureTier int

const (
	// PressureLow indicates minimal pressure. Only micro-compaction (Layer 1)
	// is warranted.
	PressureLow PressureTier = iota

	// PressureMedium indicates moderate pressure. Session memory compaction
	// and tool-output compression should be activated.
	PressureMedium

	// PressureHigh indicates critical pressure. Full compaction with
	// aggressive summarization is required.
	PressureHigh
)

// String returns a human-readable name for the tier.
func (t PressureTier) String() string {
	switch t {
	case PressureLow:
		return "low"
	case PressureMedium:
		return "medium"
	case PressureHigh:
		return "high"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func parsePressureTier(s string) (PressureTier, bool) {
	switch strings.ToLower(s) {
	case "low":
		return PressureLow, true
	case "medium":
		return PressureMedium, true
	case "high":
		return PressureHigh, true
	default:
		return PressureLow, false
	}
}

// ---------------------------------------------------------------------------
// PressureConfig
// ---------------------------------------------------------------------------

// PressureConfig holds configurable thresholds that map context usage to
// pressure tiers. When UseAbsoluteOffsets is true (default), thresholds are
// computed as absolute token offsets from the context window limit. Otherwise,
// the legacy percentage-based thresholds (LowThreshold, MediumThreshold,
// HighThreshold) are used.
type PressureConfig struct {
	// UseAbsoluteOffsets selects absolute-offset mode (true) or legacy
	// percentage mode (false). Default: true.
	UseAbsoluteOffsets bool

	// SoftOffset is the absolute token count reserved below the context
	// window limit at which Low pressure begins. Default: 20000.
	SoftOffset int64

	// CompactOffset is the absolute token count reserved below the context
	// window limit at which Medium pressure begins. Default: 13000.
	CompactOffset int64

	// HardOffset is the absolute token count reserved below the context
	// window limit at which High pressure begins. A minimum floor of 1000
	// is enforced: effective hard offset = max(HardOffset, 1000). Default: 3000.
	HardOffset int64

	// LowThreshold is the percentage at which Low pressure begins (legacy mode).
	// Below this value no compaction is needed at all. Default: 70.
	LowThreshold float64

	// MediumThreshold is the percentage at which Medium pressure begins (legacy mode).
	// Default: 85.
	MediumThreshold float64

	// HighThreshold is the percentage at which High pressure begins (legacy mode).
	// Default: 95.
	HighThreshold float64
}

// DefaultPressureConfig returns the standard configuration with absolute
// offsets enabled: SoftOffset=20000, CompactOffset=13000, HardOffset=3000.
// Legacy percentage thresholds (70/85/95) are also populated for fallback.
func DefaultPressureConfig() PressureConfig {
	return PressureConfig{
		UseAbsoluteOffsets: true,
		SoftOffset:         20000,
		CompactOffset:      13000,
		HardOffset:         3000,
		LowThreshold:       70.0,
		MediumThreshold:    85.0,
		HighThreshold:      95.0,
	}
}

// ---------------------------------------------------------------------------
// PressureCalculator
// ---------------------------------------------------------------------------

// CalculatePressure computes the current pressure percentage from token usage
// and the total context window. Returns 0 when contextWindow is zero or
// negative.
func CalculatePressure(currentTokens, contextWindow int64) float64 {
	if contextWindow <= 0 {
		return 0
	}
	return float64(currentTokens) / float64(contextWindow) * 100
}

// CalculatePressureTier determines the pressure tier from current token usage
// and the context window, using the supplied threshold configuration. When
// cfg.UseAbsoluteOffsets is true, thresholds are computed as absolute token
// offsets from the context window limit. Otherwise, legacy percentage-based
// thresholds are used. Returns the raw pressure percentage alongside the tier.
func CalculatePressureTier(currentTokens, contextWindow int64, cfg PressureConfig) (pressure float64, tier PressureTier) {
	pressure = CalculatePressure(currentTokens, contextWindow)
	if cfg.UseAbsoluteOffsets {
		effectiveHard := cfg.HardOffset
		if effectiveHard < 1000 {
			effectiveHard = 1000
		}
		softThreshold := contextWindow - cfg.SoftOffset
		compactThreshold := contextWindow - cfg.CompactOffset
		hardThreshold := contextWindow - effectiveHard
		switch {
		case currentTokens >= hardThreshold:
			return pressure, PressureHigh
		case currentTokens >= compactThreshold:
			return pressure, PressureMedium
		case currentTokens >= softThreshold:
			return pressure, PressureLow
		default:
			return pressure, PressureLow
		}
	}
	switch {
	case pressure >= cfg.HighThreshold:
		return pressure, PressureHigh
	case pressure >= cfg.MediumThreshold:
		return pressure, PressureMedium
	case pressure >= cfg.LowThreshold:
		return pressure, PressureLow
	default:
		return pressure, PressureLow
	}
}

// ---------------------------------------------------------------------------
// PressureCompactionSelector (Layer 5)
// ---------------------------------------------------------------------------

// TokenUsageFunc retrieves the current token usage and context window size.
// Implementations typically query the LCM store for the live session.
type TokenUsageFunc func(ctx context.Context) (currentTokens int64, contextWindow int64, err error)

// PressureCompactionSelector selects which compaction layers to activate based
// on the current pressure tier. It implements CompactionLayer at priority 5
// (Warning/Error Thresholds) so it integrates into the CompactionLayerManager.
//
// The tier-to-layers mapping is:
//
//	Low    → micro-compaction only
//	Medium → session memory compaction + compress tool outputs
//	High   → full compaction + aggressive summarization
//
// Sub-layers for each tier are supplied at construction time so the selector
// itself does not hard-code any compaction logic.
type PressureCompactionSelector struct {
	cfg          PressureConfig
	usageFn      TokenUsageFunc
	tierLayers   map[PressureTier][]CompactionLayer
	overrideTier *PressureTier
}

// NewPressureCompactionSelector creates a Layer 5 selector with the given
// configuration, token-usage callback, and per-tier layer mappings. If cfg has
// zero-valued thresholds they are filled from DefaultPressureConfig.
func NewPressureCompactionSelector(
	cfg PressureConfig,
	usageFn TokenUsageFunc,
	tierLayers map[PressureTier][]CompactionLayer,
) *PressureCompactionSelector {
	cfg = fillDefaults(cfg)
	if tierLayers == nil {
		tierLayers = make(map[PressureTier][]CompactionLayer)
	}
	return &PressureCompactionSelector{
		cfg:        cfg,
		usageFn:    usageFn,
		tierLayers: tierLayers,
	}
}

// SetOverrideTier sets a one-shot pressure tier override. The override is
// consumed after the next Compact call.
func (s *PressureCompactionSelector) SetOverrideTier(tier PressureTier) {
	s.overrideTier = &tier
}

// fillDefaults replaces zero-valued fields with the defaults.
func fillDefaults(cfg PressureConfig) PressureConfig {
	defs := DefaultPressureConfig()
	if cfg.SoftOffset == 0 {
		cfg.SoftOffset = defs.SoftOffset
	}
	if cfg.CompactOffset == 0 {
		cfg.CompactOffset = defs.CompactOffset
	}
	if cfg.HardOffset == 0 {
		cfg.HardOffset = defs.HardOffset
	}
	if cfg.LowThreshold == 0 {
		cfg.LowThreshold = defs.LowThreshold
	}
	if cfg.MediumThreshold == 0 {
		cfg.MediumThreshold = defs.MediumThreshold
	}
	if cfg.HighThreshold == 0 {
		cfg.HighThreshold = defs.HighThreshold
	}
	return cfg
}

// Name returns "pressure-selector".
func (s *PressureCompactionSelector) Name() string { return "pressure-selector" }

// Priority returns 5 (Layer 5: Warning/Error Thresholds).
func (s *PressureCompactionSelector) Priority() int { return 5 }

// ShouldCompact reports whether the current pressure is at or above the Low
// threshold, meaning some compaction is warranted.
func (s *PressureCompactionSelector) ShouldCompact(ctx context.Context, budget Budget) bool {
	currentTokens, contextWindow, err := s.usageFn(ctx)
	if err != nil {
		return false
	}
	pressure := CalculatePressure(currentTokens, contextWindow)
	return pressure >= s.cfg.LowThreshold
}

// Compact determines the current pressure tier and runs the sub-layers
// registered for that tier. It returns the aggregate result.
func (s *PressureCompactionSelector) Compact(ctx context.Context, budget Budget) (*CompactionLayerResult, error) {
	currentTokens, contextWindow, err := s.usageFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("pressure-selector: reading token usage: %w", err)
	}

	tier := PressureLow
	if s.overrideTier != nil {
		tier = *s.overrideTier
		s.overrideTier = nil
	} else {
		_, tier = CalculatePressureTier(currentTokens, contextWindow, s.cfg)
	}
	return s.runTierLayers(ctx, tier, budget)
}

// SelectLayers returns the compaction layers registered for the given tier.
// Returns nil if no layers are registered for the tier.
func (s *PressureCompactionSelector) SelectLayers(tier PressureTier) []CompactionLayer {
	return s.tierLayers[tier]
}

// Tier returns the current pressure tier by querying the token usage.
func (s *PressureCompactionSelector) Tier(ctx context.Context) (PressureTier, error) {
	currentTokens, contextWindow, err := s.usageFn(ctx)
	if err != nil {
		return PressureLow, fmt.Errorf("pressure-selector: reading token usage: %w", err)
	}
	_, tier := CalculatePressureTier(currentTokens, contextWindow, s.cfg)
	return tier, nil
}

// runTierLayers executes the sub-layers for the given tier in order and
// aggregates their results.
func (s *PressureCompactionSelector) runTierLayers(ctx context.Context, tier PressureTier, budget Budget) (*CompactionLayerResult, error) {
	layers := s.tierLayers[tier]
	if len(layers) == 0 {
		return &CompactionLayerResult{LayerName: s.Name()}, nil
	}

	var aggregate CompactionLayerResult
	for _, layer := range layers {
		if !layer.ShouldCompact(ctx, budget) {
			continue
		}
		result, err := layer.Compact(ctx, budget)
		if err != nil {
			return nil, fmt.Errorf("pressure-selector tier %s layer %s: %w", tier, layer.Name(), err)
		}
		if result != nil && result.ActionTaken {
			aggregate.TokensFreed += result.TokensFreed
			aggregate.ItemsAffected += result.ItemsAffected
			aggregate.ActionTaken = true
		}
	}
	if aggregate.ActionTaken {
		aggregate.LayerName = s.Name()
	}
	return &aggregate, nil
}

// ---------------------------------------------------------------------------
// GraduatedPressureSystem
// ---------------------------------------------------------------------------

// GraduatedPressureSystem selects CompressionStrategy instances based on
// context window pressure tiers. It complements PressureCompactionSelector
// by mapping tiers to strategy chains rather than compaction layers.
//
// The tier-to-strategy mapping is:
//
//	Level 1 (≥70%): PurgeErrors only (light compression).
//	Level 2 (≥85%): PurgeErrors + Dedup + Message (medium compression).
//	Level 3 (≥95%): All strategies (heavy compression).
type GraduatedPressureSystem struct {
	cfg        PressureConfig
	limits     ContextLimits
	strategies map[PressureTier][]CompressionStrategy
}

type graduatedPressureOpts struct {
	purgeErrorsEnabled bool
}

// GraduatedPressureOption configures a GraduatedPressureSystem.
type GraduatedPressureOption func(*graduatedPressureOpts)

// WithPurgeErrorsEnabled controls whether the purge-errors strategy is
// included. Defaults to true when no option is provided.
func WithPurgeErrorsEnabled(enabled bool) GraduatedPressureOption {
	return func(o *graduatedPressureOpts) { o.purgeErrorsEnabled = enabled }
}

// NewGraduatedPressureSystem creates a GraduatedPressureSystem with the given
// configuration, context limits, and LLM client. It builds the default
// tier-to-strategy mapping from the client. If cfg has zero-valued thresholds
// they are filled from DefaultPressureConfig.
func NewGraduatedPressureSystem(cfg PressureConfig, limits ContextLimits, llm LLMClient, opts ...GraduatedPressureOption) *GraduatedPressureSystem {
	cfg = fillDefaults(cfg)

	gOpts := graduatedPressureOpts{purgeErrorsEnabled: true}
	for _, o := range opts {
		o(&gOpts)
	}

	dedup := NewDedupCompression(llm)
	message := NewMessageCompression(llm)
	rng := NewRangeCompression(llm)

	var purgeErrors CompressionStrategy
	if gOpts.purgeErrorsEnabled {
		purgeErrors = NewPurgeErrorsCompression(llm)
	}

	lowStrategies := []CompressionStrategy{}
	medStrategies := []CompressionStrategy{}
	highStrategies := []CompressionStrategy{}

	if purgeErrors != nil {
		lowStrategies = append(lowStrategies, purgeErrors)
		medStrategies = append(medStrategies, purgeErrors)
		highStrategies = append(highStrategies, purgeErrors)
	}
	medStrategies = append(medStrategies, dedup, message)
	highStrategies = append(highStrategies, dedup, message, rng)

	return &GraduatedPressureSystem{
		cfg:    cfg,
		limits: limits,
		strategies: map[PressureTier][]CompressionStrategy{
			PressureLow:    lowStrategies,
			PressureMedium: medStrategies,
			PressureHigh:   highStrategies,
		},
	}
}

// StrategiesForTier returns the compression strategies registered for the
// given pressure tier. Returns nil for unrecognised tiers.
func (g *GraduatedPressureSystem) StrategiesForTier(tier PressureTier) []CompressionStrategy {
	return g.strategies[tier]
}

// TierForTokens determines the pressure tier from a raw token count using the
// ContextLimits and PressureConfig. Returns PressureLow for zero/negative
// context limits.
func (g *GraduatedPressureSystem) TierForTokens(currentTokens int64) PressureTier {
	maxTokens := int64(g.limits.MaxTokens)
	if maxTokens <= 0 {
		return PressureLow
	}
	_, tier := CalculatePressureTier(currentTokens, maxTokens, g.cfg)
	return tier
}

// StrategiesForTokens returns the compression strategies for the tier that
// corresponds to the given token count.
func (g *GraduatedPressureSystem) StrategiesForTokens(currentTokens int64) []CompressionStrategy {
	tier := g.TierForTokens(currentTokens)
	return g.StrategiesForTier(tier)
}

// Limits returns the ContextLimits used by this system.
func (g *GraduatedPressureSystem) Limits() ContextLimits {
	return g.limits
}
