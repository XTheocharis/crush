package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/fsext"
)

const (
	InitFlagFilename = "init"
)

type ProjectInitFlag struct {
	Initialized bool `json:"initialized"`
}

func Init(workingDir, dataDir string, debug bool) (*ConfigStore, error) {
	store, err := Load(workingDir, dataDir, debug)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func ProjectNeedsInitialization(store *ConfigStore) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("config not loaded")
	}

	cfg := store.Config()
	flagFilePath := filepath.Join(cfg.Options.DataDirectory, InitFlagFilename)

	_, err := os.Stat(flagFilePath)
	if err == nil {
		return false, nil
	}

	if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check init flag file: %w", err)
	}

	someContextFileExists, err := contextPathsExist(store.WorkingDir())
	if err != nil {
		return false, fmt.Errorf("failed to check for context files: %w", err)
	}
	if someContextFileExists {
		return false, nil
	}

	// If the working directory has no non-ignored files, skip initialization step
	empty, err := dirHasNoVisibleFiles(store.WorkingDir())
	if err != nil {
		return false, fmt.Errorf("failed to check if directory is empty: %w", err)
	}
	if empty {
		return false, nil
	}

	return true, nil
}

// [XRUSH: begin: contextPathsExist rewritten to support nested paths and directory entries]
func contextPathsExist(dir string) (bool, error) {
	// Check if any of the default context paths exist in the directory.
	// This handles both top-level files (e.g. "CRUSH.md") and nested paths
	// (e.g. ".github/copilot-instructions.md") and directory entries
	// (e.g. ".cursor/rules/").
	for _, path := range defaultContextPaths {
		fullPath := filepath.Join(dir, path)

		// Paths ending with "/" are directory checks.
		if strings.HasSuffix(path, "/") {
			if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
				return true, nil
			}
			continue
		}

		if fileExists(fullPath) {
			return true, nil
		}
	}

	return false, nil
}

// [XRUSH: end]

// dirHasNoVisibleFiles returns true if the directory has no files/dirs after applying ignore rules.
func dirHasNoVisibleFiles(dir string) (bool, error) {
	files, _, err := fsext.ListDirectory(dir, nil, 1, 1)
	if err != nil {
		return false, err
	}
	return len(files) == 0, nil
}

func MarkProjectInitialized(store *ConfigStore) error {
	if store == nil {
		return fmt.Errorf("config not loaded")
	}
	flagFilePath := filepath.Join(store.Config().Options.DataDirectory, InitFlagFilename)

	file, err := os.Create(flagFilePath)
	if err != nil {
		return fmt.Errorf("failed to create init flag file: %w", err)
	}
	defer file.Close()

	return nil
}

func HasInitialDataConfig(store *ConfigStore) bool {
	if store == nil {
		return false
	}
	cfgPath := GlobalConfigData()
	if _, err := os.Stat(cfgPath); err != nil {
		return false
	}
	return store.Config().IsConfigured()
}
