package nudge

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextLimitNudge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentTokens int64
		contextWindow int64
		expectNudge   bool
		expectContent string
	}{
		{
			name:          "high pressure above max limit triggers nudge",
			currentTokens: 105000,
			contextWindow: 110000, // 95.4% usage → PressureHigh, exceeds MaxContextLimit(100000)
			expectNudge:   true,
			expectContent: "context-limit",
		},
		{
			name:          "high pressure at max limit triggers nudge",
			currentTokens: 100000,
			contextWindow: 105000, // ~95.2% → PressureHigh, tokens == MaxContextLimit
			expectNudge:   true,
			expectContent: "context-limit",
		},
		{
			name:          "high pressure below max limit no nudge",
			currentTokens: 95000,
			contextWindow: 100000, // 95% → PressureHigh, but tokens < MaxContextLimit(100000)
			expectNudge:   false,
		},
		{
			name:          "medium pressure no nudge regardless of tokens",
			currentTokens: 90000,
			contextWindow: 100000, // 90% → PressureMedium, even though > MaxContextLimit in raw count
			expectNudge:   false,
		},
		{
			name:          "low pressure no nudge",
			currentTokens: 50000,
			contextWindow: 100000, // 50% → PressureLow
			expectNudge:   false,
		},
		{
			name:          "zero tokens no nudge",
			currentTokens: 0,
			contextWindow: 100000,
			expectNudge:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultNudgeConfig()
			injector := NewNudgeInjector(&cfg, nil)

			prompt := "<system>base instructions</system>"
			result, err := injector.Inject(context.Background(), prompt, tt.currentTokens, tt.contextWindow)

			require.NoError(t, err)

			if tt.expectNudge {
				require.NotEqual(t, prompt, result, "prompt should be modified with nudge")
				require.True(t, strings.Contains(result, tt.expectContent),
					"nudge should contain %q, got: %s", tt.expectContent, result)
				require.True(t, strings.Contains(result, `<nudge`),
					"nudge should use XML-like <nudge> tags, got: %s", result)
				require.True(t, strings.Contains(result, `</nudge>`),
					"nudge should close XML-like tags, got: %s", result)
			} else {
				require.Equal(t, prompt, result, "prompt should be unchanged when no nudge triggered")
			}
		})
	}
}

func TestNoNudgeBelowMinContextLimit(t *testing.T) {
	t.Parallel()

	// Tokens are well below MinContextLimit (50000) — no nudge even at high pressure.
	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	prompt := "<system>instructions</system>"
	result, err := injector.Inject(context.Background(), prompt, 30000, 31000)

	require.NoError(t, err)
	require.Equal(t, prompt, result, "should not nudge when tokens below MinContextLimit")
}

func TestNudgeContainsTokenWarning(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	prompt := "<system>instructions</system>"
	// 105000/110000 = 95.4% → PressureHigh, 105000 > MaxContextLimit(100000)
	result, err := injector.Inject(context.Background(), prompt, 105000, 110000)
	require.NoError(t, err)

	require.Contains(t, result, "context-limit")
	require.Contains(t, result, "tokens")
}

func TestDefaultNudgeConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	require.Equal(t, int64(50000), cfg.MinContextLimit)
	require.Equal(t, int64(100000), cfg.MaxContextLimit)
	require.Equal(t, 5, cfg.NudgeFrequency)
	require.Equal(t, 15, cfg.IterationNudgeThreshold)
	require.Equal(t, "soft", cfg.NudgeForce)
}

func TestNudgeConfig_Custom(t *testing.T) {
	t.Parallel()

	cfg := NudgeConfig{
		MinContextLimit:         30000,
		MaxContextLimit:         80000,
		NudgeFrequency:          3,
		IterationNudgeThreshold: 10,
		NudgeForce:              "hard",
	}
	require.Equal(t, int64(30000), cfg.MinContextLimit)
	require.Equal(t, int64(80000), cfg.MaxContextLimit)
	require.Equal(t, 3, cfg.NudgeFrequency)
	require.Equal(t, 10, cfg.IterationNudgeThreshold)
	require.Equal(t, "hard", cfg.NudgeForce)
}

