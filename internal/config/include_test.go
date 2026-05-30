package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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
	require.Contains(t, err.Error(), "unexpected endif")
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

func TestTruncateFunction_Multibyte(t *testing.T) {
	t.Parallel()

	// Japanese characters are 3 bytes each in UTF-8.
	input := strings.Repeat("日本語テスト", maxContentChars/5+100)
	result := truncate(input)
	require.Contains(t, result, "<!-- truncated at 40K -->")
	require.True(t, utf8.ValidString(result))
	// Verify the content before the marker has exactly maxContentChars runes.
	markerIdx := strings.Index(result, truncationMarker)
	content := result[:markerIdx]
	require.Equal(t, maxContentChars, utf8.RuneCountInString(content))
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

func TestIncludeProcessing(t *testing.T) {
	t.Parallel()

	t.Run("SimpleJSONInclude", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		providersFile := filepath.Join(tmp, "providers.json")
		require.NoError(t, os.WriteFile(providersFile, []byte(`{"openai":{"api_key":"$KEY"}}`), 0o644))

		main := filepath.Join(tmp, "crush.json")
		content := `{"providers":{"@include":"providers.json"},"options":{"debug":true}}`
		require.NoError(t, os.WriteFile(main, []byte(content), 0o644))

		data, err := os.ReadFile(main)
		require.NoError(t, err)

		result, err := processJSONIncludes(data, tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))

		providers, ok := parsed["providers"].(map[string]any)
		require.True(t, ok, "providers should be an object")
		openai, ok := providers["openai"].(map[string]any)
		require.True(t, ok, "providers.openai should be an object")
		require.Equal(t, "$KEY", openai["api_key"])

		options, ok := parsed["options"].(map[string]any)
		require.True(t, ok, "options should be an object")
		require.Equal(t, true, options["debug"])
	})

	t.Run("NestedJSONInclude", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "sub"), 0o755))

		inner := filepath.Join(tmp, "sub", "inner.json")
		require.NoError(t, os.WriteFile(inner, []byte(`{"model":"gpt-4"}`), 0o644))

		middle := filepath.Join(tmp, "sub", "middle.json")
		require.NoError(t, os.WriteFile(middle, []byte(`{"@include":"inner.json"}`), 0o644))

		main := filepath.Join(tmp, "crush.json")
		content := `{"models":{"@include":"sub/middle.json"}}`
		require.NoError(t, os.WriteFile(main, []byte(content), 0o644))

		data, err := os.ReadFile(main)
		require.NoError(t, err)

		result, err := processJSONIncludes(data, tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))
		models, ok := parsed["models"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "gpt-4", models["model"])
	})

	t.Run("CycleDetection", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.json")
		b := filepath.Join(tmp, "b.json")
		require.NoError(t, os.WriteFile(a, []byte(`{"@include":"b.json"}`), 0o644))
		require.NoError(t, os.WriteFile(b, []byte(`{"@include":"a.json"}`), 0o644))

		content := `{"@include":"a.json"}`
		_, err := processJSONIncludes([]byte(content), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("PathEscape", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		content := `{"@include":"../../../etc/passwd"}`
		_, err := processJSONIncludes([]byte(content), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "'..'")
	})

	t.Run("AbsolutePath", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		content := `{"@include":"/etc/passwd"}`
		_, err := processJSONIncludes([]byte(content), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "absolute")
	})

	t.Run("MissingFile", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		content := `{"data":{"@include":"nonexistent.json"}}`
		_, err := processJSONIncludes([]byte(content), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to read")
	})

	t.Run("InvalidJSONInclude", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		bad := filepath.Join(tmp, "bad.json")
		require.NoError(t, os.WriteFile(bad, []byte(`not json`), 0o644))

		content := `{"@include":"bad.json"}`
		_, err := processJSONIncludes([]byte(content), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not valid JSON")
	})

	t.Run("NoIncludesPassthrough", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		content := `{"options":{"debug":true}}`
		result, err := processJSONIncludes([]byte(content), tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))
		options := parsed["options"].(map[string]any)
		require.Equal(t, true, options["debug"])
	})

	t.Run("IncludeInArray", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		item := filepath.Join(tmp, "item.json")
		require.NoError(t, os.WriteFile(item, []byte(`{"name":"test"}`), 0o644))

		content := `{"items":[{"@include":"item.json"},{"name":"other"}]}`
		result, err := processJSONIncludes([]byte(content), tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))
		items := parsed["items"].([]any)
		require.Len(t, items, 2)
		first := items[0].(map[string]any)
		require.Equal(t, "test", first["name"])
	})

	t.Run("IncludeNotSingleKey", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		// @include with extra keys is NOT treated as an include directive.
		content := `{"@include":"file.json","extra":true}`
		result, err := processJSONIncludes([]byte(content), tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))
		require.Equal(t, "file.json", parsed["@include"])
		require.Equal(t, true, parsed["extra"])
	})
}

