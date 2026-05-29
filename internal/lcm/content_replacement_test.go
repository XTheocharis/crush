package lcm

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplacementStateStringValues(t *testing.T) {
	tests := []struct {
		state ReplacementState
		want  string
	}{
		{ReplacementActive, "active"},
		{ReplacementRestored, "restored"},
		{ReplacementSuperseded, "superseded"},
		{ReplacementPinned, "pinned"},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			require.Equal(t, tt.want, string(tt.state))
		})
	}
}

func TestValidTransitions(t *testing.T) {
	validCases := []struct {
		from ReplacementState
		to   ReplacementState
	}{
		{ReplacementActive, ReplacementRestored},
		{ReplacementActive, ReplacementSuperseded},
		{ReplacementActive, ReplacementPinned},
		{ReplacementRestored, ReplacementActive},
		{ReplacementPinned, ReplacementActive},
	}
	for _, tc := range validCases {
		t.Run("valid_"+string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to)
			require.NoError(t, err)
		})
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalidCases := []struct {
		from ReplacementState
		to   ReplacementState
	}{
		{ReplacementSuperseded, ReplacementActive},
		{ReplacementSuperseded, ReplacementRestored},
		{ReplacementSuperseded, ReplacementPinned},
		{ReplacementRestored, ReplacementSuperseded},
		{ReplacementRestored, ReplacementPinned},
		{ReplacementPinned, ReplacementSuperseded},
		{ReplacementPinned, ReplacementRestored},
		{ReplacementActive, ReplacementActive},
	}
	for _, tc := range invalidCases {
		t.Run("invalid_"+string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			err := ValidateTransition(tc.from, tc.to)
			require.Error(t, err)
			var badTrans ErrInvalidStateTransition
			require.True(t, errors.As(err, &badTrans))
			require.Equal(t, tc.from, badTrans.From)
			require.Equal(t, tc.to, badTrans.To)
		})
	}
}

func TestContentReplacementStructFields(t *testing.T) {
	r := ContentReplacement{
		ID:                    1,
		SessionID:             "sess_abc",
		Position:              42,
		MessageID:             sql.NullString{String: "msg_123", Valid: true},
		FileID:                sql.NullString{String: "file_abc", Valid: true},
		State:                 ReplacementActive,
		Round:                 3,
		OriginalTokenCount:    1000,
		ReplacementTokenCount: 200,
		CreatedAt:             1700000000,
		UpdatedAt:             1700000001,
	}
	require.Equal(t, int64(1), r.ID)
	require.Equal(t, "sess_abc", r.SessionID)
	require.Equal(t, int64(42), r.Position)
	require.True(t, r.MessageID.Valid)
	require.Equal(t, "msg_123", r.MessageID.String)
	require.Equal(t, ReplacementActive, r.State)
	require.Equal(t, 3, r.Round)
	require.Equal(t, 1000, r.OriginalTokenCount)
	require.Equal(t, 200, r.ReplacementTokenCount)
	require.Equal(t, int64(1700000000), r.CreatedAt)
}

func TestContentReplacementNullFields(t *testing.T) {
	r := ContentReplacement{
		ID:        2,
		SessionID: "sess_def",
		Position:  10,
		State:     ReplacementPinned,
	}
	require.False(t, r.MessageID.Valid)
	require.False(t, r.FileID.Valid)
}

func TestErrorTypesImplementError(t *testing.T) {
	require.Error(t, ErrNoActiveReplacement)
	require.Error(t, ErrBudgetExceeded)

	transErr := ErrInvalidStateTransition{From: ReplacementActive, To: ReplacementActive}
	require.Error(t, transErr)
	require.Contains(t, transErr.Error(), "active")
}

func TestErrInvalidStateTransitionMessage(t *testing.T) {
	err := ErrInvalidStateTransition{From: ReplacementPinned, To: ReplacementSuperseded}
	require.Equal(t, `lcm: invalid state transition from "pinned" to "superseded"`, err.Error())
}

func TestValidateTransitionUnknownFromState(t *testing.T) {
	err := ValidateTransition(ReplacementState("unknown"), ReplacementActive)
	require.Error(t, err)
	var badTrans ErrInvalidStateTransition
	require.True(t, errors.As(err, &badTrans))
}

func TestContentReplacementStoreInterface(t *testing.T) {
	// Verify the interface exists and has the right method signatures
	// by assigning a nil typed value. This will fail to compile if the
	// interface changes.
	_ = (ContentReplacementStore)(nil)
}