func TestContextLimitNudge_NudgeBlockFormat(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	prompt := "<system>instructions</system>"
	result, err := injector.Inject(context.Background(), prompt, 105000, 110000)
	require.NoError(t, err)

	// Verify the nudge block format: <nudge type="context-limit" force="soft">...</nudge>
	require.Contains(t, result, `<nudge type="context-limit"`)
	require.Contains(t, result, `</nudge>`)

	// Verify the nudge appears after the original prompt.
	promptEnd := strings.LastIndex(result, "</system>")
	nudgeStart := strings.Index(result, "<nudge")
	require.True(t, nudgeStart > promptEnd,
		"nudge should appear after the prompt content, promptEnd=%d, nudgeStart=%d",
		promptEnd, nudgeStart)
}

func TestNudgeTypeConstants(t *testing.T) {
	t.Parallel()

	require.Equal(t, NudgeType("context-limit"), NudgeContextLimit)
	require.Equal(t, NudgeType("turn"), NudgeTurn)
	require.Equal(t, NudgeType("iteration"), NudgeIteration)
}

func TestNudgeInjector_UsesPressureTier(t *testing.T) {
	t.Parallel()

	// Verify that the nudge system uses pressure tier calculation correctly.
	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	// At exactly 100% with a 100000 window → PressureHigh, tokens 100000 == MaxContextLimit.
	result, err := injector.Inject(context.Background(), "prompt", 100000, 100000)
	require.NoError(t, err)

	// 100% usage → PressureHigh, tokens >= MaxContextLimit → nudge.
	require.Contains(t, result, "context-limit")
}

func TestNudgeInjector_CustomMaxContextLimit(t *testing.T) {
	t.Parallel()

	cfg := NudgeConfig{
		MinContextLimit:         50000,
		MaxContextLimit:         80000,
		NudgeFrequency:          5,
		IterationNudgeThreshold: 15,
		NudgeForce:              "soft",
	}
	injector := NewNudgeInjector(&cfg, nil)

	// 85000/90000 = 94.4% → not quite PressureHigh with default thresholds,
	// but let's use 86000/90000 = 95.5% → PressureHigh.
	// 86000 > MaxContextLimit(80000) → should nudge.
	result, err := injector.Inject(context.Background(), "prompt", 86000, 90000)
	require.NoError(t, err)
	require.Contains(t, result, "context-limit")
}

func TestNudgeInjector_RespectsMinContextLimit(t *testing.T) {
	t.Parallel()

	// Custom config with high MinContextLimit.
	cfg := NudgeConfig{
		MinContextLimit:         80000,
		MaxContextLimit:         100000,
		NudgeFrequency:          5,
		IterationNudgeThreshold: 15,
		NudgeForce:              "soft",
	}
	injector := NewNudgeInjector(&cfg, nil)

	// 75000 tokens is below MinContextLimit(80000), even though pressure is high.
	result, err := injector.Inject(context.Background(), "prompt", 75000, 78000)
	require.NoError(t, err)
	require.Equal(t, "prompt", result, "should not nudge below MinContextLimit")
}

func TestNudgeInjector_NilConfig(t *testing.T) {
	t.Parallel()

	// Nil config should use defaults and work correctly.
	injector := NewNudgeInjector(nil, nil)
	require.NotNil(t, injector)

	result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
	require.NoError(t, err)
	require.Contains(t, result, "context-limit")
}

func TestNudgeBlockContent(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
	require.NoError(t, err)

	// The nudge should contain actionable guidance.
	require.Contains(t, result, "context-limit")
	require.Contains(t, result, "tokens")
	// Should contain a warning about approaching context limits.
	require.True(t,
		strings.Contains(result, "approaching") || strings.Contains(result, "exceeded") || strings.Contains(result, "limit"),
		"nudge should contain limit-related warning text")
}

// --- Turn-Nudge tests (Task 4) ---

