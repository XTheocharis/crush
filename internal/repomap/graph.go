package repomap

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// GraphEdge captures a weighted identifier edge between two files.
type GraphEdge struct {
	From     string
	To       string
	Ident    string
	Weight   float64
	RefCount int
}

// FileGraph is a directed multigraph over repository files.
type FileGraph struct {
	Nodes []string
	Edges []GraphEdge
}

func buildGraph(tags []treesitter.Tag, chatFiles []string, mentionedIdents []string) *FileGraph {
	nodes := make(map[string]struct{})
	defsByIdent := make(map[string]map[string]struct{})
	defsByFile := make(map[string]map[string]struct{})
	refsByFile := make(map[string]map[string]int)
	totalRefs := 0

	for _, tag := range tags {
		relPath := normalizeGraphRelPath(tag.RelPath)
		name := strings.TrimSpace(tag.Name)
		if relPath == "" || name == "" {
			continue
		}

		nodes[relPath] = struct{}{}

		switch tag.Kind {
		case "def":
			if defsByIdent[name] == nil {
				defsByIdent[name] = make(map[string]struct{})
			}
			defsByIdent[name][relPath] = struct{}{}

			if defsByFile[relPath] == nil {
				defsByFile[relPath] = make(map[string]struct{})
			}
			defsByFile[relPath][name] = struct{}{}
		case "ref":
			if refsByFile[relPath] == nil {
				refsByFile[relPath] = make(map[string]int)
			}
			refsByFile[relPath][name]++
			totalRefs++
		}
	}

	// 1) Per-file lexical backfill.
	for relPath, defs := range defsByFile {
		if len(defs) == 0 || len(refsByFile[relPath]) > 0 {
			continue
		}

		for ident := range defs {
			if !isLexicalIdentifier(ident) {
				continue
			}
			if refsByFile[relPath] == nil {
				refsByFile[relPath] = make(map[string]int)
			}
			refsByFile[relPath][ident]++
			totalRefs++
		}
	}

	// 2) No-reference global fallback.
	if totalRefs == 0 {
		for ident, defFiles := range defsByIdent {
			for relPath := range defFiles {
				if refsByFile[relPath] == nil {
					refsByFile[relPath] = make(map[string]int)
				}
				refsByFile[relPath][ident]++
			}
		}
	}

	refsByIdent := make(map[string]map[string]int)
	for relPath, refs := range refsByFile {
		for ident, count := range refs {
			if count > 0 {
				if refsByIdent[ident] == nil {
					refsByIdent[ident] = make(map[string]int)
				}
				refsByIdent[ident][relPath] += count
			}
		}
	}

	chatSet := make(map[string]struct{}, len(chatFiles))
	for _, path := range chatFiles {
		if relPath := normalizeGraphRelPath(path); relPath != "" {
			chatSet[relPath] = struct{}{}
		}
	}

	mentionedSet := make(map[string]struct{}, len(mentionedIdents))
	for _, ident := range mentionedIdents {
		if ident = strings.TrimSpace(ident); ident != "" {
			mentionedSet[ident] = struct{}{}
		}
	}

	var edges []GraphEdge

	// 3) Self-edges for orphan definitions.
	for ident, defFiles := range defsByIdent {
		if _, referenced := refsByIdent[ident]; !referenced {
			for relPath := range defFiles {
				edges = append(edges, GraphEdge{
					From:     relPath,
					To:       relPath,
					Ident:    ident,
					Weight:   0.1,
					RefCount: 0,
				})
			}
		}
	}

	// 4) Cross-file ref -> def edges.
	for ident, refs := range refsByIdent {
		defFiles := defsByIdent[ident]
		if len(defFiles) == 0 {
			continue
		}

		baseMul := identifierBaseMultiplier(ident, len(defFiles), mentionedSet)
		for from, count := range refs {
			useMul := baseMul
			if _, inChat := chatSet[from]; inChat {
				useMul *= 50
			}
			weight := useMul * math.Sqrt(float64(count))
			if weight <= 0 {
				continue
			}
			for to := range defFiles {
				edges = append(edges, GraphEdge{
					From:     from,
					To:       to,
					Ident:    ident,
					Weight:   weight,
					RefCount: count,
				})
			}
		}
	}

	nodeList := make([]string, 0, len(nodes))
	for relPath := range nodes {
		nodeList = append(nodeList, relPath)
	}
	sort.Strings(nodeList)

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		if edges[i].Ident != edges[j].Ident {
			return edges[i].Ident < edges[j].Ident
		}
		if edges[i].RefCount != edges[j].RefCount {
			return edges[i].RefCount < edges[j].RefCount
		}
		return edges[i].Weight < edges[j].Weight
	})

	return &FileGraph{Nodes: nodeList, Edges: edges}
}

func normalizeGraphRelPath(path string) string {
	if path = strings.TrimSpace(path); path == "" {
		return ""
	}
	if path = filepath.ToSlash(filepath.Clean(path)); path == "." {
		return ""
	}
	return path
}

func identifierBaseMultiplier(ident string, defsCount int, mentionedSet map[string]struct{}) float64 {
	mul := 1.0
	if _, mentioned := mentionedSet[ident]; mentioned {
		mul *= 10
	}
	if isLongStructuredIdentifier(ident) {
		mul *= 10
	}
	if strings.HasPrefix(ident, "_") {
		mul *= 0.1
	}
	if defsCount > 5 {
		mul *= 0.1
	}
	return mul
}

func isLongStructuredIdentifier(ident string) bool {
	if len([]rune(ident)) < 8 {
		return false
	}
	if strings.Contains(ident, "_") || strings.Contains(ident, "-") {
		return true
	}
	return looksCamelCase(ident)
}

func looksCamelCase(ident string) bool {
	var hasLower, hasUpper bool
	for _, r := range ident {
		if unicode.IsLower(r) {
			hasLower = true
		} else if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasLower && hasUpper
}

func isLexicalIdentifier(ident string) bool {
	if ident == "" {
		return false
	}
	for i, r := range ident {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}
