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
	registry          *Registry
	persistenceMatrix *RuntimePersistenceMatrix
}

type runtimeAdapterConfig struct {
	parser            treesitter.Parser
	outputProfile     OutputProfile
	persistenceMatrix *RuntimePersistenceMatrix
}

// RuntimeAdapterOption configures RuntimeAdapter behavior.
type RuntimeAdapterOption func(*runtimeAdapterConfig)

// WithRuntimeTreeSitter enables tree-sitter exploration for runtime adapter use.
func WithRuntimeTreeSitter(parser treesitter.Parser) RuntimeAdapterOption {
	return func(cfg *runtimeAdapterConfig) {
		cfg.parser = parser
	}
}

// WithRuntimeOutputProfile sets formatter profile behavior for runtime adapter use.
func WithRuntimeOutputProfile(profile OutputProfile) RuntimeAdapterOption {
	return func(cfg *runtimeAdapterConfig) {
		cfg.outputProfile = profile
	}
}

// WithRuntimePersistenceMatrix injects a preloaded persistence matrix.
func WithRuntimePersistenceMatrix(matrix *RuntimePersistenceMatrix) RuntimeAdapterOption {
	return func(cfg *runtimeAdapterConfig) {
		cfg.persistenceMatrix = matrix
	}
}

// NewRuntimeAdapter creates a runtime adapter with an explorer registry.
// When a parser is configured, tree-sitter exploration is enabled.
func NewRuntimeAdapter(opts ...RuntimeAdapterOption) *RuntimeAdapter {
	cfg := runtimeAdapterConfig{outputProfile: OutputProfileEnhancement}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	registryOpts := []RegistryOption{WithOutputProfile(cfg.outputProfile)}
	if cfg.parser != nil {
		registryOpts = append(registryOpts, WithTreeSitter(cfg.parser))
	}

	matrix := cfg.persistenceMatrix
	if matrix == nil {
		loaded, err := LoadRuntimePersistenceMatrix(cfg.outputProfile)
		if err == nil {
			matrix = loaded
		}
	}

	return &RuntimeAdapter{
		registry:          NewRegistry(registryOpts...),
		persistenceMatrix: matrix,
	}
}

// Explore runs file exploration and returns summary, explorer, and
// path-level persistence decision suitable for lcm_large_files writes.
func (a *RuntimeAdapter) Explore(
	ctx context.Context,
	sessionID, path string,
	content []byte,
) (summary string, explorer string, persist bool, err error) {
	if a == nil || a.registry == nil {
		return "", "", false, errNilRuntimeAdapter
	}

	result, err := a.registry.Explore(ctx, ExploreInput{
		Path:      path,
		Content:   content,
		SessionID: sessionID,
	})
	if err != nil {
		return "", "", false, err
	}

	explorerUsed := strings.TrimSpace(result.ExplorerUsed)
	policy := RuntimePersistencePolicy{Persist: true}
	if a.persistenceMatrix != nil {
		policy = a.persistenceMatrix.PolicyForExplorer(explorerUsed)
	}

	return strings.TrimSpace(result.Summary), explorerUsed, policy.Persist, nil
}
