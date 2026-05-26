package explorer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFallbackExplorer_ShebangDetection verifies that FallbackExplorer detects
// shebang languages and annotates both the summary and ExplorerUsed field.
func TestFallbackExplorer_ShebangDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		content         []byte
		expectedUsed    string
		summaryContains []string
		summaryExcludes []string
	}{
		{
			name:         "python shebang detected",
			content:      []byte("#!/usr/bin/env python3\nimport sys\nprint('hello')\n"),
			expectedUsed: "fallback:shebang:python",
			summaryContains: []string{
				"Text file:",
				"detected: python via shebang",
				"import sys",
			},
		},
		{
			name:         "bash shebang detected",
			content:      []byte("#!/bin/bash\nset -euo pipefail\necho hello\n"),
			expectedUsed: "fallback:shebang:shell",
			summaryContains: []string{
				"Text file:",
				"detected: shell via shebang",
				"echo hello",
			},
		},
		{
			name:         "node shebang detected",
			content:      []byte("#!/usr/bin/env node\nconsole.log('hello');\n"),
			expectedUsed: "fallback:shebang:javascript",
			summaryContains: []string{
				"Text file:",
				"detected: javascript via shebang",
			},
		},
		{
			name:         "deno shebang detected as typescript",
			content:      []byte("#!/usr/bin/env deno\nconsole.log('hello');\n"),
			expectedUsed: "fallback:shebang:typescript",
			summaryContains: []string{
				"Text file:",
				"detected: typescript via shebang",
			},
		},
		{
			name:         "perl shebang detected",
			content:      []byte("#!/usr/bin/perl\nprint \"hello\\n\";\n"),
			expectedUsed: "fallback:shebang:perl",
			summaryContains: []string{
				"detected: perl via shebang",
			},
		},
		{
			name:         "no shebang unchanged behavior",
			content:      []byte("just some text content\nwith multiple lines\n"),
			expectedUsed: "fallback",
			summaryContains: []string{
				"Unknown text file:",
				"just some text content",
			},
			summaryExcludes: []string{
				"detected:",
				"via shebang",
			},
		},
	}

	explorer := &FallbackExplorer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := explorer.Explore(context.Background(), ExploreInput{
				// Use a path with no extension so no other explorer would
				// claim it — simulating the last-resort scenario.
				Path:    "somefile",
				Content: tt.content,
			})
			require.NoError(t, err)
			require.Equal(t, tt.expectedUsed, result.ExplorerUsed,
				"ExplorerUsed mismatch")

			for _, s := range tt.summaryContains {
				require.Contains(t, result.Summary, s,
					"summary should contain %q", s)
			}
			for _, s := range tt.summaryExcludes {
				require.NotContains(t, result.Summary, s,
					"summary should not contain %q", s)
			}
			require.Greater(t, result.TokenEstimate, 0,
				"token estimate should be positive")
		})
	}
}

// TestFallbackExplorer_BinaryWithShebang verifies that when content fails the
// looksLikeText() check (>30% non-printable), the binary path is taken and the
// shebang is not annotated. The shebang is only detectable from text content.
func TestFallbackExplorer_BinaryWithShebang(t *testing.T) {
	t.Parallel()

	// Build content that starts with a valid shebang but has enough
	// non-printable bytes to fail looksLikeText().
	// looksLikeText() samples up to 512 bytes and checks for >30%
	// non-printable (bytes < 32 excluding \t, \n, \r) or any null byte.
	// We use control characters (not null — null returns false immediately)
	// to push past the 30% threshold.
	shebang := []byte("#!/usr/bin/env python3\n")
	// Fill the rest of the first 512 bytes with ~40% control characters
	// interspersed with printable text.
	var body bytes.Buffer
	remaining := 512 - len(shebang)
	for i := range remaining {
		if i%3 == 0 {
			// Non-printable control byte (not null, not \t/\n/\r).
			body.WriteByte(0x01)
		} else {
			body.WriteByte('A')
		}
	}

	content := append(shebang, body.Bytes()...)

	// Sanity: verify this content actually fails looksLikeText.
	require.False(t, looksLikeText(content),
		"test content should fail looksLikeText")

	explorer := &FallbackExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "ambiguous",
		Content: content,
	})
	require.NoError(t, err)

	// Binary path should be taken.
	require.Equal(t, "fallback", result.ExplorerUsed,
		"binary content should use plain 'fallback'")
	require.Contains(t, result.Summary, "Unknown binary file:",
		"summary should indicate binary file")
	require.Contains(t, result.Summary, "Hex preview:",
		"summary should include hex preview")
	require.NotContains(t, result.Summary, "shebang",
		"binary path should not mention shebang")
}

// TestFallbackExplorer_NoShebangUnchanged verifies that FallbackExplorer
// behavior is unchanged when no shebang is present.
func TestFallbackExplorer_NoShebangUnchanged(t *testing.T) {
	t.Parallel()

	explorer := &FallbackExplorer{}

	t.Run("text without shebang", func(t *testing.T) {
		t.Parallel()
		content := []byte("some random text\nwithout any special header\n")
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "noext",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "fallback", result.ExplorerUsed)
		require.Contains(t, result.Summary, "Unknown text file:")
		require.NotContains(t, result.Summary, "shebang")
	})

	t.Run("binary without shebang", func(t *testing.T) {
		t.Parallel()
		// ELF header — definitely binary.
		content := []byte{0x7F, 0x45, 0x4C, 0x46, 0x01, 0x01, 0x01, 0x00}
		result, err := explorer.Explore(context.Background(), ExploreInput{
			Path:    "mystery",
			Content: content,
		})
		require.NoError(t, err)
		require.Equal(t, "fallback", result.ExplorerUsed)
		require.Contains(t, result.Summary, "Unknown binary file:")
	})
}

