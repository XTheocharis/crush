package explorer

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// GoExplorer explores Go source files using go/parser.
type GoExplorer struct{}

func (e *GoExplorer) CanHandle(path string, content []byte) bool {
	return strings.ToLower(filepath.Ext(path)) == ".go"
}

func (e *GoExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Go file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "go", TokenEstimate: estimateTokens(summary)}, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, input.Path, input.Content, parser.ParseComments)
	if err != nil {
		// Fallback to simple text display on parse error
		content, _ := sampleContent(input.Content, 12000)
		summary := fmt.Sprintf("Go file (parse error): %s\n%s", filepath.Base(input.Path), content)
		return ExploreResult{Summary: summary, ExplorerUsed: "go", TokenEstimate: estimateTokens(summary)}, nil
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Go file: %s\n", filepath.Base(input.Path))
	fmt.Fprintf(&summary, "Package: %s\n", file.Name.Name)

	// Build constraints
	if file.Doc != nil {
		for _, comment := range file.Doc.List {
			text := comment.Text
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
				fmt.Fprintf(&summary, "Build constraint: %s\n", strings.TrimPrefix(text, "//"))
			}
		}
	}

	// Imports
	if len(file.Imports) > 0 {
		var stdlib, external, internal []string
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			// Categorize imports
			if !strings.Contains(path, ".") {
				stdlib = append(stdlib, path)
			} else if strings.HasPrefix(path, "github.com/charmbracelet/crush") {
				internal = append(internal, path)
			} else {
				external = append(external, path)
			}
		}

		summary.WriteString("\nImports:\n")
		if len(stdlib) > 0 {
			summary.WriteString("  Standard library:\n")
			for _, imp := range stdlib {
				fmt.Fprintf(&summary, "    - %s\n", imp)
			}
		}
		if len(external) > 0 {
			summary.WriteString("  External:\n")
			for _, imp := range external {
				fmt.Fprintf(&summary, "    - %s\n", imp)
			}
		}
		if len(internal) > 0 {
			summary.WriteString("  Internal:\n")
			for _, imp := range internal {
				fmt.Fprintf(&summary, "    - %s\n", imp)
			}
		}
	}

	// Types
	var types []string
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeName := typeSpec.Name.Name
			switch typeSpec.Type.(type) {
			case *ast.StructType:
				types = append(types, fmt.Sprintf("struct %s", typeName))
			case *ast.InterfaceType:
				types = append(types, fmt.Sprintf("interface %s", typeName))
			default:
				types = append(types, fmt.Sprintf("type %s", typeName))
			}
		}
	}
	if len(types) > 0 {
		summary.WriteString("\nTypes:\n")
		for _, t := range types {
			fmt.Fprintf(&summary, "  - %s\n", t)
		}
	}

	// Functions
	var functions []string
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		funcName := funcDecl.Name.Name
		if funcDecl.Recv != nil {
			// Method
			recvType := exprToString(funcDecl.Recv.List[0].Type)
			functions = append(functions, fmt.Sprintf("(%s) %s", recvType, funcName))
		} else {
			// Function
			functions = append(functions, funcName)
		}
	}
	if len(functions) > 0 {
		summary.WriteString("\nFunctions/Methods:\n")
		for _, f := range functions {
			fmt.Fprintf(&summary, "  - %s\n", f)
		}
	}

	// Constants and Variables
	var consts, vars []string
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				switch genDecl.Tok {
				case token.CONST:
					consts = append(consts, name.Name)
				case token.VAR:
					vars = append(vars, name.Name)
				}
			}
		}
	}
	if len(consts) > 0 {
		summary.WriteString("\nConstants:\n")
		for _, c := range consts {
			fmt.Fprintf(&summary, "  - %s\n", c)
		}
	}
	if len(vars) > 0 {
		summary.WriteString("\nVariables:\n")
		for _, v := range vars {
			fmt.Fprintf(&summary, "  - %s\n", v)
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "go",
		TokenEstimate: estimateTokens(result),
	}, nil
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return "unknown"
	}
}

