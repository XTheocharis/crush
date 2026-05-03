package config

import (
	"github.com/charmbracelet/crush/internal/csync"
)

func newConfig() *Config {
	return &Config{
		Agents:       map[string]Agent{},
		MCP:          map[string]MCPConfig{},
		LSP:          map[string]LSPConfig{},
		Models:       map[SelectedModelType]SelectedModel{},
		RecentModels: map[SelectedModelType][]SelectedModel{},
		Options: &Options{
			TUI: &TUIOptions{},
		},
		Permissions: &Permissions{},
		Providers:   csync.NewMap[string, ProviderConfig](),
	}
}
