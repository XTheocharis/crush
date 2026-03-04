package repomap

import (
	"sort"
)

const (
	stageSpecialPrelude = 0
	stageRankedDefs     = 1
	stageGraphNodes     = 2
	stageRemainingFiles = 3
)

// StageEntry is one candidate entry in the stage-0+1/2/3 assembly.
type StageEntry struct {
	Stage int
	File  string
	Ident string
	Rank  float64
}

// AssembleStageEntries assembles stage-0+1/2/3 entries in order:
// stage0 special prelude, stage1 ranked defs, stage2 remaining graph nodes,
// stage3 remaining repo files.
func AssembleStageEntries(
	specialPrelude []string,
	rankedDefs []RankedDefinition,
	graphNodes []string,
	repoFiles []string,
	chatFiles []string,
	parityMode bool,
) []StageEntry {
	chatSet := make(map[string]struct{}, len(chatFiles))
	for _, f := range normalizeUniqueGraphPaths(chatFiles) {
		chatSet[f] = struct{}{}
	}

	sortedDefs := append([]RankedDefinition(nil), rankedDefs...)
	sortRankedDefinitions(sortedDefs)

	rankedFileSet := make(map[string]struct{})
	for _, d := range sortedDefs {
		file := normalizeGraphRelPath(d.File)
		if file != "" {
			rankedFileSet[file] = struct{}{}
		}
	}

	entries := make([]StageEntry, 0, len(specialPrelude)+len(sortedDefs)+len(graphNodes)+len(repoFiles))

	// Stage 0: special-file prelude.
	seenStage0 := make(map[string]struct{})
	for _, f := range specialPrelude {
		file := normalizeGraphRelPath(f)
		if file == "" {
			continue
		}
		if _, chat := chatSet[file]; chat {
			continue
		}
		if _, inRanked := rankedFileSet[file]; inRanked {
			continue
		}
		if _, seen := seenStage0[file]; seen {
			continue
		}
		seenStage0[file] = struct{}{}
		entries = append(entries, StageEntry{Stage: stageSpecialPrelude, File: file})
	}

	// Stage 1: ranked definitions.
	stage1FileSet := make(map[string]struct{})
	for _, d := range sortedDefs {
		file := normalizeGraphRelPath(d.File)
		if file == "" {
			continue
		}
		if _, chat := chatSet[file]; chat {
			continue
		}
		ident := d.Ident
		if ident == "" {
			continue
		}
		entries = append(entries, StageEntry{Stage: stageRankedDefs, File: file, Ident: ident, Rank: d.Rank})
		stage1FileSet[file] = struct{}{}
	}

	// Stage 2: remaining graph nodes.
	stage2Seen := make(map[string]struct{})
	for _, f := range graphNodes {
		file := normalizeGraphRelPath(f)
		if file == "" {
			continue
		}
		if _, chat := chatSet[file]; chat {
			continue
		}
		if _, inStage1 := stage1FileSet[file]; inStage1 {
			continue
		}
		if _, seen := stage2Seen[file]; seen {
			continue
		}
		stage2Seen[file] = struct{}{}
		entries = append(entries, StageEntry{Stage: stageGraphNodes, File: file})
	}

	// Stage 3: remaining repo files.
	stage3Input := make([]string, 0, len(repoFiles))
	for _, f := range repoFiles {
		file := normalizeGraphRelPath(f)
		if file != "" {
			stage3Input = append(stage3Input, file)
		}
	}
	if !parityMode {
		sort.Strings(stage3Input)
	}

	stage3Seen := make(map[string]struct{})
	for _, file := range stage3Input {
		if _, chat := chatSet[file]; chat {
			continue
		}
		if _, inStage1 := stage1FileSet[file]; inStage1 {
			continue
		}
		if _, inStage2 := stage2Seen[file]; inStage2 {
			continue
		}
		if _, seen := stage3Seen[file]; seen {
			continue
		}
		stage3Seen[file] = struct{}{}
		entries = append(entries, StageEntry{Stage: stageRemainingFiles, File: file})
	}

	return entries
}
