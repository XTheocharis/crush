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

// defaultWalkDownDepth is the default directory depth for downward scanning.
const defaultWalkDownDepth = 2

// skipDirs are directory names that are skipped during downward walking.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
	"__pycache__":  true,
	".tox":         true,
	".venv":        true,
	".svn":         true,
	".hg":          true,
}

// WalkContextPaths walks from root upward toward the user's home directory,
// collecting paths to context files (AGENTS.md, CLAUDE.md, CRUSH.md, etc.)
// that exist at each level. It also scans downward from root into immediate
// subdirectories up to [defaultWalkDownDepth] levels deep, skipping common
// dependency and build directories.
//
// Walking stops at the home directory (inclusive). Results are deduplicated
// by absolute path and returned in priority order: upward (deepest first),
// then downward (shallowest subdirectory first).
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

	downward := walkDownward(root, defaultWalkDownDepth, seen)
	result = append(result, downward...)

	return result, nil
}

// walkDownward scans subdirectories of root up to maxDepth levels, collecting
// context files. It skips directories listed in skipDirs. The seen map is
// mutated in place to deduplicate against upward results.
func walkDownward(root string, maxDepth int, seen map[string]bool) []string {
	var result []string

	// Walk starting from depth 1 (immediate children of root).
	for _, entry := range readDirNames(root) {
		if skipDirs[entry] {
			continue
		}
		sub := filepath.Join(root, entry)
		walkDownwardLevel(sub, 1, maxDepth, seen, &result)
	}

	return result
}

func walkDownwardLevel(dir string, depth, maxDepth int, seen map[string]bool, result *[]string) {
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
			*result = append(*result, absCandidate)
		}
	}

	if depth >= maxDepth {
		return
	}

	for _, entry := range readDirNames(dir) {
		if skipDirs[entry] {
			continue
		}
		sub := filepath.Join(dir, entry)
		walkDownwardLevel(sub, depth+1, maxDepth, seen, result)
	}
}

// readDirNames returns sorted directory entry names for the given path.
// Non-directory entries and unreadable dirs are silently skipped.
func readDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
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
