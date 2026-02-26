package explorer

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

var errNilRuntimeAdapter = errors.New("explorer runtime adapter is nil")

// RuntimeAdapter provides a minimal runtime API for LCM decorator usage.
// It executes exploration and returns persistence-ready fields.
type RuntimeAdapter struct {
	registry *Registry
}

// NewRuntimeAdapter creates a runtime adapter with an explorer registry.
// When parser is non-nil, tree-sitter exploration is enabled.
func NewRuntimeAdapter(parser treesitter.Parser) *RuntimeAdapter {
	opts := make([]RegistryOption, 0, 1)
	if parser != nil {
		opts = append(opts, WithTreeSitter(parser))
	}

	return &RuntimeAdapter{registry: NewRegistry(opts...)}
}

// Explore runs file exploration and returns summary and explorer values suitable
// for persistence in lcm_large_files.
func (a *RuntimeAdapter) Explore(
	ctx context.Context,
	sessionID, path string,
	content []byte,
) (summary string, explorer string, err error) {
	if a == nil || a.registry == nil {
		return "", "", errNilRuntimeAdapter
	}

	result, err := a.registry.Explore(ctx, ExploreInput{
		Path:      path,
		Content:   content,
		SessionID: sessionID,
	})
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(result.Summary), strings.TrimSpace(result.ExplorerUsed), nil
}
