//go:build !treesitter

package tools

// newSymbolParserFromAny delegates to the stub newSymbolParser.
func newSymbolParserFromAny(p any) symbolParser {
	return newSymbolParser(p)
}
