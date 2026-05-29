package rewind

import (
	"context"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/mock"
)

// mockQuerier is a testify/mock-based implementation of db.Querier for use
// in tests across the rewind package.
type mockQuerier struct {
	mock.Mock
}

var _ db.Querier = (*mockQuerier)(nil)

func (m *mockQuerier) AddSnapshotFile(ctx context.Context, arg db.AddSnapshotFileParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) AppendLcmContextItem(ctx context.Context, arg db.AppendLcmContextItemParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) ClearSessionSummaryMessageID(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) CloneSessionFiles(ctx context.Context, arg db.CloneSessionFilesParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) CloneSessionMessages(ctx context.Context, arg db.CloneSessionMessagesParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) CountTurnSnapshots(ctx context.Context, sessionID string) (int64, error) {
	args := m.Called(ctx, sessionID)
	var zero int64
	if v := args.Get(0); v != nil {
		return v.(int64), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) {
	args := m.Called(ctx, arg)
	var zero db.File
	if v := args.Get(0); v != nil {
		return v.(db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) CreateMessage(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) {
	args := m.Called(ctx, arg)
	var zero db.Message
	if v := args.Get(0); v != nil {
		return v.(db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error) {
	args := m.Called(ctx, arg)
	var zero db.Session
	if v := args.Get(0); v != nil {
		return v.(db.Session), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) CreateTurnSnapshot(ctx context.Context, arg db.CreateTurnSnapshotParams) (db.TurnSnapshot, error) {
	args := m.Called(ctx, arg)
	var zero db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.(db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) DeleteAllLcmContextItems(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteFile(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) DeleteLcmSummary(ctx context.Context, summaryID string) error {
	args := m.Called(ctx, summaryID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteLcmSummaryMessages(ctx context.Context, summaryID string) error {
	args := m.Called(ctx, summaryID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteLcmSummaryParents(ctx context.Context, summaryID string) error {
	args := m.Called(ctx, summaryID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteMessage(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) DeleteMessagesAfterSeq(ctx context.Context, arg db.DeleteMessagesAfterSeqParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteOldTurnSnapshots(ctx context.Context, arg db.DeleteOldTurnSnapshotsParams) (int64, error) {
	args := m.Called(ctx, arg)
	var zero int64
	if v := args.Get(0); v != nil {
		return v.(int64), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) DeleteRepoMapFileCache(ctx context.Context, arg db.DeleteRepoMapFileCacheParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteRepoMapTagsByPath(ctx context.Context, arg db.DeleteRepoMapTagsByPathParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSession(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSessionFiles(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSessionMessages(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSessionRankings(ctx context.Context, arg db.DeleteSessionRankingsParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSessionReadOnlyPaths(ctx context.Context, arg db.DeleteSessionReadOnlyPathsParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSessionTurnSnapshots(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSnapshotFiles(ctx context.Context, snapshotID string) error {
	args := m.Called(ctx, snapshotID)
	return args.Error(0)
}

func (m *mockQuerier) DeleteSnapshotsAfterSeq(ctx context.Context, arg db.DeleteSnapshotsAfterSeqParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) DeleteTurnSnapshot(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockQuerier) GetAverageResponseTime(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	var zero int64
	if v := args.Get(0); v != nil {
		return v.(int64), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetContentReplacement(ctx context.Context, id int64) (db.LcmContentReplacement, error) {
	args := m.Called(ctx, id)
	var zero db.LcmContentReplacement
	if v := args.Get(0); v != nil {
		return v.(db.LcmContentReplacement), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetContentReplacementsByFileID(ctx context.Context, arg db.GetContentReplacementsByFileIDParams) ([]db.LcmContentReplacement, error) {
	args := m.Called(ctx, arg)
	var zero []db.LcmContentReplacement
	if v := args.Get(0); v != nil {
		return v.([]db.LcmContentReplacement), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetContentReplacementsBySessionPosition(ctx context.Context, arg db.GetContentReplacementsBySessionPositionParams) ([]db.LcmContentReplacement, error) {
	args := m.Called(ctx, arg)
	var zero []db.LcmContentReplacement
	if v := args.Get(0); v != nil {
		return v.([]db.LcmContentReplacement), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetFile(ctx context.Context, id string) (db.File, error) {
	args := m.Called(ctx, id)
	var zero db.File
	if v := args.Get(0); v != nil {
		return v.(db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetFileByPathAndSession(ctx context.Context, arg db.GetFileByPathAndSessionParams) (db.File, error) {
	args := m.Called(ctx, arg)
	var zero db.File
	if v := args.Get(0); v != nil {
		return v.(db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetFileRead(ctx context.Context, arg db.GetFileReadParams) (db.ReadFile, error) {
	args := m.Called(ctx, arg)
	var zero db.ReadFile
	if v := args.Get(0); v != nil {
		return v.(db.ReadFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetFileWrite(ctx context.Context, arg db.GetFileWriteParams) (db.WrittenFile, error) {
	args := m.Called(ctx, arg)
	var zero db.WrittenFile
	if v := args.Get(0); v != nil {
		return v.(db.WrittenFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetHourDayHeatmap(ctx context.Context) ([]db.GetHourDayHeatmapRow, error) {
	args := m.Called(ctx)
	var zero []db.GetHourDayHeatmapRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetHourDayHeatmapRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLastSession(ctx context.Context) (db.Session, error) {
	args := m.Called(ctx)
	var zero db.Session
	if v := args.Get(0); v != nil {
		return v.(db.Session), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLatestTurnSnapshot(ctx context.Context, sessionID string) (db.TurnSnapshot, error) {
	args := m.Called(ctx, sessionID)
	var zero db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.(db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLatestUserMessage(ctx context.Context, sessionID string) (db.Message, error) {
	args := m.Called(ctx, sessionID)
	var zero db.Message
	if v := args.Get(0); v != nil {
		return v.(db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLcmContextTokenCount(ctx context.Context, sessionID string) (any, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0), args.Error(1)
}

func (m *mockQuerier) GetLcmLargeFile(ctx context.Context, fileID string) (db.LcmLargeFile, error) {
	args := m.Called(ctx, fileID)
	var zero db.LcmLargeFile
	if v := args.Get(0); v != nil {
		return v.(db.LcmLargeFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLcmSessionConfig(ctx context.Context, sessionID string) (db.LcmSessionConfig, error) {
	args := m.Called(ctx, sessionID)
	var zero db.LcmSessionConfig
	if v := args.Get(0); v != nil {
		return v.(db.LcmSessionConfig), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetLcmSummary(ctx context.Context, summaryID string) (db.LcmSummary, error) {
	args := m.Called(ctx, summaryID)
	var zero db.LcmSummary
	if v := args.Get(0); v != nil {
		return v.(db.LcmSummary), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetMessage(ctx context.Context, id string) (db.Message, error) {
	args := m.Called(ctx, id)
	var zero db.Message
	if v := args.Get(0); v != nil {
		return v.(db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetMessageBySessionAndSeq(ctx context.Context, arg db.GetMessageBySessionAndSeqParams) (db.Message, error) {
	args := m.Called(ctx, arg)
	var zero db.Message
	if v := args.Get(0); v != nil {
		return v.(db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetMessageCountByTimeRange(ctx context.Context, arg db.GetMessageCountByTimeRangeParams) (int64, error) {
	args := m.Called(ctx, arg)
	var zero int64
	if v := args.Get(0); v != nil {
		return v.(int64), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetMessagesByTimeRange(ctx context.Context, arg db.GetMessagesByTimeRangeParams) ([]db.Message, error) {
	args := m.Called(ctx, arg)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetRecentActivity(ctx context.Context) ([]db.GetRecentActivityRow, error) {
	args := m.Called(ctx)
	var zero []db.GetRecentActivityRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetRecentActivityRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetRepoMapFileCache(ctx context.Context, repoKey string) ([]db.RepoMapFileCache, error) {
	args := m.Called(ctx, repoKey)
	var zero []db.RepoMapFileCache
	if v := args.Get(0); v != nil {
		return v.([]db.RepoMapFileCache), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetRepoMapFileCacheByPath(ctx context.Context, arg db.GetRepoMapFileCacheByPathParams) (db.RepoMapFileCache, error) {
	args := m.Called(ctx, arg)
	var zero db.RepoMapFileCache
	if v := args.Get(0); v != nil {
		return v.(db.RepoMapFileCache), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetSessionByID(ctx context.Context, id string) (db.Session, error) {
	args := m.Called(ctx, id)
	var zero db.Session
	if v := args.Get(0); v != nil {
		return v.(db.Session), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetToolUsage(ctx context.Context) ([]db.GetToolUsageRow, error) {
	args := m.Called(ctx)
	var zero []db.GetToolUsageRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetToolUsageRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetTotalStats(ctx context.Context) (db.GetTotalStatsRow, error) {
	args := m.Called(ctx)
	var zero db.GetTotalStatsRow
	if v := args.Get(0); v != nil {
		return v.(db.GetTotalStatsRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetTurnSnapshot(ctx context.Context, id string) (db.TurnSnapshot, error) {
	args := m.Called(ctx, id)
	var zero db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.(db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetTurnSnapshotAtOrBeforeSeq(ctx context.Context, arg db.GetTurnSnapshotAtOrBeforeSeqParams) (db.TurnSnapshot, error) {
	args := m.Called(ctx, arg)
	var zero db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.(db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetTurnSnapshotByMessage(ctx context.Context, arg db.GetTurnSnapshotByMessageParams) (db.TurnSnapshot, error) {
	args := m.Called(ctx, arg)
	var zero db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.(db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetUsageByDay(ctx context.Context) ([]db.GetUsageByDayRow, error) {
	args := m.Called(ctx)
	var zero []db.GetUsageByDayRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetUsageByDayRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetUsageByDayOfWeek(ctx context.Context) ([]db.GetUsageByDayOfWeekRow, error) {
	args := m.Called(ctx)
	var zero []db.GetUsageByDayOfWeekRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetUsageByDayOfWeekRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetUsageByHour(ctx context.Context) ([]db.GetUsageByHourRow, error) {
	args := m.Called(ctx)
	var zero []db.GetUsageByHourRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetUsageByHourRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) GetUsageByModel(ctx context.Context) ([]db.GetUsageByModelRow, error) {
	args := m.Called(ctx)
	var zero []db.GetUsageByModelRow
	if v := args.Get(0); v != nil {
		return v.([]db.GetUsageByModelRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) InsertLcmContextItem(ctx context.Context, arg db.InsertLcmContextItemParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmLargeFile(ctx context.Context, arg db.InsertLcmLargeFileParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmMapItem(ctx context.Context, arg db.InsertLcmMapItemParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmMapRun(ctx context.Context, arg db.InsertLcmMapRunParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmSummary(ctx context.Context, arg db.InsertLcmSummaryParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmSummaryMessage(ctx context.Context, arg db.InsertLcmSummaryMessageParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertLcmSummaryParent(ctx context.Context, arg db.InsertLcmSummaryParentParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) InsertRepoMapTag(ctx context.Context, arg db.InsertRepoMapTagParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) ListAllUserMessages(ctx context.Context) ([]db.Message, error) {
	args := m.Called(ctx)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListContentReplacementsByRound(ctx context.Context, arg db.ListContentReplacementsByRoundParams) ([]db.LcmContentReplacement, error) {
	args := m.Called(ctx, arg)
	var zero []db.LcmContentReplacement
	if v := args.Get(0); v != nil {
		return v.([]db.LcmContentReplacement), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListContentReplacementsByState(ctx context.Context, arg db.ListContentReplacementsByStateParams) ([]db.LcmContentReplacement, error) {
	args := m.Called(ctx, arg)
	var zero []db.LcmContentReplacement
	if v := args.Get(0); v != nil {
		return v.([]db.LcmContentReplacement), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListFilesByPath(ctx context.Context, path string) ([]db.File, error) {
	args := m.Called(ctx, path)
	var zero []db.File
	if v := args.Get(0); v != nil {
		return v.([]db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListFilesBySession(ctx context.Context, sessionID string) ([]db.File, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.File
	if v := args.Get(0); v != nil {
		return v.([]db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLatestSessionFiles(ctx context.Context, sessionID string) ([]db.File, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.File
	if v := args.Get(0); v != nil {
		return v.([]db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLcmContextItems(ctx context.Context, sessionID string) ([]db.LcmContextItem, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.LcmContextItem
	if v := args.Get(0); v != nil {
		return v.([]db.LcmContextItem), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLcmLargeFilesBySession(ctx context.Context, sessionID string) ([]db.LcmLargeFile, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.LcmLargeFile
	if v := args.Get(0); v != nil {
		return v.([]db.LcmLargeFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLcmSummariesBySession(ctx context.Context, sessionID string) ([]db.LcmSummary, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.LcmSummary
	if v := args.Get(0); v != nil {
		return v.([]db.LcmSummary), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLcmSummaryMessages(ctx context.Context, summaryID string) ([]db.LcmSummaryMessage, error) {
	args := m.Called(ctx, summaryID)
	var zero []db.LcmSummaryMessage
	if v := args.Get(0); v != nil {
		return v.([]db.LcmSummaryMessage), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListLcmSummaryParents(ctx context.Context, summaryID string) ([]db.LcmSummaryParent, error) {
	args := m.Called(ctx, summaryID)
	var zero []db.LcmSummaryParent
	if v := args.Get(0); v != nil {
		return v.([]db.LcmSummaryParent), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListMessagesBySession(ctx context.Context, sessionID string) ([]db.Message, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListMessagesBySessionSeq(ctx context.Context, sessionID string) ([]db.Message, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListMessagesInSeqRange(ctx context.Context, arg db.ListMessagesInSeqRangeParams) ([]db.Message, error) {
	args := m.Called(ctx, arg)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListNewFiles(ctx context.Context) ([]db.File, error) {
	args := m.Called(ctx)
	var zero []db.File
	if v := args.Get(0); v != nil {
		return v.([]db.File), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListRecentSessionReadFiles(ctx context.Context, arg db.ListRecentSessionReadFilesParams) ([]db.ReadFile, error) {
	args := m.Called(ctx, arg)
	var zero []db.ReadFile
	if v := args.Get(0); v != nil {
		return v.([]db.ReadFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListRepoMapDefsByName(ctx context.Context, arg db.ListRepoMapDefsByNameParams) ([]db.ListRepoMapDefsByNameRow, error) {
	args := m.Called(ctx, arg)
	var zero []db.ListRepoMapDefsByNameRow
	if v := args.Get(0); v != nil {
		return v.([]db.ListRepoMapDefsByNameRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListRepoMapTags(ctx context.Context, repoKey string) ([]db.ListRepoMapTagsRow, error) {
	args := m.Called(ctx, repoKey)
	var zero []db.ListRepoMapTagsRow
	if v := args.Get(0); v != nil {
		return v.([]db.ListRepoMapTagsRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSessionRankings(ctx context.Context, arg db.ListSessionRankingsParams) ([]db.RepoMapSessionRanking, error) {
	args := m.Called(ctx, arg)
	var zero []db.RepoMapSessionRanking
	if v := args.Get(0); v != nil {
		return v.([]db.RepoMapSessionRanking), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSessionReadFiles(ctx context.Context, sessionID string) ([]db.ReadFile, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.ReadFile
	if v := args.Get(0); v != nil {
		return v.([]db.ReadFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSessionReadOnlyPaths(ctx context.Context, arg db.ListSessionReadOnlyPathsParams) ([]string, error) {
	args := m.Called(ctx, arg)
	var zero []string
	if v := args.Get(0); v != nil {
		return v.([]string), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSessionWrittenFiles(ctx context.Context, sessionID string) ([]db.WrittenFile, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.WrittenFile
	if v := args.Get(0); v != nil {
		return v.([]db.WrittenFile), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSessions(ctx context.Context) ([]db.Session, error) {
	args := m.Called(ctx)
	var zero []db.Session
	if v := args.Get(0); v != nil {
		return v.([]db.Session), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListSnapshotFiles(ctx context.Context, snapshotID string) ([]db.ListSnapshotFilesRow, error) {
	args := m.Called(ctx, snapshotID)
	var zero []db.ListSnapshotFilesRow
	if v := args.Get(0); v != nil {
		return v.([]db.ListSnapshotFilesRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListTurnSnapshotsBySession(ctx context.Context, sessionID string) ([]db.TurnSnapshot, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.TurnSnapshot
	if v := args.Get(0); v != nil {
		return v.([]db.TurnSnapshot), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) ListUserMessagesBySession(ctx context.Context, sessionID string) ([]db.Message, error) {
	args := m.Called(ctx, sessionID)
	var zero []db.Message
	if v := args.Get(0); v != nil {
		return v.([]db.Message), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) RecordContentReplacement(ctx context.Context, arg db.RecordContentReplacementParams) (int64, error) {
	args := m.Called(ctx, arg)
	var zero int64
	if v := args.Get(0); v != nil {
		return v.(int64), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) RecordFileRead(ctx context.Context, arg db.RecordFileReadParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) RecordFileWrite(ctx context.Context, arg db.RecordFileWriteParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) RenameSession(ctx context.Context, arg db.RenameSessionParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) SearchLcmSummaries(ctx context.Context, arg db.SearchLcmSummariesParams) ([]db.SearchLcmSummariesRow, error) {
	args := m.Called(ctx, arg)
	var zero []db.SearchLcmSummariesRow
	if v := args.Get(0); v != nil {
		return v.([]db.SearchLcmSummariesRow), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) UpdateContentReplacementState(ctx context.Context, arg db.UpdateContentReplacementStateParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateLcmLargeFileExploration(ctx context.Context, arg db.UpdateLcmLargeFileExplorationParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateLcmMapItem(ctx context.Context, arg db.UpdateLcmMapItemParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateLcmMapRunStatus(ctx context.Context, arg db.UpdateLcmMapRunStatusParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateLcmSessionConfig(ctx context.Context, arg db.UpdateLcmSessionConfigParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateMessage(ctx context.Context, arg db.UpdateMessageParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateMessageTokenCount(ctx context.Context, arg db.UpdateMessageTokenCountParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpdateSession(ctx context.Context, arg db.UpdateSessionParams) (db.Session, error) {
	args := m.Called(ctx, arg)
	var zero db.Session
	if v := args.Get(0); v != nil {
		return v.(db.Session), args.Error(1)
	}
	return zero, args.Error(1)
}

func (m *mockQuerier) UpdateSessionTitleAndUsage(ctx context.Context, arg db.UpdateSessionTitleAndUsageParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpsertLcmSessionConfig(ctx context.Context, arg db.UpsertLcmSessionConfigParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpsertRepoMapFileCache(ctx context.Context, arg db.UpsertRepoMapFileCacheParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpsertSessionRanking(ctx context.Context, arg db.UpsertSessionRankingParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *mockQuerier) UpsertSessionReadOnlyPath(ctx context.Context, arg db.UpsertSessionReadOnlyPathParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

// XRUSH: mock for ListRecentReadFiles
func (m *mockQuerier) ListRecentReadFiles(ctx context.Context, readAt int64) ([]db.ReadFile, error) {
	args := m.Called(ctx, readAt)
	var zero []db.ReadFile
	if v := args.Get(0); v != nil {
		return v.([]db.ReadFile), args.Error(1)
	}
	return zero, args.Error(1)
}