func TestTurnNudge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		turnCount     int
		expectNudge   bool
		expectContent string
	}{
		{
			name:        "turn 5 triggers nudge (frequency 5)",
			turnCount:   5,
			expectNudge: true,
		},
		{
			name:        "turn 10 triggers nudge (frequency 5)",
			turnCount:   10,
			expectNudge: true,
		},
		{
			name:        "turn 15 triggers nudge (frequency 5)",
			turnCount:   15,
			expectNudge: true,
		},
		{
			name:        "turn 1 no nudge",
			turnCount:   1,
			expectNudge: false,
		},
		{
			name:        "turn 2 no nudge",
			turnCount:   2,
			expectNudge: false,
		},
		{
			name:        "turn 3 no nudge",
			turnCount:   3,
			expectNudge: false,
		},
		{
			name:        "turn 4 no nudge",
			turnCount:   4,
			expectNudge: false,
		},
		{
			name:        "turn 6 no nudge",
			turnCount:   6,
			expectNudge: false,
		},
		{
			name:        "turn 9 no nudge",
			turnCount:   9,
			expectNudge: false,
		},
		{
			name:        "turn 0 no nudge",
			turnCount:   0,
			expectNudge: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultNudgeConfig()
			// Use a custom tierFn that returns Medium to allow turn/iteration
			// nudges (they require Medium or higher).
			mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
			injector := NewNudgeInjector(&cfg, mediumFn)

			prompt := "<system>base instructions</system>"
			params := InjectParams{
				Prompt:        prompt,
				CurrentTokens: 1000,
				ContextWindow: 100000,
				TurnCount:     tt.turnCount,
			}
			result, err := injector.InjectFull(context.Background(), params)
			require.NoError(t, err)

			if tt.expectNudge {
				require.NotEqual(t, prompt, result,
					"prompt should be modified with turn-nudge")
				require.Contains(t, result, `type="turn"`,
					"nudge should contain turn type attribute")
				require.Contains(t, result, `</nudge>`,
					"nudge should close XML tags")
			} else {
				require.Equal(t, prompt, result,
					"prompt should be unchanged when turn-nudge not triggered")
			}
		})
	}
}

func TestTurnNudge_NotInjectedAtLowPressure(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	// Force Low pressure — turn nudges should NOT fire even at turn 5.
	lowFn := func(_, _ int64) PressureTier { return PressureLow }
	injector := NewNudgeInjector(&cfg, lowFn)

	prompt := "instructions"
	params := InjectParams{
		Prompt:        prompt,
		CurrentTokens: 1000,
		ContextWindow: 100000,
		TurnCount:     5,
	}
	result, err := injector.InjectFull(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, prompt, result,
		"turn-nudge should not fire at Low pressure")
}

func TestTurnNudge_CustomFrequency(t *testing.T) {
	t.Parallel()

	cfg := NudgeConfig{
		NudgeFrequency:          3,
		IterationNudgeThreshold: 15,
		NudgeForce:              "soft",
	}
	mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
	injector := NewNudgeInjector(&cfg, mediumFn)

	prompt := "prompt"

	// Turn 3 should trigger with frequency 3.
	result, err := injector.InjectFull(context.Background(), InjectParams{
		Prompt: prompt, CurrentTokens: 1000, ContextWindow: 100000, TurnCount: 3,
	})
	require.NoError(t, err)
	require.Contains(t, result, `type="turn"`)

	// Turn 6 should also trigger.
	result, err = injector.InjectFull(context.Background(), InjectParams{
		Prompt: prompt, CurrentTokens: 1000, ContextWindow: 100000, TurnCount: 6,
	})
	require.NoError(t, err)
	require.Contains(t, result, `type="turn"`)

	// Turn 5 should NOT trigger with frequency 3.
	result, err = injector.InjectFull(context.Background(), InjectParams{
		Prompt: prompt, CurrentTokens: 1000, ContextWindow: 100000, TurnCount: 5,
	})
	require.NoError(t, err)
	require.Equal(t, prompt, result)
}

// --- Iteration-Nudge tests (Task 4) ---

func TestIterationNudge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		iterationCount int
		expectNudge    bool
		expectTypeAttr string
	}{
		{
			name:           "16 iterations triggers nudge (threshold 15)",
			iterationCount: 16,
			expectNudge:    true,
		},
		{
			name:           "20 iterations triggers nudge",
			iterationCount: 20,
			expectNudge:    true,
		},
		{
			name:           "15 iterations no nudge (must be > threshold)",
			iterationCount: 15,
			expectNudge:    false,
		},
		{
			name:           "10 iterations no nudge",
			iterationCount: 10,
			expectNudge:    false,
		},
		{
			name:           "0 iterations no nudge",
			iterationCount: 0,
			expectNudge:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultNudgeConfig()
			mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
			injector := NewNudgeInjector(&cfg, mediumFn)

			prompt := "<system>base instructions</system>"
			params := InjectParams{
				Prompt:         prompt,
				CurrentTokens:  1000,
				ContextWindow:  100000,
				IterationCount: tt.iterationCount,
			}
			result, err := injector.InjectFull(context.Background(), params)
			require.NoError(t, err)

			if tt.expectNudge {
				require.NotEqual(t, prompt, result,
					"prompt should be modified with iteration-nudge")
				require.Contains(t, result, `type="iteration"`,
					"nudge should contain iteration type attribute")
				require.Contains(t, result, `</nudge>`,
					"nudge should close XML tags")
			} else {
				require.Equal(t, prompt, result,
					"prompt should be unchanged when iteration-nudge not triggered")
			}
		})
	}
}

