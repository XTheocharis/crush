package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

type mockModelBuilder struct {
	buildFn func(ctx context.Context, providerCfg config.ProviderConfig, sel config.SelectedModel, isSubAgent bool) (Model, error)
	calls   atomic.Int32
}

func (m *mockModelBuilder) BuildModel(ctx context.Context, providerCfg config.ProviderConfig, sel config.SelectedModel, isSubAgent bool) (Model, error) {
	m.calls.Add(1)
	if m.buildFn != nil {
		return m.buildFn(ctx, providerCfg, sel, isSubAgent)
	}
	return Model{
		Model:    testutil.NewStubLM(testutil.WithProvider(sel.Provider), testutil.WithModel(sel.Model)),
		ModelCfg: sel,
		CatwalkCfg: catwalk.Model{
			ID:               sel.Model,
			DefaultMaxTokens: 4096,
		},
	}, nil
}

func TestTieredModelProvider_ModelForTier_PreBuiltModels(t *testing.T) {
	t.Parallel()

	smallModel := Model{
		Model:    testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("small-model")),
		ModelCfg: config.SelectedModel{Model: "small-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{
			ID:               "small-model",
			DefaultMaxTokens: 2048,
		},
	}
	largeModel := Model{
		Model:    testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("large-model")),
		ModelCfg: config.SelectedModel{Model: "large-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{
			ID:               "large-model",
			DefaultMaxTokens: 8192,
		},
	}

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: smallModel,
			config.SelectedModelTypeLarge: largeModel,
		},
		NewRateLimitCoordinator(),
	)

	m, err := provider.ModelForTier(context.Background(), config.SelectedModelTypeSmall)
	require.NoError(t, err)
	require.Equal(t, "small-model", m.ModelCfg.Model)
	require.Equal(t, int64(2048), m.CatwalkCfg.DefaultMaxTokens)

	m, err = provider.ModelForTier(context.Background(), config.SelectedModelTypeLarge)
	require.NoError(t, err)
	require.Equal(t, "large-model", m.ModelCfg.Model)
	require.Equal(t, int64(8192), m.CatwalkCfg.DefaultMaxTokens)
}

func TestTieredModelProvider_CachingReturnsSameInstance(t *testing.T) {
	t.Parallel()

	smallModel := Model{
		Model:      testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("small-model")),
		ModelCfg:   config.SelectedModel{Model: "small-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{ID: "small-model"},
	}

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: smallModel,
		},
		NewRateLimitCoordinator(),
	)

	m1, err := provider.ModelForTier(context.Background(), config.SelectedModelTypeSmall)
	require.NoError(t, err)

	m2, err := provider.ModelForTier(context.Background(), config.SelectedModelTypeSmall)
	require.NoError(t, err)

	require.Equal(t, m1.ModelCfg, m2.ModelCfg, "cached model should have same config")
	require.Equal(t, m1.CatwalkCfg.ID, m2.CatwalkCfg.ID, "cached model should have same catwalk ID")
}

func TestTieredModelProvider_CachingBuildsOnceForDynamicModel(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	cfg.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Model:    "dynamic-model",
		Provider: "test",
	}
	cfg.Config().Providers.Set("test", config.ProviderConfig{
		ID:   "test",
		Type: "openai-compat",
		Models: []catwalk.Model{
			{ID: "dynamic-model", DefaultMaxTokens: 4096},
		},
	})

	builder := &mockModelBuilder{}
	provider := NewTieredModelProvider(cfg, builder, nil, NewRateLimitCoordinator())

	_, err := provider.ModelForTier(context.Background(), config.SelectedModelTypeLarge)
	require.NoError(t, err)
	require.Equal(t, int32(1), builder.calls.Load(), "should build once")

	_, err = provider.ModelForTier(context.Background(), config.SelectedModelTypeLarge)
	require.NoError(t, err)
	require.Equal(t, int32(1), builder.calls.Load(), "should not rebuild on second call")
}

func TestTieredModelProvider_UnconfiguredTierReturnsError(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(cfg, &mockModelBuilder{}, nil, NewRateLimitCoordinator())

	_, err := provider.ModelForTier(context.Background(), "nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no model configured for tier")
}

func TestTieredModelProvider_HasTier(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	cfg.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Model: "gpt-4", Provider: "openai",
	}

	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: {},
		},
		NewRateLimitCoordinator(),
	)

	require.True(t, provider.HasTier(config.SelectedModelTypeSmall), "pre-built small tier")
	require.True(t, provider.HasTier(config.SelectedModelTypeLarge), "config-based large tier")
	require.False(t, provider.HasTier("nonexistent"), "missing tier")
}

func TestTieredModelProvider_Tiers(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	cfg.Config().Models[config.SelectedModelTypeLarge] = config.SelectedModel{
		Model: "gpt-4", Provider: "openai",
	}

	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: {},
		},
		NewRateLimitCoordinator(),
	)

	tiers := provider.Tiers()
	require.Len(t, tiers, 2)
	require.Contains(t, tiers, config.SelectedModelTypeSmall)
	require.Contains(t, tiers, config.SelectedModelTypeLarge)
}

func TestTieredModelProvider_ResolveTieredModel_NilProvider(t *testing.T) {
	t.Parallel()

	defaultModel := Model{
		ModelCfg: config.SelectedModel{Model: "default"},
	}

	m, err := ResolveTieredModel(context.Background(), nil, nil, 0, defaultModel)
	require.NoError(t, err)
	require.Equal(t, "default", m.ModelCfg.Model)
}

