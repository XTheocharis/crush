package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
	"github.com/stretchr/testify/require"
)

func writeMockNpmCompanion(t *testing.T, entrypoint string) string {
	t.Helper()
	dir := t.TempDir()
	writeMockNode(t, dir)
	script := filepath.Join(dir, "npm")
	content := "#!/bin/sh\n" +
		"PREFIX=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ -n \"$PREFIX\" ]; then PREFIX_ARG=\"$arg\"; PREFIX=\"\"; continue; fi\n" +
		"  if [ \"$arg\" = \"--prefix\" ]; then PREFIX=\"1\"; fi\n" +
		"done\n" +
		"mkdir -p \"${PREFIX_ARG}/node_modules/.bin\"\n" +
		"mkdir -p \"${PREFIX_ARG}/node_modules/typescript/lib\"\n" +
		"touch \"${PREFIX_ARG}/node_modules/.bin/" + entrypoint + "\"\n" +
		"touch \"${PREFIX_ARG}/node_modules/.bin/typescript-language-server\"\n" +
		"touch \"${PREFIX_ARG}/node_modules/typescript-svelte-plugin\"\n" +
		"touch \"${PREFIX_ARG}/node_modules/@vue/typescript-plugin\"\n" +
		"chmod +x \"${PREFIX_ARG}/node_modules/.bin/" + entrypoint + "\"\n" +
		"chmod +x \"${PREFIX_ARG}/node_modules/.bin/typescript-language-server\"\n"
	err := os.WriteFile(script, []byte(content), 0o755)
	require.NoError(t, err)
	return dir
}

func TestCompanionBasicInstall(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "svelteserver"
	mockDir := writeMockNpmCompanion(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:           "svelte-language-server",
		Version:           "0.18.0",
		Entrypoint:        entrypoint,
		CompanionPackages: []string{"typescript@6.0.3", "typescript-language-server@5.1.3", "typescript-svelte-plugin@0.3.52"},
		CompanionServer:   "typescript-language-server",
	}

	path, err := inst.Install(context.Background(), "svelte-language-server", cfg)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "svelte-language-server", "0.18.0", "node_modules", ".bin", entrypoint)
	if runtime.GOOS == "windows" {
		expected += ".cmd"
	}
	require.Equal(t, expected, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
}

func TestCompanionIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "svelteserver"
	mockDir := writeMockNpmCompanion(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:           "svelte-language-server",
		Version:           "0.18.0",
		Entrypoint:        entrypoint,
		CompanionPackages: []string{"typescript@6.0.3", "typescript-language-server@5.1.3", "typescript-svelte-plugin@0.3.52"},
		CompanionServer:   "typescript-language-server",
	}

	path1, err := inst.Install(context.Background(), "svelte-language-server", cfg)
	require.NoError(t, err)

	path2, err := inst.Install(context.Background(), "svelte-language-server", cfg)
	require.NoError(t, err)

	require.Equal(t, path1, path2)
}

func TestCompanionResolvePathsSvelte(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "svelte-language-server", "0.18.0")

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "svelte-language-server",
		Entrypoint: "svelteserver",
	}

	paths := inst.ResolveCompanionPaths(installDir, cfg)

	entrypoint := "svelteserver"
	if runtime.GOOS == "windows" {
		entrypoint += ".cmd"
	}
	require.Equal(t, filepath.Join(installDir, "node_modules", ".bin", entrypoint), paths.PrimaryBinary)
	require.Equal(t, filepath.Join(installDir, "node_modules", ".bin", "typescript-language-server"), paths.CompanionBinary)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript-svelte-plugin"), paths.PluginPath)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), paths.TSDKPath)
}

func TestCompanionResolvePathsVue(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "vue-language-server", "3.1.5")

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "@vue/language-server",
		Entrypoint: "vue-language-server",
	}

	paths := inst.ResolveCompanionPaths(installDir, cfg)

	entrypoint := "vue-language-server"
	if runtime.GOOS == "windows" {
		entrypoint += ".cmd"
	}
	require.Equal(t, filepath.Join(installDir, "node_modules", ".bin", entrypoint), paths.PrimaryBinary)
	require.Equal(t, filepath.Join(installDir, "node_modules", ".bin", "typescript-language-server"), paths.CompanionBinary)
	require.Equal(t, filepath.Join(installDir, "node_modules", "@vue", "typescript-plugin"), paths.PluginPath)
	require.Equal(t, filepath.Join(installDir, "node_modules", "typescript", "lib"), paths.TSDKPath)
}

func TestCompanionMissingRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", "")

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "svelte-language-server",
		Version:    "0.18.0",
		Entrypoint: "svelteserver",
	}

	_, err := inst.Install(context.Background(), "svelte-language-server", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRuntimeMissing))
}

func TestCompanionNpmFails(t *testing.T) {
	tmpDir := t.TempDir()
	mockDir := writeMockNpmFail(t)
	prependPATH(t, mockDir)

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:           "svelte-language-server",
		Version:           "0.18.0",
		Entrypoint:        "svelteserver",
		CompanionPackages: []string{"typescript@6.0.3"},
	}

	_, err := inst.Install(context.Background(), "svelte-language-server", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))

	installDir := filepath.Join(tmpDir, "svelte-language-server", "0.18.0")
	_, statErr := os.Stat(installDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestCompanionTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockNode(t, dir)
	err := os.WriteFile(filepath.Join(dir, "npm"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o755)
	require.NoError(t, err)
	prependPATH(t, dir)

	inst := NewCompanionInstaller(tmpDir)
	inst.timeout = 100 * time.Millisecond

	cfg := catalog.InstallConfig{
		Package:           "svelte-language-server",
		Version:           "0.18.0",
		Entrypoint:        "svelteserver",
		CompanionPackages: []string{"typescript@6.0.3"},
	}

	_, err = inst.Install(context.Background(), "svelte-language-server", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallTimeout))
}

func TestCompanionEntrypointMissing(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockNode(t, dir)
	err := os.WriteFile(filepath.Join(dir, "npm"), []byte("#!/bin/sh\n"), 0o755)
	require.NoError(t, err)
	prependPATH(t, dir)

	inst := NewCompanionInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:           "svelte-language-server",
		Version:           "0.18.0",
		Entrypoint:        "svelteserver",
		CompanionPackages: []string{"typescript@6.0.3"},
	}

	_, err = inst.Install(context.Background(), "svelte-language-server", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))
	require.Contains(t, err.Error(), "entrypoint not found")
}

func TestBuildCompanionVersionString(t *testing.T) {
	cfg := catalog.InstallConfig{
		Version:           "0.18.0",
		CompanionPackages: []string{"typescript@6.0.3", "typescript-language-server@5.1.3"},
	}
	got := buildCompanionVersionString(cfg)
	require.Equal(t, "0.18.0_typescript@6.0.3_typescript-language-server@5.1.3", got)
}
