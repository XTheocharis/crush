package extensions

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/rewind"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestRewindServiceUnification(t *testing.T) {
	ext.ResetForTesting()

	sqlDB := openRewindTestDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	workingDir := t.TempDir()

	rewindSvc := rewind.NewService(q, sessions, workingDir)

	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:      sessions,
		DB:            sqlDB,
		Config:        nil,
		WorkingDir:    workingDir,
		RewindService: rewindSvc,
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	got := rewindExt.Service()
	require.NotNil(t, got)
	require.Same(t, rewindSvc, got, "RewindExtension must use the exact service injected via HostDeps")
}

func TestRewindExtensionStepHooksWhenServiceProvided(t *testing.T) {
	ext.ResetForTesting()

	sqlDB := openRewindTestDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	workingDir := t.TempDir()

	rewindSvc := rewind.NewService(q, sessions, workingDir)

	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:      sessions,
		DB:            sqlDB,
		Config:        nil,
		WorkingDir:    workingDir,
		RewindService: rewindSvc,
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	hooks := rewindExt.StepHooks()
	require.Len(t, hooks, 1)
	require.Equal(t, "rewind-snapshot", hooks[0].Name)
}

func TestRewindExtensionNoHookWhenServiceNil(t *testing.T) {
	ext.ResetForTesting()

	extHost := ext.NewExtensionHost(ext.HostDeps{
		WorkingDir:    t.TempDir(),
		RewindService: nil,
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	hooks := rewindExt.StepHooks()
	require.Nil(t, hooks)
}

type mockMessageService struct {
	message.Service
}

func (m *mockMessageService) ListUserMessages(_ context.Context, _ string) ([]message.Message, error) {
	return nil, nil
}

func (m *mockMessageService) Subscribe(_ context.Context) <-chan pubsub.Event[message.Message] {
	return nil
}

type mockRewindService struct {
	rewind.Service
	cleanupCalled atomic.Bool
	cleanupErr    error
}

func (m *mockRewindService) CaptureSnapshot(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *mockRewindService) CleanupOldSnapshots(_ context.Context, _ string) error {
	m.cleanupCalled.Store(true)
	return m.cleanupErr
}

func TestCleanupOldSnapshotsCalled(t *testing.T) {
	ext.ResetForTesting()

	workingDir := t.TempDir()
	mockSvc := &mockRewindService{}

	extHost := ext.NewExtensionHost(ext.HostDeps{
		WorkingDir:    workingDir,
		RewindService: mockSvc,
		Messages:      &mockMessageService{},
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	hooks := rewindExt.StepHooks()
	require.Len(t, hooks, 1)
	require.Equal(t, "rewind-snapshot", hooks[0].Name)

	var noResult fantasy.StepResult
	err := hooks[0].OnStepFinish(context.Background(), "test-session", noResult)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return mockSvc.cleanupCalled.Load()
	}, 2*time.Second, 10*time.Millisecond, "CleanupOldSnapshots should have been called after CaptureSnapshot")
}

func TestSnapshotSeqNotZero(t *testing.T) {
	ext.ResetForTesting()

	sqlDB := openRewindTestDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	msgSvc := message.NewService(q, message.WithDebounce(0))
	workingDir := t.TempDir()

	rewindSvc := rewind.NewService(q, sessions, workingDir)

	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:      sessions,
		Messages:      msgSvc,
		DB:            sqlDB,
		Config:        nil,
		WorkingDir:    workingDir,
		RewindService: rewindSvc,
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	ctx := context.Background()

	sess, err := sessions.Create(ctx, "seq-test")
	require.NoError(t, err)

	_, err = msgSvc.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "first"}},
	})
	require.NoError(t, err)

	_, err = msgSvc.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "second"}},
	})
	require.NoError(t, err)

	hooks := rewindExt.StepHooks()
	require.Len(t, hooks, 1)

	var capturedSeq int
	originalSvc := rewindExt.Service()
	wrappedSvc := &seqCapturingService{Service: originalSvc, capturedSeq: &capturedSeq}
	rewindExt.setService(wrappedSvc)

	err = hooks[0].OnStepFinish(ctx, sess.ID, fantasy.StepResult{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return wrappedSvc.captureCalled.Load()
	}, 2*time.Second, 10*time.Millisecond, "CaptureSnapshot should have been called asynchronously")
	require.Greater(t, capturedSeq, 0, "snapshot should capture seq > 0 when the latest user message has seq > 0")
}

