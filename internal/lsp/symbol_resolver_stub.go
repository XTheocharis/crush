//go:build !treesitter

package lsp

import (
	"context"
	"fmt"
)

// ResolutionMode determines how a symbol query is interpreted.
type ResolutionMode int

const (
	ModeSimple ResolutionMode = iota
	ModeRelative
	ModeAbsolute
)

// SymbolCandidate represents a resolved symbol with its metadata and location.
type SymbolCandidate struct {
	Name          string
	Package       string
	Kind          string
	OverloadIndex int
	Location      Location
}

// SymbolQuery is the parsed representation of a user-provided resolution
// request.
type SymbolQuery struct {
	Name          string
	Package       string
	ModuleRoot    string
	OverloadIndex int
	Mode          ResolutionMode
}

// Location is a stub type for protocol.Location when treesitter is disabled.
type Location struct {
	URI   string
	Range Range
}

// Range represents a text range in a document.
type Range struct {
	Start Position
	End   Position
}

// Position represents a position in a document.
type Position struct {
	Line      int
	Character int
}

// SymbolResolver is a stub that returns errors when treesitter is not enabled.
type SymbolResolver struct{}

// NewSymbolResolver returns a stub SymbolResolver when treesitter is not
// enabled.
func NewSymbolResolver(_ *Manager) *SymbolResolver {
	return &SymbolResolver{}
}

// ParseQuery parses a raw input string into a structured SymbolQuery.
func ParseQuery(input string) SymbolQuery {
	return SymbolQuery{OverloadIndex: -1}
}

// Resolve returns an error when treesitter is not enabled.
func (r *SymbolResolver) Resolve(
	_ context.Context,
	query string,
	_ string,
	_ string,
) ([]SymbolCandidate, error) {
	return nil, fmt.Errorf(
		"symbol resolver not available: treesitter not enabled (query: %q)",
		query,
	)
}
