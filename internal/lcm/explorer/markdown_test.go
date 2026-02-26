package explorer

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/require"
)

func TestMarkdownExplorer_CanHandle(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected bool
	}{
		{name: ".md extension", path: "file.md", content: []byte("# Hello"), expected: true},
		{name: ".markdown extension", path: "file.markdown", content: []byte("# Hello"), expected: true},
		{name: ".MD uppercase", path: "file.MD", content: []byte("# Hello"), expected: true},
		{name: ".MARKDOWN uppercase", path: "file.MARKDOWN", content: []byte("# Hello"), expected: true},
		{name: "no markdown extension", path: "file.txt", content: []byte("# Hello"), expected: false},
		{name: ".md with path", path: "docs/guide.md", content: []byte("# Guide"), expected: true},
		{name: ".json not handled", path: "file.json", content: []byte("{}"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := e.CanHandle(tc.path, tc.content)
			require.Equal(t, tc.expected, got, "CanHandle(%q)", tc.path)
		})
	}
}

func TestMarkdownExplorer_Explore_Smoke(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()
	input := ExploreInput{
		Path:    "test.md",
		Content: []byte("# Test\n\nHello world."),
	}

	result, err := e.Explore(ctx, input)
	require.NoError(t, err)
	require.NotEmpty(t, result.Summary)
	require.Equal(t, "markdown", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)
	require.Contains(t, result.Summary, "test.md")
	require.Contains(t, result.Summary, "Markdown file")
}

func TestMarkdownExplorer_Frontmatter(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	t.Run("with YAML frontmatter", func(t *testing.T) {
		t.Parallel()
		content := `---
title: "My Document"
date: 2024-01-01
tags: [test, markdown]
author: "Test Author"
---

# Heading
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)
		require.Contains(t, result.Summary, "Frontmatter: true")
		require.Contains(t, result.Summary, "Frontmatter keys: 4")
	})

	t.Run("without frontmatter", func(t *testing.T) {
		t.Parallel()
		content := `# Heading

Just content without frontmatter.
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)
		require.Contains(t, result.Summary, "Frontmatter: false")
	})

	t.Run("empty frontmarker", func(t *testing.T) {
		t.Parallel()
		content := `---

# Heading
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)
		require.Contains(t, result.Summary, "Frontmatter: true")
	})

	t.Run("frontmatter with leading whitespace", func(t *testing.T) {
		t.Parallel()
		content := ` ---
title: Test
---

# Heading
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)
		// With leading whitespace, it's not valid YAML frontmatter
		require.Contains(t, result.Summary, "Frontmatter: false")
	})

	t.Run("invalid YAML frontmarker", func(t *testing.T) {
		t.Parallel()
		content := `---
title: [invalid yaml
---

# Heading
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)
		require.Contains(t, result.Summary, "Frontmatter: true")
		// Key count will be 0 if YAML is invalid
		require.Contains(t, result.Summary, "Frontmatter keys:")
	})
}

func TestMarkdownExplorer_HeadingHierarchy(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	t.Run("ATX style headings", func(t *testing.T) {
		t.Parallel()
		content := `# H1 Header
## H2 Header
### H3 Header
#### H4 Header
##### H5 Header
###### H6 Header

# Another H1
## Another H2

Content here.
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Heading hierarchy:")
		require.Contains(t, result.Summary, "H1: 2")
		require.Contains(t, result.Summary, "H2: 2")
		require.Contains(t, result.Summary, "H3: 1")
		require.Contains(t, result.Summary, "H4: 1")
		require.Contains(t, result.Summary, "H5: 1")
		require.Contains(t, result.Summary, "H6: 1")
		require.Contains(t, result.Summary, "Total: 8")
	})

	t.Run("Setext style headings", func(t *testing.T) {
		t.Parallel()
		content := `Level 1 Heading
================

Level 2 Heading
----------------

Content paragraph.
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Heading hierarchy:")
		require.Contains(t, result.Summary, "H1: 1")
		require.Contains(t, result.Summary, "H2: 1")
		require.Contains(t, result.Summary, "Total: 2")
	})

	t.Run("no headings", func(t *testing.T) {
		t.Parallel()
		content := `Just some text without any headings.

Another paragraph with more text.
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Heading hierarchy:")
		require.Contains(t, result.Summary, "H1: 0")
		require.Contains(t, result.Summary, "Total: 0")
	})

	t.Run("invalid ATX headings", func(t *testing.T) {
		t.Parallel()
		content := `####### Too many hashes
#hashtag not a heading
#Heading but with trailing hashes ##
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		// #Heading is not valid (no space after #)
		// "####### Too many hashes" is not valid (more than 6)
		require.Contains(t, result.Summary, "H1: 1") // Only #Heading but with trailing hashes
		require.Contains(t, result.Summary, "Total: 1")
	})
}

func TestMarkdownExplorer_FencedCodeBlocks(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	t.Run("various languages", func(t *testing.T) {
		t.Parallel()
		// Use string concatenation to avoid backtick escaping issues in raw strings
		content := "# Code Examples\n\n" +
			"```go\n" +
			"func main() {\n" +
			"    fmt.Println(\"Hello\")\n" +
			"}\n" +
			"```\n\n" +
			"```python\n" +
			"def hello():\n" +
			"    print(\"Hello\")\n" +
			"```\n\n" +
			"```javascript\n" +
			"console.log(\"Hello\");\n" +
			"```\n\n" +
			"```\n" +
			"no language specified here\n" +
			"```\n"
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Fenced code blocks:")
		require.Contains(t, result.Summary, "go: 1")
		require.Contains(t, result.Summary, "python: 1")
		require.Contains(t, result.Summary, "javascript: 1")
		require.Contains(t, result.Summary, "unknown/plain: 1")
	})

	t.Run("no code blocks", func(t *testing.T) {
		t.Parallel()
		content := `# Document

Just text here with **bold** and *italic*.

> A blockquote
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.NotContains(t, result.Summary, "Fenced code blocks:")
	})

	t.Run("multiple blocks same language", func(t *testing.T) {
		t.Parallel()
		content := "```go\n" +
			"func a() {}\n" +
			"```\n\n" +
			"```go\n" +
			"func b() {}\n" +
			"```\n\n" +
			"```go\n" +
			"func c() {}\n" +
			"```\n"
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "go: 3")
	})

	t.Run("language with extension", func(t *testing.T) {
		t.Parallel()
		content := "```go.mod\n" +
			"module example\n" +
			"```\n"
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "go.mod: 1")
	})
}

