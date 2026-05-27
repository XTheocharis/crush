package lcm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock implementations for restore behaviour tests
// ---------------------------------------------------------------------------

// mockFileRegistrar records calls to OpenFiles.
type mockFileRegistrar struct {
	mu    sync.Mutex
	calls [][]string
	err   error
}

func (m *mockFileRegistrar) OpenFiles(_ context.Context, files []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(files))
	copy(cp, files)
	m.calls = append(m.calls, cp)
	return m.err
}

func (m *mockFileRegistrar) getCalls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]string, len(m.calls))
	copy(out, m.calls)
	return out
}

// mockMapInjector records calls to RequestInjection.
type mockMapInjector struct {
	mu       sync.Mutex
	sessions []string
}

func (m *mockMapInjector) RequestInjection(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = append(m.sessions, sessionID)
}

func (m *mockMapInjector) getSessions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.sessions))
	copy(out, m.sessions)
	return out
}

// ---------------------------------------------------------------------------
// CompactionLayer interface compliance
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_ImplementsCompactionLayer(t *testing.T) {
	t.Parallel()
	var _ CompactionLayer = (*PostCompactCleaner)(nil)
}

// ---------------------------------------------------------------------------
// Name / Priority
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Name(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	require.Equal(t, "post-compact-cleanup", p.Name())
}

func TestPostCompactCleaner_Priority(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	require.Equal(t, 4, p.Priority())
}

// ---------------------------------------------------------------------------
// ShouldCompact
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_ShouldCompact_NilStore(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{SessionID: "s1"})
	require.False(t, p.ShouldCompact(context.Background(), Budget{}))
}

func TestPostCompactCleaner_ShouldCompact_EmptySessionID(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store})
	require.False(t, p.ShouldCompact(context.Background(), Budget{}))
}

func TestPostCompactCleaner_ShouldCompact_NothingPreserved(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})
	require.False(t, p.ShouldCompact(context.Background(), Budget{}))
}

func TestPostCompactCleaner_ShouldCompact_EmptyPreservedContext(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{})
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})
	require.False(t, p.ShouldCompact(context.Background(), Budget{}))
}

func TestPostCompactCleaner_ShouldCompact_HasPreservedContext(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{SystemPromptContext: "system prompt"})
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})
	require.True(t, p.ShouldCompact(context.Background(), Budget{}))
}

// ---------------------------------------------------------------------------
// Compact — error cases
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Compact_NilStore_ReturnsError(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{SessionID: "s1"})
	_, err := p.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStoreIsNil))
}

func TestPostCompactCleaner_Compact_EmptySessionID_ReturnsError(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store})
	_, err := p.Compact(context.Background(), Budget{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSessionIDEmpty))
}

// ---------------------------------------------------------------------------
// Compact — graceful no-op
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Compact_NothingPreserved_NoOp(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})

	result, err := p.Compact(context.Background(), Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
	require.Equal(t, "post-compact-cleanup", result.LayerName)
}

func TestPostCompactCleaner_Compact_EmptyPreservedContext_NoOp(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{})
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})

	result, err := p.Compact(context.Background(), Budget{})
	require.NoError(t, err)
	require.False(t, result.ActionTaken)
	require.Equal(t, 0, result.ItemsAffected)
}

// ---------------------------------------------------------------------------
// Compact — full 4-step restore with real behaviour
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Compact_AllFourSteps(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	fr := &mockFileRegistrar{}
	mi := &mockMapInjector{}
	om := newMockOMStore()
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SystemPromptContext: "system prompt info",
		ActiveFiles:         []string{"file1.go", "file2.go"},
		RepoMapContent:      "repo map content",
		ToolState:           "tool state data",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:         store,
		SessionID:     "s1",
		FileRegistrar: fr,
		MapInjector:   mi,
		OMStore:       om,
	})
	require.True(t, p.ShouldCompact(ctx, Budget{}))

	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 4, result.ItemsAffected)
	require.Equal(t, "post-compact-cleanup", result.LayerName)

	// Preserved context was cleared.
	require.Nil(t, store.Load("s1"))

	// System prompt was saved to restored area.
	restored := store.LoadRestored("s1")
	require.NotNil(t, restored)
	require.Equal(t, "system prompt info", restored.SystemPromptContext)

	// Active files were passed to the file registrar.
	calls := fr.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"file1.go", "file2.go"}, calls[0])

	// Repo map injection was requested.
	sessions := mi.getSessions()
	require.Equal(t, []string{"s1"}, sessions)

	// Tool state was persisted to operational memory.
	val, ok, err := om.Get(ctx, "s1", "restored_tool_state")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "tool state data", val)
}

