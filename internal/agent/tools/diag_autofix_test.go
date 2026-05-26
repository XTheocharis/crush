package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutoFixMissingImport(t *testing.T) {
	t.Parallel()

	initialContent := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	content := initialContent

	getDiags := func(_ string) []DiagnosticInfo {
		if strings.Contains(content, "fmt.") && !strings.Contains(content, `"fmt"`) {
			return []DiagnosticInfo{
				{FilePath: "test.go", Line: 3, Severity: SeverityError, Message: "undefined: fmt"},
			}
		}
		return nil
	}

	fixer := NewAutoFixer(
		[]FixStrategy{&MissingImportFixer{}, &UnusedVarFixer{}},
		getDiags,
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalAttempts)
	require.Equal(t, 1, result.FixesApplied)
	require.Empty(t, result.RemainingErrors)
	require.Contains(t, content, `"fmt"`)
}

func TestAutoFixUnusedVar(t *testing.T) {
	t.Parallel()

	initialContent := "package main\n\nfunc main() {\n\tx := 5\n}\n"
	content := initialContent

	getDiags := func(_ string) []DiagnosticInfo {
		var diags []DiagnosticInfo
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, "x := ") {
				diags = append(diags, DiagnosticInfo{
					FilePath: "test.go",
					Line:     uint32(i),
					Severity: SeverityError,
					Message:  "x declared but not used",
				})
			}
		}
		return diags
	}

	fixer := NewAutoFixer(
		[]FixStrategy{&MissingImportFixer{}, &UnusedVarFixer{}},
		getDiags,
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalAttempts)
	require.Equal(t, 1, result.FixesApplied)
	require.Empty(t, result.RemainingErrors)
	require.Contains(t, content, "_ = ")
	require.NotContains(t, content, "x :=")
}

func TestAutoFixMaxAttempts(t *testing.T) {
	t.Parallel()

	content := "package main\n"

	fixer := NewAutoFixer(
		[]FixStrategy{&alwaysChangeFixer{}},
		func(_ string) []DiagnosticInfo {
			return []DiagnosticInfo{
				{FilePath: "test.go", Line: 1, Severity: SeverityError, Message: "stub error"},
			}
		},
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)
	fixer.MaxAttempts = 2

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 2, result.TotalAttempts)
	require.Len(t, result.RemainingErrors, 1)
}

func TestAutoFixNoFixableErrors(t *testing.T) {
	t.Parallel()

	content := "package main\n"

	fixer := NewAutoFixer(
		[]FixStrategy{&MissingImportFixer{}, &UnusedVarFixer{}},
		func(_ string) []DiagnosticInfo {
			return []DiagnosticInfo{
				{FilePath: "test.go", Line: 1, Severity: SeverityError, Message: "syntax error"},
			}
		},
		func(_ string) (string, error) { return content, nil },
		func(_ string, _ string) error { return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 0, result.TotalAttempts)
	require.Len(t, result.RemainingErrors, 1)
}

func TestAutoFixReducesErrors(t *testing.T) {
	t.Parallel()

	initialContent := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n\tx := 5\n}\n"
	content := initialContent

	getDiags := func(_ string) []DiagnosticInfo {
		var diags []DiagnosticInfo
		if strings.Contains(content, "fmt.") && !strings.Contains(content, `"fmt"`) {
			diags = append(diags, DiagnosticInfo{
				FilePath: "test.go", Line: 3, Severity: SeverityError, Message: "undefined: fmt",
			})
		}
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, "x := ") {
				diags = append(diags, DiagnosticInfo{
					FilePath: "test.go",
					Line:     uint32(i),
					Severity: SeverityError,
					Message:  "x declared but not used",
				})
			}
		}
		return diags
	}

	fixer := NewAutoFixer(
		[]FixStrategy{&MissingImportFixer{}, &UnusedVarFixer{}},
		getDiags,
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.TotalAttempts, 1)
	require.GreaterOrEqual(t, result.FixesApplied, 1)
	require.Empty(t, result.RemainingErrors)
}

func TestAutoFixFormattingSkips(t *testing.T) {
	t.Parallel()

	content := "package main\n"

	fixer := NewAutoFixer(
		[]FixStrategy{&FormattingFixer{}},
		func(_ string) []DiagnosticInfo {
			return []DiagnosticInfo{
				{FilePath: "test.go", Line: 1, Severity: SeverityError, Message: "syntax error"},
			}
		},
		func(_ string) (string, error) { return content, nil },
		func(_ string, _ string) error { return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 0, result.TotalAttempts)
}

func TestAutoFixNoErrors(t *testing.T) {
	t.Parallel()

	content := "package main\n"

	fixer := NewAutoFixer(
		[]FixStrategy{&MissingImportFixer{}},
		func(_ string) []DiagnosticInfo { return nil },
		func(_ string) (string, error) { return content, nil },
		func(_ string, _ string) error { return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 0, result.TotalAttempts)
	require.Empty(t, result.RemainingErrors)
}

func TestAutoFixUnusedVarMultiAssign(t *testing.T) {
	t.Parallel()

	content := "package main\n\nfunc main() {\n\tx, err := doThing()\n}\n"

	getDiags := func(_ string) []DiagnosticInfo {
		return []DiagnosticInfo{
			{FilePath: "test.go", Line: 3, Severity: SeverityError, Message: "x declared but not used"},
		}
	}

	fixer := NewAutoFixer(
		[]FixStrategy{&UnusedVarFixer{}},
		getDiags,
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.Equal(t, 1, result.FixesApplied)
	require.Contains(t, content, "_, err := doThing()")
}

func TestAutoFixFormattingApplied(t *testing.T) {
	t.Parallel()

	content := "package main\n\nfunc main(){\n}\n"
	applied := false

	fmtFixer := &FormattingFixer{
		FormatFunc: func(c string, _ DiagnosticInfo) (string, error) {
			applied = true
			return strings.ReplaceAll(c, "main(){", "main() {"), nil
		},
	}

	fixer := NewAutoFixer(
		[]FixStrategy{fmtFixer},
		func(_ string) []DiagnosticInfo {
			if !applied {
				return []DiagnosticInfo{
					{FilePath: "test.go", Line: 2, Severity: SeverityError, Message: "missing space"},
				}
			}
			return nil
		},
		func(_ string) (string, error) { return content, nil },
		func(_ string, c string) error { content = c; return nil },
	)

	result, err := fixer.Run(context.Background(), "test.go")
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, 1, result.TotalAttempts)
	require.Empty(t, result.RemainingErrors)
}

type alwaysChangeFixer struct{}

func (alwaysChangeFixer) Name() string                 { return "always-change" }
func (alwaysChangeFixer) CanFix(_ DiagnosticInfo) bool { return true }
func (alwaysChangeFixer) Apply(content string, _ DiagnosticInfo) (string, error) {
	return content + "// attempt\n", nil
}
