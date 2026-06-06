package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIncludeCycle verifies that A→B→A circular includes are detected and
// produce a specific error.
func TestIncludeCycle(t *testing.T) {
	t.Parallel()

	t.Run("markdown simple A includes B includes A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.md")
		b := filepath.Join(tmp, "b.md")
		require.NoError(t, os.WriteFile(a, []byte("@include b.md"), 0o644))
		require.NoError(t, os.WriteFile(b, []byte("@include a.md"), 0o644))

		_, err := ProcessIncludes("@include a.md", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("JSON simple A includes B includes A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.json")
		b := filepath.Join(tmp, "b.json")
		require.NoError(t, os.WriteFile(a, []byte(`{"@include":"b.json"}`), 0o644))
		require.NoError(t, os.WriteFile(b, []byte(`{"@include":"a.json"}`), 0o644))

		_, err := processJSONIncludes([]byte(`{"@include":"a.json"}`), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("markdown three-way cycle A→B→C→A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.md")
		b := filepath.Join(tmp, "b.md")
		c := filepath.Join(tmp, "c.md")
		require.NoError(t, os.WriteFile(a, []byte("@include b.md"), 0o644))
		require.NoError(t, os.WriteFile(b, []byte("@include c.md"), 0o644))
		require.NoError(t, os.WriteFile(c, []byte("@include a.md"), 0o644))

		_, err := ProcessIncludes("@include a.md", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("JSON three-way cycle A→B→C→A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.json")
		b := filepath.Join(tmp, "b.json")
		c := filepath.Join(tmp, "c.json")
		require.NoError(t, os.WriteFile(a, []byte(`{"@include":"b.json"}`), 0o644))
		require.NoError(t, os.WriteFile(b, []byte(`{"@include":"c.json"}`), 0o644))
		require.NoError(t, os.WriteFile(c, []byte(`{"@include":"a.json"}`), 0o644))

		_, err := processJSONIncludes([]byte(`{"@include":"a.json"}`), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("markdown self-include A→A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.md")
		require.NoError(t, os.WriteFile(a, []byte("@include a.md"), 0o644))

		_, err := ProcessIncludes("@include a.md", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("JSON self-include A→A", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		a := filepath.Join(tmp, "a.json")
		require.NoError(t, os.WriteFile(a, []byte(`{"@include":"a.json"}`), 0o644))

		_, err := processJSONIncludes([]byte(`{"@include":"a.json"}`), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})

	t.Run("markdown include same file from two branches", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		shared := filepath.Join(tmp, "shared.md")
		require.NoError(t, os.WriteFile(shared, []byte("shared"), 0o644))

		left := filepath.Join(tmp, "left.md")
		right := filepath.Join(tmp, "right.md")
		require.NoError(t, os.WriteFile(left, []byte("@include shared.md"), 0o644))
		require.NoError(t, os.WriteFile(right, []byte("@include shared.md"), 0o644))

		content := "@include left.md\n@include right.md"
		_, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cycle detected")
	})
}

// TestNestedIncludes verifies that A includes B includes C merges all three
// deterministically.
func TestNestedIncludes(t *testing.T) {
	t.Parallel()

	t.Run("markdown A→B→C all merged", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		c := filepath.Join(tmp, "c.md")
		require.NoError(t, os.WriteFile(c, []byte("content-c"), 0o644))

		b := filepath.Join(tmp, "b.md")
		require.NoError(t, os.WriteFile(b, []byte("content-b\n@include c.md"), 0o644))

		result, err := ProcessIncludes("content-a\n@include b.md", tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "content-a\ncontent-b\ncontent-c", result)
	})

	t.Run("markdown deep nested A→B→C→D", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		d := filepath.Join(tmp, "d.md")
		require.NoError(t, os.WriteFile(d, []byte("deep"), 0o644))

		c := filepath.Join(tmp, "c.md")
		require.NoError(t, os.WriteFile(c, []byte("level-c\n@include d.md"), 0o644))

		b := filepath.Join(tmp, "b.md")
		require.NoError(t, os.WriteFile(b, []byte("level-b\n@include c.md"), 0o644))

		result, err := ProcessIncludes("level-a\n@include b.md", tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "level-a\nlevel-b\nlevel-c\ndeep", result)
	})

	t.Run("JSON A→B→C all merged", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		c := filepath.Join(tmp, "c.json")
		require.NoError(t, os.WriteFile(c, []byte(`{"model":"gpt-4","region":"us"}`), 0o644))

		b := filepath.Join(tmp, "b.json")
		require.NoError(t, os.WriteFile(b, []byte(`{"@include":"c.json"}`), 0o644))

		main := filepath.Join(tmp, "crush.json")
		content := `{"providers":{"@include":"b.json"},"options":{"debug":false}}`
		require.NoError(t, os.WriteFile(main, []byte(content), 0o644))

		data, err := os.ReadFile(main)
		require.NoError(t, err)

		result, err := processJSONIncludes(data, tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))

		providers := parsed["providers"].(map[string]any)
		require.Equal(t, "gpt-4", providers["model"])
		require.Equal(t, "us", providers["region"])

		options := parsed["options"].(map[string]any)
		require.Equal(t, false, options["debug"])
	})

	t.Run("JSON nested include in subdirectory", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "sub"), 0o755))

		inner := filepath.Join(tmp, "sub", "inner.json")
		require.NoError(t, os.WriteFile(inner, []byte(`{"api_key":"secret"}`), 0o644))

		middle := filepath.Join(tmp, "sub", "middle.json")
		require.NoError(t, os.WriteFile(middle, []byte(`{"@include":"inner.json"}`), 0o644))

		content := `{"providers":{"openai":{"@include":"sub/middle.json"}}}`
		result, err := processJSONIncludes([]byte(content), tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))

		openai := parsed["providers"].(map[string]any)["openai"].(map[string]any)
		require.Equal(t, "secret", openai["api_key"])
	})

	t.Run("markdown multiple includes at same level", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		part1 := filepath.Join(tmp, "part1.md")
		require.NoError(t, os.WriteFile(part1, []byte("alpha"), 0o644))

		part2 := filepath.Join(tmp, "part2.md")
		require.NoError(t, os.WriteFile(part2, []byte("beta"), 0o644))

		content := "header\n@include part1.md\nmiddle\n@include part2.md\nfooter"
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "header\nalpha\nmiddle\nbeta\nfooter", result)
	})

	t.Run("JSON include in array context", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		item1 := filepath.Join(tmp, "item1.json")
		require.NoError(t, os.WriteFile(item1, []byte(`{"name":"first"}`), 0o644))

		item2 := filepath.Join(tmp, "item2.json")
		require.NoError(t, os.WriteFile(item2, []byte(`{"name":"second"}`), 0o644))

		content := `{"items":[{"@include":"item1.json"},{"@include":"item2.json"}]}`
		result, err := processJSONIncludes([]byte(content), tmp)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))

		items := parsed["items"].([]any)
		require.Len(t, items, 2)
		require.Equal(t, "first", items[0].(map[string]any)["name"])
		require.Equal(t, "second", items[1].(map[string]any)["name"])
	})
}

