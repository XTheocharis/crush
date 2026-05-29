//go:build treesitter

package lsp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/powernap/pkg/lsp/protocol"
)

// ResolutionMode determines how a symbol query is interpreted.
type ResolutionMode int

const (
	// ModeSimple resolves a bare name within the current package (e.g. "foo").
	ModeSimple ResolutionMode = iota
	// ModeRelative resolves a dot-separated package.symbol path (e.g. "pkg.foo").
	ModeRelative
	// ModeAbsolute resolves a fully qualified module path (e.g.
	// "github.com/org/repo/pkg.foo").
	ModeAbsolute
)

// SymbolCandidate represents a resolved symbol with its metadata and location.
type SymbolCandidate struct {
	// Name is the short symbol name (e.g. "Foo", "handleRequest").
	Name string
	// Package is the enclosing package path (e.g. "pkg", "github.com/org/repo/pkg").
	Package string
	// Kind describes the symbol category: "function", "type", "method",
	// "var", "const", "namespace", etc.
	Kind string
	// OverloadIndex is the 0-based position among symbols sharing the same
	// name. A value of -1 means no overload disambiguation was applied.
	OverloadIndex int
	// Location is the LSP-reported position of the symbol.
	Location protocol.Location
}

// SymbolQuery is the parsed representation of a user-provided resolution
// request.
type SymbolQuery struct {
	// Name is the symbol name to search for.
	Name string
	// Package is an optional package qualifier.
	Package string
	// ModuleRoot is an optional full module path prefix (used in absolute mode).
	ModuleRoot string
	// OverloadIndex selects the Nth candidate (0-based) among results sharing
	// the same name. A value of -1 means "return all candidates".
	OverloadIndex int
	// Mode is the detected resolution mode.
	Mode ResolutionMode
}

// SymbolResolver resolves symbol names to candidate locations using the LSP
// manager's workspace/symbol endpoint. It supports three resolution modes
// (simple, relative, absolute) with optional [N] overload disambiguation.
type SymbolResolver struct {
	manager *Manager
}

// NewSymbolResolver creates a SymbolResolver backed by the given LSP manager.
func NewSymbolResolver(m *Manager) *SymbolResolver {
	return &SymbolResolver{manager: m}
}

// ParseQuery parses a raw input string into a structured SymbolQuery.
//
// Modes:
//   - Simple: no dots and no path separators → "foo" searches the current
//     package.
//   - Relative: dots but no path separators → "pkg.foo" searches a specific
//     package.
//   - Absolute: contains "/" → "github.com/org/repo/pkg.foo" resolves by full
//     module path.
//
// An optional "[N]" suffix selects the Nth overload (0-indexed).
func ParseQuery(input string) SymbolQuery {
	input = strings.TrimSpace(input)
	q := SymbolQuery{
		OverloadIndex: -1,
	}

	// Extract overload index: "foo[2]" → name "foo", index 2.
	input, q.OverloadIndex = parseOverloadIndex(input)

	// Detect mode by presence of dots and path separators.
	hasSlash := strings.Contains(input, "/")
	hasDot := strings.Contains(input, ".")

	switch {
	case hasSlash:
		q.Mode = ModeAbsolute
		// Last dot after the final slash separates module path from symbol.
		lastSlash := strings.LastIndex(input, "/")
		remainder := input[lastSlash+1:]
		if idx := strings.LastIndex(remainder, "."); idx >= 0 {
			q.ModuleRoot = input[:lastSlash+1] + remainder[:idx]
			q.Name = remainder[idx+1:]
		} else {
			// No dot after last slash — treat the whole remainder as the name.
			q.ModuleRoot = input[:lastSlash]
			q.Name = remainder
		}
	case hasDot:
		q.Mode = ModeRelative
		idx := strings.LastIndex(input, ".")
		q.Package = input[:idx]
		q.Name = input[idx+1:]
	default:
		q.Mode = ModeSimple
		q.Name = input
	}

	return q
}

// parseOverloadIndex extracts a trailing "[N]" suffix and returns the
// remaining input and the parsed index. Returns -1 if no index is present.
func parseOverloadIndex(input string) (string, int) {
	if len(input) < 3 || input[len(input)-1] != ']' {
		return input, -1
	}
	open := strings.LastIndex(input, "[")
	if open < 0 {
		return input, -1
	}
	numStr := input[open+1 : len(input)-1]
	if len(numStr) == 0 {
		return input, -1
	}
	var idx int
	for _, ch := range numStr {
		if ch < '0' || ch > '9' {
			return input, -1
		}
		idx = idx*10 + int(ch-'0')
	}
	return input[:open], idx
}

