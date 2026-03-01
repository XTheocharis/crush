package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONExplorer(t *testing.T) {
	content := []byte(`{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.17.1",
    "lodash": "^4.17.21"
  },
  "scripts": {
    "start": "node index.js",
    "test": "jest"
  }
}`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "package.json",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "json", result.ExplorerUsed, "Expected json explorer")

	expectations := []string{
		"JSON file",
		"name",
		"version",
		"dependencies",
	}

	for _, exp := range expectations {
		require.True(t, strings.Contains(result.Summary, exp), "Expected summary to contain %q", exp)
	}
}

func TestCSVExplorer(t *testing.T) {
	content := []byte(`name,age,city
Alice,30,New York
Bob,25,San Francisco
Charlie,35,Chicago`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "data.csv",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "csv", result.ExplorerUsed, "Expected csv explorer")

	expectations := []string{
		"CSV file",
		"Rows: 4",
		"Columns: 3",
		"name",
		"age",
		"city",
	}

	for _, exp := range expectations {
		require.True(t, strings.Contains(result.Summary, exp), "Expected summary to contain %q", exp)
	}
}

func TestShellExplorer(t *testing.T) {
	content := []byte(`#!/bin/bash

source ./common.sh

export DATABASE_URL="postgres://localhost"

function deploy() {
    echo "Deploying..."
}

function cleanup() {
    echo "Cleaning up..."
}

deploy
cleanup
`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "deploy.sh",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "shell", result.ExplorerUsed, "Expected shell explorer")

	expectations := []string{
		"Shell script",
		"#!/bin/bash",
		"./common.sh",
		"deploy",
		"cleanup",
		"DATABASE_URL",
	}

	for _, exp := range expectations {
		require.True(t, strings.Contains(result.Summary, exp), "Expected summary to contain %q", exp)
	}
}

func TestBinaryExplorer(t *testing.T) {
	// Binary content (PNG header)
	content := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "image.png",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "binary", result.ExplorerUsed, "Expected binary explorer")

	require.True(t, strings.Contains(result.Summary, "Binary file"), "Expected summary to contain 'Binary file'")
}

func TestTextExplorer(t *testing.T) {
	content := []byte(`This is a plain text file.
It has multiple lines.
And contains some information.`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "README.txt",
		Content: content,
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "text", result.ExplorerUsed, "Expected text explorer")

	require.True(t, strings.Contains(result.Summary, "Text file"), "Expected summary to contain 'Text file'")
}

func TestShebangDetection(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "python shebang",
			content:  []byte("#!/usr/bin/env python3\nprint('hello')"),
			expected: "python",
		},
		{
			name:     "bash shebang",
			content:  []byte("#!/bin/bash\necho hello"),
			expected: "shell",
		},
		{
			name:     "node shebang",
			content:  []byte("#!/usr/bin/env node\nconsole.log('hello')"),
			expected: "javascript",
		},
		{
			name:     "no shebang",
			content:  []byte("echo hello"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectShebang(tt.content)
			require.Equal(t, tt.expected, result, "detectShebang() = %q, expected %q", result, tt.expected)
		})
	}
}

func TestLooksLikeText(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "plain text",
			content:  []byte("Hello, world!"),
			expected: true,
		},
		{
			name:     "binary with null byte",
			content:  []byte{0x00, 0x01, 0x02},
			expected: false,
		},
		{
			name:     "text with newlines",
			content:  []byte("Line 1\nLine 2\nLine 3"),
			expected: true,
		},
		{
			name:     "empty content",
			content:  []byte{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := looksLikeText(tt.content)
			require.Equal(t, tt.expected, result, "looksLikeText() = %v, expected %v", result, tt.expected)
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "empty",
			content:  "",
			expected: 0,
		},
		{
			name:     "short text",
			content:  "test",
			expected: 1,
		},
		{
			name:     "longer text",
			content:  "This is a test string",
			expected: 6, // 21 chars / 4 = 5.25, rounds up to 6
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokens(tt.content)
			require.Equal(t, tt.expected, result, "estimateTokens(%q) = %d, expected %d", tt.content, result, tt.expected)
		})
	}
}

