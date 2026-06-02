// Package catalog provides an embedded LSP server download catalog.
// It embeds catalog.json at compile time and offers lookup helpers for
// resolving platform-specific binary download URLs.
package catalog

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
)

//go:embed catalog.json
var catalogJSON []byte

// PlatformEntry holds the download URL and SHA256 hash for a single
// os/arch combination.
type PlatformEntry struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	DownloadType string `json:"download_type,omitempty"` // "binary" (default), "gzip", "zip", "tar.gz"
}

// ServerEntry describes a downloadable LSP server, including its version
// and per-platform binary details.
type ServerEntry struct {
	Version   string                   `json:"version"`
	Platforms map[string]PlatformEntry `json:"platforms"` // key: "os/arch"
}

var (
	once    sync.Once
	servers map[string]ServerEntry
	loadErr error
)

// load parses the embedded catalog JSON exactly once. Meta keys starting
// with "_" are silently discarded.
func load() {
	once.Do(func() {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(catalogJSON, &raw); err != nil {
			loadErr = err
			slog.Warn("Failed to parse LSP catalog", "error", err)
			return
		}

		servers = make(map[string]ServerEntry, len(raw))
		for key, val := range raw {
			// Skip meta/annotation keys.
			if strings.HasPrefix(key, "_") {
				continue
			}
			var entry ServerEntry
			if err := json.Unmarshal(val, &entry); err != nil {
				slog.Warn("Failed to parse catalog entry", "server", key, "error", err)
				continue
			}
			servers[key] = entry
		}
	})
}

// Lookup returns the catalog entry for the given server name.
// The boolean is false when the server is not present in the catalog.
func Lookup(serverName string) (ServerEntry, bool) {
	load()
	entry, ok := servers[serverName]
	return entry, ok
}

// ResolveDownloadURL returns the platform-specific download URL, SHA256, and
// download type for the given server, OS, and architecture. The platform key
// format is "os/arch" (e.g. "linux/amd64", "darwin/arm64").
// Returns ("", "", "", false) when the server or platform is not found.
func ResolveDownloadURL(serverName, goos, goarch string) (url, sha256, downloadType string, ok bool) {
	load()
	entry, found := servers[serverName]
	if !found {
		return "", "", "", false
	}
	platformKey := goos + "/" + goarch
	pe, found := entry.Platforms[platformKey]
	if !found {
		return "", "", "", false
	}
	return pe.URL, pe.SHA256, pe.DownloadType, true
}

// AllServers returns the full catalog map. The returned map must not be
// modified by callers.
func AllServers() map[string]ServerEntry {
	load()
	return servers
}
