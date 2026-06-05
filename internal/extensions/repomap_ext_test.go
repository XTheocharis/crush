package extensions

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepomapExtensionShutdownClosesService(t *testing.T) {
	t.Parallel()

	var closed atomic.Int32
	e := &RepomapExtension{
		active:   true,
		closeSvc: func() { closed.Add(1) },
	}

	require.Equal(t, int32(0), closed.Load(), "closeSvc should not be called yet")

	err := e.Shutdown(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(1), closed.Load(), "closeSvc should be called exactly once")
	require.False(t, e.active)
	require.Nil(t, e.closeSvc, "closeSvc should be cleared after shutdown")
}

func TestRepomapExtensionShutdownNilCloseSvc(t *testing.T) {
	t.Parallel()

	e := &RepomapExtension{active: true}
	err := e.Shutdown(context.Background())
	require.NoError(t, err)
	require.False(t, e.active)
}
