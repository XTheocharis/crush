package agent

import (
	"context"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
)

// LCMLLMAdapter wraps a resolved Model and provider configuration into a
// lightweight LLM client suitable for LCM summarization calls. It implements
// the Complete(ctx, systemPrompt, userPrompt) (string, error) signature
// expected by the LCM summarizer without importing the lcm package directly
// (the extension layer bridges the two).
type LCMLLMAdapter struct {
	model              Model
	providerOptions    fantasy.ProviderOptions
	systemPromptPrefix string
	maxOutputTokens    *int64
}

// NewLCMLLMClient constructs an LLM adapter for LCM summarization from a
// resolved Model and its provider configuration.
func NewLCMLLMClient(model Model, providerCfg config.ProviderConfig) *LCMLLMAdapter {
	var maxOutputTokens *int64
	tokens := model.CatwalkCfg.DefaultMaxTokens
	if model.ModelCfg.MaxTokens > 0 {
		tokens = model.ModelCfg.MaxTokens
	}
	// Cap output tokens for summarization. The context window is shared
	// between input and output, so large output budgets starve the input
	// and cause "Prompt exceeds max length" errors from the provider.
	// Allow at most 1/3 of the context window for output.
	if cw := model.CatwalkCfg.ContextWindow; cw > 0 {
		if maxCap := cw / 3; tokens > maxCap {
			tokens = maxCap
		}
	}
	if tokens > 0 {
		maxOutputTokens = &tokens
	}

	return &LCMLLMAdapter{
		model:              model,
		providerOptions:    getProviderOptions(model, providerCfg),
		systemPromptPrefix: providerCfg.SystemPromptPrefix,
		maxOutputTokens:    maxOutputTokens,
	}
}

// Complete sends a system+user prompt pair through the wrapped LLM and returns
// the text response. This satisfies the LCM LLMClient interface.
func (a *LCMLLMAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	prompt := make(fantasy.Prompt, 0, 3)
	if a.systemPromptPrefix != "" {
		prompt = append(prompt, fantasy.NewSystemMessage(a.systemPromptPrefix))
	}
	prompt = append(prompt,
		fantasy.NewSystemMessage(systemPrompt),
		fantasy.NewUserMessage(userPrompt),
	)

	resp, err := a.model.Model.Generate(ctx, fantasy.Call{
		Prompt:           prompt,
		MaxOutputTokens:  a.maxOutputTokens,
		Temperature:      a.model.ModelCfg.Temperature,
		TopP:             a.model.ModelCfg.TopP,
		TopK:             a.model.ModelCfg.TopK,
		PresencePenalty:  a.model.ModelCfg.PresencePenalty,
		FrequencyPenalty: a.model.ModelCfg.FrequencyPenalty,
		UserAgent:        userAgent,
		ProviderOptions:  a.providerOptions,
	})
	if err != nil {
		return "", err
	}
	return resp.Content.Text(), nil
}
