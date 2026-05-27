package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Builtin agent name constants matching config.AgentCoder / config.AgentTask.
const (
	BuiltinCoder = "coder"
	BuiltinTask  = "task"
)

// AgentConfigLoader loads agent configs from multiple sources with override
// precedence: runtime > project > builtin.
type AgentConfigLoader struct {
	// workingDir is the project root used to discover .crush/agents/project/
	// YAML overrides.
	workingDir string

	// builtin holds the hardcoded default configs keyed by agent name.
	builtin map[string]AgentConfig

	// project is the cached project-level overrides, keyed by agent name.
	// Populated on first call to Load or LoadAll.
	project map[string]AgentConfig

	// projectLoaded tracks whether project YAML files have been read.
	projectLoaded bool
}

// NewAgentConfigLoader creates a loader that uses workingDir to discover
// project-level agent YAML overrides in .crush/agents/project/.
func NewAgentConfigLoader(workingDir string) *AgentConfigLoader {
	return &AgentConfigLoader{
		workingDir: workingDir,
		builtin:    defaultBuiltinConfigs(),
		project:    make(map[string]AgentConfig),
	}
}

// Load returns the merged AgentConfig for the named agent, applying
// precedence: runtime > project > builtin.
//
// name must be a known builtin agent ("coder" or "task"). If name is not a
// builtin, Load attempts to find a project-level config for it; if none
// exists, it returns an error.
func (l *AgentConfigLoader) Load(name string, runtime AgentConfig) (AgentConfig, error) {
	if err := l.ensureProjectLoaded(); err != nil {
		return AgentConfig{}, fmt.Errorf("agent config loader: %w", err)
	}

	// Start with builtin (or zero value if unknown).
	base := l.builtin[name]

	// Layer project overrides on top.
	if proj, ok := l.project[name]; ok {
		base = mergeConfigs(base, proj)
	}

	// Layer runtime overrides on top.
	base = mergeConfigs(base, runtime)

	// Fill in the name and agent type if still empty.
	if base.Name == "" {
		base.Name = name
	}
	if base.AgentType == "" {
		base.AgentType = name
	}

	if err := base.Validate(); err != nil {
		return AgentConfig{}, fmt.Errorf("agent config loader: %w", err)
	}

	return base, nil
}

// LoadAll returns merged configs for every known agent (builtin + project-only
// agents), applying runtime overrides for each when present. The runtime map
// is keyed by agent name.
func (l *AgentConfigLoader) LoadAll(runtime map[string]AgentConfig) (map[string]AgentConfig, error) {
	if err := l.ensureProjectLoaded(); err != nil {
		return nil, fmt.Errorf("agent config loader: %w", err)
	}

	// Collect all agent names.
	names := make(map[string]bool)
	for name := range l.builtin {
		names[name] = true
	}
	for name := range l.project {
		names[name] = true
	}

	result := make(map[string]AgentConfig, len(names))
	for name := range names {
		rt := runtime[name] // zero value if absent, which is fine.
		cfg, err := l.Load(name, rt)
		if err != nil {
			return nil, fmt.Errorf("agent config loader: load %q: %w", name, err)
		}
		result[name] = cfg
	}

	return result, nil
}

// Builtin returns a copy of the builtin config for the given agent name.
// Returns false if no builtin config exists for that name.
func (l *AgentConfigLoader) Builtin(name string) (AgentConfig, bool) {
	cfg, ok := l.builtin[name]
	return cfg, ok
}

// ensureProjectLoaded lazily reads .crush/agents/project/*.yaml files on first
// access. Subsequent calls are no-ops.
func (l *AgentConfigLoader) ensureProjectLoaded() error {
	if l.projectLoaded {
		return nil
	}
	l.projectLoaded = true

	dir := filepath.Join(l.workingDir, ".crush", "agents", "project")

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read project agents dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		cfg, err := loadAgentYAML(path)
		if err != nil {
			return fmt.Errorf("load project agent %s: %w", path, err)
		}

		// Derive the agent name from the file stem if Name is not set in YAML.
		name := cfg.Name
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			cfg.Name = name
		}
		if cfg.AgentType == "" {
			cfg.AgentType = name
		}

		l.project[name] = cfg
	}

	return nil
}

// loadAgentYAML reads a single YAML file and unmarshals it into an AgentConfig.
func loadAgentYAML(path string) (AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AgentConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}

// mergeConfigs returns a new AgentConfig where non-zero fields in overlay
// replace the corresponding fields in base. Slices and maps in overlay replace
// (not append to) those in base. Zero values in overlay are ignored.
func mergeConfigs(base, overlay AgentConfig) AgentConfig {
	result := base

	if overlay.Name != "" {
		result.Name = overlay.Name
	}
	if overlay.AgentType != "" {
		result.AgentType = overlay.AgentType
	}
	if overlay.Description != "" {
		result.Description = overlay.Description
	}
	if len(overlay.Tools) > 0 {
		result.Tools = overlay.Tools
	}
	if len(overlay.Permissions) > 0 {
		result.Permissions = overlay.Permissions
	}
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}
	if overlay.MaxSteps != 0 {
		result.MaxSteps = overlay.MaxSteps
	}
	if overlay.MaxTurns != 0 {
		result.MaxTurns = overlay.MaxTurns
	}
	if overlay.Model != "" {
		result.Model = overlay.Model
	}
	if overlay.PermMode != "" {
		result.PermMode = overlay.PermMode
	}
	if overlay.SystemPrompt != "" {
		result.SystemPrompt = overlay.SystemPrompt
	}
	if len(overlay.Environment) > 0 {
		result.Environment = overlay.Environment
	}
	if len(overlay.AllowedMCP) > 0 {
		result.AllowedMCP = overlay.AllowedMCP
	}

	return result
}

// defaultBuiltinConfigs returns the hardcoded defaults for "coder" and "task".
func defaultBuiltinConfigs() map[string]AgentConfig {
	return map[string]AgentConfig{
		BuiltinCoder: {
			Name:        BuiltinCoder,
			AgentType:   BuiltinCoder,
			Description: "An agent that helps with executing coding tasks.",
			MaxTokens:   4096,
			MaxSteps:    25,
			PermMode:    "ask",
		},
		BuiltinTask: {
			Name:        BuiltinTask,
			AgentType:   BuiltinTask,
			Description: "An agent that helps with searching for context and finding implementation details.",
			MaxTokens:   4096,
			MaxSteps:    25,
			PermMode:    "ask",
		},
	}
}
