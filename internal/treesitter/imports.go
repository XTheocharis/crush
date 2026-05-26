//go:build treesitter

package treesitter

import (
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractImports(lang string, root *tree_sitter.Node, content []byte) []ImportInfo {
	if root == nil {
		return nil
	}

	switch GetQueryKey(lang) {
	case "go":
		return extractGoImports(root, content)
	case "python":
		return extractPythonImports(root, content)
	case "typescript", "javascript":
		return extractTypeScriptImports(root, content)
	case "java":
		return extractJavaImports(root, content)
	case "rust":
		return extractRustImports(root, content)
	case "ruby":
		return extractRubyImports(root, content)
	case "csharp":
		return extractCSharpImports(root, content)
	case "c", "cpp":
		return extractCImports(root, content)
	case "php":
		return extractPHPImports(root, content)
	case "kotlin":
		return extractKotlinImports(root, content)
	case "swift":
		return extractSwiftImports(root, content)
	case "scala":
		return extractScalaImports(root, content)
	default:
		return nil
	}
}

func extractGoImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	cursor := root.Walk()
	defer cursor.Close()

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() != "import_declaration" {
			return true
		}

		childCount := int(n.ChildCount())
		for i := range childCount {
			child := n.Child(uint(i))
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "import_spec":
				path := extractGoImportSpecPath(child, content)
				if path != "" {
					imports = append(imports, ImportInfo{
						Path:     path,
						Category: classifyImport("go", path),
					})
				}
			case "import_spec_list":
				// Grouped imports: import_spec_list contains
				// import_spec children.
				subCount := int(child.ChildCount())
				for j := range subCount {
					sub := child.Child(uint(j))
					if sub == nil || sub.Kind() != "import_spec" {
						continue
					}
					path := extractGoImportSpecPath(sub, content)
					if path != "" {
						imports = append(imports, ImportInfo{
							Path:     path,
							Category: classifyImport("go", path),
						})
					}
				}
			}
		}
		return false
	})

	return imports
}

func extractGoImportSpecPath(spec *tree_sitter.Node, content []byte) string {
	childCount := int(spec.ChildCount())
	for i := range childCount {
		child := spec.Child(uint(i))
		if child == nil {
			continue
		}
		if child.Kind() == "interpreted_string_literal" {
			text := strings.TrimSpace(child.Utf8Text(content))
			return strings.Trim(text, `"`)
		}
	}
	return ""
}

func extractPythonImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("python", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "import_statement":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				switch child.Kind() {
				case "dotted_name":
					text := strings.TrimSpace(child.Utf8Text(content))
					first := strings.Split(text, ".")[0]
					addImport(first)
				case "aliased_import":
					nameChild := child.Child(0)
					if nameChild != nil {
						text := strings.TrimSpace(nameChild.Utf8Text(content))
						first := strings.Split(text, ".")[0]
						addImport(first)
					}
				}
			}
			return false

		case "import_from_statement":
			moduleName := n.ChildByFieldName("module_name")
			if moduleName != nil {
				text := strings.TrimSpace(moduleName.Utf8Text(content))
				addImport(text)
			}
			return false

		default:
			return true
		}
	})

	return imports
}

func extractTypeScriptImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		path = strings.Trim(path, `"'`)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("typescript", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "import_statement":
			source := n.ChildByFieldName("source")
			if source != nil {
				addImport(source.Utf8Text(content))
			}
			return false

		case "call_expression":
			funcNode := n.ChildByFieldName("function")
			if funcNode != nil {
				if funcNode.Utf8Text(content) == "require" {
					args := n.ChildByFieldName("arguments")
					if args != nil {
						childCount := int(args.ChildCount())
						for i := range childCount {
							arg := args.Child(uint(i))
							if arg != nil && (arg.Kind() == "string" || arg.Kind() == "string_fragment") {
								text := strings.Trim(arg.Utf8Text(content), `"'`)
								addImport(text)
								break
							}
						}
					}
				}
			}
			return true

		default:
			return true
		}
	})

	return imports
}

func extractJavaImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		path = strings.TrimSuffix(path, ";")
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("java", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "package_declaration":
			return false

		case "import_declaration":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				switch child.Kind() {
				case "scoped_identifier", "scoped_type_identifier", "asterisk":
					text := strings.TrimSpace(child.Utf8Text(content))
					text = strings.TrimSuffix(text, ";")
					addImport(text)
				}
			}
			return false

		default:
			return true
		}
	})

	return imports
}

func extractRustImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("rust", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "use_declaration":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				switch child.Kind() {
				case "scoped_identifier", "scoped_use_list", "use_list",
					"use_wildcard", "scoped_type_identifier":
					text := strings.TrimSpace(child.Utf8Text(content))
					addImport(text)
				}
			}
			return false

		case "mod_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				text := strings.TrimSpace(nameNode.Utf8Text(content))
				addImport(text)
			}
			return false

		default:
			return true
		}
	})

	return imports
}

func extractRubyImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		path = strings.Trim(path, `"'`)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("ruby", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "call":
			methodNode := n.ChildByFieldName("method")
			if methodNode != nil {
				method := methodNode.Utf8Text(content)
				switch method {
				case "require", "require_relative":
					args := n.ChildByFieldName("arguments")
					if args != nil {
						childCount := int(args.ChildCount())
						for i := range childCount {
							arg := args.Child(uint(i))
							if arg != nil && (arg.Kind() == "string" || arg.Kind() == "string_content") {
								text := arg.Utf8Text(content)
								text = strings.Trim(text, `"'`)
								addImport(text)
								break
							}
						}
					}
				case "include", "extend":
					args := n.ChildByFieldName("arguments")
					if args != nil {
						childCount := int(args.ChildCount())
						for i := range childCount {
							arg := args.Child(uint(i))
							if arg != nil {
								switch arg.Kind() {
								case "constant", "scope_resolution",
									"module":
									text := strings.TrimSpace(arg.Utf8Text(content))
									addImport(text)
								}
							}
						}
					}
				}
			}
			return true

		default:
			return true
		}
	})

	return imports
}

func extractCImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string, category string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: category,
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() != "preproc_include" {
			return true
		}

		childCount := int(n.ChildCount())
		for i := range childCount {
			child := n.Child(uint(i))
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "string_literal":
				// Local include: "file.h" — extract from
				// string_content child to avoid quote
				// stripping ambiguity.
				text := cIncludeStringContent(child, content)
				addImport(text, ImportCategoryLocal)
			case "system_lib_string":
				// System include: <file.h>
				text := strings.TrimSpace(child.Utf8Text(content))
				text = strings.Trim(text, "<>")
				category := classifyCSystemImport(text)
				addImport(text, category)
			}
		}
		return false
	})

	return imports
}

// cIncludeStringContent extracts the path from a string_literal node
// produced by a #include "..." directive. It reads the string_content
// child directly to avoid edge cases with quote stripping.
func cIncludeStringContent(node *tree_sitter.Node, content []byte) string {
	childCount := int(node.ChildCount())
	for i := range childCount {
		child := node.Child(uint(i))
		if child != nil && child.Kind() == "string_content" {
			return strings.TrimSpace(child.Utf8Text(content))
		}
	}
	// Fallback: strip quotes from the full text.
	text := strings.TrimSpace(node.Utf8Text(content))
	return strings.Trim(text, `"`)
}

