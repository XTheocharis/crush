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

	// Install method fields for non-binary LSP servers.
	InstallMethod     string         `json:"install_method,omitempty"`     // "binary" (default), "npm", "pip", "path", "companion", "jdtls"
	InstallPackage    string         `json:"install_package,omitempty"`    // npm/pip package name
	InstallVersion    string         `json:"install_version,omitempty"`    // pinned version
	InstallEntrypoint string         `json:"install_entrypoint,omitempty"` // binary name produced
	MultiBinary       bool           `json:"multi_binary,omitempty"`       // one install → multiple binaries
	RuntimeDep        string         `json:"runtime_dep,omitempty"`        // "node", "python", "go", "jvm", ""
	CompanionPackages []string       `json:"companion_packages,omitempty"` // additional npm packages
	CompanionServer   string         `json:"companion_server,omitempty"`   // companion TS server name
	InitOptions       map[string]any `json:"init_options,omitempty"`       // LSP initializationOptions
	VsixURL           string         `json:"vsix_url,omitempty"`           // VSIX download URL
	VsixSHA256        string         `json:"vsix_sha256,omitempty"`        // SHA256 of VSIX
}

// InstallConfig holds the resolved install configuration for a non-binary
// LSP server.
type InstallConfig struct {
	Method            string
	Package           string
	Version           string
	Entrypoint        string
	RuntimeDep        string
	MultiBinary       bool
	CompanionPackages []string
	CompanionServer   string
	InitOptions       map[string]any
	VsixURL           string
	VsixSHA256        string
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

// ResolveInstallMethod returns the install configuration for servers that
// use a non-binary install method (e.g. npm, pip, path). Returns
// (InstallConfig{}, false) for binary-only servers or unknown names.
func ResolveInstallMethod(name string) (InstallConfig, bool) {
	load()
	entry, ok := servers[name]
	if !ok || entry.InstallMethod == "" || entry.InstallMethod == "binary" {
		return InstallConfig{}, false
	}
	return InstallConfig{
		Method:            entry.InstallMethod,
		Package:           entry.InstallPackage,
		Version:           entry.InstallVersion,
		Entrypoint:        entry.InstallEntrypoint,
		RuntimeDep:        entry.RuntimeDep,
		MultiBinary:       entry.MultiBinary,
		CompanionPackages: entry.CompanionPackages,
		CompanionServer:   entry.CompanionServer,
		InitOptions:       entry.InitOptions,
		VsixURL:           entry.VsixURL,
		VsixSHA256:        entry.VsixSHA256,
	}, true
}
