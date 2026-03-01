package treesitter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type runtimeLanguageExceptionsArtifact struct {
	Version   string   `json:"version"`
	Languages []string `json:"languages"`
}

func loadRuntimeLanguageExceptions(t *testing.T) map[string]struct{} {
	t.Helper()

	path := filepath.Join("testdata", "runtime_language_exceptions.v1.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read runtime language exceptions artifact")

	var artifact runtimeLanguageExceptionsArtifact
	require.NoError(t, json.Unmarshal(data, &artifact), "parse runtime language exceptions artifact")
	require.Equal(t, "1", artifact.Version, "runtime language exceptions artifact version")
	require.NotEmpty(t, artifact.Languages, "runtime language exceptions artifact must list at least one language")

	set := make(map[string]struct{}, len(artifact.Languages))
	for _, lang := range artifact.Languages {
		key := GetQueryKey(lang)
		require.NotEmpty(t, key, "exception language must resolve to canonical query key: %q", lang)
		set[key] = struct{}{}
	}

	return set
}

func canonicalManifestLanguageSet(t *testing.T) map[string]struct{} {
	t.Helper()

	manifest, err := LoadLanguagesManifest()
	require.NoError(t, err)

	set := make(map[string]struct{}, len(manifest.Languages))
	for _, lang := range manifest.Languages {
		key := GetQueryKey(lang.Name)
		require.NotEmpty(t, key, "manifest language %q must resolve to canonical query key", lang.Name)
		set[key] = struct{}{}
	}

	return set
}

func runtimeActivatedLanguageSet(manifestSet map[string]struct{}) map[string]struct{} {
	runtimeSet := make(map[string]struct{}, len(manifestSet))
	for key := range manifestSet {
		if languageForQueryKey(key) != nil {
			runtimeSet[key] = struct{}{}
		}
	}
	return runtimeSet
}

// TestManifestRuntimeClosurePolicy enforces canonical runtime closure policy:
// every manifest language must be runtime-activated or explicitly versioned
// as parity-nonruntime in runtime_language_exceptions.v1.json.
func TestManifestRuntimeClosurePolicy(t *testing.T) {
	t.Parallel()

	manifestSet := canonicalManifestLanguageSet(t)
	runtimeSet := runtimeActivatedLanguageSet(manifestSet)
	exceptionSet := loadRuntimeLanguageExceptions(t)

	missing := make([]string, 0)
	for lang := range manifestSet {
		if _, ok := runtimeSet[lang]; ok {
			continue
		}
		if _, ok := exceptionSet[lang]; ok {
			continue
		}
		missing = append(missing, lang)
	}
	sort.Strings(missing)
	require.Empty(t, missing, "manifest languages must be runtime-activated or explicitly exceptioned")

	for lang := range exceptionSet {
		_, inManifest := manifestSet[lang]
		require.True(t, inManifest, "exception language %q is not present in canonical manifest set", lang)

		_, inRuntime := runtimeSet[lang]
		require.False(t, inRuntime, "exception language %q is runtime-activated; remove from exceptions artifact", lang)

		require.True(t, HasTagsQuery(lang), "exception language %q must still have a vendored tags query", lang)
	}
}

