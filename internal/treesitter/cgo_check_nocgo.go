//go:build !cgo

package treesitter

func init() {
	panic("crush requires CGO_ENABLED=1 and a C compiler for tree-sitter support")
}
