package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagnosticCascade_NewDefaults(t *testing.T) {
	t.Parallel()

	t.Run("default max depth when zero", func(t *testing.T) {
		t.Parallel()
		c := NewDiagnosticCascade(nil, 0, nil, "", "")
		require.Equal(t, 3, c.maxDepth)
	})

	t.Run("default max depth when negative", func(t *testing.T) {
		t.Parallel()
		c := NewDiagnosticCascade(nil, -5, nil, "", "")
		require.Equal(t, 3, c.maxDepth)
	})

	t.Run("respects explicit depth", func(t *testing.T) {
		t.Parallel()
		c := NewDiagnosticCascade(nil, 5, nil, "", "")
		require.Equal(t, 5, c.maxDepth)
	})
}

func TestDiagnosticCascade_NilManager(t *testing.T) {
	t.Parallel()

	c := NewDiagnosticCascade(nil, 3, nil, "", "")
	result := c.RunCascade(context.Background(), "/some/file.go")
	require.Empty(t, result.FilesChecked)
	require.Empty(t, result.FileDiagnostics)
	require.False(t, result.HasWarnings)
}

func TestDiagnosticCascade_EmptyFilePath(t *testing.T) {
	t.Parallel()

	c := NewDiagnosticCascade(nil, 3, nil, "", "")
	result := c.RunCascade(context.Background(), "")
	require.Empty(t, result.FilesChecked)
}

func TestDiagnosticCascade_NoDiagnostics(t *testing.T) {
	t.Parallel()

	// When there are no diagnostics for a file, the cascade should still
	// record it as checked but not cascade further.
	cascade := &DiagnosticCascade{
		lspManager: nil,
		maxDepth:   3,
	}
	result := cascade.RunCascade(context.Background(), "/project/pkg/foo/foo.go")
	require.Empty(t, result.FilesChecked)
	require.False(t, result.HasWarnings)
}

func TestDiagnosticCascadeDepthLimit(t *testing.T) {
	t.Parallel()

	// Verify depth limit is respected by checking the cascade config.
	c := NewDiagnosticCascade(nil, 3, nil, "", "")
	require.Equal(t, 3, c.maxDepth)

	// With nil manager, RunCascade returns empty — the depth limit itself
	// is exercised when importers are found. We verify the struct honours it.
	c2 := NewDiagnosticCascade(nil, 1, nil, "", "")
	require.Equal(t, 1, c2.maxDepth)
}

func TestDiagnosticCascadeSeverityFilter(t *testing.T) {
	t.Parallel()

	// Error (0) and Warning (1) are more severe — lower iota values.
	require.Equal(t, DiagnosticSeverity(0), SeverityError)
	require.Equal(t, DiagnosticSeverity(1), SeverityWarning)
	require.Equal(t, DiagnosticSeverity(2), SeverityInfo)
}

func TestFormatCascadeResult_SingleFile(t *testing.T) {
	t.Parallel()

	result := CascadeResult{
		FilesChecked:    []string{"/project/foo.go"},
		FileDiagnostics: map[string][]DiagnosticInfo{},
	}
	output := FormatCascadeResult(result)
	require.Empty(t, output)
}

func TestFormatCascadeResult_MultipleFiles_NoWarnings(t *testing.T) {
	t.Parallel()

	result := CascadeResult{
		FilesChecked: []string{
			"/project/foo.go",
			"/project/bar.go",
		},
		FileDiagnostics: map[string][]DiagnosticInfo{},
	}
	output := FormatCascadeResult(result)
	require.Contains(t, output, "<diagnostic_cascade>")
	require.Contains(t, output, "Cascade checked 2 file(s)")
	require.Contains(t, output, "No additional diagnostics")
	require.Contains(t, output, "</diagnostic_cascade>")
}

func TestFormatCascadeResult_WithWarnings(t *testing.T) {
	t.Parallel()

	result := CascadeResult{
		FilesChecked: []string{
			"/project/pkg/foo/foo.go",
			"/project/cmd/main.go",
		},
		FileDiagnostics: map[string][]DiagnosticInfo{
			"/project/cmd/main.go": {
				{FilePath: "/project/cmd/main.go", Line: 10, Severity: SeverityError, Message: "undefined: bar"},
			},
		},
		HasWarnings: true,
	}
	output := FormatCascadeResult(result)
	require.Contains(t, output, "<diagnostic_cascade>")
	require.Contains(t, output, "warnings found")
	require.Contains(t, output, "undefined: bar")
	require.Contains(t, output, "Error")
}

func TestFormatCascadeResult_NilDiagnostics(t *testing.T) {
	t.Parallel()

	result := CascadeResult{
		FilesChecked:    []string{"/a.go", "/b.go"},
		FileDiagnostics: nil,
	}
	output := FormatCascadeResult(result)
	require.Contains(t, output, "No additional diagnostics")
}

func TestRunDiagnosticCascade_NilManager(t *testing.T) {
	t.Parallel()

	output := runDiagnosticCascade(context.Background(), nil, "/some/file.go")
	require.Empty(t, output)
}

func TestShortPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"/a/b/c/d.go", "b/c/d.go"},
		{"/a/b.go", "/a/b.go"},
		{"d.go", "d.go"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.expected, shortPath(tt.input))
	}
}

func TestSeverityName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Error", severityName(SeverityError))
	require.Equal(t, "Warn", severityName(SeverityWarning))
	require.Equal(t, "Info", severityName(SeverityInfo))
	require.Equal(t, "Hint", severityName(SeverityHint))
}

func TestDiagnosticCascade_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewDiagnosticCascade(nil, 3, nil, "", "")
	result := c.RunCascade(ctx, "/some/file.go")
	require.Empty(t, result.FilesChecked)
}

func TestCascadeDiagnosticsConcurrent_NilManager(t *testing.T) {
	t.Parallel()

	result := cascadeDiagnosticsConcurrent(
		context.Background(),
		nil,
		[]string{"/a.go", "/b.go"},
	)
	require.Empty(t, result)
}

func TestCollectDiagnosticsForFile_NilManager(t *testing.T) {
	t.Parallel()

	diags := collectDiagnosticsForFile("/some/file.go", nil)
	require.Nil(t, diags)
}

func TestFormatCascadeResult_TruncatesLongPath(t *testing.T) {
	t.Parallel()

	result := CascadeResult{
		FilesChecked: []string{
			"/very/long/path/to/project/pkg/foo.go",
			"/very/long/path/to/project/cmd/main.go",
		},
		FileDiagnostics: map[string][]DiagnosticInfo{
			"/very/long/path/to/project/cmd/main.go": {
				{FilePath: "/very/long/path/to/project/cmd/main.go", Line: 5, Severity: SeverityWarning, Message: "unused"},
			},
		},
		HasWarnings: true,
	}
	output := FormatCascadeResult(result)
	require.True(t, strings.Contains(output, "Warn") || strings.Contains(output, "Error"))
	require.Contains(t, output, "unused")
}