func TestSampleContent(t *testing.T) {
	longContent := []byte(strings.Repeat("a", 20000))

	result, sampled := sampleContent(longContent, 9000)

	require.True(t, sampled, "Expected content to be sampled")

	require.True(t, strings.Contains(result, "[SAMPLED]"), "Expected sampled content to contain [SAMPLED] marker")

	// Should have begin + middle + end sections
	parts := strings.Split(result, "[SAMPLED]")
	require.Equal(t, 3, len(parts), "Expected 3 parts (begin, middle, end), got %d", len(parts))
}

// TestFallbackChainOrder verifies Phase 2A fallback chain ordering:
// Binary -> JSON/CSV/YAML/TOML/INI/XML/HTML/Markdown/LaTeX/SQLite/Logs -> TreeSitterExplorer -> Shell -> Text -> Fallback.
func TestFallbackChainOrder(t *testing.T) {
	reg := NewRegistry()

	mustBeBefore := func(before, after any) {
		t.Helper()
		beforeIdx, afterIdx := -1, -1
		for i, e := range reg.explorers {
			switch before.(type) {
			case *BinaryExplorer:
				if _, ok := e.(*BinaryExplorer); ok {
					beforeIdx = i
				}
			case *JSONExplorer:
				if _, ok := e.(*JSONExplorer); ok {
					beforeIdx = i
				}
			case *HTMLExplorer:
				if _, ok := e.(*HTMLExplorer); ok {
					beforeIdx = i
				}
			case *ShellExplorer:
				if _, ok := e.(*ShellExplorer); ok {
					beforeIdx = i
				}
			case *TextExplorer:
				if _, ok := e.(*TextExplorer); ok {
					beforeIdx = i
				}
			}

			switch after.(type) {
			case *JSONExplorer:
				if _, ok := e.(*JSONExplorer); ok {
					afterIdx = i
				}
			case *ShellExplorer:
				if _, ok := e.(*ShellExplorer); ok {
					afterIdx = i
				}
			case *TextExplorer:
				if _, ok := e.(*TextExplorer); ok {
					afterIdx = i
				}
			case *FallbackExplorer:
				if _, ok := e.(*FallbackExplorer); ok {
					afterIdx = i
				}
			}
		}
		require.GreaterOrEqualf(t, beforeIdx, 0, "did not find expected explorer (%T)", before)
		require.GreaterOrEqualf(t, afterIdx, 0, "did not find expected explorer (%T)", after)
		require.Lessf(t, beforeIdx, afterIdx, "expected %T before %T, got indexes %d >= %d", before, after, beforeIdx, afterIdx)
	}

	mustBeBefore(&BinaryExplorer{}, &JSONExplorer{})
	mustBeBefore(&HTMLExplorer{}, &ShellExplorer{})
	mustBeBefore(&ShellExplorer{}, &TextExplorer{})
	mustBeBefore(&TextExplorer{}, &FallbackExplorer{})

	mockParser := &mockTreeSitterParser{supports: map[string]bool{}, hasTags: map[string]bool{}}
	regWithTS := NewRegistry(WithTreeSitter(mockParser))

	logsIdx, tsIdx, shellIdx := -1, -1, -1
	for i, e := range regWithTS.explorers {
		if _, ok := e.(*LogsExplorer); ok {
			logsIdx = i
		}
		if _, ok := e.(*TreeSitterExplorer); ok {
			tsIdx = i
		}
		if _, ok := e.(*ShellExplorer); ok {
			shellIdx = i
		}
	}
	require.GreaterOrEqualf(t, logsIdx, 0, "expected logs explorer in chain")
	require.GreaterOrEqualf(t, tsIdx, 0, "expected treesitter explorer in chain")
	require.GreaterOrEqualf(t, shellIdx, 0, "expected shell explorer in chain")
	require.Lessf(t, logsIdx, tsIdx, "expected ordering Logs < TreeSitter, got logs=%d treesitter=%d", logsIdx, tsIdx)
	require.Lessf(t, tsIdx, shellIdx, "expected ordering TreeSitter < Shell, got treesitter=%d shell=%d", tsIdx, shellIdx)
}

