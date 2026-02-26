package explorer

import (
	"testing"

	"github.com/charmbracelet/crush/internal/treesitter"
	"github.com/stretchr/testify/require"
)

func TestEnrichAnalysis_Python(t *testing.T) {
	t.Parallel()

	analysis := &treesitter.FileAnalysis{
		Language: "python",
		Imports: []treesitter.ImportInfo{
			{Path: "requests"},
		},
		Symbols: []treesitter.SymbolInfo{
			{Name: "_helper", Kind: "function", Line: 3},
			{Name: "Service", Kind: "class", Line: 8},
		},
	}
	content := []byte(`import os
from .utils import helper
from dataclasses import dataclass

@dataclass
class Service:
    pass

async def stream_items():
    yield 1

if __name__ == "__main__":
    pass
`)

	enriched := EnrichAnalysis(analysis, content)
	require.NotNil(t, enriched)
	require.Equal(t, "python", enriched.Language)

	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryStdlib], "os")
	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryLocal], ".utils")
	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryThirdParty], "requests")

	require.Contains(t, enriched.Idioms, "dataclass")
	require.Contains(t, enriched.Idioms, "async_generator")
	require.Contains(t, enriched.ModulePatterns, "python_main_guard")

	require.Len(t, enriched.Symbols, 2)
	require.Equal(t, "private", enriched.Symbols[0].Visibility)
	require.Equal(t, "public", enriched.Symbols[1].Visibility)
}

func TestEnrichAnalysis_TypeScriptIdiomsAndModulePatterns(t *testing.T) {
	t.Parallel()

	analysis := &treesitter.FileAnalysis{
		Language: "typescript",
		Symbols: []treesitter.SymbolInfo{
			{Name: "MyComponent", Kind: "function", Line: 4, Modifiers: []string{"export"}},
			{Name: "_internal", Kind: "function", Line: 9},
		},
	}
	content := []byte(`import fs from "fs"
import React from "react"

export const MyComponent = () => {
  return <div>Hello</div>
}

function* seq() { yield 1 }
module.exports = { MyComponent }
export default MyComponent
`)

	enriched := EnrichAnalysis(analysis, content)
	require.NotNil(t, enriched)

	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryStdlib], "fs")
	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryThirdParty], "react")

	require.Contains(t, enriched.Idioms, "react_component")
	require.Contains(t, enriched.ModulePatterns, "commonjs_exports")
	require.Contains(t, enriched.ModulePatterns, "esm_default_export")

	require.Len(t, enriched.Symbols, 2)
	require.Equal(t, "public", enriched.Symbols[0].Visibility)
	require.Equal(t, "private", enriched.Symbols[1].Visibility)
}

func TestEnrichAnalysis_GoImportCategorizationAndVisibility(t *testing.T) {
	t.Parallel()

	analysis := &treesitter.FileAnalysis{
		Language: "go",
		Imports: []treesitter.ImportInfo{
			{Path: "fmt"},
			{Path: "github.com/pkg/errors"},
			{Path: "github.com/charmbracelet/crush/internal/lcm"},
		},
		Symbols: []treesitter.SymbolInfo{
			{Name: "ExportedFn", Kind: "function", Line: 10},
			{Name: "unexportedFn", Kind: "function", Line: 15},
		},
	}
	content := []byte(`package main

func main() {}
`)

	enriched := EnrichAnalysis(analysis, content)
	require.NotNil(t, enriched)

	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryStdlib], "fmt")
	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryThirdParty], "github.com/pkg/errors")
	require.Contains(t, enriched.ImportCategories[treesitter.ImportCategoryLocal], "github.com/charmbracelet/crush/internal/lcm")
	require.Contains(t, enriched.ModulePatterns, "go_main_package")

	require.Len(t, enriched.Symbols, 2)
	require.Equal(t, "public", enriched.Symbols[0].Visibility)
	require.Equal(t, "private", enriched.Symbols[1].Visibility)
}

func TestClassifyImportCategoriesFocused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		lang string
		imp  string
		want string
	}{
		{name: "go stdlib", lang: "go", imp: "net/http", want: treesitter.ImportCategoryStdlib},
		{name: "go third party", lang: "go", imp: "example.com/mod", want: treesitter.ImportCategoryThirdParty},
		{name: "go local repo", lang: "go", imp: "github.com/charmbracelet/crush/internal/lcm", want: treesitter.ImportCategoryLocal},
		{name: "python stdlib", lang: "python", imp: "json", want: treesitter.ImportCategoryStdlib},
		{name: "python third party", lang: "python", imp: "requests", want: treesitter.ImportCategoryThirdParty},
		{name: "python local", lang: "python", imp: ".utils", want: treesitter.ImportCategoryLocal},
		{name: "node stdlib protocol", lang: "typescript", imp: "node:fs", want: treesitter.ImportCategoryStdlib},
		{name: "node local alias", lang: "javascript", imp: "@/app/utils", want: treesitter.ImportCategoryLocal},
		{name: "node third party", lang: "javascript", imp: "react", want: treesitter.ImportCategoryThirdParty},
		{name: "c stdlib", lang: "c", imp: "stdlib.h", want: treesitter.ImportCategoryStdlib},
		{name: "cpp stdlib", lang: "cpp", imp: "vector", want: treesitter.ImportCategoryStdlib},
		{name: "cpp third party", lang: "cpp", imp: "boost/algorithm/string.hpp", want: treesitter.ImportCategoryThirdParty},
		{name: "unknown language unknown import", lang: "zig", imp: "pkg", want: treesitter.ImportCategoryUnknown},
		{name: "relative local import", lang: "zig", imp: "./mod", want: treesitter.ImportCategoryLocal},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, classifyImport(tc.lang, tc.imp))
		})
	}
}

func TestCategorizeImportsBuckets(t *testing.T) {
	t.Parallel()

	categorized := categorizeImports("go", []string{
		"fmt",
		"github.com/pkg/errors",
		"github.com/charmbracelet/crush/internal/lcm",
	})

	require.Equal(t, []string{"fmt"}, categorized[treesitter.ImportCategoryStdlib])
	require.Equal(t, []string{"github.com/pkg/errors"}, categorized[treesitter.ImportCategoryThirdParty])
	require.Equal(t, []string{"github.com/charmbracelet/crush/internal/lcm"}, categorized[treesitter.ImportCategoryLocal])
	require.NotContains(t, categorized, treesitter.ImportCategoryUnknown)
}
