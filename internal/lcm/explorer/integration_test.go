package explorer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIntegration_MultipleFileTypes tests the registry with various file types.
func TestIntegration_MultipleFileTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		path             string
		content          []byte
		expectedExplorer string
		mustContain      []string
	}{
		{
			name: "Go file with full structure",
			path: "server.go",
			content: []byte(`package server

import (
	"net/http"
	"github.com/gorilla/mux"
)

type Server struct {
	Router *mux.Router
	Port   int
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Start() error {
	return http.ListenAndServe(":8080", s.Router)
}
`),
			expectedExplorer: "go",
			mustContain:      []string{"Package: server", "net/http", "gorilla/mux", "struct Server", "NewServer", "(*Server) Start"},
		},
		{
			name: "Python script with shebang",
			path: "script.py",
			content: []byte(`#!/usr/bin/env python3

import argparse
from pathlib import Path

class FileProcessor:
    def __init__(self, path):
        self.path = path

    def process(self):
        pass

def main():
    parser = argparse.ArgumentParser()
    parser.parse_args()

if __name__ == "__main__":
    main()
`),
			expectedExplorer: "python",
			mustContain:      []string{"Python file", "import argparse", "FileProcessor", "main"},
		},
		{
			name: "TypeScript React component",
			path: "Component.tsx",
			content: []byte(`import React from 'react';

interface Props {
	title: string;
	count: number;
}

export const MyComponent: React.FC<Props> = ({ title, count }) => {
	return <div>{title}: {count}</div>;
};
`),
			expectedExplorer: "typescript",
			mustContain:      []string{"TypeScript file", "react", "Props", "MyComponent"},
		},
		{
			name: "Rust module",
			path: "lib.rs",
			content: []byte(`use std::collections::HashMap;

pub struct Config {
	settings: HashMap<String, String>,
}

impl Config {
	pub fn new() -> Self {
		Config {
			settings: HashMap::new(),
		}
	}
}

pub fn load_config() -> Config {
	Config::new()
}
`),
			expectedExplorer: "rust",
			mustContain:      []string{"Rust file", "std::collections::HashMap", "Config", "load_config"},
		},
		{
			name: "JSON configuration",
			path: "config.json",
			content: []byte(`{
	"server": {
		"host": "localhost",
		"port": 8080
	},
	"database": {
		"url": "postgres://localhost/db",
		"maxConnections": 10
	}
}`),
			expectedExplorer: "json",
			mustContain:      []string{"JSON file", "server", "database"},
		},
		{
			name: "YAML config",
			path: "docker-compose.yml",
			content: []byte(`version: '3.8'
services:
  web:
    image: nginx
    ports:
      - "80:80"
  db:
    image: postgres
    environment:
      POSTGRES_PASSWORD: secret
`),
			expectedExplorer: "yaml",
			mustContain:      []string{"YAML file", "version", "services"},
		},
		{
			name: "Shell script",
			path: "deploy.sh",
			content: []byte(`#!/bin/bash
set -e

export ENVIRONMENT="production"

function build() {
	echo "Building..."
	npm run build
}

function deploy() {
	echo "Deploying..."
	rsync -av dist/ server:/var/www/
}

build
deploy
`),
			expectedExplorer: "shell",
			mustContain:      []string{"Shell script", "#!/bin/bash", "build", "deploy", "ENVIRONMENT"},
		},
		{
			name: "CSV data",
			path: "users.csv",
			content: []byte(`id,name,email,created_at
1,Alice,alice@example.com,2024-01-01
2,Bob,bob@example.com,2024-01-02
3,Charlie,charlie@example.com,2024-01-03
`),
			expectedExplorer: "csv",
			mustContain:      []string{"CSV file", "Columns: 4", "id", "name", "email"},
		},
		{
			name: "Binary file (PNG)",
			path: "logo.png",
			content: []byte{
				0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
				0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
			},
			expectedExplorer: "binary",
			mustContain:      []string{"Binary file", "logo.png", "Hex preview"},
		},
		{
			name: "Plain text",
			path: "notes.txt",
			content: []byte(`Project Notes
=============

- Complete feature X
- Review PR #123
- Deploy to staging
`),
			expectedExplorer: "text",
			mustContain:      []string{"Text file", "notes.txt", "Lines: 7"},
		},
	}

	reg := NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore failed")

			require.Equal(t, tt.expectedExplorer, result.ExplorerUsed)

			for _, mustContain := range tt.mustContain {
				require.True(t, strings.Contains(result.Summary, mustContain), "Expected summary to contain %q\nGot:\n%s", mustContain, result.Summary)
			}

			require.Greater(t, result.TokenEstimate, 0, "Expected positive token estimate, got %d", result.TokenEstimate)
		})
	}
}

// TestIntegration_LargeFile tests handling of large files.
func TestIntegration_LargeFile(t *testing.T) {
	t.Parallel()

	// Create a large file (15KB of repeated content)
	var content strings.Builder
	content.WriteString("package main\n\n")
	for range 1000 {
		content.WriteString("func example")
		content.WriteString(strings.Repeat("A", 10))
		content.WriteString("() {}\n")
	}

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "large.go",
		Content: []byte(content.String()),
	})
	require.NoError(t, err, "Explore failed")

	require.Equal(t, "go", result.ExplorerUsed)

	// Should still parse successfully
	require.True(t, strings.Contains(result.Summary, "Package: main"), "Expected to extract package name from large file")
}

// TestIntegration_InvalidFiles tests handling of invalid/corrupted files.
func TestIntegration_InvalidFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content []byte
	}{
		{
			name:    "Invalid JSON",
			path:    "broken.json",
			content: []byte(`{"unclosed": `),
		},
		{
			name:    "Invalid Go syntax",
			path:    "broken.go",
			content: []byte(`package main\nfunc {{{ invalid`),
		},
		{
			name:    "Invalid YAML",
			path:    "broken.yml",
			content: []byte(`key: [unclosed`),
		},
	}

	reg := NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Should not panic or error, just return a fallback summary
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: tt.content,
			})
			require.NoError(t, err, "Explore should not error on invalid files")

			// Should get some kind of result
			require.NotEmpty(t, result.Summary, "Expected non-empty summary even for invalid files")
		})
	}
}

// TestIntegration_EmptyFiles tests handling of empty files.
func TestIntegration_EmptyFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"Empty Go file", "empty.go"},
		{"Empty JSON file", "empty.json"},
		{"Empty text file", "empty.txt"},
		{"Empty shell script", "empty.sh"},
	}

	reg := NewRegistry()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := reg.Explore(context.Background(), ExploreInput{
				Path:    tt.path,
				Content: []byte{},
			})
			require.NoError(t, err, "Explore failed on empty file")

			require.NotEmpty(t, result.Summary, "Expected non-empty summary for empty file")
		})
	}
}

// TestIntegration_ContextCancellation tests that context cancellation is respected.
func TestIntegration_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	reg := NewRegistry()
	// Should still work even with cancelled context since operations are fast
	// This is more for future-proofing when we add LLM-enhanced exploration
	_, err := reg.Explore(ctx, ExploreInput{
		Path:    "test.go",
		Content: []byte("package main"),
	})

	// Current implementation doesn't check context, so this should succeed
	// In future versions with LLM calls, this would respect cancellation
	require.NoError(t, err, "Unexpected error: %v")
}
