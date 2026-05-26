package lcm

import (
	"context"
	"fmt"
	"sync"
)

// PreservedContext holds operational context that was saved before compaction
// and needs to be restored after the context window is freed.
type PreservedContext struct {
	// SystemPromptContext holds system prompt additions to restore.
	SystemPromptContext string

	// ActiveFiles holds file paths that were active at compaction time.
	ActiveFiles []string

	// RepoMapContent holds the repo map text to restore.
	RepoMapContent string

	// ToolState holds serialized tool execution state to restore.
	ToolState string

	// SkillState holds serialized skill/tool state to restore.
	SkillState string
}

// IsEmpty reports whether there is nothing to restore.
func (pc *PreservedContext) IsEmpty() bool {
	return pc.SystemPromptContext == "" &&
		len(pc.ActiveFiles) == 0 &&
		pc.RepoMapContent == "" &&
		pc.ToolState == "" &&
		pc.SkillState == ""
}

// PreservedContextStore is a thread-safe store for preserved context keyed by
// session ID. Layers that strip operational context save it here before
// compaction; the PostCompactCleaner reads and clears it after compaction.
type PreservedContextStore struct {
	mu   sync.RWMutex
	data map[string]*PreservedContext
}

// NewPreservedContextStore creates a new empty store.
func NewPreservedContextStore() *PreservedContextStore {
	return &PreservedContextStore{
		data: make(map[string]*PreservedContext),
	}
}

// Save stores preserved context for a session, replacing any previous value.
func (s *PreservedContextStore) Save(sessionID string, pc *PreservedContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = pc
}

// Load returns the preserved context for a session, or nil if none exists.
func (s *PreservedContextStore) Load(sessionID string) *PreservedContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[sessionID]
}

// Delete removes the preserved context for a session.
func (s *PreservedContextStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, sessionID)
}

// restorableKey returns the internal key used for restored context entries.
func restorableKey(sessionID string) string { return "restored:" + sessionID }

// SaveRestored saves preserved context under a "restored" key for the session
// so it survives the post-compact Delete and can be read by prompt assembly.
// Non-empty fields in pc are merged into any existing restored entry.
func (s *PreservedContextStore) SaveRestored(sessionID string, pc *PreservedContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := restorableKey(sessionID)
	existing := s.data[key]
	if existing == nil {
		s.data[key] = &PreservedContext{}
		existing = s.data[key]
	}
	if pc.SystemPromptContext != "" {
		existing.SystemPromptContext = pc.SystemPromptContext
	}
	if len(pc.ActiveFiles) > 0 {
		existing.ActiveFiles = pc.ActiveFiles
	}
	if pc.RepoMapContent != "" {
		existing.RepoMapContent = pc.RepoMapContent
	}
	if pc.ToolState != "" {
		existing.ToolState = pc.ToolState
	}
	if pc.SkillState != "" {
		existing.SkillState = pc.SkillState
	}
}

// LoadRestored returns the restored context for a session, or nil if none.
func (s *PreservedContextStore) LoadRestored(sessionID string) *PreservedContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[restorableKey(sessionID)]
}

// DeleteRestored removes the restored context for a session.
func (s *PreservedContextStore) DeleteRestored(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, restorableKey(sessionID))
}

// FileRegistrar re-registers files with the LSP manager for diagnostics after
// compaction. Defined here to avoid a circular import on the lsp package.
type FileRegistrar interface {
	// OpenFiles opens the given files in their respective LSP servers.
	OpenFiles(ctx context.Context, files []string) error
}

// MapInjector signals the repo-map service to re-inject on the next prompt
// cycle. Defined here to avoid a circular import on the repomap package.
type MapInjector interface {
	// RequestInjection clears any injection tracking so the next prepare-step
	// re-evaluates whether the repo map should be injected.
	RequestInjection(sessionID string)
}

// PostCompactCleanerConfig controls the behaviour of the PostCompactCleaner.
type PostCompactCleanerConfig struct {
	// Store is the preserved context store. Required.
	Store *PreservedContextStore

	// SessionID is the session this cleaner operates on.
	SessionID string

	// FileRegistrar re-registers active files with the LSP. Optional.
	FileRegistrar FileRegistrar

	// MapInjector signals repo-map re-injection. Optional.
	MapInjector MapInjector

	// OMStore persists tool state to operational memory. Optional.
	OMStore OperationalMemoryStore
}

// PostCompactCleaner is Layer 4 of the compaction framework. It restores
// operational context that was preserved before compaction ran, after earlier
// layers have freed tokens in the context window.
//
// The 4-step restore sequence:
//
//  1. Restore system prompt context
//  2. Restore active file list
//  3. Restore repo map
//  4. Restore tool state
type PostCompactCleaner struct {
	cfg PostCompactCleanerConfig
}

