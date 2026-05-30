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
			{"main.go:1: err1"},
			{"main.go:1: err2"},
			{"main.go:1: err3"},
			{"main.go:1: err4"},
			{"main.go:1: err5"},
			{"main.go:1: err6"},
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

func TestConvergenceDetection(t *testing.T) {
	t.Parallel()

	t.Run("stops early when same errors repeat", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		original := "package main\n"
		require.NoError(t, os.WriteFile(fp, []byte(original), 0o644))

		e := &AutofixExtension{loopEnabled: true, active: true}
		// Every lint call returns the exact same error set.
		// Iteration 1: lint→err, re-lint→err → fingerprint match → count=1.
		// Iteration 2: lint→err, re-lint→err → fingerprint match → count=2 → break.
		// + 1 final lint = 5 total calls.
		// Without convergence, 3 iterations × 2 + 1 final = 7 calls.
		sameErrors := [][]string{
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
			{"main.go:1: persistent error"},
		}
		linter := &mockLinter{results: sameErrors}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(context.Background(), dir, []string{fp}, linter, rollback)
		// Should stop at iteration 2 due to convergence (5 lint calls),
		// not exhaust all 3 iterations (which would be 7 calls).
		require.Equal(t, 5, linter.calls)

		// File should be rolled back.
		content, err := os.ReadFile(fp)
		require.NoError(t, err)
		require.Equal(t, original, string(content))
	})

	t.Run("does not stop when errors change between iterations", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fp := filepath.Join(dir, "main.go")
		original := "package main\n"
		require.NoError(t, os.WriteFile(fp, []byte(original), 0o644))

		e := &AutofixExtension{loopEnabled: true, active: true}
		// Errors are different each call → fingerprints never match → full 3 iterations.
		changingErrors := [][]string{
			{"main.go:1: err_a"},
			{"main.go:1: err_b"},
			{"main.go:1: err_c"},
			{"main.go:1: err_d"},
			{"main.go:1: err_e"},
			{"main.go:1: err_f"},
			{"main.go:1: err_final"},
		}
		linter := &mockLinter{results: changingErrors}
		rollback := tools.NewRollbackManager()

		e.fullAutoFixCycle(context.Background(), dir, []string{fp}, linter, rollback)
		// All 3 iterations exhausted: 6 + 1 final = 7 calls.
		require.Equal(t, 7, linter.calls)
	})
}

func TestFingerprintErrors(t *testing.T) {
	t.Parallel()

	t.Run("returns empty for empty input", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", fingerprintErrors(nil))
		require.Equal(t, "", fingerprintErrors([]string{}))
	})

	t.Run("same errors produce same fingerprint regardless of order", func(t *testing.T) {
		t.Parallel()
		fp1 := fingerprintErrors([]string{"a.go:1: err", "b.go:2: err"})
		fp2 := fingerprintErrors([]string{"b.go:2: err", "a.go:1: err"})
		require.Equal(t, fp1, fp2)
	})

	t.Run("different errors produce different fingerprints", func(t *testing.T) {
		t.Parallel()
		fp1 := fingerprintErrors([]string{"a.go:1: err"})
		fp2 := fingerprintErrors([]string{"a.go:1: other"})
		require.NotEqual(t, fp1, fp2)
	})

	t.Run("deduplicates identical errors", func(t *testing.T) {
		t.Parallel()
		fp1 := fingerprintErrors([]string{"a.go:1: err", "a.go:1: err"})
		fp2 := fingerprintErrors([]string{"a.go:1: err"})
		require.Equal(t, fp1, fp2)
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