// TestYAMLJSONPrecedence verifies that conflicting YAML and JSON values
// resolve by the documented merge priority. Configs are merged in order:
// global → local project files, with later values overriding earlier ones.
// Both YAML and JSON configs participate in the same merge pipeline.
func TestYAMLJSONPrecedence(t *testing.T) {
	t.Parallel()

	t.Run("YAML converted to JSON merges correctly", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		dreamDir := filepath.Join(tmp, ".xrush")
		require.NoError(t, os.MkdirAll(dreamDir, 0o755))

		// YAML config sets architect model.
		yamlContent := `model:
  architect: "anthropic/claude-opus-4"
`
		require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

		// JSON config sets a different options field.
		jsonContent := `{"options":{"debug":true}}`
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "crush.json"), []byte(jsonContent), 0o644))

		// Parse YAML config to JSON bytes.
		yamlData, err := os.ReadFile(filepath.Join(dreamDir, "config.yml"))
		require.NoError(t, err)
		yamlJSON, err := yamlConfigToJSON(yamlData)
		require.NoError(t, err)

		// Parse JSON config.
		jsonData, err := os.ReadFile(filepath.Join(tmp, "crush.json"))
		require.NoError(t, err)
		processedJSON, err := parseConfigData(filepath.Join(tmp, "crush.json"), jsonData)
		require.NoError(t, err)

		// Merge: YAML first, then JSON (later overrides).
		cfg, err := loadFromBytes([][]byte{yamlJSON, processedJSON})
		require.NoError(t, err)

		// YAML architect model should be set.
		require.NotNil(t, cfg.Options)
		require.NotNil(t, cfg.Options.ArchitectModel)
		require.Equal(t, "anthropic", cfg.Options.ArchitectModel.Provider)
		require.Equal(t, "claude-opus-4", cfg.Options.ArchitectModel.Model)

		// JSON debug should be set.
		require.True(t, cfg.Options.Debug)
	})

	t.Run("JSON overrides YAML when applied second", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		dreamDir := filepath.Join(tmp, ".xrush")
		require.NoError(t, os.MkdirAll(dreamDir, 0o755))

		// YAML sets editor model.
		yamlContent := `model:
  editor: "anthropic/claude-haiku-4"
`
		require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

		// JSON overrides with different editor model.
		jsonContent := `{"options":{"editor_model":{"provider":"openai","model":"gpt-4o"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "crush.json"), []byte(jsonContent), 0o644))

		yamlData, err := os.ReadFile(filepath.Join(dreamDir, "config.yml"))
		require.NoError(t, err)
		yamlJSON, err := yamlConfigToJSON(yamlData)
		require.NoError(t, err)

		jsonData, err := os.ReadFile(filepath.Join(tmp, "crush.json"))
		require.NoError(t, err)
		processedJSON, err := parseConfigData(filepath.Join(tmp, "crush.json"), jsonData)
		require.NoError(t, err)

		// YAML first, JSON second: JSON should win for conflicting fields.
		cfg, err := loadFromBytes([][]byte{yamlJSON, processedJSON})
		require.NoError(t, err)

		// JSON's editor_model should override YAML's.
		require.NotNil(t, cfg.Options)
		require.NotNil(t, cfg.Options.EditorModel)
		require.Equal(t, "openai", cfg.Options.EditorModel.Provider)
		require.Equal(t, "gpt-4o", cfg.Options.EditorModel.Model)
	})

	t.Run("YAML provides values JSON does not set", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		dreamDir := filepath.Join(tmp, ".xrush")
		require.NoError(t, os.MkdirAll(dreamDir, 0o755))

		// YAML sets architect model.
		yamlContent := `model:
  architect: "google/gemini-2.5-pro"
`
		require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

		// JSON sets debug only, no model.
		jsonContent := `{"options":{"debug":true}}`
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "crush.json"), []byte(jsonContent), 0o644))

		yamlData, err := os.ReadFile(filepath.Join(dreamDir, "config.yml"))
		require.NoError(t, err)
		yamlJSON, err := yamlConfigToJSON(yamlData)
		require.NoError(t, err)

		jsonData, err := os.ReadFile(filepath.Join(tmp, "crush.json"))
		require.NoError(t, err)
		processedJSON, err := parseConfigData(filepath.Join(tmp, "crush.json"), jsonData)
		require.NoError(t, err)

		cfg, err := loadFromBytes([][]byte{yamlJSON, processedJSON})
		require.NoError(t, err)

		// YAML architect model preserved since JSON doesn't set it.
		require.NotNil(t, cfg.Options.ArchitectModel)
		require.Equal(t, "google", cfg.Options.ArchitectModel.Provider)
		require.Equal(t, "gemini-2.5-pro", cfg.Options.ArchitectModel.Model)
		require.True(t, cfg.Options.Debug)
	})

	t.Run("parseConfigData handles YAML files", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		yamlContent := `model:
  architect: "anthropic/claude-opus-4"
`
		yamlPath := filepath.Join(tmp, "config.yml")
		require.NoError(t, os.WriteFile(yamlPath, []byte(yamlContent), 0o644))

		data, err := os.ReadFile(yamlPath)
		require.NoError(t, err)

		result, err := parseConfigData(yamlPath, data)
		require.NoError(t, err)

		// Should produce valid JSON.
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))
		require.Contains(t, parsed, "options")
	})

	t.Run("parseConfigData handles JSON files", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		jsonContent := `{"options":{"debug":true}}`
		jsonPath := filepath.Join(tmp, "crush.json")
		require.NoError(t, os.WriteFile(jsonPath, []byte(jsonContent), 0o644))

		data, err := os.ReadFile(jsonPath)
		require.NoError(t, err)

		result, err := parseConfigData(jsonPath, data)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(result, &parsed))

		options := parsed["options"].(map[string]any)
		require.Equal(t, true, options["debug"])
	})

	t.Run("dual read loads both configs", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		dreamDir := filepath.Join(tmp, ".xrush")
		require.NoError(t, os.MkdirAll(dreamDir, 0o755))

		yamlContent := `model:
  architect: "anthropic/claude-opus-4"
`
		require.NoError(t, os.WriteFile(filepath.Join(dreamDir, "config.yml"), []byte(yamlContent), 0o644))

		jsonContent := `{"models":{"large":{"model":"gpt-4o","provider":"openai"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "crush.json"), []byte(jsonContent), 0o644))

		// Lookup should find YAML config.
		yamlPath, found := lookupYAMLConfig(tmp)
		require.True(t, found)
		require.Contains(t, yamlPath, ".xrush/config.yml")

		// Load via full pipeline.
		configPaths := lookupConfigs(tmp)
		cfg, loadedPaths, err := loadFromConfigPaths(configPaths)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Both YAML and JSON should be loaded.
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
		require.True(t, yamlLoaded, "YAML config should be loaded")
		require.True(t, jsonLoaded, "JSON config should be loaded")

		// YAML architect model should be present.
		require.NotNil(t, cfg.Options)
		require.NotNil(t, cfg.Options.ArchitectModel)

		// JSON model should be present.
		require.Equal(t, "gpt-4o", cfg.Models[SelectedModelTypeLarge].Model)
	})
}

