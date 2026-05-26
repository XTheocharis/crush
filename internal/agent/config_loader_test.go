package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentConfigLoader_BuiltinCoder(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, BuiltinCoder, cfg.AgentType)
	require.Equal(t, "An agent that helps with executing coding tasks.", cfg.Description)
	require.Equal(t, 4096, cfg.MaxTokens)
	require.Equal(t, 25, cfg.MaxSteps)
	require.Equal(t, "ask", cfg.PermMode)
}

func TestAgentConfigLoader_BuiltinTask(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	cfg, err := loader.Load(BuiltinTask, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinTask, cfg.Name)
	require.Equal(t, BuiltinTask, cfg.AgentType)
	require.Equal(t, "An agent that helps with searching for context and finding implementation details.", cfg.Description)
	require.Equal(t, 4096, cfg.MaxTokens)
	require.Equal(t, 25, cfg.MaxSteps)
	require.Equal(t, "ask", cfg.PermMode)
}

func TestAgentConfigLoader_RuntimeOverridesBuiltin(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	rt := AgentConfig{
		Model:        "claude-sonnet-4-20250514",
		MaxTurns:     100,
		PermMode:     "auto",
		SystemPrompt: "Custom prompt.",
		Tools:        []string{"bash", "view"},
		Environment:  map[string]string{"FOO": "bar"},
	}
	cfg, err := loader.Load(BuiltinCoder, rt)
	require.NoError(t, err)

	require.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
	require.Equal(t, 100, cfg.MaxTurns)
	require.Equal(t, "auto", cfg.PermMode)
	require.Equal(t, "Custom prompt.", cfg.SystemPrompt)
	require.Equal(t, []string{"bash", "view"}, cfg.Tools)
	require.Equal(t, map[string]string{"FOO": "bar"}, cfg.Environment)

	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, 4096, cfg.MaxTokens)
	require.Equal(t, 25, cfg.MaxSteps)
}

func TestAgentConfigLoader_ProjectOverridesBuiltin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
description: "Project-level coder override"
max_tokens: 8192
max_steps: 50
model: gpt-4
tools:
  - bash
  - edit
  - view
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)

	require.Equal(t, "coder", cfg.Name)
	require.Equal(t, "Project-level coder override", cfg.Description)
	require.Equal(t, 8192, cfg.MaxTokens)
	require.Equal(t, 50, cfg.MaxSteps)
	require.Equal(t, "gpt-4", cfg.Model)
	require.Equal(t, []string{"bash", "edit", "view"}, cfg.Tools)

	require.Equal(t, "ask", cfg.PermMode)
}

func TestAgentConfigLoader_RuntimeOverridesProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
max_tokens: 8192
model: gpt-4
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	rt := AgentConfig{
		Model:    "claude-opus-4",
		MaxTurns: 200,
	}
	cfg, err := loader.Load(BuiltinCoder, rt)
	require.NoError(t, err)

	require.Equal(t, "claude-opus-4", cfg.Model)
	require.Equal(t, 200, cfg.MaxTurns)
	require.Equal(t, 8192, cfg.MaxTokens)
	require.Equal(t, BuiltinCoder, cfg.AgentType)
}

func TestAgentConfigLoader_FullPrecedence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
description: "Project description"
max_tokens: 8192
max_steps: 50
model: gpt-4
perm_mode: auto
tools:
  - bash
  - edit
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	rt := AgentConfig{
		Model:        "claude-sonnet-4-20250514",
		SystemPrompt: "Runtime prompt.",
		MaxTurns:     300,
	}
	cfg, err := loader.Load(BuiltinCoder, rt)
	require.NoError(t, err)

	require.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
	require.Equal(t, "Runtime prompt.", cfg.SystemPrompt)
	require.Equal(t, 300, cfg.MaxTurns)
	require.Equal(t, "Project description", cfg.Description)
	require.Equal(t, 8192, cfg.MaxTokens)
	require.Equal(t, 50, cfg.MaxSteps)
	require.Equal(t, "auto", cfg.PermMode)
	require.Equal(t, []string{"bash", "edit"}, cfg.Tools)
}

