package lsp

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
	"github.com/stretchr/testify/require"
)

func TestCompanionInitOptionsSvelte(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "svelte-language-server", "0.18.0")
	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "svelte-language-server",
		Version:    "0.18.0",
		Entrypoint: "svelteserver",
	}

	cm := NewCompanionManager()
	cm.RegisterCompanion("svelte-language-server", inst, cfg, installDir)

	opts := cm.GetInitOptions("svelte-language-server")
	require.NotNil(t, opts)

	tsOpts, ok := opts["typescript"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), tsOpts["tsdk"])

	jsOpts, ok := opts["javascript"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), jsOpts["tsdk"])

	jstsOpts, ok := opts["js/ts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), jstsOpts["tsdk"])
}

func TestCompanionInitOptionsVue(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "vue-language-server", "3.1.5")
	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "@vue/language-server",
		Version:    "3.1.5",
		Entrypoint: "vue-language-server",
	}

	cm := NewCompanionManager()
	cm.RegisterCompanion("vue-language-server", inst, cfg, installDir)

	opts := cm.GetInitOptions("vue-language-server")
	require.NotNil(t, opts)

	tsOpts, ok := opts["typescript"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), tsOpts["tsdk"])

	vueOpts, ok := opts["vue"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, vueOpts["hybridMode"])
}

func TestCompanionInitOptionsUnknown(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "unknown-server", "1.0.0")
	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-other-server",
		Version:    "1.0.0",
		Entrypoint: "some-server",
	}

	cm := NewCompanionManager()
	cm.RegisterCompanion("unknown-server", inst, cfg, installDir)

	opts := cm.GetInitOptions("unknown-server")
	require.Nil(t, opts)
}

func TestCompanionManagerGetInitOptionsNotRegistered(t *testing.T) {
	cm := NewCompanionManager()
	opts := cm.GetInitOptions("nonexistent")
	require.Nil(t, opts)
}

func TestCompanionManagerRegisterAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "svelte-language-server", "0.18.0")
	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "svelte-language-server",
		Version:    "0.18.0",
		Entrypoint: "svelteserver",
	}

	cm := NewCompanionManager()

	require.Nil(t, cm.GetInitOptions("svelte-language-server"))

	cm.RegisterCompanion("svelte-language-server", inst, cfg, installDir)

	opts := cm.GetInitOptions("svelte-language-server")
	require.NotNil(t, opts)
	require.Contains(t, opts, "typescript")
}

func TestCompanionInstallDir(t *testing.T) {
	got := companionInstallDir("/cache", "svelte-language-server", "0.18.0")
	require.Equal(t, filepath.Join("/cache", "svelte-language-server", "0.18.0"), got)
}
