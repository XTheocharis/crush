// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"context"
	"os"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	"github.com/charmbracelet/crush/internal/config"
)

// LLMClient matches the interface used by lcm.Summarizer and explorer.Registry.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// fantasyAdapter wraps a fantasy.LanguageModel to implement LLMClient.
type fantasyAdapter struct {
	model fantasy.LanguageModel
}

func (a *fantasyAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := a.model.Generate(ctx, fantasy.Call{
		Prompt: fantasy.Prompt{
			fantasy.NewSystemMessage(systemPrompt),
			fantasy.NewUserMessage(userPrompt),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content.Text(), nil
}

// SkipIfNoIntegration skips the test unless CRUSH_INTEGRATION=1 is set.
func SkipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("CRUSH_INTEGRATION") == "" {
		t.Skip("skipping integration test (set CRUSH_INTEGRATION=1 to run)")
	}
}

// NewLLMClient loads the developer config and creates a real LLM client
// from the configured small model provider. Requires CRUSH_GLOBAL_CONFIG
// to point at the developer config directory (e.g., ../xrush).
func NewLLMClient(t *testing.T) LLMClient {
	t.Helper()

	store := LoadDevConfig(t)
	cfg := store.Config()

	smallModel, ok := cfg.Models[config.SelectedModelTypeSmall]
	if !ok {
		t.Fatal("no small model configured in dev config")
	}

	providerCfg, ok := cfg.Providers.Get(smallModel.Provider)
	if !ok {
		t.Fatalf("provider %q not found in dev config", smallModel.Provider)
	}

	provider, err := openaicompat.New(
		openaicompat.WithBaseURL(providerCfg.BaseURL),
		openaicompat.WithAPIKey(providerCfg.APIKey),
	)
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	model, err := provider.LanguageModel(t.Context(), smallModel.Model)
	if err != nil {
		t.Fatalf("getting language model %q: %v", smallModel.Model, err)
	}

	return &fantasyAdapter{model: model}
}

// LoadDevConfig loads the developer config from CRUSH_GLOBAL_CONFIG.
func LoadDevConfig(t *testing.T) *config.ConfigStore {
	t.Helper()

	globalConfig := os.Getenv("CRUSH_GLOBAL_CONFIG")
	if globalConfig == "" {
		t.Fatal("CRUSH_GLOBAL_CONFIG must be set for integration tests")
	}

	// Use a temp working dir so project-local configs don't interfere.
	cfg, err := config.Load(t.TempDir(), "", false)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	return cfg
}
