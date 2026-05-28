# Layer 1c: ContentReplacementState

## Background

The DREAM specification describes a "ContentReplacementState" concept at Layer 1c
of the compaction pipeline. In the DREAM model, content replacement is not a
one-shot truncation but a stateful operation: each replacement records what was
removed, what replaced it, and why. This state can be queried, reversed, and
audited across compaction rounds.

Crush's current Layer 1 (MicroCompactor) handles the mechanics of truncating
large tool outputs, but it does not track replacement state. Layer 1c would sit
between MicroCompactor's raw truncation and the higher-level summarization
layers, providing a durable record of every content swap and the ability to undo
or inspect those swaps after the fact.

The closest existing analogue is `ReversibleCompactor` (in `reversible.go`),
which saves original messages alongside compressed summaries for later
decompression. ContentReplacementState extends that idea from summary
decompression to the full lifecycle of inline content replacements: truncation,
reference insertion, restore, and cross-round tracking.

## Current State

### MicroCompactor (Layer 1)

MicroCompactor is the first layer in the 8-layer compaction pipeline. It lives
in `compaction_layers.go` (lines 120-280) and does the following:

1. Scans context entries for messages whose token count exceeds a threshold
   (default: `LargeOutputThreshold`).
2. Calls `Store.InsertLargeTextContent` to persist the full content in the
   `lcm_large_files` SQLite table.
3. Replaces the inline text with a compact reference string containing the file
   ID and a preview of the first N characters (default: 2000).

```go
// Current replacement output format:
ref := fmt.Sprintf("[Large File Stored: %s]\nLCM File ID: %s\n\nPreview (first %d chars):\n%s",
    fileID, fileID, previewLimit, preview)
```

MicroCompactor has no concept of replacement state. It does not record:

- Which message IDs were compacted.
- What the original token count was before replacement.
- When the replacement happened (which compaction round).
- Whether the replacement is still active or has been superseded.

It checks `isAlreadyReferenced()` to avoid double-compacting, but this is a
simple string match on marker text, not a stateful lookup.

### ReversibleCompactor

`ReversibleCompactor` (in `reversible.go`) provides reversible summary
decompression. It saves original messages as JSON alongside a summary, keyed by
`block_id`. It offers:

- `SaveReversibleState`: persists original messages for a summary.
- `Decompress`: retrieves original messages, with detail-level filtering
  (`full`, `partial`, `metadata`).
- `DeleteReversibleState`: cleans up orphaned state.

This is close to what ContentReplacementState needs, but it operates at the
summary level, not the inline-content-replacement level. It cannot track a
sequence of replacements across rounds or handle the case where content is
replaced, then restored, then replaced again.

### PostCompactCleaner (Layer 4)

`PostCompactCleaner` (in `post_compact.go`) restores operational context after
compaction: system prompt additions, active files, repo map content, and tool
state. It uses `PreservedContextStore`, a thread-safe in-memory map keyed by
session ID.

This is a restore mechanism, but it is in-memory only and scoped to operational
context, not content replacements. It demonstrates the save/restore pattern but
does not persist across process restarts.

## Gap Analysis

### What MicroCompactor lacks

1. **No before/after mapping.** When MicroCompactor replaces content, it writes
   to the large-files table but does not record a mapping from the message ID
   (or context entry ID) to the replacement details. If something goes wrong,
   there is no way to look up what was replaced or what the original looked like
   without scanning all large files.

2. **No undo/restore across rounds.** Each compaction run is independent. If a
   replacement from round 2 needs to be undone in round 5 (because the user
   asked to see the original, or because budget pressure relaxed), there is no
   mechanism to do so. The content is in the large-files table, but the link
   back to the context entry is lost.

3. **No replacement decisions persisted.** MicroCompactor's decisions are
   implicit in the text it produces. If you want to answer "why was this message
   compacted?" or "how much did this replacement save?", you have to reverse-
   engineer the answer from the reference marker string.

4. **No explicit lifecycle state machine.** Content goes from "full" to
   "replaced" with no intermediate states. There is no "partially restored",
   "superseded", or "pinned" state. This limits the flexibility of downstream
   layers that might want to make decisions based on replacement state.

### What ContentReplacementState would add

| Capability | MicroCompactor | ContentReplacementState |
|---|---|---|
| Record replacement mapping | No | Yes |
| Undo/restore specific items | No | Yes |
| Cross-round tracking | No | Yes |
| Persisted decisions | Implicit | Explicit (SQLite) |
| Lifecycle states | Binary (full/replaced) | Multi-state |
| Query replacement history | No | Yes |
| Budget-aware restore | No | Yes |