func TestMarkdownExplorer_Links(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	t.Run("all link types", func(t *testing.T) {
		t.Parallel()
		content := `# Links Document

Inline links: [OpenAI](https://openai.com) and [GitHub](https://github.com "GitHub")

Reference-style links: [text][id1] and [more][id2]
[id1]: https://example.com
[id2]: https://another.com

Implicit reference: [Google][] and [Bing][]
[Google]: https://google.com

Autolinks: <http://example.com> and <https://secure.com>
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Links:")
		require.Contains(t, result.Summary, "Inline links (markdown style): 2")
		require.Contains(t, result.Summary, "Reference-style links: 2")
		require.Contains(t, result.Summary, "Autolinks (http/https URLs): 2")
		require.Contains(t, result.Summary, "Reference definitions: 3")
	})

	t.Run("only inline links", func(t *testing.T) {
		t.Parallel()
		content := `[Link1](url1) and [Link2](url2 "title") and [Link3](url3)`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Inline links (markdown style): 3")
		require.Contains(t, result.Summary, "Reference-style links: 0")
	})

	t.Run("only autolinks", func(t *testing.T) {
		t.Parallel()
		content := `Visit <http://example.com> or <https://secure.com/path>`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Autolinks (http/https URLs): 2")
	})

	t.Run("reference definitions with titles", func(t *testing.T) {
		t.Parallel()
		content := `[id]: https://example.com "Title text"
  [id2]: /relative/path 'another title'
   [id3]: ftp://server.com
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Reference definitions: 3")
	})

	t.Run("no links", func(t *testing.T) {
		t.Parallel()
		content := `Just some text with no links at all.

Not even brackets [like this].
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Inline links (markdown style): 0")
		require.Contains(t, result.Summary, "Reference-style links: 0")
		require.Contains(t, result.Summary, "Autolinks (http/https URLs): 0")
		require.Contains(t, result.Summary, "Reference definitions: 0")
	})

	t.Run("invalid reference definition format", func(t *testing.T) {
		t.Parallel()
		content := `Not a definition: [id] without colon

text

[id]: https://example.com  <-- this is a valid one
`
		input := ExploreInput{Path: "test.md", Content: []byte(content)}
		result, err := e.Explore(ctx, input)
		require.NoError(t, err)

		require.Contains(t, result.Summary, "Reference definitions: 1")
	})
}

