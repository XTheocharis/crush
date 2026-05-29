package lcm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReplacementState represents the lifecycle state of a content replacement.
type ReplacementState string

const (
	// ReplacementActive means the replacement is currently in effect.
	ReplacementActive ReplacementState = "active"
	// ReplacementRestored means the original content has been restored.
	ReplacementRestored ReplacementState = "restored"
	// ReplacementSuperseded means a newer replacement has taken over.
	ReplacementSuperseded ReplacementState = "superseded"
	// ReplacementPinned means the replacement is pinned and won't be evicted.
	ReplacementPinned ReplacementState = "pinned"
)

// ValidTransitions defines the allowed state transitions for replacement
// lifecycle management. Keys are source states; values are the set of
// permitted target states.
var ValidTransitions = map[ReplacementState]map[ReplacementState]bool{
	ReplacementActive: {
		ReplacementRestored:   true,
		ReplacementSuperseded: true,
		ReplacementPinned:     true,
	},
	ReplacementRestored: {
		ReplacementActive: true,
	},
	ReplacementSuperseded: {},
	ReplacementPinned: {
		ReplacementActive: true,
	},
}

// ContentReplacement represents a single replacement of context content within
// a session. It tracks the original and replacement token counts, the
// lifecycle state, and references to associated messages and files.
type ContentReplacement struct {
	ID                    int64
	SessionID             string
	Position              int64
	MessageID             sql.NullString
	FileID                sql.NullString
	State                 ReplacementState
	Round                 int
	OriginalTokenCount    int
	ReplacementTokenCount int
	CreatedAt             int64
	UpdatedAt             int64
}

// ContentReplacementStore defines the persistence interface for content
// replacement records.
type ContentReplacementStore interface {
	// RecordReplacement inserts a new content replacement and returns its ID.
	RecordReplacement(ctx context.Context, replacement ContentReplacement) (int64, error)
	// GetBySessionPosition returns all replacements for a given session position.
	GetBySessionPosition(ctx context.Context, sessionID string, position int64) ([]ContentReplacement, error)
	// GetByFileID returns all replacements associated with a file ID.
	GetByFileID(ctx context.Context, sessionID string, fileID string) ([]ContentReplacement, error)
	// ListByState returns all replacements in the given state for a session.
	ListByState(ctx context.Context, sessionID string, state ReplacementState) ([]ContentReplacement, error)
	// UpdateState transitions a replacement to a new state.
	UpdateState(ctx context.Context, id int64, newState ReplacementState) error
	// ListByRound returns all replacements created in a specific compaction round.
	ListByRound(ctx context.Context, sessionID string, round int) ([]ContentReplacement, error)
}

// --- Error types ---

// ErrNoActiveReplacement indicates that no active replacement was found for
// the requested operation.
var ErrNoActiveReplacement = errors.New("lcm: no active replacement found")

// ErrBudgetExceeded indicates that the token budget would be exceeded by the
// requested replacement operation.
var ErrBudgetExceeded = errors.New("lcm: replacement budget exceeded")

// ErrInvalidStateTransition indicates that the requested state transition is
// not allowed by the replacement lifecycle state machine.
type ErrInvalidStateTransition struct {
	From ReplacementState
	To   ReplacementState
}

// Error implements the error interface.
func (e ErrInvalidStateTransition) Error() string {
	return fmt.Sprintf("lcm: invalid state transition from %q to %q", e.From, e.To)
}

// ValidateTransition checks whether a transition from one state to another is
// allowed by the ValidTransitions state machine. Returns ErrInvalidStateTransition
// if the transition is not permitted.
func ValidateTransition(from, to ReplacementState) error {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return ErrInvalidStateTransition{From: from, To: to}
	}
	if !allowed[to] {
		return ErrInvalidStateTransition{From: from, To: to}
	}
	return nil
}
