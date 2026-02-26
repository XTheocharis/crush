package treesitter

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapExtension_OverrideExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ext      string
		wantLang string
	}{
		{"jsx maps to javascript", "jsx", "javascript"},
		{"jsx with dot", ".jsx", "javascript"},
		{"jsx uppercase", ".JSX", "javascript"},
		{"tsx maps to typescript", "tsx", "typescript"},
		{"tsx with dot", ".tsx", "typescript"},
		{"cs maps to csharp", "cs", "csharp"},
		{"ml maps to ocaml", "ml", "ocaml"},
		{"mli maps to ocaml_interface", "mli", "ocaml_interface"},
		{"kt maps to kotlin", "kt", "kotlin"},
		{"kts maps to kotlin", "kts", "kotlin"},
		{"tf maps to hcl", "tf", "hcl"},
		{"tfvars maps to hcl", "tfvars", "hcl"},
		{"hcl maps to hcl", "hcl", "hcl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(tt.ext)
			require.Equal(t, tt.wantLang, got)
		})
	}
}

func TestMapExtension_BaseExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ext      string
		wantLang string
	}{
		{"go", "go", "go"},
		{"py", "py", "python"},
		{"pyw", "pyw", "python"},
		{"pyx", "pyx", "python"},
		{"js", "js", "javascript"},
		{"mjs", "mjs", "javascript"},
		{"cjs", "cjs", "javascript"},
		{"ts", "ts", "typescript"},
		{"mts", "mts", "typescript"},
		{"c", "c", "c"},
		{"cpp", "cpp", "cpp"},
		{"rs", "rs", "rust"},
		{"php", "php", "php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(tt.ext)
			require.Equal(t, tt.wantLang, got)
		})
	}
}

func TestMapExtension_CaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ext      string
		wantLang string
	}{
		{".GO", "go"},
		{".Py", "python"},
		{".JaVa", "java"},
		{".RS", "rust"},
		{".JSX", "javascript"},
		{".TSX", "typescript"},
		{".CS", "csharp"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(tt.ext)
			require.Equal(t, tt.wantLang, got)
		})
	}
}

func TestMapExtension_UnknownExtensions(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		".unknown",
		"xyz",
		"txt",
		"bin",
		".",
	}

	for _, ext := range tests {
		t.Run(ext, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(ext)
			require.Equal(t, "", got, "unknown extension should return empty string")
		})
	}
}

func TestMapPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantLang string
	}{
		{"simple go file", "main.go", "go"},
		{"path with dir", "internal/config/config.go", "go"},
		{"jsx file", "components/Button.jsx", "javascript"},
		{"tsx file", "components/Button.tsx", "typescript"},
		{"csharp file", "Program.cs", "csharp"},
		{"ocaml file", "main.ml", "ocaml"},
		{"ocaml interface", "main.mli", "ocaml_interface"},
		{"terraform file", "main.tf", "hcl"},
		{"kotlin file", "Main.kt", "kotlin"},
		{"no extension", "Makefile", ""},
		{"empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MapPath(tt.path)
			require.Equal(t, tt.wantLang, got)
		})
	}
}

func TestGetQueryKey_LanguageAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lang    string
		wantKey string
	}{
		{"tsx aliased to typescript", "tsx", "typescript"},
		{"tsx uppercase", "TSX", "typescript"},
		{"tsx with spaces", " tsx ", "typescript"},
		{"go no alias", "go", "go"},
		{"python no alias", "python", "python"},
		{"javascript no alias", "javascript", "javascript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetQueryKey(tt.lang)
			require.Equal(t, tt.wantKey, got)
		})
	}
}

func TestGetQueryKey_EmptyInput(t *testing.T) {
	t.Parallel()

	got := GetQueryKey("")
	require.Equal(t, "", got)

	got = GetQueryKey("   ")
	require.Equal(t, "", got)
}

