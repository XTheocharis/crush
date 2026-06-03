package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
)

// Sentinel errors for install operations.
var (
	ErrRuntimeMissing = errors.New("runtime dependency not available")
	ErrInstallTimeout = errors.New("install timed out")
	ErrInstallFailed  = errors.New("install failed")
)

// Installer is the interface for LSP server installers.
type Installer interface {
	// Install provisions the LSP server and returns the absolute path to
	// the server binary. If already installed (idempotent), returns the
	// cached path.
	Install(ctx context.Context, name string, cfg catalog.InstallConfig) (string, error)
}

// NpmInstaller installs LSP servers via npm.
type NpmInstaller struct {
	cacheDir string        // Base directory for installations.
	timeout  time.Duration // Per-install timeout (default 120s).
	mu       sync.Map      // Keyed by "name/version" for concurrent install protection.
}

// NewNpmInstaller creates a new npm-based installer.
func NewNpmInstaller(cacheDir string) *NpmInstaller {
	return &NpmInstaller{
		cacheDir: cacheDir,
		timeout:  120 * time.Second,
	}
}

// Install provisions an npm-based LSP server.
func (n *NpmInstaller) Install(ctx context.Context, name string, cfg catalog.InstallConfig) (string, error) {
	// 1. Check runtime dependencies.
	if !IsRuntimeAvailable("node") || !IsRuntimeAvailable("npm") {
		return "", fmt.Errorf("npm install %s: %w", name, ErrRuntimeMissing)
	}

	// 2. Compute install directory and entrypoint path.
	installDir := filepath.Join(n.cacheDir, name, cfg.Version)
	entrypointPath := n.entrypointPath(installDir, cfg.Entrypoint)

	// 3. Fast path: check if already installed (idempotency).
	if info, err := os.Stat(entrypointPath); err == nil && info.Mode().IsRegular() {
		return entrypointPath, nil
	}

	// 4. Acquire per-package lock for concurrent install protection.
	key := name + "/" + cfg.Version
	muIface, _ := n.mu.LoadOrStore(key, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	n.mu.Delete(key) // Clean up lock entry after use.

	// Double-check after acquiring lock (another goroutine may have
	// completed the install while we waited).
	if info, err := os.Stat(entrypointPath); err == nil && info.Mode().IsRegular() {
		return entrypointPath, nil
	}

	// 5. Create install directory.
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("npm install %s: mkdir: %w", name, err)
	}

	// 6. Run npm install with timeout.
	installCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	pkgSpec := cfg.Package + "@" + cfg.Version
	cmd := exec.CommandContext(
		installCtx, "npm", "install",
		"--ignore-scripts", "--prefix", installDir, pkgSpec,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Clean up partial state on failure.
		os.RemoveAll(installDir)
		if installCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("npm install %s: %w", name, ErrInstallTimeout)
		}
		return "", fmt.Errorf("npm install %s: %w: %s", name, ErrInstallFailed, string(output))
	}

	// 7. Verify entrypoint exists after install.
	if _, err := os.Stat(entrypointPath); err != nil {
		os.RemoveAll(installDir)
		return "", fmt.Errorf(
			"npm install %s: entrypoint not found at %s: %w",
			name, entrypointPath, ErrInstallFailed,
		)
	}

	return entrypointPath, nil
}

// entrypointPath returns the platform-specific path to the entrypoint binary.
func (n *NpmInstaller) entrypointPath(installDir, entrypoint string) string {
	p := filepath.Join(installDir, "node_modules", ".bin", entrypoint)
	if runtime.GOOS == "windows" {
		p += ".cmd"
	}
	return p
}
