//go:build ignore

// Package qaexplorer provides a deterministic fixture for testing the
// explorer subsystem's semantic code analysis. This file is never compiled
// (//go:build ignore); it exists solely as parseable content for the
// tree-sitter-based explorer.
package qaexplorer

import "fmt"

// FixtureStruct is an exported struct with two fields used in QA assertions.
type FixtureStruct struct {
	Name  string
	Value int
}

// FixtureFunc returns a deterministic integer value for testing.
func FixtureFunc() int {
	fmt.Println("qaexplorer fixture")
	return 42
}
