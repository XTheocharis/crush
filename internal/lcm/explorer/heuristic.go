package explorer

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/crush/internal/lcm/explorer/stdlib"
	"github.com/charmbracelet/crush/internal/treesitter"
)

// EnrichedSymbol contains inferred metadata for one symbol.
type EnrichedSymbol struct {
	Name       string
	Kind       string
	Line       int
	Visibility string
}

// EnrichedAnalysis augments tree-sitter analysis with heuristic signals.
type EnrichedAnalysis struct {
	Language         string
	ImportCategories map[string][]string
	Symbols          []EnrichedSymbol
	Idioms           []string
	ModulePatterns   []string
}

// EnrichAnalysis applies lightweight language heuristics to tree-sitter output.
func EnrichAnalysis(analysis *treesitter.FileAnalysis, content []byte) *EnrichedAnalysis {
	if analysis == nil {
		return &EnrichedAnalysis{}
	}

	lang := strings.ToLower(strings.TrimSpace(analysis.Language))
	text := string(content)
	imports := collectImports(lang, analysis.Imports, text)
	categorized := categorizeImports(lang, imports)
	idioms := detectIdioms(lang, text)
	modulePatterns := detectModulePatterns(lang, text)
	enrichedSymbols := inferSymbolVisibility(lang, analysis.Symbols)

	return &EnrichedAnalysis{
		Language:         lang,
		ImportCategories: categorized,
		Symbols:          enrichedSymbols,
		Idioms:           idioms,
		ModulePatterns:   modulePatterns,
	}
}

func collectImports(lang string, existing []treesitter.ImportInfo, content string) []string {
	seen := map[string]struct{}{}
	imports := make([]string, 0, len(existing)+8)

	for _, imp := range existing {
		p := strings.TrimSpace(imp.Path)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		imports = append(imports, p)
	}

	for _, imp := range parseImportsFromContent(lang, content) {
		imp = strings.TrimSpace(imp)
		if imp == "" {
			continue
		}
		if _, ok := seen[imp]; ok {
			continue
		}
		seen[imp] = struct{}{}
		imports = append(imports, imp)
	}

	return imports
}

func parseImportsFromContent(lang, content string) []string {
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(strings.Trim(v, `"'`))
		if v != "" {
			out = append(out, v)
		}
	}

	switch lang {
	case "go":
		re := regexp.MustCompile(`(?m)^\s*import\s+(?:[\w.]+\s+)?"([^"]+)"`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	case "python":
		importRe := regexp.MustCompile(`(?m)^\s*import\s+([^\n#]+)`)
		fromRe := regexp.MustCompile(`(?m)^\s*from\s+([\.\w]+)\s+import\s+`)
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			parts := strings.Split(m[1], ",")
			for _, p := range parts {
				p = strings.TrimSpace(strings.Fields(p)[0])
				add(strings.Split(p, ".")[0])
			}
		}
		for _, m := range fromRe.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	case "javascript", "typescript", "tsx", "jsx":
		importRe := regexp.MustCompile(`(?m)^\s*import\s+(?:.+?\s+from\s+)?["']([^"']+)["']`)
		requireRe := regexp.MustCompile(`require\(["']([^"']+)["']\)`)
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
		for _, m := range requireRe.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	case "rust":
		re := regexp.MustCompile(`(?m)^\s*use\s+([^;]+);`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			path := strings.TrimSpace(m[1])
			if i := strings.Index(path, "::"); i > 0 {
				path = path[:i]
			}
			add(path)
		}
	case "java":
		re := regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([^;]+);`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	case "ruby":
		reqRe := regexp.MustCompile(`(?m)^\s*require\s+["']([^"']+)["']`)
		relRe := regexp.MustCompile(`(?m)^\s*require_relative\s+["']([^"']+)["']`)
		for _, m := range reqRe.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
		for _, m := range relRe.FindAllStringSubmatch(content, -1) {
			add("./" + strings.TrimPrefix(m[1], "./"))
		}
	case "c", "cpp":
		re := regexp.MustCompile(`(?m)^\s*#include\s+[<"]([^>"]+)[>"]`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	}

	return out
}

func categorizeImports(lang string, imports []string) map[string][]string {
	cats := map[string][]string{
		treesitter.ImportCategoryStdlib:     {},
		treesitter.ImportCategoryThirdParty: {},
		treesitter.ImportCategoryLocal:      {},
		treesitter.ImportCategoryUnknown:    {},
	}

	for _, imp := range imports {
		cat := classifyImport(lang, imp)
		cats[cat] = append(cats[cat], imp)
	}

	for k := range cats {
		if len(cats[k]) == 0 {
			delete(cats, k)
			continue
		}
		sort.Strings(cats[k])
	}

	return cats
}

