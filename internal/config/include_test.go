package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncludeDirectives(t *testing.T) {
	t.Parallel()

	t.Run("SimpleInclude", TestSimpleInclude)
	t.Run("RecursiveInclude", TestRecursiveInclude)
	t.Run("CycleDetection", TestCycleDetection)
	t.Run("MaxDepthExceeded", TestMaxDepthExceeded)
	t.Run("Truncation", TestTruncation)
	// ConditionalEnv uses t.Setenv, incompatible with t.Parallel — runs as standalone test instead.
	t.Run("ConditionalLanguage", TestConditionalLanguage)
	t.Run("ConditionalFile", TestConditionalFile)
	t.Run("NestedConditionalsError", TestNestedConditionalsError)
	t.Run("UnmatchedEndifError", TestUnmatchedEndifError)
	t.Run("UnclosedConditionalError", TestUnclosedConditionalError)
	t.Run("AbsolutePathError", TestAbsolutePathError)
	t.Run("PathEscapeError", TestPathEscapeError)
	t.Run("MissingFileError", TestMissingFileError)
	t.Run("NoIncludesPassthrough", TestNoIncludesPassthrough)
	t.Run("IncludeWithSurroundingContent", TestIncludeWithSurroundingContent)
	t.Run("SharedSeenAcrossRecursion", TestSharedSeenAcrossRecursion)
	t.Run("TruncateFunction", TestTruncateFunction)
	t.Run("IsSubPath", TestIsSubPath)
	// FileAwareEvaluator uses t.Setenv, incompatible with t.Parallel — runs as standalone test instead.
}

func TestSimpleInclude(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	included := filepath.Join(tmp, "snippet.md")
	require.NoError(t, os.WriteFile(included, []byte("included content"), 0o644))

	content := "before\n@include snippet.md\nafter"
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "before\nincluded content\nafter", result)
}

func TestRecursiveInclude(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "sub"), 0o755))

	inner := filepath.Join(tmp, "sub", "inner.md")
	require.NoError(t, os.WriteFile(inner, []byte("inner content"), 0o644))

	middle := filepath.Join(tmp, "sub", "middle.md")
	require.NoError(t, os.WriteFile(middle, []byte("@include inner.md"), 0o644))

	content := "@include sub/middle.md"
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "inner content", result)
}

func TestCycleDetection(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")

	require.NoError(t, os.WriteFile(a, []byte("@include b.md"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("@include a.md"), 0o644))

	_, err := ProcessIncludes("@include a.md", tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cycle detected")
}

func TestMaxDepthExceeded(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	const n = 8
	for i := range n {
		name := filepath.Join(tmp, fmt.Sprintf("level%d.md", i))
		next := ""
		if i < n-1 {
			next = fmt.Sprintf("@include level%d.md", i+1)
		}
		require.NoError(t, os.WriteFile(name, []byte(next), 0o644))
	}

	_, err := ProcessIncludes("@include level0.md", tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeded maximum depth")
}

func TestTruncation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	big := strings.Repeat("x", maxContentChars+1000)
	included := filepath.Join(tmp, "big.md")
	require.NoError(t, os.WriteFile(included, []byte(big), 0o644))

	content := "@include big.md"
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(strings.TrimSpace(result), "<!-- truncated at 40K -->"))
	require.LessOrEqual(t, len(result), maxContentChars+len(truncationMarker)+10)
}

func TestConditionalEnv(t *testing.T) {
	tmp := t.TempDir()

	t.Setenv("MY_TEST_VAR", "1")
	content := `before
<!-- if: env:MY_TEST_VAR -->
visible
<!-- endif -->
after`
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.Contains(t, result, "visible")
	require.Contains(t, result, "before")
	require.Contains(t, result, "after")

	content = `before
<!-- if: env:NONEXISTENT_VAR_XYZ -->
hidden
<!-- endif -->
after`
	result, err = ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.NotContains(t, result, "hidden")
	require.Contains(t, result, "before")
	require.Contains(t, result, "after")
}

func TestConditionalLanguage(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	eval := FileAwareEvaluator(filepath.Join(tmp, "main.go"))
	content := `<!-- if: language:go -->
go code
<!-- endif -->
<!-- if: language:py -->
py code
<!-- endif -->`
	result, err := ProcessIncludes(content, tmp, 0, nil, eval)
	require.NoError(t, err)
	require.Contains(t, result, "go code")
	require.NotContains(t, result, "py code")
}

func TestConditionalFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	eval := FileAwareEvaluator(filepath.Join(tmp, "main.go"))
	content := `<!-- if: file:*.go -->
go file content
<!-- endif -->
<!-- if: file:*.py -->
py file content
<!-- endif -->`
	result, err := ProcessIncludes(content, tmp, 0, nil, eval)
	require.NoError(t, err)
	require.Contains(t, result, "go file content")
	require.NotContains(t, result, "py file content")
}

func TestNestedConditionalsError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `<!-- if: env:FOO -->
<!-- if: env:BAR -->
nested
<!-- endif -->
<!-- endif -->`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nested conditional")
}

func TestUnmatchedEndifError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `<!-- endif -->`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected <!-- endif -->")
}

func TestUnclosedConditionalError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `<!-- if: env:FOO -->
no end`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unclosed conditional")
}

func TestAbsolutePathError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `@include /etc/passwd`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "absolute path")
}

func TestPathEscapeError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `@include ../../../etc/passwd`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes project directory")
}

func TestMissingFileError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := `@include nonexistent.md`
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read")
}

func TestNoIncludesPassthrough(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	content := "just some text\nno includes here"
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.Equal(t, content, result)
}

func TestIncludeWithSurroundingContent(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	snippet := filepath.Join(tmp, "snippet.md")
	require.NoError(t, os.WriteFile(snippet, []byte("line1\nline2"), 0o644))

	content := "header\n@include snippet.md\nfooter"
	result, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "header\nline1\nline2\nfooter", result)
}

func TestSharedSeenAcrossRecursion(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	shared := filepath.Join(tmp, "shared.md")
	require.NoError(t, os.WriteFile(shared, []byte("shared content"), 0o644))

	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	require.NoError(t, os.WriteFile(a, []byte("@include shared.md"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("@include shared.md"), 0o644))

	content := "@include a.md\n@include b.md"
	_, err := ProcessIncludes(content, tmp, 0, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cycle detected")
}

func TestTruncateFunction(t *testing.T) {
	t.Parallel()

	short := "hello"
	require.Equal(t, short, truncate(short))

	long := strings.Repeat("a", maxContentChars+500)
	result := truncate(long)
	require.Contains(t, result, "<!-- truncated at 40K -->")
	require.Equal(t, maxContentChars, strings.Index(result, truncationMarker))
}

func TestIsSubPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	absTmp, err := filepath.Abs(tmp)
	require.NoError(t, err)

	require.True(t, isSubPath(absTmp, filepath.Join(absTmp, "sub", "file.md")))
	require.True(t, isSubPath(absTmp, absTmp))
	require.False(t, isSubPath(absTmp, filepath.Dir(absTmp)))
}

func TestFileAwareEvaluator(t *testing.T) {
	eval := FileAwareEvaluator("/project/main.go")

	require.True(t, eval("language", "go"))
	require.False(t, eval("language", "py"))
	require.True(t, eval("file", "*.go"))
	require.False(t, eval("file", "*.py"))
	require.False(t, eval("unknown", "value"))

	t.Setenv("TEST_EVAL_VAR", "1")
	require.True(t, eval("env", "TEST_EVAL_VAR"))
	require.False(t, eval("env", "SURELY_NOT_SET_XYZ_123"))
}