func TestGetQueryKey_Normalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"Go", "go"},
		{"PYTHON", "python"},
		{"  JavaScript  ", "javascript"},
		{"TypeScript", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := GetQueryKey(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestGetTagsQueryPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lang string
		want string
	}{
		{"go query path", "go", "queries/go-tags.scm"},
		{"python query path", "python", "queries/python-tags.scm"},
		{"tsx aliased to typescript", "tsx", "queries/typescript-tags.scm"},
		{"typescript query path", "typescript", "queries/typescript-tags.scm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := GetTagsQueryPath(tt.lang)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExtensionOverridesCompleteness(t *testing.T) {
	t.Parallel()

	// Verify all required overrides from the plan are present
	requiredOverrides := map[string]string{
		"jsx":    "javascript",
		"tsx":    "typescript",
		"cs":     "csharp",
		"ml":     "ocaml",
		"mli":    "ocaml_interface",
		"kt":     "kotlin",
		"tf":     "hcl",
		"hcl":    "hcl",
		"tfvars": "hcl",
		"kts":    "kotlin",
	}

	for ext, expectedLang := range requiredOverrides {
		t.Run(ext, func(t *testing.T) {
			t.Parallel()
			actual := extensionOverrides[ext]
			require.Equal(t, expectedLang, actual, "extension %s should map to %s", ext, expectedLang)
		})
	}
}

func TestLanguageAliasesCompleteness(t *testing.T) {
	t.Parallel()

	// Verify all required aliases from the plan are present
	requiredAliases := map[string]string{
		"tsx":     "typescript",
		"c_sharp": "csharp",
	}

	for lang, expectedAlias := range requiredAliases {
		t.Run(lang, func(t *testing.T) {
			t.Parallel()
			actual := languageAliases[lang]
			require.Equal(t, expectedAlias, actual, "language %s should alias to %s", lang, expectedAlias)
		})
	}
}

func TestHasTags(t *testing.T) {
	t.Parallel()

	// HasTags should check embedded queries and apply aliases
	// For bootstrap languages (go, python), we expect true
	require.True(t, HasTags("go"), "go should have tags")
	require.True(t, HasTags("Go"), "Go should have tags (case insensitive)")
	require.True(t, HasTags("python"), "python should have tags")
	require.True(t, HasTags("PYTHON"), "PYTHON should have tags (case insensitive)")

	require.True(t, HasTags("typescript"), "typescript should have tags")
	require.True(t, HasTags("tsx"), "tsx should resolve to typescript tags")
	require.True(t, HasTags("csharp"), "csharp should have tags")
	require.True(t, HasTags("c_sharp"), "c_sharp should alias to csharp tags")
	require.True(t, HasTags("zig"), "zig fallback query should be available")
}

func TestOverridePriority(t *testing.T) {
	t.Parallel()

	// Ensure overrides take precedence over default mappings
	// Test with jsx: since it's in extensionOverrides, it should map to "javascript"
	// not whatever might be in BaseExtensions (which we don't use for overriding behaviors)
	tests := []struct {
		ext  string
		want string
	}{
		{"jsx", "javascript"},
		{"tsx", "typescript"},
		{"cs", "csharp"},
		{"tf", "hcl"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(tt.ext)
			require.Equal(t, tt.want, got, "overrides should take priority")
		})
	}
}

func TestLoadLanguagesManifest_Phase1BSet(t *testing.T) {
	t.Parallel()

	manifest, err := LoadLanguagesManifest()
	require.NoError(t, err)
	require.Equal(t, 39, len(manifest.Languages), "expected full Phase 1B language set")

	seen := make(map[string]struct{}, len(manifest.Languages))
	for _, lang := range manifest.Languages {
		require.NotEmpty(t, lang.Name)
		_, dup := seen[lang.Name]
		require.False(t, dup, "duplicate language entry: %s", lang.Name)
		seen[lang.Name] = struct{}{}
	}

	for _, required := range []string{"go", "python", "typescript", "kotlin", "ql", "csharp", "zig"} {
		_, ok := seen[required]
		require.True(t, ok, "missing language in manifest: %s", required)
	}
}

func TestVendoredTagsQueries_NoInheritsDirective(t *testing.T) {
	t.Parallel()

	entries, err := fs.Glob(queriesFS, "queries/*-tags.scm")
	require.NoError(t, err)
	require.Len(t, entries, 39, "expected vendored query count to match manifest")

	for _, entry := range entries {
		entry := entry
		t.Run(entry, func(t *testing.T) {
			t.Parallel()
			content, err := queriesFS.ReadFile(entry)
			require.NoError(t, err)
			require.NotContains(t, string(content), "; inherits:", "query must be self-contained: %s", entry)
		})
	}
}

func TestLoadTagsQuery_AliasPrecedenceCSharp(t *testing.T) {
	t.Parallel()

	primary, err := LoadTagsQuery("csharp")
	require.NoError(t, err)
	alias, err := LoadTagsQuery(GetQueryKey("c_sharp"))
	require.NoError(t, err)
	fallback, err := queriesFS.ReadFile("queries/c_sharp-tags.scm")
	require.NoError(t, err)

	require.Equal(t, string(primary), string(alias), "c_sharp alias must resolve to primary csharp query")
	require.NotEqual(t, strings.TrimSpace(string(primary)), strings.TrimSpace(string(fallback)), "primary and fallback csharp queries should remain distinct files")
}

func TestExtensionMappingGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ext  string
		want string
	}{
		// Override extensions
		{".jsx", "javascript"},
		{"jsx", "javascript"},
		{".tsx", "typescript"},
		{"tsx", "typescript"},
		{".cs", "csharp"},
		{"cs", "csharp"},
		{".ml", "ocaml"},
		{"ml", "ocaml"},
		{".mli", "ocaml_interface"},
		{"mli", "ocaml_interface"},
		{".kt", "kotlin"},
		{"kt", "kotlin"},
		{".kts", "kotlin"},
		{"kts", "kotlin"},
		{".tf", "hcl"},
		{"tf", "hcl"},
		{".tfvars", "hcl"},
		{"tfvars", "hcl"},
		{".hcl", "hcl"},
		{"hcl", "hcl"},
		// Base extensions sampling
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".rs", "rust"},
		{".java", "java"},
		{".cpp", "cpp"},
		{".rb", "ruby"},
		{".php", "php"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			t.Parallel()
			got := MapExtension(tt.ext)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestQueryKeyGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		lang    string
		wantKey string
	}{
		// Language aliases
		{"tsx", "typescript"},
		{"TSX", "typescript"},
		{" tsx ", "typescript"},
		{"c_sharp", "csharp"},
		{"C_SHARP", "csharp"},
		{" c_sharp ", "csharp"},
		// Non-aliased languages
		{"go", "go"},
		{"python", "python"},
		{"javascript", "javascript"},
		{"typescript", "typescript"},
		{"csharp", "csharp"},
		{"rust", "rust"},
		{"java", "java"},
		{"kotlin", "kotlin"},
		// Case normalization
		{"Go", "go"},
		{"Python", "python"},
		{"TypeScript", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			t.Parallel()
			got := GetQueryKey(tt.lang)
			require.Equal(t, tt.wantKey, got)
		})
	}
}

