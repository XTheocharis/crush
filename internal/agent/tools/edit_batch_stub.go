//go:build !treesitter

package tools

import (
	"context"
	"fmt"
	"strings"
)

type EditOp struct {
	FilePath   string
	OldContent string
	NewContent string
}

type OpResult struct {
	Index   int
	Success bool
	Error   string
}

type BatchResult struct {
	OverallSuccess bool
	PerOpResults   []OpResult
	RolledBack     bool
	AnchorMaps     map[string]*AnchorMap
	ParseErrors    map[string]string
}

type ContentStore interface {
	Get(filePath string) (string, bool)
	Set(filePath string, content string)
}

type MapContentStore struct {
	data map[string]string
}

func NewMapContentStore(initial map[string]string) *MapContentStore {
	data := make(map[string]string, len(initial))
	for k, v := range initial {
		data[k] = v
	}
	return &MapContentStore{data: data}
}

func (s *MapContentStore) Get(filePath string) (string, bool) {
	c, ok := s.data[filePath]
	return c, ok
}

func (s *MapContentStore) Set(filePath string, content string) {
	s.data[filePath] = content
}

func (s *MapContentStore) Snapshot() map[string]string {
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

type BatchProcessor struct {
	store          ContentStore
	anchorInterval int
}

func NewBatchProcessor(store ContentStore, _ interface{}, anchorInterval int) *BatchProcessor {
	return &BatchProcessor{
		store:          store,
		anchorInterval: anchorInterval,
	}
}

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
	for filePath := range modifiedFiles {
		content, _ := bp.store.Get(filePath)
		anchorMaps[filePath] = BuildAnchorMap(content, bp.anchorInterval)
	}

	return &BatchResult{
		OverallSuccess: true,
		PerOpResults:   results,
		RolledBack:     false,
		AnchorMaps:     anchorMaps,
		ParseErrors:    make(map[string]string),
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

type ASTAnchorBridge struct{}

func NewASTAnchorBridge(_ interface{}) *ASTAnchorBridge {
	return &ASTAnchorBridge{}
}

func (b *ASTAnchorBridge) VerifyBatch(_ context.Context, _ map[string]string) map[string]string {
	return nil
}

func (b *ASTAnchorBridge) RebuildAnchorMap(content string, interval int) *AnchorMap {
	return BuildAnchorMap(content, interval)
}
