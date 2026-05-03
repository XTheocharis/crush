//go:build !cgo

package treesitter

func init() {
	panic("crush source builds require CGO_ENABLED=1 and a working C compiler; non-CGO builds are unsupported")
}
