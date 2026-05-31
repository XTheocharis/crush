//go:build treesitter

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/crush/internal/treesitter"
)

// EditOp is a single string-replacement operation targeting a file path.
type EditOp struct {
	FilePath   string
	OldContent string
	NewContent string
}

// OpResult records the outcome of a single EditOp within a batch.
type OpResult struct {
	Index   int
	Success bool
	Error   string
}

// MaxBatchFiles is the maximum number of files allowed in a single batch.
const MaxBatchFiles = 50

// BatchResult is the aggregate outcome of a BatchProcessor.Apply call.
type BatchResult struct {
	OverallSuccess      bool
	PerOpResults        []OpResult
	RolledBack          bool
	AnchorMaps          map[string]*AnchorMap
	ParseErrors         map[string]string
	BaselineDiagnostics *DiagnosticSnapshot
}

// DiagnosticSnapshot records pre-edit LSP diagnostics for a set of files.
type DiagnosticSnapshot struct {
	// Diags maps file paths to their diagnostic messages collected before
	// editing. Best-effort: may be empty if LSP is unavailable.
	Diags map[string][]string
}

// DiagnosticsFunc collects diagnostics for a single file path. Returns a
// slice of diagnostic message strings (may be empty).
type DiagnosticsFunc func(ctx context.Context, filePath string) []string

// ContentStore abstracts file content access for in-memory batch processing.
type ContentStore interface {
	Get(filePath string) (string, bool)
	Set(filePath string, content string)
}

// MapContentStore is an in-memory ContentStore backed by a map.
type MapContentStore struct {
	data map[string]string
}

// NewMapContentStore returns a MapContentStore initialised from the supplied map.
func NewMapContentStore(initial map[string]string) *MapContentStore {
	data := make(map[string]string, len(initial))
	for k, v := range initial {
		data[k] = v
	}
	return &MapContentStore{data: data}
}

// Get returns the content for filePath.
func (s *MapContentStore) Get(filePath string) (string, bool) {
	c, ok := s.data[filePath]
	return c, ok
}

// Set writes content for filePath.
func (s *MapContentStore) Set(filePath string, content string) {
	s.data[filePath] = content
}

