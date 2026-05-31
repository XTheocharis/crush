package catalog

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	url, sha, ok := ResolveDownloadURL("marksman", "linux", "amd64")
	require.True(t, ok, "marksman linux/amd64 should resolve")
	require.NotEmpty(t, url, "URL should be non-empty")
	require.NotEmpty(t, sha, "SHA256 should be non-empty")
	require.Equal(t, "be5098e8213219269c47fc0d916a66fa31ce0602ec967475c722260aabf26087", sha)
}

func TestResolveDownloadURLNotFound(t *testing.T) {
	_, _, ok := ResolveDownloadURL("nonexistent-server-xyz", "linux", "amd64")
	require.False(t, ok, "unknown server should not resolve")
}

func TestResolveDownloadURLPlatformNotFound(t *testing.T) {
	_, _, ok := ResolveDownloadURL("marksman", "freebsd", "riscv64")
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
	url, _, ok := ResolveDownloadURL("marksman", runtime.GOOS, runtime.GOARCH)
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