## Proposed Design

### Core Types

```go
// ReplacementState tracks the lifecycle of a single content replacement.
type ReplacementState string

const (
    // ReplacementActive means the content has been replaced and the
    // reference is live in the context window.
    ReplacementActive ReplacementState = "active"

    // ReplacementRestored means the original content has been restored
    // and the replacement reference removed.
    ReplacementRestored ReplacementState = "restored"

    // ReplacementSuperseded means a subsequent compaction round replaced
    // the reference itself (e.g., the preview was truncated further).
    ReplacementSuperseded ReplacementState = "superseded"

    // ReplacementPinned means the content was marked as "do not compact"
    // by the user or a downstream layer.
    ReplacementPinned ReplacementState = "pinned"
)

// ContentReplacement records a single content swap.
type ContentReplacement struct {
    // ID is the unique identifier for this replacement record.
    ID string

    // SessionID is the session this replacement belongs to.
    SessionID string

    // ContextEntryID is the context entry that was replaced.
    ContextEntryID string

    // MessageID is the message whose content was replaced.
    MessageID string

    // FileID is the LCM large-file ID where the original content is stored.
    FileID string

    // State is the current lifecycle state.
    State ReplacementState

    // Round is the compaction round in which this replacement was created.
    // Round 0 means it happened outside the normal layer pipeline.
    Round int

    // OriginalTokenCount is the token count of the original content.
    OriginalTokenCount int64

    // ReplacementTokenCount is the token count of the reference + preview.
    ReplacementTokenCount int64

    // CreatedAt is when the replacement was created.
    CreatedAt time.Time

    // UpdatedAt is when the state last changed.
    UpdatedAt time.Time
}
```

### ContentReplacementStore

```go
// ContentReplacementStore persists replacement state in SQLite.
// It is backed by a new table in the LCM migration path.
type ContentReplacementStore struct {
    store *Store
}

// NewContentReplacementStore creates a store backed by the given LCM Store.
func NewContentReplacementStore(s *Store) *ContentReplacementStore {
    return &ContentReplacementStore{store: s}
}

// RecordReplacement saves a new replacement record.
func (s *ContentReplacementStore) RecordReplacement(
    ctx context.Context,
    repl ContentReplacement,
) error

// GetByContextEntry looks up the replacement record for a context entry.
// Returns the latest replacement (by round) if multiple exist.
func (s *ContentReplacementStore) GetByContextEntry(
    ctx context.Context,
    sessionID, contextEntryID string,
) (*ContentReplacement, error)

// GetByFileID looks up the replacement record for a stored file.
func (s *ContentReplacementStore) GetByFileID(
    ctx context.Context,
    sessionID, fileID string,
) (*ContentReplacement, error)

// ListByState returns all replacements in the given state for a session.
func (s *ContentReplacementStore) ListByState(
    ctx context.Context,
    sessionID string,
    state ReplacementState,
) ([]ContentReplacement, error)

// UpdateState transitions a replacement to a new state.
func (s *ContentReplacementStore) UpdateState(
    ctx context.Context,
    id string,
    newState ReplacementState,
) error

// ListByRound returns all replacements created in a given compaction round.
func (s *ContentReplacementStore) ListByRound(
    ctx context.Context,
    sessionID string,
    round int,
) ([]ContentReplacement, error)
```

### Integration with the 8-Layer Pipeline

ContentReplacementState would be consumed by MicroCompactor, not implemented as
a separate layer. The change is additive: MicroCompactor gains a reference to a
`ContentReplacementStore` and records each replacement it makes.

```go
type MicroCompactorConfig struct {
    TokenThreshold int64
    PreviewChars   int
    Store          *Store
    SessionID      string

    // ReplacementStore records replacement state for each truncation.
    // If nil, MicroCompactor operates in its current stateless mode.
    ReplacementStore *ContentReplacementStore

    // Round tracks the current compaction round for replacement records.
    Round int
}
```

When `ReplacementStore` is non-nil, `Compact` would call
`RecordReplacement` after each successful content swap. The `ShouldCompact`
method would check replacement state to skip entries that are already replaced
(rather than relying on the `isAlreadyReferenced` string match).

### Restore API

A new method on the Manager provides restore capability:

```go
// RestoreReplacement restores the original content for a context entry,
// reversing a prior MicroCompactor replacement. It updates the replacement
// state to "restored" and returns the original content from the large-files
// table.
//
// Returns an error if the entry has no active replacement or the budget
// cannot accommodate the restored content.
func (m *Manager) RestoreReplacement(
    ctx context.Context,
    sessionID, contextEntryID string,
    budget Budget,
) (string, error)

// RestoreAllByRound restores all replacements from a specific compaction
// round. Useful for undoing an entire compaction pass.
func (m *Manager) RestoreAllByRound(
    ctx context.Context,
    sessionID string,
    round int,
    budget Budget,
) ([]string, error)
```

### Persistence Schema

A new SQLite migration adds a `lcm_content_replacements` table:

```sql
CREATE TABLE lcm_content_replacements (
    id                 TEXT PRIMARY KEY,
    session_id         TEXT NOT NULL,
    context_entry_id   TEXT NOT NULL,
    message_id         TEXT NOT NULL,
    file_id            TEXT NOT NULL,
    state              TEXT NOT NULL DEFAULT 'active',
    round              INTEGER NOT NULL DEFAULT 0,
    original_token_count   INTEGER NOT NULL DEFAULT 0,
    replacement_token_count INTEGER NOT NULL DEFAULT 0,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (session_id) REFERENCES sessions(id),
    FOREIGN KEY (file_id)    REFERENCES lcm_large_files(file_id)
);

CREATE INDEX idx_replacements_session_state
    ON lcm_content_replacements(session_id, state);

CREATE INDEX idx_replacements_session_round
    ON lcm_content_replacements(session_id, round);

CREATE INDEX idx_replacements_context_entry
    ON lcm_content_replacements(context_entry_id);
```

The foreign key to `lcm_large_files` ensures replacement records cannot outlive
their stored content. The `session_id + state` index supports the common query
of listing all active replacements for a session.

## Code Sketches

### MicroCompactor Compact with State Tracking

```go
func (m *MicroCompactor) Compact(
    ctx context.Context, budget Budget,
) (*CompactionLayerResult, error) {
    if m.cfg.Store == nil {
        return nil, fmt.Errorf("micro-compactor: %w", ErrStoreIsNil)
    }
    if m.cfg.SessionID == "" {
        return nil, fmt.Errorf("micro-compactor: %w", ErrSessionIDEmpty)
    }

    entries, err := m.cfg.Store.GetContextEntries(ctx, m.cfg.SessionID)
    if err != nil {
        return nil, fmt.Errorf("getting context entries: %w", err)
    }

    threshold := m.cfg.threshold()
    previewLimit := m.cfg.previewLimit()
    var totalFreed int64
    var affected int

    for _, entry := range entries {
        if entry.ItemType != "message" || entry.TokenCount <= threshold {
            continue
        }

        // Check replacement state if store is available.
        if m.cfg.ReplacementStore != nil {
            existing, _ := m.cfg.ReplacementStore.GetByContextEntry(
                ctx, m.cfg.SessionID, entry.ID,
            )
            if existing != nil && existing.State == ReplacementActive {
                continue // Already replaced and active.
            }
        }

        // ... (existing message lookup and content storage logic) ...

        fileID, err := m.cfg.Store.InsertLargeTextContent(
            ctx, m.cfg.SessionID, msgText, "",
        )
        if err != nil {
            continue
        }

        // Record the replacement state.
        if m.cfg.ReplacementStore != nil {
            ref := fmt.Sprintf(
                "[Large File Stored: %s]\nLCM File ID: %s\n\nPreview (first %d chars):\n%s",
                fileID, fileID, previewLimit, preview,
            )
            repl := ContentReplacement{
                ID:                    GenerateReplacementID(entry.ID, m.cfg.Round),
                SessionID:             m.cfg.SessionID,
                ContextEntryID:        entry.ID,
                MessageID:             entry.MessageID,
                FileID:                fileID,
                State:                 ReplacementActive,
                Round:                 m.cfg.Round,
                OriginalTokenCount:    entry.TokenCount,
                ReplacementTokenCount: EstimateTokens(ref),
            }
            if err := m.cfg.ReplacementStore.RecordReplacement(ctx, repl); err != nil {
                // Log but don't fail the compaction pass.
                slog.Warn("Failed to record replacement state",
                    "error", err, "entry", entry.ID)
            }
        }

        totalFreed += freed
        affected++
    }

    return &CompactionLayerResult{
        LayerName:     m.Name(),
        TokensFreed:   totalFreed,
        ItemsAffected: affected,
        ActionTaken:   affected > 0,
    }, nil
}
```

### Manager Restore Method

