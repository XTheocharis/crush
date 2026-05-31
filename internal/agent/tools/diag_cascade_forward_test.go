//go:build treesitter

package tools

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestForwardImport_NilParser(t *testing.T) {
	t.Parallel()

	result, err := ForwardImportResolution(
		context.Background(), nil, "/some/file.go", "/project", "example.com/project",
	)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestForwardImport_EmptyPath(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	result, err := ForwardImportResolution(
		context.Background(), parser, "", "/project", "example.com/project",
	)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestForwardImport_NonexistentFile(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	_, err := ForwardImportResolution(
		context.Background(), parser, "/nonexistent/file.go", "/project", "example.com/project",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read file")
}

func TestForwardImport_NoImports(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, goFile, tmpDir, "example.com/project",
	)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestForwardImport_SkipsStdlib(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	content := `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.Join([]string{"a", "b"}, ","))
}
`
	err := os.WriteFile(goFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, goFile, tmpDir, "example.com/project",
	)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestForwardImport_ResolvesLocalPackage(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	modulePath := "example.com/project"

	// Create the imported package directory.
	pkgDir := filepath.Join(tmpDir, "utils")
	err := os.MkdirAll(pkgDir, 0o755)
	require.NoError(t, err)

	// Write the imported package with exported symbols.
	pkgFile := filepath.Join(pkgDir, "utils.go")
	pkgContent := `package utils

import "fmt"

func Helper() string {
	return "help"
}

func helperInternal() string {
	return "internal"
}

type Config struct {
	Name string
}

type unexported struct{}

var Version = "1.0"

const MaxRetries = 3
`
	err = os.WriteFile(pkgFile, []byte(pkgContent), 0o644)
	require.NoError(t, err)

	// Write the importing file.
	mainFile := filepath.Join(tmpDir, "main.go")
	mainContent := `package main

import (
	"fmt"
	"` + modulePath + `/utils"
)

func main() {
	fmt.Println(utils.Helper())
}
`
	err = os.WriteFile(mainFile, []byte(mainContent), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, mainFile, tmpDir, modulePath,
	)
	require.NoError(t, err)

	importPath := modulePath + "/utils"
	require.Contains(t, result, importPath, "expected import path %q in result", importPath)

	symbols := result[importPath]
	sort.Strings(symbols)

	require.Contains(t, symbols, "Config")
	require.Contains(t, symbols, "Helper")
	require.Contains(t, symbols, "MaxRetries")
	require.Contains(t, symbols, "Version")
	require.NotContains(t, symbols, "helperInternal")
	require.NotContains(t, symbols, "unexported")
}

func TestForwardImport_SkipsThirdParty(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	modulePath := "example.com/project"

	mainFile := filepath.Join(tmpDir, "main.go")
	content := `package main

import (
	"github.com/some/external/pkg"
)

func main() {
	pkg.DoSomething()
}
`
	err := os.WriteFile(mainFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, mainFile, tmpDir, modulePath,
	)
	require.NoError(t, err)
	require.Empty(t, result, "third-party imports should be skipped")
}

func TestForwardImport_NonexistentImportDir(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	modulePath := "example.com/project"

	mainFile := filepath.Join(tmpDir, "main.go")
	content := `package main

import "` + modulePath + `/nonexistent"

func main() {}
`
	err := os.WriteFile(mainFile, []byte(content), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, mainFile, tmpDir, modulePath,
	)
	require.NoError(t, err)
	require.Empty(t, result, "non-existent import should be skipped gracefully")
}

func TestForwardImport_SkipsTestFiles(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	tmpDir := t.TempDir()
	modulePath := "example.com/project"

	pkgDir := filepath.Join(tmpDir, "pkg")
	err := os.MkdirAll(pkgDir, 0o755)
	require.NoError(t, err)

	// Production file with exported symbol.
	prodFile := filepath.Join(pkgDir, "handler.go")
	err = os.WriteFile(prodFile, []byte(`package pkg

func Handle() {}
`), 0o644)
	require.NoError(t, err)

	// Test file with exported symbol that should be ignored.
	testFile := filepath.Join(pkgDir, "handler_test.go")
	err = os.WriteFile(testFile, []byte(`package pkg

func TestHandle() {}
`), 0o644)
	require.NoError(t, err)

	mainFile := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(mainFile, []byte(`package main

import "`+modulePath+`/pkg"

func main() {}
`), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		context.Background(), parser, mainFile, tmpDir, modulePath,
	)
	require.NoError(t, err)

	importPath := modulePath + "/pkg"
	require.Contains(t, result, importPath)
	require.Contains(t, result[importPath], "Handle")
	require.NotContains(t, result[importPath], "TestHandle")
}

func TestForwardImport_ContextCancellation(t *testing.T) {
	t.Parallel()

	parser := treesitter.NewParser()
	defer parser.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmpDir := t.TempDir()
	mainFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(mainFile, []byte("package main\n"), 0o644)
	require.NoError(t, err)

	result, err := ForwardImportResolution(
		ctx, parser, mainFile, tmpDir, "example.com/project",
	)
	// Cancelled context may return empty result or context error.
	if err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
	_ = result
}

func TestIsGoExported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected bool
	}{
		{"Hello", true},
		{"hello", false},
		{"", false},
		{"_private", false},
		{"Config", true},
		{"MAX_RETRIES", true},
		{"123Start", false},
	}

	for _, tt := range tests {
		require.Equal(t, tt.expected, isGoExported(tt.name), "isGoExported(%q)", tt.name)
	}
}
