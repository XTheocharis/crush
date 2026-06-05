// Package rewind provides session rewind, fork, and edit capabilities
// that restore session state to a prior turn boundary.
package rewind

import (
	"context"
	"time"
)

// RewindMode determines what gets reverted during a rewind operation.
type RewindMode int

const (
	// RewindCodeOnly restores files to their snapshot state but keeps the
	// conversation messages intact.
	RewindCodeOnly RewindMode = iota

	// RewindConvoOnly deletes messages after the target turn but does not
	// restore file contents.
	RewindConvoOnly

	// RewindBoth restores files and deletes messages after the target turn.
	RewindBoth
)

// TurnSnapshot represents a snapshot of file state at a user message boundary.
type TurnSnapshot struct {
	ID             string
	SessionID      string
	UserMessageID  string
	UserMessageSeq int
	CreatedAt      time.Time
}

// SnapshotFile represents a single file captured in a snapshot.
type SnapshotFile struct {
	FileID  string
	Path    string
	Version string
	Content string
}

// RewindResult holds the outcome of a rewind operation.
type RewindResult struct {
	MessagesDeleted int
	FilesRestored   int
	Snapshot        *TurnSnapshot
	ExtractedText   string
}

// ForkResult holds the outcome of a fork operation.
type ForkResult struct {
	NewSessionID    string
	NewSessionTitle string
	MessagesCloned  int
}

// EditResult holds the outcome of an edit-message operation.
type EditResult struct {
	NewMessageID  string
	ExtractedText string
}

// Snapshotter captures and retrieves turn-level file snapshots.
type Snapshotter interface {
	// CaptureSnapshot captures file state at the given user message sequence.
	CaptureSnapshot(ctx context.Context, sessionID string, userMessageSeq int) error
	// GetSnapshotAtOrBeforeSeq returns the most recent snapshot at or before the
	// given sequence number.
	GetSnapshotAtOrBeforeSeq(ctx context.Context, sessionID string, seq int) (*TurnSnapshot, error)
	// GetSnapshotFiles returns all files belonging to a snapshot.
	GetSnapshotFiles(ctx context.Context, snapshotID string) ([]SnapshotFile, error)
	// DeleteSnapshotsAfterSeq removes all snapshots with a sequence number
	// greater than the given value.
	DeleteSnapshotsAfterSeq(ctx context.Context, sessionID string, seq int) error
	// CleanupOldSnapshots removes stale snapshots for a session based on a
	// retention policy.
	CleanupOldSnapshots(ctx context.Context, sessionID string) error
}

// Rewinder reverts session state to a prior turn.
type Rewinder interface {
	// Rewind reverts the session to the state at the given sequence number,
	// according to the specified mode.
	Rewind(ctx context.Context, sessionID string, seq int, mode RewindMode) (*RewindResult, error)
}

// Forker creates an independent copy of a session from a given point.
type Forker interface {
	// Fork creates a new session that clones messages up to and including the
	// given sequence number.
	Fork(ctx context.Context, sessionID string, seq int) (*ForkResult, error)
}

// Editor extracts and edits a previous user message.
type Editor interface {
	// ExtractMessageText extracts the user message text at the given sequence
	// for editing, without modifying the database.
	ExtractMessageText(ctx context.Context, sessionID string, seq int) (*EditResult, error)

	// UpdateMessageText updates the text of a user message at the given
	// sequence in-place, without triggering an LLM response.
	UpdateMessageText(ctx context.Context, sessionID string, seq int, newText string) error
}

// PostRewindHook is a callback invoked after messages are deleted during a
// rewind operation. Errors are logged but do not fail the rewind.
type PostRewindHook func(ctx context.Context, sessionID string) error

// RewinderOption configures a rewinder.
type RewinderOption func(*rewinder)

// Service composes all rewind sub-services.
type Service interface {
	Snapshotter
	Rewinder
	Forker
	Editor
}
