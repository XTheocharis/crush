package lsp

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
	"github.com/stretchr/testify/require"
)

func buildTestVSIX(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func buildTestVSIXWithBinaries(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func fakeVSIXFiles() map[string]string {
	return map[string]string{
		"extension/server/plugins/org.eclipse.equinox.launcher_1.7.100.v20251111-0406.jar": "launcher-content",
		"extension/server/config_linux/config.ini":                                         "config",
		"extension/server/config_mac_arm/config.ini":                                       "config",
		"extension/server/config_mac/config.ini":                                           "config",
		"extension/server/config_win/config.ini":                                           "config",
	}
}

func fakeVSIXFilesWithJRE() map[string][]byte {
	files := make(map[string][]byte)
	for k, v := range fakeVSIXFiles() {
		files[k] = []byte(v)
	}
	javaName := "java"
	if runtime.GOOS == "windows" {
		javaName = "java.exe"
	}
	files["extension/jre/21.0.10-linux-x86_64/bin/"+javaName] = []byte("#!/bin/bash\nexec java")
	return files
}

func TestJdtlsInstallerBasicInstall(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	vsixData := buildTestVSIXWithBinaries(t, fakeVSIXFilesWithJRE())
	vsixHash := sha256Hex(vsixData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(vsixData)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	installer := NewJdtlsInstaller(cacheDir)
	installer.timeout = 30 * time.Second

	cfg := catalog.InstallConfig{
		Method:     "jdtls",
		Version:    "1.54.0-923",
		VsixURL:    server.URL + "/java.vsix",
		VsixSHA256: vsixHash,
	}

	cmd, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, cmd)

	args := installer.GetJdtlsArgs("eclipse-jdtls")
	require.NotEmpty(t, args)

	hasJar := false
	for _, a := range args {
		if a == "-jar" {
			hasJar = true
			break
		}
	}
	require.True(t, hasJar, "args should contain -jar flag")
}

func TestJdtlsInstallerIdempotent(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	cacheDir := t.TempDir()
	installDir := filepath.Join(cacheDir, "eclipse-jdtls", "1.54.0-923")
	extDir := filepath.Join(installDir, "extension", "server")
	pluginsDir := filepath.Join(extDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginsDir, 0o755))

	launcherPath := filepath.Join(pluginsDir, "org.eclipse.equinox.launcher_1.7.100.v20251111-0406.jar")
	require.NoError(t, os.WriteFile(launcherPath, []byte("fake"), 0o644))

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(500)
	}))
	defer server.Close()

	installer := NewJdtlsInstaller(cacheDir)
	cfg := catalog.InstallConfig{
		Method:     "jdtls",
		Version:    "1.54.0-923",
		VsixURL:    server.URL + "/java.vsix",
		VsixSHA256: "deadbeef",
	}

	cmd, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, cmd)
	require.Equal(t, 0, requestCount, "should not make HTTP request when already installed")
}

func TestJdtlsInstallerMissingJVM(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer t.Setenv("PATH", origPath)

	cacheDir := t.TempDir()
	installer := NewJdtlsInstaller(cacheDir)

	cfg := catalog.InstallConfig{
		Method:  "jdtls",
		Version: "1.54.0-923",
	}

	_, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRuntimeMissing)
}

func TestJdtlsInstallerSHA256Mismatch(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	vsixData := buildTestVSIX(t, fakeVSIXFiles())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(vsixData)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	installer := NewJdtlsInstaller(cacheDir)
	installer.timeout = 30 * time.Second

	cfg := catalog.InstallConfig{
		Method:     "jdtls",
		Version:    "1.54.0-923",
		VsixURL:    server.URL + "/java.vsix",
		VsixSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}

	_, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SHA256 mismatch")
}

func TestJdtlsInstallerJREPathFallback(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	vsixData := buildTestVSIX(t, fakeVSIXFiles())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(vsixData)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	installer := NewJdtlsInstaller(cacheDir)
	installer.timeout = 30 * time.Second

	cfg := catalog.InstallConfig{
		Method:     "jdtls",
		Version:    "1.54.0-923",
		VsixURL:    server.URL + "/java.vsix",
		VsixSHA256: sha256Hex(vsixData),
	}

	cmd, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, cmd)

	javaPath, _ := exec.LookPath("java")
	require.Contains(t, cmd, javaPath)
}

