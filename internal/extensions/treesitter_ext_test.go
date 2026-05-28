//go:build treesitter

package extensions

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/stretchr/testify/require"
)

func TestTreesitterExtension_ValidationHandlerWired(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{
				Enabled: true,
			},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	handler := e.Handler()
	require.NotNil(t, handler, "Handler() must return non-nil ValidationHandler when validation is enabled")
	require.True(t, handler.Enabled())
}

func TestTreesitterExtension_ValidationHandlerInactiveWithoutConfig(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.False(t, e.active)
	require.Nil(t, e.Handler())
}

func TestTreesitterExtension_ValidationHandlerDisabledConfig(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{
				Enabled: false,
			},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.True(t, e.active)

	handler := e.Handler()
	require.NotNil(t, handler, "Handler() should be non-nil even when validation disabled (inert handler)")
	require.False(t, handler.Enabled())
}

func TestTreesitterExtension_ShutdownClearsHandler(t *testing.T) {
	e := &TreesitterExtension{}
	host := &mockHostContext{cfg: &config.Config{
		Options: &config.Options{
			Validation: &config.ValidationOptions{Enabled: true},
		},
	}}

	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	require.NotNil(t, e.Handler())

	err = e.Shutdown(context.Background())
	require.NoError(t, err)
	require.Nil(t, e.Handler())
	require.False(t, e.active)
}

func TestTreesitterExtension_ToolProviderRemoved(t *testing.T) {
	_ = ext.Extension(&TreesitterExtension{})
}

func TestTreesitterExtension_Name(t *testing.T) {
	e := &TreesitterExtension{}
	require.Equal(t, "treesitter-validation", e.Name())
}
