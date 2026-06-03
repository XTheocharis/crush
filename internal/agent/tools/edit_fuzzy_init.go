//go:build treesitter

package tools

import "github.com/charmbracelet/crush/internal/treesitter"

// newSymbolParserFromAny type-asserts an any value to treesitter.Parser
// and delegates to newSymbolParser.
func newSymbolParserFromAny(p any) symbolParser {
	if ts, ok := p.(treesitter.Parser); ok {
		return newSymbolParser(ts)
	}
	return nil
}