func TestAgentConfigLoader_NoProjectDir(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, 4096, cfg.MaxTokens)
}

func TestAgentConfigLoader_EmptyProjectDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, 4096, cfg.MaxTokens)
}

func TestAgentConfigLoader_ProjectCustomAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: reviewer
agent_type: reviewer
description: "Code review agent"
max_tokens: 2048
max_steps: 10
tools:
  - view
  - grep
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "reviewer.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load("reviewer", AgentConfig{})
	require.NoError(t, err)

	require.Equal(t, "reviewer", cfg.Name)
	require.Equal(t, "reviewer", cfg.AgentType)
	require.Equal(t, "Code review agent", cfg.Description)
	require.Equal(t, 2048, cfg.MaxTokens)
	require.Equal(t, 10, cfg.MaxSteps)
	require.Equal(t, []string{"view", "grep"}, cfg.Tools)
}

func TestAgentConfigLoader_ProjectAgentNameFromFilename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `description: "Agent without explicit name"
max_tokens: 1024
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "scout.yml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load("scout", AgentConfig{})
	require.NoError(t, err)

	require.Equal(t, "scout", cfg.Name)
	require.Equal(t, "scout", cfg.AgentType)
	require.Equal(t, "Agent without explicit name", cfg.Description)
	require.Equal(t, 1024, cfg.MaxTokens)
}

func TestAgentConfigLoader_LoadAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
description: "Project coder"
max_tokens: 8192
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	rt := map[string]AgentConfig{
		BuiltinCoder: {Model: "runtime-model"},
		BuiltinTask:  {MaxTurns: 50},
	}

	all, err := loader.LoadAll(rt)
	require.NoError(t, err)

	coder, ok := all[BuiltinCoder]
	require.True(t, ok)
	require.Equal(t, "runtime-model", coder.Model)
	require.Equal(t, 8192, coder.MaxTokens)
	require.Equal(t, "Project coder", coder.Description)

	task, ok := all[BuiltinTask]
	require.True(t, ok)
	require.Equal(t, 50, task.MaxTurns)
	require.Equal(t, BuiltinTask, task.Name)
}

func TestAgentConfigLoader_LoadAllIncludesProjectOnlyAgents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: reviewer
description: "Code review agent"
max_tokens: 2048
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "reviewer.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	all, err := loader.LoadAll(nil)
	require.NoError(t, err)

	_, hasCoder := all[BuiltinCoder]
	require.True(t, hasCoder)
	_, hasTask := all[BuiltinTask]
	require.True(t, hasTask)
	reviewer, hasReviewer := all["reviewer"]
	require.True(t, hasReviewer)
	require.Equal(t, "reviewer", reviewer.Name)
	require.Equal(t, 2048, reviewer.MaxTokens)
}

func TestAgentConfigLoader_BuiltinMethod(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())

	cfg, ok := loader.Builtin(BuiltinCoder)
	require.True(t, ok)
	require.Equal(t, BuiltinCoder, cfg.Name)

	cfg, ok = loader.Builtin(BuiltinTask)
	require.True(t, ok)
	require.Equal(t, BuiltinTask, cfg.Name)

	_, ok = loader.Builtin("nonexistent")
	require.False(t, ok)
}

func TestAgentConfigLoader_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte("invalid: [yaml: content"), 0o644))

	loader := NewAgentConfigLoader(dir)
	_, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse")
}

func TestAgentConfigLoader_IgnoresNonYAMLFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "notes.txt"), []byte("ignore me"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "script.sh"), []byte("#!/bin/sh"), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, BuiltinCoder, cfg.Name)
	require.Equal(t, 4096, cfg.MaxTokens)
}

func TestAgentConfigLoader_MultipleProjectFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	coderYAML := `name: coder
max_tokens: 8192
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(coderYAML), 0o644))

	taskYAML := `name: task
max_steps: 40
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "task.yml"), []byte(taskYAML), 0o644))

	loader := NewAgentConfigLoader(dir)

	coder, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, 8192, coder.MaxTokens)
	require.Equal(t, 25, coder.MaxSteps)

	task, err := loader.Load(BuiltinTask, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, 4096, task.MaxTokens)
	require.Equal(t, 40, task.MaxSteps)
}

func TestAgentConfigLoader_EnvironmentMap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
environment:
  FOO: bar
  BAZ: qux
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"FOO": "bar", "BAZ": "qux"}, cfg.Environment)
}

func TestAgentConfigLoader_AllowedMCP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
allowed_mcp:
  github:
    - search_code
    - read_file
  filesystem:
    - read
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	cfg, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"github":     {"search_code", "read_file"},
		"filesystem": {"read"},
	}, cfg.AllowedMCP)
}

func TestAgentConfigLoader_RuntimeZeroFieldsPreserveBase(t *testing.T) {
	t.Parallel()

	loader := NewAgentConfigLoader(t.TempDir())
	rt := AgentConfig{MaxTokens: 0, MaxSteps: 0}
	cfg, err := loader.Load(BuiltinCoder, rt)
	require.NoError(t, err)
	require.Equal(t, 4096, cfg.MaxTokens)
	require.Equal(t, 25, cfg.MaxSteps)
}

func TestAgentConfigLoader_ProjectCaching(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: coder
max_tokens: 8192
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "coder.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)

	cfg1, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, 8192, cfg1.MaxTokens)

	require.NoError(t, os.Remove(filepath.Join(agentDir, "coder.yaml")))

	cfg2, err := loader.Load(BuiltinCoder, AgentConfig{})
	require.NoError(t, err)
	require.Equal(t, 8192, cfg2.MaxTokens)
}

func TestAgentConfigLoader_SystemPromptOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".crush", "agents", "project")
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	yamlContent := `name: task
system_prompt: "Project-level system prompt"
`
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "task.yaml"), []byte(yamlContent), 0o644))

	loader := NewAgentConfigLoader(dir)
	rt := AgentConfig{SystemPrompt: "Runtime prompt"}
	cfg, err := loader.Load(BuiltinTask, rt)
	require.NoError(t, err)
	require.Equal(t, "Runtime prompt", cfg.SystemPrompt)
}

func TestMergeConfigs(t *testing.T) {
	t.Parallel()

	base := AgentConfig{
		Name:        "coder",
		AgentType:   "coder",
		Description: "Base description",
		MaxTokens:   4096,
		MaxSteps:    25,
		PermMode:    "ask",
		Tools:       []string{"bash"},
		Environment: map[string]string{"A": "1"},
		AllowedMCP:  map[string][]string{"s1": {"t1"}},
	}

	overlay := AgentConfig{
		Description: "Overlay description",
		MaxTokens:   8192,
		Model:       "gpt-4",
		Tools:       []string{"bash", "edit"},
		Environment: map[string]string{"B": "2"},
	}

	result := mergeConfigs(base, overlay)

	require.Equal(t, "coder", result.Name)
	require.Equal(t, "coder", result.AgentType)
	require.Equal(t, "Overlay description", result.Description)
	require.Equal(t, 8192, result.MaxTokens)
	require.Equal(t, 25, result.MaxSteps)
	require.Equal(t, "ask", result.PermMode)
	require.Equal(t, "gpt-4", result.Model)
	require.Equal(t, []string{"bash", "edit"}, result.Tools)
	require.Equal(t, map[string]string{"B": "2"}, result.Environment)
	require.Equal(t, map[string][]string{"s1": {"t1"}}, result.AllowedMCP)
}

func TestMergeConfigsZeroOverlay(t *testing.T) {
	t.Parallel()

	base := AgentConfig{
		Name:      "coder",
		MaxTokens: 4096,
		MaxSteps:  25,
	}
	result := mergeConfigs(base, AgentConfig{})
	require.Equal(t, base, result)
}
