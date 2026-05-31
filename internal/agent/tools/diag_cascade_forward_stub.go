//go:build !treesitter

package tools

import (
	"context"
	"fmt"
)

// ForwardImportResolution returns an error when treesitter is not enabled.
func ForwardImportResolution(
	_ context.Context,
	_ any,
	_ string,
	_ string,
	_ string,
) (map[string][]string, error) {
	return nil, fmt.Errorf("forward import resolution not available: treesitter not enabled")
}
