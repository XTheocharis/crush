package lsp

import (
	"path/filepath"
	"sync"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
)

// CompanionManager manages companion server init options.
// It computes the initializationOptions needed for companion LSP servers
// (Svelte, Vue) based on the resolved install paths.
type CompanionManager struct {
	mu         sync.RWMutex
	initOpts   map[string]map[string]any // server name -> init options
	installers map[string]*CompanionInstaller
	configs    map[string]catalog.InstallConfig
}

// NewCompanionManager creates a new CompanionManager.
func NewCompanionManager() *CompanionManager {
	return &CompanionManager{
		initOpts:   make(map[string]map[string]any),
		installers: make(map[string]*CompanionInstaller),
		configs:    make(map[string]catalog.InstallConfig),
	}
}

// RegisterCompanion registers a companion server's installer and config.
// Called during resolveViaInstaller when a companion install succeeds.
func (cm *CompanionManager) RegisterCompanion(name string, installer *CompanionInstaller, cfg catalog.InstallConfig, installDir string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.installers[name] = installer
	cm.configs[name] = cfg
	cm.initOpts[name] = cm.computeInitOptions(installer, cfg, installDir)
}

// GetInitOptions returns the companion init options for a server, or nil if
// not a companion.
func (cm *CompanionManager) GetInitOptions(name string) map[string]any {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.initOpts[name]
}

func (cm *CompanionManager) computeInitOptions(installer *CompanionInstaller, cfg catalog.InstallConfig, installDir string) map[string]any {
	paths := installer.ResolveCompanionPaths(installDir, cfg)

	switch cfg.Package {
	case "svelte-language-server":
		return map[string]any{
			"typescript": map[string]any{
				"tsdk": paths.TSDKPath,
			},
			"javascript": map[string]any{
				"tsdk": paths.TSDKPath,
			},
			"js/ts": map[string]any{
				"tsdk": paths.TSDKPath,
			},
		}
	case "@vue/language-server":
		return map[string]any{
			"typescript": map[string]any{
				"tsdk": paths.TSDKPath,
			},
			"vue": map[string]any{
				"hybridMode": true,
			},
		}
	default:
		return nil
	}
}

func companionInstallDir(cacheDir, name, version string) string {
	return filepath.Join(cacheDir, name, version)
}
