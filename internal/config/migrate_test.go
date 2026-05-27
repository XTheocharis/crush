package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYAMLMigration(t *testing.T) {
	t.Parallel()

	t.Run("migrates architect and editor models", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, ".xrush", "config.yml")

		jsonContent := `{
			"options": {
				"architect_model": {"provider": "anthropic", "model": "claude-opus-4"},
				"editor_model": {"provider": "openai", "model": "gpt-4o-mini"}
			}
		}`
		require.NoError(t, os.WriteFile(jsonPath, []byte(jsonContent), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.NoError(t, err)

		dc, err := ReadYAMLConfig(yamlPath)
		require.NoError(t, err)
		require.NotNil(t, dc.Model)
		require.Equal(t, "anthropic/claude-opus-4", dc.Model.Architect)
		require.Equal(t, "openai/gpt-4o-mini", dc.Model.Editor)
	})

	t.Run("migrates LSP languages", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, ".xrush", "config.yml")

		jsonContent := `{
			"lsp": {
				"go": {"command": "gopls"},
				"typescript": {"command": "typescript-language-server"},
				"python": {"command": "pyright"}
			}
		}`
		require.NoError(t, os.WriteFile(jsonPath, []byte(jsonContent), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.NoError(t, err)

		dc, err := ReadYAMLConfig(yamlPath)
		require.NoError(t, err)
		require.NotNil(t, dc.LSP)
		require.Equal(t, "auto", dc.LSP.Mode)
		require.Equal(t, []string{"go", "python", "typescript"}, dc.LSP.Languages)
	})

	t.Run("migrates LCM options to context section", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, ".xrush", "config.yml")

		jsonContent := `{
			"options": {
				"lcm": {
					"ctx_cutoff_threshold": 0.75,
					"large_tool_output_token_threshold": 15000
				}
			}
		}`
		require.NoError(t, os.WriteFile(jsonPath, []byte(jsonContent), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.NoError(t, err)

		dc, err := ReadYAMLConfig(yamlPath)
		require.NoError(t, err)
		require.NotNil(t, dc.Context)
		require.NotNil(t, dc.Context.AutoCompact)
		require.Equal(t, 15000, dc.Context.AutoCompact.BufferTokens)
	})

	t.Run("handles empty config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, ".xrush", "config.yml")

		require.NoError(t, os.WriteFile(jsonPath, []byte("{}"), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.NoError(t, err)

		dc, err := ReadYAMLConfig(yamlPath)
		require.NoError(t, err)
		require.Nil(t, dc.Model)
		require.Nil(t, dc.Context)
		require.Nil(t, dc.Observation)
		require.Nil(t, dc.DCP)
		require.Nil(t, dc.Quality)
		require.Nil(t, dc.LSP)
	})

	t.Run("fails on missing JSON file", func(t *testing.T) {
		t.Parallel()

		err := MigrateJSONToYAML("/nonexistent/crush.json", "/tmp/config.yml")
		require.Error(t, err)
	})

	t.Run("fails on invalid JSON", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, ".xrush", "config.yml")

		require.NoError(t, os.WriteFile(jsonPath, []byte("not json"), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.Error(t, err)
	})

	t.Run("creates parent directories", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		jsonPath := filepath.Join(dir, "crush.json")
		yamlPath := filepath.Join(dir, "deep", "nested", ".xrush", "config.yml")

		require.NoError(t, os.WriteFile(jsonPath, []byte("{}"), 0o644))

		err := MigrateJSONToYAML(jsonPath, yamlPath)
		require.NoError(t, err)

		_, err = os.Stat(yamlPath)
		require.NoError(t, err)
	})
}

func TestYAMLMigrationBytes(t *testing.T) {
	t.Parallel()

	t.Run("converts JSON bytes to YAML bytes", func(t *testing.T) {
		t.Parallel()

		jsonData := []byte(`{
			"options": {
				"architect_model": {"provider": "anthropic", "model": "claude-opus-4"}
			}
		}`)

		yamlData, err := MigrateJSONBytesToYAML(jsonData)
		require.NoError(t, err)
		require.NotEmpty(t, yamlData)

		var dc xrushConfig
		require.NoError(t, yaml.Unmarshal(yamlData, &dc))
		require.NotNil(t, dc.Model)
		require.Equal(t, "anthropic/claude-opus-4", dc.Model.Architect)
	})
}

func TestYAMLRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("preserves all spec sections", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		dreamDir := filepath.Join(dir, ".xrush")
		require.NoError(t, os.MkdirAll(dreamDir, 0o755))

		original := &xrushConfig{
			Model: &xrushModelConfig{
				Architect: "anthropic/claude-opus-4",
				Editor:    "anthropic/claude-haiku-4",
				Router: &xrushRouterConfig{
					Tiers: []xrushRouterTier{
						{UpTo: 10000, Model: "openai/gpt-4o-mini"},
						{UpTo: 30000, Model: "anthropic/claude-haiku-4"},
					},
				},
			},
			Context: &xrushContextConfig{
				AutoCompact: &xrushAutoCompactConfig{
					BufferTokens:      13000,
					OutputReservation: 20000,
					PostCompact: &xrushPostCompactConfig{
						MaxFiles:    5,
						TokenBudget: 50000,
					},
				},
			},
			Observation: &xrushObservationConfig{
				Observer: &xrushObserverConfig{
					MessageTokens: 30000,
					BufferRatio:   0.2,
					Model:         "google/gemini-2.5-flash",
				},
				Reflector: &xrushReflectorConfig{
					ObservationTokens: 40000,
					BufferActivation:  0.5,
					Model:             "google/gemini-2.5-flash",
				},
			},
			DCP: &xrushDCPConfig{
				Compress: &xrushDCPCompressConfig{
					MaxContextLimit: 100000,
					MinContextLimit: 50000,
					NudgeFrequency:  5,
				},
				Strategies: &xrushDCPStrategiesConfig{
					Deduplication: true,
					PurgeErrors:   true,
				},
			},
			Quality: &xrushQualityConfig{
				LintOnWrite: true,
				AutoCommit:  true,
				MaxRetries:  3,
			},
			LSP: &xrushLSPConfig{
				Mode:           "auto",
				StartupTimeout: "30s",
				Languages:      []string{"go", "typescript", "python"},
			},
		}

		path := filepath.Join(dreamDir, "config.yml")
		require.NoError(t, WriteYAMLConfig(path, original))

		loaded, err := ReadYAMLConfig(path)
		require.NoError(t, err)

		require.NotNil(t, loaded.Model)
		require.Equal(t, "anthropic/claude-opus-4", loaded.Model.Architect)
		require.Equal(t, "anthropic/claude-haiku-4", loaded.Model.Editor)
		require.NotNil(t, loaded.Model.Router)
		require.Len(t, loaded.Model.Router.Tiers, 2)
		require.Equal(t, 10000, loaded.Model.Router.Tiers[0].UpTo)
		require.Equal(t, "openai/gpt-4o-mini", loaded.Model.Router.Tiers[0].Model)
		require.Equal(t, 30000, loaded.Model.Router.Tiers[1].UpTo)
		require.Equal(t, "anthropic/claude-haiku-4", loaded.Model.Router.Tiers[1].Model)

		require.NotNil(t, loaded.Context)
		require.NotNil(t, loaded.Context.AutoCompact)
		require.Equal(t, 13000, loaded.Context.AutoCompact.BufferTokens)
		require.Equal(t, 20000, loaded.Context.AutoCompact.OutputReservation)
		require.NotNil(t, loaded.Context.AutoCompact.PostCompact)
		require.Equal(t, 5, loaded.Context.AutoCompact.PostCompact.MaxFiles)
		require.Equal(t, 50000, loaded.Context.AutoCompact.PostCompact.TokenBudget)

		require.NotNil(t, loaded.Observation)
		require.NotNil(t, loaded.Observation.Observer)
		require.Equal(t, 30000, loaded.Observation.Observer.MessageTokens)
		require.Equal(t, 0.2, loaded.Observation.Observer.BufferRatio)
		require.Equal(t, "google/gemini-2.5-flash", loaded.Observation.Observer.Model)
		require.NotNil(t, loaded.Observation.Reflector)
		require.Equal(t, 40000, loaded.Observation.Reflector.ObservationTokens)
		require.Equal(t, 0.5, loaded.Observation.Reflector.BufferActivation)
		require.Equal(t, "google/gemini-2.5-flash", loaded.Observation.Reflector.Model)

		require.NotNil(t, loaded.DCP)
		require.NotNil(t, loaded.DCP.Compress)
		require.Equal(t, 100000, loaded.DCP.Compress.MaxContextLimit)
		require.Equal(t, 50000, loaded.DCP.Compress.MinContextLimit)
		require.Equal(t, 5, loaded.DCP.Compress.NudgeFrequency)
		require.NotNil(t, loaded.DCP.Strategies)
		require.True(t, loaded.DCP.Strategies.Deduplication)
		require.True(t, loaded.DCP.Strategies.PurgeErrors)

		require.NotNil(t, loaded.Quality)
		require.True(t, loaded.Quality.LintOnWrite)
		require.True(t, loaded.Quality.AutoCommit)
		require.Equal(t, 3, loaded.Quality.MaxRetries)

		require.NotNil(t, loaded.LSP)
		require.Equal(t, "auto", loaded.LSP.Mode)
		require.Equal(t, "30s", loaded.LSP.StartupTimeout)
		require.Equal(t, []string{"go", "typescript", "python"}, loaded.LSP.Languages)
	})

	t.Run("preserves minimal config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "config.yml")

		original := &xrushConfig{
			Model: &xrushModelConfig{
				Architect: "openai/gpt-4o",
			},
		}
		require.NoError(t, WriteYAMLConfig(path, original))

		loaded, err := ReadYAMLConfig(path)
		require.NoError(t, err)
		require.NotNil(t, loaded.Model)
		require.Equal(t, "openai/gpt-4o", loaded.Model.Architect)
		require.Empty(t, loaded.Model.Editor)
		require.Nil(t, loaded.Context)
		require.Nil(t, loaded.Observation)
		require.Nil(t, loaded.DCP)
		require.Nil(t, loaded.Quality)
		require.Nil(t, loaded.LSP)
	})

	t.Run("round-trips through fromConfig and toConfig", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Options: &Options{
				ArchitectModel: &SelectedModel{Provider: "anthropic", Model: "claude-opus-4"},
				EditorModel:    &SelectedModel{Provider: "openai", Model: "gpt-4o-mini"},
			},
		}

		dc := fromConfig(cfg)
		require.NotNil(t, dc.Model)
		require.Equal(t, "anthropic/claude-opus-4", dc.Model.Architect)
		require.Equal(t, "openai/gpt-4o-mini", dc.Model.Editor)

		rtCfg := dc.toConfig()
		require.NotNil(t, rtCfg.Options)
		require.Equal(t, "anthropic", rtCfg.Options.ArchitectModel.Provider)
		require.Equal(t, "claude-opus-4", rtCfg.Options.ArchitectModel.Model)
		require.Equal(t, "openai", rtCfg.Options.EditorModel.Provider)
		require.Equal(t, "gpt-4o-mini", rtCfg.Options.EditorModel.Model)
	})
}
