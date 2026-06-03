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

func writeMockNode(t *testing.T, dir string) {
	t.Helper()
	node := filepath.Join(dir, "node")
	err := os.WriteFile(node, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	require.NoError(t, err)
}

func writeMockNpm(t *testing.T, entrypoint string) string {
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
		"touch \"${PREFIX_ARG}/node_modules/.bin/" + entrypoint + "\"\n" +
		"chmod +x \"${PREFIX_ARG}/node_modules/.bin/" + entrypoint + "\"\n"
	err := os.WriteFile(script, []byte(content), 0o755)
	require.NoError(t, err)
	return dir
}

func writeMockNpmFail(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMockNode(t, dir)
	script := filepath.Join(dir, "npm")
	err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	require.NoError(t, err)
	return dir
}

func prependPATH(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+orig)
}

func TestNpmInstallerBasicInstall(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "test-entry"
	mockDir := writeMockNpm(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewNpmInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: entrypoint,
	}

	path, err := inst.Install(context.Background(), "test-lsp", cfg)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "test-lsp", "1.0.0", "node_modules", ".bin", entrypoint)
	if runtime.GOOS == "windows" {
		expected += ".cmd"
	}
	require.Equal(t, expected, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
}

func TestNpmInstallerIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "idem-entry"
	mockDir := writeMockNpm(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewNpmInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "2.0.0",
		Entrypoint: entrypoint,
	}

	path1, err := inst.Install(context.Background(), "idem-lsp", cfg)
	require.NoError(t, err)

	path2, err := inst.Install(context.Background(), "idem-lsp", cfg)
	require.NoError(t, err)

	require.Equal(t, path1, path2)
}

func TestNpmInstallerMissingRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", "")

	inst := NewNpmInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err := inst.Install(context.Background(), "no-runtime-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRuntimeMissing))
}

func TestNpmInstallerNpmFails(t *testing.T) {
	tmpDir := t.TempDir()
	mockDir := writeMockNpmFail(t)
	prependPATH(t, mockDir)

	inst := NewNpmInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "bad-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err := inst.Install(context.Background(), "fail-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))

	installDir := filepath.Join(tmpDir, "fail-lsp", "1.0.0")
	_, statErr := os.Stat(installDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestNpmInstallerTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockNode(t, dir)
	err := os.WriteFile(filepath.Join(dir, "npm"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o755)
	require.NoError(t, err)
	prependPATH(t, dir)

	inst := NewNpmInstaller(tmpDir)
	inst.timeout = 100 * time.Millisecond

	cfg := catalog.InstallConfig{
		Package:    "slow-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err = inst.Install(context.Background(), "timeout-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallTimeout))
}

func TestNpmInstallerEntrypointMissing(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockNode(t, dir)
	err := os.WriteFile(filepath.Join(dir, "npm"), []byte("#!/bin/sh\n"), 0o755)
	require.NoError(t, err)
	prependPATH(t, dir)

	inst := NewNpmInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "noentry-lsp",
		Version:    "1.0.0",
		Entrypoint: "missing-entry",
	}

	_, err = inst.Install(context.Background(), "noentry-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))
	require.Contains(t, err.Error(), "entrypoint not found")
}

func TestNpmInstallerEntrypointPath(t *testing.T) {
	t.Parallel()

	inst := NewNpmInstaller(t.TempDir())

	got := inst.entrypointPath("/cache/lsp/name/1.0", "my-server")

	expected := filepath.Join("/cache/lsp/name/1.0", "node_modules", ".bin", "my-server")
	if runtime.GOOS == "windows" {
		expected += ".cmd"
	}
	require.Equal(t, expected, got)
}

func writeMockPython(t *testing.T, dir string) {
	t.Helper()
	python := filepath.Join(dir, "python")
	err := os.WriteFile(python, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	require.NoError(t, err)
}

func writeMockPip(t *testing.T, entrypoint string) string {
	t.Helper()
	dir := t.TempDir()
	writeMockPython(t, dir)
	content := "#!/bin/sh\n" +
		"TARGET=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ -n \"$TARGET\" ]; then TARGET_DIR=\"$arg\"; TARGET=\"\"; continue; fi\n" +
		"  if [ \"$arg\" = \"--target\" ]; then TARGET=\"1\"; fi\n" +
		"done\n" +
		"mkdir -p \"${TARGET_DIR}/bin\"\n" +
		"touch \"${TARGET_DIR}/bin/" + entrypoint + "\"\n" +
		"chmod +x \"${TARGET_DIR}/bin/" + entrypoint + "\"\n"
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
		require.NoError(t, err)
	}
	return dir
}

func writeMockPipFail(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMockPython(t, dir)
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 1\n"), 0o755)
		require.NoError(t, err)
	}
	return dir
}

func writeMockPipNoBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMockPython(t, dir)
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
		require.NoError(t, err)
	}
	return dir
}

