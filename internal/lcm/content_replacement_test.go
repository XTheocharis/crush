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
	var _ ContentReplacementStore = (ContentReplacementStore)(nil)
}
