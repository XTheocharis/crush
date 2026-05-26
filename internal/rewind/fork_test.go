package rewind

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type forkMockQuerier struct {
	mockQuerier
}

func (m *forkMockQuerier) CloneSessionMessages(ctx context.Context, arg db.CloneSessionMessagesParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *forkMockQuerier) CloneSessionFiles(ctx context.Context, arg db.CloneSessionFilesParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

func (m *forkMockQuerier) DeleteMessagesAfterSeq(ctx context.Context, arg db.DeleteMessagesAfterSeqParams) error {
	args := m.Called(ctx, arg)
	return args.Error(0)
}

type forkMockSessionService struct {
	mock.Mock
}

func (m *forkMockSessionService) Create(ctx context.Context, title string) (session.Session, error) {
	args := m.Called(ctx, title)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) CreateTitleSession(ctx context.Context, parentSessionID string) (session.Session, error) {
	args := m.Called(ctx, parentSessionID)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (session.Session, error) {
	args := m.Called(ctx, toolCallID, parentSessionID, title)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) Get(ctx context.Context, id string) (session.Session, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) GetLast(ctx context.Context) (session.Session, error) {
	args := m.Called(ctx)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) List(ctx context.Context) ([]session.Session, error) {
	args := m.Called(ctx)
	return args.Get(0).([]session.Session), args.Error(1)
}

func (m *forkMockSessionService) Save(ctx context.Context, s session.Session) (session.Session, error) {
	args := m.Called(ctx, s)
	return args.Get(0).(session.Session), args.Error(1)
}

func (m *forkMockSessionService) UpdateTitleAndUsage(ctx context.Context, sessionID, title string, promptTokens, completionTokens int64, cost float64) error {
	args := m.Called(ctx, sessionID, title, promptTokens, completionTokens, cost)
	return args.Error(0)
}

func (m *forkMockSessionService) Rename(ctx context.Context, id string, title string) error {
	args := m.Called(ctx, id, title)
	return args.Error(0)
}

func (m *forkMockSessionService) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *forkMockSessionService) CreateAgentToolSessionID(messageID, toolCallID string) string {
	args := m.Called(messageID, toolCallID)
	return args.String(0)
}

func (m *forkMockSessionService) ParseAgentToolSessionID(sessionID string) (string, string, bool) {
	args := m.Called(sessionID)
	return args.String(0), args.String(1), args.Bool(2)
}

func (m *forkMockSessionService) IsAgentToolSession(sessionID string) bool {
	args := m.Called(sessionID)
	return args.Bool(0)
}

func (m *forkMockSessionService) GetUsageByRole(ctx context.Context, sessionID string) (map[string]float64, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).(map[string]float64), args.Error(1)
}

func (m *forkMockSessionService) Subscribe(ctx context.Context) <-chan pubsub.Event[session.Session] {
	return nil
}

func TestFork_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	origSessionID := "orig-session-123"
	origTitle := "My coding session"

	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, origSessionID).
		Return(session.Session{ID: origSessionID, Title: origTitle}, nil)

	ms.On("List", ctx).
		Return([]session.Session{}, nil)

	forkedSession := session.Session{
		ID:    "new-fork-session-456",
		Title: origTitle + " (fork)",
	}
	ms.On("Create", ctx, origTitle+" (fork)").
		Return(forkedSession, nil)

	ms.On("Save", ctx, mock.MatchedBy(func(s session.Session) bool {
		return s.ID == forkedSession.ID && s.ParentSessionID == origSessionID
	})).Return(forkedSession, nil)

	mq.On("CloneSessionMessages", ctx, mock.MatchedBy(func(arg db.CloneSessionMessagesParams) bool {
		return arg.SessionID_3 == origSessionID && arg.SessionID == forkedSession.ID
	})).Return(nil)

	mq.On("CloneSessionFiles", ctx, mock.MatchedBy(func(arg db.CloneSessionFilesParams) bool {
		return arg.SessionID_2 == origSessionID && arg.SessionID == forkedSession.ID
	})).Return(nil)

	mq.On("DeleteMessagesAfterSeq", ctx, mock.MatchedBy(func(arg db.DeleteMessagesAfterSeqParams) bool {
		return arg.SessionID == forkedSession.ID && arg.Seq == int64(5)
	})).Return(nil)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, origSessionID, 5)

	require.NoError(t, err)
	require.Equal(t, forkedSession.ID, result.NewSessionID)
	require.Equal(t, origTitle+" (fork)", result.NewSessionTitle)
	require.Equal(t, 5, result.MessagesCloned)

	mq.AssertExpectations(t)
	ms.AssertExpectations(t)
}

