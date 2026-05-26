package rewind

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockSnapshotter struct {
	mock.Mock
}

func (m *mockSnapshotter) CaptureSnapshot(ctx context.Context, sessionID string, userMessageSeq int) error {
	args := m.Called(ctx, sessionID, userMessageSeq)
	return args.Error(0)
}

func (m *mockSnapshotter) GetSnapshotAtOrBeforeSeq(ctx context.Context, sessionID string, seq int) (*TurnSnapshot, error) {
	args := m.Called(ctx, sessionID, seq)
	if v := args.Get(0); v != nil {
		return v.(*TurnSnapshot), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSnapshotter) GetSnapshotFiles(ctx context.Context, snapshotID string) ([]SnapshotFile, error) {
	args := m.Called(ctx, snapshotID)
	if v := args.Get(0); v != nil {
		return v.([]SnapshotFile), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockSnapshotter) DeleteSnapshotsAfterSeq(ctx context.Context, sessionID string, seq int) error {
	args := m.Called(ctx, sessionID, seq)
	return args.Error(0)
}

func (m *mockSnapshotter) CleanupOldSnapshots(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func TestRewind_CodeOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	snap := &TurnSnapshot{
		ID:             "snap-1",
		SessionID:      "sess-1",
		UserMessageSeq: 5,
		CreatedAt:      time.Now(),
	}
	files := []SnapshotFile{
		{FileID: "f1", Path: "foo.txt", Content: "hello world"},
		{FileID: "f2", Path: "sub/bar.txt", Content: "nested content"},
	}

	q := new(mockQuerier)
	s := new(mockSnapshotter)

	s.On("GetSnapshotAtOrBeforeSeq", mock.Anything, "sess-1", 5).
		Return(snap, nil)
	s.On("GetSnapshotFiles", mock.Anything, "snap-1").
		Return(files, nil)

	r := NewRewinder(q, s, tmpDir)
	result, err := r.Rewind(context.Background(), "sess-1", 5, RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 2, result.FilesRestored)
	require.Equal(t, snap, result.Snapshot)
	require.Equal(t, 0, result.MessagesDeleted)

	content1, err := os.ReadFile(filepath.Join(tmpDir, "foo.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello world", string(content1))

	content2, err := os.ReadFile(filepath.Join(tmpDir, "sub/bar.txt"))
	require.NoError(t, err)
	require.Equal(t, "nested content", string(content2))

	s.AssertExpectations(t)
}

func TestRewind_ConvoOnly(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(5),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(5),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(4),
	}).Return(nil)
	s := new(mockSnapshotter)

	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 4).
		Return(nil)

	r := NewRewinder(q, s, t.TempDir())
	result, err := r.Rewind(context.Background(), "sess-1", 5, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)
	require.Equal(t, 0, result.FilesRestored)
	require.Nil(t, result.Snapshot)

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRewind_Both(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	snap := &TurnSnapshot{
		ID:             "snap-1",
		SessionID:      "sess-1",
		UserMessageSeq: 3,
		CreatedAt:      time.Now(),
	}
	files := []SnapshotFile{
		{FileID: "f1", Path: "a.txt", Content: "restored"},
	}

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(3),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(3),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(2),
	}).Return(nil)
	s := new(mockSnapshotter)

	s.On("GetSnapshotAtOrBeforeSeq", mock.Anything, "sess-1", 3).
		Return(snap, nil)
	s.On("GetSnapshotFiles", mock.Anything, "snap-1").
		Return(files, nil)
	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 2).
		Return(nil)

	r := NewRewinder(q, s, tmpDir)
	result, err := r.Rewind(context.Background(), "sess-1", 3, RewindBoth)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)
	require.Equal(t, snap, result.Snapshot)

	content, err := os.ReadFile(filepath.Join(tmpDir, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "restored", string(content))

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRewind_CodeOnly_SnapshotNotFound(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	s := new(mockSnapshotter)

	s.On("GetSnapshotAtOrBeforeSeq", mock.Anything, "sess-1", 99).
		Return(nil, os.ErrNotExist)

	r := NewRewinder(q, s, t.TempDir())
	_, err := r.Rewind(context.Background(), "sess-1", 99, RewindCodeOnly)
	require.Error(t, err)

	s.AssertExpectations(t)
}

func TestRewind_ConvoOnly_DeleteMessagesError(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(5),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(5),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(4),
	}).Return(os.ErrPermission)
	s := new(mockSnapshotter)

	r := NewRewinder(q, s, t.TempDir())
	_, err := r.Rewind(context.Background(), "sess-1", 5, RewindConvoOnly)
	require.Error(t, err)

	q.AssertExpectations(t)
}