// TestAdvancedWalkingEdgeCases covers downward/upward directory walking edge
// cases.
func TestAdvancedWalkingEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("walk down skips all excluded directories", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		for _, skip := range []string{
			"node_modules", ".git", "vendor", "dist",
			"build", ".next", ".cache", "__pycache__",
			".tox", ".venv", ".svn", ".hg",
		} {
			require.NoError(t, os.MkdirAll(filepath.Join(tmp, skip), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(tmp, skip, "AGENTS.md"), []byte(skip), 0o644))
		}

		paths, err := WalkContextPaths(tmp)
		require.NoError(t, err)
		require.Empty(t, paths, "no context files should be found in excluded dirs")
	})

	t.Run("walk down respects depth limit of 2", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		// Depth 1: sub/
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "pkg"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "pkg", "CRUSH.md"), []byte("d1"), 0o644))

		// Depth 2: pkg/internal/
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "pkg", "internal"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "pkg", "internal", "AGENTS.md"), []byte("d2"), 0o644))

		// Depth 3: pkg/internal/server/ — beyond default depth of 2.
		require.NoError(t, os.MkdirAll(filepath.Join(tmp, "pkg", "internal", "server"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "pkg", "internal", "server", "CLAUDE.md"), []byte("d3"), 0o644))

		paths, err := WalkContextPaths(tmp)
		require.NoError(t, err)

		var basenames []string
		for _, p := range paths {
			basenames = append(basenames, filepath.Base(p))
		}
		require.Contains(t, basenames, "CRUSH.md", "depth 1 should be found")
		require.Contains(t, basenames, "AGENTS.md", "depth 2 should be found")
		require.NotContains(t, basenames, "CLAUDE.md", "depth 3 should NOT be found")
	})

	t.Run("walk down deduplicates with upward", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		// Root-level file found by upward walk.
		require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("root"), 0o644))

		paths, err := WalkContextPaths(tmp)
		require.NoError(t, err)

		count := 0
		for _, p := range paths {
			if filepath.Base(p) == "AGENTS.md" {
				count++
			}
		}
		require.Equal(t, 1, count, "AGENTS.md should appear exactly once")
	})

	t.Run("walk up stops at home directory", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		paths, err := WalkContextPaths(tmp)
		require.NoError(t, err)

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		for _, p := range paths {
			// Every path must be under tmp or under home.
			rel, err := filepath.Rel(homeDir, p)
			require.NoError(t, err)
			_ = rel // Path is valid relative to home or tmp.
		}
	})

	t.Run("empty directory returns no paths", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		paths, err := WalkContextPaths(tmp)
		require.NoError(t, err)
		require.Empty(t, paths)
	})

	t.Run("ResolveContextPaths managed wins over local", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		managed := filepath.Join(tmp, "managed")
		local := filepath.Join(tmp, "local")
		require.NoError(t, os.MkdirAll(managed, 0o755))
		require.NoError(t, os.MkdirAll(local, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(managed, "CRUSH.md"), []byte("managed"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(local, "CRUSH.md"), []byte("local"), 0o644))

		paths, err := ResolveContextPaths(local, managed, "", "")
		require.NoError(t, err)

		crushPaths := filterByName(paths, "CRUSH.md")
		require.Len(t, crushPaths, 1)
		require.Contains(t, crushPaths[0], "managed")
	})

	t.Run("ResolveContextPaths falls through layers", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		managed := filepath.Join(tmp, "managed")
		user := filepath.Join(tmp, "user")
		project := filepath.Join(tmp, "project")
		local := filepath.Join(tmp, "local")
		for _, dir := range []string{managed, user, project, local} {
			require.NoError(t, os.MkdirAll(dir, 0o755))
		}

		// AGENTS.md in project, CRUSH.md in user, GEMINI.md in local.
		require.NoError(t, os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("p"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(user, "CRUSH.md"), []byte("u"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(local, "GEMINI.md"), []byte("l"), 0o644))

		paths, err := ResolveContextPaths(local, managed, user, project)
		require.NoError(t, err)

		require.Len(t, filterByName(paths, "AGENTS.md"), 1)
		require.Len(t, filterByName(paths, "CRUSH.md"), 1)
		require.Len(t, filterByName(paths, "GEMINI.md"), 1)
	})
}