// classifyCSystemImport classifies a system (angle-bracket) include as
// stdlib or third_party.
func classifyCSystemImport(path string) string {
	cStdlib := map[string]struct{}{
		// C standard library headers.
		"assert.h": {}, "complex.h": {}, "ctype.h": {}, "errno.h": {},
		"fenv.h": {}, "float.h": {}, "inttypes.h": {}, "iso646.h": {},
		"limits.h": {}, "locale.h": {}, "math.h": {}, "setjmp.h": {},
		"signal.h": {}, "stdalign.h": {}, "stdarg.h": {}, "stdatomic.h": {},
		"stdbool.h": {}, "stddef.h": {}, "stdint.h": {}, "stdio.h": {},
		"stdlib.h": {}, "stdnoreturn.h": {}, "string.h": {}, "tgmath.h": {},
		"threads.h": {}, "time.h": {}, "uchar.h": {}, "wchar.h": {},
		"wctype.h": {},
		// C++ standard library headers (no .h suffix).
		"algorithm": {}, "array": {}, "atomic": {}, "bitset": {},
		"chrono": {}, "codecvt": {}, "complex": {}, "condition_variable": {},
		"deque": {}, "exception": {}, "filesystem": {}, "forward_list": {},
		"fstream": {}, "functional": {}, "future": {}, "initializer_list": {},
		"iomanip": {}, "ios": {}, "iosfwd": {}, "iostream": {},
		"istream": {}, "iterator": {}, "limits": {}, "list": {},
		"locale": {}, "map": {}, "memory": {}, "mutex": {},
		"new": {}, "numeric": {}, "optional": {}, "ostream": {},
		"queue": {}, "random": {}, "ratio": {}, "regex": {},
		"scoped_allocator": {}, "set": {}, "shared_mutex": {},
		"span": {}, "sstream": {}, "stack": {}, "stdexcept": {},
		"streambuf": {}, "string": {}, "string_view": {},
		"strstream": {}, "system_error": {}, "thread": {},
		"tuple": {}, "type_traits": {}, "typeindex": {},
		"typeinfo": {}, "unordered_map": {}, "unordered_set": {},
		"utility": {}, "valarray": {}, "variant": {}, "vector": {},
		"version": {}, "concepts": {}, "coroutine": {},
		"compare": {}, "format": {}, "ranges": {},
		"syncstream": {}, "barrier": {}, "latch": {},
		"semaphore": {}, "source_location": {}, "stop_token": {},
		// C++ wrappers for C headers.
		"cassert": {}, "ccomplex": {}, "cctype": {}, "cerrno": {},
		"cfenv": {}, "cfloat": {}, "cinttypes": {}, "ciso646": {},
		"climits": {}, "clocale": {}, "cmath": {}, "csetjmp": {},
		"csignal": {}, "cstdalign": {}, "cstdarg": {}, "cstdbool": {},
		"cstddef": {}, "cstdint": {}, "cstdio": {}, "cstdlib": {},
		"cstring": {}, "ctgmath": {}, "ctime": {}, "cuchar": {},
		"cwchar": {}, "cwctype": {},
		// POSIX common headers.
		"pthread.h": {}, "unistd.h": {}, "fcntl.h": {},
		"sys/types.h": {}, "sys/stat.h": {}, "sys/socket.h": {},
		"arpa/inet.h": {}, "netinet/in.h": {}, "dlfcn.h": {},
		"poll.h": {}, "sched.h": {}, "semaphore.h": {},
		"mqueue.h": {},
	}

	if _, ok := cStdlib[path]; ok {
		return ImportCategoryStdlib
	}
	return ImportCategoryThirdParty
}

func extractKotlinImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("kotlin", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() == "import" {
			// import children: "import" keyword, then
			// qualified_identifier with identifier / "." nodes.
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				if child.Kind() == "qualified_identifier" {
					text := strings.TrimSpace(child.Utf8Text(content))
					addImport(text)
					break
				}
			}
			return false
		}
		return true
	})

	return imports
}

func extractSwiftImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("swift", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() == "import_declaration" {
			// import_declaration children: "import" keyword, optional
			// import_kind (struct/class/func/etc.), then identifier(s).
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				// The module name appears as an "identifier" or
				// dotted "accessible_modifier" / "name" child.
				if child.Kind() == "identifier" {
					text := strings.TrimSpace(child.Utf8Text(content))
					addImport(text)
				}
			}
			return false
		}
		return true
	})

	return imports
}

func extractScalaImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("scala", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() == "import_declaration" {
			// import_declaration children: "import" keyword, then
			// alternating identifier / "." nodes forming the dotted path.
			childCount := int(n.ChildCount())
			var parts []string
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				kind := child.Kind()
				if kind == "identifier" {
					parts = append(parts, strings.TrimSpace(child.Utf8Text(content)))
				}
			}
			if len(parts) > 0 {
				addImport(strings.Join(parts, "."))
			}
			return false
		}
		return true
	})

	return imports
}

func extractCSharpImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("csharp", path),
		})
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		if n.Kind() != "using_directive" {
			return true
		}

		childCount := int(n.ChildCount())
		for i := range childCount {
			child := n.Child(uint(i))
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "identifier", "qualified_name":
				text := strings.TrimSpace(child.Utf8Text(content))
				addImport(text)
			}
		}
		return false
	})

	return imports
}

func extractPHPImports(root *tree_sitter.Node, content []byte) []ImportInfo {
	var imports []ImportInfo
	seen := map[string]struct{}{}

	cursor := root.Walk()
	defer cursor.Close()

	addImport := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		imports = append(imports, ImportInfo{
			Path:     path,
			Category: classifyImport("php", path),
		})
	}

	var extractNamespacePath func(n *tree_sitter.Node)
	extractNamespacePath = func(n *tree_sitter.Node) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case "qualified_name":
			text := strings.TrimSpace(n.Utf8Text(content))
			addImport(text)
		case "namespace_use_clause":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child != nil {
					extractNamespacePath(child)
				}
			}
		}
	}

	visitNodes(cursor, func(n *tree_sitter.Node) bool {
		switch n.Kind() {
		case "namespace_use_declaration":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child != nil {
					extractNamespacePath(child)
				}
			}
			return false

		case "include_expression", "require_expression",
			"include_once_expression", "require_once_expression":
			childCount := int(n.ChildCount())
			for i := range childCount {
				child := n.Child(uint(i))
				if child == nil {
					continue
				}
				switch child.Kind() {
				case "string":
					subCount := int(child.ChildCount())
					for j := range subCount {
						sub := child.Child(uint(j))
						if sub != nil && sub.Kind() == "string_content" {
							text := strings.TrimSpace(sub.Utf8Text(content))
							addImport(text)
						}
					}
				case "string_content":
					text := strings.TrimSpace(child.Utf8Text(content))
					addImport(text)
				case "encapsed_string":
					text := strings.TrimSpace(child.Utf8Text(content))
					text = strings.Trim(text, `"'`)
					addImport(text)
				}
			}
			return false

		default:
			return true
		}
	})

	return imports
}