func TestContentReplacementFreeze(t *testing.T) {
	tests := []struct {
		name  string
		state ReplacementState
	}{
		{"active_to_frozen", ReplacementActive},
		{"pinned_to_frozen", ReplacementPinned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := ContentReplacement{
				ID:                    1,
				SessionID:             "sess_abc",
				Position:              42,
				MessageID:             sql.NullString{String: "msg_123", Valid: true},
				FileID:                sql.NullString{String: "file_abc", Valid: true},
				State:                 tt.state,
				Round:                 3,
				OriginalTokenCount:    1000,
				ReplacementTokenCount: 200,
				CreatedAt:             1700000000,
				UpdatedAt:             1700000001,
			}
			err := cr.Freeze()
			require.NoError(t, err)
			require.Equal(t, ReplacementFrozen, cr.State)
		})
	}
}

func TestContentReplacementFreezeInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state ReplacementState
	}{
		{"frozen", ReplacementFrozen},
		{"restored", ReplacementRestored},
		{"superseded", ReplacementSuperseded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := ContentReplacement{State: tt.state}
			err := cr.Freeze()
			require.Error(t, err)
			require.Equal(t, tt.state, cr.State) // State must not change.
		})
	}
}

func TestContentReplacementClone(t *testing.T) {
	original := ContentReplacement{
		ID:                    42,
		SessionID:             "sess_clone",
		Position:              99,
		MessageID:             sql.NullString{String: "msg_xyz", Valid: true},
		FileID:                sql.NullString{String: "file_123", Valid: true},
		State:                 ReplacementPinned,
		Round:                 5,
		OriginalTokenCount:    800,
		ReplacementTokenCount: 150,
		CreatedAt:             1700000000,
		UpdatedAt:             1700000001,
	}

	clone := original.Clone()

	// Clone is a new pointer.
	require.NotSame(t, &original, clone)

	// Clone has zero ID and active state.
	require.Equal(t, int64(0), clone.ID)
	require.Equal(t, ReplacementActive, clone.State)

	// All other fields are copied.
	require.Equal(t, original.SessionID, clone.SessionID)
	require.Equal(t, original.Position, clone.Position)
	require.Equal(t, original.MessageID, clone.MessageID)
	require.Equal(t, original.FileID, clone.FileID)
	require.Equal(t, original.Round, clone.Round)
	require.Equal(t, original.OriginalTokenCount, clone.OriginalTokenCount)
	require.Equal(t, original.ReplacementTokenCount, clone.ReplacementTokenCount)
	require.Equal(t, original.CreatedAt, clone.CreatedAt)
	require.Equal(t, original.UpdatedAt, clone.UpdatedAt)
}

func TestContentReplacementCloneIndependence(t *testing.T) {
	original := ContentReplacement{
		ID:                    10,
		SessionID:             "sess_indep",
		Position:              7,
		MessageID:             sql.NullString{String: "msg_orig", Valid: true},
		FileID:                sql.NullString{String: "file_orig", Valid: true},
		State:                 ReplacementActive,
		Round:                 1,
		OriginalTokenCount:    500,
		ReplacementTokenCount: 100,
		CreatedAt:             1700000000,
		UpdatedAt:             1700000001,
	}

	clone := original.Clone()

	// Mutate the clone.
	clone.SessionID = "sess_mutated"
	clone.Position = 999
	clone.MessageID = sql.NullString{String: "msg_mutated", Valid: true}
	clone.FileID = sql.NullString{String: "file_mutated", Valid: true}
	clone.Round = 99
	clone.OriginalTokenCount = 0
	clone.ReplacementTokenCount = 0
	clone.CreatedAt = 0
	clone.UpdatedAt = 0

	// Original must remain unchanged.
	require.Equal(t, "sess_indep", original.SessionID)
	require.Equal(t, int64(7), original.Position)
	require.Equal(t, sql.NullString{String: "msg_orig", Valid: true}, original.MessageID)
	require.Equal(t, sql.NullString{String: "file_orig", Valid: true}, original.FileID)
	require.Equal(t, 1, original.Round)
	require.Equal(t, 500, original.OriginalTokenCount)
	require.Equal(t, 100, original.ReplacementTokenCount)
	require.Equal(t, int64(1700000000), original.CreatedAt)
	require.Equal(t, int64(1700000001), original.UpdatedAt)
}