// TestParserRuntimeActivation_Gate defines hard gate tests for runtime activation behavior.
// This test ensures that:
// 1. Registered + query-backed languages parse tags correctly at runtime
// 2. Unsupported languages fail gracefully and are reported deterministically
func TestParserRuntimeActivation_Gate(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	// Define expected activation behavior for manifest languages
	// "ok" means: has runtime grammar AND query-backed, should parse tags
	// "no-runtime" means: in manifest but no runtime grammar, should not parse tags
	// "broken-query" means: has runtime grammar but query is broken, should return error
	testCases := []struct {
		name             string
		lang             string
		queryKey         string
		extension        string
		canonicalExtLang string // For aliases, the language that the extension maps to
		unmappedExt      bool   // True if the extension has no mapping
		activationType   string // "ok", "no-runtime", or "broken-query"
	}{
		// Runtime-registered + Query-backed (should parse tags)
		{name: "go", lang: "go", queryKey: "go", extension: "go", activationType: "ok"},
		{name: "python", lang: "python", queryKey: "python", extension: "py", activationType: "ok"},
		{name: "typescript", lang: "typescript", queryKey: "typescript", extension: "ts", activationType: "ok"},
		{name: "javascript", lang: "javascript", queryKey: "javascript", extension: "js", activationType: "ok"},
		{name: "rust", lang: "rust", queryKey: "rust", extension: "rs", activationType: "ok"},
		{name: "cpp", lang: "cpp", queryKey: "cpp", extension: "cpp", activationType: "ok"},
		{name: "c", lang: "c", queryKey: "c", extension: "c", activationType: "ok"},
		{name: "java", lang: "java", queryKey: "java", extension: "java", activationType: "ok"},
		{name: "ruby", lang: "ruby", queryKey: "ruby", extension: "rb", activationType: "ok"},
		{name: "php", lang: "php", queryKey: "php", extension: "php", activationType: "ok"},
		{name: "lua", lang: "lua", queryKey: "lua", extension: "lua", activationType: "ok"},
		{name: "haskell", lang: "haskell", queryKey: "haskell", extension: "hs", activationType: "ok"},
		{name: "dart", lang: "dart", queryKey: "dart", extension: "dart", activationType: "ok"},
		{name: "scala", lang: "scala", queryKey: "scala", extension: "scala", activationType: "ok"},
		{name: "arduino", lang: "arduino", queryKey: "arduino", extension: "ino", activationType: "ok"},
		{name: "chatito", lang: "chatito", queryKey: "chatito", extension: "cht", activationType: "ok"},
		{name: "hcl", lang: "hcl", queryKey: "hcl", extension: "hcl", activationType: "ok"},
		{name: "properties", lang: "properties", queryKey: "properties", extension: "properties", activationType: "ok"},
		{name: "csharp", lang: "csharp", queryKey: "csharp", extension: "cs", activationType: "ok"},
		{name: "ocaml", lang: "ocaml", queryKey: "ocaml", extension: "ml", activationType: "ok"},
		{name: "ocaml_interface", lang: "ocaml_interface", queryKey: "ocaml_interface", extension: "mli", activationType: "ok"},

		// Alias test cases (should resolve to runtime-supported query keys)
		{name: "tsx alias", lang: "tsx", queryKey: "typescript", extension: "tsx", canonicalExtLang: "typescript", activationType: "ok"},
		{name: "c_sharp alias", lang: "c_sharp", queryKey: "csharp", extension: "cs", canonicalExtLang: "csharp", activationType: "ok"},
		{name: "jsx alias", lang: "jsx", queryKey: "javascript", extension: "jsx", canonicalExtLang: "javascript", activationType: "ok"},

		// Languages with runtime grammar but broken query (should fail with error)
		{name: "julia broken-query", lang: "julia", queryKey: "julia", extension: "jl", activationType: "broken-query"},

		// Manifest languages without runtime grammar (should not parse tags)
		{name: "commonlisp no-runtime", lang: "commonlisp", queryKey: "commonlisp", extension: "lisp", activationType: "no-runtime"},
		{name: "d no-runtime", lang: "d", queryKey: "d", extension: "d", activationType: "no-runtime"},
		{name: "elisp no-runtime", lang: "elisp", queryKey: "elisp", extension: "el", unmappedExt: true, activationType: "no-runtime"},
		{name: "elixir no-runtime", lang: "elixir", queryKey: "elixir", extension: "ex", activationType: "no-runtime"},
		{name: "elm no-runtime", lang: "elm", queryKey: "elm", extension: "elm", activationType: "no-runtime"},
		{name: "fortran no-runtime", lang: "fortran", queryKey: "fortran", extension: "f90", unmappedExt: true, activationType: "no-runtime"},
		{name: "gleam no-runtime", lang: "gleam", queryKey: "gleam", extension: "gleam", activationType: "no-runtime"},
		{name: "kotlin no-runtime", lang: "kotlin", queryKey: "kotlin", extension: "kt", activationType: "no-runtime"},
		{name: "matlab no-runtime", lang: "matlab", queryKey: "matlab", extension: "m", activationType: "no-runtime"},
		{name: "ql no-runtime", lang: "ql", queryKey: "ql", extension: "ql", activationType: "no-runtime"},
		{name: "r no-runtime", lang: "r", queryKey: "r", extension: "r", activationType: "no-runtime"},
		{name: "racket no-runtime", lang: "racket", queryKey: "racket", extension: "rkt", activationType: "no-runtime"},
		{name: "solidity no-runtime", lang: "solidity", queryKey: "solidity", extension: "sol", activationType: "no-runtime"},
		{name: "swift no-runtime", lang: "swift", queryKey: "swift", extension: "swift", activationType: "no-runtime"},
		{name: "udev no-runtime", lang: "udev", queryKey: "udev", extension: "rules", activationType: "no-runtime"},
		{name: "zig no-runtime", lang: "zig", queryKey: "zig", extension: "zig", unmappedExt: true, activationType: "no-runtime"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Verify language is supported through manifest
			require.True(t, p.SupportsLanguage(tc.lang), "language %s should be in manifest and thus supported", tc.lang)

			// Verify query key resolves correctly
			require.Equal(t, tc.queryKey, GetQueryKey(tc.lang), "query key for %s should resolve to %s", tc.lang, tc.queryKey)

			// Verify extension mapping
			if tc.canonicalExtLang != "" {
				require.Equal(t, tc.canonicalExtLang, MapExtension(tc.extension), "extension .%s should map to %s", tc.extension, tc.canonicalExtLang)
			} else if !tc.unmappedExt {
				require.Equal(t, tc.lang, MapExtension(tc.extension), "extension .%s should map to %s", tc.extension, tc.lang)
			} else {
				// For unmapped extensions, verify they are not mapped
				got := MapExtension(tc.extension)
				require.Empty(t, got, "extension .%s should be unmapped", tc.extension)
			}

			// Verify tags query exists
			require.True(t, p.HasTags(tc.lang), "language %s should have tags query", tc.lang)

			switch tc.activationType {
			case "ok":
				// Runtime-registered + query-backed: should parse tags successfully
				t.Run("runtime parses tags", func(t *testing.T) {
					t.Parallel()

					src := minimalSourceForExtension(tc.extension)

					path := "/tmp/test." + tc.extension
					analysis, err := p.Analyze(context.Background(), path, src)

					require.NoError(t, err, "analysis should succeed without error")
					require.NotNil(t, analysis, "analysis should not be nil")
					require.Equal(t, tc.queryKey, analysis.Language, "analysis language should be query key %s", tc.queryKey)

					if tc.lang == "go" || tc.lang == "python" || tc.lang == "rust" || tc.lang == "javascript" || tc.lang == "typescript" {
						// For well-tested languages with known tag extraction, verify non-empty tags
						require.NotEmpty(t, analysis.Tags, "activation: %s should extract tags from source", tc.lang)
					} else {
						// For other runtime languages, verify tags parsing does not crash
						// (some minimal sources may not produce tags, which is acceptable)
						require.NotNil(t, analysis.Tags, "tags should be non-nil slice")
					}
				})
			case "no-runtime":
				// No runtime grammar: should fail gracefully with empty tags
				t.Run("no-runtime fails gracefully", func(t *testing.T) {
					t.Parallel()

					src := minimalSourceForExtension(tc.extension)

					path := "/tmp/test." + tc.extension
					analysis, err := p.Analyze(context.Background(), path, src)

					require.NoError(t, err, "analysis should succeed without error (graceful handling)")
					require.NotNil(t, analysis, "analysis should not be nil")

					// For unmapped extensions, language detection fails and returns empty string
					if tc.unmappedExt {
						require.Empty(t, analysis.Language, "unmapped extension should result in empty language")
					} else {
						require.Equal(t, tc.lang, analysis.Language, "analysis language should be %s (from manifest)", tc.lang)
					}

					require.Empty(t, analysis.Tags, "no-runtime language %s should return empty tags", tc.lang)
					require.Empty(t, analysis.Symbols, "no-runtime language %s should return empty symbols", tc.lang)
				})
			case "broken-query":
				// Runtime grammar exists but query is broken: should fail with error
				t.Run("broken-query fails with error", func(t *testing.T) {
					t.Parallel()

					src := minimalSourceForExtension(tc.extension)

					path := "/tmp/test." + tc.extension
					analysis, err := p.Analyze(context.Background(), path, src)

					require.Error(t, err, "analysis should fail with error for broken query")
					require.Nil(t, analysis, "analysis should be nil on error")
					require.Contains(t, err.Error(), "compile tags query", "error should mention query compilation")
				})
			}
		})
	}
}

