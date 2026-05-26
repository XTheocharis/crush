package rewind

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewSnapshotter(t *testing.T) {
	t.Parallel()

	s := NewSnapshotter(nil)
	require.NotNil(t, s)

	s = NewSnapshotter(nil, WithMaxPerSession(5))
	require.NotNil(t, s)
}

func TestSnapshotCapture_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	msgID := uuid.NewString()
	fileID1 := uuid.NewString()
	fileID2 := uuid.NewString()
	snapshotID := uuid.NewString()

	createdSnapshot := db.TurnSnapshot{
		ID:             snapshotID,
		SessionID:      sessionID,
		UserMessageID:  msgID,
		UserMessageSeq: 3,
		CreatedAt:      1700000000,
	}

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{ID: msgID, SessionID: sessionID, Seq: 3}, nil)
	q.On("CreateTurnSnapshot", mock.Anything, mock.MatchedBy(func(arg db.CreateTurnSnapshotParams) bool {
		return arg.SessionID == sessionID && arg.UserMessageID == msgID && arg.UserMessageSeq == 3 && arg.ID != ""
	})).Return(createdSnapshot, nil)
	q.On("ListLatestSessionFiles", mock.Anything, sessionID).
		Return([]db.File{
			{ID: fileID1, SessionID: sessionID, Path: "main.go", Version: 2, Content: "package main"},
			{ID: fileID2, SessionID: sessionID, Path: "util.go", Version: 1, Content: "package util"},
		}, nil)
	q.On("AddSnapshotFile", mock.Anything, mock.MatchedBy(func(arg db.AddSnapshotFileParams) bool {
		return arg.SnapshotID == snapshotID
	})).Return(nil)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 3)
	require.NoError(t, err)

	q.AssertExpectations(t)
}

func TestSnapshotCapture_GetLatestUserMessageError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{}, sql.ErrNoRows)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "getting latest user message")

	q.AssertExpectations(t)
}

func TestSnapshotCapture_CreateSnapshotError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	msgID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{ID: msgID, SessionID: sessionID}, nil)
	q.On("CreateTurnSnapshot", mock.Anything, mock.Anything).
		Return(db.TurnSnapshot{}, sql.ErrTxDone)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "creating turn snapshot")

	q.AssertExpectations(t)
}

func TestSnapshotCapture_ListFilesError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	msgID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{ID: msgID, SessionID: sessionID}, nil)
	q.On("CreateTurnSnapshot", mock.Anything, mock.Anything).
		Return(db.TurnSnapshot{ID: "snap1"}, nil)
	q.On("ListLatestSessionFiles", mock.Anything, sessionID).
		Return([]db.File(nil), sql.ErrConnDone)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "listing session files")

	q.AssertExpectations(t)
}

func TestSnapshotCapture_AddFileError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	msgID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{ID: msgID, SessionID: sessionID}, nil)
	q.On("CreateTurnSnapshot", mock.Anything, mock.Anything).
		Return(db.TurnSnapshot{ID: "snap1"}, nil)
	q.On("ListLatestSessionFiles", mock.Anything, sessionID).
		Return([]db.File{{ID: "f1", Path: "broken.go", Version: 1}}, nil)
	q.On("AddSnapshotFile", mock.Anything, mock.Anything).
		Return(sql.ErrTxDone)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "adding snapshot file broken.go")

	q.AssertExpectations(t)
}

func TestSnapshotCapture_NoFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	msgID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetLatestUserMessage", mock.Anything, sessionID).
		Return(db.Message{ID: msgID, SessionID: sessionID}, nil)
	q.On("CreateTurnSnapshot", mock.Anything, mock.Anything).
		Return(db.TurnSnapshot{ID: "snap1"}, nil)
	q.On("ListLatestSessionFiles", mock.Anything, sessionID).
		Return([]db.File{}, nil)

	s := NewSnapshotter(q)
	err := s.CaptureSnapshot(ctx, sessionID, 1)
	require.NoError(t, err)

	q.AssertNotCalled(t, "AddSnapshotFile", mock.Anything, mock.Anything)
}

func TestGetSnapshotAtOrBeforeSeq_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()
	snapshotID := uuid.NewString()
	msgID := uuid.NewString()

	dbSnap := db.TurnSnapshot{
		ID:             snapshotID,
		SessionID:      sessionID,
		UserMessageID:  msgID,
		UserMessageSeq: 5,
		CreatedAt:      1700000000,
	}

	q := new(mockQuerier)
	q.On("GetTurnSnapshotAtOrBeforeSeq", mock.Anything, mock.MatchedBy(func(arg db.GetTurnSnapshotAtOrBeforeSeqParams) bool {
		return arg.SessionID == sessionID && arg.UserMessageSeq == 7
	})).Return(dbSnap, nil)

	s := NewSnapshotter(q)
	snap, err := s.GetSnapshotAtOrBeforeSeq(ctx, sessionID, 7)
	require.NoError(t, err)
	require.Equal(t, snapshotID, snap.ID)
	require.Equal(t, sessionID, snap.SessionID)
	require.Equal(t, msgID, snap.UserMessageID)
	require.Equal(t, 5, snap.UserMessageSeq)
	require.Equal(t, time.Unix(1700000000, 0), snap.CreatedAt)

	q.AssertExpectations(t)
}

