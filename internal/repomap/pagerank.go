package repomap

import (
	"math"
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

const (
	pageRankDamping    = 0.85
	pageRankTolerance  = 1e-6
	pageRankIterations = 100
)

// RankedDefinition is a definition-level rank entry.
type RankedDefinition struct {
	File  string
	Ident string
	Rank  float64
}

// RankedDef is a ranked definition entry under one file.
type RankedDef struct {
	Name string
	Line int
	Rank float64
}

// RankedFile is the aggregated per-file ranking output.
type RankedFile struct {
	Path string
	Rank float64
	Defs []RankedDef
}

type inboundEdge struct {
	from   int
	weight float64
}

// BuildPersonalization computes the personalization vector according to the
// repo-map rules.
func BuildPersonalization(allFiles, chatFiles, mentionedFnames, mentionedIdents []string) map[string]float64 {
	files := normalizeUniqueGraphPaths(allFiles)
	if len(files) == 0 {
		return nil
	}
	fileSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		fileSet[f] = struct{}{}
	}

	base := 100.0 / float64(len(files))
	pers := make(map[string]float64)

	for _, f := range normalizeUniqueGraphPaths(chatFiles) {
		if _, ok := fileSet[f]; ok {
			pers[f] += base
		}
	}

	for _, f := range normalizeUniqueGraphPaths(mentionedFnames) {
		if _, ok := fileSet[f]; !ok {
			continue
		}
		if pers[f] < base {
			pers[f] = base
		}
	}

	mentionedSet := make(map[string]struct{}, len(mentionedIdents))
	for _, ident := range mentionedIdents {
		ident = strings.TrimSpace(ident)
		if ident == "" {
			continue
		}
		mentionedSet[ident] = struct{}{}
	}

	for _, f := range files {
		components := pathComponentsForPersonalization(f)
		matched := false
		for ident := range mentionedSet {
			if _, ok := components[ident]; ok {
				matched = true
				break
			}
		}
		if matched {
			pers[f] += base
		}
	}

	if len(pers) == 0 {
		return nil
	}
	return pers
}

// Rank computes file-level PageRank and distributes it to destination
// definitions.
func Rank(graph *FileGraph, personalization map[string]float64) []RankedDefinition {
	scores, _, ok := runPageRank(graph, personalization)
	if !ok && len(personalization) > 0 {
		scores, _, ok = runPageRank(graph, nil)
	}
	if !ok {
		return []RankedDefinition{}
	}

	defs := distributeRankToDefinitions(graph, scores)
	sortRankedDefinitions(defs)
	return defs
}

// RankFiles computes definition ranks and aggregates them to file ranks.
func RankFiles(graph *FileGraph, personalization map[string]float64, tags []treesitter.Tag) []RankedFile {
	defs := Rank(graph, personalization)
	return AggregateRankedFiles(defs, tags)
}

func runPageRank(graph *FileGraph, personalization map[string]float64) (map[string]float64, []float64, bool) {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil, nil, false
	}

	nodes := append([]string(nil), graph.Nodes...)
	sort.Strings(nodes)

	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		index[n] = i
	}

	outWeights := make([]float64, len(nodes))
	incoming := make([][]inboundEdge, len(nodes))
	var totalWeight float64

	for _, e := range graph.Edges {
		if e.Weight <= 0 {
			continue
		}
		from := normalizeGraphRelPath(e.From)
		to := normalizeGraphRelPath(e.To)
		fromIdx, okFrom := index[from]
		toIdx, okTo := index[to]
		if !okFrom || !okTo {
			continue
		}

		incoming[toIdx] = append(incoming[toIdx], inboundEdge{from: fromIdx, weight: e.Weight})
		outWeights[fromIdx] += e.Weight
		totalWeight += e.Weight
	}

	if totalWeight <= 0 {
		return nil, nil, false
	}

	p, hasPersonalization := normalizePersonalization(nodes, index, personalization)
	rank := make([]float64, len(nodes))
	uniform := make([]float64, len(nodes))
	for i := range rank {
		rank[i] = 1.0 / float64(len(nodes))
		uniform[i] = 1.0 / float64(len(nodes))
	}

	danglingDist := uniform
	if hasPersonalization {
		danglingDist = p
	}

	deltas := make([]float64, 0, pageRankIterations)
	for range pageRankIterations {
		next := make([]float64, len(nodes))
		for i := range next {
			next[i] = (1 - pageRankDamping) * p[i]
		}

		var danglingMass float64
		for i, w := range outWeights {
			if w <= 0 {
				danglingMass += rank[i]
			}
		}
		if danglingMass > 0 {
			scaled := pageRankDamping * danglingMass
			for i := range next {
				next[i] += scaled * danglingDist[i]
			}
		}

		for toIdx, inEdges := range incoming {
			var inSum float64
			for _, in := range inEdges {
				den := outWeights[in.from]
				if den <= 0 {
					continue
				}
				inSum += rank[in.from] * (in.weight / den)
			}
			next[toIdx] += pageRankDamping * inSum
		}

		var delta float64
		for i := range next {
			delta += math.Abs(next[i] - rank[i])
		}
		deltas = append(deltas, delta)
		rank = next

		if delta < pageRankTolerance {
			break
		}
	}

	scores := make(map[string]float64, len(nodes))
	for i, n := range nodes {
		scores[n] = rank[i]
	}
	return scores, deltas, true
}