func TestIterationNudge_NotInjectedAtLowPressure(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	lowFn := func(_, _ int64) PressureTier { return PressureLow }
	injector := NewNudgeInjector(&cfg, lowFn)

	prompt := "instructions"
	params := InjectParams{
		Prompt:         prompt,
		CurrentTokens:  1000,
		ContextWindow:  100000,
		IterationCount: 20,
	}
	result, err := injector.InjectFull(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, prompt, result,
		"iteration-nudge should not fire at Low pressure")
}

func TestIterationNudge_CustomThreshold(t *testing.T) {
	t.Parallel()

	cfg := NudgeConfig{
		NudgeFrequency:          5,
		IterationNudgeThreshold: 10,
		NudgeForce:              "soft",
	}
	mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
	injector := NewNudgeInjector(&cfg, mediumFn)

	prompt := "prompt"

	// 11 iterations > threshold 10 → nudge.
	result, err := injector.InjectFull(context.Background(), InjectParams{
		Prompt: prompt, CurrentTokens: 1000, ContextWindow: 100000, IterationCount: 11,
	})
	require.NoError(t, err)
	require.Contains(t, result, `type="iteration"`)

	// 10 iterations == threshold → no nudge (must be strictly >).
	result, err = injector.InjectFull(context.Background(), InjectParams{
		Prompt: prompt, CurrentTokens: 1000, ContextWindow: 100000, IterationCount: 10,
	})
	require.NoError(t, err)
	require.Equal(t, prompt, result)
}

// --- NoNudgeLowPressure: verifies zero advisory nudges at Low pressure ---

func TestNoNudgeLowPressure(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	lowFn := func(_, _ int64) PressureTier { return PressureLow }
	injector := NewNudgeInjector(&cfg, lowFn)

	prompt := "instructions"

	// Even with high turn count and high iteration count, Low pressure means
	// no turn or iteration nudges.
	result, err := injector.InjectFull(context.Background(), InjectParams{
		Prompt:         prompt,
		CurrentTokens:  1000,
		ContextWindow:  100000,
		TurnCount:      25,
		IterationCount: 50,
	})
	require.NoError(t, err)
	require.Equal(t, prompt, result,
		"no advisory nudges should fire at Low pressure")
}

// --- Stacking: both turn and iteration nudges can coexist ---

func TestTurnAndIterationNudgeStack(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
	injector := NewNudgeInjector(&cfg, mediumFn)

	prompt := "prompt"
	params := InjectParams{
		Prompt:         prompt,
		CurrentTokens:  1000,
		ContextWindow:  100000,
		TurnCount:      10, // 10 % 5 == 0 → turn-nudge
		IterationCount: 20, // 20 > 15 → iteration-nudge
	}
	result, err := injector.InjectFull(context.Background(), params)
	require.NoError(t, err)

	require.Contains(t, result, `type="turn"`,
		"should contain turn-nudge block")
	require.Contains(t, result, `type="iteration"`,
		"should contain iteration-nudge block")
}

// --- InjectFull context-limit still works alongside turn/iteration ---

func TestInjectFull_ContextLimitWithTurnAndIteration(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	// High pressure triggers context-limit nudge.
	highFn := func(_, _ int64) PressureTier { return PressureHigh }
	injector := NewNudgeInjector(&cfg, highFn)

	prompt := "prompt"
	params := InjectParams{
		Prompt:         prompt,
		CurrentTokens:  105000, // >= MaxContextLimit
		ContextWindow:  110000, // high pressure
		TurnCount:      10,
		IterationCount: 20,
	}
	result, err := injector.InjectFull(context.Background(), params)
	require.NoError(t, err)

	// All three nudge types should be present.
	require.Contains(t, result, `type="context-limit"`,
		"should contain context-limit nudge")
	require.Contains(t, result, `type="turn"`,
		"should contain turn-nudge")
	require.Contains(t, result, `type="iteration"`,
		"should contain iteration-nudge")
}

// --- Inject delegates to InjectFull ---

func TestInject_DelegatesToInjectFull(t *testing.T) {
	t.Parallel()

	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	// The legacy Inject() method should behave identically to InjectFull
	// with zero turn/iteration counts.
	prompt := "prompt"
	result1, err1 := injector.Inject(context.Background(), prompt, 105000, 110000)
	require.NoError(t, err1)

	result2, err2 := injector.InjectFull(context.Background(), InjectParams{
		Prompt:        prompt,
		CurrentTokens: 105000,
		ContextWindow: 110000,
	})
	require.NoError(t, err2)

	require.Equal(t, result1, result2,
		"Inject() should produce same result as InjectFull() with zero counts")
}

