package config

import "cmp"

// RepoMapOptions configures repository map generation.
type RepoMapOptions struct {
	// Disabled turns off repo map generation entirely.
	Disabled bool `json:"disabled,omitempty" jsonschema:"description=Disable repo map generation entirely"`
	// MaxTokens overrides the dynamic token budget for the rendered map.
	// When zero, the dynamic default is used.
	MaxTokens int `json:"max_tokens,omitempty" jsonschema:"description=Override max token budget for rendered map (0 = dynamic)"`
	// ExcludeGlobs are additional glob patterns excluded from scanning.
	ExcludeGlobs []string `json:"exclude_globs,omitempty" jsonschema:"description=Additional glob patterns to exclude from repo map scanning"`
	// RefreshMode controls when the map is regenerated.
	RefreshMode string `json:"refresh_mode,omitempty" jsonschema:"description=When to regenerate the repo map: auto files manual or always"`
	// MapMulNoFiles is the budget multiplier when no files are in chat.
	MapMulNoFiles float64 `json:"map_mul_no_files,omitempty" jsonschema:"description=Budget multiplier when no files are in chat (default 2.0)"`
	// ParserPoolSize sets tree-sitter parser pool capacity.
	// Zero uses the runtime default.
	ParserPoolSize int `json:"parser_pool_size,omitempty" jsonschema:"description=Tree-sitter parser pool size (0 = runtime default)"`
}

func (o RepoMapOptions) merge(t RepoMapOptions) RepoMapOptions {
	o.Disabled = o.Disabled || t.Disabled
	o.MaxTokens = cmp.Or(t.MaxTokens, o.MaxTokens)
	o.ExcludeGlobs = sortedCompact(append(o.ExcludeGlobs, t.ExcludeGlobs...))
	o.RefreshMode = cmp.Or(t.RefreshMode, o.RefreshMode)
	if t.MapMulNoFiles != 0 {
		o.MapMulNoFiles = t.MapMulNoFiles
	}
	o.ParserPoolSize = cmp.Or(t.ParserPoolSize, o.ParserPoolSize)
	return o
}

// DefaultRepoMapMaxTokens computes the dynamic token budget based on model context
// window size: min(max(contextWindow/8, 1024), 4096).
func DefaultRepoMapMaxTokens(modelContextWindow int) int {
	budget := min(max(modelContextWindow/8, 1024), 4096)
	return budget
}

// DefaultRepoMapOptions returns repo map defaults.
func DefaultRepoMapOptions() RepoMapOptions {
	return RepoMapOptions{
		RefreshMode:   "auto",
		MapMulNoFiles: 2.0,
	}
}
