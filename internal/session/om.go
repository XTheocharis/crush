package session

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// OMEntry is a single operational memory entry with priority.
type OMEntry struct {
	Key      string
	Value    string
	Priority string
}

// OperationalMemory is a per-session key-value store backed by SQLite.
// All operations are scoped to a specific session ID and thread ID,
// ensuring strict isolation between sessions and threads. An empty
// thread ID provides backward compatibility with pre-thread callers.
type OperationalMemory struct {
	db *sql.DB
}

// NewOperationalMemory creates a new OperationalMemory backed by the given
// database connection.
func NewOperationalMemory(db *sql.DB) *OperationalMemory {
	return &OperationalMemory{db: db}
}

// Set upserts a key-value pair for the given session with default "medium"
// priority and no thread scoping. If the key already exists its value and
// updated_at timestamp are updated.
func (om *OperationalMemory) Set(ctx context.Context, sessionID, key, value string) error {
	return om.SetWithPriorityThread(ctx, sessionID, "", key, value, "medium")
}

// SetThread upserts a key-value pair for the given session and thread with
// default "medium" priority.
func (om *OperationalMemory) SetThread(ctx context.Context, sessionID, threadID, key, value string) error {
	return om.SetWithPriorityThread(ctx, sessionID, threadID, key, value, "medium")
}

// SetWithPriority upserts a key-value pair for the given session with the
// specified priority and no thread scoping.
func (om *OperationalMemory) SetWithPriority(ctx context.Context, sessionID, key, value, priority string) error {
	return om.SetWithPriorityThread(ctx, sessionID, "", key, value, priority)
}

// SetWithPriorityThread upserts a key-value pair for the given session and
// thread with the specified priority ("high", "medium", or "low"). If the
// key already exists its value, priority, and updated_at timestamp are
// updated.
func (om *OperationalMemory) SetWithPriorityThread(ctx context.Context, sessionID, threadID, key, value, priority string) error {
	const q = `
		INSERT INTO session_operational_memory (session_id, thread_id, key, value, priority, updated_at)
		VALUES (?, ?, ?, ?, ?, strftime('%s', 'now'))
		ON CONFLICT(session_id, thread_id, key) DO UPDATE SET
			value = excluded.value,
			priority = excluded.priority,
			updated_at = strftime('%s', 'now')
	`
	_, err := om.db.ExecContext(ctx, q, sessionID, threadID, key, value, normalizePriority(priority))
	if err != nil {
		return fmt.Errorf("upserting operational memory key %q for session %q thread %q: %w", key, sessionID, threadID, err)
	}
	return nil
}

// Get retrieves the value for a key within a session (no thread scoping).
// It returns the value, whether the key was found, and any error.
func (om *OperationalMemory) Get(ctx context.Context, sessionID, key string) (string, bool, error) {
	return om.GetThread(ctx, sessionID, "", key)
}

// GetThread retrieves the value for a key within a session and thread.
// It returns the value, whether the key was found, and any error.
func (om *OperationalMemory) GetThread(ctx context.Context, sessionID, threadID, key string) (string, bool, error) {
	const q = `SELECT value FROM session_operational_memory WHERE session_id = ? AND thread_id = ? AND key = ?`
	var value string
	err := om.db.QueryRowContext(ctx, q, sessionID, threadID, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("getting operational memory key %q for session %q thread %q: %w", key, sessionID, threadID, err)
	}
	return value, true, nil
}

// Delete removes a key from a session's operational memory (no thread
// scoping). It is not an error if the key does not exist.
func (om *OperationalMemory) Delete(ctx context.Context, sessionID, key string) error {
	return om.DeleteThread(ctx, sessionID, "", key)
}

// DeleteThread removes a key from a session's operational memory for a
// specific thread. It is not an error if the key does not exist.
func (om *OperationalMemory) DeleteThread(ctx context.Context, sessionID, threadID, key string) error {
	const q = `DELETE FROM session_operational_memory WHERE session_id = ? AND thread_id = ? AND key = ?`
	_, err := om.db.ExecContext(ctx, q, sessionID, threadID, key)
	if err != nil {
		return fmt.Errorf("deleting operational memory key %q for session %q thread %q: %w", key, sessionID, threadID, err)
	}
	return nil
}

// List returns all key-value pairs for a session as a map (no thread
// scoping). Returns an empty map if the session has no operational memory
// entries.
func (om *OperationalMemory) List(ctx context.Context, sessionID string) (map[string]string, error) {
	return om.ListThread(ctx, sessionID, "")
}