// ---------------------------------------------------------------------------
// Compact — partial items with behaviour verification
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Compact_OnlySystemPrompt(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{
		SystemPromptContext: "system prompt info",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})
	result, err := p.Compact(context.Background(), Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	restored := store.LoadRestored("s1")
	require.NotNil(t, restored)
	require.Equal(t, "system prompt info", restored.SystemPromptContext)
}

func TestPostCompactCleaner_Compact_OnlyActiveFiles(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	fr := &mockFileRegistrar{}
	store.Save("s1", &PreservedContext{
		ActiveFiles: []string{"a.go", "b.go", "c.go"},
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:         store,
		SessionID:     "s1",
		FileRegistrar: fr,
	})
	result, err := p.Compact(context.Background(), Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	calls := fr.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"a.go", "b.go", "c.go"}, calls[0])
}

func TestPostCompactCleaner_Compact_OnlyRepoMap(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	mi := &mockMapInjector{}
	store.Save("s1", &PreservedContext{
		RepoMapContent: "repo map data",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:       store,
		SessionID:   "s1",
		MapInjector: mi,
	})
	result, err := p.Compact(context.Background(), Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	require.Equal(t, []string{"s1"}, mi.getSessions())
}

func TestPostCompactCleaner_Compact_OnlyToolState(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	om := newMockOMStore()
	ctx := context.Background()
	store.Save("s1", &PreservedContext{
		ToolState: "active tool state",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:     store,
		SessionID: "s1",
		OMStore:   om,
	})
	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	val, ok, err := om.Get(ctx, "s1", "restored_tool_state")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "active tool state", val)
}

func TestPostCompactCleaner_Compact_TwoItems(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	mi := &mockMapInjector{}
	om := newMockOMStore()
	ctx := context.Background()
	store.Save("s1", &PreservedContext{
		ActiveFiles:    []string{"main.go"},
		RepoMapContent: "map data",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:       store,
		SessionID:   "s1",
		MapInjector: mi,
		OMStore:     om,
	})
	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)

	require.Equal(t, []string{"s1"}, mi.getSessions())
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_Compact_Idempotent_SecondCallIsNoOp(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	fr := &mockFileRegistrar{}
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SystemPromptContext: "prompt",
		ActiveFiles:         []string{"a.go"},
		RepoMapContent:      "map",
		ToolState:           "state",
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:         store,
		SessionID:     "s1",
		FileRegistrar: fr,
	})

	result1, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result1.ActionTaken)
	require.Equal(t, 4, result1.ItemsAffected)

	result2, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.False(t, result2.ActionTaken)
	require.Equal(t, 0, result2.ItemsAffected)

	// FileRegistrar was called exactly once.
	require.Len(t, fr.getCalls(), 1)
}

func TestPostCompactCleaner_Compact_Idempotent_SameResultWhenCalledTwice(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	ctx := context.Background()

	pc := &PreservedContext{
		SystemPromptContext: "prompt",
		ActiveFiles:         []string{"a.go"},
	}

	store.Save("s1", pc)
	p := NewPostCompactCleaner(PostCompactCleanerConfig{Store: store, SessionID: "s1"})

	result1, err1 := p.Compact(ctx, Budget{})
	require.NoError(t, err1)

	store.Save("s1", pc)
	result2, err2 := p.Compact(ctx, Budget{})
	require.NoError(t, err2)

	require.Equal(t, result1.ActionTaken, result2.ActionTaken)
	require.Equal(t, result1.ItemsAffected, result2.ItemsAffected)
	require.Equal(t, result1.LayerName, result2.LayerName)
}

