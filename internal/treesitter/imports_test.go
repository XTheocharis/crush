//go:build treesitter

package treesitter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

func TestExtractCppUsesCExtractor(t *testing.T) {
	t.Parallel()

	// Verify that "cpp" language also routes to extractCImports.
	cppLang := tree_sitter.NewLanguage(tree_sitter_cpp.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cppLang))

	src := []byte(`#include <iostream>
#include <vector>
#include "myclass.h"

int main() { return 0; }
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("cpp", tree.RootNode(), src)
	require.Len(t, imports, 3)

	expected := map[string]string{
		"iostream":  ImportCategoryStdlib,
		"vector":    ImportCategoryStdlib,
		"myclass.h": ImportCategoryLocal,
	}

	for _, imp := range imports {
		cat, ok := expected[imp.Path]
		require.True(t, ok, "unexpected import: %s", imp.Path)
		require.Equal(t, cat, imp.Category,
			"expected %s to be %s, got %s", imp.Path, cat, imp.Category)
	}
}

func TestExtractCDuplicateIncludes(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`#include <stdio.h>
#include <stdio.h>
#include "foo.h"
#include "foo.h"

int main() { return 0; }
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.Len(t, imports, 2, "duplicate includes should be deduplicated")
}

func TestExtractCNilRoot(t *testing.T) {
	t.Parallel()

	imports := extractImports("c", nil, nil)
	require.Nil(t, imports)
}