// PythonExplorer explores Python files.
type PythonExplorer struct{}

func (e *PythonExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "py" || ext == "pyw" || ext == "pyx" {
		return true
	}
	return detectShebang(content) == "python"
}

func (e *PythonExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Python file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "python", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "Python file: %s\n", filepath.Base(input.Path))

	// Imports
	importRe := regexp.MustCompile(`(?m)^(?:from\s+(\S+)\s+)?import\s+(.+)$`)
	imports := importRe.FindAllStringSubmatch(content, -1)
	if len(imports) > 0 {
		summary.WriteString("\nImports:\n")
		seen := make(map[string]bool)
		for _, match := range imports {
			var imp string
			if match[1] != "" {
				imp = fmt.Sprintf("from %s import %s", match[1], match[2])
			} else {
				imp = fmt.Sprintf("import %s", match[2])
			}
			if !seen[imp] {
				fmt.Fprintf(&summary, "  - %s\n", imp)
				seen[imp] = true
			}
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^class\s+(\w+)(?:\(([^)]*)\))?:`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			if match[2] != "" {
				fmt.Fprintf(&summary, "  - %s(%s)\n", match[1], match[2])
			} else {
				fmt.Fprintf(&summary, "  - %s\n", match[1])
			}
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^def\s+(\w+)\s*\(([^)]*)\)`)
	functions := funcRe.FindAllStringSubmatch(content, -1)
	if len(functions) > 0 {
		summary.WriteString("\nFunctions:\n")
		for _, match := range functions {
			fmt.Fprintf(&summary, "  - %s(%s)\n", match[1], match[2])
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "python",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// JavaScriptExplorer explores JavaScript files.
type JavaScriptExplorer struct{}

func (e *JavaScriptExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "js" || ext == "mjs" || ext == "cjs" || ext == "jsx" {
		return true
	}
	return detectShebang(content) == "javascript"
}

func (e *JavaScriptExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("JavaScript file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "javascript", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "JavaScript file: %s\n", filepath.Base(input.Path))

	// Imports
	importRe := regexp.MustCompile(`(?m)^import\s+(?:(.+?)\s+from\s+)?['"]([^'"]+)['"]`)
	requireRe := regexp.MustCompile(`(?m)(?:const|let|var)\s+(?:\{[^}]+\}|\w+)\s*=\s*require\(['"]([^'"]+)['"]\)`)

	imports := importRe.FindAllStringSubmatch(content, -1)
	requires := requireRe.FindAllStringSubmatch(content, -1)

	if len(imports) > 0 || len(requires) > 0 {
		summary.WriteString("\nImports:\n")
		seen := make(map[string]bool)
		for _, match := range imports {
			imp := match[2]
			if !seen[imp] {
				fmt.Fprintf(&summary, "  - %s\n", imp)
				seen[imp] = true
			}
		}
		for _, match := range requires {
			imp := match[1]
			if !seen[imp] {
				fmt.Fprintf(&summary, "  - %s (require)\n", imp)
				seen[imp] = true
			}
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^(?:export\s+)?class\s+(\w+)`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`)
	arrowRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)

	functions := funcRe.FindAllStringSubmatch(content, -1)
	arrows := arrowRe.FindAllStringSubmatch(content, -1)

	if len(functions) > 0 || len(arrows) > 0 {
		summary.WriteString("\nFunctions:\n")
		for _, match := range functions {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
		for _, match := range arrows {
			fmt.Fprintf(&summary, "  - %s (arrow)\n", match[1])
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "javascript",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// TypeScriptExplorer explores TypeScript files.
type TypeScriptExplorer struct{}

func (e *TypeScriptExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "ts" || ext == "tsx" || ext == "mts" || ext == "cts" {
		return true
	}
	return detectShebang(content) == "typescript"
}

func (e *TypeScriptExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("TypeScript file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "typescript", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "TypeScript file: %s\n", filepath.Base(input.Path))

	// Imports
	importRe := regexp.MustCompile(`(?m)^import\s+(?:(.+?)\s+from\s+)?['"]([^'"]+)['"]`)
	imports := importRe.FindAllStringSubmatch(content, -1)
	if len(imports) > 0 {
		summary.WriteString("\nImports:\n")
		seen := make(map[string]bool)
		for _, match := range imports {
			imp := match[2]
			if !seen[imp] {
				fmt.Fprintf(&summary, "  - %s\n", imp)
				seen[imp] = true
			}
		}
	}

	// Interfaces
	interfaceRe := regexp.MustCompile(`(?m)^(?:export\s+)?interface\s+(\w+)`)
	interfaces := interfaceRe.FindAllStringSubmatch(content, -1)
	if len(interfaces) > 0 {
		summary.WriteString("\nInterfaces:\n")
		for _, match := range interfaces {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Types
	typeRe := regexp.MustCompile(`(?m)^(?:export\s+)?type\s+(\w+)`)
	types := typeRe.FindAllStringSubmatch(content, -1)
	if len(types) > 0 {
		summary.WriteString("\nTypes:\n")
		for _, match := range types {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^(?:export\s+)?class\s+(\w+)`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions - also match typed arrow functions like: const Foo: React.FC<Props> = () => {}
	funcRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+(\w+)\s*\(`)
	arrowRe := regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?\([^)]*\)\s*=>`)

	functions := funcRe.FindAllStringSubmatch(content, -1)
	arrows := arrowRe.FindAllStringSubmatch(content, -1)

	if len(functions) > 0 || len(arrows) > 0 {
		summary.WriteString("\nFunctions:\n")
		for _, match := range functions {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
		for _, match := range arrows {
			fmt.Fprintf(&summary, "  - %s (arrow)\n", match[1])
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "typescript",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// RustExplorer explores Rust files.
type RustExplorer struct{}

func (e *RustExplorer) CanHandle(path string, content []byte) bool {
	return strings.ToLower(filepath.Ext(path)) == ".rs"
}

func (e *RustExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Rust file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "rust", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "Rust file: %s\n", filepath.Base(input.Path))

	// Use statements
	useRe := regexp.MustCompile(`(?m)^use\s+([^;]+);`)
	uses := useRe.FindAllStringSubmatch(content, -1)
	if len(uses) > 0 {
		summary.WriteString("\nUse statements:\n")
		for _, match := range uses {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Structs
	structRe := regexp.MustCompile(`(?m)^(?:pub\s+)?struct\s+(\w+)`)
	structs := structRe.FindAllStringSubmatch(content, -1)
	if len(structs) > 0 {
		summary.WriteString("\nStructs:\n")
		for _, match := range structs {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Enums
	enumRe := regexp.MustCompile(`(?m)^(?:pub\s+)?enum\s+(\w+)`)
	enums := enumRe.FindAllStringSubmatch(content, -1)
	if len(enums) > 0 {
		summary.WriteString("\nEnums:\n")
		for _, match := range enums {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Traits
	traitRe := regexp.MustCompile(`(?m)^(?:pub\s+)?trait\s+(\w+)`)
	traits := traitRe.FindAllStringSubmatch(content, -1)
	if len(traits) > 0 {
		summary.WriteString("\nTraits:\n")
		for _, match := range traits {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:pub\s+)?(?:async\s+)?fn\s+(\w+)\s*\(`)
	functions := funcRe.FindAllStringSubmatch(content, -1)
	if len(functions) > 0 {
		summary.WriteString("\nFunctions:\n")
		for _, match := range functions {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "rust",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// JavaExplorer explores Java files.
type JavaExplorer struct{}

func (e *JavaExplorer) CanHandle(path string, content []byte) bool {
	return strings.ToLower(filepath.Ext(path)) == ".java"
}

func (e *JavaExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Java file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "java", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "Java file: %s\n", filepath.Base(input.Path))

	// Package
	pkgRe := regexp.MustCompile(`(?m)^package\s+([^;]+);`)
	if match := pkgRe.FindStringSubmatch(content); match != nil {
		fmt.Fprintf(&summary, "Package: %s\n", match[1])
	}

	// Imports
	importRe := regexp.MustCompile(`(?m)^import\s+(?:static\s+)?([^;]+);`)
	imports := importRe.FindAllStringSubmatch(content, -1)
	if len(imports) > 0 {
		summary.WriteString("\nImports:\n")
		for _, match := range imports {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^(?:public\s+|private\s+|protected\s+)?(?:abstract\s+|final\s+)?class\s+(\w+)`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Interfaces
	interfaceRe := regexp.MustCompile(`(?m)^(?:public\s+)?interface\s+(\w+)`)
	interfaces := interfaceRe.FindAllStringSubmatch(content, -1)
	if len(interfaces) > 0 {
		summary.WriteString("\nInterfaces:\n")
		for _, match := range interfaces {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Methods
	methodRe := regexp.MustCompile(`(?m)^\s+(?:public\s+|private\s+|protected\s+)?(?:static\s+)?(?:\w+\s+)?(\w+)\s*\([^)]*\)\s*(?:throws\s+[^{]+)?\{`)
	methods := methodRe.FindAllStringSubmatch(content, -1)
	if len(methods) > 0 {
		summary.WriteString("\nMethods:\n")
		seen := make(map[string]bool)
		for _, match := range methods {
			method := match[1]
			if !seen[method] && method != "if" && method != "for" && method != "while" {
				fmt.Fprintf(&summary, "  - %s\n", method)
				seen[method] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "java",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// CppExplorer explores C++ files.
type CppExplorer struct{}

func (e *CppExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "cpp" || ext == "cxx" || ext == "cc" || ext == "hpp" || ext == "hxx" || ext == "hh"
}

func (e *CppExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("C++ file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "cpp", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "C++ file: %s\n", filepath.Base(input.Path))

	// Includes
	includeRe := regexp.MustCompile(`(?m)^#include\s+[<"]([^>"]+)[>"]`)
	includes := includeRe.FindAllStringSubmatch(content, -1)
	if len(includes) > 0 {
		summary.WriteString("\nIncludes:\n")
		for _, match := range includes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Namespaces
	namespaceRe := regexp.MustCompile(`(?m)^namespace\s+(\w+)`)
	namespaces := namespaceRe.FindAllStringSubmatch(content, -1)
	if len(namespaces) > 0 {
		summary.WriteString("\nNamespaces:\n")
		for _, match := range namespaces {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^(?:template\s*<[^>]+>\s*)?class\s+(\w+)`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Structs
	structRe := regexp.MustCompile(`(?m)^(?:template\s*<[^>]+>\s*)?struct\s+(\w+)`)
	structs := structRe.FindAllStringSubmatch(content, -1)
	if len(structs) > 0 {
		summary.WriteString("\nStructs:\n")
		for _, match := range structs {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:inline\s+|static\s+|virtual\s+|extern\s+)*(?:\w+(?:::\w+)*\s+)?(\w+)\s*\([^)]*\)\s*(?:const\s*)?(?:override\s*)?[{;]`)
	functions := funcRe.FindAllStringSubmatch(content, -1)
	if len(functions) > 0 {
		summary.WriteString("\nFunctions:\n")
		seen := make(map[string]bool)
		for _, match := range functions {
			fn := match[1]
			if !seen[fn] && fn != "if" && fn != "for" && fn != "while" && fn != "switch" {
				fmt.Fprintf(&summary, "  - %s\n", fn)
				seen[fn] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "cpp",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// CExplorer explores C files.
type CExplorer struct{}

func (e *CExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	return ext == "c" || ext == "h"
}

func (e *CExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("C file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "c", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "C file: %s\n", filepath.Base(input.Path))

	// Includes
	includeRe := regexp.MustCompile(`(?m)^#include\s+[<"]([^>"]+)[>"]`)
	includes := includeRe.FindAllStringSubmatch(content, -1)
	if len(includes) > 0 {
		summary.WriteString("\nIncludes:\n")
		for _, match := range includes {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Structs
	structRe := regexp.MustCompile(`(?m)^(?:typedef\s+)?struct\s+(\w+)`)
	structs := structRe.FindAllStringSubmatch(content, -1)
	if len(structs) > 0 {
		summary.WriteString("\nStructs:\n")
		for _, match := range structs {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Typedefs
	typedefRe := regexp.MustCompile(`(?m)^typedef\s+(?:struct\s+)?(?:\w+\s+)?(\w+)\s*;`)
	typedefs := typedefRe.FindAllStringSubmatch(content, -1)
	if len(typedefs) > 0 {
		summary.WriteString("\nTypedefs:\n")
		for _, match := range typedefs {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Functions
	funcRe := regexp.MustCompile(`(?m)^(?:static\s+|extern\s+|inline\s+)*(?:\w+\s+\*?)(\w+)\s*\([^)]*\)\s*[{;]`)
	functions := funcRe.FindAllStringSubmatch(content, -1)
	if len(functions) > 0 {
		summary.WriteString("\nFunctions:\n")
		seen := make(map[string]bool)
		for _, match := range functions {
			fn := match[1]
			if !seen[fn] && fn != "if" && fn != "for" && fn != "while" && fn != "switch" {
				fmt.Fprintf(&summary, "  - %s\n", fn)
				seen[fn] = true
			}
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "c",
		TokenEstimate: estimateTokens(result),
	}, nil
}

// RubyExplorer explores Ruby files.
type RubyExplorer struct{}

func (e *RubyExplorer) CanHandle(path string, content []byte) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "rb" || ext == "rake" {
		return true
	}
	return detectShebang(content) == "ruby"
}

func (e *RubyExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("Ruby file too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "ruby", TokenEstimate: estimateTokens(summary)}, nil
	}

	content := string(input.Content)
	var summary strings.Builder
	fmt.Fprintf(&summary, "Ruby file: %s\n", filepath.Base(input.Path))

	// Requires
	requireRe := regexp.MustCompile(`(?m)^require\s+['"]([^'"]+)['"]`)
	requires := requireRe.FindAllStringSubmatch(content, -1)
	if len(requires) > 0 {
		summary.WriteString("\nRequires:\n")
		for _, match := range requires {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Classes
	classRe := regexp.MustCompile(`(?m)^class\s+(\w+)(?:\s+<\s+(\S+))?`)
	classes := classRe.FindAllStringSubmatch(content, -1)
	if len(classes) > 0 {
		summary.WriteString("\nClasses:\n")
		for _, match := range classes {
			if match[2] != "" {
				fmt.Fprintf(&summary, "  - %s < %s\n", match[1], match[2])
			} else {
				fmt.Fprintf(&summary, "  - %s\n", match[1])
			}
		}
	}

	// Modules
	moduleRe := regexp.MustCompile(`(?m)^module\s+(\w+)`)
	modules := moduleRe.FindAllStringSubmatch(content, -1)
	if len(modules) > 0 {
		summary.WriteString("\nModules:\n")
		for _, match := range modules {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	// Methods
	methodRe := regexp.MustCompile(`(?m)^(?:\s+)?def\s+(\w+)`)
	methods := methodRe.FindAllStringSubmatch(content, -1)
	if len(methods) > 0 {
		summary.WriteString("\nMethods:\n")
		for _, match := range methods {
			fmt.Fprintf(&summary, "  - %s\n", match[1])
		}
	}

	result := summary.String()
	return ExploreResult{
		Summary:       result,
		ExplorerUsed:  "ruby",
		TokenEstimate: estimateTokens(result),
	}, nil
}
