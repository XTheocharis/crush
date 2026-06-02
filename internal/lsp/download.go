package lsp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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
	"strings"
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
//
// The download type is determined by cfg.DownloadType:
//   - "" or "binary": bare binary (default).
//   - "gzip": gzip-compressed binary.
//   - "zip": zip archive containing the binary.
//   - "tar.gz": tar.gz archive containing the binary.
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

	downloadType := resolveDownloadType(cfg, resp)

	switch downloadType {
	case "gzip":
		tmpFile.Close()
		if err := decompressGzip(resp.Body, tmpPath); err != nil {
			return "", fmt.Errorf("lsp download: gzip decompression failed: %w", err)
		}
	case "zip":
		tmpFile.Close()
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("lsp download: failed to clean up temp file: %w", err)
		}
		if err := extractBinaryFromZip(resp.Body, name, tmpPath); err != nil {
			return "", fmt.Errorf("lsp download: zip extraction failed: %w", err)
		}
	case "tar.gz":
		tmpFile.Close()
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("lsp download: failed to clean up temp file: %w", err)
		}
		if err := extractBinaryFromTarGz(resp.Body, name, tmpPath); err != nil {
			return "", fmt.Errorf("lsp download: tar.gz extraction failed: %w", err)
		}
	default:
		if _, err := io.Copy(tmpFile, resp.Body); err != nil {
			tmpFile.Close()
			return "", fmt.Errorf("lsp download: failed to write %s: %w", name, err)
		}
		tmpFile.Close()
	}

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

// resolveDownloadType determines the download format from the config, HTTP
// response headers, or URL extension.
func resolveDownloadType(cfg config.AutoDownloadConfig, resp *http.Response) string {
	if cfg.DownloadType != "" {
		return cfg.DownloadType
	}
	if resp != nil && resp.Header.Get("Content-Encoding") == "gzip" {
		return "gzip"
	}
	lower := strings.ToLower(cfg.URL)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".gz") {
		return "gzip"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	return "binary"
}

// decompressGzip writes the gzip-compressed reader to dstPath.
func decompressGzip(r io.Reader, dstPath string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to open gzip reader: %w", err)
	}
	defer gz.Close()

	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, gz); err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	return nil
}

// extractBinaryFromZip reads a zip archive from r and extracts the binary
// matching serverName to dstPath.
func extractBinaryFromZip(r io.Reader, serverName, dstPath string) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read zip body: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}

	match := findBinaryInZip(zr.File, serverName)
	if match == nil {
		return fmt.Errorf("no binary matching %q found in zip archive", serverName)
	}

	rc, err := match.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip entry %q: %w", match.Name, err)
	}
	defer rc.Close()

	return writeFile(dstPath, rc)
}

// extractBinaryFromTarGz reads a tar.gz archive from r and extracts the binary
// matching serverName to dstPath.
func extractBinaryFromTarGz(r io.Reader, serverName, dstPath string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to open gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	type fileEntry struct {
		name    string
		content []byte
	}
	var files []fileEntry

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
		if err != nil {
			return fmt.Errorf("failed to read tar entry %q: %w", hdr.Name, err)
		}
		files = append(files, fileEntry{name: hdr.Name, content: data})
	}

	for _, f := range files {
		if archiveNameMatches(f.name, serverName) {
			return writeFile(dstPath, bytes.NewReader(f.content))
		}
	}

	for _, f := range files {
		return writeFile(dstPath, bytes.NewReader(f.content))
	}

	return fmt.Errorf("no binary matching %q found in tar.gz archive", serverName)
}

// findArchiveBinary finds the best matching file in a zip archive.
func findBinaryInZip(files []*zip.File, serverName string) *zip.File {
	for _, f := range files {
		if archiveNameMatches(f.Name, serverName) {
			return f
		}
	}
	for _, f := range files {
		if !f.FileInfo().IsDir() {
			return f
		}
	}
	return nil
}

// archiveNameMatches checks if an archive entry name matches the server name.
// It handles paths like "server-v1.0/server" or "server-linux-amd64/server".
func archiveNameMatches(entryPath, serverName string) bool {
	base := filepath.Base(entryPath)
	return base == serverName
}

func writeFile(dstPath string, r io.Reader) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
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