func TestExtractCSharpImports(t *testing.T) {
	t.Parallel()

	csLang := tree_sitter.NewLanguage(tree_sitter_c_sharp.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(csLang))

	src := []byte(`using System;
using System.Collections.Generic;
using static System.Math;
using MyApp.Services;
using Newtonsoft.Json;

namespace MyApp {
    class Program {}
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("csharp", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["System"], "missing import System")
	require.True(t, paths["System.Collections.Generic"], "missing import System.Collections.Generic")
	require.True(t, paths["System.Math"], "missing import System.Math (using static)")
	require.True(t, paths["MyApp.Services"], "missing import MyApp.Services")
	require.True(t, paths["Newtonsoft.Json"], "missing import Newtonsoft.Json")

	for _, imp := range imports {
		switch {
		case strings.HasPrefix(imp.Path, "System"):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		case strings.HasPrefix(imp.Path, "MyApp"):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		case strings.HasPrefix(imp.Path, "Newtonsoft"):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractCSharpEmpty(t *testing.T) {
	t.Parallel()

	csLang := tree_sitter.NewLanguage(tree_sitter_c_sharp.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(csLang))

	src := []byte(`namespace MyApp {
    class Program {}
}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("csharp", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports for empty using list")
}

func TestExtractCSharpDedup(t *testing.T) {
	t.Parallel()

	csLang := tree_sitter.NewLanguage(tree_sitter_c_sharp.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(csLang))

	src := []byte(`using System;
using System;
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("csharp", tree.RootNode(), src)
	count := 0
	for _, imp := range imports {
		if imp.Path == "System" {
			count++
		}
	}
	require.Equal(t, 1, count, "expected deduplication of System import")
}

func TestExtractPHPImports(t *testing.T) {
	t.Parallel()

	phpLang := tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(phpLang))

	src := []byte(`<?php
use Some\Namespace\MyClass;
use function Some\namespace\myFunc;
require 'vendor/autoload.php';
require_once 'config.php';
include 'helpers.php';
include_once 'utils.php';
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("php", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["Some\\Namespace\\MyClass"], "missing use Some\\Namespace\\MyClass")
	require.True(t, paths["Some\\namespace\\myFunc"], "missing use function Some\\namespace\\myFunc")
	require.True(t, paths["vendor/autoload.php"], "missing require vendor/autoload.php")
	require.True(t, paths["config.php"], "missing require_once config.php")
	require.True(t, paths["helpers.php"], "missing include helpers.php")
	require.True(t, paths["utils.php"], "missing include_once utils.php")

	for _, imp := range imports {
		switch {
		case strings.Contains(imp.Path, "/") || strings.HasSuffix(imp.Path, ".php"):
			require.Equal(t, ImportCategoryLocal, imp.Category, "expected %s to be local", imp.Path)
		case strings.Contains(imp.Path, "\\"):
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractPHPEmpty(t *testing.T) {
	t.Parallel()

	phpLang := tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(phpLang))

	src := []byte(`<?php
echo "hello";
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("php", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports for empty use/require list")
}

func TestExtractPHPDedup(t *testing.T) {
	t.Parallel()

	phpLang := tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(phpLang))

	src := []byte(`<?php
use Some\Namespace\MyClass;
use Some\Namespace\MyClass;
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("php", tree.RootNode(), src)
	count := 0
	for _, imp := range imports {
		if imp.Path == "Some\\Namespace\\MyClass" {
			count++
		}
	}
	require.Equal(t, 1, count, "expected deduplication of PHP use import")
}

func TestExtractPHPNamespaceClassification(t *testing.T) {
	t.Parallel()

	phpLang := tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(phpLang))

	src := []byte(`<?php
use Monolog\Logger;
use GuzzleHttp\Client;
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("php", tree.RootNode(), src)
	require.NotEmpty(t, imports)

	for _, imp := range imports {
		require.Equal(t, ImportCategoryThirdParty, imp.Category,
			"expected namespace import %s to be third_party", imp.Path)
	}
}

func TestExtractCCSystemThirdParty(t *testing.T) {
	t.Parallel()

	cLang := tree_sitter.NewLanguage(tree_sitter_c.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(cLang))

	src := []byte(`#include <gtk/gtk.h>
#include <libavcodec/avcodec.h>

int main() { return 0; }
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("c", tree.RootNode(), src)
	require.Len(t, imports, 2)

	for _, imp := range imports {
		require.Equal(t, ImportCategoryThirdParty, imp.Category,
			"expected %s to be third_party", imp.Path)
	}
}

func TestExtractKotlinImports(t *testing.T) {
	t.Parallel()

	kotlinLang := tree_sitter.NewLanguage(tree_sitter_kotlin.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(kotlinLang))

	src := []byte(`package com.example

import kotlin.collections.List
import java.util.ArrayList
import com.example.MyClass
import org.foo.bar.Baz

fun main() {}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("kotlin", tree.RootNode(), src)
	require.NotEmpty(t, imports, "expected import extraction to produce results")

	paths := make(map[string]bool)
	for _, imp := range imports {
		paths[imp.Path] = true
	}
	require.True(t, paths["kotlin.collections.List"], "missing import kotlin.collections.List")
	require.True(t, paths["java.util.ArrayList"], "missing import java.util.ArrayList")
	require.True(t, paths["com.example.MyClass"], "missing import com.example.MyClass")
	require.True(t, paths["org.foo.bar.Baz"], "missing import org.foo.bar.Baz")

	for _, imp := range imports {
		switch {
		case strings.HasPrefix(imp.Path, "kotlin.") || strings.HasPrefix(imp.Path, "java."):
			require.Equal(t, ImportCategoryStdlib, imp.Category, "expected %s to be stdlib", imp.Path)
		default:
			require.Equal(t, ImportCategoryThirdParty, imp.Category, "expected %s to be third_party", imp.Path)
		}
	}
}

func TestExtractKotlinEmpty(t *testing.T) {
	t.Parallel()

	kotlinLang := tree_sitter.NewLanguage(tree_sitter_kotlin.Language())
	p := tree_sitter.NewParser()
	t.Cleanup(p.Close)
	require.NoError(t, p.SetLanguage(kotlinLang))

	src := []byte(`package com.example

fun main() {}
`)
	tree := p.Parse(src, nil)
	t.Cleanup(tree.Close)

	imports := extractImports("kotlin", tree.RootNode(), src)
	require.Empty(t, imports, "expected no imports for empty import list")
}
