package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/stretchr/testify/require"
)

type mockLinter struct {
	results [][]string
	calls   int
	err     error
}

func (m *mockLinter) RunLint(_ context.Context, _ []string) ([]string, error) {
	idx := m.calls
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return nil, nil
}

type mockTester struct {
	results [][]string
	calls   int
	err     error
}

func (m *mockTester) RunTests(_ context.Context) ([]string, error) {
	idx := m.calls
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return nil, nil
}

func TestAutoFixLoop_NoLintErrorsPassesImmediately(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{results: [][]string{nil}}
	tester := &mockTester{results: [][]string{nil}}
	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	loop.MaxRetries = 3

	result, err := loop.Run(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Attempts, 1)
	require.Empty(t, result.FinalLintErrors)
	require.Empty(t, result.FinalTestErrors)
}

func TestAutoFixLoop_FixesOnFirstAttempt(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{
		results: [][]string{
			{"file.go:10: undefined: foo"},
			nil,
		},
	}
	tester := &mockTester{results: [][]string{nil}}

	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "file.go")
	require.NoError(t, os.WriteFile(fp, []byte("package p\n"), 0o644))

	fixer := tools.NewAutoFixer(
		nil,
		func(_ string) []tools.DiagnosticInfo { return nil },
		func(p string) (string, error) { b, err := os.ReadFile(p); return string(b), err },
		func(p, c string) error { return os.WriteFile(p, []byte(c), 0o644) },
	)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	result, err := loop.Run(context.Background(), []string{fp})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Attempts, 1)
}

func TestAutoFixLoop_ExhaustedRetriesRollback(t *testing.T) {
	t.Parallel()

	errMsg := "file.go:1: persistent error"
	linter := &mockLinter{
		results: [][]string{
			{errMsg},
			{errMsg},
			{errMsg},
			{errMsg},
			{errMsg},
			{errMsg},
			{errMsg},
		},
	}
	tester := &mockTester{
		results: [][]string{
			nil,
			nil,
			nil,
			nil,
		},
	}

	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "file.go")
	originalContent := "package original\n"
	require.NoError(t, os.WriteFile(fp, []byte(originalContent), 0o644))

	fixer := tools.NewAutoFixer(
		nil,
		func(_ string) []tools.DiagnosticInfo { return nil },
		func(p string) (string, error) { b, err := os.ReadFile(p); return string(b), err },
		func(p, c string) error { return os.WriteFile(p, []byte(c), 0o644) },
	)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	loop.MaxRetries = 3

	result, err := loop.Run(context.Background(), []string{fp})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Len(t, result.Attempts, 3)
	require.True(t, result.Attempts[2].RolledBack)
	require.Equal(t, []string{errMsg}, result.FinalLintErrors)

	content, readErr := os.ReadFile(fp)
	require.NoError(t, readErr)
	require.Equal(t, originalContent, string(content))
}

func TestAutoFixLoop_ContextCancellation(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{results: [][]string{{"error"}}}
	tester := &mockTester{results: [][]string{nil}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "file.go")
	require.NoError(t, os.WriteFile(fp, []byte("package p\n"), 0o644))

	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	_, err := loop.Run(ctx, []string{fp})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestAutoFixLoop_LinterError(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{err: errors.New("linter crashed")}
	tester := &mockTester{}

	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	_, err := loop.Run(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "linter crashed")
}

func TestAutoFixLoop_TesterError(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{results: [][]string{nil}}
	tester := &mockTester{err: errors.New("test runner crashed")}

	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	_, err := loop.Run(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "test runner crashed")
}

func TestAutoFixLoop_TestFailuresWithCleanLint(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{
		results: [][]string{
			nil,
			nil,
			nil,
		},
	}
	tester := &mockTester{
		results: [][]string{
			{"file_test.go:10: FAIL: TestSomething"},
			nil,
		},
	}

	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	loop.MaxRetries = 3

	result, err := loop.Run(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Attempts, 2)
	require.Equal(t, 1, result.Attempts[0].AttemptNum)
	require.Equal(t, 2, result.Attempts[1].AttemptNum)
}

func TestReflectOnErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []string
		expect []string
	}{
		{
			name:   "empty input",
			input:  []string{},
			expect: nil,
		},
		{
			name:   "lint format with column",
			input:  []string{"main.go:42:10: unused variable"},
			expect: []string{"main.go:42:10: unused variable"},
		},
		{
			name:   "lint format without column",
			input:  []string{"main.go:42: syntax error"},
			expect: []string{"main.go:42: syntax error"},
		},
		{
			name:   "deduplication",
			input:  []string{"a.go:1: err", "a.go:1: err"},
			expect: []string{"a.go:1: err"},
		},
		{
			name:   "unknown format passes through",
			input:  []string{"some random text"},
			expect: []string{"some random text"},
		},
		{
			name:   "whitespace trimmed",
			input:  []string{"  a.go:1: err  "},
			expect: []string{"a.go:1: err"},
		},
		{
			name:   "blank lines skipped",
			input:  []string{"", "a.go:1: err", ""},
			expect: []string{"a.go:1: err"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reflectOnErrors(tc.input)
			require.Equal(t, tc.expect, got)
		})
	}
}