// Resolve resolves a symbol query using workspace/symbol across all running
// LSP servers. The currentPkg parameter provides context for simple-mode
// queries, and moduleRoot is used for absolute-mode filtering.
func (r *SymbolResolver) Resolve(
	ctx context.Context,
	query string,
	currentPkg string,
	moduleRoot string,
) ([]SymbolCandidate, error) {
	q := ParseQuery(query)

	var candidates []SymbolCandidate

	// Collect workspace symbols from all running LSP clients.
	for name, client := range r.manager.clients.Seq2() {
		symbols, err := r.resolveFromClient(ctx, name, client, q, currentPkg, moduleRoot)
		if err != nil {
			continue
		}
		candidates = append(candidates, symbols...)
	}

	// Sort candidates: prefer exact name matches, then by package match,
	// then alphabetically.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name ||
			(candidates[i].Name == candidates[j].Name && candidates[i].Package < candidates[j].Package)
	})

	// Apply overload index if specified.
	if q.OverloadIndex >= 0 {
		if q.OverloadIndex >= len(candidates) {
			return nil, fmt.Errorf(
				"overload index %d out of range: found %d candidates for %q",
				q.OverloadIndex, len(candidates), query,
			)
		}
		candidates = candidates[q.OverloadIndex : q.OverloadIndex+1]
	}

	return candidates, nil
}

// resolveFromClient queries a single LSP client and filters results by mode.
func (r *SymbolResolver) resolveFromClient(
	ctx context.Context,
	serverName string,
	client *Client,
	q SymbolQuery,
	currentPkg string,
	moduleRoot string,
) ([]SymbolCandidate, error) {
	var result []protocol.SymbolInformation
	err := r.manager.executor.Submit(ctx, serverName, func() error {
		var ferr error
		result, ferr = client.WorkspaceSymbol(ctx, q.Name)
		return ferr
	})
	if err != nil {
		return nil, err
	}

	var candidates []SymbolCandidate
	for _, sym := range result {
		c := symbolInfoToCandidate(sym)
		if r.matchesMode(c, q, currentPkg, moduleRoot) {
			candidates = append(candidates, c)
		}
	}
	return candidates, nil
}

// matchesMode checks whether a candidate matches the query's resolution mode.
func (r *SymbolResolver) matchesMode(
	c SymbolCandidate,
	q SymbolQuery,
	currentPkg string,
	moduleRoot string,
) bool {
	switch q.Mode {
	case ModeSimple:
		// Match if the candidate is in the current package or has no package
		// qualifier.
		if currentPkg == "" {
			return true
		}
		return c.Package == currentPkg || strings.HasSuffix(c.Package, "/"+currentPkg)
	case ModeRelative:
		return strings.HasSuffix(c.Package, "/"+q.Package) || c.Package == q.Package
	case ModeAbsolute:
		if moduleRoot == "" {
			return true
		}
		return strings.HasPrefix(c.Package, moduleRoot)
	default:
		return true
	}
}

// symbolInfoToCandidate converts a protocol.SymbolInformation to a
// SymbolCandidate.
func symbolInfoToCandidate(sym protocol.SymbolInformation) SymbolCandidate {
	pkg := ""
	if sym.ContainerName != "" {
		pkg = sym.ContainerName
	}

	return SymbolCandidate{
		Name:          sym.Name,
		Package:       pkg,
		Kind:          symbolKindToString(sym.Kind),
		OverloadIndex: -1,
		Location:      sym.Location,
	}
}

// symbolKindToString maps an LSP SymbolKind to a human-readable string.
func symbolKindToString(kind protocol.SymbolKind) string {
	switch kind {
	case protocol.File:
		return "file"
	case protocol.Module:
		return "module"
	case protocol.Namespace:
		return "namespace"
	case protocol.Package:
		return "package"
	case protocol.Class:
		return "class"
	case protocol.Method:
		return "method"
	case protocol.Property:
		return "property"
	case protocol.Field:
		return "field"
	case protocol.Constructor:
		return "constructor"
	case protocol.Enum:
		return "enum"
	case protocol.Interface:
		return "interface"
	case protocol.Function:
		return "function"
	case protocol.Variable:
		return "var"
	case protocol.Constant:
		return "const"
	case protocol.String:
		return "string"
	case protocol.Number:
		return "number"
	case protocol.Boolean:
		return "boolean"
	case protocol.Array:
		return "array"
	case protocol.Object:
		return "object"
	case protocol.Key:
		return "key"
	case protocol.Null:
		return "null"
	case protocol.EnumMember:
		return "enum_member"
	case protocol.Struct:
		return "struct"
	case protocol.Event:
		return "event"
	case protocol.Operator:
		return "operator"
	case protocol.TypeParameter:
		return "type_parameter"
	default:
		return "unknown"
	}
}