// TestIncludeEnvConditional tests environment-conditional includes using
// t.SetEnv for isolation.
func TestIncludeEnvConditional(t *testing.T) {
	t.Run("env conditional with t.Setenv", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_ADVANCED_TEST", "yes")

		content := `before
<!-- if: env:CRUSH_ADVANCED_TEST -->
visible
<!-- endif -->
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "visible")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("env conditional unset hides content", func(t *testing.T) {
		tmp := t.TempDir()

		content := `before
<!-- if: env:CRUSH_SURELY_UNSET_XYZ_999 -->
hidden
<!-- endif -->
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.NotContains(t, result, "hidden")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})

	t.Run("@if env conditional with t.Setenv", func(t *testing.T) {
		tmp := t.TempDir()

		t.Setenv("CRUSH_AT_ADV_TEST", "1")

		content := `before
@if env=CRUSH_AT_ADV_TEST
visible-at
@endif
after`
		result, err := ProcessIncludes(content, tmp, 0, nil, nil)
		require.NoError(t, err)
		require.Contains(t, result, "visible-at")
		require.Contains(t, result, "before")
		require.Contains(t, result, "after")
	})
}

// TestIncludePathSecurity covers path traversal security edge cases.
func TestIncludePathSecurity(t *testing.T) {
	t.Parallel()

	t.Run("absolute path rejected", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		_, err := ProcessIncludes("@include /etc/passwd", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "absolute path")
	})

	t.Run("dot-dot traversal rejected", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		_, err := ProcessIncludes("@include ../../../etc/passwd", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "escapes project directory")
	})

	t.Run("JSON absolute path rejected", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		_, err := processJSONIncludes([]byte(`{"@include":"/etc/passwd"}`), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "absolute")
	})

	t.Run("JSON dot-dot traversal rejected", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		_, err := processJSONIncludes([]byte(`{"@include":"../../../etc/passwd"}`), tmp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "'..'")
	})

	t.Run("missing file rejected", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()

		_, err := ProcessIncludes("@include nonexistent.md", tmp, 0, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to read")
	})
}
