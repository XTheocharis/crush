package explorer

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithTempFile(t *testing.T) { //nolint:tparallel // "write failure" subtest uses t.Setenv, incompatible with t.Parallel on parent
	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		var captured string
		err := withTempFile("crush-test-*.dat", []byte("hello world"), func(path string) error {
			captured = path
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "hello world", string(data))
			return nil
		})
		require.NoError(t, err)
		require.NotEmpty(t, captured)

		// File should be cleaned up after withTempFile returns.
		_, err = os.Stat(captured)
		require.True(t, os.IsNotExist(err), "temp file should be removed after return")
	})

	t.Run("write failure read-only dir", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod not effective on Windows")
		}

		// Override TMPDIR to a read-only directory so os.CreateTemp fails.
		roDir := filepath.Join(t.TempDir(), "readonly")
		require.NoError(t, os.Mkdir(roDir, 0o755))
		require.NoError(t, os.Chmod(roDir, 0o444))
		t.Cleanup(func() {
			os.Chmod(roDir, 0o755)
		})

		t.Setenv("TMPDIR", roDir)

		called := false
		err := withTempFile("crush-test-*.dat", []byte("data"), func(_ string) error {
			called = true
			return nil
		})
		require.Error(t, err, "should fail when temp dir is read-only")
		require.False(t, called, "fn must not be called when temp file creation fails")
	})

	t.Run("fn error propagation", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("callback failed")
		err := withTempFile("crush-test-*.dat", []byte("data"), func(_ string) error {
			return sentinel
		})
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("cleanup on fn error", func(t *testing.T) {
		t.Parallel()

		var captured string
		_ = withTempFile("crush-test-*.dat", []byte("data"), func(path string) error {
			captured = path
			// Verify file exists while fn is running.
			_, err := os.Stat(path)
			require.NoError(t, err, "temp file should exist during fn")
			return errors.New("boom")
		})
		require.NotEmpty(t, captured)

		// File should still be removed even though fn returned an error.
		_, err := os.Stat(captured)
		require.True(t, os.IsNotExist(err), "temp file should be removed after fn error")
	})
}
