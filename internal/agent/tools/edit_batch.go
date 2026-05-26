//go:build treesitter

package tools

import (
	"context"
	"fmt"
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

// BatchResult is the aggregate outcome of a BatchProcessor.Apply call.
type BatchResult struct {
	OverallSuccess bool
	PerOpResults   []OpResult
	RolledBack     bool
	AnchorMaps     map[string]*AnchorMap
	ParseErrors    map[string]string
}

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
			OverallSuccess: false,
			PerOpResults:   results,
			RolledBack:     true,
			AnchorMaps:     make(map[string]*AnchorMap),
			ParseErrors:    make(map[string]string),
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
		OverallSuccess: true,
		PerOpResults:   results,
		RolledBack:     false,
		AnchorMaps:     anchorMaps,
		ParseErrors:    parseErrors,
	}, nil
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
