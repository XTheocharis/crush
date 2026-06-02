//go:build !treesitter

package extensions

import (
	"context"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/ext"
)

// TreesitterExtension is a no-op when tree-sitter is not enabled.
type TreesitterExtension struct {
	handler *tools.ValidationHandler
}

func (e *TreesitterExtension) Name() string { return "treesitter-validation" }

func (e *TreesitterExtension) Init(_ context.Context, _ ext.HostContext) error {
	return nil
}

func (e *TreesitterExtension) Shutdown(_ context.Context) error {
	return nil
}

// Handler returns nil when tree-sitter is not available.
func (e *TreesitterExtension) Handler() *tools.ValidationHandler {
	return nil
}

// StepHooks returns nil when tree-sitter is not available.
func (e *TreesitterExtension) StepHooks() []ext.StepHook {
	return nil
}

func extractEditInfoFromStep(_ fantasy.StepResult) []editInfo {
	return nil
}

type editInfo struct {
	filePath   string
	oldContent string
	newContent string
	editSpec   tools.EditSpec
}

var (
	_ ext.Extension        = (*TreesitterExtension)(nil)
	_ ext.StepHookProvider = (*TreesitterExtension)(nil)
)
