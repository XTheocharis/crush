package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYAMLRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dreamDir := filepath.Join(dir, ".xrush")
	require.NoError(t, os.MkdirAll(dreamDir, 0o755))

	yamlContent := `model:
  architect: "anthropic/claude-opus-4"
  editor: "anthropic/claude-haiku-4"
  router:
    tiers:
      - up_to: 10000
        model: "openai/gpt-4o-mini"
      - up_to: 30000
        model: "anthropic/claude-haiku-4"
context:
  autoCompact:
    buffer_tokens: 13000
    output_reservation: 20000
    post_compact:
      max_files: 5
      token_budget: 50000
observation:
  observer:
    message_tokens: 30000
    buffer_ratio: 0.2
    model: "google/gemini-2.5-flash"
  reflector:
    observation_tokens: 40000
    buffer_activation: 0.5
    model: "google/gemini-2.5-flash"
dcp:
  compress:
    maxContextLimit: 100000
    minContextLimit: 50000
    nudgeFrequency: 5
  strategies:
    deduplication: true
    purgeErrors: true
quality:
  lint_on_write: true
  auto_commit: true
  max_retries: 3
lsp:
  mode: auto
  startup_timeout: 30s
  languages: []
`
	require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

	cfg, err := loadYAMLConfig(filepath.Join(dreamDir, "config.yml"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify model mapping.
	require.NotNil(t, cfg.Options, "Options should be set when model fields are present")
	require.NotNil(t, cfg.Options.ArchitectModel, "ArchitectModel should be set")
	require.Equal(t, "anthropic", cfg.Options.ArchitectModel.Provider)
	require.Equal(t, "claude-opus-4", cfg.Options.ArchitectModel.Model)

	require.NotNil(t, cfg.Options.EditorModel, "EditorModel should be set")
	require.Equal(t, "anthropic", cfg.Options.EditorModel.Provider)
	require.Equal(t, "claude-haiku-4", cfg.Options.EditorModel.Model)
}

func TestYAMLReadMinimal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dreamDir := filepath.Join(dir, ".xrush")
	require.NoError(t, os.MkdirAll(dreamDir, 0o755))

	yamlContent := `model:
  architect: "openai/gpt-4o"
`
	require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

	cfg, err := loadYAMLConfig(filepath.Join(dreamDir, "config.yml"))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Options)
	require.NotNil(t, cfg.Options.ArchitectModel)
	require.Equal(t, "openai", cfg.Options.ArchitectModel.Provider)
	require.Equal(t, "gpt-4o", cfg.Options.ArchitectModel.Model)
	require.Nil(t, cfg.Options.EditorModel, "EditorModel should be nil when not specified")
}

func TestYAMLReadEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dreamDir := filepath.Join(dir, ".xrush")
	require.NoError(t, os.MkdirAll(dreamDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(""), 0o644))

	cfg, err := loadYAMLConfig(filepath.Join(dreamDir, "config.yml"))
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestYAMLReadNonexistent(t *testing.T) {
	t.Parallel()

	_, err := loadYAMLConfig("/nonexistent/path/config.yml")
	require.Error(t, err)
}

func TestYAMLToConfigMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantArch *SelectedModel
		wantEdit *SelectedModel
	}{
		{
			name:     "empty config",
			yaml:     "",
			wantArch: nil,
			wantEdit: nil,
		},
		{
			name: "architect only",
			yaml: `model:
  architect: "openai/gpt-4o"
`,
			wantArch: &SelectedModel{Provider: "openai", Model: "gpt-4o"},
			wantEdit: nil,
		},
		{
			name: "editor only",
			yaml: `model:
  editor: "anthropic/claude-haiku-4"
`,
			wantArch: nil,
			wantEdit: &SelectedModel{Provider: "anthropic", Model: "claude-haiku-4"},
		},
		{
			name: "both models",
			yaml: `model:
  architect: "google/gemini-2.5-pro"
  editor: "google/gemini-2.5-flash"
`,
			wantArch: &SelectedModel{Provider: "google", Model: "gemini-2.5-pro"},
			wantEdit: &SelectedModel{Provider: "google", Model: "gemini-2.5-flash"},
		},
		{
			name: "model without provider slash",
			yaml: `model:
  architect: "my-model"
`,
			wantArch: &SelectedModel{Provider: "", Model: "my-model"},
			wantEdit: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dreamDir := filepath.Join(dir, ".xrush")
			require.NoError(t, os.MkdirAll(dreamDir, 0o755))

			path := filepath.Join(dreamDir, "config.yml")
			require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o644))

			cfg, err := loadYAMLConfig(path)
			require.NoError(t, err)

			if tt.wantArch == nil {
				if cfg.Options != nil {
					require.Nil(t, cfg.Options.ArchitectModel)
				}
			} else {
				require.NotNil(t, cfg.Options, "Options should not be nil when architect is expected")
				require.NotNil(t, cfg.Options.ArchitectModel, "ArchitectModel should not be nil")
				require.Equal(t, tt.wantArch.Provider, cfg.Options.ArchitectModel.Provider)
				require.Equal(t, tt.wantArch.Model, cfg.Options.ArchitectModel.Model)
			}

			if tt.wantEdit == nil {
				if cfg.Options != nil {
					require.Nil(t, cfg.Options.EditorModel)
				}
			} else {
				require.NotNil(t, cfg.Options, "Options should not be nil when editor is expected")
				require.NotNil(t, cfg.Options.EditorModel, "EditorModel should not be nil")
				require.Equal(t, tt.wantEdit.Provider, cfg.Options.EditorModel.Provider)
				require.Equal(t, tt.wantEdit.Model, cfg.Options.EditorModel.Model)
			}
		})
	}
}

func TestJSONFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create crush.json without .xrush/config.yml.
	jsonContent := `{"models":{"large":{"model":"gpt-4o","provider":"openai"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crush.json"), []byte(jsonContent), 0o644))

	// Verify no YAML config found.
	yamlPath, found := lookupYAMLConfig(dir)
	require.False(t, found, "Should not find YAML config")
	require.Empty(t, yamlPath)

	// Load using existing JSON path.
	configPaths := lookupConfigs(dir)
	cfg, loadedPaths, err := loadFromConfigPaths(configPaths)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify JSON config loaded correctly.
	require.Equal(t, "gpt-4o", cfg.Models[SelectedModelTypeLarge].Model)
	require.Equal(t, "openai", cfg.Models[SelectedModelTypeLarge].Provider)

	// Verify the JSON path was loaded.
	jsonLoaded := false
	for _, p := range loadedPaths {
		if filepath.Base(p) == "crush.json" {
			jsonLoaded = true
		}
	}
	require.True(t, jsonLoaded, "crush.json should be in loaded paths")
}

func TestDualRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dreamDir := filepath.Join(dir, ".xrush")
	require.NoError(t, os.MkdirAll(dreamDir, 0o755))

	// Create YAML config.
	yamlContent := `model:
  architect: "anthropic/claude-opus-4"
  editor: "anthropic/claude-haiku-4"
`
	require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

	// Also create JSON config — YAML should take precedence.
	jsonContent := `{"models":{"large":{"model":"gpt-4o","provider":"openai"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crush.json"), []byte(jsonContent), 0o644))

	// Verify YAML config is found.
	yamlPath, found := lookupYAMLConfig(dir)
	require.True(t, found, "Should find YAML config")
	require.Contains(t, yamlPath, ".xrush/config.yml")

	// Load configs — both YAML and JSON should be in the path list.
	configPaths := lookupConfigs(dir)
	cfg, loadedPaths, err := loadFromConfigPaths(configPaths)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify YAML fields mapped correctly.
	require.NotNil(t, cfg.Options, "Options should be set from YAML config")
	require.NotNil(t, cfg.Options.ArchitectModel, "ArchitectModel from YAML should be set")
	require.Equal(t, "anthropic", cfg.Options.ArchitectModel.Provider)
	require.Equal(t, "claude-opus-4", cfg.Options.ArchitectModel.Model)

	// Verify JSON fields also loaded (merge behavior).
	require.Equal(t, "gpt-4o", cfg.Models[SelectedModelTypeLarge].Model)

	// Both paths should be in loaded list.
	yamlLoaded := false
	jsonLoaded := false
	for _, p := range loadedPaths {
		if filepath.Base(p) == "config.yml" {
			yamlLoaded = true
		}
		if filepath.Base(p) == "crush.json" {
			jsonLoaded = true
		}
	}
	require.True(t, yamlLoaded, "YAML config should be in loaded paths")
	require.True(t, jsonLoaded, "JSON config should be in loaded paths")
}

func TestParseModelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input        string
		wantProvider string
		wantModel    string
	}{
		{"anthropic/claude-opus-4", "anthropic", "claude-opus-4"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"google/gemini-2.5-flash", "google", "gemini-2.5-flash"},
		{"my-model", "", "my-model"},
		{"", "", ""},
		{"provider/", "provider", ""},
		{"/model", "", "model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			provider, model := parseModelString(tt.input)
			require.Equal(t, tt.wantProvider, provider)
			require.Equal(t, tt.wantModel, model)
		})
	}
}

func TestIsYAMLFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{".xrush/config.yml", true},
		{".xrush/config.yaml", true},
		{"crush.json", false},
		{"config.YML", true},
		{"config.YAML", true},
		{"crush.yml.bak", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			got := isYAMLFile(tt.path)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestXrushRouterConfig_ToConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		yaml            string
		expectedTiers   int
		expectedUpTo    int
		expectNoOptions bool
	}{
		{
			name: "router with tiers populates RouterTiers",
			yaml: `model:
  router:
    tiers:
      - up_to: 10000
        model: "openai/gpt-4o-mini"
      - up_to: 30000
        model: "anthropic/claude-haiku-4"
`,
			expectedTiers: 2,
			expectedUpTo:  10000,
		},
		{
			name: "router with single tier",
			yaml: `model:
  router:
    tiers:
      - up_to: 8000
        model: "google/gemini-2.5-flash"
`,
			expectedTiers: 1,
			expectedUpTo:  8000,
		},
		{
			name: "no router sets no options",
			yaml: `model:
  architect: "openai/gpt-4o"
`,
			expectNoOptions: true,
		},
		{
			name: "router with empty tiers",
			yaml: `model:
  router:
    tiers: []
`,
			expectNoOptions: true,
		},
		{
			name:            "empty config sets no options",
			yaml:            "",
			expectNoOptions: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dreamDir := filepath.Join(dir, ".xrush")
			require.NoError(t, os.MkdirAll(dreamDir, 0o755))

			path := filepath.Join(dreamDir, "config.yml")
			require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o644))

			cfg, err := loadYAMLConfig(path)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.expectNoOptions {
				return
			}

			require.NotNil(t, cfg.Options, "Options should be set when router is configured")
			require.Equal(t, 0, cfg.Options.RouterTokenLimit, "RouterTokenLimit should NOT be set from yaml tiers")
			require.Len(t, cfg.Options.RouterTiers, tt.expectedTiers)
			require.Equal(t, tt.expectedUpTo, cfg.Options.RouterTiers[0].UpToTokens)
		})
	}
}

func TestYAMLConfigConvertsToJSONForMergePipeline(t *testing.T) {
	t.Parallel()

	// Verify that a YAML config can be converted to JSON bytes that produce
	// the same Config when parsed by the standard JSON pipeline.
	dir := t.TempDir()
	dreamDir := filepath.Join(dir, ".xrush")
	require.NoError(t, os.MkdirAll(dreamDir, 0o755))

	yamlContent := `model:
  architect: "anthropic/claude-opus-4"
  editor: "anthropic/claude-haiku-4"
`
	require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

	cfg, err := loadYAMLConfig(filepath.Join(dreamDir, "config.yml"))
	require.NoError(t, err)

	// Verify the Config can round-trip through JSON serialization.
	jsonBytes, err := json.Marshal(cfg)
	require.NoError(t, err)

	var cfg2 Config
	require.NoError(t, json.Unmarshal(jsonBytes, &cfg2))

	require.NotNil(t, cfg2.Options)
	require.NotNil(t, cfg2.Options.ArchitectModel)
	require.Equal(t, "anthropic", cfg2.Options.ArchitectModel.Provider)
	require.Equal(t, "claude-opus-4", cfg2.Options.ArchitectModel.Model)
}
