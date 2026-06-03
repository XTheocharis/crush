package lsp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
)

// JdtlsInstaller installs Eclipse JDT Language Server via VSIX download.
type JdtlsInstaller struct {
	cacheDir  string
	timeout   time.Duration
	mu        sync.Map
	argsCache sync.Map
}

// NewJdtlsInstaller creates a new JDTLS installer.
func NewJdtlsInstaller(cacheDir string) *JdtlsInstaller {
	return &JdtlsInstaller{
		cacheDir: cacheDir,
		timeout:  300 * time.Second,
	}
}

// GetJdtlsArgs returns the computed JDTLS launch args for the given server name.
func (j *JdtlsInstaller) GetJdtlsArgs(name string) []string {
	if v, ok := j.argsCache.Load(name); ok {
		return v.([]string)
	}
	return nil
}

// Install downloads and extracts the JDTLS VSIX, returning the JRE binary path.
func (j *JdtlsInstaller) Install(ctx context.Context, name string, cfg catalog.InstallConfig) (string, error) {
	if !IsRuntimeAvailable("jvm") {
		return "", fmt.Errorf("jdtls install %s: %w", name, ErrRuntimeMissing)
	}

	installDir := filepath.Join(j.cacheDir, name, cfg.Version)
	extDir := filepath.Join(installDir, "extension", "server")

	launcherPattern := filepath.Join(extDir, "plugins", "org.eclipse.equinox.launcher_*.jar")
	if matches, _ := filepath.Glob(launcherPattern); len(matches) > 0 {
		jrePath, err := j.resolveJRE(installDir)
		if err != nil {
			return "", fmt.Errorf("jdtls install %s: %w", name, err)
		}
		j.cacheArgs(name, matches[0], extDir, installDir)
		return jrePath, nil
	}

	key := name + "/" + cfg.Version
	muIface, _ := j.mu.LoadOrStore(key, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	j.mu.Delete(key)

	if matches, _ := filepath.Glob(launcherPattern); len(matches) > 0 {
		jrePath, err := j.resolveJRE(installDir)
		if err != nil {
			return "", fmt.Errorf("jdtls install %s: %w", name, err)
		}
		j.cacheArgs(name, matches[0], extDir, installDir)
		return jrePath, nil
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("jdtls install %s: mkdir: %w", name, err)
	}

	if cfg.VsixURL == "" {
		return "", fmt.Errorf("jdtls install %s: no VSIX URL configured", name)
	}

	vsixData, err := j.downloadVSIX(ctx, cfg.VsixURL)
	if err != nil {
		os.RemoveAll(installDir)
		return "", fmt.Errorf("jdtls install %s: download: %w", name, err)
	}

	if cfg.VsixSHA256 != "" {
		h := sha256.Sum256(vsixData)
		got := hex.EncodeToString(h[:])
		if got != cfg.VsixSHA256 {
			os.RemoveAll(installDir)
			return "", fmt.Errorf("jdtls install %s: SHA256 mismatch: expected %s, got %s", name, cfg.VsixSHA256, got)
		}
	}

	if err := extractVSIX(vsixData, installDir); err != nil {
		os.RemoveAll(installDir)
		return "", fmt.Errorf("jdtls install %s: extract: %w", name, err)
	}

	matches, err := filepath.Glob(launcherPattern)
	if err != nil || len(matches) == 0 {
		os.RemoveAll(installDir)
		return "", fmt.Errorf("jdtls install %s: launcher JAR not found: %w", name, ErrInstallFailed)
	}

	jrePath, err := j.resolveJRE(installDir)
	if err != nil {
		return "", fmt.Errorf("jdtls install %s: %w", name, err)
	}

	j.cacheArgs(name, matches[0], extDir, installDir)
	return jrePath, nil
}

func (j *JdtlsInstaller) downloadVSIX(ctx context.Context, url string) ([]byte, error) {
	dlCtx, cancel := context.WithTimeout(ctx, j.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

func (j *JdtlsInstaller) resolveJRE(installDir string) (string, error) {
	javaName := "java"
	if runtime.GOOS == "windows" {
		javaName = "java.exe"
	}

	bundledPattern := filepath.Join(installDir, "extension", "jre", "*", "bin", javaName)
	if matches, _ := filepath.Glob(bundledPattern); len(matches) > 0 {
		return matches[0], nil
	}

	p, err := exec.LookPath("java")
	if err != nil {
		return "", fmt.Errorf("no JRE found: %w", ErrRuntimeMissing)
	}
	return p, nil
}

func (j *JdtlsInstaller) cacheArgs(name, launcherJAR, extDir, installDir string) {
	configDir := jdtlsConfigDir(extDir)
	dataDir := filepath.Join(j.cacheDir, name, "workspace")

	args := []string{
		"-Declipse.application=org.eclipse.jdt.ls.core.id1",
		"-Dosgi.bundles.defaultStartLevel=4",
		"-Declipse.product=org.eclipse.jdt.ls.core.product",
		"-Dlog.level=ALL",
		"-Xmx1G",
		"--add-modules=ALL-SYSTEM",
		"--add-opens", "java.base/java.util=ALL-UNNAMED",
		"--add-opens", "java.base/java.lang=ALL-UNNAMED",
		"-jar", launcherJAR,
		"-configuration", configDir,
		"-data", dataDir,
	}
	j.argsCache.Store(name, args)
}

func jdtlsConfigDir(extDir string) string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
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

func extractVSIX(data []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)

		// Guard against zip slip: reject entries that escape the target directory.
		if !strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), cleanDest) && filepath.Clean(path) != filepath.Clean(dest) {
			return fmt.Errorf("zip entry %q escapes target directory", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", path, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(path), err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}

		outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %s: %w", path, err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}

// ResolveJdtlsPlatformURL looks up the platform-specific VSIX URL and SHA256
// from the catalog platforms map and returns them in an InstallConfig.
func ResolveJdtlsPlatformURL(name string) (url, sha256 string, ok bool) {
	entry, found := catalog.Lookup(name)
	if !found {
		return "", "", false
	}

	platformKey := runtime.GOOS + "/" + runtime.GOARCH
	pe, found := entry.Platforms[platformKey]
	if !found {
		return "", "", false
	}
	return pe.URL, pe.SHA256, true
}