func normalizePersonalization(nodes []string, index map[string]int, personalization map[string]float64) ([]float64, bool) {
	uniform := make([]float64, len(nodes))
	for i := range uniform {
		uniform[i] = 1.0 / float64(len(nodes))
	}
	if len(personalization) == 0 {
		return uniform, false
	}

	p := make([]float64, len(nodes))
	var sum float64
	for node, val := range personalization {
		if val <= 0 {
			continue
		}
		node = normalizeGraphRelPath(node)
		i, ok := index[node]
		if !ok {
			continue
		}
		p[i] += val
		sum += val
	}
	if sum <= 0 {
		return uniform, false
	}
	for i := range p {
		p[i] /= sum
	}
	return p, true
}

func distributeRankToDefinitions(graph *FileGraph, fileRanks map[string]float64) []RankedDefinition {
	if graph == nil || len(graph.Edges) == 0 || len(fileRanks) == 0 {
		return nil
	}

	outWeights := make(map[string]float64)
	for _, e := range graph.Edges {
		if e.Weight <= 0 {
			continue
		}
		from := normalizeGraphRelPath(e.From)
		if from == "" {
			continue
		}
		outWeights[from] += e.Weight
	}

	type key struct {
		file  string
		ident string
	}
	acc := make(map[key]float64)

	for _, e := range graph.Edges {
		if e.Weight <= 0 {
			continue
		}
		from := normalizeGraphRelPath(e.From)
		to := normalizeGraphRelPath(e.To)
		ident := strings.TrimSpace(e.Ident)
		if from == "" || to == "" || ident == "" {
			continue
		}

		totalOut := outWeights[from]
		if totalOut <= 0 {
			continue
		}
		r := fileRanks[from]
		if r <= 0 {
			continue
		}

		acc[key{file: to, ident: ident}] += r * (e.Weight / totalOut)
	}

	defs := make([]RankedDefinition, 0, len(acc))
	for k, rank := range acc {
		if rank <= 0 {
			continue
		}
		defs = append(defs, RankedDefinition{File: k.file, Ident: k.ident, Rank: rank})
	}
	return defs
}

func sortRankedDefinitions(defs []RankedDefinition) {
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Rank != defs[j].Rank {
			return defs[i].Rank > defs[j].Rank
		}
		if defs[i].File != defs[j].File {
			return defs[i].File < defs[j].File
		}
		return defs[i].Ident < defs[j].Ident
	})
}

// AggregateRankedFiles converts definition-level ranks to file-level ranking.
func AggregateRankedFiles(defs []RankedDefinition, tags []treesitter.Tag) []RankedFile {
	if len(defs) == 0 {
		return nil
	}

	lineByFileIdent := buildDefinitionLineIndex(tags)
	byFile := make(map[string]*RankedFile)

	for _, def := range defs {
		file := normalizeGraphRelPath(def.File)
		ident := strings.TrimSpace(def.Ident)
		if file == "" || ident == "" || def.Rank <= 0 {
			continue
		}

		rf := byFile[file]
		if rf == nil {
			rf = &RankedFile{Path: file}
			byFile[file] = rf
		}
		rf.Rank += def.Rank

		line := 0
		if byIdent := lineByFileIdent[file]; byIdent != nil {
			line = byIdent[ident]
		}
		rf.Defs = append(rf.Defs, RankedDef{Name: ident, Line: line, Rank: def.Rank})
	}

	files := make([]RankedFile, 0, len(byFile))
	for _, rf := range byFile {
		sort.Slice(rf.Defs, func(i, j int) bool {
			if rf.Defs[i].Rank != rf.Defs[j].Rank {
				return rf.Defs[i].Rank > rf.Defs[j].Rank
			}
			if rf.Defs[i].Name != rf.Defs[j].Name {
				return rf.Defs[i].Name < rf.Defs[j].Name
			}
			return rf.Defs[i].Line < rf.Defs[j].Line
		})
		files = append(files, *rf)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Rank != files[j].Rank {
			return files[i].Rank > files[j].Rank
		}
		return files[i].Path < files[j].Path
	})
	return files
}

func buildDefinitionLineIndex(tags []treesitter.Tag) map[string]map[string]int {
	idx := make(map[string]map[string]int)
	for _, tag := range tags {
		if tag.Kind != "def" {
			continue
		}
		file := normalizeGraphRelPath(tag.RelPath)
		ident := strings.TrimSpace(tag.Name)
		if file == "" || ident == "" {
			continue
		}
		if idx[file] == nil {
			idx[file] = make(map[string]int)
		}
		if line, ok := idx[file][ident]; !ok || (tag.Line > 0 && (line <= 0 || tag.Line < line)) {
			idx[file][ident] = tag.Line
		}
	}
	return idx
}

func normalizeUniqueGraphPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel := normalizeGraphRelPath(p)
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func pathComponentsForPersonalization(relPath string) map[string]struct{} {
	relPath = normalizeGraphRelPath(relPath)
	components := make(map[string]struct{})
	if relPath == "" {
		return components
	}

	parts := strings.Split(relPath, "/")
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		components[p] = struct{}{}
	}

	base := parts[len(parts)-1]
	if base != "" {
		components[base] = struct{}{}
		if ext := path.Ext(base); ext != "" {
			if noExt := strings.TrimSuffix(base, ext); noExt != "" {
				components[noExt] = struct{}{}
			}
		}
	}
	return components
}
