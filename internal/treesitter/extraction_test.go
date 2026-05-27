//go:build treesitter

package treesitter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// --- ImportInfo extraction tests ---

func TestExtractImportsGo(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

import "fmt"
import "strings"

import (
	"os"
	"net/http"
)

func main() {}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, _, err := loader.ExtractTags("go", "main.go", tree.RootNode(), src)
	require.NoError(t, err)
	_ = tags

	imports := extractImports("go", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["fmt"], "missing import fmt")
	require.True(t, paths["strings"], "missing import strings")
	require.True(t, paths["os"], "missing import os")
	require.True(t, paths["net/http"], "missing import net/http")
}

func TestExtractImportsPython(t *testing.T) {
	t.Parallel()

	pyLang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(pyLang))

	src := []byte(`import os
import sys
from collections import OrderedDict
from typing import List, Dict
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("python", pyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	imports := extractImports("python", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["os"], "missing import os")
	require.True(t, paths["sys"], "missing import sys")
	require.True(t, paths["collections"], "missing import from collections")
	require.True(t, paths["typing"], "missing import from typing")
}

func TestExtractImportsTypeScript(t *testing.T) {
	t.Parallel()

	tsLang := tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(tsLang))

	src := []byte(`import { readFileSync } from "fs";
import * as path from "path";
const expr = require("express");
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("typescript", tsLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	imports := extractImports("typescript", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["fs"], "missing import fs")
	require.True(t, paths["path"], "missing import path")
	require.True(t, paths["express"], "missing require express")
}

func TestExtractImportsEmptySource(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("go", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports from empty source")
}

// --- SymbolInfo completeness tests ---

func TestSymbolInfoGoFunctionWithDetails(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

// Add returns the sum of two integers.
func Add(a int, b int) int {
	return a + b
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("go", "math.go", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, symbols, "expected at least one symbol")

	var add *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "Add" {
			add = &symbols[i]
			break
		}
	}
	require.NotNil(t, add, "expected to find Add function symbol")
	require.Equal(t, "function", add.Kind)
	require.NotEmpty(t, add.Params, "expected Params to be populated for Add")
	require.Equal(t, "int", add.ReturnType, "expected ReturnType to be populated")
	require.NotEmpty(t, add.DocComment, "expected DocComment to be populated")
}

func TestSymbolInfoGoMethodWithParent(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

type Server struct{}

// Start boots the server.
func (s *Server) Start(addr string) error {
	return nil
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("go", "server.go", tree.RootNode(), src)
	require.NoError(t, err)

	var start *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "Start" {
			start = &symbols[i]
			break
		}
	}
	require.NotNil(t, start, "expected to find Start method symbol")
	require.Equal(t, "method", start.Kind)
	require.Equal(t, "Server", start.Parent, "expected Parent to be Server")
	require.NotEmpty(t, start.Params, "expected Params to be populated")
	require.Equal(t, "error", start.ReturnType, "expected ReturnType to be error")
	require.NotEmpty(t, start.DocComment, "expected DocComment to be populated")
}

func TestSymbolInfoPythonFunctionWithDetails(t *testing.T) {
	t.Parallel()

	pyLang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(pyLang))

	src := []byte(`def greet(name: str, loud: bool = False) -> str:
    """Say hello to someone."""
    return f"Hello {name}"
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("python", pyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("python", "hello.py", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, symbols, "expected at least one symbol")

	var greet *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "greet" {
			greet = &symbols[i]
			break
		}
	}
	require.NotNil(t, greet, "expected to find greet function symbol")
	require.Equal(t, "function", greet.Kind)
	require.NotEmpty(t, greet.Params, "expected Params to be populated")
	require.Equal(t, "str", greet.ReturnType, "expected ReturnType to be populated")
	require.NotEmpty(t, greet.DocComment, "expected DocComment to be populated")
}

func TestSymbolInfoPythonDecoratedFunction(t *testing.T) {
	t.Parallel()

	pyLang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(pyLang))

	src := []byte(`class MyHandler:
    @staticmethod
    @cache
    def process(data: bytes) -> bool:
        """Process the data."""
        return True
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("python", pyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("python", "handler.py", tree.RootNode(), src)
	require.NoError(t, err)

	var process *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "process" {
			process = &symbols[i]
			break
		}
	}
	require.NotNil(t, process, "expected to find process function symbol")
	require.Equal(t, "MyHandler", process.Parent, "expected Parent to be MyHandler")
	require.Contains(t, process.Decorators, "staticmethod", "expected staticmethod decorator")
	require.Contains(t, process.Decorators, "cache", "expected cache decorator")
	require.NotEmpty(t, process.Params, "expected Params to be populated")
	require.Equal(t, "bool", process.ReturnType)
	require.NotEmpty(t, process.DocComment, "expected DocComment")
}

func TestSymbolInfoTypeScriptFunctionWithDetails(t *testing.T) {
	t.Parallel()

	tsLang := tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(tsLang))

	src := []byte(`function add(a: number, b: number): number {
  return a + b;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("typescript", tsLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("typescript", "add.ts", tree.RootNode(), src)
	require.NoError(t, err)
	require.NotEmpty(t, symbols, "expected at least one symbol")

	var add *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "add" {
			add = &symbols[i]
			break
		}
	}
	require.NotNil(t, add, "expected to find add function symbol")
	require.Equal(t, "function", add.Kind)
	require.NotEmpty(t, add.Params, "expected Params to be populated")
	require.Equal(t, "number", add.ReturnType, "expected ReturnType to be populated")
}

func TestSymbolInfoGoStructAndInterface(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

// Reader reads stuff.
type Reader interface{}

// Config holds settings.
type Config struct{}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("go", "types.go", tree.RootNode(), src)
	require.NoError(t, err)

	names := make(map[string]*SymbolInfo)
	for i := range symbols {
		names[symbols[i].Name] = &symbols[i]
	}

	reader, ok := names["Reader"]
	require.True(t, ok, "expected Reader symbol")
	if ok {
		require.Equal(t, "interface", reader.Kind)
		require.NotEmpty(t, reader.DocComment, "expected DocComment for Reader")
	}

	config, ok := names["Config"]
	require.True(t, ok, "expected Config symbol")
	if ok {
		require.Equal(t, "class", config.Kind)
		require.NotEmpty(t, config.DocComment, "expected DocComment for Config")
	}
}

func TestSymbolInfoGoExistingFieldsUnchanged(t *testing.T) {
	t.Parallel()

	goLang := tree_sitter.NewLanguage(tree_sitter_go.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(goLang))

	src := []byte(`package main

func Run() {
	Run()
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("go", goLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	tags, symbols, err := loader.ExtractTags("go", "main.go", tree.RootNode(), src)
	require.NoError(t, err)

	// Existing tag extraction must remain unchanged.
	hasRunDef := false
	hasRunRef := false
	for _, tag := range tags {
		if tag.Name == "Run" && tag.Kind == "def" && tag.NodeType == "function" {
			hasRunDef = true
		}
		if tag.Name == "Run" && tag.Kind == "ref" && tag.NodeType == "call" {
			hasRunRef = true
		}
	}
	require.True(t, hasRunDef, "expected Run def tag")
	require.True(t, hasRunRef, "expected Run ref tag")

	// Symbol must still have basic fields.
	require.NotEmpty(t, symbols)
	var run *SymbolInfo
	for i := range symbols {
		if symbols[i].Name == "Run" {
			run = &symbols[i]
			break
		}
	}
	require.NotNil(t, run)
	require.Equal(t, "function", run.Kind)
	require.GreaterOrEqual(t, run.Line, 1)
	require.GreaterOrEqual(t, run.EndLine, run.Line)
}

func TestSymbolInfoPythonClassAsParent(t *testing.T) {
	t.Parallel()

	pyLang := tree_sitter.NewLanguage(tree_sitter_python.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(pyLang))

	src := []byte(`class Animal:
    """An animal."""

    def speak(self) -> str:
        """Make a sound."""
        return ""

    @property
    def name(self) -> str:
        """Get name."""
        return ""
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("python", pyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("python", "animal.py", tree.RootNode(), src)
	require.NoError(t, err)

	symMap := make(map[string]*SymbolInfo)
	for i := range symbols {
		symMap[symbols[i].Name] = &symbols[i]
	}

	speak, ok := symMap["speak"]
	require.True(t, ok, "expected speak symbol")
	if ok {
		require.Equal(t, "Animal", speak.Parent, "expected Parent=Animal for speak")
		require.NotEmpty(t, speak.Params, "expected Params for speak")
		require.Equal(t, "str", speak.ReturnType)
		require.NotEmpty(t, speak.DocComment)
	}

	name, ok := symMap["name"]
	require.True(t, ok, "expected name symbol")
	if ok {
		require.Equal(t, "Animal", name.Parent, "expected Parent=Animal for name")
		require.Contains(t, name.Decorators, "property", "expected property decorator")
	}
}

func TestSymbolInfoTypeScriptClassWithModifiers(t *testing.T) {
	t.Parallel()

	tsLang := tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(tsLang))

	src := []byte(`export class UserService {
  private data: string[] = [];

  public async fetchData(url: string): Promise<string> {
    return "";
  }
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("typescript", tsLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	_, symbols, err := loader.ExtractTags("typescript", "service.ts", tree.RootNode(), src)
	require.NoError(t, err)

	symMap := make(map[string]*SymbolInfo)
	for i := range symbols {
		symMap[symbols[i].Name] = &symbols[i]
	}

	svc, ok := symMap["UserService"]
	require.True(t, ok, "expected UserService symbol")
	if ok {
		require.Equal(t, "class", svc.Kind)
		require.Contains(t, svc.Modifiers, "export", "expected export modifier")
	}

	fetchData, ok := symMap["fetchData"]
	require.True(t, ok, "expected fetchData symbol")
	if ok {
		require.Contains(t, fetchData.Modifiers, "public", "expected public modifier")
		require.Contains(t, fetchData.Modifiers, "async", "expected async modifier")
		require.NotEmpty(t, fetchData.Params, "expected Params for fetchData")
	}
}

func TestExtractScalaImports(t *testing.T) {
	t.Parallel()

	scalaLang := tree_sitter.NewLanguage(tree_sitter_scala.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(scalaLang))

	src := []byte(`import scala.collection.mutable.HashMap
import java.util.List
import javax.servlet.http.HttpServletRequest
import org.apache.http.HttpRequest
import scala.concurrent.Future

object Main {
  def main(args: Array[String]): Unit = {}
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("scala", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["scala.collection.mutable.HashMap"], "missing import scala.collection.mutable.HashMap")
	require.True(t, paths["java.util.List"], "missing import java.util.List")
	require.True(t, paths["javax.servlet.http.HttpServletRequest"], "missing import javax.servlet.http.HttpServletRequest")
	require.True(t, paths["org.apache.http.HttpRequest"], "missing import org.apache.http.HttpRequest")
	require.True(t, paths["scala.concurrent.Future"], "missing import scala.concurrent.Future")

	for _, imp := range imports {
		switch {
		case strings.HasPrefix(imp.Path, "scala."):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "java."):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "javax."):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "org."):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractScalaEmpty(t *testing.T) {
	t.Parallel()

	scalaLang := tree_sitter.NewLanguage(tree_sitter_scala.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(scalaLang))

	src := []byte(`object Main {
  def main(args: Array[String]): Unit = {}
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("scala", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports from source without imports")
}

func TestClassifyKotlinImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		category string
	}{
		{"kotlin.collections.List", ImportCategoryStdlib},
		{"java.util.ArrayList", ImportCategoryStdlib},
		{"javax.swing.JFrame", ImportCategoryStdlib},
		{"android.os.Bundle", ImportCategoryStdlib},
		{"org.jetbrains.kotlin.Kotlin", ImportCategoryThirdParty},
		{"com.example.MyClass", ImportCategoryThirdParty},
	}
	for _, tt := range tests {
		require.Equal(t, tt.category, classifyImport("kotlin", tt.path),
			"classifyImport(%q, %q)", "kotlin", tt.path)
	}
}

func TestClassifySwiftImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		category string
	}{
		{"Foundation", ImportCategoryStdlib},
		{"UIKit", ImportCategoryStdlib},
		{"SwiftUI", ImportCategoryStdlib},
		{"Combine", ImportCategoryStdlib},
		{"XCTest", ImportCategoryStdlib},
		{"CoreData", ImportCategoryStdlib},
		{"Security", ImportCategoryStdlib},
		{"Network", ImportCategoryStdlib},
		{"Darwin", ImportCategoryStdlib},
		{"Glibc", ImportCategoryStdlib},
		{"Alamofire", ImportCategoryThirdParty},
		{"MyModule", ImportCategoryThirdParty},
		{"Kingfisher", ImportCategoryThirdParty},
	}
	for _, tt := range tests {
		require.Equal(t, tt.category, classifyImport("swift", tt.path),
			"classifyImport(%q, %q)", "swift", tt.path)
	}
}

func TestClassifyScalaImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		category string
	}{
		{"scala.collection.mutable.HashMap", ImportCategoryStdlib},
		{"scala.concurrent.Future", ImportCategoryStdlib},
		{"java.util.List", ImportCategoryStdlib},
		{"javax.servlet.http.HttpServletRequest", ImportCategoryStdlib},
		{"org.apache.http.HttpRequest", ImportCategoryThirdParty},
		{"com.example.MyClass", ImportCategoryThirdParty},
	}
	for _, tt := range tests {
		require.Equal(t, tt.category, classifyImport("scala", tt.path),
			"classifyImport(%q, %q)", "scala", tt.path)
	}
}

func TestAnalyzeReturnsImports(t *testing.T) {
	t.Parallel()

	pr := NewParser()
	t.Cleanup(func() { require.NoError(t, pr.Close()) })

	src := []byte(`package main

import "fmt"
import "os"

func main() {
	fmt.Println("hello")
}
`)

	analysis, err := pr.Analyze(t.Context(), "/tmp/main.go", src)
	require.NoError(t, err)
	require.NotNil(t, analysis)
	require.NotEmpty(t, analysis.Imports, "expected Analyze to return imports")

	paths := make(map[string]bool)
	for _, imp := range analysis.Imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["fmt"], "missing import fmt")
	require.True(t, paths["os"], "missing import os")
}

func TestExtractImportsJava(t *testing.T) {
	t.Parallel()

	javaLang := tree_sitter.NewLanguage(tree_sitter_java.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(javaLang))

	src := []byte(`package com.example;

import java.util.List;
import java.util.ArrayList;
import org.apache.http.HttpRequest;

public class Main {
    public static void main(String[] args) {}
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("java", javaLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	imports := extractImports("java", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["java.util.List"], "missing import java.util.List")
	require.True(t, paths["java.util.ArrayList"], "missing import java.util.ArrayList")
	require.True(t, paths["org.apache.http.HttpRequest"], "missing import org.apache.http.HttpRequest")

	for _, imp := range imports {
		switch {
		case strings.HasPrefix(imp.Path, "java."):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "org."):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractImportsRust(t *testing.T) {
	t.Parallel()

	rustLang := tree_sitter.NewLanguage(tree_sitter_rust.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(rustLang))

	src := []byte(`use std::collections::HashMap;
use crate::models::User;
use serde::Deserialize;
mod utils;

fn main() {}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("rust", rustLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	imports := extractImports("rust", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["std::collections::HashMap"], "missing use std::collections::HashMap")
	require.True(t, paths["crate::models::User"], "missing use crate::models::User")
	require.True(t, paths["serde::Deserialize"], "missing use serde::Deserialize")
	require.True(t, paths["utils"], "missing mod utils")

	for _, imp := range imports {
		switch {
		case strings.HasPrefix(imp.Path, "std::"):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "crate::"):
			require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
		case strings.HasPrefix(imp.Path, "serde::"):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractImportsRuby(t *testing.T) {
	t.Parallel()

	rubyLang := tree_sitter.NewLanguage(tree_sitter_ruby.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(rubyLang))

	src := []byte(`require 'json'
require_relative './helper'
require 'rails'
include MyModule

class App
end
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	loader := NewQueryLoader()
	loader.RegisterLanguage("ruby", rubyLang)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })

	imports := extractImports("ruby", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["json"], "missing require json")
	require.True(t, paths["./helper"], "missing require_relative ./helper")
	require.True(t, paths["rails"], "missing require rails")
	require.True(t, paths["MyModule"], "missing include MyModule")

	for _, imp := range imports {
		switch imp.Path {
		case "json":
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case "./helper":
			require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
		case "rails":
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractCIncludes(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>

int main() {
    return 0;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["stdio.h"], "missing include stdio.h")
	require.True(t, paths["stdlib.h"], "missing include stdlib.h")
	require.True(t, paths["pthread.h"], "missing include pthread.h")

	for _, imp := range imports {
		switch imp.Path {
		case "stdio.h", "stdlib.h":
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case "pthread.h":
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		}
	}
}

func TestExtractCLocal(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`#include "myfile.h"
#include "utils/helper.h"

int main() {
    return 0;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["myfile.h"], "missing include myfile.h")
	require.True(t, paths["utils/helper.h"], "missing include utils/helper.h")

	for _, imp := range imports {
		require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
	}
}

func TestExtractCMixed(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`#include <stdio.h>
#include <openssl/ssl.h>
#include "myheader.h"
#include "utils/util.h"

int main() {
    return 0;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.Len(t, imports, 4, "expected 4 imports")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["stdio.h"], "missing include stdio.h")
	require.True(t, paths["openssl/ssl.h"], "missing include openssl/ssl.h")
	require.True(t, paths["myheader.h"], "missing include myheader.h")
	require.True(t, paths["utils/util.h"], "missing include utils/util.h")

	for _, imp := range imports {
		switch imp.Path {
		case "stdio.h":
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case "openssl/ssl.h":
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		case "myheader.h", "utils/util.h":
			require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
		}
	}
}

func TestExtractCEmpty(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`int main() {
    return 0;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports from source without includes")
}

func TestExtractCppIncludes(t *testing.T) {
	t.Parallel()

	cppLang := tree_sitter.NewLanguage(tree_sitter_cpp.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cppLang))

	src := []byte(`#include <iostream>
#include <vector>
#include <boost/asio.hpp>
#include "myclass.h"

int main() {
    return 0;
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("cpp", tree.RootNode(), src)
	require.Len(t, imports, 4, "expected 4 imports")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["iostream"], "missing include iostream")
	require.True(t, paths["vector"], "missing include vector")
	require.True(t, paths["boost/asio.hpp"], "missing include boost/asio.hpp")
	require.True(t, paths["myclass.h"], "missing include myclass.h")

	for _, imp := range imports {
		switch imp.Path {
		case "iostream", "vector":
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case "boost/asio.hpp":
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		case "myclass.h":
			require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
		}
	}
}
