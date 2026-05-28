package extensions

import (
	"context"
	"testing"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestModelRouterExtension_Name(t *testing.T) {
	e := &ModelRouterExtension{}
	require.Equal(t, "model_router", e.Name())
}

func TestModelRouterExtension_LastRoutedModel_DefaultZero(t *testing.T) {
	e := &ModelRouterExtension{}
	require.Equal(t, config.SelectedModelType(""), e.LastRoutedModel())
}

func TestModelRouterExtension_BinaryRouterStoresSmall(t *testing.T) {
	e := &ModelRouterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	_, err = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hi"),
	})
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeSmall, e.LastRoutedModel())
}

func TestModelRouterExtension_BinaryRouterStoresLarge(t *testing.T) {
	e := &ModelRouterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	var longText string
	for i := 0; i < 20000; i++ {
		longText += "x"
	}
	_, err = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(longText),
	})
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeLarge, e.LastRoutedModel())
}

func TestModelRouterExtension_TierRouterStoresResult(t *testing.T) {
	e := &ModelRouterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			RouterTiers: []config.RoutingTier{
				{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
				{UpToTokens: 10000, ModelType: config.SelectedModelTypeLarge},
			},
		},
	}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	_, err = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("hello"),
	})
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeSmall, e.LastRoutedModel())

	var bigText string
	for i := 0; i < 5000; i++ {
		bigText += "y"
	}
	_, err = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(bigText),
	})
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelTypeLarge, e.LastRoutedModel())
}

func TestModelRouterExtension_UpdatesPerCall(t *testing.T) {
	e := &ModelRouterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	_, _ = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("short"),
	})
	require.Equal(t, config.SelectedModelTypeSmall, e.LastRoutedModel())

	var bigText string
	for i := 0; i < 20000; i++ {
		bigText += "z"
	}
	_, _ = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage(bigText),
	})
	require.Equal(t, config.SelectedModelTypeLarge, e.LastRoutedModel())
}

func TestModelRouterExtension_ShutdownClearsState(t *testing.T) {
	e := &ModelRouterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	_, _ = e.selectModel(context.Background(), "s1", []fantasy.Message{
		fantasy.NewUserMessage("short"),
	})
	require.NotEqual(t, config.SelectedModelType(""), e.LastRoutedModel())

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.Equal(t, config.SelectedModelType(""), e.LastRoutedModel())
	require.Nil(t, e.StepHooks())
}

func TestModelRouterExtension_InactiveReturnsNil(t *testing.T) {
	e := &ModelRouterExtension{}
	require.Nil(t, e.StepHooks())
	require.Equal(t, config.SelectedModelType(""), e.LastRoutedModel())
}

func TestEstimateCharCount(t *testing.T) {
	msgs := []fantasy.Message{
		fantasy.NewUserMessage("hello"),
		fantasy.NewUserMessage("world"),
	}
	require.Equal(t, 10, estimateCharCount(msgs))
}
