package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigLoaderWired(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	loader := NewAgentConfigLoader(dir)
	require.NotNil(t, loader)

	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, 4096, cfg.MaxTokens)
}

func TestAgentConfigLoaderProjectOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
description: "Project-level override"
max_tokens: 16384
max_steps: 60
model: claude-opus-4
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)

	require.Equal(t, "coder", cfg.Name)
	require.Equal(t, "Project-level override", cfg.Description)
	require.Equal(t, 16384, cfg.MaxTokens)
	require.Equal(t, 60, cfg.MaxSteps)
	require.Equal(t, "claude-opus-4", cfg.Model)
}

func TestAgentConfigLoaderBuiltinFallback(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)

	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, BuiltinCoder, cfg.AgentType)
	require.Equal(t, 4096, cfg.MaxTokens)
	require.Equal(t, 25, cfg.MaxSteps)
	require.Equal(t, "ask", cfg.PermMode)
}

func TestCoordinatorLoadAgentConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
max_tokens: 9999
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	cfg, err := config.Init(dir, "", false)
	require.NoError(t, err)
	coord := &coordinator{
		cfg:          cfg,
		configLoader: NewAgentConfigLoader(dir),
	}

	got, err := coord.LoadAgentConfig(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, 9999, got.MaxTokens)
}
