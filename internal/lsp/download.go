package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/charmbracelet/crush/internal/config"
)

// ErrHashMismatch is returned when the downloaded file's SHA256 does not match
// the expected hash.
var ErrHashMismatch = errors.New("lsp download: SHA256 hash mismatch")

// Downloader handles downloading and caching LSP server binaries.
type Downloader struct {
	cacheDir string
	client   *http.Client
}

// NewDownloader creates a Downloader that stores binaries in cacheDir.
// If client is nil, a default client with a 60-second timeout is used.
func NewDownloader(cacheDir string, client *http.Client) *Downloader {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Downloader{cacheDir: cacheDir, client: client}
}

// Download fetches the binary for the named LSP server according to cfg.
// If cfg.URL is empty, it returns ("", nil) (skip download).
// If a cached file with matching SHA256 already exists, it is returned without
// re-downloading. Otherwise the file is fetched, verified, and cached.
func (d *Downloader) Download(ctx context.Context, name string, cfg config.AutoDownloadConfig) (string, error) {
	if cfg.URL == "" {
		return "", nil
	}

	cachePath := d.cachePath(name)

	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		if verifySHA256(cachePath, cfg.SHA256) == nil {
			slog.Debug("Using cached LSP binary", "name", name, "path", cachePath)
			return cachePath, nil
		}
		slog.Debug("Cached LSP binary hash mismatch, re-downloading", "name", name)
	}

	if err := os.MkdirAll(d.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("lsp download: failed to create cache directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(d.cacheDir, name+".download-*")
	if err != nil {
		return "", fmt.Errorf("lsp download: failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("lsp download: failed to create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("lsp download: failed to fetch %s: %w", cfg.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return "", fmt.Errorf("lsp download: unexpected status %d for %s", resp.StatusCode, cfg.URL)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("lsp download: failed to write %s: %w", name, err)
	}
	tmpFile.Close()

	if err := verifySHA256(tmpPath, cfg.SHA256); err != nil {
		return "", err
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", fmt.Errorf("lsp download: failed to chmod %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		return "", fmt.Errorf("lsp download: failed to cache %s: %w", name, err)
	}

	slog.Debug("Downloaded and cached LSP binary", "name", name, "path", cachePath)
	return cachePath, nil
}

func (d *Downloader) cachePath(name string) string {
	return filepath.Join(d.cacheDir, name)
}

func verifySHA256(path, expectedHex string) error {
	if expectedHex == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("lsp download: failed to open for hash check: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("lsp download: failed to hash %s: %w", path, err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHex {
		return fmt.Errorf("%w: expected %s, got %s", ErrHashMismatch, expectedHex, got)
	}
	return nil
}

// LSPCacheDir returns the cache directory for LSP binaries, respecting
// CRUSH_CACHE_DIR and XDG_CACHE_HOME.
func LSPCacheDir() string {
	return filepath.Join(config.GlobalCacheDir(), "lsps")
}

// DefaultDownloader creates a Downloader using the standard LSP cache
// directory.
func DefaultDownloader() *Downloader {
	return NewDownloader(LSPCacheDir(), nil)
}

// ResolveDownloadPath checks whether an LSP server binary needs to be
// auto-downloaded. If cfg has a download URL, it runs the download and returns
// the cached binary path. Otherwise it returns the original command.
func ResolveDownloadPath(ctx context.Context, name, command string, cfg config.AutoDownloadConfig) (string, error) {
	if cfg.URL == "" {
		return command, nil
	}

	d := DefaultDownloader()
	path, err := d.Download(ctx, name, cfg)
	if err != nil {
		return "", err
	}
	if path == "" {
		return command, nil
	}
	return path, nil
}

// PlatformArchiveName returns a filename like "gopls-linux-amd64" suitable for
// constructing platform-specific download URLs.
func PlatformArchiveName(name string) string {
	return fmt.Sprintf("%s-%s-%s", name, runtime.GOOS, runtime.GOARCH)
}
