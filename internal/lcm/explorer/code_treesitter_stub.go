//go:build !treesitter

package explorer

// newTreeSitterExplorer returns nil when built without tree-sitter support.
func newTreeSitterExplorer(_ any, _ OutputProfile) Explorer {
	return nil
}