// Snapshot returns a copy of the current store contents.
func (s *MapContentStore) Snapshot() map[string]string {
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

// BatchProcessor applies multiple EditOps atomically (all-or-nothing).
// After a successful apply it rebuilds AnchorMaps for every modified file
// and optionally verifies parse success via ASTAnchorBridge.
type BatchProcessor struct {
	store          ContentStore
	parser         treesitter.Parser
	anchorInterval int
	diagsFn        DiagnosticsFunc
}

// NewBatchProcessor creates a BatchProcessor backed by store.
// parser may be nil (skips AST verification). anchorInterval controls
// AnchorMap granularity (0 uses the default).
func NewBatchProcessor(store ContentStore, parser treesitter.Parser, anchorInterval int) *BatchProcessor {
	return &BatchProcessor{
		store:          store,
		parser:         parser,
		anchorInterval: anchorInterval,
	}
}

// WithDiagnosticsCapture sets an optional function used to collect LSP
// diagnostics for each file before batch editing begins. The capture is
// best-effort: if fn is nil or the function returns no results, the batch
// proceeds without baseline diagnostics.
func (bp *BatchProcessor) WithDiagnosticsCapture(fn DiagnosticsFunc) *BatchProcessor {
	bp.diagsFn = fn
	return bp
}

// Apply applies all ops atomically against the ContentStore.
// On any failure all changes are rolled back. On success, anchor maps are
// rebuilt and optional AST verification is performed.
func (bp *BatchProcessor) Apply(ops []EditOp) (*BatchResult, error) {
	if len(ops) == 0 {
		return &BatchResult{
			OverallSuccess: true,
			PerOpResults:   nil,
			AnchorMaps:     make(map[string]*AnchorMap),
			ParseErrors:    make(map[string]string),
		}, nil
	}

	if len(ops) > MaxBatchFiles {
		return nil, fmt.Errorf("batch exceeds maximum of %d files", MaxBatchFiles)
	}

	if err := detectOverlaps(ops, bp.store); err != nil {
		return nil, err
	}

	baseline := bp.captureBaseline(context.Background(), ops)

	snapshot := bp.snapshotStore()

	results := make([]OpResult, len(ops))
	modifiedFiles := make(map[string]string)
	allOK := true

	for i, op := range ops {
		results[i] = OpResult{Index: i}

		current, exists := bp.store.Get(op.FilePath)
		if !exists {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("file not found: %s", op.FilePath)
			allOK = false
			break
		}

		if !strings.Contains(current, op.OldContent) {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("old_content not found in %s", op.FilePath)
			allOK = false
			break
		}

		updated := strings.Replace(current, op.OldContent, op.NewContent, 1)
		bp.store.Set(op.FilePath, updated)
		modifiedFiles[op.FilePath] = updated
		results[i].Success = true
	}

	if !allOK {
		bp.restoreSnapshot(snapshot)
		return &BatchResult{
			OverallSuccess:      false,
			PerOpResults:        results,
			RolledBack:          true,
			AnchorMaps:          make(map[string]*AnchorMap),
			ParseErrors:         make(map[string]string),
			BaselineDiagnostics: baseline,
		}, nil
	}

	anchorMaps := make(map[string]*AnchorMap, len(modifiedFiles))
	parseErrors := make(map[string]string)

	for filePath := range modifiedFiles {
		content, _ := bp.store.Get(filePath)
		anchorMaps[filePath] = BuildAnchorMap(content, bp.anchorInterval)
	}

	if bp.parser != nil {
		bridge := NewASTAnchorBridge(bp.parser)
		pe := bridge.VerifyBatch(context.Background(), modifiedFiles)
		for fp, errMsg := range pe {
			parseErrors[fp] = errMsg
		}
	}

	return &BatchResult{
		OverallSuccess:      true,
		PerOpResults:        results,
		RolledBack:          false,
		AnchorMaps:          anchorMaps,
		ParseErrors:         parseErrors,
		BaselineDiagnostics: baseline,
	}, nil
}

// captureBaseline collects LSP diagnostics for each unique file in ops
// before editing. Returns an empty snapshot if no diagnostics function is
// configured or if the function returns no results.
func (bp *BatchProcessor) captureBaseline(ctx context.Context, ops []EditOp) *DiagnosticSnapshot {
	snap := &DiagnosticSnapshot{Diags: make(map[string][]string)}
	if bp.diagsFn == nil {
		return snap
	}

	seen := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		if _, ok := seen[op.FilePath]; ok {
			continue
		}
		seen[op.FilePath] = struct{}{}
		diags := bp.diagsFn(ctx, op.FilePath)
		if len(diags) > 0 {
			snap.Diags[op.FilePath] = diags
		}
	}
	return snap
}

func (bp *BatchProcessor) snapshotStore() map[string]string {
	if mcs, ok := bp.store.(*MapContentStore); ok {
		return mcs.Snapshot()
	}
	return make(map[string]string)
}

func (bp *BatchProcessor) restoreSnapshot(snap map[string]string) {
	for filePath, content := range snap {
		bp.store.Set(filePath, content)
	}
}

// resolvedRange holds the byte offsets for a single EditOp within a file.
type resolvedRange struct {
	filePath string
	start    int // byte offset of OldContent within file
	end      int // start + len(OldContent)
	opIndex  int // index in the original ops slice
}

// detectOverlaps checks all ops for overlapping byte ranges on the same file.
// It resolves each OldContent to its position in the current file content and
// returns an error listing all conflicting operation pairs. The content store
// is read but not modified.
func detectOverlaps(ops []EditOp, store ContentStore) error {
	type fileRanges struct {
		ranges []resolvedRange
	}
	byFile := make(map[string]*fileRanges)

	for i, op := range ops {
		content, exists := store.Get(op.FilePath)
		if !exists {
			continue
		}

		idx := strings.Index(content, op.OldContent)
		if idx < 0 {
			continue
		}

		fr, ok := byFile[op.FilePath]
		if !ok {
			fr = &fileRanges{}
			byFile[op.FilePath] = fr
		}
		fr.ranges = append(fr.ranges, resolvedRange{
			filePath: op.FilePath,
			start:    idx,
			end:      idx + len(op.OldContent),
			opIndex:  i,
		})
	}

	var conflicts []string
	for _, fr := range byFile {
		ranges := fr.ranges
		if len(ranges) < 2 {
			continue
		}

		sort.Slice(ranges, func(i, j int) bool {
			return ranges[i].start < ranges[j].start
		})

		for i := 0; i < len(ranges)-1; i++ {
			for j := i + 1; j < len(ranges); j++ {
				if ranges[i].end > ranges[j].start {
					conflicts = append(conflicts, fmt.Sprintf(
						"ops[%d] [%d:%d) and ops[%d] [%d:%d) overlap in %s",
						ranges[i].opIndex, ranges[i].start, ranges[i].end,
						ranges[j].opIndex, ranges[j].start, ranges[j].end,
						ranges[i].filePath,
					))
				}
				break
			}
		}
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("batch overlap: %s", strings.Join(conflicts, "; "))
	}
	return nil
}