func TestJdtlsInstallerDownloadFailure(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	installer := NewJdtlsInstaller(cacheDir)
	installer.timeout = 30 * time.Second

	cfg := catalog.InstallConfig{
		Method:  "jdtls",
		Version: "1.54.0-923",
		VsixURL: server.URL + "/java.vsix",
	}

	_, err := installer.Install(context.Background(), "eclipse-jdtls", cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status 500")
}

func TestJdtlsConfigDirMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goos     string
		goarch   string
		expected string
	}{
		{"linux", "amd64", "config_linux"},
		{"linux", "arm64", "config_linux_arm"},
		{"darwin", "arm64", "config_mac_arm"},
		{"darwin", "amd64", "config_mac"},
		{"windows", "amd64", "config_win"},
	}

	for _, tc := range tests {
		result := jdtlsConfigDirWithPlatform(tc.goos, tc.goarch, "/ext")
		require.Equal(t, filepath.Join("/ext", tc.expected), result, fmt.Sprintf("goos=%s goarch=%s", tc.goos, tc.goarch))
	}
}

func jdtlsConfigDirWithPlatform(goos, goarch, extDir string) string {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return filepath.Join(extDir, "config_linux")
	case "linux/arm64":
		return filepath.Join(extDir, "config_linux_arm")
	case "darwin/arm64":
		return filepath.Join(extDir, "config_mac_arm")
	case "darwin/amd64":
		return filepath.Join(extDir, "config_mac")
	case "windows/amd64":
		return filepath.Join(extDir, "config_win")
	default:
		return filepath.Join(extDir, "config_linux")
	}
}

func TestExtractVSIX(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"extension/server/plugins/test.jar": "jar-content",
		"extension/server/config_linux/":    "",
		"extension/readme.md":               "readme",
	}
	vsixData := buildTestVSIX(t, files)

	dest := t.TempDir()
	require.NoError(t, extractVSIX(vsixData, dest))

	content, err := os.ReadFile(filepath.Join(dest, "extension", "server", "plugins", "test.jar"))
	require.NoError(t, err)
	require.Equal(t, "jar-content", string(content))

	content, err = os.ReadFile(filepath.Join(dest, "extension", "readme.md"))
	require.NoError(t, err)
	require.Equal(t, "readme", string(content))
}

func TestResolveJdtlsPlatformURL(t *testing.T) {
	t.Parallel()

	url, sha, ok := ResolveJdtlsPlatformURL("eclipse-jdtls")
	require.True(t, ok, "eclipse-jdtls should have platform URLs")
	require.NotEmpty(t, url)
	require.NotEmpty(t, sha)

	platformKey := runtime.GOOS + "/" + runtime.GOARCH
	expectedEntry := map[string]struct {
		urlPrefix string
		sha       string
	}{
		"linux/amd64":   {"https://github.com/redhat-developer/vscode-java/releases/download/v1.54.0/java-linux-x64-1.54.0-923.vsix", "9d4b15da54e25a0192f9bac073f086c015397d3676623b68dbf83a5dbaf5132b"},
		"linux/arm64":   {"https://github.com/redhat-developer/vscode-java/releases/download/v1.54.0/java-linux-arm64-1.54.0-923.vsix", "e2bb22c427d90da8dbb1afff72ff1e2dce38d50b76deb02d7bc313a330a1330c"},
		"darwin/amd64":  {"https://github.com/redhat-developer/vscode-java/releases/download/v1.54.0/java-darwin-x64-1.54.0-923.vsix", "dfc98abc4e54165a78372e280242a039671729b1b03420608df3b10c6b629fb6"},
		"darwin/arm64":  {"https://github.com/redhat-developer/vscode-java/releases/download/v1.54.0/java-darwin-arm64-1.54.0-923.vsix", "c54c45cb0d2579d8e0a4ddeb24d4a9dd0b460d07d9366adea2b38a1da22a463c"},
		"windows/amd64": {"https://github.com/redhat-developer/vscode-java/releases/download/v1.54.0/java-win32-x64-1.54.0-923.vsix", "66f3914987edeccfee8a2558470e0fde4f8c4154232ff4baa5d73373ebc819d4"},
	}

	if exp, ok := expectedEntry[platformKey]; ok {
		require.Equal(t, exp.urlPrefix, url)
		require.Equal(t, exp.sha, sha)
	}
}