// ---------------------------------------------------------------------------
// Individual restore steps — behaviour verification
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_RestoreSystemPrompt_Empty(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{SystemPromptContext: ""}
	restored, err := p.restoreSystemPrompt(context.Background(), pc)
	require.NoError(t, err)
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreSystemPrompt_Present_WritesToStore(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:     store,
		SessionID: "s1",
	})
	pc := &PreservedContext{SystemPromptContext: "system prompt data"}
	restored, err := p.restoreSystemPrompt(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)

	got := store.LoadRestored("s1")
	require.NotNil(t, got)
	require.Equal(t, "system prompt data", got.SystemPromptContext)
}

func TestPostCompactCleaner_RestoreSystemPrompt_Present_NilStore_NoOp(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{SessionID: "s1"})
	pc := &PreservedContext{SystemPromptContext: "system prompt data"}
	restored, err := p.restoreSystemPrompt(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)
}

func TestPostCompactCleaner_RestoreActiveFiles_Empty(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{ActiveFiles: nil}
	restored, err := p.restoreActiveFiles(context.Background(), pc)
	require.NoError(t, err)
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreActiveFiles_CallsRegistrar(t *testing.T) {
	t.Parallel()
	fr := &mockFileRegistrar{}
	p := NewPostCompactCleaner(PostCompactCleanerConfig{FileRegistrar: fr})
	pc := &PreservedContext{ActiveFiles: []string{"a.go", "b.go"}}
	restored, err := p.restoreActiveFiles(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)

	calls := fr.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"a.go", "b.go"}, calls[0])
}

func TestPostCompactCleaner_RestoreActiveFiles_RegistrarError(t *testing.T) {
	t.Parallel()
	fr := &mockFileRegistrar{err: fmt.Errorf("lsp unavailable")}
	p := NewPostCompactCleaner(PostCompactCleanerConfig{FileRegistrar: fr})
	pc := &PreservedContext{ActiveFiles: []string{"a.go"}}
	restored, err := p.restoreActiveFiles(context.Background(), pc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "re-registering active files")
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreActiveFiles_NilRegistrar(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{ActiveFiles: []string{"a.go"}}
	restored, err := p.restoreActiveFiles(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)
}

func TestPostCompactCleaner_RestoreRepoMap_Empty(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{RepoMapContent: ""}
	restored, err := p.restoreRepoMap(context.Background(), pc)
	require.NoError(t, err)
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreRepoMap_CallsInjector(t *testing.T) {
	t.Parallel()
	mi := &mockMapInjector{}
	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		MapInjector: mi,
		SessionID:   "s1",
	})
	pc := &PreservedContext{RepoMapContent: "repo map content"}
	restored, err := p.restoreRepoMap(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)

	require.Equal(t, []string{"s1"}, mi.getSessions())
}

func TestPostCompactCleaner_RestoreRepoMap_NilInjector(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{RepoMapContent: "repo map content"}
	restored, err := p.restoreRepoMap(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)
}

func TestPostCompactCleaner_RestoreToolState_Empty(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{ToolState: ""}
	restored, err := p.restoreToolState(context.Background(), pc)
	require.NoError(t, err)
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreToolState_WritesToOM(t *testing.T) {
	t.Parallel()
	om := newMockOMStore()
	ctx := context.Background()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		OMStore:   om,
		SessionID: "s1",
	})
	pc := &PreservedContext{ToolState: "tool execution state"}
	restored, err := p.restoreToolState(ctx, pc)
	require.NoError(t, err)
	require.True(t, restored)

	val, ok, err := om.Get(ctx, "s1", "restored_tool_state")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "tool execution state", val)
}

func TestPostCompactCleaner_RestoreToolState_OMError(t *testing.T) {
	t.Parallel()
	om := &mockOMStoreErr{err: fmt.Errorf("db locked")}
	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		OMStore:   om,
		SessionID: "s1",
	})
	pc := &PreservedContext{ToolState: "state"}
	restored, err := p.restoreToolState(context.Background(), pc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "persisting tool state")
	require.False(t, restored)
}

func TestPostCompactCleaner_RestoreToolState_NilOMStore(t *testing.T) {
	t.Parallel()
	p := NewPostCompactCleaner(PostCompactCleanerConfig{})
	pc := &PreservedContext{ToolState: "state"}
	restored, err := p.restoreToolState(context.Background(), pc)
	require.NoError(t, err)
	require.True(t, restored)
}

