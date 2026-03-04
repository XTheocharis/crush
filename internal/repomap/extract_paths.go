package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func repoKeyForRoot(rootDir string) string {
	root := strings.TrimSpace(rootDir)
	if root == "" {
		return ""
	}

	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	root = filepath.Clean(root)
	root = filepath.ToSlash(root)

	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])
}

func normalizeRepoRelPath(rootDir, path string) (string, error) {
	root := strings.TrimSpace(rootDir)
	if root == "" {
		return "", fmt.Errorf("root directory is empty")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root dir: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absRoot, candidate)
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	absCandidate = filepath.Clean(absCandidate)

	rel, err := filepath.Rel(absRoot, absCandidate)
	if err != nil {
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	if rel == "." {
		return "", fmt.Errorf("path %q resolves to repository root", path)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside repository root", path)
	}

	return filepath.ToSlash(filepath.Clean(rel)), nil
}

func normalizeFileUniverse(rootDir string, fileUniverse []string) ([]string, error) {
	if len(fileUniverse) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(fileUniverse))
	normalized := make([]string, 0, len(fileUniverse))
	for _, path := range fileUniverse {
		rel, err := normalizeRepoRelPath(rootDir, path)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		normalized = append(normalized, rel)
	}

	sort.Strings(normalized)
	return normalized, nil
}
