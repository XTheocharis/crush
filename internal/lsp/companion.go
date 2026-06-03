package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/lsp/catalog"
)

// CompanionPaths holds all resolved paths needed for companion LSP initialization.
type CompanionPaths struct {
	PrimaryBinary   string // Main LSP binary (e.g. svelteserver).
	CompanionBinary string // TS language server binary.
	PluginPath      string // TS plugin directory (e.g. typescript-svelte-plugin).
	TSDKPath        string // TypeScript SDK lib directory.
}

// CompanionInstaller installs multiple npm packages into one directory for
// companion LSP servers (e.g. Svelte + TypeScript, Vue + TypeScript).
type CompanionInstaller struct {
	cacheDir string
	timeout  time.Duration
	mu       sync.Map
}

// NewCompanionInstaller creates a new companion installer.
func NewCompanionInstaller(cacheDir string) *CompanionInstaller {
	return &CompanionInstaller{
		cacheDir: cacheDir,
		timeout:  120 * time.Second,
	}
}

// Install installs the primary package and all companion packages into one
// directory. It implements the Installer interface.
func (c *CompanionInstaller) Install(ctx context.Context, name string, cfg catalog.InstallConfig) (string, error) {
	// 1. Runtime check.
	if !IsRuntimeAvailable("node") {
		return "", fmt.Errorf("companion install %s: %w", name, ErrRuntimeMissing)
	}
	if !IsRuntimeAvailable("npm") {
		return "", fmt.Errorf("companion install %s: %w", name, ErrRuntimeMissing)
	}

	// 2. Compute install dir.
	installDir := filepath.Join(c.cacheDir, name, cfg.Version)

	// 3. Build expected version string (primary_ver+ts_ver+ts_ls_ver+plugin_ver).
	expectedVersion := buildCompanionVersionString(cfg)

	// 4. Idempotency: check .installed_version file.
	entrypoint := cfg.Entrypoint
	if runtime.GOOS == "windows" {
		entrypoint += ".cmd"
	}
	binaryPath := filepath.Join(installDir, "node_modules", ".bin", entrypoint)

	versionFile := filepath.Join(installDir, ".installed_version")
	if existing, err := os.ReadFile(versionFile); err == nil {
		if strings.TrimSpace(string(existing)) == expectedVersion {
			if _, err := os.Stat(binaryPath); err == nil {
				return binaryPath, nil
			}
		}
	}

	// 5. Concurrent install protection.
	key := name + "/" + cfg.Version
	muIface, _ := c.mu.LoadOrStore(key, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	c.mu.Delete(key)

	// Double-check after acquiring lock.
	if existing, err := os.ReadFile(versionFile); err == nil {
		if strings.TrimSpace(string(existing)) == expectedVersion {
			if _, err := os.Stat(binaryPath); err == nil {
				return binaryPath, nil
			}
		}
	}

	// 6. Create install dir.
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return "", fmt.Errorf("companion install %s: %w", name, err)
	}

	// 7. Build npm install command with ALL packages.
	args := []string{"install", "--ignore-scripts", "--prefix", installDir}

	// Primary package.
	args = append(args, fmt.Sprintf("%s@%s", cfg.Package, cfg.Version))

	// Companion packages (already in "name@version" format from catalog).
	for _, pkg := range cfg.CompanionPackages {
		args = append(args, pkg)
	}

	// 8. Run npm install with timeout.
	installCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(installCtx, "npm", args...)
	cmd.Dir = installDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(installDir)
		if installCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("companion install %s: %w", name, ErrInstallTimeout)
		}
		return "", fmt.Errorf("companion install %s: %w: %s", name, ErrInstallFailed, string(output))
	}

	// 9. Verify primary entrypoint exists after install.
	if _, err := os.Stat(binaryPath); err != nil {
		os.RemoveAll(installDir)
		return "", fmt.Errorf(
			"companion install %s: entrypoint not found at %s: %w",
			name, binaryPath, ErrInstallFailed,
		)
	}

	// 10. Write .installed_version file.
	if err := os.WriteFile(versionFile, []byte(expectedVersion), 0o644); err != nil {
		return "", fmt.Errorf("companion install %s: failed to write version file: %w", name, err)
	}

	return binaryPath, nil
}

// ResolveCompanionPaths returns all resolved paths needed for LSP initialization.
func (c *CompanionInstaller) ResolveCompanionPaths(installDir string, cfg catalog.InstallConfig) CompanionPaths {
	entrypoint := cfg.Entrypoint
	if runtime.GOOS == "windows" {
		entrypoint += ".cmd"
	}

	paths := CompanionPaths{
		PrimaryBinary:   filepath.Join(installDir, "node_modules", ".bin", entrypoint),
		CompanionBinary: filepath.Join(installDir, "node_modules", ".bin", "typescript-language-server"),
		TSDKPath:        filepath.Join(installDir, "node_modules", "typescript", "lib"),
	}

	// Plugin path varies by server.
	if cfg.Package == "svelte-language-server" {
		paths.PluginPath = filepath.Join(installDir, "node_modules", "typescript-svelte-plugin")
	} else if cfg.Package == "@vue/language-server" {
		paths.PluginPath = filepath.Join(installDir, "node_modules", "@vue", "typescript-plugin")
	}

	return paths
}

// buildCompanionVersionString creates a version string encoding all component
// versions.
func buildCompanionVersionString(cfg catalog.InstallConfig) string {
	parts := []string{cfg.Version}
	for _, pkg := range cfg.CompanionPackages {
		parts = append(parts, pkg)
	}
	return strings.Join(parts, "_")
}