// mockOMStoreErr returns an error on every call.
type mockOMStoreErr struct {
	err error
}

func (m *mockOMStoreErr) Set(_ context.Context, _, _, _ string) error { return m.err }
func (m *mockOMStoreErr) Get(_ context.Context, _, _ string) (string, bool, error) {
	return "", false, m.err
}
func (m *mockOMStoreErr) Delete(_ context.Context, _, _ string) error { return m.err }
func (m *mockOMStoreErr) List(_ context.Context, _ string) (map[string]string, error) {
	return nil, m.err
}

func (m *mockOMStoreErr) ListByPriority(_ context.Context, _ string) ([]session.OMEntry, error) {
	return nil, m.err
}

// ---------------------------------------------------------------------------
// PreservedContext.IsEmpty
// ---------------------------------------------------------------------------

func TestPreservedContext_IsEmpty_AllFieldsEmpty(t *testing.T) {
	t.Parallel()
	require.True(t, (&PreservedContext{}).IsEmpty())
	require.True(t, (&PreservedContext{ActiveFiles: nil}).IsEmpty())
}

func TestPreservedContext_IsEmpty_SystemPromptSet(t *testing.T) {
	t.Parallel()
	require.False(t, (&PreservedContext{SystemPromptContext: "x"}).IsEmpty())
}

func TestPreservedContext_IsEmpty_ActiveFilesSet(t *testing.T) {
	t.Parallel()
	require.False(t, (&PreservedContext{ActiveFiles: []string{"a"}}).IsEmpty())
}

func TestPreservedContext_IsEmpty_RepoMapSet(t *testing.T) {
	t.Parallel()
	require.False(t, (&PreservedContext{RepoMapContent: "x"}).IsEmpty())
}

func TestPreservedContext_IsEmpty_ToolStateSet(t *testing.T) {
	t.Parallel()
	require.False(t, (&PreservedContext{ToolState: "x"}).IsEmpty())
}

// ---------------------------------------------------------------------------
// PreservedContextStore
// ---------------------------------------------------------------------------

func TestPreservedContextStore_SaveAndLoad(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	pc := &PreservedContext{SystemPromptContext: "test"}
	store.Save("s1", pc)
	require.Equal(t, pc, store.Load("s1"))
}

func TestPreservedContextStore_LoadMissing(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	require.Nil(t, store.Load("nonexistent"))
}

func TestPreservedContextStore_Delete(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{ToolState: "x"})
	require.NotNil(t, store.Load("s1"))
	store.Delete("s1")
	require.Nil(t, store.Load("s1"))
}

func TestPreservedContextStore_SaveReplaces(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{SystemPromptContext: "first"})
	store.Save("s1", &PreservedContext{SystemPromptContext: "second"})
	require.Equal(t, "second", store.Load("s1").SystemPromptContext)
}

// ---------------------------------------------------------------------------
// PreservedContextStore — Restored context methods
// ---------------------------------------------------------------------------

func TestPreservedContextStore_SaveRestored_WritesAndLoads(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.SaveRestored("s1", &PreservedContext{SystemPromptContext: "hello"})
	got := store.LoadRestored("s1")
	require.NotNil(t, got)
	require.Equal(t, "hello", got.SystemPromptContext)
}

func TestPreservedContextStore_SaveRestored_MergesFields(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()

	store.SaveRestored("s1", &PreservedContext{SystemPromptContext: "prompt"})
	store.SaveRestored("s1", &PreservedContext{RepoMapContent: "map"})

	got := store.LoadRestored("s1")
	require.NotNil(t, got)
	require.Equal(t, "prompt", got.SystemPromptContext)
	require.Equal(t, "map", got.RepoMapContent)
}

func TestPreservedContextStore_SaveRestored_SeparateFromPreserved(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()

	store.Save("s1", &PreservedContext{ToolState: "preserved"})
	store.SaveRestored("s1", &PreservedContext{SystemPromptContext: "restored"})

	require.NotNil(t, store.Load("s1"))
	require.Equal(t, "preserved", store.Load("s1").ToolState)

	got := store.LoadRestored("s1")
	require.NotNil(t, got)
	require.Equal(t, "restored", got.SystemPromptContext)
}

