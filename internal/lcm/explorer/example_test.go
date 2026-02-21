package explorer_test

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/crush/internal/lcm/explorer"
)

func ExampleRegistry_Explore_goFile() {
	content := []byte(`package main

import (
	"fmt"
)

type Server struct {
	Port int
}

func main() {
	fmt.Println("Hello")
}
`)

	reg := explorer.NewRegistry()
	result, err := reg.Explore(context.Background(), explorer.ExploreInput{
		Path:    "main.go",
		Content: content,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Explorer used: %s\n", result.ExplorerUsed)
	fmt.Printf("Token estimate: %d\n", result.TokenEstimate)
	fmt.Println("Summary:")
	fmt.Println(result.Summary)

	// Output will show:
	// Explorer used: go
	// Token estimate: (some number)
	// Summary: (Go file structure)
}

func ExampleRegistry_Explore_jsonFile() {
	content := []byte(`{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.0.0"
  }
}`)

	reg := explorer.NewRegistry()
	result, err := reg.Explore(context.Background(), explorer.ExploreInput{
		Path:    "package.json",
		Content: content,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Explorer: %s\n", result.ExplorerUsed)
	fmt.Println(result.Summary)
}

func ExampleRegistry_Explore_pythonFile() {
	content := []byte(`#!/usr/bin/env python3

import os
from typing import List

class Calculator:
    def add(self, a, b):
        return a + b

def main():
    calc = Calculator()
    print(calc.add(1, 2))
`)

	reg := explorer.NewRegistry()
	result, err := reg.Explore(context.Background(), explorer.ExploreInput{
		Path:    "calc.py",
		Content: content,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Explorer: %s\n", result.ExplorerUsed)
	fmt.Println(result.Summary)
}

func ExampleRegistry_Explore_fromFile() {
	// Reading an actual file from disk
	content, err := os.ReadFile("testdata/sample.go")
	if err != nil {
		// Handle error (file may not exist in test)
		return
	}

	reg := explorer.NewRegistry()
	result, err := reg.Explore(context.Background(), explorer.ExploreInput{
		Path:    "testdata/sample.go",
		Content: content,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("File explored with: %s\n", result.ExplorerUsed)
	fmt.Printf("Estimated tokens: %d\n", result.TokenEstimate)
}

func ExampleRegistry_Explore_withSession() {
	// Example showing forward-designed session support
	content := []byte(`package main`)

	reg := explorer.NewRegistry()
	result, err := reg.Explore(context.Background(), explorer.ExploreInput{
		Path:      "main.go",
		Content:   content,
		SessionID: "session-123", // For future LLM-enhanced exploration
		Model:     nil,           // For future LLM config
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Explorer: %s\n", result.ExplorerUsed)
}