func TestAtIfSyntax(t *testing.T) {
	t.Run("EnvSet", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_AT_TEST", "1")

		content := `before
@if env=CRUSH_AT_TEST
visible
@endif
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "visible")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("EnvUnset", func(t *testing.T) {
		tmp := t.TempDir()

		content := `before
@if env=NONEXISTENT_AT_VAR
hidden
@endif
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.NotContains(t, result, "hidden")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("LanguageCondition", func(t *testing.T) {
		tmp := t.TempDir()

		eval := FileAwareEvaluator(filepath.Join(tmp, "main.go"))
		content := `@if language=go
go code
@endif
@if language=py
py code
@endif`
		result, err := ProcessIncludes(content, tmp, 0, nil, eval)
		require.NoError(t, err)
		require.Contains(t, result, "go code")
		require.NotContains(t, result, "py code")
	})

	t.Run("NestedError", func(t *testing.T) {
		tmp := t.TempDir()

		content := `@if env=FOO
@if env=BAR
nested
@endif
@endif`
		_, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nested conditional")
	})

	t.Run("UnmatchedEndif", func(t *testing.T) {
		tmp := t.TempDir()

		_, err := ProcessIncludes(`@endif`, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected endif")
	})

	t.Run("UnclosedConditional", func(t *testing.T) {
		tmp := t.TempDir()

		content := "@if env=FOO\nno end"
		_, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unclosed conditional")
	})

	t.Run("MixedSyntax", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_MIXED_TEST", "1")

		content := `before
<!-- if: env:CRUSH_MIXED_TEST -->
html-style visible
<!-- endif -->
@if env=CRUSH_MIXED_TEST
at-style visible
@endif
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "html-style visible")
		require.Contains(t, result, "at-style visible")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("CrossSyntaxEndifError", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_CROSS_TEST", "1")

		content := `@if env=CRUSH_CROSS_TEST
content
<!-- endif -->`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "content")
	})

	t.Run("ConditionalWithInclude", func(t *testing.T) {
		tmp := t.TempDir()

		snippet := filepath.Join(tmp, "snippet.md")
		require.NoError(t, os.WriteFile(snippet, []byte("included content"), 0o644))

		t.Setenv("CRUSH_AT_INCLUDE", "yes")

		content := `header
@if env=CRUSH_AT_INCLUDE
@include snippet.md
@endif
footer`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "included content")
		require.Contains(t, result, "header")
		require.Contains(t, result, "footer")
	})
}

func TestConditionalSyntax(t *testing.T) {
	t.Run("EnvSet", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_TEST_COND", "1")

		content := `before
<!-- if: env:CRUSH_TEST_COND -->
visible
<!-- endif -->
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "visible")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("EnvUnset", func(t *testing.T) {
		tmp := t.TempDir()

		content := `before
<!-- if: env:NONEXISTENT_VAR_XYZ -->
hidden
<!-- endif -->
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.NotContains(t, result, "hidden")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("LanguageCondition", func(t *testing.T) {
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
	})

	t.Run("FileCondition", func(t *testing.T) {
		tmp := t.TempDir()

		eval := FileAwareEvaluator(filepath.Join(tmp, "main.go"))
		content := `<!-- if: file:*.go -->
go content
<!-- endif -->
<!-- if: file:*.py -->
py content
<!-- endif -->`
		result, err := ProcessIncludes(content, tmp, 0, nil, eval)
		require.NoError(t, err)
		require.Contains(t, result, "go content")
		require.NotContains(t, result, "py content")
	})

	t.Run("NestedError", func(t *testing.T) {
		tmp := t.TempDir()

		content := `<!-- if: env:FOO -->
<!-- if: env:BAR -->
nested
<!-- endif -->
<!-- endif -->`
		_, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "nested conditional")
	})

	t.Run("UnmatchedEndif", func(t *testing.T) {
		tmp := t.TempDir()

		_, err := ProcessIncludes(`<!-- endif -->`, tmp, 0, nil, nil)
		require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected endif")
	})

	t.Run("UnclosedConditional", func(t *testing.T) {
		tmp := t.TempDir()

		content := "<!-- if: env:FOO -->\nno end"
		_, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unclosed conditional")
	})

	t.Run("ConditionalWithInclude", func(t *testing.T) {
		tmp := t.TempDir()

		snippet := filepath.Join(tmp, "snippet.md")
		require.NoError(t, os.WriteFile(snippet, []byte("included content"), 0o644))

		t.Setenv("CRUSH_COND_INCLUDE", "yes")

		content := `header
<!-- if: env:CRUSH_COND_INCLUDE -->
@include snippet.md
<!-- endif -->
footer`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "included content")
		require.Contains(t, result, "header")
		require.Contains(t, result, "footer")
	})
}