func TestGetSnapshotAtOrBeforeSeq_NotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()

	q := new(mockQuerier)
	q.On("GetTurnSnapshotAtOrBeforeSeq", mock.Anything, mock.Anything).
		Return(db.TurnSnapshot{}, sql.ErrNoRows)

	s := NewSnapshotter(q)
	snap, err := s.GetSnapshotAtOrBeforeSeq(ctx, sessionID, 99)
	require.Error(t, err)
	require.Nil(t, snap)

	q.AssertExpectations(t)
}

func TestGetSnapshotFiles_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	snapshotID := uuid.NewString()

	q := new(mockQuerier)
	q.On("ListSnapshotFiles", mock.Anything, snapshotID).
		Return([]db.ListSnapshotFilesRow{
			{SnapshotID: snapshotID, FileID: "f1", Path: "a.go", Version: 2, Content: "package a"},
			{SnapshotID: snapshotID, FileID: "f2", Path: "b.go", Version: 1, Content: "package b"},
		}, nil)

	s := NewSnapshotter(q)
	files, err := s.GetSnapshotFiles(ctx, snapshotID)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "f1", files[0].FileID)
	require.Equal(t, "a.go", files[0].Path)
	require.Equal(t, "2", files[0].Version)
	require.Equal(t, "package a", files[0].Content)
	require.Equal(t, "f2", files[1].FileID)
	require.Equal(t, "b.go", files[1].Path)
	require.Equal(t, "1", files[1].Version)
	require.Equal(t, "package b", files[1].Content)

	q.AssertExpectations(t)
}

func TestGetSnapshotFiles_Empty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	snapshotID := uuid.NewString()

	q := new(mockQuerier)
	q.On("ListSnapshotFiles", mock.Anything, snapshotID).
		Return([]db.ListSnapshotFilesRow(nil), nil)

	s := NewSnapshotter(q)
	files, err := s.GetSnapshotFiles(ctx, snapshotID)
	require.NoError(t, err)
	require.Empty(t, files)

	q.AssertExpectations(t)
}

func TestGetSnapshotFiles_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	q := new(mockQuerier)
	q.On("ListSnapshotFiles", mock.Anything, "snap").
		Return([]db.ListSnapshotFilesRow(nil), sql.ErrConnDone)

	s := NewSnapshotter(q)
	files, err := s.GetSnapshotFiles(ctx, "snap")
	require.Error(t, err)
	require.Nil(t, files)
	require.Contains(t, err.Error(), "listing snapshot files")

	q.AssertExpectations(t)
}

func TestDeleteSnapshotsAfterSeq_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()

	q := new(mockQuerier)
	q.On("DeleteSnapshotsAfterSeq", mock.Anything, mock.MatchedBy(func(arg db.DeleteSnapshotsAfterSeqParams) bool {
		return arg.SessionID == sessionID && arg.UserMessageSeq == 5
	})).Return(nil)

	s := NewSnapshotter(q)
	err := s.DeleteSnapshotsAfterSeq(ctx, sessionID, 5)
	require.NoError(t, err)

	q.AssertExpectations(t)
}

func TestDeleteSnapshotsAfterSeq_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	q := new(mockQuerier)
	q.On("DeleteSnapshotsAfterSeq", mock.Anything, mock.Anything).
		Return(sql.ErrTxDone)

	s := NewSnapshotter(q)
	err := s.DeleteSnapshotsAfterSeq(ctx, "session", 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deleting snapshots after seq")

	q.AssertExpectations(t)
}

func TestCleanupOldSnapshots_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()

	q := new(mockQuerier)
	q.On("DeleteOldTurnSnapshots", mock.Anything, mock.MatchedBy(func(arg db.DeleteOldTurnSnapshotsParams) bool {
		return arg.SessionID == sessionID && arg.Column2 == int64(3) && arg.SessionID_2 == sessionID
	})).Return(int64(2), nil)

	s := NewSnapshotter(q, WithMaxPerSession(3))
	err := s.CleanupOldSnapshots(ctx, sessionID)
	require.NoError(t, err)

	q.AssertExpectations(t)
}

func TestCleanupOldSnapshots_DefaultMax(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := uuid.NewString()

	q := new(mockQuerier)
	q.On("DeleteOldTurnSnapshots", mock.Anything, mock.MatchedBy(func(arg db.DeleteOldTurnSnapshotsParams) bool {
		return arg.Column2 == int64(50)
	})).Return(int64(0), nil)

	s := NewSnapshotter(q)
	err := s.CleanupOldSnapshots(ctx, sessionID)
	require.NoError(t, err)

	q.AssertExpectations(t)
}

func TestCleanupOldSnapshots_Error(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	q := new(mockQuerier)
	q.On("DeleteOldTurnSnapshots", mock.Anything, mock.Anything).
		Return(int64(0), sql.ErrTxDone)

	s := NewSnapshotter(q)
	err := s.CleanupOldSnapshots(ctx, "session")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cleaning up old snapshots")

	q.AssertExpectations(t)
}
