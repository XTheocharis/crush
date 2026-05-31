package rewind

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// editMockQuerier satisfies db.Querier for edit tests.
type editMockQuerier struct {
	mock.Mock
}

func (m *editMockQuerier) GetMessageBySessionAndSeq(ctx context.Context, arg db.GetMessageBySessionAndSeqParams) (db.Message, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).(db.Message), args.Error(1)
}

func (m *editMockQuerier) DeleteMessagesAfterSeq(ctx context.Context, arg db.DeleteMessagesAfterSeqParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *editMockQuerier) ListMessagesInSeqRange(ctx context.Context, arg db.ListMessagesInSeqRangeParams) ([]db.Message, error) {
	args := m.Called(ctx, arg)
	return args.Get(0).([]db.Message), args.Error(1)
}

// Stub out remaining Querier methods so the mock compiles.
func (m *editMockQuerier) AddSnapshotFile(ctx context.Context, arg db.AddSnapshotFileParams) error {
	return nil
}

func (m *editMockQuerier) AppendLcmContextItem(ctx context.Context, arg db.AppendLcmContextItemParams) error {
	return nil
}

func (m *editMockQuerier) ClearSessionSummaryMessageID(ctx context.Context, id string) error {
	return nil
}

func (m *editMockQuerier) CloneSessionFiles(ctx context.Context, arg db.CloneSessionFilesParams) error {
	return nil
}

func (m *editMockQuerier) CloneSessionMessages(ctx context.Context, arg db.CloneSessionMessagesParams) error {
	return nil
}

func (m *editMockQuerier) CountTurnSnapshots(ctx context.Context, sessionID string) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) {
	return db.File{}, nil
}

func (m *editMockQuerier) CreateMessage(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) {
	return db.Message{}, nil
}

func (m *editMockQuerier) CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error) {
	return db.Session{}, nil
}

func (m *editMockQuerier) CreateTurnSnapshot(ctx context.Context, arg db.CreateTurnSnapshotParams) (db.TurnSnapshot, error) {
	return db.TurnSnapshot{}, nil
}

func (m *editMockQuerier) DeleteAllLcmContextItems(ctx context.Context, sessionID string) error {
	return nil
}
func (m *editMockQuerier) DeleteFile(ctx context.Context, id string) error       { return nil }
func (m *editMockQuerier) DeleteLcmSummary(ctx context.Context, id string) error { return nil }
func (m *editMockQuerier) DeleteLcmSummaryMessages(ctx context.Context, id string) error {
	return nil
}

func (m *editMockQuerier) DeleteLcmSummaryParents(ctx context.Context, id string) error {
	return nil
}
func (m *editMockQuerier) DeleteMessage(ctx context.Context, id string) error { return nil }
func (m *editMockQuerier) DeleteOldTurnSnapshots(ctx context.Context, arg db.DeleteOldTurnSnapshotsParams) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) DeleteRepoMapFileCache(ctx context.Context, arg db.DeleteRepoMapFileCacheParams) error {
	return nil
}

func (m *editMockQuerier) DeleteRepoMapTagsByPath(ctx context.Context, arg db.DeleteRepoMapTagsByPathParams) error {
	return nil
}
func (m *editMockQuerier) DeleteSession(ctx context.Context, id string) error      { return nil }
func (m *editMockQuerier) DeleteSessionFiles(ctx context.Context, id string) error { return nil }
func (m *editMockQuerier) DeleteSessionMessages(ctx context.Context, id string) error {
	return nil
}

func (m *editMockQuerier) DeleteSessionRankings(ctx context.Context, arg db.DeleteSessionRankingsParams) error {
	return nil
}

func (m *editMockQuerier) DeleteSessionReadOnlyPaths(ctx context.Context, arg db.DeleteSessionReadOnlyPathsParams) error {
	return nil
}

