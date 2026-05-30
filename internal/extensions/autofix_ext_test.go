package extensions

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAutoFixConfigWired(t *testing.T) {
	t.Parallel()

	t.Run("default is disabled", func(t *testing.T) {
		t.Parallel()
		e := &AutofixExtension{}
		host := &mockHostContext{cfg: &config.Config{}}
		require.NoError(t, e.Init(context.Background(), host))
		require.False(t, e.loopEnabled)
	})

	t.Run("nil options is disabled", func(t *testing.T) {
		t.Parallel()
		e := &AutofixExtension{}
		host := &mockHostContext{cfg: &config.Config{Options: nil}}
		require.NoError(t, e.Init(context.Background(), host))
		require.False(t, e.loopEnabled)
	})

	t.Run("nil validation is disabled", func(t *testing.T) {
		t.Parallel()
		e := &AutofixExtension{}
		host := &mockHostContext{cfg: &config.Config{Options: &config.Options{}}}
		require.NoError(t, e.Init(context.Background(), host))
		require.False(t, e.loopEnabled)
	})

	t.Run("explicit false is disabled", func(t *testing.T) {
		t.Parallel()
		e := &AutofixExtension{}
		host := &mockHostContext{cfg: &config.Config{
			Options: &config.Options{
				Validation: &config.ValidationOptions{AutoFixLoopEnabled: false},
			},
		}}
		require.NoError(t, e.Init(context.Background(), host))
		require.False(t, e.loopEnabled)
	})

	t.Run("enabled when config is true", func(t *testing.T) {
		t.Parallel()
		e := &AutofixExtension{}
		host := &mockHostContext{cfg: &config.Config{
			Options: &config.Options{
				Validation: &config.ValidationOptions{AutoFixLoopEnabled: true},
			},
		}}
		require.NoError(t, e.Init(context.Background(), host))
		require.True(t, e.loopEnabled)
	})
}

func TestAutofixExtension_Name(t *testing.T) {
	t.Parallel()
	e := &AutofixExtension{}
	require.Equal(t, "autofix", e.Name())
}

func TestAutofixExtension_Shutdown(t *testing.T) {
	t.Parallel()
	e := &AutofixExtension{}
	host := &mockHostContext{cfg: &config.Config{}}
	require.NoError(t, e.Init(context.Background(), host))
	require.True(t, e.active)
	require.NoError(t, e.Shutdown(context.Background()))
	require.False(t, e.active)
}
