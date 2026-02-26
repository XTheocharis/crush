package repomap

import (
	"path"
	"sort"
	"strings"
)

var specialRootFiles = map[string]struct{}{
	"README.md":      {},
	"go.mod":         {},
	"package.json":   {},
	"Cargo.toml":     {},
	"pyproject.toml": {},
	"Dockerfile":     {},
	".env.example":   {},
}

// IsSpecialFile reports whether relPath is part of the special-file prelude set.
// Root-scoped files match only at repo root, except .github/workflows/*.yml|yaml.
func IsSpecialFile(relPath string) bool {
	relPath = normalizeGraphRelPath(relPath)
	if relPath == "" {
		return false
	}

	if strings.HasPrefix(relPath, ".github/workflows/") {
		base := strings.ToLower(path.Base(relPath))
		if strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") {
			return true
		}
	}

	if strings.Contains(relPath, "/") {
		return false
	}
	_, ok := specialRootFiles[relPath]
	return ok
}

// BuildSpecialPrelude selects stage-0 special files from otherFnames only,
// excluding files already represented in ranked output.
func BuildSpecialPrelude(otherFnames []string, rankedFiles []string, parityMode bool) []string {
	other := normalizeUniqueGraphPaths(otherFnames)
	if !parityMode {
		// Enhancement mode allows deterministic behavior; keep lexical order.
		sort.Strings(other)
	}

	rankedSet := make(map[string]struct{})
	for _, f := range normalizeUniqueGraphPaths(rankedFiles) {
		rankedSet[f] = struct{}{}
	}

	out := make([]string, 0)
	for _, f := range other {
		if !IsSpecialFile(f) {
			continue
		}
		if _, inRanked := rankedSet[f]; inRanked {
			continue
		}
		out = append(out, f)
	}
	return out
}
