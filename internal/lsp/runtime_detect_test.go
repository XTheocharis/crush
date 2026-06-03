package lsp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectRuntime(t *testing.T) {
	t.Parallel()

	t.Run("empty_dep", func(t *testing.T) {
		t.Parallel()
		path, err := DetectRuntime("")
		require.NoError(t, err)
		require.Empty(t, path)
	})

	t.Run("node", func(t *testing.T) {
		t.Parallel()
		path, err := DetectRuntime("node")
		if err != nil {
			require.Error(t, err)
		} else {
			require.NotEmpty(t, path)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		_, err := DetectRuntime("nonexistent_runtime")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown runtime dependency")
	})
}

func TestDetectPython(t *testing.T) {
	t.Parallel()
	path, err := DetectPython()
	if err != nil {
		require.Error(t, err)
	} else {
		require.NotEmpty(t, path)
	}
}

func TestDetectUvx(t *testing.T) {
	t.Parallel()
	path, err := DetectUvx()
	if err != nil {
		require.Error(t, err)
	} else {
		require.NotEmpty(t, path)
	}
}

func TestIsRuntimeAvailable(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		require.True(t, IsRuntimeAvailable(""))
	})

	t.Run("nonexistent", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsRuntimeAvailable("nonexistent_runtime_xyz"))
	})
}