// ASTAnchorBridge verifies edited files parse correctly via tree-sitter
// and provides anchor map rebuilding.
type ASTAnchorBridge struct {
	parser treesitter.Parser
}

// NewASTAnchorBridge creates a bridge with the given tree-sitter Parser.
func NewASTAnchorBridge(parser treesitter.Parser) *ASTAnchorBridge {
	return &ASTAnchorBridge{parser: parser}
}

// VerifyBatch checks that every file in files parses. It returns a map of
// filePath→errorMessage for failures. Unsupported languages are silently
// skipped.
func (b *ASTAnchorBridge) VerifyBatch(ctx context.Context, files map[string]string) map[string]string {
	if b.parser == nil {
		return nil
	}

	errs := make(map[string]string)
	for filePath, content := range files {
		lang := treesitter.MapPath(filePath)
		if lang == "" || !b.parser.SupportsLanguage(lang) {
			continue
		}

		tree, err := b.parser.ParseTree(ctx, filePath, []byte(content))
		if err != nil {
			errs[filePath] = fmt.Sprintf("AST parse failed: %v", err)
			continue
		}
		if tree != nil {
			tree.Close()
		}
	}

	return errs
}

// RebuildAnchorMap creates a fresh AnchorMap for the given content.
func (b *ASTAnchorBridge) RebuildAnchorMap(content string, interval int) *AnchorMap {
	return BuildAnchorMap(content, interval)
}

// SymbolAnchorRange maps a tree-sitter symbol to the anchors that cover its
// line range. This enables edit operations like "replace function body" by
// resolving symbol → anchor range → edit operation.
type SymbolAnchorRange struct {
	Symbol      treesitter.SymbolInfo
	StartAnchor HashAnchor // First anchor at or before the symbol start line.
	EndAnchor   HashAnchor // Last anchor at or before the symbol end line.
}

// MapSymbolsToAnchors maps tree-sitter symbols to the hash anchors that cover
// their line ranges. For each symbol, it finds the first anchor at or before
// the symbol's start line and the last anchor at or before the symbol's end
// line. Symbols with no anchor preceding their start line are skipped.
func (b *ASTAnchorBridge) MapSymbolsToAnchors(
	ctx context.Context,
	fileID string,
	content string,
	symbols []treesitter.SymbolInfo,
	interval int,
) ([]SymbolAnchorRange, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	am := BuildAnchorMap(content, interval)
	if len(am.Anchors) == 0 {
		return nil, nil
	}

	// Anchors are sorted by LineNum (ascending) from BuildAnchorMap.
	// Use binary search to find anchors efficiently.
	anchors := am.Anchors

	results := make([]SymbolAnchorRange, 0, len(symbols))
	for _, sym := range symbols {
		startIdx := findLastAnchorAtOrBefore(anchors, sym.Line)
		if startIdx < 0 {
			// No anchor precedes the symbol start line; skip this symbol.
			continue
		}
		endIdx := findLastAnchorAtOrBefore(anchors, sym.EndLine)
		if endIdx < 0 {
			continue
		}

		results = append(results, SymbolAnchorRange{
			Symbol:      sym,
			StartAnchor: anchors[startIdx],
			EndAnchor:   anchors[endIdx],
		})
	}

	return results, nil
}

// findLastAnchorAtOrBefore returns the index of the anchor with the highest
// LineNum that is <= targetLine, using binary search. Returns -1 if no such
// anchor exists.
func findLastAnchorAtOrBefore(anchors []HashAnchor, targetLine int) int {
	lo, hi := 0, len(anchors)-1
	result := -1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if anchors[mid].LineNum <= targetLine {
			result = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return result
}