```go
func (m *Manager) RestoreReplacement(
    ctx context.Context,
    sessionID, contextEntryID string,
    budget Budget,
) (string, error) {
    repl, err := m.replacementStore.GetByContextEntry(ctx, sessionID, contextEntryID)
    if err != nil {
        return "", fmt.Errorf("looking up replacement: %w", err)
    }
    if repl == nil || repl.State != ReplacementActive {
        return "", ErrNoActiveReplacement
    }

    // Check budget headroom.
    delta := repl.OriginalTokenCount - repl.ReplacementTokenCount
    if budget.ContextTokens+delta > budget.HardLimit {
        return "", fmt.Errorf(
            "restoring %d tokens would exceed hard limit (%d): %w",
            delta, budget.HardLimit, ErrBudgetExceeded,
        )
    }

    // Fetch original content from large-files table.
    original, err := m.store.GetLargeFileContent(ctx, repl.FileID, sessionID, 0)
    if err != nil {
        return "", fmt.Errorf("fetching original content: %w", err)
    }

    // Transition state.
    if err := m.replacementStore.UpdateState(
        ctx, repl.ID, ReplacementRestored,
    ); err != nil {
        return "", fmt.Errorf("updating replacement state: %w", err)
    }

    return original, nil
}
```

## Risks and Open Questions

### Performance

Each replacement adds a database write during compaction. MicroCompactor already
writes to `lcm_large_files`; adding a second write to `lcm_content_replacements`
roughly doubles the write cost per item. For sessions with many large outputs,
this could slow compaction by a measurable margin.

**Mitigation:** Batch inserts. Collect all replacements in a slice and do a
single transactional insert at the end of `Compact`, rather than one insert per
item. The existing `Store` already supports `WithTx` transaction composition.

### Interaction with Other Layers

Layers 2-5b may modify or remove context entries that have active replacement
records. For example, `StaleEvictionLayer` (Layer 3) might evict a tool output
whose content was already replaced by MicroCompactor. If the eviction removes
the context entry but not the replacement record, the record becomes an orphan.

**Mitigation:** `PostCompactCleaner` (Layer 4) should garbage-collect
replacement records whose context entries no longer exist. This can be a
lightweight pass at the end of each compaction round.

### Memory Usage

Sessions with hundreds of large tool outputs could accumulate hundreds of
replacement records. Each record is small (a few hundred bytes), so in-memory
representation is not a concern. The SQLite table will grow over time, but the
indexes keep common queries fast.

**Open question:** Should replacement records be pruned when a session ends, or
kept for cross-session ancestry lookups? The `lcm_large_files` table already
supports cross-session access via `GetAncestorSessionIDs`. Replacement records
should follow the same policy.

### Backward Compatibility

Existing sessions have no replacement records. MicroCompactor will continue to
work without `ReplacementStore` (the field is optional in the config). Old
sessions will not have replacement state, but the `isAlreadyReferenced` string
match still prevents double-compaction.

**Open question:** Should we backfill replacement records for existing
`lcm_large_files` entries? This would require scanning the table and matching
file IDs to context entries, which may not be possible if context entries have
been summarized away.

### Concurrency

The `ContentReplacementStore` must be safe for concurrent use across sessions.
Each session operates on its own rows (filtered by `session_id`), so the main
concurrency concern is write contention on the SQLite database. The existing
`Store` already handles this with transactional writes.

### Restore Scope

The current design supports restoring individual items and entire rounds. An
open question is whether we need a "partial restore" that expands the preview
without fully restoring the original content. This would be useful when budget
pressure relaxes slightly but not enough for full restoration.

**Possible extension:** A `RestorePartial` method that increases the preview
length without removing the reference. This would create a new replacement
record with a larger `ReplacementTokenCount` and transition the old one to
`ReplacementSuperseded`.

### Relationship to ReversibleCompactor

ContentReplacementState and `ReversibleCompactor` serve overlapping but
distinct purposes. ReversibleCompactor deals with summary decompression
(restoring original messages from a compressed summary). ContentReplacementState
deals with inline content swaps (restoring original text from a truncated
reference). Both store originals and support restore, but they operate at
different levels of the compaction pipeline.

**Open question:** Should they share a common interface or storage mechanism?
Convergence might reduce code duplication, but the two use cases have different
lifecycle patterns. Summaries are created once and decompressed on demand.
Content replacements may be toggled multiple times as budget pressure changes.
Keeping them separate for now seems prudent, with convergence as a future
refactoring target once both are stable.
