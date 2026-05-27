//go:build !treesitter

package extensions

import (
	"context"

	"github.com/charmbracelet/crush/internal/ext"
)

// TreesitterExtension is a no-op when tree-sitter is not enabled.
type TreesitterExtension struct{}

func (e *TreesitterExtension) Name() string { return "treesitter-validation" }

func (e *TreesitterExtension) Init(_ context.Context, _ ext.HostContext) error {
	return nil
}

func (e *TreesitterExtension) Shutdown(_ context.Context) error {
	return nil
}

var _ ext.Extension = (*TreesitterExtension)(nil)