func TestPreservedContextStore_LoadRestored_Missing(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	require.Nil(t, store.LoadRestored("nonexistent"))
}

func TestPreservedContextStore_DeleteRestored(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.SaveRestored("s1", &PreservedContext{ToolState: "x"})
	require.NotNil(t, store.LoadRestored("s1"))
	store.DeleteRestored("s1")
	require.Nil(t, store.LoadRestored("s1"))
}

func TestPreservedContextStore_DeleteRestored_DoesNotAffectPreserved(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{ToolState: "preserved"})
	store.SaveRestored("s1", &PreservedContext{SystemPromptContext: "restored"})

	store.DeleteRestored("s1")
	require.Nil(t, store.LoadRestored("s1"))
	require.NotNil(t, store.Load("s1"))
}

// ---------------------------------------------------------------------------
// Registration with CompactionLayerManager
// ---------------------------------------------------------------------------

func TestPostCompactCleaner_RegisteredAsLayer4(t *testing.T) {
	t.Parallel()
	micro := NewMicroCompactor(MicroCompactorConfig{})
	postCompact := NewPostCompactCleaner(PostCompactCleanerConfig{})

	mgr := NewCompactionLayerManager(micro, postCompact)
	layers := mgr.Layers()

	require.Len(t, layers, 2)
	require.Equal(t, "micro-compactor", layers[0].Name())
	require.Equal(t, 1, layers[0].Priority())
	require.Equal(t, "post-compact-cleanup", layers[1].Name())
	require.Equal(t, 4, layers[1].Priority())
}

func TestPostCompactCleaner_RunAllWithLayerManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewPreservedContextStore()
	store.Save("s1", &PreservedContext{
		SystemPromptContext: "prompt",
		ActiveFiles:         []string{"main.go"},
	})

	postCompact := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:     store,
		SessionID: "s1",
	})

	micro := NewMicroCompactor(MicroCompactorConfig{})

	mgr := NewCompactionLayerManager(micro, postCompact)
	result, err := mgr.RunAll(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 2, result.ItemsAffected)

	result2, err2 := mgr.RunAll(ctx, Budget{})
	require.NoError(t, err2)
	require.False(t, result2.ActionTaken)
}

// ---------------------------------------------------------------------------
// SkillState round-trip
// ---------------------------------------------------------------------------

func TestSkillStateRoundTrip(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()

	original := &PreservedContext{
		SkillState: `{"skills":["git-master","code-review"],"tools_delta":["mcp_context7_get-library-doc"],"agents":["coder","task"]}`,
	}
	store.Save("s1", original)

	loaded := store.Load("s1")
	require.NotNil(t, loaded)
	require.Equal(t, original.SkillState, loaded.SkillState)

	store.Delete("s1")
	require.Nil(t, store.Load("s1"))
}

func TestPreservedContextIsEmpty_SkillStateSet(t *testing.T) {
	t.Parallel()
	require.False(t, (&PreservedContext{SkillState: `{"skills":[]}`}).IsEmpty())
}

func TestPreservedContextIsEmpty_OnlySkillState_NotEmpty(t *testing.T) {
	t.Parallel()
	pc := &PreservedContext{SkillState: "non-empty"}
	require.False(t, pc.IsEmpty())
}

func TestPreservedContextStore_SaveRestored_MergesSkillState(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()

	store.SaveRestored("s1", &PreservedContext{SystemPromptContext: "prompt"})
	store.SaveRestored("s1", &PreservedContext{SkillState: `{"skills":["git-master"]}`})

	got := store.LoadRestored("s1")
	require.NotNil(t, got)
	require.Equal(t, "prompt", got.SystemPromptContext)
	require.Equal(t, `{"skills":["git-master"]}`, got.SkillState)
}

// ---------------------------------------------------------------------------
// AgentConfigRestorer integration
// ---------------------------------------------------------------------------

// mockAgentConfigRestorer records calls to RestoreAgentConfig.
type mockAgentConfigRestorer struct {
	mu      sync.Mutex
	calls   []map[string][]string
	lastErr error
}

