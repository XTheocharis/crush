package repomap

import (
	"context"
	"math"
	"strings"
)

// TokenCounter counts tokens for arbitrary text.
type TokenCounter interface {
	Count(ctx context.Context, model string, text string) (int, error)
}

// TokenCounterProvider resolves a TokenCounter for a model family.
type TokenCounterProvider interface {
	CounterForModel(model string) (TokenCounter, bool)
	MetadataForModel(model string) (TokenizerMetadata, bool)
}

// TokenizerMetadata carries provenance fields for parity tuple artifacts.
type TokenizerMetadata struct {
	TokenizerID      string
	TokenizerVersion string
	Supported        bool
}

// ParityTokenMetrics contains parity/safety split counts.
type ParityTokenMetrics struct {
	ParityTokens float64
	SafetyTokens int
}

var charsPerToken = map[string]float64{
	"go":         3.2,
	"rust":       3.2,
	"c":          3.2,
	"cpp":        3.2,
	"c++":        3.2,
	"python":     3.8,
	"ruby":       3.8,
	"java":       3.4,
	"csharp":     3.4,
	"kotlin":     3.4,
	"javascript": 3.5,
	"typescript": 3.5,
	"html":       2.8,
	"xml":        2.8,
	"svg":        2.8,
	"json":       3.0,
	"yaml":       3.0,
	"toml":       3.0,
	"default":    3.5,
}

// EstimateTokens returns ceiling(len(text)/ratio). For rendered mixed-language
// map output, callers should pass default/unknown language.
func EstimateTokens(text, lang string) int {
	ratio := charsPerToken["default"]
	if r, ok := charsPerToken[strings.ToLower(strings.TrimSpace(lang))]; ok && r > 0 {
		ratio = r
	}
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / ratio))
}

// CountParityAndSafetyTokens computes parity tokens and safety tokens.
// - parity_tokens: tokenizer-backed when available, else heuristic estimate.
// - safety_tokens: max(parity_tokens_ceiled, ceil(heuristic*1.15)).
func CountParityAndSafetyTokens(
	ctx context.Context,
	counter TokenCounter,
	model string,
	text string,
	lang string,
) (ParityTokenMetrics, error) {
	parity := float64(EstimateTokens(text, lang))
	if counter != nil {
		tok, err := counter.Count(ctx, model, text)
		if err != nil {
			return ParityTokenMetrics{}, err
		}
		parity = float64(tok)
	}

	heuristic := float64(EstimateTokens(text, lang))
	safety := int(math.Ceil(math.Max(math.Ceil(parity), math.Ceil(heuristic*1.15))))
	return ParityTokenMetrics{ParityTokens: parity, SafetyTokens: safety}, nil
}