func writeMockPipTopLevel(t *testing.T, entrypoint string) string {
	t.Helper()
	dir := t.TempDir()
	writeMockPython(t, dir)
	content := "#!/bin/sh\n" +
		"TARGET=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ -n \"$TARGET\" ]; then TARGET_DIR=\"$arg\"; TARGET=\"\"; continue; fi\n" +
		"  if [ \"$arg\" = \"--target\" ]; then TARGET=\"1\"; fi\n" +
		"done\n" +
		"touch \"${TARGET_DIR}/" + entrypoint + "\"\n" +
		"chmod +x \"${TARGET_DIR}/" + entrypoint + "\"\n"
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
		require.NoError(t, err)
	}
	return dir
}

func writeMockUv(t *testing.T, entrypoint string) string {
	t.Helper()
	dir := t.TempDir()
	writeMockPython(t, dir)
	script := filepath.Join(dir, "uv")
	content := "#!/bin/sh\n" +
		"if [ \"$1\" != \"pip\" ]; then echo \"expected pip subcommand\" >&2; exit 1; fi\n" +
		"shift\n" +
		"TARGET=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ -n \"$TARGET\" ]; then TARGET_DIR=\"$arg\"; TARGET=\"\"; continue; fi\n" +
		"  if [ \"$arg\" = \"--target\" ]; then TARGET=\"1\"; fi\n" +
		"done\n" +
		"/usr/bin/mkdir -p \"${TARGET_DIR}/bin\"\n" +
		"/usr/bin/touch \"${TARGET_DIR}/bin/" + entrypoint + "\"\n" +
		"/usr/bin/chmod +x \"${TARGET_DIR}/bin/" + entrypoint + "\"\n"
	err := os.WriteFile(script, []byte(content), 0o755)
	require.NoError(t, err)
	return dir
}

func TestPipInstallerUvFallback(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "uv-entry"
	mockDir := writeMockUv(t, entrypoint)
	// Isolate PATH to mock dir only so pip/pip3/pip are not found,
	// forcing fallback to the mock uv.
	t.Setenv("PATH", mockDir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: entrypoint,
	}

	path, err := inst.Install(context.Background(), "uv-lsp", cfg)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "uv-lsp", "1.0.0", "bin", entrypoint)
	require.Equal(t, expected, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
}

func TestPipInstallerBasicInstall(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "test-entry"
	mockDir := writeMockPip(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: entrypoint,
	}

	path, err := inst.Install(context.Background(), "test-lsp", cfg)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "test-lsp", "1.0.0", "bin", entrypoint)
	require.Equal(t, expected, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
}

func TestPipInstallerTopLevelEntrypoint(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "top-level-entry"
	mockDir := writeMockPipTopLevel(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: entrypoint,
	}

	path, err := inst.Install(context.Background(), "test-lsp", cfg)
	require.NoError(t, err)

	expected := filepath.Join(tmpDir, "test-lsp", "1.0.0", entrypoint)
	require.Equal(t, expected, path)
}

func TestPipInstallerIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	entrypoint := "idem-entry"
	mockDir := writeMockPip(t, entrypoint)
	prependPATH(t, mockDir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "2.0.0",
		Entrypoint: entrypoint,
	}

	path1, err := inst.Install(context.Background(), "idem-lsp", cfg)
	require.NoError(t, err)

	path2, err := inst.Install(context.Background(), "idem-lsp", cfg)
	require.NoError(t, err)

	require.Equal(t, path1, path2)
}

func TestPipInstallerMissingRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PATH", "")

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "some-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err := inst.Install(context.Background(), "no-runtime-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRuntimeMissing))
}

func TestPipInstallerPipFails(t *testing.T) {
	tmpDir := t.TempDir()
	mockDir := writeMockPipFail(t)
	prependPATH(t, mockDir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "bad-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err := inst.Install(context.Background(), "fail-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))

	installDir := filepath.Join(tmpDir, "fail-lsp", "1.0.0")
	_, statErr := os.Stat(installDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestPipInstallerTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockPython(t, dir)
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexec sleep 60\n"), 0o755)
		require.NoError(t, err)
	}
	prependPATH(t, dir)

	inst := NewPipInstaller(tmpDir)
	inst.timeout = 100 * time.Millisecond

	cfg := catalog.InstallConfig{
		Package:    "slow-lsp",
		Version:    "1.0.0",
		Entrypoint: "entry",
	}

	_, err := inst.Install(context.Background(), "timeout-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallTimeout))
}

func TestPipInstallerEntrypointMissing(t *testing.T) {
	tmpDir := t.TempDir()

	dir := t.TempDir()
	writeMockPython(t, dir)
	for _, name := range []string{"pip", "pip3"} {
		err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
		require.NoError(t, err)
	}
	prependPATH(t, dir)

	inst := NewPipInstaller(tmpDir)
	cfg := catalog.InstallConfig{
		Package:    "noentry-lsp",
		Version:    "1.0.0",
		Entrypoint: "missing-entry",
	}

	_, err := inst.Install(context.Background(), "noentry-lsp", cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInstallFailed))
	require.Contains(t, err.Error(), "entrypoint not found")
}
