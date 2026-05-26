package explorer

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SymbolInfo describes a named symbol extracted from a file.
type SymbolInfo struct {
	// Name is the symbol identifier (e.g., function/method/variable name).
	Name string
	// Kind is the symbol category (e.g., "function", "class", "method", "variable").
	Kind string
	// StartLine is the 1-based line where the symbol begins.
	StartLine int
	// EndLine is the 1-based line where the symbol ends (0 if unknown).
	EndLine int
}

// CodeSection describes a contiguous region of code within a file.
type CodeSection struct {
	// Name is the section identifier (e.g., function or class name).
	Name string
	// Type is the section category (e.g., "function", "class", "method",
	// "interface").
	Type string
	// StartLine is the 1-based starting line.
	StartLine int
	// EndLine is the 1-based ending line.
	EndLine int
	// Content is the raw text of the section (empty when not captured).
	Content string
}

// FileStructure holds a structured representation of a file's contents.
type FileStructure struct {
	// Symbols lists extracted symbols.
	Symbols []SymbolInfo
	// Imports lists import declarations (module/package paths).
	Imports []string
	// Sections lists code sections (functions, classes, etc.).
	Sections []CodeSection
}

// ParseFileStructure parses a text summary from Explore() into a
// FileStructure. It recognises tree-sitter and shell explorer output
// conventions (lines like "  - function foo (line 12)").
func ParseFileStructure(summary string) *FileStructure {
	fs := &FileStructure{}

	// Pattern: "  - kind name (visibility, line N)" or "  - kind name (line N)"
	// or "  - kind name (visibility)" or "  - kind name"
	symbolRe := regexp.MustCompile(
		`^\s+-\s+(\S+)\s+(\S+)\s+(?:\((\S+),\s+line\s+(\d+)\)|\((\S+)\)|\(line\s+(\d+)\))?$`,
	)
	// Simplified pattern that captures "kind name ..." lines.
	symbolSimpleRe := regexp.MustCompile(
		`^\s+-\s+(\S+)\s+(\S+?)(?:\s+\(.*?line\s+(\d+).*?\))?$`,
	)

	scanner := bufio.NewScanner(strings.NewReader(summary))
	for scanner.Scan() {
		line := scanner.Text()

		// Detect imports: "  - path (category)" or "  - path".
		if strings.HasPrefix(line, "  - ") && !symbolRe.MatchString(line) {
			rest := strings.TrimPrefix(line, "  - ")
			if rest != "" && !strings.Contains(rest, " ") {
				// Bare path after dash — treat as import.
				fs.Imports = append(fs.Imports, rest)
				continue
			}
			// "path (category)" — strip the category part.
			if idx := strings.LastIndex(rest, " ("); idx > 0 {
				fs.Imports = append(fs.Imports, rest[:idx])
				continue
			}
		}

		// Try full pattern first.
		if m := symbolRe.FindStringSubmatch(line); m != nil {
			sym := SymbolInfo{Name: m[2], Kind: m[1]}
			if m[4] != "" {
				sym.StartLine, _ = strconv.Atoi(m[4])
			} else if m[6] != "" {
				sym.StartLine, _ = strconv.Atoi(m[6])
			}
			fs.Symbols = append(fs.Symbols, sym)
			fs.Sections = append(fs.Sections, CodeSection{
				Name:      sym.Name,
				Type:      sym.Kind,
				StartLine: sym.StartLine,
				EndLine:   sym.EndLine,
			})
			continue
		}

		// Fallback to simplified pattern.
		if m := symbolSimpleRe.FindStringSubmatch(line); m != nil {
			sym := SymbolInfo{Name: m[2], Kind: m[1]}
			if m[3] != "" {
				sym.StartLine, _ = strconv.Atoi(m[3])
			}
			fs.Symbols = append(fs.Symbols, sym)
			fs.Sections = append(fs.Sections, CodeSection{
				Name:      sym.Name,
				Type:      sym.Kind,
				StartLine: sym.StartLine,
				EndLine:   sym.EndLine,
			})
		}
	}

	return fs
}

// String returns a human-readable representation of the FileStructure.
func (fs *FileStructure) String() string {
	var sb strings.Builder
	if len(fs.Imports) > 0 {
		sb.WriteString("Imports:\n")
		for _, imp := range fs.Imports {
			fmt.Fprintf(&sb, "  - %s\n", imp)
		}
	}
	if len(fs.Symbols) > 0 {
		sb.WriteString("Symbols:\n")
		for _, sym := range fs.Symbols {
			if sym.StartLine > 0 {
				fmt.Fprintf(&sb, "  - %s %s (line %d)\n", sym.Kind, sym.Name, sym.StartLine)
			} else {
				fmt.Fprintf(&sb, "  - %s %s\n", sym.Kind, sym.Name)
			}
		}
	}
	return sb.String()
}

// ExploreStructured returns a structured representation of a file. It uses
// the first explorer that can handle the file and parses the text result into
// a FileStructure. When the underlying explorer is a TreeSitterExplorer it
// extracts symbols directly; otherwise it falls back to ParseFileStructure on
// the text summary.
func (r *Registry) ExploreStructured(ctx context.Context, input ExploreInput) (*FileStructure, error) {
	// Use tree-sitter explorer directly if available for richer data.
	if r.tsParser != nil {
		tsExp := newTreeSitterExplorer(r.tsParser, r.formatterProfile)
		if tsExp != nil && tsExp.CanHandle(input.Path, input.Content) {
			result, err := tsExp.Explore(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("structured explore failed: %w", err)
			}
			return ParseFileStructure(result.Summary), nil
		}
	}

	// Fall back to the general explore + parse approach.
	result, err := r.Explore(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("structured explore failed: %w", err)
	}
	return ParseFileStructure(result.Summary), nil
}
