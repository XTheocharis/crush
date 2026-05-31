//go:build !treesitter

package tools

// newSymbolParser returns nil when tree-sitter is not available.
func newSymbolParser(_ any) symbolParser {
	return nil
}