// TestDispatchPriority verifies that file types are dispatched to the correct
// explorer according to the fallback chain priority.
func TestDispatchPriority(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		content          []byte
		expectedExplorer string
	}{
		{
			name:             "PNG binary file",
			path:             "image.png",
			content:          []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expectedExplorer: "binary",
		},
		{
			name:             "JSON file",
			path:             "config.json",
			content:          []byte(`{"key": "value"}`),
			expectedExplorer: "json",
		},
		{
			name:             "CSV file",
			path:             "data.csv",
			content:          []byte("a,b,c\n1,2,3"),
			expectedExplorer: "csv",
		},
		{
			name:             "YAML file",
			path:             "config.yaml",
			content:          []byte("key: value\n"),
			expectedExplorer: "yaml",
		},
		{
			name:             "TOML file",
			path:             "config.toml",
			content:          []byte("[section]\nkey = \"value\"\n"),
			expectedExplorer: "toml",
		},
		{
			name:             "INI file",
			path:             "config.ini",
			content:          []byte("[section]\nkey=value\n"),
			expectedExplorer: "ini",
		},
		{
			name:             "XML file",
			path:             "data.xml",
			content:          []byte("<root><item/></root>\n"),
			expectedExplorer: "xml",
		},
		{
			name:             "HTML file",
			path:             "index.html",
			content:          []byte("<html><body></body></html>\n"),
			expectedExplorer: "html",
		},
		{
			name:             "Markdown file",
			path:             "README.md",
			content:          []byte("# Title\n\nSome markdown content."),
			expectedExplorer: "markdown",
		},
		{
			name:             "LaTeX file",
			path:             "paper.tex",
			content:          []byte("\\section{Intro}\nSome LaTeX content."),
			expectedExplorer: "latex",
		},
		{
			name:             "SQLite file by extension",
			path:             "database.db",
			content:          []byte("not really sqlite"),
			expectedExplorer: "sqlite",
		},
		{
			name:             "Log file",
			path:             "app.log",
			content:          []byte("[ERROR] something failed\n[INFO] started"),
			expectedExplorer: "logs",
		},
		{
			name:             "Shell script with extension",
			path:             "script.sh",
			content:          []byte("#!/bin/bash\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "Shell script with shebang, no extension",
			path:             "myscript",
			content:          []byte("#!/bin/bash\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "Go file (text fallback without tree-sitter)",
			path:             "main.go",
			content:          []byte("package main\nfunc main() {}\n"),
			expectedExplorer: "text",
		},
		{
			name:             "Plain text file",
			path:             "README.txt",
			content:          []byte("This is a plain text file.\n"),
			expectedExplorer: "text",
		},
		{
			name:             "Unknown text file",
			path:             "unknown.xyz",
			content:          []byte("Some text content\n"),
			expectedExplorer: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry()
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore failed")
			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed, "Expected explorer %q, got %q", tt.expectedExplorer, result.ExplorerUsed)
		})
	}
}

