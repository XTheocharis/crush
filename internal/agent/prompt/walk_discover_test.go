package prompt

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func newTestConfigStore(opts ...func(*config.Options)) *config.ConfigStore {
	o := &config.Options{
		ContextPaths: []string{},
	}
	for _, fn := range opts {
		fn(o)
	}
	return config.NewTestStore(&config.Config{Options: o})
}

func TestAutoDiscoveryFindsContextFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("auto-discovered agents"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("auto-discovered crush"), 0o644))

	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Contains(t, result, "auto-discovered agents")
	require.Contains(t, result, "auto-discovered crush")
}

func TestAutoDiscoveryEmptyDirNoContextFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Empty(t, strings.TrimSpace(result))
}

func TestExplicitPathsStillWork(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	explicitFile := filepath.Join(tmp, "custom-instructions.md")
	require.NoError(t, os.WriteFile(explicitFile, []byte("explicit content"), 0o644))

	store := newTestConfigStore(func(o *config.Options) {
		o.ContextPaths = []string{explicitFile}
	})
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Contains(t, result, "explicit content")
}

func TestExplicitOverridesDiscoveredByBasename(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Auto-discovered file at root.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("discovered content"), 0o644))

	// Explicit path to a different AGENTS.md.
	explicitDir := t.TempDir()
	explicitFile := filepath.Join(explicitDir, "AGENTS.md")
	require.NoError(t, os.WriteFile(explicitFile, []byte("explicit content"), 0o644))

	store := newTestConfigStore(func(o *config.Options) {
		o.ContextPaths = []string{explicitFile}
	})
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	// Explicit AGENTS.md should appear.
	require.Contains(t, result, "explicit content")

	// Discovered AGENTS.md should NOT appear (same basename overridden).
	require.NotContains(t, result, "discovered content")
}

func TestDiscoveredAndExplicitCoexistDifferentBasenames(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Discovered file.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("discovered agents"), 0o644))

	// Explicit file with different basename.
	explicitFile := filepath.Join(tmp, "custom.md")
	require.NoError(t, os.WriteFile(explicitFile, []byte("explicit custom"), 0o644))

	store := newTestConfigStore(func(o *config.Options) {
		o.ContextPaths = []string{explicitFile}
	})
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Contains(t, result, "discovered agents")
	require.Contains(t, result, "explicit custom")
}

func TestDeduplicationByAbsPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("content"), 0o644))

	// No explicit paths, so only discovered. WalkContextPaths already deduplicates.
	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	agentsCount := 0
	for _, line := range lines {
		if strings.Contains(line, "AGENTS.md") {
			agentsCount++
		}
	}
	require.Equal(t, 1, agentsCount, "AGENTS.md should appear exactly once")
}

func TestDiscoveryFindsSubdirectoryContextFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "pkg", "CRUSH.md"), []byte("pkg context"), 0o644))

	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Contains(t, result, "pkg context")
}

func TestExplicitEmptyOnlyDiscovered(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("only discovered"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("also discovered"), 0o644))

	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}|{{.Content}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	require.Contains(t, result, "only discovered")
	require.Contains(t, result, "also discovered")
}

func TestDiscoveredPathsAreSortedByBasename(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CRUSH.md"), []byte("c"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("cl"), 0o644))

	store := newTestConfigStore()
	p, err := NewPrompt("test", "{{range .ContextFiles}}{{.Path}}\n{{end}}", WithWorkingDir(tmp))
	require.NoError(t, err)

	result, err := p.Build(context.Background(), "test-provider", "test-model", store)
	require.NoError(t, err)

	var basenames []string
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		basenames = append(basenames, filepath.Base(line))
	}
	sort.Strings(basenames)
	require.Contains(t, basenames, "AGENTS.md")
	require.Contains(t, basenames, "CRUSH.md")
	require.Contains(t, basenames, "CLAUDE.md")
}