func TestSnapshotSeqFallbackToZero(t *testing.T) {
	ext.ResetForTesting()

	sqlDB := openRewindTestDB(t)
	q := db.New(sqlDB)
	sessions := session.NewService(q, sqlDB)
	msgSvc := message.NewService(q, message.WithDebounce(0))
	workingDir := t.TempDir()

	rewindSvc := rewind.NewService(q, sessions, workingDir)

	extHost := ext.NewExtensionHost(ext.HostDeps{
		Sessions:      sessions,
		Messages:      msgSvc,
		DB:            sqlDB,
		Config:        nil,
		WorkingDir:    workingDir,
		RewindService: rewindSvc,
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	ctx := context.Background()

	sess, err := sessions.Create(ctx, "seq-fallback")
	require.NoError(t, err)

	var capturedSeq int
	originalSvc := rewindExt.Service()
	wrappedSvc := &seqCapturingService{Service: originalSvc, capturedSeq: &capturedSeq}
	rewindExt.setService(wrappedSvc)

	hooks := rewindExt.StepHooks()
	require.Len(t, hooks, 1)

	err = hooks[0].OnStepFinish(ctx, sess.ID, fantasy.StepResult{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return wrappedSvc.captureCalled.Load()
	}, 2*time.Second, 10*time.Millisecond, "CaptureSnapshot should have been called asynchronously")
	require.Equal(t, 0, capturedSeq, "snapshot should fall back to seq=0 when no user messages exist")
}

func TestSnapshotPersistsAfterContextCancellation(t *testing.T) {
	ext.ResetForTesting()

	workingDir := t.TempDir()
	var captured atomic.Bool
	mockSvc := &ctxCancelCapturingService{captured: &captured}

	extHost := ext.NewExtensionHost(ext.HostDeps{
		WorkingDir:    workingDir,
		RewindService: mockSvc,
		Messages:      &mockMessageService{},
	})

	rewindExt := &RewindExtension{}
	require.NoError(t, extHost.Register(rewindExt))
	require.NoError(t, extHost.Bootstrap(context.Background()))
	t.Cleanup(func() { _ = extHost.Shutdown(context.Background()) })

	hooks := rewindExt.StepHooks()
	require.Len(t, hooks, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := hooks[0].OnStepFinish(ctx, "test-session", fantasy.StepResult{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return captured.Load()
	}, 2*time.Second, 10*time.Millisecond, "CaptureSnapshot must complete even after parent context is cancelled")
}

type ctxCancelCapturingService struct {
	rewind.Service
	captured *atomic.Bool
}

func (s *ctxCancelCapturingService) CaptureSnapshot(_ context.Context, _ string, _ int) error {
	s.captured.Store(true)
	return nil
}

func (s *ctxCancelCapturingService) CleanupOldSnapshots(_ context.Context, _ string) error {
	return nil
}

type seqCapturingService struct {
	rewind.Service
	capturedSeq   *int
	captureCalled atomic.Bool
}

func (s *seqCapturingService) CaptureSnapshot(_ context.Context, _ string, userMessageSeq int) error {
	s.captureCalled.Store(true)
	*s.capturedSeq = userMessageSeq
	return nil
}

func (s *seqCapturingService) CleanupOldSnapshots(_ context.Context, _ string) error {
	return nil
}

// Service returns the rewind service for testing.
func (e *RewindExtension) Service() rewind.Service {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.service
}

func (e *RewindExtension) setService(svc rewind.Service) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.service = svc
}

var gooseOnce sync.Once

func openRewindTestDB(t *testing.T) *sql.DB {
	t.Helper()
	gooseOnce.Do(func() {
		goose.SetBaseFS(db.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose.SetDialect: %v", err)
		}
	})

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)",
		dbPath,
	)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, sqlDB.PingContext(context.Background()))
	require.NoError(t, goose.Up(sqlDB, "migrations"))
	return sqlDB
}