// TestFallbackExplorer_InterpreterMappings exercises a representative subset
// of the 31 interpreter-to-language mappings from detectShebang() through
// FallbackExplorer.
func TestFallbackExplorer_InterpreterMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shebang      string
		expectedLang string
	}{
		{"#!/usr/bin/env python3", "python"},
		{"#!/usr/bin/env python2", "python"},
		{"#!/usr/bin/python", "python"},
		{"#!/usr/bin/env node", "javascript"},
		{"#!/usr/bin/env nodejs", "javascript"},
		{"#!/usr/bin/perl", "perl"},
		{"#!/usr/bin/env php", "php"},
		{"#!/bin/bash", "shell"},
		{"#!/bin/sh", "shell"},
		{"#!/bin/zsh", "shell"},
		{"#!/usr/bin/env fish", "shell"},
		{"#!/usr/bin/env dash", "shell"},
		{"#!/usr/bin/env ksh", "shell"},
		{"#!/usr/bin/env lua", "lua"},
		{"#!/usr/bin/env swift", "swift"},
		{"#!/usr/bin/env julia", "julia"},
		{"#!/usr/bin/env elixir", "elixir"},
		{"#!/usr/bin/env deno", "typescript"},
		{"#!/usr/bin/env ts-node", "typescript"},
		{"#!/usr/bin/env Rscript", "r"},
		{"#!/usr/bin/env groovy", "groovy"},
		{"#!/usr/bin/env scala", "scala"},
		{"#!/usr/bin/env kotlin", "kotlin"},
	}

	explorer := &FallbackExplorer{}

	for _, tt := range tests {
		t.Run(tt.shebang, func(t *testing.T) {
			t.Parallel()

			content := []byte(tt.shebang + "\nsome code here\n")
			result, err := explorer.Explore(context.Background(), ExploreInput{
				Path:    "scriptfile",
				Content: content,
			})
			require.NoError(t, err)

			expectedUsed := "fallback:shebang:" + tt.expectedLang
			require.Equal(t, expectedUsed, result.ExplorerUsed,
				"shebang %q should yield explorer %q", tt.shebang, expectedUsed)
			require.Contains(t, result.Summary,
				"detected: "+tt.expectedLang+" via shebang",
				"summary should mention detected language for shebang %q", tt.shebang)
		})
	}
}

// TestFallbackExplorer_TestdataFixtures reads the testdata fixture files and
// verifies FallbackExplorer processes them correctly.
func TestFallbackExplorer_TestdataFixtures(t *testing.T) {
	t.Parallel()

	explorer := &FallbackExplorer{}
	testdataDir := filepath.Join("testdata")

	tests := []struct {
		fixture      string
		expectedUsed string
		summaryHas   string
	}{
		{
			fixture:      "shebang_python",
			expectedUsed: "fallback:shebang:python",
			summaryHas:   "detected: python via shebang",
		},
		{
			fixture:      "shebang_bash",
			expectedUsed: "fallback:shebang:shell",
			summaryHas:   "detected: shell via shebang",
		},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(filepath.Join(testdataDir, tt.fixture))
			require.NoError(t, err, "failed to read fixture %s", tt.fixture)

			result, err := explorer.Explore(context.Background(), ExploreInput{
				Path:    tt.fixture,
				Content: content,
			})
			require.NoError(t, err)
			require.Equal(t, tt.expectedUsed, result.ExplorerUsed)
			require.Contains(t, result.Summary, tt.summaryHas)
		})
	}
}

// TestFallbackExplorer_CanHandleAlwaysTrue confirms that FallbackExplorer
// always claims to handle any file — it is the last-resort explorer.
func TestFallbackExplorer_CanHandleAlwaysTrue(t *testing.T) {
	t.Parallel()

	explorer := &FallbackExplorer{}
	inputs := []struct {
		path    string
		content []byte
	}{
		{"file.go", []byte("package main")},
		{"noext", []byte("random")},
		{"binary", []byte{0x00, 0xFF}},
		{"", nil},
	}

	for _, in := range inputs {
		require.True(t, explorer.CanHandle(in.path, in.content),
			"FallbackExplorer.CanHandle should always return true for path=%q", in.path)
	}
}

// TestFallbackExplorer_LargeFileWithShebang verifies shebang annotation in
// the too-large-file path.
func TestFallbackExplorer_LargeFileWithShebang(t *testing.T) {
	t.Parallel()

	// Build content larger than MaxFullLoadSize with a python shebang.
	shebang := "#!/usr/bin/env python3\n"
	padding := strings.Repeat("x", MaxFullLoadSize+100)
	content := []byte(shebang + padding)

	explorer := &FallbackExplorer{}
	result, err := explorer.Explore(context.Background(), ExploreInput{
		Path:    "bigscript",
		Content: content,
	})
	require.NoError(t, err)
	require.Equal(t, "fallback:shebang:python", result.ExplorerUsed)
	require.Contains(t, result.Summary, "File too large:")
	require.Contains(t, result.Summary, "Detected language: python (via shebang)")
}
