package lcm

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/crush/internal/db"
)

// TimeGapCompactor is a sub-layer that detects time gaps greater than
// GapThreshold (default 30s) between consecutive messages and compacts
// older tool-result messages that fall between those gaps. This targets
// scenarios where the user stepped away or context switched, making
// prior tool outputs less relevant to the active conversation.
//
// It sits between MicroCompactor (Layer 1) and DedupCompactionLayer
// (Layer 2) in the compaction pipeline, using priority 1b (15).
type TimeGapCompactor struct {
	store        *Store
	sessionID    string
	gapThreshold time.Duration
}

// TimeGapCompactorConfig configures the TimeGapCompactor. If GapThreshold
// is zero, it defaults to 30 seconds.
type TimeGapCompactorConfig struct {
	Store        *Store
	SessionID    string
	GapThreshold time.Duration
}

// NewTimeGapCompactor creates a new TimeGapCompactor with the given config.
func NewTimeGapCompactor(cfg TimeGapCompactorConfig) *TimeGapCompactor {
	threshold := cfg.GapThreshold
	if threshold == 0 {
		threshold = 30 * time.Second
	}
	return &TimeGapCompactor{
		store:        cfg.Store,
		sessionID:    cfg.SessionID,
		gapThreshold: threshold,
	}
}

// Name returns "time-gap-compactor".
func (t *TimeGapCompactor) Name() string { return "time-gap-compactor" }

// Priority returns 15 (between Layer 1 and Layer 2).
func (t *TimeGapCompactor) Priority() int { return 15 }

// ShouldCompact returns true when there are consecutive message entries
// whose timestamps have a gap exceeding GapThreshold and at least one of
// those messages is a tool result older than the gap boundary.
func (t *TimeGapCompactor) ShouldCompact(ctx context.Context, _ Budget) bool {
	if t.store == nil || t.sessionID == "" {
		return false
	}

	gaps, err := t.findGapRegions(ctx)
	if err != nil {
		return false
	}
	return len(gaps) > 0
}

// Compact finds tool-result messages in gap regions and replaces them
// with archive stubs. A gap region is the set of messages that appear
// before a detected time gap. Only tool-result messages are compacted
// since those tend to be verbose and less relevant after a context switch.
func (t *TimeGapCompactor) Compact(ctx context.Context, _ Budget) (*CompactionLayerResult, error) {
	if t.store == nil {
		return nil, fmt.Errorf("time-gap-compactor: %w", ErrStoreIsNil)
	}
	if t.sessionID == "" {
		return nil, fmt.Errorf("time-gap-compactor: %w", ErrSessionIDEmpty)
	}

	gaps, err := t.findGapRegions(ctx)
	if err != nil {
		return nil, fmt.Errorf("time-gap-compactor: finding gaps: %w", err)
	}
	if len(gaps) == 0 {
		return &CompactionLayerResult{LayerName: t.Name()}, nil
	}

	msgs, err := t.store.GetMessages(ctx, t.sessionID)
	if err != nil {
		return nil, fmt.Errorf("time-gap-compactor: getting messages: %w", err)
	}
	msgContent := make(map[string]string, len(msgs))
	for _, m := range msgs {
		msgContent[m.ID] = m.Content
	}

	entries, err := t.store.GetContextEntries(ctx, t.sessionID)
	if err != nil {
		return nil, fmt.Errorf("time-gap-compactor: getting entries: %w", err)
	}
	entryByMsgID := make(map[string]ContextEntry, len(entries))
	for _, e := range entries {
		if e.ItemType == "message" && e.MessageID != "" {
			entryByMsgID[e.MessageID] = e
		}
	}

	toCompact := t.collectPreGapToolResults(ctx, gaps)

	var totalFreed int64
	var affected int

	for i, msgID := range toCompact {
		text := msgContent[msgID]
		if isAlreadyReferenced(text) {
			continue
		}

		entry, ok := entryByMsgID[msgID]
		if !ok {
			continue
		}

		stubID := SummaryIDPrefix + "tgap_" + contentHash(text)[:12] + "_" + fmt.Sprintf("%04d", i)
		stubContent := "[Archived from " + entry.MessageID + "] " + truncateString(text, 200)
		if err := t.store.q.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
			SummaryID:  stubID,
			SessionID:  t.sessionID,
			Kind:       KindArchiveStub,
			Content:    stubContent,
			TokenCount: entry.TokenCount / 10,
		}); err != nil {
			continue
		}
		totalFreed += entry.TokenCount
		affected++
	}

	return &CompactionLayerResult{
		LayerName:     t.Name(),
		TokensFreed:   totalFreed,
		ItemsAffected: affected,
		ActionTaken:   affected > 0,
	}, nil
}