// TestParserRuntimeActivation_UnsupportedLanguagesAreDeterministic verifies that
// languages outside the manifest fail in a deterministic way.
func TestParserRuntimeActivation_UnsupportedLanguagesAreDeterministic(t *testing.T) {
	t.Parallel()

	p := NewParser()
	t.Cleanup(func() {
		require.NoError(t, p.Close())
	})

	// Languages not in the manifest
	unsupportedLanguages := []string{
		"cobol", "fortran_legacy", "visual_basic", "perl", "assembly", "unknown_lang", "nonexistent",
	}

	for _, lang := range unsupportedLanguages {
		t.Run(lang, func(t *testing.T) {
			t.Parallel()

			// Verify language is not supported
			require.False(t, p.SupportsLanguage(lang), "language %s should not be supported", lang)

			// Verify no tags query
			require.False(t, p.HasTags(lang), "language %s should not have tags query", lang)

			// Verify deterministic behavior via Analyze with unknown extension
			path := "/tmp/test." + lang + "ext"
			src := []byte("unknown content")
			analysis, err := p.Analyze(context.Background(), path, src)

			require.NoError(t, err, "analysis should succeed without error")
			require.NotNil(t, analysis, "analysis should not be nil")
			require.Empty(t, analysis.Tags, "unsupported language should return empty tags")
			require.Empty(t, analysis.Symbols, "unsupported language should return empty symbols")
		})
	}
}