func (m *editMockQuerier) DeleteSessionTurnSnapshots(ctx context.Context, id string) error {
	return nil
}
func (m *editMockQuerier) DeleteSnapshotFiles(ctx context.Context, id string) error { return nil }
func (m *editMockQuerier) DeleteSnapshotsAfterSeq(ctx context.Context, arg db.DeleteSnapshotsAfterSeqParams) error {
	return nil
}
func (m *editMockQuerier) DeleteTurnSnapshot(ctx context.Context, id string) error { return nil }
func (m *editMockQuerier) GetAverageResponseTime(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) GetContentReplacement(ctx context.Context, id int64) (db.LcmContentReplacement, error) {
	return db.LcmContentReplacement{}, nil
}

func (m *editMockQuerier) GetContentReplacementsByFileID(ctx context.Context, arg db.GetContentReplacementsByFileIDParams) ([]db.LcmContentReplacement, error) {
	return nil, nil
}

func (m *editMockQuerier) GetContentReplacementsBySessionPosition(ctx context.Context, arg db.GetContentReplacementsBySessionPositionParams) ([]db.LcmContentReplacement, error) {
	return nil, nil
}

func (m *editMockQuerier) GetFile(ctx context.Context, id string) (db.File, error) {
	return db.File{}, nil
}

func (m *editMockQuerier) GetFileByPathAndSession(ctx context.Context, arg db.GetFileByPathAndSessionParams) (db.File, error) {
	return db.File{}, nil
}

func (m *editMockQuerier) GetFileRead(ctx context.Context, arg db.GetFileReadParams) (db.ReadFile, error) {
	return db.ReadFile{}, nil
}

func (m *editMockQuerier) GetFileWrite(ctx context.Context, arg db.GetFileWriteParams) (db.WrittenFile, error) {
	return db.WrittenFile{}, nil
}

func (m *editMockQuerier) GetHourDayHeatmap(ctx context.Context) ([]db.GetHourDayHeatmapRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetLastSession(ctx context.Context) (db.Session, error) {
	return db.Session{}, nil
}

func (m *editMockQuerier) GetLatestTurnSnapshot(ctx context.Context, id string) (db.TurnSnapshot, error) {
	return db.TurnSnapshot{}, nil
}

func (m *editMockQuerier) GetLatestUserMessage(ctx context.Context, id string) (db.Message, error) {
	return db.Message{}, nil
}

func (m *editMockQuerier) GetLcmContextTokenCount(ctx context.Context, id string) (any, error) {
	return nil, nil
}

func (m *editMockQuerier) GetLcmLargeFile(ctx context.Context, id string) (db.LcmLargeFile, error) {
	return db.LcmLargeFile{}, nil
}

func (m *editMockQuerier) GetLcmSessionConfig(ctx context.Context, id string) (db.LcmSessionConfig, error) {
	return db.LcmSessionConfig{}, nil
}

func (m *editMockQuerier) GetLcmSummary(ctx context.Context, id string) (db.LcmSummary, error) {
	return db.LcmSummary{}, nil
}

func (m *editMockQuerier) GetMessage(ctx context.Context, id string) (db.Message, error) {
	return db.Message{}, nil
}

func (m *editMockQuerier) GetMessageCountByTimeRange(ctx context.Context, arg db.GetMessageCountByTimeRangeParams) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) GetMessagesByTimeRange(ctx context.Context, arg db.GetMessagesByTimeRangeParams) ([]db.Message, error) {
	return nil, nil
}

