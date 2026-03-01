package repomap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// RenderRepoMap produces scope-aware output from ranked stage entries.
// Stage-0 (special prelude), stage-2 (graph nodes), and stage-3 (remaining
// files) entries emit bare filenames. Stage-1 (ranked definitions) entries
// are rendered with TreeContext for scope-aware │-prefixed output.
//
// Per-file errors (read failures, unsupported languages, parse errors) are
// absorbed with a flat-definition fallback. Only context-level errors
// (deadline exceeded, cancellation) are propagated.
func RenderRepoMap(
	ctx context.Context,
	entries []StageEntry,
	tags map[string][]treesitter.Tag,
	parser treesitter.Parser,
	rootDir string,
) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}

	// Group entries by file, preserving stage order.
	type fileGroup struct {
		file    string
		entries []StageEntry
	}
	seen := make(map[string]int, len(entries))
	groups := make([]fileGroup, 0, len(entries))
	for _, e := range entries {
		idx, ok := seen[e.File]
		if !ok {
			idx = len(groups)
			seen[e.File] = idx
			groups = append(groups, fileGroup{file: e.File, entries: nil})
		}
		groups[idx].entries = append(groups[idx].entries, e)
	}

	// File content cache to avoid re-reading the same file.
	contentCache := make(map[string][]byte)

	var out strings.Builder
	for _, g := range groups {
		// Context cancellation check.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		hasStage1 := false
		for _, e := range g.entries {
			if e.Stage == stageRankedDefs {
				hasStage1 = true
				break
			}
		}

		if hasStage1 {
			rendered := renderStage1File(ctx, g.file, g.entries, tags, parser, rootDir, contentCache)
			out.WriteString(rendered)
			// Release cached content after rendering.
			delete(contentCache, g.file)
		} else {
			// Stage-0, stage-2, stage-3: bare filename (no colon).
			out.WriteString(g.file)
			out.WriteByte('\n')
		}
	}

	return out.String(), nil
}

// renderStage1File renders a single file's stage-1 entries using TreeContext.
// On failure, falls back to flat S1|file|ident lines.
func renderStage1File(
	ctx context.Context,
	file string,
	fileEntries []StageEntry,
	tags map[string][]treesitter.Tag,
	parser treesitter.Parser,
	rootDir string,
	contentCache map[string][]byte,
) string {
	// Collect only stage-1 entries for LOI computation.
	stage1Entries := make([]StageEntry, 0, len(fileEntries))
	for _, e := range fileEntries {
		if e.Stage == stageRankedDefs {
			stage1Entries = append(stage1Entries, e)
		}
	}

	fileTags := tags[file]
	loi := buildLinesOfInterest(stage1Entries, fileTags)

	if len(loi) == 0 {
		return renderFlatDefs(file, stage1Entries)
	}

	// Read file content (cached).
	content, ok := contentCache[file]
	if !ok {
		absPath := filepath.Join(rootDir, filepath.FromSlash(file))
		data, err := os.ReadFile(absPath)
		if err != nil {
			// File cannot be read: fall back to flat format.
			return renderFlatDefs(file, stage1Entries)
		}
		content = data
		contentCache[file] = content
	}

	// Check language support before parsing.
	lang := treesitter.MapPath(file)
	if lang == "" || (parser != nil && !parser.SupportsLanguage(lang)) {
		// Unsupported language: silently fall back.
		return renderFlatDefs(file, stage1Entries)
	}

	if parser == nil {
		return renderFlatDefs(file, stage1Entries)
	}

	// Parse the AST.
	tree, err := parser.ParseTree(ctx, file, content)
	if err != nil {
		// Check if this is a context error — propagation happens at the
		// caller level (RenderRepoMap). Here we just fall back.
		if ctx.Err() != nil {
			return ""
		}
		// Parse failure on supported language: log warning.
		slog.Warn("Repo-map parse failed, falling back to flat defs",
			"file", file, "error", err)
		return renderFlatDefs(file, stage1Entries)
	}
	// Explicitly close tree after construction (NOT deferred in loop).
	lines := strings.Split(string(content), "\n")
	rendered := RenderTreeContext(lines, loi)
	tree.Close()

	if rendered == "" {
		return renderFlatDefs(file, stage1Entries)
	}

	var b strings.Builder
	b.WriteString(file)
	b.WriteString(":\n")
	b.WriteString(rendered)
	return b.String()
}

// buildLinesOfInterest converts stage-1 entries and tags into a set of
// 0-indexed line numbers for TreeContext rendering.
//
// Tag.Line is 1-indexed (assigned as int(start.Row)+1 at query.go:165).
// TreeContext line indices are 0-indexed. We subtract 1 when storing.
//
// Multiple definitions can share the same Name (e.g., Go methods
// func (A) String() and func (B) String()). All definition lines per
// name are stored.
func buildLinesOfInterest(
	fileEntries []StageEntry,
	fileTags []treesitter.Tag,
) map[int]struct{} {
	defLines := make(map[string][]int, len(fileTags))
	for _, tag := range fileTags {
		if tag.Kind != "def" {
			continue
		}
		defLines[tag.Name] = append(defLines[tag.Name], tag.Line-1) // 1->0 indexed.
	}
	loi := make(map[int]struct{}, len(fileEntries))
	for _, entry := range fileEntries {
		if entry.Stage != stageRankedDefs {
			continue
		}
		for _, line := range defLines[entry.Ident] {
			loi[line] = struct{}{}
		}
	}
	return loi
}

// renderFlatDefs emits the S1|file|ident fallback format for stage-1 entries.
func renderFlatDefs(file string, entries []StageEntry) string {
	var b strings.Builder
	for _, e := range entries {
		if e.Stage != stageRankedDefs {
			continue
		}
		b.WriteString("S1|")
		b.WriteString(file)
		b.WriteByte('|')
		b.WriteString(e.Ident)
		b.WriteByte('\n')
	}
	return b.String()
}
