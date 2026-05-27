package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// walkContextFileNames are the context file names that WalkContextPaths looks
// for at each directory level when walking from root toward home.
var walkContextFileNames = []string{
	"AGENTS.md",
	"CLAUDE.md",
	"CRUSH.md",
	"GEMINI.md",
	"CLAUDE.local.md",
	"CRUSH.local.md",
	"GEMINI.local.md",
	".cursorrules",
	".github/copilot-instructions.md",
	"CRUSH.memory.md",
}

// WalkContextPaths walks from root upward toward the user's home directory,
// collecting paths to context files (AGENTS.md, CLAUDE.md, CRUSH.md, etc.)
// that exist at each level. Walking stops at the home directory (inclusive).
// Results are deduplicated by absolute path and returned in order from root
// (deepest) to home (shallowest), with the root directory's matches first.
func WalkContextPaths(root string) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}
	homeDir, err = filepath.Abs(homeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	// Build the walk order from home down to root so deeper matches are
	// appended last, then the final reverse puts deepest first.
	var dirs []string
	cur := root
	for {
		dirs = append(dirs, cur)
		if cur == homeDir {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	slices.Reverse(dirs)

	var result []string
	seen := make(map[string]bool)

	for _, dir := range dirs {
		for _, name := range walkContextFileNames {
			candidate := filepath.Join(dir, name)
			absCandidate, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			if seen[absCandidate] {
				continue
			}
			if fileExists(candidate) {
				seen[absCandidate] = true
				result = append(result, absCandidate)
			}
		}
	}

	slices.Reverse(result)
	return result, nil
}

// ResolveContextPaths resolves context file paths across 4 layers:
// managed → user → project → local. The first layer to contain a file with a
// given basename wins; subsequent layers are skipped for that name.
// Empty layer directory strings are ignored.
func ResolveContextPaths(localDir, managedDir, userDir, projectDir string) ([]string, error) {
	seen := make(map[string]bool)

	localPaths, err := WalkContextPaths(localDir)
	if err != nil {
		return nil, err
	}

	var result []string
	add := func(path string) {
		base := filepath.Base(path)
		if seen[base] {
			return
		}
		seen[base] = true
		result = append(result, path)
	}

	for _, dir := range []string{managedDir, userDir, projectDir} {
		if dir == "" {
			continue
		}
		for _, name := range walkContextFileNames {
			candidate := filepath.Join(dir, name)
			if fileExists(candidate) {
				abs, err := filepath.Abs(candidate)
				if err != nil {
					continue
				}
				add(abs)
			}
		}
	}

	for _, p := range localPaths {
		add(p)
	}

	return result, nil
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
