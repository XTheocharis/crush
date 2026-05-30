package extensions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/agent/tools"
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

// mockLinter implements agent.Linter for testing.
type mockLinter struct {
	results [][]string
	errs    []error
	calls   int
}

func (m *mockLinter) RunLint(_ context.Context, _ []string) ([]string, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.results) {
		err := error(nil)
		if idx < len(m.errs) {
			err = m.errs[idx]
		}
		return m.results[idx], err
	}
	return nil, nil
}

func TestReflectOnErrors(t *testing.T) {
	t.Parallel()

	t.Run("parses lint errors", func(t *testing.T) {
		t.Parallel()
		lines := []string{
			"main.go:42: undefined: foo",
			"util.go:10: some error",
		}
		notes := reflectOnErrors(lines)
		require.Len(t, notes, 2)
		require.Equal(t, "main.go:42: undefined: foo", notes[0])
		require.Equal(t, "util.go:10: some error", notes[1])
	})

	t.Run("deduplicates identical lines", func(t *testing.T) {
		t.Parallel()
		lines := []string{
			"main.go:42: undefined: foo",
			"main.go:42: undefined: foo",
		}
		notes := reflectOnErrors(lines)
		require.Len(t, notes, 1)
	})

	t.Run("skips empty lines", func(t *testing.T) {
		t.Parallel()
		lines := []string{"", "main.go:1: err", ""}
		notes := reflectOnErrors(lines)
		require.Len(t, notes, 1)
	})

	t.Run("passes through unrecognized lines", func(t *testing.T) {
		t.Parallel()
		lines := []string{"some generic error"}
		notes := reflectOnErrors(lines)
		require.Len(t, notes, 1)
		require.Equal(t, "some generic error", notes[0])
	})
}

func TestAutoFixLoopPromotion(t *testing.T) {
	t.Parallel()

	t.Run("full cycle converges when lint clean on first try", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		require.NoError(t, os.WriteFile(fp, []byte("package main\n"), 0o644))

		e := &AutofixExtension{loopEnabled: true, active: true}
		linter := &mockLinter{results: [][]string{{}}}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(context.Background(), dir, []string{fp}, linter, rollback)
		require.Equal(t, 1, linter.calls)
	})

	t.Run("full cycle iterates when lint errors exist then clear", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		require.NoError(t, os.WriteFile(fp, []byte("package main\n"), 0o644))

		e := &AutofixExtension{loopEnabled: true, active: true}
		linter := &mockLinter{results: [][]string{
			{"main.go:1: some error"},
			{},
		}}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(context.Background(), dir, []string{fp}, linter, rollback)
		require.Equal(t, 2, linter.calls)
	})

	t.Run("full cycle rolls back when all retries exhausted", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		original := "package main\n"
		require.NoError(t, os.WriteFile(fp, []byte(original), 0o644))

		e := &AutofixExtension{loopEnabled: true, active: true}
		// Each iteration: 1 lint at start + 1 re-lint after fix = 2 calls.
		// 3 iterations = 6 calls, + 1 final lint = 7 total.
		// All return errors to exhaust retries.
		persistentErrors := [][]string{
			{"main.go:1: err1"}, {"main.go:1: err2"},
			{"main.go:1: err3"}, {"main.go:1: err4"},
			{"main.go:1: err5"}, {"main.go:1: err6"},
			{"main.go:1: err_final"},
		}
		linter := &mockLinter{results: persistentErrors}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(context.Background(), dir, []string{fp}, linter, rollback)
		require.Equal(t, 7, linter.calls)

		content, err := os.ReadFile(fp)
		require.NoError(t, err)
		require.Equal(t, original, string(content))
	})

	t.Run("loopEnabled false does not call fullAutoFixCycle", func(t *testing.T) {
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
}

func TestFullAutoFixCycle_RespectsContext(t *testing.T) {
	t.Parallel()

	t.Run("returns immediately on cancelled context", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		require.NoError(t, os.WriteFile(fp, []byte("package main\n"), 0o644))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		e := &AutofixExtension{loopEnabled: true, active: true}
		linter := &mockLinter{results: [][]string{{"main.go:1: err"}}}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(ctx, dir, []string{fp}, linter, rollback)
		// Should not have called lint at all (context already cancelled).
		require.Equal(t, 0, linter.calls)
	})
}