// ListThread returns all key-value pairs for a session and thread as a map.
func (om *OperationalMemory) ListThread(ctx context.Context, sessionID, threadID string) (map[string]string, error) {
	const q = `SELECT key, value FROM session_operational_memory WHERE session_id = ? AND thread_id = ? ORDER BY key`
	rows, err := om.db.QueryContext(ctx, q, sessionID, threadID)
	if err != nil {
		return nil, fmt.Errorf("listing operational memory for session %q thread %q: %w", sessionID, threadID, err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning operational memory row for session %q thread %q: %w", sessionID, threadID, err)
		}
		result[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating operational memory rows for session %q thread %q: %w", sessionID, threadID, err)
	}
	return result, nil
}

// ListByPriority returns all entries for a session sorted by priority
// (no thread scoping): high first, then medium, then low.
func (om *OperationalMemory) ListByPriority(ctx context.Context, sessionID string) ([]OMEntry, error) {
	return om.ListByPriorityThread(ctx, sessionID, "")
}

// ListByPriorityThread returns all entries for a session and thread sorted
// by priority: high first, then medium, then low.
func (om *OperationalMemory) ListByPriorityThread(ctx context.Context, sessionID, threadID string) ([]OMEntry, error) {
	const q = `SELECT key, value, COALESCE(priority, 'medium') FROM session_operational_memory WHERE session_id = ? AND thread_id = ?`
	rows, err := om.db.QueryContext(ctx, q, sessionID, threadID)
	if err != nil {
		return nil, fmt.Errorf("listing operational memory by priority for session %q thread %q: %w", sessionID, threadID, err)
	}
	defer rows.Close()

	var entries []OMEntry
	for rows.Next() {
		var e OMEntry
		if err := rows.Scan(&e.Key, &e.Value, &e.Priority); err != nil {
			return nil, fmt.Errorf("scanning operational memory row for session %q thread %q: %w", sessionID, threadID, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating operational memory rows for session %q thread %q: %w", sessionID, threadID, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return priorityOrder(entries[i].Priority) < priorityOrder(entries[j].Priority)
	})

	return entries, nil
}

func normalizePriority(p string) string {
	switch p {
	case "high", "medium", "low":
		return p
	default:
		return "medium"
	}
}

func priorityOrder(p string) int {
	switch p {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 1
	}
}

// PriorityEmoji returns the emoji corresponding to a priority level.
// "high" → 🔴, "medium" → 🟡, "low" → 🟢. Unknown priorities return 🟡.
func PriorityEmoji(p string) string {
	switch p {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "🟡"
	}
}

// FormatPriorityEmoji returns the priority string prefixed with its emoji,
// e.g. "🔴 high".
func FormatPriorityEmoji(p string) string {
	return PriorityEmoji(p) + " " + p
}

// ThreadScopedEntry extends OMEntry with temporal information used for
// relevance-based decay scoring. UpdatedAt is the Unix timestamp when the
// entry was last modified, and Relevance is a [0,1] score computed via
// exponential decay from the entry's age relative to a reference time.
type ThreadScopedEntry struct {
	OMEntry
	UpdatedAt int64
	Relevance float64
}

// TemporalAnchorHalfLife is the half-life duration used for exponential decay
// of operational memory relevance. After one half-life (1 hour), a memory's
// relevance drops to 50%.
const TemporalAnchorHalfLife = time.Hour

// RelevanceDecay computes an exponential decay factor for the given age
// (duration since last update). The result is 1.0 when age is 0 and halves
// every TemporalAnchorHalfLife. The formula is:
//
//	relevance = 0.5^(age / halfLife)
func RelevanceDecay(age time.Duration) float64 {
	if age <= 0 {
		return 1.0
	}
	return math.Pow(0.5, age.Seconds()/TemporalAnchorHalfLife.Seconds())
}

// ThreadScopedOM wraps OperationalMemory with a fixed sessionID and threadID,
// so callers do not need to pass these identifiers on every call. An empty
// threadID provides backward-compatible session-scoped behavior.
type ThreadScopedOM struct {
	om        *OperationalMemory
	sessionID string
	threadID  string
}

// NewThreadScopedOM creates a ThreadScopedOM bound to the given session and
// thread. Pass an empty threadID for session-scoped (backward-compatible)
// behavior.
func NewThreadScopedOM(om *OperationalMemory, sessionID, threadID string) *ThreadScopedOM {
	return &ThreadScopedOM{om: om, sessionID: sessionID, threadID: threadID}
}

// SessionID returns the session this ThreadScopedOM is bound to.
func (tsom *ThreadScopedOM) SessionID() string { return tsom.sessionID }

// ThreadID returns the thread this ThreadScopedOM is bound to.
func (tsom *ThreadScopedOM) ThreadID() string { return tsom.threadID }

// Set upserts a key-value pair with default "medium" priority.
func (tsom *ThreadScopedOM) Set(ctx context.Context, key, value string) error {
	return tsom.om.SetWithPriorityThread(ctx, tsom.sessionID, tsom.threadID, key, value, "medium")
}

// SetWithPriority upserts a key-value pair with the specified priority.
func (tsom *ThreadScopedOM) SetWithPriority(ctx context.Context, key, value, priority string) error {
	return tsom.om.SetWithPriorityThread(ctx, tsom.sessionID, tsom.threadID, key, value, priority)
}

// Get retrieves the value for a key. Returns the value, whether it was found,
// and any error.
func (tsom *ThreadScopedOM) Get(ctx context.Context, key string) (string, bool, error) {
	return tsom.om.GetThread(ctx, tsom.sessionID, tsom.threadID, key)
}

// Delete removes a key. It is not an error if the key does not exist.
func (tsom *ThreadScopedOM) Delete(ctx context.Context, key string) error {
	return tsom.om.DeleteThread(ctx, tsom.sessionID, tsom.threadID, key)
}

// List returns all key-value pairs as a map.
func (tsom *ThreadScopedOM) List(ctx context.Context) (map[string]string, error) {
	return tsom.om.ListThread(ctx, tsom.sessionID, tsom.threadID)
}

// ListByPriority returns entries sorted by priority: high first, then medium,
// then low.
func (tsom *ThreadScopedOM) ListByPriority(ctx context.Context) ([]OMEntry, error) {
	return tsom.om.ListByPriorityThread(ctx, tsom.sessionID, tsom.threadID)
}

// ListWithRelevance returns entries enriched with temporal relevance scores,
// sorted by relevance-adjusted priority. The referenceTime parameter
// determines the "now" used for decay computation — pass time.Now() in
// production or a fixed time in tests.
func (tsom *ThreadScopedOM) ListWithRelevance(ctx context.Context, referenceTime time.Time) ([]ThreadScopedEntry, error) {
	const q = `SELECT key, value, COALESCE(priority, 'medium'), updated_at FROM session_operational_memory WHERE session_id = ? AND thread_id = ?`
	rows, err := tsom.om.db.QueryContext(ctx, q, tsom.sessionID, tsom.threadID)
	if err != nil {
		return nil, fmt.Errorf("listing thread-scoped entries with relevance for session %q thread %q: %w", tsom.sessionID, tsom.threadID, err)
	}
	defer rows.Close()

	var entries []ThreadScopedEntry
	for rows.Next() {
		var e ThreadScopedEntry
		if err := rows.Scan(&e.Key, &e.Value, &e.Priority, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning thread-scoped entry for session %q thread %q: %w", tsom.sessionID, tsom.threadID, err)
		}
		age := referenceTime.Sub(time.Unix(e.UpdatedAt, 0))
		e.Relevance = RelevanceDecay(age)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating thread-scoped entries for session %q thread %q: %w", tsom.sessionID, tsom.threadID, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		// Effective score: priority weight × relevance.
		scoreI := float64(priorityOrderMax()-priorityOrder(entries[i].Priority)) * entries[i].Relevance
		scoreJ := float64(priorityOrderMax()-priorityOrder(entries[j].Priority)) * entries[j].Relevance
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		// Tiebreak by recency (newer first).
		return entries[i].UpdatedAt > entries[j].UpdatedAt
	})

	return entries, nil
}

// FormatThreadMemory assembles a human-readable summary of all thread-scoped
// memory entries, each prefixed with its priority emoji and annotated with
// relevance percentage. Returns an empty string if no entries exist.
func (tsom *ThreadScopedOM) FormatThreadMemory(ctx context.Context, referenceTime time.Time) (string, error) {
	entries, err := tsom.ListWithRelevance(ctx, referenceTime)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s: %s (relevance: %.0f%%)\n",
			PriorityEmoji(e.Priority), e.Key, e.Value, e.Relevance*100)
	}
	return b.String(), nil
}

// priorityOrderMax returns the maximum priorityOrder value for score scaling.
func priorityOrderMax() int { return 3 }