func TestNewAutoFixLoop_Defaults(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{}
	tester := &mockTester{}
	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	require.Equal(t, 3, loop.MaxRetries)
	require.True(t, loop.Enabled)
	require.Same(t, linter, loop.Linter)
	require.Same(t, tester, loop.Tester)
	require.Same(t, fixer, loop.Fixer)
	require.Same(t, rollback, loop.Rollback)
}

func TestAutoFixLoop_Disabled(t *testing.T) {
	t.Parallel()

	linter := &mockLinter{results: [][]string{{"error"}}}
	tester := &mockTester{results: [][]string{nil}}
	fixer := tools.NewAutoFixer(nil, nil, nil, nil)
	rollback := tools.NewRollbackManager()

	loop := NewAutoFixLoop(linter, tester, fixer, rollback)
	loop.Enabled = false

	result, err := loop.Run(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Empty(t, result.Attempts)
	require.Zero(t, linter.calls, "disabled loop should not call linter")
}

func TestAutoCommit_CreatesCommit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, exec.CommandContext(context.Background(), "git", "init", tmpDir).Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.name", "Test").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("hello"), 0o644))
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "add", "-A").Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "commit", "-m", "initial").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("package p\n"), 0o644))

	hash, err := AutoCommit(context.Background(), tmpDir, "add file.go", DefaultAutoCommitConfig())
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	out, err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "log", "-1", "--format=%s").Output()
	require.NoError(t, err)
	require.Equal(t, "auto: add file.go\n", string(out))
}

func TestAutoCommit_NotGitRepo_NoError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	hash, err := AutoCommit(context.Background(), tmpDir, "should not fail", DefaultAutoCommitConfig())
	require.NoError(t, err)
	require.Empty(t, hash)
}

func TestAutoCommit_Disabled_NoCommit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, exec.CommandContext(context.Background(), "git", "init", tmpDir).Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.email", "test@test.com").Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "config", "user.name", "Test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("package p\n"), 0o644))
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "add", "-A").Run())
	require.NoError(t, exec.CommandContext(context.Background(), "git", "-C", tmpDir, "commit", "-m", "initial").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.go"), []byte("package q\n"), 0o644))

	cfg := AutoCommitConfig{Enabled: false}
	hash, err := AutoCommit(context.Background(), tmpDir, "should not commit", cfg)
	require.NoError(t, err)
	require.Empty(t, hash)

	out, err := exec.CommandContext(context.Background(), "git", "-C", tmpDir, "log", "-1", "--format=%s").Output()
	require.NoError(t, err)
	require.Equal(t, "initial\n", string(out))
}

func TestParseGoTestOutput_SingleFailure(t *testing.T) {
	t.Parallel()

	output := `=== RUN   TestSomething
--- FAIL: TestSomething (0.00s)
    file_test.go:10: expected true, got false
FAIL`

	failures := ParseGoTestOutput(output)
	require.Len(t, failures, 1)
	require.Equal(t, "TestSomething", failures[0].TestName)
	require.Empty(t, failures[0].File)
	require.Equal(t, 0, failures[0].Line)
	require.Equal(t, "file_test.go:10: expected true, got false", failures[0].Message)
}

func TestParseGoTestOutput_MultipleFailures(t *testing.T) {
	t.Parallel()

	output := `=== RUN   TestA
--- FAIL: TestA (a_test.go:5)
    a_test.go:5: error A
=== RUN   TestB
--- FAIL: TestB (b_test.go:10)
    b_test.go:10: error B
FAIL`

	failures := ParseGoTestOutput(output)
	require.Len(t, failures, 2)

	require.Equal(t, "TestA", failures[0].TestName)
	require.Equal(t, "a_test.go", failures[0].File)
	require.Equal(t, 5, failures[0].Line)
	require.Equal(t, "a_test.go:5: error A", failures[0].Message)

	require.Equal(t, "TestB", failures[1].TestName)
	require.Equal(t, "b_test.go", failures[1].File)
	require.Equal(t, 10, failures[1].Line)
	require.Equal(t, "b_test.go:10: error B", failures[1].Message)
}

func TestParseGoTestOutput_NoFailures(t *testing.T) {
	t.Parallel()

	output := `=== RUN   TestPass
--- PASS: TestPass (0.00s)
ok  github.com/example/pkg  0.123s`

	failures := ParseGoTestOutput(output)
	require.Empty(t, failures)
}

