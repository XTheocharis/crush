//go:build !treesitter

package repomap

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
)

// RankedDefinition is a definition-level rank entry.
type RankedDefinition struct {
	File  string
	Ident string
	Rank  float64
	Line  int
}

func sortRankedDefinitions(defs []RankedDefinition) {
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Rank != defs[j].Rank {
			return defs[i].Rank > defs[j].Rank
		}
		if defs[i].File != defs[j].File {
			return defs[i].File < defs[j].File
		}
		return defs[i].Ident < defs[j].Ident
	})
}

func normalizeGraphRelPath(path string) string {
	if path = strings.TrimSpace(path); path == "" {
		return ""
	}
	if path = filepath.ToSlash(filepath.Clean(path)); path == "." {
		return ""
	}
	return path
}

func normalizeUniqueGraphPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel := normalizeGraphRelPath(p)
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

// RunInjectionKey identifies one logical Run() for injection gating.
type RunInjectionKey struct {
	RootUserMessageID string
	QueueGeneration   int64
}

type runInjectionKeyContextKey string

const runInjectionKeyCtxKey runInjectionKeyContextKey = "run_injection_key"

// WithRunInjectionKey stores the run injection key in context.
func WithRunInjectionKey(ctx context.Context, key RunInjectionKey) context.Context {
	return context.WithValue(ctx, runInjectionKeyCtxKey, key)
}

// RunInjectionKeyFromContext retrieves the run injection key from context.
func RunInjectionKeyFromContext(ctx context.Context) (RunInjectionKey, bool) {
	v := ctx.Value(runInjectionKeyCtxKey)
	if v == nil {
		return RunInjectionKey{}, false
	}
	key, ok := v.(RunInjectionKey)
	if !ok {
		return RunInjectionKey{}, false
	}
	if key.RootUserMessageID == "" {
		return RunInjectionKey{}, false
	}
	return key, true
}
