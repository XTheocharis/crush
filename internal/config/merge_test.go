package config

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/csync"
	"github.com/stretchr/testify/require"
)

// TestConfigMerging defines the rules on how configuration merging works.
// Generally, things are either appended to or replaced by the later configuration.
// Whether one or the other happen depends on effects its effects.
func TestConfigMerging(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		c := exerciseMerge(t, Config{}, Config{})
		require.NotNil(t, c)
	})

	t.Run("mcps", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"foo": {
					Command: "foo-mcp",
					Args:    []string{"serve"},
					Type:    MCPSSE,
					Timeout: 10,
				},
				"zaz": {
					Disabled: true,
					Env:      map[string]string{"FOO": "bar"},
					Headers:  map[string]string{"api-key": "exposed"},
					URL:      "nope",
				},
			},
		}, Config{
			MCP: MCPs{
				"foo": {
					Args:    []string{"serve", "--stdio"},
					Type:    MCPStdio,
					Timeout: 7,
				},
				"bar": {
					Command: "bar",
				},
				"zaz": {
					Env:     map[string]string{"FOO": "foo", "BAR": "bar"},
					Headers: map[string]string{"api-key": "$API"},
					URL:     "http://bar",
				},
			},
		})
		require.NotNil(t, c)
		require.Len(t, slices.Collect(maps.Keys(c.MCP)), 3)

		// foo: merged from both configs
		foo := c.MCP["foo"]
		require.Equal(t, "foo-mcp", foo.Command)
		require.Equal(t, []string{"serve", "--stdio"}, foo.Args)
		require.Equal(t, MCPStdio, foo.Type)
		require.Equal(t, 10, foo.Timeout) // max of 10 and 7

		// bar: only in second config
		require.Equal(t, "bar", c.MCP["bar"].Command)

		// zaz: merged, env/headers merged, disabled stays true
		zaz := c.MCP["zaz"]
		require.True(t, zaz.Disabled)
		require.Equal(t, "http://bar", zaz.URL)
		require.Equal(t, "foo", zaz.Env["FOO"]) // overwritten
		require.Equal(t, "bar", zaz.Env["BAR"]) // added
		require.Equal(t, "$API", zaz.Headers["api-key"])
	})

	t.Run("lsps", func(t *testing.T) {
		result := exerciseMerge(t, Config{
			LSP: LSPs{
				"gopls": LSPConfig{
					Env:         map[string]string{"FOO": "bar"},
					RootMarkers: []string{"go.sum"},
					FileTypes:   []string{"go"},
				},
			},
		}, Config{
			LSP: LSPs{
				"gopls": LSPConfig{
					Command:     "gopls",
					InitOptions: map[string]any{"a": 10},
					RootMarkers: []string{"go.sum"},
				},
			},
		}, Config{
			LSP: LSPs{
				"gopls": LSPConfig{
					Args:        []string{"serve", "--stdio"},
					InitOptions: map[string]any{"a": 12, "b": 18},
					RootMarkers: []string{"go.sum", "go.mod"},
					FileTypes:   []string{"go"},
					Disabled:    true,
				},
			},
		},
			Config{
				LSP: LSPs{
					"gopls": LSPConfig{
						Options:     map[string]any{"opt1": "10"},
						RootMarkers: []string{"go.work"},
					},
				},
			},
		)
		require.NotNil(t, result)
		require.Equal(t, LSPConfig{
			Disabled:    true,
			Command:     "gopls",
			Args:        []string{"serve", "--stdio"},
			Env:         map[string]string{"FOO": "bar"},
			FileTypes:   []string{"go"},
			RootMarkers: []string{"go.mod", "go.sum", "go.work"},
			InitOptions: map[string]any{"a": 12.0, "b": 18.0},
			Options:     map[string]any{"opt1": "10"},
		}, result.LSP["gopls"])
	})

	t.Run("tui_options", func(t *testing.T) {
		maxDepth := 5
		maxItems := 100
		newMaxDepth := 10
		newMaxItems := 200

		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: false,
					DiffMode:    "unified",
					Completions: Completions{
						MaxDepth: &maxDepth,
						MaxItems: &maxItems,
					},
				},
			},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: true,
					DiffMode:    "split",
					Completions: Completions{
						MaxDepth: &newMaxDepth,
						MaxItems: &newMaxItems,
					},
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Options.TUI.CompactMode)
		require.Equal(t, "split", c.Options.TUI.DiffMode)
		require.Equal(t, newMaxDepth, *c.Options.TUI.Completions.MaxDepth)
	})

	t.Run("options", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				ContextPaths:              []string{"CRUSH.md"},
				Debug:                     false,
				DebugLSP:                  false,
				DisableProviderAutoUpdate: false,
				DisableMetrics:            false,
				DataDirectory:             ".crush",
				DisabledTools:             []string{"bash"},
				Attribution: &Attribution{
					TrailerStyle:  TrailerStyleNone,
					GeneratedWith: false,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				ContextPaths:              []string{".cursorrules"},
				Debug:                     true,
				DebugLSP:                  true,
				DisableProviderAutoUpdate: true,
				DisableMetrics:            true,
				DataDirectory:             ".custom",
				DisabledTools:             []string{"edit"},
				Attribution: &Attribution{
					TrailerStyle:  TrailerStyleCoAuthoredBy,
					GeneratedWith: true,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"CRUSH.md", ".cursorrules"}, c.Options.ContextPaths)
		require.True(t, c.Options.Debug)
		require.True(t, c.Options.DebugLSP)
		require.True(t, c.Options.DisableProviderAutoUpdate)
		require.True(t, c.Options.DisableMetrics)
		require.Equal(t, ".custom", c.Options.DataDirectory)
		require.Equal(t, []string{"bash", "edit"}, c.Options.DisabledTools)
		require.Equal(t, TrailerStyleCoAuthoredBy, c.Options.Attribution.TrailerStyle)
		require.True(t, c.Options.Attribution.GeneratedWith)
	})

	t.Run("tools", func(t *testing.T) {
		maxDepth := 5
		maxItems := 100
		newMaxDepth := 10
		newMaxItems := 200

		c := exerciseMerge(t, Config{
			Tools: Tools{
				Ls: ToolLs{
					MaxDepth: &maxDepth,
					MaxItems: &maxItems,
				},
			},
		}, Config{
			Tools: Tools{
				Ls: ToolLs{
					MaxDepth: &newMaxDepth,
					MaxItems: &newMaxItems,
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, newMaxDepth, *c.Tools.Ls.MaxDepth)
	})

	t.Run("repo_map_options", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{
					Disabled:      false,
					MaxTokens:     2048,
					ExcludeGlobs:  []string{"*.log"},
					RefreshMode:   "auto",
					MapMulNoFiles: 2.0,
				},
			},
		}, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{
					Disabled:      true,
					MaxTokens:     4096,
					ExcludeGlobs:  []string{"*.tmp"},
					RefreshMode:   "manual",
					MapMulNoFiles: 3.0,
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Tools.RepoMap.Disabled, "disabled should be ORed (true because second is true)")
		require.Equal(t, 4096, c.Tools.RepoMap.MaxTokens, "max_tokens should use second value (non-zero)")
		require.Equal(t, []string{"*.log", "*.tmp"}, c.Tools.RepoMap.ExcludeGlobs, "exclude_globs should be appended")
		require.Equal(t, "manual", c.Tools.RepoMap.RefreshMode, "refresh_mode should use second value")
		require.Equal(t, 3.0, c.Tools.RepoMap.MapMulNoFiles, "map_mul_no_files should use second value")
	})

	t.Run("repo_map_parser_pool_size_last_non_zero", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{ParserPoolSize: 4},
			},
		}, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{ParserPoolSize: 0},
			},
		}, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{ParserPoolSize: 8},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, 8, c.Tools.RepoMap.ParserPoolSize)
	})

	t.Run("repo_map_second_wins_nonzero", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{
					MaxTokens:     2048,
					MapMulNoFiles: 2.5,
				},
			},
		}, Config{
			Tools: Tools{
				RepoMap: RepoMapOptions{
					MaxTokens:     0,
					MapMulNoFiles: 0,
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, 2048, c.Tools.RepoMap.MaxTokens, "max_tokens should keep first when second is zero")
		require.Equal(t, 2.5, c.Tools.RepoMap.MapMulNoFiles, "map_mul_no_files should keep first when second is zero")
	})

	t.Run("models", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Models: map[SelectedModelType]SelectedModel{
				"large": {
					Model:    "gpt-4",
					Provider: "openai",
				},
			},
		}, Config{
			Models: map[SelectedModelType]SelectedModel{
				"large": {
					Model:    "gpt-4o",
					Provider: "openai",
				},
				"small": {
					Model:    "gpt-3.5-turbo",
					Provider: "openai",
				},
			},
		})

		require.NotNil(t, c)
		require.Len(t, c.Models, 2)
		require.Equal(t, "gpt-4o", c.Models["large"].Model)
		require.Equal(t, "gpt-3.5-turbo", c.Models["small"].Model)
	})

	t.Run("schema", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Schema: "https://example.com/schema.json",
		}, Config{
			Schema: "https://example.com/new-schema.json",
		})

		require.NotNil(t, c)
		require.Equal(t, "https://example.com/schema.json", c.Schema)
	})

	t.Run("schema_empty_first", func(t *testing.T) {
		c := exerciseMerge(t, Config{}, Config{
			Schema: "https://example.com/schema.json",
		})

		require.NotNil(t, c)
		require.Equal(t, "https://example.com/schema.json", c.Schema)
	})

	t.Run("permissions", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Permissions: &Permissions{
				AllowedTools: []string{"bash", "view"},
			},
		}, Config{
			Permissions: &Permissions{
				AllowedTools: []string{"edit", "write"},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"bash", "view", "edit", "write"}, c.Permissions.AllowedTools)
	})

	t.Run("mcp_timeout_max", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					Timeout: 10,
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					Timeout: 5,
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, 10, c.MCP["test"].Timeout)
	})

	t.Run("mcp_disabled_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					Disabled: false,
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					Disabled: true,
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.MCP["test"].Disabled)
	})

	t.Run("lsp_disabled_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					Disabled: false,
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					Disabled: true,
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.LSP["test"].Disabled)
	})

	t.Run("lsp_args_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					Args: []string{"old", "args"},
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					Args: []string{"new", "args"},
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"new", "args"}, c.LSP["test"].Args)
	})

	t.Run("lsp_filetypes_merged_and_deduplicated", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					FileTypes: []string{"go", "mod"},
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					FileTypes: []string{"go", "sum"},
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"go", "mod", "sum"}, c.LSP["test"].FileTypes)
	})

	t.Run("lsp_rootmarkers_merged_and_deduplicated", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					RootMarkers: []string{"go.mod", "go.sum"},
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					RootMarkers: []string{"go.sum", "go.work"},
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"go.mod", "go.sum", "go.work"}, c.LSP["test"].RootMarkers)
	})

	t.Run("options_attribution_nil", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				Attribution: &Attribution{
					TrailerStyle:  TrailerStyleCoAuthoredBy,
					GeneratedWith: true,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, TrailerStyleCoAuthoredBy, c.Options.Attribution.TrailerStyle)
		require.True(t, c.Options.Attribution.GeneratedWith)
	})

	t.Run("tui_compact_mode_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: false,
				},
			},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: true,
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Options.TUI.CompactMode)
	})

	t.Run("tui_diff_mode_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{
					DiffMode: "unified",
				},
			},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{
					DiffMode: "split",
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, "split", c.Options.TUI.DiffMode)
	})

	t.Run("options_data_directory_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				DataDirectory: ".crush",
				TUI:           &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				DataDirectory: ".custom",
				TUI:           &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, ".custom", c.Options.DataDirectory)
	})

	t.Run("mcp_args_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					Args: []string{"old"},
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					Args: []string{"new"},
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"new"}, c.MCP["test"].Args)
	})

	t.Run("mcp_command_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					Command: "old-command",
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					Command: "new-command",
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, "new-command", c.MCP["test"].Command)
	})

	t.Run("mcp_type_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					Type: MCPSSE,
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					Type: MCPStdio,
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, MCPStdio, c.MCP["test"].Type)
	})

	t.Run("mcp_url_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					URL: "http://old",
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					URL: "http://new",
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, "http://new", c.MCP["test"].URL)
	})

	t.Run("lsp_command_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					Command: "old-command",
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					Command: "new-command",
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, "new-command", c.LSP["test"].Command)
	})

	t.Run("lsp_timeout_max", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			LSP: LSPs{
				"test": {
					Timeout: 60,
				},
			},
		}, Config{
			LSP: LSPs{
				"test": {
					Timeout: 30,
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, 60, c.LSP["test"].Timeout)
	})

	t.Run("mcp_disabled_tools_appended", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			MCP: MCPs{
				"test": {
					DisabledTools: []string{"tool1"},
				},
			},
		}, Config{
			MCP: MCPs{
				"test": {
					DisabledTools: []string{"tool2"},
				},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"tool1", "tool2"}, c.MCP["test"].DisabledTools)
	})

	t.Run("options_skills_paths_appended", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				SkillsPaths: []string{"/path/1"},
				TUI:         &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				SkillsPaths: []string{"/path/2"},
				TUI:         &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, []string{"/path/1", "/path/2"}, c.Options.SkillsPaths)
	})

	t.Run("tui_transparent_replaced", func(t *testing.T) {
		trueVal := true
		falseVal := false
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{
					Transparent: &falseVal,
				},
			},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{
					Transparent: &trueVal,
				},
			},
		})

		require.NotNil(t, c)
		require.True(t, *c.Options.TUI.Transparent)
	})

	t.Run("options_initialize_as_replaced", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				InitializeAs: "CRUSH.md",
				TUI:          &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				InitializeAs: "AGENTS.md",
				TUI:          &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.Equal(t, "AGENTS.md", c.Options.InitializeAs)
	})

	t.Run("options_auto_lsp_replaced", func(t *testing.T) {
		trueVal := true
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				AutoLSP: &trueVal,
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.True(t, *c.Options.AutoLSP)
	})

	t.Run("options_disable_auto_summarize_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				DisableAutoSummarize: false,
				TUI:                  &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				DisableAutoSummarize: true,
				TUI:                  &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Options.DisableAutoSummarize)
	})

	t.Run("options_disable_default_providers_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				DisableDefaultProviders: false,
				TUI:                     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				DisableDefaultProviders: true,
				TUI:                     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Options.DisableDefaultProviders)
	})

	t.Run("provider_config_merge_preserves_fields", func(t *testing.T) {
		// Tests that merging a later provider config with empty fields
		// does not overwrite earlier non-empty fields.
		c := exerciseMerge(t, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					APIKey:  "key1",
					BaseURL: "https://api.openai.com/v1",
				},
			}),
		}, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					APIKey:  "key2",
					BaseURL: "https://api.openai.com/v2",
				},
			}),
		}, Config{
			// Later config with empty provider - should not clear fields.
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {},
			}),
		})

		require.NotNil(t, c)
		pc, ok := c.Providers.Get("openai")
		require.True(t, ok)
		require.Equal(t, "key2", pc.APIKey)
		require.Equal(t, "https://api.openai.com/v2", pc.BaseURL)
	})

	t.Run("provider_config_disable_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					APIKey:  "key1",
					Disable: false,
				},
			}),
		}, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					Disable: true,
				},
			}),
		})

		require.NotNil(t, c)
		pc, ok := c.Providers.Get("openai")
		require.True(t, ok)
		require.True(t, pc.Disable)
		require.Equal(t, "key1", pc.APIKey)
	})

	t.Run("tui_nil_in_first_config", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{},
		}, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: true,
					DiffMode:    "split",
				},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.TUI)
		require.True(t, c.Options.TUI.CompactMode)
		require.Equal(t, "split", c.Options.TUI.DiffMode)
	})

	t.Run("tui_nil_in_second_config", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{
					CompactMode: true,
				},
			},
		}, Config{
			Options: &Options{},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.TUI)
		require.True(t, c.Options.TUI.CompactMode)
	})

	t.Run("lcm_options_merged", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				LCM: &LCMOptions{
					CtxCutoffThreshold:            0.6,
					DisableLargeToolOutput:        false,
					LargeToolOutputTokenThreshold: 10000,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				LCM: &LCMOptions{
					CtxCutoffThreshold:            0.8,
					LargeToolOutputTokenThreshold: 5000,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.LCM)
		require.InDelta(t, 0.8, c.Options.LCM.CtxCutoffThreshold, 0.001)
		require.Equal(t, 5000, c.Options.LCM.LargeToolOutputTokenThreshold)
	})

	t.Run("lcm_explorer_output_profile_last_non_empty", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				LCM: &LCMOptions{ExplorerOutputProfile: "enhancement"},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				LCM: &LCMOptions{ExplorerOutputProfile: "parity"},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.LCM)
		require.Equal(t, "parity", c.Options.LCM.ExplorerOutputProfile)
	})

	t.Run("lcm_disable_large_tool_output_true_if_any", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				LCM: &LCMOptions{
					DisableLargeToolOutput: false,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				LCM: &LCMOptions{
					DisableLargeToolOutput: true,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.True(t, c.Options.LCM.DisableLargeToolOutput)
	})

	t.Run("lcm_nil_in_first_config", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				LCM: &LCMOptions{
					CtxCutoffThreshold: 0.7,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.LCM)
		require.InDelta(t, 0.7, c.Options.LCM.CtxCutoffThreshold, 0.001)
	})

	t.Run("repo_map_options_merged", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					Disabled:      false,
					MaxTokens:     1024,
					ExcludeGlobs:  []string{"vendor/**", "*.min.js"},
					RefreshMode:   "auto",
					MapMulNoFiles: 2.0,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					Disabled:      true,
					MaxTokens:     2048,
					ExcludeGlobs:  []string{"*.min.js", "dist/**"},
					RefreshMode:   "manual",
					MapMulNoFiles: 3.5,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.RepoMap)
		require.True(t, c.Options.RepoMap.Disabled)
		require.Equal(t, 2048, c.Options.RepoMap.MaxTokens)
		require.Equal(t, "manual", c.Options.RepoMap.RefreshMode)
		require.Equal(t, 3.5, c.Options.RepoMap.MapMulNoFiles)
		require.Equal(t, []string{"*.min.js", "dist/**", "vendor/**"}, c.Options.RepoMap.ExcludeGlobs)
	})

	t.Run("repo_map_parser_pool_size_options_last_non_zero", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{ParserPoolSize: 2},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{ParserPoolSize: 0},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{ParserPoolSize: 6},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.RepoMap)
		require.Equal(t, 6, c.Options.RepoMap.ParserPoolSize)
	})

	t.Run("repo_map_disabled_or_latch", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{Disabled: true},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{Disabled: false},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.RepoMap)
		require.True(t, c.Options.RepoMap.Disabled)
	})

	t.Run("repo_map_nil_in_first_config", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{TUI: &TUIOptions{}},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					MaxTokens:     4096,
					RefreshMode:   "always",
					MapMulNoFiles: 1.5,
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.RepoMap)
		require.Equal(t, 4096, c.Options.RepoMap.MaxTokens)
		require.Equal(t, "always", c.Options.RepoMap.RefreshMode)
		require.Equal(t, 1.5, c.Options.RepoMap.MapMulNoFiles)
	})

	t.Run("repo_map_nil_in_second_config", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					MaxTokens:     4096,
					ExcludeGlobs:  []string{"vendor/**"},
					RefreshMode:   "always",
					MapMulNoFiles: 1.5,
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{TUI: &TUIOptions{}},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Options.RepoMap)
		// First config values are preserved when second has nil
		require.Equal(t, 4096, c.Options.RepoMap.MaxTokens)
		require.Equal(t, "always", c.Options.RepoMap.RefreshMode)
		require.Equal(t, 1.5, c.Options.RepoMap.MapMulNoFiles)
		require.Equal(t, []string{"vendor/**"}, c.Options.RepoMap.ExcludeGlobs)
	})

	t.Run("repo_map_maxtokens_last_non_zero", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MaxTokens: 2048},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MaxTokens: 0},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has 0, earlier non-zero value is kept
		require.Equal(t, 2048, c.Options.RepoMap.MaxTokens)
	})

	t.Run("repo_map_maxtokens_non_zero_overrides", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MaxTokens: 1024},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MaxTokens: 4096},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has non-zero, it overrides
		require.Equal(t, 4096, c.Options.RepoMap.MaxTokens)
	})

	t.Run("repo_map_refreshmode_last_non_empty", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{RefreshMode: "manual"},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{RefreshMode: ""},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has empty string, earlier non-empty is kept
		require.Equal(t, "manual", c.Options.RepoMap.RefreshMode)
	})

	t.Run("repo_map_refreshmode_non_empty_overrides", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{RefreshMode: "auto"},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{RefreshMode: "always"},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has non-empty, it overrides
		require.Equal(t, "always", c.Options.RepoMap.RefreshMode)
	})

	t.Run("repo_map_mapmulnofiles_zero_does_not_override", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MapMulNoFiles: 2.5},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MapMulNoFiles: 0},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has 0, earlier non-zero is kept
		require.Equal(t, 2.5, c.Options.RepoMap.MapMulNoFiles)
	})

	t.Run("repo_map_mapmulnofiles_non_zero_overrides", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MapMulNoFiles: 1.5},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{MapMulNoFiles: 3.0},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// When later config has non-zero, it overrides
		require.Equal(t, 3.0, c.Options.RepoMap.MapMulNoFiles)
	})

	t.Run("repo_map_excludeglobs_empty_first", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					ExcludeGlobs: []string{},
				},
				TUI: &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{
					ExcludeGlobs: []string{"vendor/**", "node_modules/**"},
				},
				TUI: &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// Empty array + globs = globs
		require.Equal(t, []string{"node_modules/**", "vendor/**"}, c.Options.RepoMap.ExcludeGlobs)
	})

	t.Run("repo_map_disabled_remains_true", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{Disabled: true},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{Disabled: false},
				TUI:     &TUIOptions{},
			},
		}, Config{
			Options: &Options{
				RepoMap: &RepoMapOptions{Disabled: false},
				TUI:     &TUIOptions{},
			},
		})

		require.NotNil(t, c)
		// Once disabled, stays disabled even if later configs don't disable it
		require.True(t, c.Options.RepoMap.Disabled)
	})

	t.Run("grep_timeout_merged", func(t *testing.T) {
		timeout1 := 10 * time.Second
		timeout2 := 15 * time.Second

		c := exerciseMerge(t, Config{
			Tools: Tools{
				Grep: ToolGrep{
					Timeout: &timeout1,
				},
			},
		}, Config{
			Tools: Tools{
				Grep: ToolGrep{
					Timeout: &timeout2,
				},
			},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Tools.Grep.Timeout)
		require.Equal(t, 15*time.Second, *c.Tools.Grep.Timeout)
	})

	t.Run("grep_timeout_kept_when_second_nil", func(t *testing.T) {
		timeout := 10 * time.Second

		c := exerciseMerge(t, Config{
			Tools: Tools{
				Grep: ToolGrep{
					Timeout: &timeout,
				},
			},
		}, Config{
			Tools: Tools{},
		})

		require.NotNil(t, c)
		require.NotNil(t, c.Tools.Grep.Timeout)
		require.Equal(t, 10*time.Second, *c.Tools.Grep.Timeout)
	})

	t.Run("provider_config_extra_headers_merged", func(t *testing.T) {
		c := exerciseMerge(t, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					ExtraHeaders: map[string]string{"X-First": "value1"},
				},
			}),
		}, Config{
			Providers: csync.NewMapFrom(map[string]ProviderConfig{
				"openai": {
					ExtraHeaders: map[string]string{"X-Second": "value2"},
				},
			}),
		})

		require.NotNil(t, c)
		pc, ok := c.Providers.Get("openai")
		require.True(t, ok)
		require.Equal(t, "value1", pc.ExtraHeaders["X-First"])
		require.Equal(t, "value2", pc.ExtraHeaders["X-Second"])
	})
}

func exerciseMerge(tb testing.TB, confs ...Config) *Config {
	tb.Helper()
	data := make([][]byte, 0, len(confs))
	for _, c := range confs {
		bts, err := json.Marshal(c)
		require.NoError(tb, err)
		data = append(data, bts)
	}
	result, err := loadFromBytes(data)
	require.NoError(tb, err)
	return result
}