func TestReflectionStrategy_GeneratesPrompt(t *testing.T) {
	t.Parallel()

	failure := TestFailure{
		TestName: "TestCompute",
		File:     "compute.go",
		Line:     42,
		Message:  "expected 42, got 0",
	}

	prompt := ReflectionStrategy(failure, "func Compute() int { return 0 }")

	require.Contains(t, prompt, "TestCompute")
	require.Contains(t, prompt, "compute.go:42")
	require.Contains(t, prompt, "expected 42, got 0")
	require.Contains(t, prompt, "func Compute() int { return 0 }")
	require.Contains(t, prompt, "Suggest a fix")
}

func TestParseGoTestJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []GoTestJSONResult
		wantLen int
		check   func(t *testing.T, results []GoTestJSONResult)
	}{
		{
			name: "single passing test",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestPass"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"output","Package":"pkg/example","Test":"TestPass","Output":"=== RUN   TestPass\n"}
{"Time":"2024-01-01T00:00:00.002Z","Action":"pass","Package":"pkg/example","Test":"TestPass","Elapsed":0.001}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 1)
				require.Equal(t, "pkg/example", results[0].Package)
				require.Equal(t, "TestPass", results[0].Test)
				require.Equal(t, "pass", results[0].Action)
				require.Equal(t, "=== RUN   TestPass", results[0].Output)
				require.InDelta(t, 0.001, results[0].Elapsed, 0.0001)
			},
		},
		{
			name: "single failing test",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestFail"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"output","Package":"pkg/example","Test":"TestFail","Output":"    file_test.go:10: expected true\n"}
{"Time":"2024-01-01T00:00:00.002Z","Action":"fail","Package":"pkg/example","Test":"TestFail","Elapsed":0.002}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 1)
				require.Equal(t, "fail", results[0].Action)
				require.Contains(t, results[0].Output, "file_test.go:10: expected true")
			},
		},
		{
			name: "skipped test",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestSkip"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"output","Package":"pkg/example","Test":"TestSkip","Output":"    skipping\n"}
{"Time":"2024-01-01T00:00:00.002Z","Action":"skip","Package":"pkg/example","Test":"TestSkip","Elapsed":0.0}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 1)
				require.Equal(t, "skip", results[0].Action)
			},
		},
		{
			name: "multiple tests mixed results",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestPass"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"pass","Package":"pkg/example","Test":"TestPass","Elapsed":0.001}
{"Time":"2024-01-01T00:00:00.002Z","Action":"run","Package":"pkg/example","Test":"TestFail"}
{"Time":"2024-01-01T00:00:00.003Z","Action":"output","Package":"pkg/example","Test":"TestFail","Output":"error msg\n"}
{"Time":"2024-01-01T00:00:00.004Z","Action":"fail","Package":"pkg/example","Test":"TestFail","Elapsed":0.002}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 2)
				require.Equal(t, "TestPass", results[0].Test)
				require.Equal(t, "pass", results[0].Action)
				require.Equal(t, "TestFail", results[1].Test)
				require.Equal(t, "fail", results[1].Action)
			},
		},
		{
			name: "output aggregation across multiple events",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestMulti"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"output","Package":"pkg/example","Test":"TestMulti","Output":"line1\n"}
{"Time":"2024-01-01T00:00:00.002Z","Action":"output","Package":"pkg/example","Test":"TestMulti","Output":"line2\n"}
{"Time":"2024-01-01T00:00:00.003Z","Action":"output","Package":"pkg/example","Test":"TestMulti","Output":"line3\n"}
{"Time":"2024-01-01T00:00:00.004Z","Action":"fail","Package":"pkg/example","Test":"TestMulti","Elapsed":0.004}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 1)
				require.Equal(t, "line1\nline2\nline3", results[0].Output)
			},
		},
		{
			name:    "empty input returns empty",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "malformed lines skipped",
			input:   "not json at all\n{\"Action\":\"pass\",\"Package\":\"p\",\"Test\":\"T\",\"Elapsed\":0.1}\nalso not json",
			wantLen: 1,
		},
		{
			name:  "package level events ignored",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"pkg/example","Elapsed":0.5}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Empty(t, results)
			},
		},
		{
			name: "subtests with slash in name",
			input: `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"pkg/example","Test":"TestGroup/subtest_a"}
{"Time":"2024-01-01T00:00:00.001Z","Action":"output","Package":"pkg/example","Test":"TestGroup/subtest_a","Output":"hello\n"}
{"Time":"2024-01-01T00:00:00.002Z","Action":"fail","Package":"pkg/example","Test":"TestGroup/subtest_a","Elapsed":0.001}`,
			check: func(t *testing.T, results []GoTestJSONResult) {
				require.Len(t, results, 1)
				require.Equal(t, "TestGroup/subtest_a", results[0].Test)
				require.Equal(t, "fail", results[0].Action)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			results := ParseGoTestJSON(tc.input)
			if tc.check != nil {
				tc.check(t, results)
			} else {
				require.Len(t, results, tc.wantLen)
			}
		})
	}
}