func TestResolveJdtlsPlatformURLNotFound(t *testing.T) {
	t.Parallel()

	_, _, ok := ResolveJdtlsPlatformURL("nonexistent-server")
	require.False(t, ok)
}

func TestDownloadVSIXSuccess(t *testing.T) {
	t.Parallel()

	data := []byte("test-vsix-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	}))
	defer server.Close()

	installer := &JdtlsInstaller{timeout: 10 * time.Second}
	result, err := installer.downloadVSIX(context.Background(), server.URL+"/test.vsix")
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func TestDownloadVSIXReadsFullBody(t *testing.T) {
	t.Parallel()

	largeData := bytes.Repeat([]byte("x"), 1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(largeData)
	}))
	defer server.Close()

	installer := &JdtlsInstaller{timeout: 30 * time.Second}
	result, err := installer.downloadVSIX(context.Background(), server.URL+"/test.vsix")
	require.NoError(t, err)
	require.Equal(t, len(largeData), len(result))
}

func TestDownloadVSIXCancelsOnContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block forever
		select {}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()

	installer := &JdtlsInstaller{timeout: 30 * time.Second}
	_, err := installer.downloadVSIX(ctx, server.URL+"/test.vsix")
	require.Error(t, err)
}

func TestResolveJREBundled(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	installDir := filepath.Join(cacheDir, "test", "1.0")
	jreDir := filepath.Join(installDir, "extension", "jre", "21.0.10-linux-x86_64", "bin")
	require.NoError(t, os.MkdirAll(jreDir, 0o755))

	javaName := "java"
	if runtime.GOOS == "windows" {
		javaName = "java.exe"
	}
	javaPath := filepath.Join(jreDir, javaName)
	require.NoError(t, os.WriteFile(javaPath, []byte("#!/bin/sh"), 0o755))

	installer := &JdtlsInstaller{cacheDir: cacheDir}
	result, err := installer.resolveJRE(installDir)
	require.NoError(t, err)
	require.Equal(t, javaPath, result)
}

func TestResolveJREPathFallback(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found on PATH")
	}

	cacheDir := t.TempDir()
	installDir := filepath.Join(cacheDir, "test", "1.0")

	installer := &JdtlsInstaller{cacheDir: cacheDir}
	result, err := installer.resolveJRE(installDir)
	require.NoError(t, err)

	expected, _ := exec.LookPath("java")
	require.Equal(t, expected, result)
}

func TestResolveJRENotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer t.Setenv("PATH", origPath)

	cacheDir := t.TempDir()
	installDir := filepath.Join(cacheDir, "test", "1.0")

	installer := &JdtlsInstaller{cacheDir: cacheDir}
	_, err := installer.resolveJRE(installDir)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRuntimeMissing)
}

func TestCacheArgsStoresCorrectArgs(t *testing.T) {
	t.Parallel()

	installer := &JdtlsInstaller{cacheDir: "/cache"}
	installer.cacheArgs("test-server", "/path/to/launcher.jar", "/ext", "/install")

	args := installer.GetJdtlsArgs("test-server")
	require.NotNil(t, args)

	found := false
	for i, a := range args {
		if a == "-jar" && i+1 < len(args) && args[i+1] == "/path/to/launcher.jar" {
			found = true
		}
	}
	require.True(t, found, "args should contain -jar /path/to/launcher.jar")

	dataDir := filepath.Join("/cache", "test-server", "workspace")
	foundData := false
	for _, a := range args {
		if a == dataDir {
			foundData = true
		}
	}
	require.True(t, foundData, "args should contain data dir")
}

func TestCacheArgsReturnsEmptyForUnknown(t *testing.T) {
	t.Parallel()

	installer := &JdtlsInstaller{}
	args := installer.GetJdtlsArgs("nonexistent")
	require.Nil(t, args)
}

func TestGetJdtlsArgsReturnsNilForUnknown(t *testing.T) {
	t.Parallel()

	installer := &JdtlsInstaller{cacheDir: "/cache"}
	args := installer.GetJdtlsArgs("nonexistent")
	require.Nil(t, args)
}