func (m *editMockQuerier) GetRecentActivity(ctx context.Context) ([]db.GetRecentActivityRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetRepoMapFileCache(ctx context.Context, id string) ([]db.RepoMapFileCache, error) {
	return nil, nil
}

func (m *editMockQuerier) GetRepoMapFileCacheByPath(ctx context.Context, arg db.GetRepoMapFileCacheByPathParams) (db.RepoMapFileCache, error) {
	return db.RepoMapFileCache{}, nil
}

func (m *editMockQuerier) GetSessionByID(ctx context.Context, id string) (db.Session, error) {
	return db.Session{}, nil
}

func (m *editMockQuerier) GetToolUsage(ctx context.Context) ([]db.GetToolUsageRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetTotalStats(ctx context.Context) (db.GetTotalStatsRow, error) {
	return db.GetTotalStatsRow{}, nil
}

func (m *editMockQuerier) GetTurnSnapshot(ctx context.Context, id string) (db.TurnSnapshot, error) {
	return db.TurnSnapshot{}, nil
}

func (m *editMockQuerier) GetTurnSnapshotAtOrBeforeSeq(ctx context.Context, arg db.GetTurnSnapshotAtOrBeforeSeqParams) (db.TurnSnapshot, error) {
	return db.TurnSnapshot{}, nil
}

func (m *editMockQuerier) GetTurnSnapshotByMessage(ctx context.Context, arg db.GetTurnSnapshotByMessageParams) (db.TurnSnapshot, error) {
	return db.TurnSnapshot{}, nil
}

func (m *editMockQuerier) GetUsageByDay(ctx context.Context) ([]db.GetUsageByDayRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetUsageByDayOfWeek(ctx context.Context) ([]db.GetUsageByDayOfWeekRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetUsageByHour(ctx context.Context) ([]db.GetUsageByHourRow, error) {
	return nil, nil
}

func (m *editMockQuerier) GetUsageByModel(ctx context.Context) ([]db.GetUsageByModelRow, error) {
	return nil, nil
}

func (m *editMockQuerier) InsertLcmContextItem(ctx context.Context, arg db.InsertLcmContextItemParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmLargeFile(ctx context.Context, arg db.InsertLcmLargeFileParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmMapItem(ctx context.Context, arg db.InsertLcmMapItemParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmMapRun(ctx context.Context, arg db.InsertLcmMapRunParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmSummary(ctx context.Context, arg db.InsertLcmSummaryParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmSummaryMessage(ctx context.Context, arg db.InsertLcmSummaryMessageParams) error {
	return nil
}

func (m *editMockQuerier) InsertLcmSummaryParent(ctx context.Context, arg db.InsertLcmSummaryParentParams) error {
	return nil
}

func (m *editMockQuerier) InsertRepoMapTag(ctx context.Context, arg db.InsertRepoMapTagParams) error {
	return nil
}

func (m *editMockQuerier) ListAllUserMessages(ctx context.Context) ([]db.Message, error) {
	return nil, nil
}

func (m *editMockQuerier) ListContentReplacementsByRound(ctx context.Context, arg db.ListContentReplacementsByRoundParams) ([]db.LcmContentReplacement, error) {
	return nil, nil
}

func (m *editMockQuerier) ListContentReplacementsByState(ctx context.Context, arg db.ListContentReplacementsByStateParams) ([]db.LcmContentReplacement, error) {
	return nil, nil
}

func (m *editMockQuerier) ListFilesByPath(ctx context.Context, id string) ([]db.File, error) {
	return nil, nil
}

func (m *editMockQuerier) ListFilesBySession(ctx context.Context, id string) ([]db.File, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLatestSessionFiles(ctx context.Context, id string) ([]db.File, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLcmContextItems(ctx context.Context, id string) ([]db.LcmContextItem, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLcmLargeFilesBySession(ctx context.Context, id string) ([]db.LcmLargeFile, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLcmSummariesBySession(ctx context.Context, id string) ([]db.LcmSummary, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLcmSummaryMessages(ctx context.Context, id string) ([]db.LcmSummaryMessage, error) {
	return nil, nil
}

func (m *editMockQuerier) ListLcmSummaryParents(ctx context.Context, id string) ([]db.LcmSummaryParent, error) {
	return nil, nil
}

func (m *editMockQuerier) ListMessagesBySession(ctx context.Context, id string) ([]db.Message, error) {
	return nil, nil
}

func (m *editMockQuerier) ListMessagesBySessionSeq(ctx context.Context, id string) ([]db.Message, error) {
	return nil, nil
}

func (m *editMockQuerier) ListNewFiles(ctx context.Context) ([]db.File, error) {
	return nil, nil
}

func (m *editMockQuerier) ListRecentSessionReadFiles(ctx context.Context, arg db.ListRecentSessionReadFilesParams) ([]db.ReadFile, error) {
	return nil, nil
}

func (m *editMockQuerier) ListRepoMapDefsByName(ctx context.Context, arg db.ListRepoMapDefsByNameParams) ([]db.ListRepoMapDefsByNameRow, error) {
	return nil, nil
}

func (m *editMockQuerier) ListRepoMapTags(ctx context.Context, id string) ([]db.ListRepoMapTagsRow, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSessionRankings(ctx context.Context, arg db.ListSessionRankingsParams) ([]db.RepoMapSessionRanking, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSessionReadFiles(ctx context.Context, id string) ([]db.ReadFile, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSessionReadOnlyPaths(ctx context.Context, arg db.ListSessionReadOnlyPathsParams) ([]string, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSessionWrittenFiles(ctx context.Context, id string) ([]db.WrittenFile, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSessions(ctx context.Context) ([]db.Session, error) {
	return nil, nil
}

func (m *editMockQuerier) ListSnapshotFiles(ctx context.Context, id string) ([]db.ListSnapshotFilesRow, error) {
	return nil, nil
}

func (m *editMockQuerier) ListTurnSnapshotsBySession(ctx context.Context, id string) ([]db.TurnSnapshot, error) {
	return nil, nil
}

func (m *editMockQuerier) ListUserMessagesBySession(ctx context.Context, id string) ([]db.Message, error) {
	return nil, nil
}

func (m *editMockQuerier) RecordContentReplacement(ctx context.Context, arg db.RecordContentReplacementParams) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) RecordFileRead(ctx context.Context, arg db.RecordFileReadParams) error {
	return nil
}

func (m *editMockQuerier) RecordFileWrite(ctx context.Context, arg db.RecordFileWriteParams) error {
	return nil
}

func (m *editMockQuerier) RenameSession(ctx context.Context, arg db.RenameSessionParams) error {
	return nil
}

func (m *editMockQuerier) SearchLcmSummaries(ctx context.Context, arg db.SearchLcmSummariesParams) ([]db.SearchLcmSummariesRow, error) {
	return nil, nil
}

func (m *editMockQuerier) UpdateContentReplacementState(ctx context.Context, arg db.UpdateContentReplacementStateParams) error {
	return nil
}

func (m *editMockQuerier) UpdateLcmLargeFileExploration(ctx context.Context, arg db.UpdateLcmLargeFileExplorationParams) error {
	return nil
}

func (m *editMockQuerier) UpdateLcmMapItem(ctx context.Context, arg db.UpdateLcmMapItemParams) error {
	return nil
}

func (m *editMockQuerier) UpdateLcmMapRunStatus(ctx context.Context, arg db.UpdateLcmMapRunStatusParams) error {
	return nil
}

func (m *editMockQuerier) UpdateLcmSessionConfig(ctx context.Context, arg db.UpdateLcmSessionConfigParams) error {
	return nil
}

func (m *editMockQuerier) UpdateMessage(ctx context.Context, arg db.UpdateMessageParams) error {
	return nil
}

func (m *editMockQuerier) UpdateMessageTokenCount(ctx context.Context, arg db.UpdateMessageTokenCountParams) error {
	return nil
}

func (m *editMockQuerier) UpdateSession(ctx context.Context, arg db.UpdateSessionParams) (db.Session, error) {
	return db.Session{}, nil
}

func (m *editMockQuerier) UpdateSessionTitleAndUsage(ctx context.Context, arg db.UpdateSessionTitleAndUsageParams) error {
	return nil
}

func (m *editMockQuerier) UpsertLcmSessionConfig(ctx context.Context, arg db.UpsertLcmSessionConfigParams) error {
	return nil
}

func (m *editMockQuerier) UpsertRepoMapFileCache(ctx context.Context, arg db.UpsertRepoMapFileCacheParams) error {
	return nil
}

func (m *editMockQuerier) UpsertSessionRanking(ctx context.Context, arg db.UpsertSessionRankingParams) error {
	return nil
}

func (m *editMockQuerier) UpsertSessionReadOnlyPath(ctx context.Context, arg db.UpsertSessionReadOnlyPathParams) error {
	return nil
}

func (m *editMockQuerier) CountMessagePartsBySession(ctx context.Context, sessionID string) (int64, error) {
	return 0, nil
}

func (m *editMockQuerier) DeleteMessagePartsByMessageID(ctx context.Context, messageID string) error {
	return nil
}

func (m *editMockQuerier) GetMapRun(ctx context.Context, runID string) (db.LcmMapRun, error) {
	return db.LcmMapRun{}, nil
}

func (m *editMockQuerier) GetMapRunItems(ctx context.Context, runID string) ([]db.LcmMapItem, error) {
	return nil, nil
}

func (m *editMockQuerier) GetMessagePartsByMessageID(ctx context.Context, messageID string) ([]db.MessagePart, error) {
	return nil, nil
}

func (m *editMockQuerier) GetMessagePartsBySessionAndType(ctx context.Context, arg db.GetMessagePartsBySessionAndTypeParams) ([]db.MessagePart, error) {
	return nil, nil
}

func (m *editMockQuerier) InsertMapRun(ctx context.Context, arg db.InsertMapRunParams) error {
	return nil
}

func (m *editMockQuerier) InsertMessagePart(ctx context.Context, arg db.InsertMessagePartParams) (db.MessagePart, error) {
	return db.MessagePart{}, nil
}

func (m *editMockQuerier) UpdateMapRunStatus(ctx context.Context, arg db.UpdateMapRunStatusParams) error {
	return nil
}

// XRUSH: mock for ListRecentReadFiles
func (m *editMockQuerier) ListRecentReadFiles(ctx context.Context, readAt int64) ([]db.ReadFile, error) {
	return nil, nil
}

// marshalTextParts builds the JSON parts string that the message package
// produces for text-only content.
func marshalTextParts(t *testing.T, texts ...string) string {
	t.Helper()
	wrappers := make([]map[string]any, 0, len(texts)+1)
	for _, txt := range texts {
		wrappers = append(wrappers, map[string]any{
			"type": "text",
			"data": map[string]string{"text": txt},
		})
	}
	// Messages created via message.Service always append a Finish part.
	wrappers = append(wrappers, map[string]any{
		"type": "finish",
		"data": map[string]any{
			"reason":  "stop",
			"time":    float64(0),
			"message": "",
			"details": "",
		},
	})
	b, err := json.Marshal(wrappers)
	require.NoError(t, err)
	return string(b)
}

func TestEditMessage_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-1"
	seq := 3
	msgID := "msg-abc"

	parts := marshalTextParts(t, "hello world")

	mq := new(editMockQuerier)

	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{
		ID:        msgID,
		SessionID: sessionID,
		Role:      "user",
		Parts:     parts,
		Seq:       int64(seq),
	}, nil)

	mq.On("ListMessagesInSeqRange", ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     999999,
	}).Return([]db.Message{
		{ID: "msg-abc", Seq: 3},
		{ID: "msg-def", Seq: 4},
		{ID: "msg-ghi", Seq: 5},
	}, nil)

	mq.On("DeleteMessagesAfterSeq", ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	}).Return(nil)

	editor := NewEditor(mq)
	result, err := editor.EditMessage(ctx, sessionID, seq)
	require.NoError(t, err)
	require.Equal(t, "hello world", result.ExtractedText)
	require.Equal(t, 3, result.MessagesDeleted)
	require.Equal(t, msgID, result.NewMessageID)
	mq.AssertExpectations(t)
}

func TestEditMessage_MultipleTextParts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-2"
	seq := 1
	msgID := "msg-multi"

	parts := marshalTextParts(t, "part one ", "part two")

	mq := new(editMockQuerier)
	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{
		ID:        msgID,
		SessionID: sessionID,
		Role:      "user",
		Parts:     parts,
		Seq:       int64(seq),
	}, nil)

	mq.On("ListMessagesInSeqRange", ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     999999,
	}).Return([]db.Message{{ID: msgID, Seq: 1}}, nil)

	mq.On("DeleteMessagesAfterSeq", ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	}).Return(nil)

	editor := NewEditor(mq)
	result, err := editor.EditMessage(ctx, sessionID, seq)
	require.NoError(t, err)
	require.Equal(t, "part one part two", result.ExtractedText)
	require.Equal(t, 1, result.MessagesDeleted)
	require.Equal(t, msgID, result.NewMessageID)
}

func TestEditMessage_NoTextContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-3"
	seq := 2
	msgID := "msg-notext"

	wrappers := []map[string]any{
		{
			"type": "finish",
			"data": map[string]any{"reason": "stop", "time": float64(0)},
		},
	}
	parts, err := json.Marshal(wrappers)
	require.NoError(t, err)

	mq := new(editMockQuerier)
	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{
		ID:        msgID,
		SessionID: sessionID,
		Role:      "user",
		Parts:     string(parts),
		Seq:       int64(seq),
	}, nil)

	mq.On("ListMessagesInSeqRange", ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     999999,
	}).Return([]db.Message{{ID: msgID, Seq: 2}}, nil)

	mq.On("DeleteMessagesAfterSeq", ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	}).Return(nil)

	editor := NewEditor(mq)
	result, err := editor.EditMessage(ctx, sessionID, seq)
	require.NoError(t, err)
	require.Equal(t, "", result.ExtractedText)
	require.Equal(t, 1, result.MessagesDeleted)
	require.Equal(t, msgID, result.NewMessageID)
}

func TestEditMessage_NonUserMessage_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-4"
	seq := 4

	mq := new(editMockQuerier)
	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{
		ID:        "msg-assistant",
		SessionID: sessionID,
		Role:      "assistant",
		Parts:     `[{"type":"text","data":{"text":"hi"}}]`,
		Seq:       int64(seq),
	}, nil)

	editor := NewEditor(mq)
	_, err := editor.EditMessage(ctx, sessionID, seq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a user message")
	mq.AssertExpectations(t)
}

func TestEditMessage_MessageNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-5"
	seq := 99

	mq := new(editMockQuerier)
	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{}, sql.ErrNoRows)

	editor := NewEditor(mq)
	_, err := editor.EditMessage(ctx, sessionID, seq)
	require.Error(t, err)
}

func TestEditMessage_DeleteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sessionID := "sess-6"
	seq := 1
	msgID := "msg-dfail"

	parts := marshalTextParts(t, "hello")

	mq := new(editMockQuerier)
	mq.On("GetMessageBySessionAndSeq", ctx, db.GetMessageBySessionAndSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq),
	}).Return(db.Message{
		ID:        msgID,
		SessionID: sessionID,
		Role:      "user",
		Parts:     parts,
		Seq:       int64(seq),
	}, nil)

	mq.On("ListMessagesInSeqRange", ctx, db.ListMessagesInSeqRangeParams{
		SessionID: sessionID,
		Seq:       int64(seq),
		Seq_2:     999999,
	}).Return([]db.Message{{ID: msgID, Seq: 1}}, nil)

	mq.On("DeleteMessagesAfterSeq", ctx, db.DeleteMessagesAfterSeqParams{
		SessionID: sessionID,
		Seq:       int64(seq - 1),
	}).Return(errors.New("db error"))

	editor := NewEditor(mq)
	_, err := editor.EditMessage(ctx, sessionID, seq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db error")
}