// minimalSourceForExtension returns minimal valid source code for a given file extension.
func minimalSourceForExtension(ext string) []byte {
	switch ext {
	case "go":
		return []byte(`package main
func main() {}
`)
	case "py", "pyx", "pxd", "pyw":
		return []byte(`def main():
    pass
`)
	case "js", "jsx", "mjs", "cjs":
		return []byte(`function main() {}
`)
	case "ts", "tsx", "mts", "cts":
		return []byte(`function main(): void {}
`)
	case "rs":
		return []byte(`fn main() {}
`)
	case "java":
		return []byte(`class Main {
    public static void main(String[] args) {}
}
`)
	case "cpp", "cxx", "cc", "hpp", "hxx", "hh":
		return []byte(`int main() {
    return 0;
}
`)
	case "c", "h":
		return []byte(`int main(void) {
    return 0;
}
`)
	case "rb", "rake":
		return []byte(`def main
end
`)
	case "php":
		return []byte(`<?php
function main() {}
`)
	case "lua":
		return []byte(`function main()
end
`)
	case "hs", "lhs":
		return []byte(`module Main where
main :: IO ()
main = return ()
`)
	case "dart":
		return []byte(`void main() {}
`)
	case "scala", "sc":
		return []byte(`object Main {
  def main(args: Array[String]): Unit = {}
}
`)
	case "ino":
		return []byte(`void setup() {}
void loop() {}
`)
	case "cht":
		return []byte(`%[greet]
hello`)
	case "lisp", "lsp":
		return []byte(`(defun main ()
  nil)`)
	case "jl":
		return []byte(`function main()
end
`)
	case "kt", "kts":
		return []byte(`fun main() {
}
`)
	case "hcl", "tf", "tfvars":
		return []byte(`resource "example" "test" {
  name = "test"
}
`)
	case "ql":
		return []byte(`from Function f
select f.getName()
`)
	case "properties":
		return []byte(`key=value
`)
	case "swift":
		return []byte(`func main() {
}
`)
	case "zig":
		return []byte(`pub fn main() void {}
`)
	case "r":
		return []byte(`main <- function() {
}
`)
	case "rkt":
		return []byte(`#lang racket
(define (main) #f)
`)
	case "sol":
		return []byte(`contract Test {
    function test() public {}
}
`)
	case "ex", "exs":
		return []byte(`defmodule Main do
  def main, do: :ok
end
`)
	case "gleam":
		return []byte(`pub fn main() {
  Nil
}
`)
	case "elm":
		return []byte(`main =
    text "Hello"
`)
	case "d", "di":
		return []byte(`void main() {
}
`)
	case "f90":
		return []byte(`program main
end program main
`)
	case "m":
		return []byte(`function main
end
`)
	case "el":
		return []byte(`(defun main () nil)
`)
	case "rules":
		return []byte(`ACTION=="add"
`)
	default:
		// Default minimal source for extensibility
		return []byte("minimal source")
	}
}
