package explorer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/testutil"
	"github.com/stretchr/testify/require"
)

// repoRoot returns the crush repository root for reading real files.
func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find the repo root.
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

func TestIntegration_ExplorerWithLLM_GoFile(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	llm := testutil.NewLLMClient(t)
	reg := NewRegistryWithLLM(llm, nil)

	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "internal/lcm/summarizer.go"))
	require.NoError(t, err)

	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "internal/lcm/summarizer.go",
		Content: content,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Summary)
	require.Contains(t, result.ExplorerUsed, "+llm",
		"should use LLM-enhanced exploration")
	require.Greater(t, result.TokenEstimate, 0)

	t.Logf("Explorer used: %s", result.ExplorerUsed)
	t.Logf("Token estimate: %d", result.TokenEstimate)
	t.Logf("Summary:\n%s", result.Summary)
}

func TestIntegration_ExplorerWithLLM_JSONFile(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	llm := testutil.NewLLMClient(t)
	reg := NewRegistryWithLLM(llm, nil)

	// Use the crush.json schema as a representative JSON file.
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "crush.json.schema"))
	if err != nil {
		// Fall back to go.mod if schema doesn't exist.
		content, err = os.ReadFile(filepath.Join(root, "go.mod"))
		require.NoError(t, err)
		result, err := reg.Explore(context.Background(), ExploreInput{
			Path:    "go.mod",
			Content: content,
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.Summary)
		t.Logf("Explorer used: %s", result.ExplorerUsed)
		t.Logf("Summary:\n%s", result.Summary)
		return
	}

	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "crush.json.schema",
		Content: content,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Summary)

	t.Logf("Explorer used: %s", result.ExplorerUsed)
	t.Logf("Summary:\n%s", result.Summary)
}

func TestIntegration_ExplorerWithLLM_MarkdownFile(t *testing.T) {
	testutil.SkipIfNoIntegration(t)
	t.Parallel()

	llm := testutil.NewLLMClient(t)
	reg := NewRegistryWithLLM(llm, nil)

	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, err)

	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "CLAUDE.md",
		Content: content,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Summary)

	// Markdown files should be explored (may or may not get +llm depending
	// on the explorer chain ordering).
	require.True(t, strings.Contains(result.ExplorerUsed, "markdown") ||
		strings.Contains(result.ExplorerUsed, "text") ||
		strings.Contains(result.ExplorerUsed, "llm"),
		"should use a text-based or LLM explorer for markdown")

	t.Logf("Explorer used: %s", result.ExplorerUsed)
	t.Logf("Summary:\n%s", result.Summary)
}