func TestTieredModelProvider_ResolveTieredModel_NilRouter(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(cfg, &mockModelBuilder{}, nil, NewRateLimitCoordinator())

	defaultModel := Model{
		ModelCfg: config.SelectedModel{Model: "default"},
	}

	m, err := ResolveTieredModel(context.Background(), nil, provider, 100, defaultModel)
	require.NoError(t, err)
	require.Equal(t, "default", m.ModelCfg.Model)
}

func TestTieredModelProvider_ResolveTieredModel_ResolvesCorrectTier(t *testing.T) {
	t.Parallel()

	smallModel := Model{
		Model:      testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("small-model")),
		ModelCfg:   config.SelectedModel{Model: "small-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{ID: "small-model"},
	}
	largeModel := Model{
		Model:      testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("large-model")),
		ModelCfg:   config.SelectedModel{Model: "large-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{ID: "large-model"},
	}

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: smallModel,
			config.SelectedModelTypeLarge: largeModel,
		},
		NewRateLimitCoordinator(),
	)

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 100000, ModelType: config.SelectedModelTypeLarge},
	})

	m, err := ResolveTieredModel(context.Background(), router, provider, 500, largeModel)
	require.NoError(t, err)
	require.Equal(t, "small-model", m.ModelCfg.Model, "low token count should resolve to small")

	m, err = ResolveTieredModel(context.Background(), router, provider, 50000, largeModel)
	require.NoError(t, err)
	require.Equal(t, "large-model", m.ModelCfg.Model, "high token count should resolve to large")
}

func TestTieredModelProvider_ResolveTieredModel_FallbackOnBuildError(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	builder := &mockModelBuilder{
		buildFn: func(_ context.Context, _ config.ProviderConfig, _ config.SelectedModel, _ bool) (Model, error) {
			return Model{}, errModelProviderNotConfigured
		},
	}

	provider := NewTieredModelProvider(cfg, builder, nil, NewRateLimitCoordinator())

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
	})

	defaultModel := Model{
		ModelCfg: config.SelectedModel{Model: "default"},
	}

	m, err := ResolveTieredModel(context.Background(), router, provider, 100, defaultModel)
	require.NoError(t, err)
	require.Equal(t, "default", m.ModelCfg.Model, "should fall back to default on build error")
}

func TestTieredModelProvider_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	smallModel := Model{
		Model:      testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("small-model")),
		ModelCfg:   config.SelectedModel{Model: "small-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{ID: "small-model"},
	}

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: smallModel,
		},
		NewRateLimitCoordinator(),
	)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := provider.ModelForTier(context.Background(), config.SelectedModelTypeSmall)
			require.NoError(t, err)
			require.Equal(t, "small-model", m.ModelCfg.Model)
		}()
	}
	wg.Wait()
}

func TestTieredModelProvider_WithTieredModelProviderOption(t *testing.T) {
	t.Parallel()

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(cfg, &mockModelBuilder{}, nil, NewRateLimitCoordinator())
	opt := WithTieredModelProvider(provider)

	c := &coordinator{}
	opt(c)
	require.NotNil(t, c.tieredProvider)
}

func TestTieredModelProvider_CoordinatorUsesTieredModelInRun(t *testing.T) {
	t.Parallel()

	smallModel := Model{
		Model:    testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("small-model")),
		ModelCfg: config.SelectedModel{Model: "small-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{
			ID:               "small-model",
			DefaultMaxTokens: 2048,
		},
	}
	largeModel := Model{
		Model:    testutil.NewStubLM(testutil.WithProvider("test"), testutil.WithModel("large-model")),
		ModelCfg: config.SelectedModel{Model: "large-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{
			ID:               "large-model",
			DefaultMaxTokens: 8192,
		},
	}

	cfg, _ := config.Init(t.TempDir(), "", false)
	provider := NewTieredModelProvider(
		cfg,
		&mockModelBuilder{},
		map[config.SelectedModelType]Model{
			config.SelectedModelTypeSmall: smallModel,
			config.SelectedModelTypeLarge: largeModel,
		},
		NewRateLimitCoordinator(),
	)

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
		{UpToTokens: 100000, ModelType: config.SelectedModelTypeLarge},
	})

	resolvedSmall, err := ResolveTieredModel(context.Background(), router, provider, 500, largeModel)
	require.NoError(t, err)
	require.Equal(t, "small-model", resolvedSmall.ModelCfg.Model)

	resolvedLarge, err := ResolveTieredModel(context.Background(), router, provider, 50000, largeModel)
	require.NoError(t, err)
	require.Equal(t, "large-model", resolvedLarge.ModelCfg.Model)
}

func TestTieredModelProvider_NilTieredProvider_BackwardCompat(t *testing.T) {
	t.Parallel()

	defaultModel := Model{
		ModelCfg: config.SelectedModel{Model: "default-model", Provider: "test"},
		CatwalkCfg: catwalk.Model{
			ID:               "default-model",
			DefaultMaxTokens: 4096,
		},
	}

	router := NewTierRouter([]config.RoutingTier{
		{UpToTokens: 1000, ModelType: config.SelectedModelTypeSmall},
	})

	m, err := ResolveTieredModel(context.Background(), router, nil, 500, defaultModel)
	require.NoError(t, err)
	require.Equal(t, "default-model", m.ModelCfg.Model)
}
