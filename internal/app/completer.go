package app

import (
	"context"
	"fmt"
	"log/slog"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
)

func newTextCompleter(store *config.ConfigStore) ext.TextCompleter {
	cfg := store.Config()

	providerCfg := cfg.GetProviderForModel(config.SelectedModelTypeSmall)
	if providerCfg == nil {
		return nil
	}

	modelCfg, ok := cfg.Models[config.SelectedModelTypeSmall]
	if !ok {
		return nil
	}

	apiKey, _ := store.Resolve(providerCfg.APIKey)
	baseURL, _ := store.Resolve(providerCfg.BaseURL)

	provider, err := buildScrubberProvider(providerCfg, apiKey, baseURL)
	if err != nil {
		slog.Debug("Scrubber provider not available", "error", err)
		return nil
	}

	return func(ctx context.Context, prompt, input string) (string, error) {
		lm, err := provider.LanguageModel(ctx, modelCfg.Model)
		if err != nil {
			return "", err
		}
		resp, err := lm.Generate(ctx, fantasy.Call{
			Prompt: []fantasy.Message{
				fantasy.NewSystemMessage(prompt),
				fantasy.NewUserMessage(input),
			},
		})
		if err != nil {
			return "", err
		}
		return resp.Content.Text(), nil
	}
}

func buildScrubberProvider(cfg *config.ProviderConfig, apiKey, baseURL string) (fantasy.Provider, error) {
	switch cfg.Type {
	case openai.Name:
		opts := []openai.Option{openai.WithAPIKey(apiKey)}
		if baseURL != "" {
			opts = append(opts, openai.WithBaseURL(baseURL))
		}
		return openai.New(opts...)
	case anthropic.Name:
		var opts []anthropic.Option
		if apiKey != "" {
			opts = append(opts, anthropic.WithAPIKey(apiKey))
		}
		if baseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(baseURL))
		}
		return anthropic.New(opts...)
	case openrouter.Name:
		return openrouter.New(openrouter.WithAPIKey(apiKey))
	case openaicompat.Name:
		opts := []openaicompat.Option{
			openaicompat.WithAPIKey(apiKey),
		}
		if baseURL != "" {
			opts = append(opts, openaicompat.WithBaseURL(baseURL))
		}
		return openaicompat.New(opts...)
	default:
		return nil, fmt.Errorf("unsupported scrubber provider type: %q", cfg.Type)
	}
}
