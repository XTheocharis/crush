package lsp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAutoDownloadSuccess(t *testing.T) {
	t.Parallel()

	content := []byte("#!/bin/sh\necho hello\n")
	hash := sha256.Sum256(content)
	sha := fmt.Sprintf("%x", hash)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/gopls",
		SHA256: sha,
	}
	path, err := d.Download(context.Background(), "gopls", cfg)
	require.NoError(t, err)
	require.FileExists(t, path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestAutoDownloadCachedNoReDownload(t *testing.T) {
	t.Parallel()

	content := []byte("binary-content-v1\n")
	hash := sha256.Sum256(content)
	sha := fmt.Sprintf("%x", hash)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(content)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/tool",
		SHA256: sha,
	}

	// First download.
	path1, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// Second download should be cached.
	path2, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.Equal(t, 1, calls, "should not re-download cached binary")
	require.Equal(t, path1, path2)
}

func TestAutoDownloadSHA256Mismatch(t *testing.T) {
	t.Parallel()

	content := []byte("some binary\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/tool",
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	_, err := d.Download(context.Background(), "tool", cfg)
	require.ErrorIs(t, err, ErrHashMismatch)
}

func TestAutoDownloadNoURLSkips(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, nil)

	cfg := config.AutoDownloadConfig{}
	path, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.Empty(t, path, "no URL should return empty path")
}

func TestAutoDownloadContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never responds.
		select {}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/slow",
		SHA256: "deadbeef",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.Download(ctx, "slow", cfg)
	require.Error(t, err)
}

func TestAutoDownloadCorruptedCacheRedownloads(t *testing.T) {
	t.Parallel()

	content := []byte("good-binary\n")
	hash := sha256.Sum256(content)
	sha := fmt.Sprintf("%x", hash)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(content)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/tool",
		SHA256: sha,
	}

	// First download succeeds.
	path, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// Corrupt the cached file.
	require.NoError(t, os.WriteFile(path, []byte("corrupted"), 0o755))

	// Second download should detect corruption and re-download.
	path2, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.Equal(t, 2, calls, "should re-download corrupted cache")

	got, err := os.ReadFile(path2)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestAutoDownloadServerUnreachable(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	// Use a client that hits a closed server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/tool",
		SHA256: "abc",
	}
	_, err := d.Download(context.Background(), "tool", cfg)
	require.Error(t, err)
}

func TestAutoDownloadCachePath(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, nil)

	path := d.cachePath("gopls")
	require.Equal(t, filepath.Join(cacheDir, "gopls"), path)
}

func TestLSPCacheDir(t *testing.T) {
	t.Setenv("CRUSH_CACHE_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	dir := LSPCacheDir()
	require.Contains(t, dir, "crush")
	require.Contains(t, dir, "lsps")
}

func fakeBinary() []byte {
	return []byte("#!/bin/sh\necho hello\n")
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

func TestDownloadGzip(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err := gw.Write(bin)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	gzData := gzBuf.Bytes()
	sha := sha256Hex(bin)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(gzData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/tool.gz",
		SHA256:       sha,
		DownloadType: "gzip",
	}
	path, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)
	require.FileExists(t, path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o111, "binary should be executable")
}

func TestDownloadGzipContentEncoding(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err := gw.Write(bin)
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	gzData := gzBuf.Bytes()
	sha := sha256Hex(bin)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(gzData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/tool",
		SHA256: sha,
	}
	path, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)
}

func TestDownloadZip(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()
	sha := sha256Hex(bin)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("my-server")
	require.NoError(t, err)
	_, err = w.Write(bin)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	zipData := zipBuf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/tool.zip",
		SHA256:       sha,
		DownloadType: "zip",
	}
	path, err := d.Download(context.Background(), "my-server", cfg)
	require.NoError(t, err)
	require.FileExists(t, path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o111, "binary should be executable")
}

func TestDownloadZipWithDirectoryPrefix(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()
	sha := sha256Hex(bin)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("tool-v1.0/tool")
	require.NoError(t, err)
	_, err = w.Write(bin)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	zipData := zipBuf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/tool.zip",
		SHA256:       sha,
		DownloadType: "zip",
	}
	path, err := d.Download(context.Background(), "tool", cfg)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)
}

func TestDownloadTarGz(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()
	sha := sha256Hex(bin)

	var tarGzBuf bytes.Buffer
	gw := gzip.NewWriter(&tarGzBuf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "my-server",
		Mode: 0o755,
		Size: int64(len(bin)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(bin)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	tarGzData := tarGzBuf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarGzData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/tool.tar.gz",
		SHA256:       sha,
		DownloadType: "tar.gz",
	}
	path, err := d.Download(context.Background(), "my-server", cfg)
	require.NoError(t, err)
	require.FileExists(t, path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o111, "binary should be executable")
}

func TestDownloadTarGzWithDirectoryPrefix(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()
	sha := sha256Hex(bin)

	var tarGzBuf bytes.Buffer
	gw := gzip.NewWriter(&tarGzBuf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "server-v2.0.0/server",
		Mode: 0o755,
		Size: int64(len(bin)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(bin)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	tarGzData := tarGzBuf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarGzData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/server.tar.gz",
		SHA256:       sha,
		DownloadType: "tar.gz",
	}
	path, err := d.Download(context.Background(), "server", cfg)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)
}

func TestDownloadPlainBinaryStillWorks(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()
	sha := sha256Hex(bin)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bin)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:    srv.URL + "/binary",
		SHA256: sha,
	}
	path, err := d.Download(context.Background(), "binary", cfg)
	require.NoError(t, err)
	require.FileExists(t, path)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&0o111, "binary should be executable")
}

func TestDownloadCorruptGzip(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not valid gzip data"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/bad.gz",
		DownloadType: "gzip",
	}
	_, err := d.Download(context.Background(), "bad", cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "gzip")
}

func TestDownloadCorruptZip(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not valid zip data"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/bad.zip",
		DownloadType: "zip",
	}
	_, err := d.Download(context.Background(), "bad", cfg)
	require.Error(t, err)
}

func TestDownloadCorruptTarGz(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte("not a valid tar"))
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/bad.tar.gz",
		DownloadType: "tar.gz",
	}
	_, err = d.Download(context.Background(), "bad", cfg)
	require.Error(t, err)
}

func TestDownloadTypeAutoDetectFromURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "gzip", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool.gz"}, nil,
	))
	require.Equal(t, "tar.gz", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool.tar.gz"}, nil,
	))
	require.Equal(t, "tar.gz", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool.tgz"}, nil,
	))
	require.Equal(t, "zip", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool.zip"}, nil,
	))
	require.Equal(t, "binary", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool"}, nil,
	))
}

func TestDownloadTypeExplicitOverridesAutoDetect(t *testing.T) {
	t.Parallel()

	require.Equal(t, "gzip", resolveDownloadType(
		config.AutoDownloadConfig{URL: "http://example.com/tool.zip", DownloadType: "gzip"}, nil,
	))
}

func TestDownloadZipNoMatchingBinary(t *testing.T) {
	t.Parallel()

	bin := fakeBinary()

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("other-binary")
	require.NoError(t, err)
	_, err = io.Copy(w, bytes.NewReader(bin))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBuf.Bytes())
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d := NewDownloader(cacheDir, srv.Client())

	cfg := config.AutoDownloadConfig{
		URL:          srv.URL + "/tool.zip",
		DownloadType: "zip",
	}
	path, err := d.Download(context.Background(), "nonexistent", cfg)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, bin, got, "should fall back to first file in archive")
}