func TestMarkdownExplorer_TokenEstimate(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	input := ExploreInput{
		Path:    "test.md",
		Content: []byte("# Test\n\nSome content here."),
	}

	result, err := e.Explore(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "markdown", result.ExplorerUsed)
	require.Greater(t, result.TokenEstimate, 0)

	// Token estimate should match estimateTokens(result.Summary)
	expectedTokens := estimateTokens(result.Summary)
	require.Equal(t, expectedTokens, result.TokenEstimate)
}

func TestMarkdownExplorer_LargeFile(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	largeContent := make([]byte, MaxFullLoadSize+1)
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	input := ExploreInput{
		Path:    "large.md",
		Content: largeContent,
	}

	result, err := e.Explore(ctx, input)
	require.NoError(t, err)
	require.Contains(t, result.Summary, "too large")
	require.Equal(t, "markdown", result.ExplorerUsed)
}

func TestMarkdownExplorer_ComplexDocument(t *testing.T) {
	t.Parallel()

	e := &MarkdownExplorer{}
	ctx := context.Background()

	// Use string concatenation for content with backticks
	content := "---\n" +
		"title: \"Complete Guide\"\n" +
		"date: 2024-01-15\n" +
		"tags: [tutorial, markdown, guide]\n" +
		"author: \"Maintainer\"\n" +
		"---\n\n" +
		"# Document Title\n\n" +
		"## Table of Contents\n" +
		"1. Introduction\n" +
		"2. Setup\n" +
		"3. Usage\n\n" +
		"## Introduction\n\n" +
		"This document covers everything about markdown.\n\n" +
		"### Basic Syntax\n\n" +
		"Here's some inline code `print()` and a fenced block:\n\n" +
		"```python\n" +
		"def hello():\n" +
		"    print(\"Hello, World!\")\n" +
		"```\n\n" +
		"### Advanced Features\n\n" +
		"```javascript\n" +
		"const greet = () => console.log(\"Hi\");\n" +
		"```\n\n" +
		"#### Lists\n\n" +
		"- Item 1\n" +
		"- Item 2\n" +
		"  - Nested item\n\n" +
		"#### Tables\n\n" +
		"| Column 1 | Column 2 |\n" +
		"|----------|----------|\n" +
		"| Value 1  | Value 2  |\n\n" +
		"##### Links and Images\n\n" +
		"Visit [OpenAI](https://openai.com) or [GitHub](https://github.com \"GitHub\").\n\n" +
		"See also [reference link][ref].\n" +
		"[ref]: https://example.com\n\n" +
		"Some autolinks too: <http://example.com>\n\n" +
		"###### Final Section\n\n" +
		"This is H6, the lowest level.\n\n" +
		"## References\n\n" +
		"Check out related docs: [Related][ref2]\n" +
		"[ref2]: https://docs.example.com\n\n" +
		"## Conclusion\n\n" +
		"End of document.\n"

	input := ExploreInput{Path: "complete.md", Content: []byte(content)}
	result, err := e.Explore(ctx, input)
	require.NoError(t, err)

	require.Equal(t, "markdown", result.ExplorerUsed)
	require.Contains(t, result.Summary, "complete.md")
	require.Contains(t, result.Summary, "Frontmatter: true")
	require.Contains(t, result.Summary, "Frontmatter keys: 4")

	require.Contains(t, result.Summary, "H1: 1")
	require.Contains(t, result.Summary, "H2: 4")
	require.Contains(t, result.Summary, "H3: 2")
	require.Contains(t, result.Summary, "H4: 2")
	require.Contains(t, result.Summary, "H5: 1")
	require.Contains(t, result.Summary, "H6: 1")

	require.Contains(t, result.Summary, "python: 1")
	require.Contains(t, result.Summary, "javascript: 1")

	require.Contains(t, result.Summary, "Inline links (markdown style): 2")
	require.Contains(t, result.Summary, "Reference-style links: 2")
	require.Contains(t, result.Summary, "Autolinks (http/https URLs): 1")
	require.Contains(t, result.Summary, "Reference definitions: 2")
}

