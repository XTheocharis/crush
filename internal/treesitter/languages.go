package treesitter

import (
	"path/filepath"
	"strings"
)

// extensionOverrides maps file extensions to their tree-sitter grammar language name.
// Several extensions require explicit overrides because the tree-sitter grammar name
// differs from the language name inferred by extension.
var extensionOverrides = map[string]string{
	"jsx":    "javascript",      // JS grammar handles JSX natively
	"tsx":    "typescript",      // TS grammar handles TSX natively
	"cs":     "csharp",          // C# source files
	"ml":     "ocaml",           // OCaml source files
	"mli":    "ocaml_interface", // OCaml interface files (separate grammar)
	"kt":     "kotlin",          // Kotlin source files
	"kts":    "kotlin",          // Kotlin script files
	"tf":     "hcl",             // Terraform uses HCL grammar
	"tfvars": "hcl",             // Terraform variables use HCL grammar
	"hcl":    "hcl",             // HCL configuration files
	"ino":    "arduino",         // Arduino sketch files
	"m":      "matlab",          // MATLAB scripts (override to avoid R conflict)
	"ql":     "ql",              // CodeQL query files
	"rkt":    "racket",          // Racket source files
	"sol":    "solidity",        // Solidity smart contracts
	"cht":    "chatito",         // Chatito training files
	"lisp":   "commonlisp",      // Common Lisp source files
	"lsp":    "commonlisp",      // Common Lisp source files (alternative extension)
	"jl":     "julia",           // Julia source files
	"gleam":  "gleam",           // Gleam source files
	"elm":    "elm",             // Elm source files
	"ex":     "elixir",          // Elixir source files
	"exs":    "elixir",          // Elixir script files
	"rules":  "udev",            // udev rules files
}

// languageAliases maps language identifiers to their query key for tags.scm lookup.
// Some languages share queries (e.g. tsx uses typescript-tags.scm in parity mode).
var languageAliases = map[string]string{
	"tsx":     "typescript", // tsx uses typescript query tags in parity mode
	"c_sharp": "csharp",     // prefer primary csharp query over fallback c_sharp
	"jsx":     "javascript", // JSX is handled by JavaScript grammar
	"csharp":  "csharp",     // ensure csharp is canonical
	"kotlin":  "kotlin",     // ensure kotlin is canonical
	"fortran": "fortran",    // ensure fortran is canonical
	"julia":   "julia",      // ensure julia is canonical
	"php":     "php",        // ensure php is canonical
	"scala":   "scala",      // ensure scala is canonical
	"zig":     "zig",        // ensure zig is canonical
	"ql":      "ql",         // ensure ql is canonical
	"haskell": "haskell",    // ensure haskell is canonical
}

// BaseExtensions provides fallback language mapping for extensions without
// explicit overrides. This covers common extensions mapped directly to their
// tree-sitter grammar names.
var BaseExtensions = map[string]string{
	"go":         "go",
	"py":         "python",
	"pyw":        "python",
	"pyx":        "python",
	"pxd":        "python",
	"js":         "javascript",
	"mjs":        "javascript",
	"cjs":        "javascript",
	"ts":         "typescript",
	"mts":        "typescript",
	"cts":        "typescript",
	"rs":         "rust",
	"java":       "java",
	"c":          "c",
	"h":          "c",
	"cpp":        "cpp",
	"cxx":        "cpp",
	"cc":         "cpp",
	"hpp":        "cpp",
	"hxx":        "cpp",
	"hh":         "cpp",
	"rb":         "ruby",
	"rake":       "ruby",
	"sh":         "bash",
	"bash":       "bash",
	"zsh":        "bash",
	"fish":       "bash",
	"php":        "php",
	"scala":      "scala",
	"sc":         "scala",
	"swift":      "swift",
	"sql":        "sql",
	"html":       "html",
	"htm":        "html",
	"css":        "css",
	"scss":       "scss",
	"less":       "less",
	"json":       "json",
	"yaml":       "yaml",
	"yml":        "yaml",
	"toml":       "toml",
	"xml":        "xml",
	"d":          "d",
	"di":         "d",
	"dart":       "dart",
	"hs":         "haskell",
	"lhs":        "haskell",
	"lua":        "lua",
	"r":          "r",
	"properties": "properties",
}

// MapExtension returns the tree-sitter language ID for a file extension.
// It checks extensionOverrides first, then falls back to BaseExtensions.
// The lookup is case-insensitive.
//
// Returns empty string if the extension is not mapped.
func MapExtension(ext string) string {
	if ext == "" {
		return ""
	}

	// Handle extensions with leading dot
	ext = strings.TrimPrefix(ext, ".")
	ext = strings.ToLower(ext)

	// Check overrides first
	if lang, ok := extensionOverrides[ext]; ok {
		return lang
	}

	// Fall back to base extensions
	if lang, ok := BaseExtensions[ext]; ok {
		return lang
	}

	return ""
}

// MapPath returns the tree-sitter language ID for a file path.
// It extracts the extension and delegates to MapExtension.
func MapPath(path string) string {
	ext := filepath.Ext(path)
	return MapExtension(ext)
}

// GetQueryKey returns the query file key for a language identifier.
// It applies languageAliases to resolve language identifiers to their
// query file base name (e.g. "tsx" -> "typescript" for "typescript-tags.scm").
//
// This is used by HasTags() and LoadTagsQuery().
func GetQueryKey(lang string) string {
	if lang == "" {
		return ""
	}

	lang = strings.TrimSpace(lang)
	lang = strings.ToLower(lang)

	// Check for alias first
	if alias, ok := languageAliases[lang]; ok {
		return alias
	}

	return lang
}

// HasTags reports whether a language has tags query support.
// It resolves language identifiers through GetQueryKey().
func HasTags(lang string) bool {
	return HasTagsQuery(GetQueryKey(lang))
}

// GetTagsQueryPath returns the embedded query file path for a language.
func GetTagsQueryPath(lang string) string {
	return "queries/" + GetQueryKey(lang) + "-tags.scm"
}
