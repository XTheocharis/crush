package nudge

import (
	"context"
	"fmt"
	"strings"
)

// NudgeType identifies the kind of nudge to inject.
type NudgeType string

const (
	// NudgeContextLimit triggers when context usage is critically high.
	NudgeContextLimit NudgeType = "context-limit"
	// NudgeTurn triggers based on conversation turn count (Task 4).
	NudgeTurn NudgeType = "turn"
	// NudgeIteration triggers based on iteration count (Task 4).
	NudgeIteration NudgeType = "iteration"
)

// PressureTier represents the severity of context window pressure, as defined
// by the parent lcm package. The nudge package defines its own type to avoid
// an import cycle.
type PressureTier int

const (
	// PressureLow indicates minimal pressure.
	PressureLow PressureTier = iota
	// PressureMedium indicates moderate pressure.
	PressureMedium
	// PressureHigh indicates critical pressure.
	PressureHigh
)

// PressureTierFunc determines the pressure tier from current token usage and
// context window. Callers typically provide lcm.CalculatePressureTier wrapped
// in a closure.
type PressureTierFunc func(currentTokens, contextWindow int64) PressureTier

// NudgeConfig controls nudge injection behaviour.
type NudgeConfig struct {
	// MinContextLimit is the minimum token count below which nudges are never
	// injected, regardless of pressure tier. Default: 50000.
	MinContextLimit int64
	// MaxContextLimit is the token count above which context-limit nudges are
	// injected when pressure is high. Default: 100000.
	MaxContextLimit int64
	// NudgeFrequency controls how often nudges are injected (every N turns).
	// Default: 5.
	NudgeFrequency int
	// IterationNudgeThreshold is the iteration count above which iteration
	// nudges are injected. Default: 15.
	IterationNudgeThreshold int
	// NudgeForce controls nudge intensity: "soft" (advisory) or "hard"
	// (directive). Default: "soft".
	NudgeForce string
}

// DefaultNudgeConfig returns NudgeConfig with standard defaults.
func DefaultNudgeConfig() NudgeConfig {
	return NudgeConfig{
		MinContextLimit:         50000,
		MaxContextLimit:         100000,
		NudgeFrequency:          5,
		IterationNudgeThreshold: 15,
		NudgeForce:              "soft",
	}
}

// NudgeInjector manages nudge injection into prompts. It follows the same
// structural pattern as GhostCue: a focused injector that decides whether to
// append nudge blocks based on runtime conditions.
type NudgeInjector struct {
	cfg    NudgeConfig
	tierFn PressureTierFunc
}

// NewNudgeInjector creates a NudgeInjector with the given configuration and
// pressure-tier function. If cfg is nil, defaults are applied. If tierFn is
// nil, a built-in default (95% threshold) is used.
func NewNudgeInjector(cfg *NudgeConfig, tierFn PressureTierFunc) *NudgeInjector {
	if cfg == nil {
		defaults := DefaultNudgeConfig()
		cfg = &defaults
	}
	if tierFn == nil {
		tierFn = defaultPressureTierFn
	}
	return &NudgeInjector{cfg: *cfg, tierFn: tierFn}
}

// defaultPressureTierFn replicates the lcm.DefaultPressureConfig thresholds
// without importing the parent package.
func defaultPressureTierFn(currentTokens, contextWindow int64) PressureTier {
	if contextWindow <= 0 {
		return PressureLow
	}
	pct := float64(currentTokens) / float64(contextWindow) * 100
	switch {
	case pct >= 95:
		return PressureHigh
	case pct >= 85:
		return PressureMedium
	default:
		return PressureLow
	}
}

// Config returns the active nudge configuration.
func (ni *NudgeInjector) Config() NudgeConfig {
	return ni.cfg
}

// InjectParams holds all parameters for nudge injection.
type InjectParams struct {
	Prompt         string
	CurrentTokens  int64
	ContextWindow  int64
	TurnCount      int
	IterationCount int
}

// Inject appends a nudge block to the prompt when the context-limit conditions
// are met. This is a convenience wrapper around InjectFull with zero
// turn/iteration counts.
func (ni *NudgeInjector) Inject(ctx context.Context, prompt string, currentTokens, contextWindow int64) (string, error) {
	return ni.InjectFull(ctx, InjectParams{
		Prompt:        prompt,
		CurrentTokens: currentTokens,
		ContextWindow: contextWindow,
	})
}

// InjectFull evaluates all nudge conditions (context-limit, turn, iteration)
// and appends matching nudge blocks to the prompt. Multiple nudges can stack.
func (ni *NudgeInjector) InjectFull(_ context.Context, params InjectParams) (string, error) {
	var blocks []string

	tier := ni.tierFn(params.CurrentTokens, params.ContextWindow)

	// Context-limit nudge requires PressureHigh and both token thresholds.
	if params.CurrentTokens >= ni.cfg.MinContextLimit &&
		params.CurrentTokens >= ni.cfg.MaxContextLimit &&
		tier == PressureHigh {
		blocks = append(blocks, ni.renderContextLimitBlock(params.CurrentTokens, params.ContextWindow))
	}

	if params.TurnCount > 0 &&
		params.TurnCount%ni.cfg.NudgeFrequency == 0 &&
		tier >= PressureMedium {
		blocks = append(blocks, fmt.Sprintf(
			`<nudge type="turn" force="%s">You are at turn %d. Consider summarizing progress periodically.</nudge>`,
			ni.cfg.NudgeForce, params.TurnCount,
		))
	}

	if params.IterationCount > ni.cfg.IterationNudgeThreshold &&
		tier >= PressureMedium {
		blocks = append(blocks, fmt.Sprintf(
			`<nudge type="iteration" force="%s">%d iterations without user input. Consider whether you're in a loop.</nudge>`,
			ni.cfg.NudgeForce, params.IterationCount,
		))
	}

	if len(blocks) == 0 {
		return params.Prompt, nil
	}

	var b strings.Builder
	b.WriteString(params.Prompt)
	for _, block := range blocks {
		b.WriteString("\n")
		b.WriteString(block)
	}
	b.WriteString("\n")
	return b.String(), nil
}

// renderContextLimitBlock produces the nudge XML block for context-limit
// warnings.
func (ni *NudgeInjector) renderContextLimitBlock(currentTokens, contextWindow int64) string {
	pct := float64(currentTokens) / float64(contextWindow) * 100
	force := ni.cfg.NudgeForce

	return fmt.Sprintf(
		`<nudge type="context-limit" force="%s">%s</nudge>`,
		force,
		fmt.Sprintf(
			"WARNING: Context usage at %.0f%% (%d/%d tokens). "+
				"You are approaching the context limit. Prioritize concise "+
				"responses, avoid repeating information, and consider summarizing "+
				"prior context to free tokens.",
			pct, currentTokens, contextWindow,
		),
	)
}