func TestQueryKeyGolden_AliasExpectations(t *testing.T) {
	t.Parallel()

	// Specifically test alias expectations mentioned in task spec:
	// - tsx should alias to typescript (for typescript-tags.scm)
	// - c_sharp should alias to csharp (prefer primary query over fallback)
	require.Equal(t, "typescript", GetQueryKey("tsx"), "tsx must alias to typescript")
	require.Equal(t, "csharp", GetQueryKey("c_sharp"), "c_sharp must alias to csharp")

	// Verify case-insensitive behavior
	require.Equal(t, "typescript", GetQueryKey("TSX"), "TSX must alias to typescript")
	require.Equal(t, "typescript", GetQueryKey(" tsx "), "strip whitespace and lower")
	require.Equal(t, "csharp", GetQueryKey("C_SHARP"), "C_SHARP must alias to csharp")
}

func TestExtensionMappingGolden_TSX_CSExpectations(t *testing.T) {
	t.Parallel()

	// Specifically test extension mapping expectations from task spec:
	// - .tsx should map to "typescript" grammar
	// - .cs should map to "csharp" grammar

	// Test .tsx mapping
	require.Equal(t, "typescript", MapExtension("tsx"), "tsx extension must map to typescript")
	require.Equal(t, "typescript", MapExtension(".tsx"), ".tsx must map to typescript")
	require.Equal(t, "typescript", MapExtension(".TSX"), "case insensitive .TSX must map to typescript")

	// Test .cs mapping
	require.Equal(t, "csharp", MapExtension("cs"), "cs extension must map to csharp")
	require.Equal(t, "csharp", MapExtension(".cs"), ".cs must map to csharp")
	require.Equal(t, "csharp", MapExtension(".CS"), "case insensitive .CS must map to csharp")
}