func TestMarkdownExplorer_GoldenEnhancement(t *testing.T) {
	t.Parallel()

	content := `---
title: "Complete Guide"
date: 2024-01-15
tags: [tutorial, markdown, guide]
author: "Maintainer"
---

# Document Title

## Table of Contents
1. Introduction
2. Setup
3. Usage

## Introduction

This document covers everything about markdown.

### Basic Syntax

Here's some inline code ` +
		"`print()`" +
		` and a fenced block:

` +
		"```python\n" +
		"def hello():\n" +
		"    print(\"Hello, World!\")\n" +
		"```\n\n" +
		"### Advanced Features\n\n" +
		"```javascript\n" +
		"const greet = () => console.log(\"Hi\");\n" +
		"```\n\n" +
		"#### Lists\n\n" +
		"- Item 1\n" +
		"- Item 2\n" +
		"  - Nested item\n\n" +
		"#### Tables\n\n" +
		"| Column 1 | Column 2 |\n" +
		"|----------|----------|\n" +
		"| Value 1  | Value 2  |\n\n" +
		"##### Links and Images\n\n" +
		"Visit [OpenAI](https://openai.com) or [GitHub](https://github.com \"GitHub\").\n\n" +
		"See also [reference link][ref].\n" +
		"[ref]: https://example.com\n\n" +
		"Some autolinks too: <http://example.com>\n\n" +
		"###### Final Section\n\n" +
		"This is H6, the lowest level.\n\n" +
		"## References\n\n" +
		"Check out related docs: [Related][ref2]\n" +
		"[ref2]: https://docs.example.com\n\n" +
		"## Conclusion\n\n" +
		"End of document.\n"

	ctx := context.Background()
	reg := NewRegistry(WithOutputProfile(OutputProfileEnhancement))
	result, err := reg.Explore(ctx, ExploreInput{Path: "complete.md", Content: []byte(content)})
	require.NoError(t, err)

	golden.RequireEqual(t, []byte(result.Summary))
}

func TestMarkdownExplorer_GoldenParity(t *testing.T) {
	t.Parallel()

	content := `---
title: "Complete Guide"
date: 2024-01-15
tags: [tutorial, markdown, guide]
author: "Maintainer"
---

# Document Title

## Table of Contents
1. Introduction
2. Setup
3. Usage

## Introduction

This document covers everything about markdown.

### Basic Syntax

Here's some inline code ` +
		"`print()`" +
		` and a fenced block:

` +
		"```python\n" +
		"def hello():\n" +
		"    print(\"Hello, World!\")\n" +
		"```\n\n" +
		"### Advanced Features\n\n" +
		"```javascript\n" +
		"const greet = () => console.log(\"Hi\");\n" +
		"```\n\n" +
		"#### Lists\n\n" +
		"- Item 1\n" +
		"- Item 2\n" +
		"  - Nested item\n\n" +
		"#### Tables\n\n" +
		"| Column 1 | Column 2 |\n" +
		"|----------|----------|\n" +
		"| Value 1  | Value 2  |\n\n" +
		"##### Links and Images\n\n" +
		"Visit [OpenAI](https://openai.com) or [GitHub](https://github.com \"GitHub\").\n\n" +
		"See also [reference link][ref].\n" +
		"[ref]: https://example.com\n\n" +
		"Some autolinks too: <http://example.com>\n\n" +
		"###### Final Section\n\n" +
		"This is H6, the lowest level.\n\n" +
		"## References\n\n" +
		"Check out related docs: [Related][ref2]\n" +
		"[ref2]: https://docs.example.com\n\n" +
		"## Conclusion\n\n" +
		"End of document.\n"

	ctx := context.Background()
	reg := NewRegistry(WithOutputProfile(OutputProfileParity))
	result, err := reg.Explore(ctx, ExploreInput{Path: "complete.md", Content: []byte(content)})
	require.NoError(t, err)

	golden.RequireEqual(t, []byte(result.Summary))
}
