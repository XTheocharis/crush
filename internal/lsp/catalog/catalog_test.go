package catalog

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveInstallMethodBinaryServer(t *testing.T) {
	_, ok := ResolveInstallMethod("marksman")
	require.False(t, ok, "binary-only servers should return false")
}

func TestResolveInstallMethodUnknown(t *testing.T) {
	_, ok := ResolveInstallMethod("nonexistent-server-xyz")
	require.False(t, ok, "unknown server should return false")
}

func TestResolveInstallMethodNPMServer(t *testing.T) {
	original := servers
	servers = map[string]ServerEntry{
		"test-npm-lsp": {
			Version:           "1.0.0",
			InstallMethod:     "npm",
			InstallPackage:    "typescript-language-server",
			InstallVersion:    "4.3.2",
			InstallEntrypoint: "typescript-language-server",
			RuntimeDep:        "node",
		},
	}
	defer func() { servers = original }()

	cfg, ok := ResolveInstallMethod("test-npm-lsp")
	require.True(t, ok)
	require.Equal(t, "npm", cfg.Method)
	require.Equal(t, "typescript-language-server", cfg.Package)
	require.Equal(t, "4.3.2", cfg.Version)
	require.Equal(t, "typescript-language-server", cfg.Entrypoint)
	require.Equal(t, "node", cfg.RuntimeDep)
}

func TestResolveInstallMethodPIPServer(t *testing.T) {
	original := servers
	servers = map[string]ServerEntry{
		"test-pip-lsp": {
			Version:        "2.0.0",
			InstallMethod:  "pip",
			InstallPackage: "python-lsp-server",
			RuntimeDep:     "python",
		},
	}
	defer func() { servers = original }()

	cfg, ok := ResolveInstallMethod("test-pip-lsp")
	require.True(t, ok)
	require.Equal(t, "pip", cfg.Method)
	require.Equal(t, "python-lsp-server", cfg.Package)
	require.Equal(t, "python", cfg.RuntimeDep)
}

func TestResolveInstallMethodPATHServer(t *testing.T) {
	original := servers
	servers = map[string]ServerEntry{
		"test-path-lsp": {
			Version:           "3.0.0",
			InstallMethod:     "path",
			InstallEntrypoint: "custom-lsp-binary",
			InitOptions: map[string]any{
				"highlight": map[string]any{"enable": true},
			},
		},
	}
	defer func() { servers = original }()

	cfg, ok := ResolveInstallMethod("test-path-lsp")
	require.True(t, ok)
	require.Equal(t, "path", cfg.Method)
	require.Equal(t, "custom-lsp-binary", cfg.Entrypoint)
	require.Empty(t, cfg.Package, "path method should have empty package")
	require.Equal(t, map[string]any{"highlight": map[string]any{"enable": true}}, cfg.InitOptions)
}

func TestResolveInstallMethodBinaryExplicitReturnsFalse(t *testing.T) {
	original := servers
	servers = map[string]ServerEntry{
		"test-explicit-binary": {
			Version:       "1.0.0",
			InstallMethod: "binary",
		},
	}
	defer func() { servers = original }()

	_, ok := ResolveInstallMethod("test-explicit-binary")
	require.False(t, ok, "explicit 'binary' install_method should return false")
}

func TestLookup(t *testing.T) {
	entry, ok := Lookup("marksman")
	require.True(t, ok, "marksman should be in the catalog")
	require.NotEmpty(t, entry.Version, "version should be set")
	require.NotEmpty(t, entry.Platforms, "platforms should be set")
}

func TestLookupNotFound(t *testing.T) {
	_, ok := Lookup("nonexistent-server-xyz")
	require.False(t, ok, "unknown server should not be found")
}

func TestResolveDownloadURL(t *testing.T) {
	url, sha, _, ok := ResolveDownloadURL("marksman", "linux", "amd64")
	require.True(t, ok, "marksman linux/amd64 should resolve")
	require.NotEmpty(t, url, "URL should be non-empty")
	require.NotEmpty(t, sha, "SHA256 should be non-empty")
	require.Equal(t, "be5098e8213219269c47fc0d916a66fa31ce0602ec967475c722260aabf26087", sha)
}

func TestResolveDownloadURLNotFound(t *testing.T) {
	_, _, _, ok := ResolveDownloadURL("nonexistent-server-xyz", "linux", "amd64")
	require.False(t, ok, "unknown server should not resolve")
}

func TestResolveDownloadURLPlatformNotFound(t *testing.T) {
	_, _, _, ok := ResolveDownloadURL("marksman", "freebsd", "riscv64")
	require.False(t, ok, "unsupported platform should not resolve")
}

func TestAllServers(t *testing.T) {
	all := AllServers()
	require.NotEmpty(t, all, "catalog should not be empty")
}

func TestIgnoreMetaKeys(t *testing.T) {
	all := AllServers()
	_, hasComment := all["_comment"]
	require.False(t, hasComment, "_comment should not be in AllServers")
	_, hasSkipped := all["_skipped_servers"]
	require.False(t, hasSkipped, "_skipped_servers should not be in AllServers")
}

func TestCurrentPlatform(t *testing.T) {
	url, _, _, ok := ResolveDownloadURL("marksman", runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Logf("marksman not available for %s/%s (expected on some platforms)", runtime.GOOS, runtime.GOARCH)
		return
	}
	require.NotEmpty(t, url)
}

func TestMultipleServersResolve(t *testing.T) {
	servers := []string{"marksman", "opa", "cosign", "hadolint"}
	for _, name := range servers {
		entry, ok := Lookup(name)
		require.True(t, ok, "%s should be in catalog", name)
		require.NotEmpty(t, entry.Version, "%s should have a version", name)
		require.NotEmpty(t, entry.Platforms, "%s should have platforms", name)
	}
}
