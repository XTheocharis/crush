//go:build ignore
// +build ignore

package lsp

import (
	"testing"

	powernapconfig "github.com/charmbracelet/x/powernap/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestPriorityConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, PriorityCritical)
	require.Equal(t, 1, PriorityHigh)
	require.Equal(t, 2, PriorityNormal)
	require.Equal(t, 3, PriorityLow)
	require.Less(t, PriorityCritical, PriorityHigh)
	require.Less(t, PriorityHigh, PriorityNormal)
	require.Less(t, PriorityNormal, PriorityLow)
}

func TestServerPriority(t *testing.T) {
	t.Parallel()
	critical := []string{
		"gopls",
		"typescript-language-server",
		"vscode-css-langserver",
		"vscode-html-languageserver",
		"json-language-server",
		"yaml-language-server",
		"rust-analyzer",
		"pyright",
		"clangd",
		"kotlin-language-server",
		"csharp-ls",
		"dockerfile-languageserver",
	}
	for _, name := range critical {
		require.Equal(t, PriorityCritical, serverPriority(name), "expected critical priority for %q", name)
	}

	unknowns := []string{"nil", "lua-language-server", "terraform-ls", "some-random-server"}
	for _, name := range unknowns {
		require.Equal(t, PriorityNormal, serverPriority(name), "expected normal priority for %q", name)
	}
}

func TestSortServersByPriority(t *testing.T) {
	t.Parallel()

	servers := map[string]*powernapconfig.ServerConfig{
		"nil":                        {Command: "nil"},
		"gopls":                      {Command: "gopls"},
		"terraform-ls":               {Command: "terraform-ls"},
		"rust-analyzer":              {Command: "rust-analyzer"},
		"typescript-language-server": {Command: "typescript-language-server"},
		"lua-language-server":        {Command: "lua-language-server"},
	}

	sorted := sortServersByPriority(servers)
	require.Len(t, sorted, len(servers))

	criticalNames := map[string]bool{
		"gopls": true, "rust-analyzer": true, "typescript-language-server": true,
	}

	lastCriticalIdx := -1
	for i, entry := range sorted {
		if criticalNames[entry.Name] {
			lastCriticalIdx = i
		}
	}

	firstNonCriticalIdx := len(sorted)
	for i, entry := range sorted {
		if !criticalNames[entry.Name] {
			firstNonCriticalIdx = i
			break
		}
	}

	require.Less(t, lastCriticalIdx, firstNonCriticalIdx,
		"all critical servers should appear before non-critical ones")
}

func TestSortServersByPriorityEmpty(t *testing.T) {
	t.Parallel()
	sorted := sortServersByPriority(map[string]*powernapconfig.ServerConfig{})
	require.Empty(t, sorted)
}

func TestSortServersByPriorityAllCritical(t *testing.T) {
	t.Parallel()
	servers := map[string]*powernapconfig.ServerConfig{
		"gopls":         {Command: "gopls"},
		"rust-analyzer": {Command: "rust-analyzer"},
	}
	sorted := sortServersByPriority(servers)
	require.Len(t, sorted, 2)
	for _, entry := range sorted {
		require.Equal(t, PriorityCritical, serverPriority(entry.Name))
	}
}

func TestSortServersByPriorityPreservesConfigs(t *testing.T) {
	t.Parallel()

	cfg := &powernapconfig.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	servers := map[string]*powernapconfig.ServerConfig{
		"gopls": cfg,
	}
	sorted := sortServersByPriority(servers)
	require.Len(t, sorted, 1)
	require.Same(t, cfg, sorted[0].Config)
	require.Equal(t, "gopls", sorted[0].Name)
}