func classifyImport(lang, imp string) string {
	imp = strings.TrimSpace(imp)
	if imp == "" {
		return treesitter.ImportCategoryUnknown
	}

	if strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../") || strings.HasPrefix(imp, "/") {
		return treesitter.ImportCategoryLocal
	}

	switch lang {
	case "go":
		if strings.HasPrefix(imp, "github.com/charmbracelet/crush/") {
			return treesitter.ImportCategoryLocal
		}
		if stdlib.IsGoStdlib(imp) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "python":
		if strings.HasPrefix(imp, ".") {
			return treesitter.ImportCategoryLocal
		}
		head := strings.Split(imp, ".")[0]
		if stdlib.IsPythonStdlib(head) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "javascript", "typescript", "tsx", "jsx":
		if strings.HasPrefix(imp, "#") || strings.HasPrefix(imp, "@/") {
			return treesitter.ImportCategoryLocal
		}
		normalized := strings.TrimPrefix(imp, "node:")
		head := strings.Split(normalized, "/")[0]
		if stdlib.IsNodeStdlib(head) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "rust":
		if imp == "crate" || imp == "self" || imp == "super" {
			return treesitter.ImportCategoryLocal
		}
		if stdlib.IsRustStdlib(imp) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "java":
		if strings.HasPrefix(imp, "java.") || strings.HasPrefix(imp, "javax.") || strings.HasPrefix(imp, "jdk.") {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "ruby":
		if strings.HasPrefix(imp, "./") {
			return treesitter.ImportCategoryLocal
		}
		if stdlib.IsRubyStdlib(imp) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "c":
		head := strings.TrimSuffix(imp, ".h")
		if stdlib.IsCStdlib(head) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	case "cpp":
		head := strings.TrimSuffix(imp, ".h")
		if stdlib.IsCppStdlib(head) || stdlib.IsCStdlib(head) {
			return treesitter.ImportCategoryStdlib
		}
		return treesitter.ImportCategoryThirdParty
	default:
		if strings.HasPrefix(imp, ".") {
			return treesitter.ImportCategoryLocal
		}
		return treesitter.ImportCategoryUnknown
	}
}

func detectIdioms(lang, content string) []string {
	out := []string{}
	add := func(v string) {
		for _, cur := range out {
			if cur == v {
				return
			}
		}
		out = append(out, v)
	}

	lower := strings.ToLower(content)

	switch lang {
	case "javascript", "typescript", "tsx", "jsx":
		reactComp := regexp.MustCompile(`(?m)(?:function|const|let|var)\s+([A-Z][A-Za-z0-9_]*)`)
		if (strings.Contains(lower, "react") || strings.Contains(content, "<")) && reactComp.MatchString(content) {
			add("react_component")
		}
		if regexp.MustCompile(`(?s)async\s+function\s*\*`).MatchString(content) || regexp.MustCompile(`(?s)async\s*\([^)]*\)\s*=>[\s\S]*?yield`).MatchString(content) {
			add("async_generator")
		}
		if strings.Contains(lower, "abstract class") {
			add("abstract_class")
		}
	case "python":
		if strings.Contains(content, "@dataclass") {
			add("dataclass")
		}
		if strings.Contains(content, "class ") && (strings.Contains(content, "(ABC)") || strings.Contains(content, "@abstractmethod")) {
			add("abstract_class")
		}
		if regexp.MustCompile(`(?s)async\s+def\s+\w+\s*\([^)]*\):[\s\S]*?\byield\b`).MatchString(content) {
			add("async_generator")
		}
	case "java", "kotlin", "scala":
		if strings.Contains(lower, "abstract class") {
			add("abstract_class")
		}
	}

	sort.Strings(out)
	return out
}

func detectModulePatterns(lang, content string) []string {
	out := []string{}
	add := func(v string) {
		for _, cur := range out {
			if cur == v {
				return
			}
		}
		out = append(out, v)
	}

	if strings.Contains(content, `if __name__ == "__main__":`) || strings.Contains(content, `if __name__ == '__main__':`) {
		add("python_main_guard")
	}
	if strings.Contains(content, "module.exports") || strings.Contains(content, "exports.") {
		add("commonjs_exports")
	}
	if strings.Contains(content, "export default") {
		add("esm_default_export")
	}
	if lang == "go" && strings.Contains(content, "package main") && regexp.MustCompile(`(?m)^\s*func\s+main\s*\(`).MatchString(content) {
		add("go_main_package")
	}
	if lang == "rust" && regexp.MustCompile(`(?m)^\s*fn\s+main\s*\(`).MatchString(content) {
		add("rust_main_function")
	}

	sort.Strings(out)
	return out
}

func inferSymbolVisibility(lang string, symbols []treesitter.SymbolInfo) []EnrichedSymbol {
	out := make([]EnrichedSymbol, 0, len(symbols))
	for _, s := range symbols {
		out = append(out, EnrichedSymbol{
			Name:       s.Name,
			Kind:       s.Kind,
			Line:       s.Line,
			Visibility: inferVisibility(lang, s),
		})
	}
	return out
}

func inferVisibility(lang string, symbol treesitter.SymbolInfo) string {
	name := strings.TrimSpace(symbol.Name)
	if name == "" {
		return "unknown"
	}

	hasModifier := func(want string) bool {
		for _, m := range symbol.Modifiers {
			if strings.EqualFold(strings.TrimSpace(m), want) {
				return true
			}
		}
		return false
	}

	switch lang {
	case "go":
		runes := []rune(name)
		if len(runes) > 0 && unicode.IsUpper(runes[0]) {
			return "public"
		}
		return "private"
	case "python":
		if strings.HasPrefix(name, "__") && !strings.HasSuffix(name, "__") {
			return "private"
		}
		if strings.HasPrefix(name, "_") {
			return "private"
		}
		return "public"
	case "java", "kotlin", "scala", "csharp":
		if hasModifier("public") {
			return "public"
		}
		if hasModifier("protected") {
			return "protected"
		}
		if hasModifier("private") {
			return "private"
		}
		return "package"
	case "rust":
		if hasModifier("pub") || hasModifier("public") {
			return "public"
		}
		return "private"
	case "javascript", "typescript", "tsx", "jsx":
		if hasModifier("export") || hasModifier("public") {
			return "public"
		}
		if strings.HasPrefix(name, "_") {
			return "private"
		}
		return "internal"
	default:
		if strings.HasPrefix(name, "_") {
			return "private"
		}
		return "public"
	}
}