func classifyImport(lang, path string) string {
	switch lang {
	case "go":
		if strings.Contains(path, ".") {
			return ImportCategoryThirdParty
		}
		return ImportCategoryStdlib
	case "python":
		first := strings.Split(path, ".")[0]
		pythonStdlib := map[string]struct{}{
			"os": {}, "sys": {}, "re": {}, "json": {}, "collections": {},
			"typing": {}, "io": {}, "math": {}, "datetime": {}, "pathlib": {},
			"functools": {}, "itertools": {}, "logging": {}, "unittest": {},
			"abc": {}, "argparse": {}, "asyncio": {}, "base64": {},
			"copy": {}, "csv": {}, "dataclasses": {}, "enum": {},
			"glob": {}, "hashlib": {}, "http": {}, "importlib": {},
			"inspect": {}, "multiprocessing": {}, "pickle": {}, "random": {},
			"shutil": {}, "socket": {}, "sqlite3": {}, "string": {},
			"struct": {}, "subprocess": {}, "tempfile": {}, "threading": {},
			"time": {}, "traceback": {}, "urllib": {}, "uuid": {},
			"xml": {}, "yaml": {}, "decimal": {}, "heapq": {},
			"bisect": {}, "array": {}, "queue": {}, "weakref": {},
			"contextlib": {}, "textwrap": {}, "operator": {}, "types": {},
		}
		if _, ok := pythonStdlib[first]; ok {
			return ImportCategoryStdlib
		}
		if strings.HasPrefix(path, ".") {
			return ImportCategoryLocal
		}
		return ImportCategoryThirdParty
	case "typescript", "javascript":
		if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") {
			return ImportCategoryLocal
		}
		return ImportCategoryThirdParty
	case "java":
		if strings.HasPrefix(path, "java.") || strings.HasPrefix(path, "javax.") ||
			strings.HasPrefix(path, "sun.") || strings.HasPrefix(path, "com.sun.") {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "rust":
		if strings.HasPrefix(path, "std::") || strings.HasPrefix(path, "core::") ||
			strings.HasPrefix(path, "alloc::") {
			return ImportCategoryStdlib
		}
		if strings.HasPrefix(path, "crate::") || strings.HasPrefix(path, "super::") ||
			strings.HasPrefix(path, "self::") {
			return ImportCategoryLocal
		}
		return ImportCategoryThirdParty
	case "ruby":
		if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") ||
			strings.HasPrefix(path, ".\\") || strings.HasPrefix(path, "..\\") {
			return ImportCategoryLocal
		}
		rubyStdlib := map[string]struct{}{
			"fileutils": {}, "json": {}, "optparse": {}, "logger": {},
			"socket": {}, "timeout": {}, "pathname": {}, "tempfile": {},
			"csv": {}, "digest": {}, "base64": {}, "uri": {},
			"net/http": {}, "net/smtp": {}, "open-uri": {}, "set": {},
			"singleton": {}, "forwardable": {}, "observer": {},
			"benchmark": {}, "pp": {}, "io": {}, "stringio": {},
			"strscan": {}, "time": {}, "date": {}, "securerandom": {},
			"openssl": {}, "zlib": {}, "yaml": {}, "erb": {},
			"find": {}, "shellwords": {}, "tmpdir": {}, "mutex_m": {},
			"monitor": {}, "conditionvariable": {}, "queue": {},
			"sizedqueue": {}, "thread": {}, "etc": {},
		}
		normalized := strings.ToLower(path)
		if _, ok := rubyStdlib[normalized]; ok {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "c", "cpp":
		if classifyCSystemImport(path) == ImportCategoryStdlib {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "kotlin":
		if strings.HasPrefix(path, "kotlin.") || strings.HasPrefix(path, "java.") ||
			strings.HasPrefix(path, "javax.") || strings.HasPrefix(path, "android.") {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "swift":
		swiftStdlib := map[string]struct{}{
			"Foundation": {}, "UIKit": {}, "Swift": {}, "Combine": {},
			"CoreData": {}, "SwiftUI": {}, "XCTest": {}, "Observation": {},
			"OSLog": {}, "Security": {}, "Network": {}, "CoreGraphics": {},
			"CoreImage": {}, "CoreText": {}, "CoreFoundation": {},
			"Darwin": {}, "Glibc": {}, "WASILibc": {}, "WinSDK": {},
		}
		if _, ok := swiftStdlib[path]; ok {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "scala":
		if strings.HasPrefix(path, "scala.") || strings.HasPrefix(path, "java.") ||
			strings.HasPrefix(path, "javax.") {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "csharp":
		if strings.HasPrefix(path, "System.") || path == "System" ||
			strings.HasPrefix(path, "Microsoft.") || path == "Microsoft" {
			return ImportCategoryStdlib
		}
		return ImportCategoryThirdParty
	case "php":
		if strings.Contains(path, "/") || strings.HasSuffix(path, ".php") {
			return ImportCategoryLocal
		}
		return ImportCategoryThirdParty
	default:
		return ImportCategoryUnknown
	}
}

func visitNodes(cursor *tree_sitter.TreeCursor, fn func(*tree_sitter.Node) bool) {
	for {
		node := cursor.Node()
		if !fn(node) {
			if !gotoNextSiblingOrParent(cursor) {
				return
			}
			continue
		}

		if !cursor.GotoFirstChild() {
			if !gotoNextSiblingOrParent(cursor) {
				return
			}
		}
	}
}

func gotoNextSiblingOrParent(cursor *tree_sitter.TreeCursor) bool {
	for {
		if cursor.GotoNextSibling() {
			return true
		}
		if !cursor.GotoParent() {
			return false
		}
	}
}