func TestRewind_CodeOnly_EmptySnapshot(t *testing.T) {
	t.Parallel()

	snap := &TurnSnapshot{
		ID:             "snap-2",
		SessionID:      "sess-1",
		UserMessageSeq: 1,
		CreatedAt:      time.Now(),
	}

	q := new(mockQuerier)
	s := new(mockSnapshotter)

	s.On("GetSnapshotAtOrBeforeSeq", mock.Anything, "sess-1", 1).
		Return(snap, nil)
	s.On("GetSnapshotFiles", mock.Anything, "snap-2").
		Return([]SnapshotFile{}, nil)

	r := NewRewinder(q, s, t.TempDir())
	result, err := r.Rewind(context.Background(), "sess-1", 1, RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 0, result.FilesRestored)
	require.Equal(t, snap, result.Snapshot)

	s.AssertExpectations(t)
}

func TestRewind_UnknownMode(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	s := new(mockSnapshotter)

	r := NewRewinder(q, s, t.TempDir())
	_, err := r.Rewind(context.Background(), "sess-1", 5, RewindMode(99))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown rewind mode")
}

func TestNewRewinder_ReturnsInterface(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	s := new(mockSnapshotter)

	r := NewRewinder(q, s, t.TempDir())
	require.NotNil(t, r)
}

func TestRewindConvo_WithPostRewindHook(t *testing.T) {
	t.Parallel()

	var hookSessionID string
	hook := func(ctx context.Context, sessionID string) error {
		hookSessionID = sessionID
		return nil
	}

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(5),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(5),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(4),
	}).Return(nil)
	s := new(mockSnapshotter)
	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 4).
		Return(nil)

	r := NewRewinder(q, s, t.TempDir(), WithPostRewindHook(hook))
	result, err := r.Rewind(context.Background(), "sess-1", 5, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)
	require.Equal(t, "sess-1", hookSessionID)

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRewindConvo_PostRewindHookError(t *testing.T) {
	t.Parallel()

	hook := func(ctx context.Context, sessionID string) error {
		return fmt.Errorf("hook failed")
	}

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(5),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(5),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(4),
	}).Return(nil)
	s := new(mockSnapshotter)
	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 4).
		Return(nil)

	r := NewRewinder(q, s, t.TempDir(), WithPostRewindHook(hook))
	result, err := r.Rewind(context.Background(), "sess-1", 5, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRewindBoth_WithPostRewindHook(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	snap := &TurnSnapshot{
		ID:             "snap-1",
		SessionID:      "sess-1",
		UserMessageSeq: 3,
		CreatedAt:      time.Now(),
	}
	files := []SnapshotFile{
		{FileID: "f1", Path: "a.txt", Content: "restored"},
	}

	var hookCalled bool
	hook := func(ctx context.Context, sessionID string) error {
		hookCalled = true
		return nil
	}

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(3),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(3),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(2),
	}).Return(nil)
	s := new(mockSnapshotter)
	s.On("GetSnapshotAtOrBeforeSeq", mock.Anything, "sess-1", 3).
		Return(snap, nil)
	s.On("GetSnapshotFiles", mock.Anything, "snap-1").
		Return(files, nil)
	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 2).
		Return(nil)

	r := NewRewinder(q, s, tmpDir, WithPostRewindHook(hook))
	result, err := r.Rewind(context.Background(), "sess-1", 3, RewindBoth)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)
	require.True(t, hookCalled, "post rewind hook should be called in rewindBoth")

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}

func TestRewindConvo_NoHook(t *testing.T) {
	t.Parallel()

	q := new(mockQuerier)
	q.On("GetMessageBySessionAndSeq", mock.Anything, db.GetMessageBySessionAndSeqParams{
		SessionID: "sess-1",
		Seq:       int64(5),
	}).Return(db.Message{
		ID:    "msg-1",
		Role:  "user",
		Parts: `[{"type":"text","data":{"text":"hello"}}]`,
	}, nil)
	q.On("ListMessagesInSeqRange", mock.Anything, db.ListMessagesInSeqRangeParams{
		SessionID: "sess-1",
		Seq:       int64(5),
		Seq_2:     int64(999999),
	}).Return([]db.Message{{ID: "msg-1"}, {ID: "msg-2"}}, nil)
	q.On("DeleteMessagesAfterSeq", mock.Anything, db.DeleteMessagesAfterSeqParams{
		SessionID: "sess-1",
		Seq:       int64(4),
	}).Return(nil)
	s := new(mockSnapshotter)
	s.On("DeleteSnapshotsAfterSeq", mock.Anything, "sess-1", 4).
		Return(nil)

	r := NewRewinder(q, s, t.TempDir())
	result, err := r.Rewind(context.Background(), "sess-1", 5, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, 2, result.MessagesDeleted)
	require.Equal(t, "hello", result.ExtractedText)

	q.AssertExpectations(t)
	s.AssertExpectations(t)
}