// NewPostCompactCleaner creates a Layer 4 PostCompactCleaner with the given
// config.
func NewPostCompactCleaner(cfg PostCompactCleanerConfig) *PostCompactCleaner {
	return &PostCompactCleaner{cfg: cfg}
}

// Name returns "post-compact-cleanup".
func (p *PostCompactCleaner) Name() string { return "post-compact-cleanup" }

// Priority returns 4 (Layer 4).
func (p *PostCompactCleaner) Priority() int { return 4 }

// ShouldCompact reports whether there is preserved context to restore for the
// configured session.
func (p *PostCompactCleaner) ShouldCompact(_ context.Context, _ Budget) bool {
	if p.cfg.Store == nil || p.cfg.SessionID == "" {
		return false
	}
	pc := p.cfg.Store.Load(p.cfg.SessionID)
	return pc != nil && !pc.IsEmpty()
}

// Compact executes the 4-step restore sequence and clears the preserved
// context for the session. Each step is independent; a failure in one step
// does not prevent subsequent steps from running. If no context is preserved,
// Compact is a graceful no-op.
func (p *PostCompactCleaner) Compact(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if p.cfg.Store == nil {
		return nil, fmt.Errorf("post-compact-cleanup: %w", ErrStoreIsNil)
	}
	if p.cfg.SessionID == "" {
		return nil, fmt.Errorf("post-compact-cleanup: %w", ErrSessionIDEmpty)
	}

	pc := p.cfg.Store.Load(p.cfg.SessionID)
	if pc == nil || pc.IsEmpty() {
		// Graceful no-op: nothing to restore.
		return &CompactionLayerResult{
			LayerName: p.Name(),
		}, nil
	}

	var itemsRestored int
	var firstErr error

	// Step 1: Restore system prompt context.
	if restored, err := p.restoreSystemPrompt(ctx, pc); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("restoring system prompt: %w", err)
		}
	} else if restored {
		itemsRestored++
	}

	// Step 2: Restore active file list.
	if restored, err := p.restoreActiveFiles(ctx, pc); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("restoring active files: %w", err)
		}
	} else if restored {
		itemsRestored++
	}

	// Step 3: Restore repo map.
	if restored, err := p.restoreRepoMap(ctx, pc); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("restoring repo map: %w", err)
		}
	} else if restored {
		itemsRestored++
	}

	// Step 4: Restore tool state.
	if restored, err := p.restoreToolState(ctx, pc); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("restoring tool state: %w", err)
		}
	} else if restored {
		itemsRestored++
	}

	// Clear preserved context after restore so subsequent calls are idempotent.
	p.cfg.Store.Delete(p.cfg.SessionID)

	if firstErr != nil {
		return nil, firstErr
	}

	return &CompactionLayerResult{
		LayerName:     p.Name(),
		ItemsAffected: itemsRestored,
		ActionTaken:   itemsRestored > 0,
	}, nil
}

// restoreSystemPrompt restores the system prompt context from preserved data.
// It writes the prompt to the restored area of the store so the next prompt
// assembly cycle can pick it up.
func (p *PostCompactCleaner) restoreSystemPrompt(_ context.Context, pc *PreservedContext) (bool, error) {
	if pc.SystemPromptContext == "" {
		return false, nil
	}
	if p.cfg.Store != nil {
		p.cfg.Store.SaveRestored(p.cfg.SessionID, &PreservedContext{
			SystemPromptContext: pc.SystemPromptContext,
		})
	}
	return true, nil
}

// restoreActiveFiles restores the active file list from preserved data by
// re-registering files with the LSP for diagnostics.
func (p *PostCompactCleaner) restoreActiveFiles(ctx context.Context, pc *PreservedContext) (bool, error) {
	if len(pc.ActiveFiles) == 0 {
		return false, nil
	}
	if p.cfg.FileRegistrar != nil {
		if err := p.cfg.FileRegistrar.OpenFiles(ctx, pc.ActiveFiles); err != nil {
			return false, fmt.Errorf("re-registering active files: %w", err)
		}
	}
	return true, nil
}

// restoreRepoMap restores the repo map content by signalling the repo-map
// service to re-inject on the next prompt cycle.
func (p *PostCompactCleaner) restoreRepoMap(_ context.Context, pc *PreservedContext) (bool, error) {
	if pc.RepoMapContent == "" {
		return false, nil
	}
	if p.cfg.MapInjector != nil {
		p.cfg.MapInjector.RequestInjection(p.cfg.SessionID)
	}
	return true, nil
}

// restoreToolState restores tool execution state by persisting it to the
// session operational memory.
func (p *PostCompactCleaner) restoreToolState(ctx context.Context, pc *PreservedContext) (bool, error) {
	if pc.ToolState == "" {
		return false, nil
	}
	if p.cfg.OMStore != nil {
		if err := p.cfg.OMStore.Set(ctx, p.cfg.SessionID, "restored_tool_state", pc.ToolState); err != nil {
			return false, fmt.Errorf("persisting tool state: %w", err)
		}
	}
	return true, nil
}
