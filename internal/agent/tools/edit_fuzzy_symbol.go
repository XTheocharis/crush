//go:build treesitter

package tools

import (
	"context"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// treesitterSymbolParser adapts treesitter.Parser to the symbolParser
// interface used by fuzzy symbol lookup.
type treesitterSymbolParser struct {
	parser treesitter.Parser
}

// newSymbolParser creates a symbolParser backed by a real tree-sitter parser.
func newSymbolParser(p treesitter.Parser) symbolParser {
	if p == nil {
		return nil
	}
	return &treesitterSymbolParser{parser: p}
}

func (t *treesitterSymbolParser) Analyze(ctx context.Context, path string, content []byte) (*symbolAnalysis, error) {
	fa, err := t.parser.Analyze(ctx, path, content)
	if err != nil {
		return nil, err
	}
	if fa == nil {
		return nil, nil
	}

	result := &symbolAnalysis{}
	for _, sym := range fa.Symbols {
		result.Symbols = append(result.Symbols, symbolDef{
			Name: sym.Name,
			Kind: sym.Kind,
			Line: sym.Line,
		})
	}
	return result, nil
}