// TestShebangDispatchParity verifies that shebang detection and extension
// dispatch maintain parity behavior across all relevant explorers.
func TestShebangDispatchParity(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		shebang          []byte
		expectedExplorer string
	}{
		{
			name:             "bash shebang",
			path:             "script.sh",
			shebang:          []byte("#!/bin/bash\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "bash shebang without extension",
			path:             "myscript",
			shebang:          []byte("#!/bin/bash\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "sh shebang",
			path:             "script",
			shebang:          []byte("#!/bin/sh\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "zsh shebang",
			path:             "zscript",
			shebang:          []byte("#!/bin/zsh\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "fish shebang",
			path:             "fishscript",
			shebang:          []byte("#!/usr/bin/env fish\necho hello\n"),
			expectedExplorer: "shell",
		},
		{
			name:             "python shebang without extension (text fallback)",
			path:             "pyscript",
			shebang:          []byte("#!/usr/bin/env python3\nprint('hello')\n"),
			expectedExplorer: "text",
		},
		{
			name:             "python shebang with py extension (text fallback)",
			path:             "script.py",
			shebang:          []byte("#!/usr/bin/env python3\nprint('hello')\n"),
			expectedExplorer: "text",
		},
		{
			name:             "node shebang without extension (text fallback)",
			path:             "nodescript",
			shebang:          []byte("#!/usr/bin/env node\nconsole.log('hello')\n"),
			expectedExplorer: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry()
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.shebang,
			})
			require.NoError(t, err, "Explore failed")
			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed, "Expected explorer %q, got %q", tt.expectedExplorer, result.ExplorerUsed)
		})
	}
}

// TestFallbackExplorerHandlesEverything verifies that FallbackExplorer
// correctly handles all file types that don't match other explorers.
func TestFallbackExplorerHandlesEverything(t *testing.T) {
	reg := NewRegistry()

	tests := []struct {
		name    string
		path    string
		content []byte
	}{
		{
			name:    "Go code",
			path:    "main.go",
			content: []byte("package main\nfunc main() {}\n"),
		},
		{
			name:    "Python code",
			path:    "test.py",
			content: []byte("print('hello')\n"),
		},
		{
			name:    "JavaScript code",
			path:    "script.js",
			content: []byte("console.log('hello');\n"),
		},
		{
			name:    "TypeScript code",
			path:    "file.ts",
			content: []byte("const x: string = 'hello';\n"),
		},
		{
			name:    "Rust code",
			path:    "main.rs",
			content: []byte("fn main() {}\n"),
		},
		{
			name:    "Java code",
			path:    "Main.java",
			content: []byte("class Main {}\n"),
		},
		{
			name:    "C++ code",
			path:    "test.cpp",
			content: []byte("int main() {}\n"),
		},
		{
			name:    "C code",
			path:    "test.c",
			content: []byte("int main(void) {}\n"),
		},
		{
			name:    "Ruby code",
			path:    "test.rb",
			content: []byte("puts 'hello'\n"),
		},
		{
			name:    "Unknown file type",
			path:    "unknown.xyz",
			content: []byte("some content\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore failed")
			// Should either be handled by a specific explorer or fall back
			if result.ExplorerUsed != "fallback" {
				// Verify we got a valid result from another explorer
				require.NotEmpty(t, result.Summary, "Expected non-empty summary for %s (got explorer %s)", tt.name, result.ExplorerUsed)
			}
		})
	}
}

// TestBinaryPriorityOverDataFormats verifies that binary files are handled
// by BinaryExplorer before any data format explorers.
func TestBinaryPriorityOverDataFormats(t *testing.T) {
	reg := NewRegistry()

	tests := []struct {
		name             string
		path             string
		content          []byte
		expectedExplorer string
	}{
		{
			name:             "PNG header",
			path:             "image.png",
			content:          []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expectedExplorer: "binary",
		},
		{
			name:             "JPEG header",
			path:             "photo.jpg",
			content:          []byte{0xFF, 0xD8, 0xFF, 0xE0},
			expectedExplorer: "binary",
		},
		{
			name:             "PDF header",
			path:             "document.pdf",
			content:          []byte("%PDF-1.4\n"),
			expectedExplorer: "binary",
		},
		{
			name:             "ZIP header",
			path:             "archive.zip",
			content:          []byte{0x50, 0x4B, 0x03, 0x04},
			expectedExplorer: "binary",
		},
		{
			name:             "ELF header",
			path:             "binary",
			content:          []byte{0x7F, 0x45, 0x4C, 0x46},
			expectedExplorer: "binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore failed")
			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed, "Expected explorer %q, got %q", tt.expectedExplorer, result.ExplorerUsed)
		})
	}
}