func (m *mockAgentConfigRestorer) RestoreAgentConfig(_ context.Context, payload map[string][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[string][]string)
	maps.Copy(cp, payload)
	m.calls = append(m.calls, cp)
	return m.lastErr
}

func (m *mockAgentConfigRestorer) getCalls() []map[string][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string][]string, len(m.calls))
	for i, c := range m.calls {
		out[i] = make(map[string][]string)
		maps.Copy(out[i], c)
	}
	return out
}

func TestPostCompactCleaner_Compact_WithAgentConfigRestorer(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	restorer := &mockAgentConfigRestorer{}
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SkillState: `{"skills":["git-master","code-review"],"tools_delta":["mcp_context7_get-library-doc"],"agents":["coder"]}`,
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:               store,
		SessionID:           "s1",
		AgentConfigRestorer: restorer,
	})

	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)

	// AgentConfigRestorer was called with the parsed payload.
	calls := restorer.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"git-master", "code-review"}, calls[0]["skills"])
	require.Equal(t, []string{"mcp_context7_get-library-doc"}, calls[0]["tools_delta"])
	require.Equal(t, []string{"coder"}, calls[0]["agents"])
}

func TestPostCompactCleaner_Compact_WithAgentConfigRestorer_Error(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	restorer := &mockAgentConfigRestorer{lastErr: fmt.Errorf("restorer failed")}
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SkillState: `{"skills":["git-master"]}`,
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:               store,
		SessionID:           "s1",
		AgentConfigRestorer: restorer,
	})

	_, err := p.Compact(ctx, Budget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "restoring agent config")
}

func TestPostCompactCleaner_Compact_WithAgentConfigRestorer_NilRestorer(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SkillState: `{"skills":["git-master"]}`,
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:     store,
		SessionID: "s1",
		// No AgentConfigRestorer configured.
	})

	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 1, result.ItemsAffected)
}

func TestPostCompactCleaner_Compact_AllFiveSteps(t *testing.T) {
	t.Parallel()
	store := NewPreservedContextStore()
	fr := &mockFileRegistrar{}
	mi := &mockMapInjector{}
	om := newMockOMStore()
	restorer := &mockAgentConfigRestorer{}
	ctx := context.Background()

	store.Save("s1", &PreservedContext{
		SystemPromptContext: "system prompt info",
		ActiveFiles:         []string{"file1.go", "file2.go"},
		RepoMapContent:      "repo map content",
		ToolState:           "tool state data",
		SkillState:          `{"skills":["git-master","code-review"]}`,
	})

	p := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:               store,
		SessionID:           "s1",
		FileRegistrar:       fr,
		MapInjector:         mi,
		OMStore:             om,
		AgentConfigRestorer: restorer,
	})

	result, err := p.Compact(ctx, Budget{})
	require.NoError(t, err)
	require.True(t, result.ActionTaken)
	require.Equal(t, 5, result.ItemsAffected)
	require.Equal(t, "post-compact-cleanup", result.LayerName)

	// Verify all restore steps were called.
	require.Len(t, fr.getCalls(), 1)
	require.Equal(t, []string{"file1.go", "file2.go"}, fr.getCalls()[0])
	require.Equal(t, []string{"s1"}, mi.getSessions())

	val, ok, err := om.Get(ctx, "s1", "restored_tool_state")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "tool state data", val)

	calls := restorer.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, []string{"git-master", "code-review"}, calls[0]["skills"])
}

func TestParseSkillState(t *testing.T) {
	t.Parallel()

	// Empty state returns nil.
	payload, err := parseSkillState("")
	require.NoError(t, err)
	require.Nil(t, payload)

	// Valid JSON is parsed correctly.
	payload, err = parseSkillState(`{"skills":["a","b"],"tools_delta":["c"],"agents":["d"]}`)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, []string{"a", "b"}, payload["skills"])
	require.Equal(t, []string{"c"}, payload["tools_delta"])
	require.Equal(t, []string{"d"}, payload["agents"])

	// Only skills present.
	payload, err = parseSkillState(`{"skills":["git-master"]}`)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, []string{"git-master"}, payload["skills"])
	require.Nil(t, payload["tools_delta"])
	require.Nil(t, payload["agents"])

	// Invalid JSON returns error.
	_, err = parseSkillState(`{invalid}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse skill state")
}
