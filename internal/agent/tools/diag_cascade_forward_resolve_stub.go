//go:build !treesitter

package tools

import "context"

func (dc *DiagnosticCascade) forwardResolve(_ context.Context, _ []string) {}