func TestFork_GetSessionFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "bad-session").
		Return(session.Session{}, context.DeadlineExceeded)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "bad-session", 3)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestFork_CreateSessionFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "orig").
		Return(session.Session{ID: "orig", Title: "Test"}, nil)
	ms.On("List", ctx).
		Return([]session.Session{}, nil)
	ms.On("Create", ctx, "Test (fork)").
		Return(session.Session{}, context.DeadlineExceeded)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "orig", 1)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestFork_CloneMessagesFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "orig").
		Return(session.Session{ID: "orig", Title: "Test"}, nil)
	ms.On("List", ctx).
		Return([]session.Session{}, nil)
	ms.On("Create", ctx, "Test (fork)").
		Return(session.Session{ID: "new"}, nil)
	ms.On("Save", ctx, mock.Anything).
		Return(session.Session{ID: "new"}, nil)
	mq.On("CloneSessionMessages", ctx, mock.Anything).
		Return(context.DeadlineExceeded)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "orig", 2)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestFork_CloneFilesFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "orig").
		Return(session.Session{ID: "orig", Title: "Test"}, nil)
	ms.On("List", ctx).
		Return([]session.Session{}, nil)
	ms.On("Create", ctx, "Test (fork)").
		Return(session.Session{ID: "new"}, nil)
	ms.On("Save", ctx, mock.Anything).
		Return(session.Session{ID: "new"}, nil)
	mq.On("CloneSessionMessages", ctx, mock.Anything).
		Return(nil)
	mq.On("CloneSessionFiles", ctx, mock.Anything).
		Return(context.DeadlineExceeded)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "orig", 2)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestFork_DeleteMessagesAfterSeqFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "orig").
		Return(session.Session{ID: "orig", Title: "Test"}, nil)
	ms.On("List", ctx).
		Return([]session.Session{}, nil)
	ms.On("Create", ctx, "Test (fork)").
		Return(session.Session{ID: "new"}, nil)
	ms.On("Save", ctx, mock.Anything).
		Return(session.Session{ID: "new"}, nil)
	mq.On("CloneSessionMessages", ctx, mock.Anything).
		Return(nil)
	mq.On("CloneSessionFiles", ctx, mock.Anything).
		Return(nil)
	mq.On("DeleteMessagesAfterSeq", ctx, mock.Anything).
		Return(context.DeadlineExceeded)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "orig", 2)

	require.Error(t, err)
	require.Nil(t, result)
}

func TestFork_SeqZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	ms.On("Get", ctx, "orig").
		Return(session.Session{ID: "orig", Title: "Empty"}, nil)
	ms.On("List", ctx).
		Return([]session.Session{}, nil)
	ms.On("Create", ctx, "Empty (fork)").
		Return(session.Session{ID: "new"}, nil)
	ms.On("Save", ctx, mock.Anything).
		Return(session.Session{ID: "new"}, nil)
	mq.On("CloneSessionMessages", ctx, mock.Anything).
		Return(nil)
	mq.On("CloneSessionFiles", ctx, mock.Anything).
		Return(nil)
	mq.On("DeleteMessagesAfterSeq", ctx, mock.MatchedBy(func(arg db.DeleteMessagesAfterSeqParams) bool {
		return arg.Seq == 0
	})).Return(nil)

	f := NewForker(mq, ms)
	result, err := f.Fork(ctx, "orig", 0)

	require.NoError(t, err)
	require.Equal(t, 0, result.MessagesCloned)
}

func TestNewForker_ReturnsInterface(t *testing.T) {
	t.Parallel()

	mq := new(forkMockQuerier)
	ms := new(forkMockSessionService)

	f := NewForker(mq, ms)
	require.NotNil(t, f)
}