// Verify integration: the nudge package's pressure calculation is consistent
// with the lcm.DefaultPressureConfig thresholds (70/85/95).
func TestNudgeUsesConsistentPressureThresholds(t *testing.T) {
	t.Parallel()

	// 96% should trigger PressureHigh in the nudge package's default function.
	cfg := DefaultNudgeConfig()
	injector := NewNudgeInjector(&cfg, nil)

	// 105000/110000 = 95.4% → PressureHigh via default thresholds.
	result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
	require.NoError(t, err)
	require.Contains(t, result, "context-limit")

	// 89% should NOT trigger PressureHigh.
	result2, err := injector.Inject(context.Background(), "prompt", 89000, 100000)
	require.NoError(t, err)
	require.Equal(t, "prompt", result2)
}

func TestNudgeConfigConsumed(t *testing.T) {
	t.Parallel()

	t.Run("default config allows nudges at standard thresholds", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultNudgeConfig()
		injector := NewNudgeInjector(&cfg, nil)

		result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
		require.NoError(t, err)
		require.Contains(t, result, "context-limit",
			"default config should allow nudge when tokens >= MaxContextLimit and pressure is high")
	})

	t.Run("custom MinContextLimit raises the floor", func(t *testing.T) {
		t.Parallel()

		cfg := NudgeConfig{
			MinContextLimit:         90000,
			MaxContextLimit:         100000,
			NudgeFrequency:          5,
			IterationNudgeThreshold: 15,
			NudgeForce:              "soft",
		}
		injector := NewNudgeInjector(&cfg, nil)

		result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
		require.NoError(t, err)
		require.Contains(t, result, "context-limit",
			"tokens above custom MinContextLimit should nudge")

		result2, err := injector.Inject(context.Background(), "prompt", 89099, 110000)
		require.NoError(t, err)
		require.Equal(t, "prompt", result2,
			"tokens below custom MinContextLimit should not nudge")
	})

	t.Run("custom MaxContextLimit controls the ceiling", func(t *testing.T) {
		t.Parallel()

		cfg := NudgeConfig{
			MinContextLimit:         50000,
			MaxContextLimit:         80000,
			NudgeFrequency:          5,
			IterationNudgeThreshold: 15,
			NudgeForce:              "hard",
		}
		highFn := func(_, _ int64) PressureTier { return PressureHigh }
		injector := NewNudgeInjector(&cfg, highFn)

		result, err := injector.Inject(context.Background(), "prompt", 85000, 90000)
		require.NoError(t, err)
		require.Contains(t, result, "context-limit")
		require.Contains(t, result, `force="hard"`,
			"custom NudgeForce should be used in output")
	})

	t.Run("custom NudgeFrequency controls turn nudges", func(t *testing.T) {
		t.Parallel()

		cfg := NudgeConfig{
			MinContextLimit:         50000,
			MaxContextLimit:         100000,
			NudgeFrequency:          3,
			IterationNudgeThreshold: 15,
			NudgeForce:              "soft",
		}
		mediumFn := func(_, _ int64) PressureTier { return PressureMedium }
		injector := NewNudgeInjector(&cfg, mediumFn)

		result, err := injector.InjectFull(context.Background(), InjectParams{
			Prompt:        "prompt",
			CurrentTokens: 90000,
			ContextWindow: 100000,
			TurnCount:     2,
		})
		require.NoError(t, err)
		require.Equal(t, "prompt", result,
			"turn 2 should not nudge with frequency 3")

		result2, err := injector.InjectFull(context.Background(), InjectParams{
			Prompt:        "prompt",
			CurrentTokens: 90000,
			ContextWindow: 100000,
			TurnCount:     3,
		})
		require.NoError(t, err)
		require.Contains(t, result2, `type="turn"`,
			"turn 3 should nudge with frequency 3")
	})

	t.Run("nil config uses defaults", func(t *testing.T) {
		t.Parallel()

		injector := NewNudgeInjector(nil, nil)

		result, err := injector.Inject(context.Background(), "prompt", 105000, 110000)
		require.NoError(t, err)
		require.Contains(t, result, "context-limit",
			"nil config should use defaults and still nudge at standard thresholds")
	})
}
