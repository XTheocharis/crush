package explorer

import (
	"context"
	"strings"
	"testing"
)

func TestGoExplorer(t *testing.T) {
	content := []byte(`package main

import (
	"fmt"
	"strings"
	"github.com/external/lib"
	"github.com/charmbracelet/crush/internal/config"
)

type Server struct {
	Port int
}

type Handler interface {
	Handle() error
}

const MaxConnections = 100

var DefaultPort = 8080

func main() {
	fmt.Println("Hello")
}

func (s *Server) Start() error {
	return nil
}
`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "test.go",
		Content: content,
	})
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "go" {
		t.Errorf("Expected go explorer, got %s", result.ExplorerUsed)
	}

	// Check for expected content
	expectations := []string{
		"Package: main",
		"fmt",
		"strings",
		"external/lib",
		"crush/internal/config",
		"struct Server",
		"interface Handler",
		"MaxConnections",
		"DefaultPort",
		"main",
		"(*Server) Start",
	}

	for _, exp := range expectations {
		if !strings.Contains(result.Summary, exp) {
			t.Errorf("Expected summary to contain %q, got:\n%s", exp, result.Summary)
		}
	}
}

func TestPythonExplorer(t *testing.T) {
	content := []byte(`#!/usr/bin/env python3
import os
import sys
from typing import List

class Calculator:
    def __init__(self):
        pass

    def add(self, a, b):
        return a + b

def main():
    calc = Calculator()
    print(calc.add(1, 2))

if __name__ == "__main__":
    main()
`)

	reg := NewRegistry()
	result, err := reg.Explore(context.Background(), ExploreInput{
		Path:    "test.py",
		Content: content,
	})
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "python" {
		t.Errorf("Expected python explorer, got %s", result.ExplorerUsed)
	}

	expectations := []string{
		"Python file",
		"import os",
		"Calculator",
		"main",
	}

	for _, exp := range expectations {
		if !strings.Contains(result.Summary, exp) {
			t.Errorf("Expected summary to contain %q", exp)
		}
	}
}

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
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "json" {
		t.Errorf("Expected json explorer, got %s", result.ExplorerUsed)
	}

	expectations := []string{
		"JSON file",
		"name",
		"version",
		"dependencies",
	}

	for _, exp := range expectations {
		if !strings.Contains(result.Summary, exp) {
			t.Errorf("Expected summary to contain %q", exp)
		}
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
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "csv" {
		t.Errorf("Expected csv explorer, got %s", result.ExplorerUsed)
	}

	expectations := []string{
		"CSV file",
		"Rows: 4",
		"Columns: 3",
		"name",
		"age",
		"city",
	}

	for _, exp := range expectations {
		if !strings.Contains(result.Summary, exp) {
			t.Errorf("Expected summary to contain %q", exp)
		}
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
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "shell" {
		t.Errorf("Expected shell explorer, got %s", result.ExplorerUsed)
	}

	expectations := []string{
		"Shell script",
		"#!/bin/bash",
		"./common.sh",
		"deploy",
		"cleanup",
		"DATABASE_URL",
	}

	for _, exp := range expectations {
		if !strings.Contains(result.Summary, exp) {
			t.Errorf("Expected summary to contain %q", exp)
		}
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
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "binary" {
		t.Errorf("Expected binary explorer, got %s", result.ExplorerUsed)
	}

	if !strings.Contains(result.Summary, "Binary file") {
		t.Errorf("Expected summary to contain 'Binary file'")
	}
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
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	if result.ExplorerUsed != "text" {
		t.Errorf("Expected text explorer, got %s", result.ExplorerUsed)
	}

	if !strings.Contains(result.Summary, "Text file") {
		t.Errorf("Expected summary to contain 'Text file'")
	}
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
			if result != tt.expected {
				t.Errorf("detectShebang() = %q, expected %q", result, tt.expected)
			}
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
			if result != tt.expected {
				t.Errorf("looksLikeText() = %v, expected %v", result, tt.expected)
			}
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
			if result != tt.expected {
				t.Errorf("estimateTokens(%q) = %d, expected %d", tt.content, result, tt.expected)
			}
		})
	}
}

func TestSampleContent(t *testing.T) {
	longContent := []byte(strings.Repeat("a", 20000))

	result, sampled := sampleContent(longContent, 9000)

	if !sampled {
		t.Error("Expected content to be sampled")
	}

	if !strings.Contains(result, "[SAMPLED]") {
		t.Error("Expected sampled content to contain [SAMPLED] marker")
	}

	// Should have begin + middle + end sections
	parts := strings.Split(result, "[SAMPLED]")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts (begin, middle, end), got %d", len(parts))
	}
}
