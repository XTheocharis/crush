package prompt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStripYAMLFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips simple frontmatter",
			input: "---\nkey: value\n---\nHello world\n",
			want:  "Hello world\n",
		},
		{
			name:  "strips multi-line frontmatter",
			input: "---\nkey: value\nother: data\nnested:\n  item: true\n---\nContent here\n",
			want:  "Content here\n",
		},
		{
			name:  "no frontmatter returns content as-is",
			input: "Just some content\nNo frontmatter here\n",
			want:  "Just some content\nNo frontmatter here\n",
		},
		{
			name:  "empty frontmatter",
			input: "---\n---\nBody text\n",
			want:  "Body text\n",
		},
		{
			name:  "frontmatter with CRLF",
			input: "---\r\nkey: value\r\n---\r\nContent\r\n",
			want:  "Content\r\n",
		},
		{
			name:  "BOM prefix stripped before frontmatter",
			input: "\uFEFF---\nkey: val\n---\nContent\n",
			want:  "Content\n",
		},
		{
			name:  "only opening delimiter not stripped",
			input: "---\nkey: value\nNo closing delimiter\n",
			want:  "---\nkey: value\nNo closing delimiter\n",
		},
		{
			name:  "empty string stays empty",
			input: "",
			want:  "",
		},
		{
			name:  "does not strip mid-file delimiters",
			input: "# Title\n\n---\n\nSome content\n",
			want:  "# Title\n\n---\n\nSome content\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripYAMLFrontmatter(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStripHTMLComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips single-line comment",
			input: "before <!-- this is a comment --> after",
			want:  "before  after",
		},
		{
			name:  "strips multi-line comment",
			input: "before <!-- multi\nline\ncomment --> after",
			want:  "before  after",
		},
		{
			name:  "strips multiple comments",
			input: "a <!-- x --> b <!-- y --> c",
			want:  "a  b  c",
		},
		{
			name:  "no comments returns as-is",
			input: "plain text no comments",
			want:  "plain text no comments",
		},
		{
			name:  "empty string stays empty",
			input: "",
			want:  "",
		},
		{
			name:  "comment at start and end",
			input: "<!-- start -->content<!-- end -->",
			want:  "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripHTMLComments(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeContent(t *testing.T) {
	t.Parallel()

	t.Run("strips frontmatter then HTML comments", func(t *testing.T) {
		t.Parallel()
		input := "---\nkey: value\n---\nHello <!-- hidden --> world\n"
		want := "Hello  world\n"
		got := SanitizeContent(input)
		require.Equal(t, want, got)
	})

	t.Run("no sanitization needed returns as-is", func(t *testing.T) {
		t.Parallel()
		input := "Clean content\n"
		got := SanitizeContent(input)
		require.Equal(t, input, got)
	})
}

func TestContextCacheBasics(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "AGENTS.md")
	content := "---\nkey: val\n---\nHello <!-- comment --> world\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cache := NewContextCache()

	// First read: should read from disk and sanitize.
	cf := cache.Get(path)
	require.NotNil(t, cf)
	require.Equal(t, "Hello  world\n\n", cf.Content)

	// Second read: should return cached version.
	cf2 := cache.Get(path)
	require.NotNil(t, cf2)
	require.Equal(t, cf.Content, cf2.Content)
}

func TestContextCacheInvalidation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	cache := NewContextCache()

	// Populate cache.
	cf := cache.Get(path)
	require.NotNil(t, cf)
	require.Equal(t, "original", cf.Content)

	// Change file mtime + content to simulate edit.
	time.Sleep(10 * time.Millisecond) // Ensure mtime differs.
	require.NoError(t, os.WriteFile(path, []byte("updated"), 0o644))

	// Should detect change and re-read.
	cf2 := cache.Get(path)
	require.NotNil(t, cf2)
	require.Equal(t, "updated", cf2.Content)
}

func TestContextCacheMissingFile(t *testing.T) {
	t.Parallel()

	cache := NewContextCache()
	cf := cache.Get("/nonexistent/path/AGENTS.md")
	require.Nil(t, cf)
}

func TestProcessFileWithInclude(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subfilePath := filepath.Join(dir, "subfile.md")
	if err := os.WriteFile(subfilePath, []byte("included-content-here"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.md")
	mainContent := "before\n@include subfile.md\nafter"
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := processFile(mainPath)
	require.NotNil(t, result)
	require.Contains(t, result.Content, "included-content-here")
	require.Contains(t, result.Content, "before")
	require.Contains(t, result.Content, "after")
	require.NotContains(t, result.Content, "@include")
}

func TestProcessFileWithMissingInclude(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.md")
	mainContent := "before\n@include nonexistent.md\nafter"
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result := processFile(mainPath)
	require.NotNil(t, result)
	require.Contains(t, result.Content, "before")
	require.Contains(t, result.Content, "@include nonexistent.md")
	require.Contains(t, result.Content, "after")
}

func TestContextCacheProcessIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subPath := filepath.Join(dir, "sub.md")
	require.NoError(t, os.WriteFile(subPath, []byte("included-content-here"), 0o644))

	mainPath := filepath.Join(dir, "main.md")
	mainContent := "before\n@include sub.md\nafter"
	require.NoError(t, os.WriteFile(mainPath, []byte(mainContent), 0o644))

	cache := NewContextCache()
	cf := cache.Get(mainPath)
	require.NotNil(t, cf)
	require.Contains(t, cf.Content, "included-content-here")
	require.Contains(t, cf.Content, "before")
	require.Contains(t, cf.Content, "after")
	require.NotContains(t, cf.Content, "@include")
}

func TestContextCacheProcessIncludesConditional(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "AGENTS.md")
	content := "header\n<!-- if: language:go -->\ngo-specific\n<!-- endif -->\ntrailer\n"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	cache := NewContextCache()
	cf := cache.Get(filePath)
	require.NotNil(t, cf)
	require.NotContains(t, cf.Content, "<!-- if:")
	require.NotContains(t, cf.Content, "<!-- endif")
	require.Contains(t, cf.Content, "header")
	require.Contains(t, cf.Content, "trailer")
}

func TestContextCacheInvalidate(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("cached"), 0o644))

	cache := NewContextCache()

	// Populate.
	cf := cache.Get(path)
	require.NotNil(t, cf)
	require.Equal(t, "cached", cf.Content)

	// Explicitly invalidate.
	cache.Invalidate(path)

	// Write new content.
	require.NoError(t, os.WriteFile(path, []byte("fresh"), 0o644))

	// Should re-read after invalidation.
	cf2 := cache.Get(path)
	require.NotNil(t, cf2)
	require.Equal(t, "fresh", cf2.Content)
}