// gapRegion describes a detected time gap: the messages before and after
// the gap boundary.
type gapRegion struct {
	// beforeMsgID is the ID of the message immediately before the gap.
	beforeMsgID string
	// afterMsgID is the ID of the message immediately after the gap.
	afterMsgID string
	// gapDuration is the duration of the gap.
	gapDuration time.Duration
}

// findGapRegions scans the context entries for this session, looks up
// message timestamps, and returns regions where the gap between consecutive
// messages exceeds GapThreshold.
func (t *TimeGapCompactor) findGapRegions(ctx context.Context) ([]gapRegion, error) {
	entries, err := t.store.GetContextEntries(ctx, t.sessionID)
	if err != nil {
		return nil, err
	}

	var msgEntries []ContextEntry
	for _, e := range entries {
		if e.ItemType == "message" && e.MessageID != "" {
			msgEntries = append(msgEntries, e)
		}
	}

	if len(msgEntries) < 2 {
		return nil, nil
	}

	msgCreatedAt, err := t.getMessageCreatedAt(ctx)
	if err != nil {
		return nil, err
	}

	var gaps []gapRegion
	for i := 0; i < len(msgEntries)-1; i++ {
		beforeTS, okBefore := msgCreatedAt[msgEntries[i].MessageID]
		afterTS, okAfter := msgCreatedAt[msgEntries[i+1].MessageID]
		if !okBefore || !okAfter {
			continue
		}

		gapDuration := time.Duration(afterTS-beforeTS) * time.Second
		if gapDuration > t.gapThreshold {
			gaps = append(gaps, gapRegion{
				beforeMsgID: msgEntries[i].MessageID,
				afterMsgID:  msgEntries[i+1].MessageID,
				gapDuration: gapDuration,
			})
		}
	}

	return gaps, nil
}

// collectPreGapToolResults returns message IDs for tool-result messages
// that appear before any detected gap region. These are candidates for
// compaction since they precede a context switch.
func (t *TimeGapCompactor) collectPreGapToolResults(ctx context.Context, gaps []gapRegion) []string {
	entries, err := t.store.GetContextEntries(ctx, t.sessionID)
	if err != nil {
		return nil
	}

	msgs, err := t.store.GetMessages(ctx, t.sessionID)
	if err != nil {
		return nil
	}

	toolRoleSet := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == "tool" {
			toolRoleSet[m.ID] = true
		}
	}

	gapBeforePositions := make(map[string]bool)
	for _, g := range gaps {
		gapBeforePositions[g.beforeMsgID] = true
	}

	var results []string
	for _, entry := range entries {
		if entry.ItemType != "message" || entry.MessageID == "" {
			continue
		}
		if !toolRoleSet[entry.MessageID] {
			continue
		}
		if gapBeforePositions[entry.MessageID] {
			results = append(results, entry.MessageID)
		}
	}

	return results
}

// getMessageCreatedAt returns a map from message ID to created_at (Unix
// seconds) for all messages in the session.
func (t *TimeGapCompactor) getMessageCreatedAt(ctx context.Context) (map[string]int64, error) {
	rows, err := t.store.rawDB.QueryContext(ctx,
		"SELECT id, created_at FROM messages WHERE session_id = ?", t.sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying message timestamps: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var id string
		var createdAt int64
		if err := rows.Scan(&id, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning message timestamp: %w", err)
		}
		result[id] = createdAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message timestamps: %w", err)
	}
	return result, nil
}
