package agent

import (
	"context"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

type fakeLanguageModel struct {
	lastCall fantasy.Call
	response *fantasy.Response
	err      error
}

func (f *fakeLanguageModel) Generate(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
	f.lastCall = call
	if f.err != nil {
		return nil, f.err
	}
	if f.response != nil {
		return f.response, nil
	}
	return &fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.TextContent{Text: "ok"},
		},
	}, nil
}

func (f *fakeLanguageModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, nil
}

func (f *fakeLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, nil
}

func (f *fakeLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, nil
}

func (f *fakeLanguageModel) Provider() string { return "fake" }
func (f *fakeLanguageModel) Model() string    { return "fake-model" }

func TestNewLCMLLMClientCompleteUsesConfiguredMessages(t *testing.T) {
	t.Parallel()

	temp := 0.25
	topP := 0.9
	maxTokens := int64(777)

	model := &fakeLanguageModel{
		response: &fantasy.Response{
			Content: fantasy.ResponseContent{
				fantasy.TextContent{Text: "summary output"},
			},
		},
	}

	client := newLCMLLMClient(Model{
		Model: model,
		CatwalkCfg: catwalk.Model{
			DefaultMaxTokens: 1234,
		},
		ModelCfg: config.SelectedModel{
			Provider:    "openai",
			Model:       "gpt-4.1-mini",
			MaxTokens:   maxTokens,
			Temperature: &temp,
			TopP:        &topP,
		},
	}, config.ProviderConfig{
		ID:                 "openai",
		Type:               "openai",
		SystemPromptPrefix: "provider prefix",
	})

	text, err := client.Complete(t.Context(), "system prompt", "user prompt")
	require.NoError(t, err)
	require.Equal(t, "summary output", text)
	require.Len(t, model.lastCall.Prompt, 3)
	require.Equal(t, fantasy.MessageRoleSystem, model.lastCall.Prompt[0].Role)
	require.Equal(t, "provider prefix", model.lastCall.Prompt[0].Content[0].(fantasy.TextPart).Text)
	require.Equal(t, "system prompt", model.lastCall.Prompt[1].Content[0].(fantasy.TextPart).Text)
	require.Equal(t, "user prompt", model.lastCall.Prompt[2].Content[0].(fantasy.TextPart).Text)
	require.NotNil(t, model.lastCall.MaxOutputTokens)
	require.Equal(t, maxTokens, *model.lastCall.MaxOutputTokens)
	require.Equal(t, &temp, model.lastCall.Temperature)
	require.Equal(t, &topP, model.lastCall.TopP)
}

func TestLCMSummarizerSelectionDefaultsToLargeModel(t *testing.T) {
	t.Parallel()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)

	store.Config().Providers.Set("openai", config.ProviderConfig{
		ID:   "openai",
		Type: "openai",
		Models: []catwalk.Model{
			{ID: "gpt-4.1", DefaultMaxTokens: 8000, ContextWindow: 200000},
			{ID: "gpt-4.1-mini", DefaultMaxTokens: 4000, ContextWindow: 128000},
		},
	})
	store.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1",
	}
	store.Config().Models[config.SelectedModelTypeSmall] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1-mini",
	}
	store.Config().Options.LCM = &config.LCMOptions{}

	c := &coordinator{cfg: store}
	modelCfg, providerCfg, catwalkModel, err := c.lcmSummarizerSelection()
	require.NoError(t, err)
	require.Equal(t, "openai", modelCfg.Provider)
	require.Equal(t, "gpt-4.1", modelCfg.Model)
	require.Equal(t, "openai", providerCfg.ID)
	require.Equal(t, "gpt-4.1", catwalkModel.ID)
}

func TestLCMSummarizerSelectionUsesConfiguredModelWhenContextIsSufficient(t *testing.T) {
	t.Parallel()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)

	store.Config().Providers.Set("openai", config.ProviderConfig{
		ID:   "openai",
		Type: "openai",
		Models: []catwalk.Model{
			{ID: "gpt-4.1", DefaultMaxTokens: 8000, ContextWindow: 200000},
			{ID: "gpt-4.1-mini", DefaultMaxTokens: 4000, ContextWindow: 128000},
			{ID: "gpt-4.1-nano", DefaultMaxTokens: 2000, ContextWindow: 200000},
		},
	})
	store.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1",
	}
	store.Config().Models[config.SelectedModelTypeSmall] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1-mini",
	}
	store.Config().Options.LCM = &config.LCMOptions{
		SummarizerModel: &config.SelectedModel{
			Provider: "openai",
			Model:    "gpt-4.1-nano",
		},
	}

	c := &coordinator{cfg: store}
	modelCfg, _, catwalkModel, err := c.lcmSummarizerSelection()
	require.NoError(t, err)
	require.Equal(t, "gpt-4.1-nano", modelCfg.Model)
	require.Equal(t, "gpt-4.1-nano", catwalkModel.ID)
}

func TestLCMSummarizerSelectionFallsBackToLargeModelWhenConfiguredContextIsTooSmall(t *testing.T) {
	t.Parallel()

	store, err := config.Init(t.TempDir(), "", false)
	require.NoError(t, err)

	store.Config().Providers.Set("openai", config.ProviderConfig{
		ID:   "openai",
		Type: "openai",
		Models: []catwalk.Model{
			{ID: "gpt-4.1", DefaultMaxTokens: 8000, ContextWindow: 200000},
			{ID: "gpt-4.1-mini", DefaultMaxTokens: 4000, ContextWindow: 128000},
			{ID: "gpt-4.1-nano", DefaultMaxTokens: 2000, ContextWindow: 64000},
		},
	})
	store.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1",
	}
	store.Config().Models[config.SelectedModelTypeSmall] = config.SelectedModel{
		Provider: "openai",
		Model:    "gpt-4.1-mini",
	}
	store.Config().Options.LCM = &config.LCMOptions{
		SummarizerModel: &config.SelectedModel{
			Provider: "openai",
			Model:    "gpt-4.1-nano",
		},
	}

	c := &coordinator{cfg: store}
	modelCfg, _, catwalkModel, err := c.lcmSummarizerSelection()
	require.NoError(t, err)
	require.Equal(t, "gpt-4.1", modelCfg.Model)
	require.Equal(t, "gpt-4.1", catwalkModel.ID)
}
