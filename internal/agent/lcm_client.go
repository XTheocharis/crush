package agent

import (
	"context"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/lcm"
)

type lcmLLMAdapter struct {
	model              Model
	providerOptions    fantasy.ProviderOptions
	systemPromptPrefix string
	maxOutputTokens    *int64
}

var _ lcm.LLMClient = (*lcmLLMAdapter)(nil)

func newLCMLLMClient(model Model, providerCfg config.ProviderConfig) lcm.LLMClient {
	var maxOutputTokens *int64
	tokens := model.CatwalkCfg.DefaultMaxTokens
	if model.ModelCfg.MaxTokens > 0 {
		tokens = model.ModelCfg.MaxTokens
	}
	if tokens > 0 {
		maxOutputTokens = &tokens
	}

	return &lcmLLMAdapter{
		model:              model,
		providerOptions:    getProviderOptions(model, providerCfg),
		systemPromptPrefix: providerCfg.SystemPromptPrefix,
		maxOutputTokens:    maxOutputTokens,
	}
}

func (a *lcmLLMAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
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
