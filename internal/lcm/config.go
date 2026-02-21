package lcm

// BudgetConfig holds configuration for token budget computation.
type BudgetConfig struct {
	ContextWindow      int64
	CutoffThreshold    float64
	SystemPromptTokens int64
	ToolTokens         int64
	ModelOutputLimit   int64
}

// ComputeBudget computes the soft and hard token thresholds.
//
// Formula:
//
//	overhead = systemPromptTokens + toolTokens
//	outputReserve = min(20000, contextWindow * 0.25)
//	hardLimit = contextWindow - overhead - outputReserve
//	softRaw = contextWindow * cutoffThreshold - overhead
//	softThreshold = max(0, min(softRaw, hardLimit))
func ComputeBudget(cfg BudgetConfig) Budget {
	overhead := cfg.SystemPromptTokens + cfg.ToolTokens
	outputReserve := int64(20000)
	if reserve25 := int64(float64(cfg.ContextWindow) * 0.25); reserve25 < outputReserve {
		outputReserve = reserve25
	}
	if cfg.ModelOutputLimit > 0 && cfg.ModelOutputLimit < outputReserve {
		outputReserve = cfg.ModelOutputLimit
	}
	hardLimit := max(cfg.ContextWindow-overhead-outputReserve, 0)
	softRaw := int64(float64(cfg.ContextWindow)*cfg.CutoffThreshold) - overhead
	softThreshold := min(max(softRaw, 0), hardLimit)
	return Budget{
		SoftThreshold: softThreshold,
		HardLimit:     hardLimit,
		ContextWindow: cfg.ContextWindow,
	}
}

// EstimateTokens estimates token count from string content.
// Uses ceiling division by CharsPerToken.
func EstimateTokens(s string) int64 {
	chars := int64(len([]rune(s)))
	return (chars + CharsPerToken - 1) / CharsPerToken
}
