package explorer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// TreeSitterExplorer explores code files using the tree-sitter parser.
type TreeSitterExplorer struct {
	parser           treesitter.Parser
	formatterProfile OutputProfile
}

var _ Explorer = (*TreeSitterExplorer)(nil)

// NewTreeSitterExplorer creates a tree-sitter-backed explorer.
func NewTreeSitterExplorer(parser treesitter.Parser) *TreeSitterExplorer {
	return &TreeSitterExplorer{parser: parser, formatterProfile: OutputProfileEnhancement}
}

func (e *TreeSitterExplorer) CanHandle(path string, content []byte) bool {
	if e == nil || e.parser == nil {
		return false
	}

	lang := treesitter.MapPath(path)
	if lang == "" {
		lang = detectLanguage(path, content)
	}
	if lang == "" {
		return false
	}

	return e.parser.SupportsLanguage(lang) && e.parser.HasTags(lang)
}

func (e *TreeSitterExplorer) Explore(ctx context.Context, input ExploreInput) (ExploreResult, error) {
	if len(input.Content) > MaxFullLoadSize {
		summary := fmt.Sprintf("File too large: %s (%d bytes)", filepath.Base(input.Path), len(input.Content))
		return ExploreResult{Summary: summary, ExplorerUsed: "treesitter", TokenEstimate: estimateTokens(summary)}, nil
	}

	if e == nil || e.parser == nil {
		summary := fmt.Sprintf("Tree-sitter parser unavailable: %s", filepath.Base(input.Path))
		return ExploreResult{Summary: summary, ExplorerUsed: "treesitter", TokenEstimate: estimateTokens(summary)}, nil
	}

	analysis, err := e.parser.Analyze(ctx, input.Path, input.Content)
	if err != nil {
		return ExploreResult{}, err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tree-sitter file: %s\n", filepath.Base(input.Path))

	lang := ""
	if analysis != nil {
		lang = analysis.Language
	}
	if lang == "" {
		lang = treesitter.MapPath(input.Path)
	}
	if lang != "" {
		fmt.Fprintf(&sb, "Language: %s\n", lang)
	}

	if analysis != nil {
		enriched := EnrichAnalysis(analysis, input.Content)

		if len(enriched.ImportCategories) > 0 {
			sb.WriteString("\nImports:\n")
			for _, cat := range []string{"stdlib", "third_party", "local", "unknown"} {
				items := enriched.ImportCategories[cat]
				for _, item := range items {
					fmt.Fprintf(&sb, "  - %s (%s)\n", item, cat)
				}
			}
		} else if len(analysis.Imports) > 0 {
			sb.WriteString("\nImports:\n")
			for _, imp := range analysis.Imports {
				if imp.Category != "" {
					fmt.Fprintf(&sb, "  - %s (%s)\n", imp.Path, imp.Category)
				} else {
					fmt.Fprintf(&sb, "  - %s\n", imp.Path)
				}
			}
		}

		if len(enriched.Symbols) > 0 {
			sb.WriteString("\nSymbols:\n")
			for _, sym := range enriched.Symbols {
				kind := strings.TrimSpace(sym.Kind)
				if kind == "" {
					kind = "symbol"
				}
				if sym.Line > 0 {
					fmt.Fprintf(&sb, "  - %s %s (%s, line %d)\n", kind, sym.Name, sym.Visibility, sym.Line)
				} else {
					fmt.Fprintf(&sb, "  - %s %s (%s)\n", kind, sym.Name, sym.Visibility)
				}
			}
		} else if len(analysis.Symbols) > 0 {
			sb.WriteString("\nSymbols:\n")
			for _, sym := range analysis.Symbols {
				kind := strings.TrimSpace(sym.Kind)
				if kind == "" {
					kind = "symbol"
				}
				if sym.Line > 0 {
					fmt.Fprintf(&sb, "  - %s %s (line %d)\n", kind, sym.Name, sym.Line)
				} else {
					fmt.Fprintf(&sb, "  - %s %s\n", kind, sym.Name)
				}
			}
		}

		if len(enriched.Idioms) > 0 {
			sb.WriteString("\nIdioms:\n")
			for _, idiom := range enriched.Idioms {
				fmt.Fprintf(&sb, "  - %s\n", idiom)
			}
		}

		if len(enriched.ModulePatterns) > 0 {
			sb.WriteString("\nModule patterns:\n")
			for _, pattern := range enriched.ModulePatterns {
				fmt.Fprintf(&sb, "  - %s\n", pattern)
			}
		}

		if len(analysis.Tags) > 0 {
			sb.WriteString("\nTags:\n")
			for _, tag := range analysis.Tags {
				if tag.Line > 0 {
					fmt.Fprintf(&sb, "  - %s %s (line %d)\n", tag.Kind, tag.Name, tag.Line)
				} else {
					fmt.Fprintf(&sb, "  - %s %s\n", tag.Kind, tag.Name)
				}
			}
		}
	}

	result := strings.TrimSpace(sb.String())
	return ExploreResult{Summary: result, ExplorerUsed: "treesitter", TokenEstimate: estimateTokens(result)}, nil
}
