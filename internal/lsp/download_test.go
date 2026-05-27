package lsp

import (
	"context"
	"crypto/sha256"
	"fmt"
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
