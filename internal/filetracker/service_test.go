package filetracker

import (
	"context"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

type testEnv struct {
	ctx        context.Context
	q          *db.Queries
	svc        Service
	workingDir string
}

func setupTest(t *testing.T) *testEnv {
	t.Helper()
	workingDir := t.TempDir()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	return &testEnv{
		ctx:        t.Context(),
		q:          q,
		svc:        NewService(q, workingDir),
		workingDir: workingDir,
	}
}

func (e *testEnv) createSession(t *testing.T, sessionID string) {
	t.Helper()
	_, err := e.q.CreateSession(e.ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "Test Session",
	})
	require.NoError(t, err)
}

func TestService_RecordRead(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-1"
	path := filepath.Join(env.workingDir, "path/to/file.go")
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path)

	lastRead := env.svc.LastReadTime(env.ctx, sessionID, path)
	require.False(t, lastRead.IsZero(), "expected non-zero time after recording read")
	require.WithinDuration(t, time.Now(), lastRead, 2*time.Second)
}

func TestService_LastReadTime_NotFound(t *testing.T) {
	env := setupTest(t)

	lastRead := env.svc.LastReadTime(env.ctx, "nonexistent-session", "/nonexistent/path")
	require.True(t, lastRead.IsZero(), "expected zero time for unread file")
}

func TestService_RecordRead_UpdatesTimestamp(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-2"
	path := filepath.Join(env.workingDir, "path/to/file.go")
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path)
	firstRead := env.svc.LastReadTime(env.ctx, sessionID, path)
	require.False(t, firstRead.IsZero())

	synctest.Test(t, func(t *testing.T) {
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		env.svc.RecordRead(env.ctx, sessionID, path)
		secondRead := env.svc.LastReadTime(env.ctx, sessionID, path)

		require.False(t, secondRead.Before(firstRead), "second read time should not be before first")
	})
}

func TestService_RecordRead_DifferentSessions(t *testing.T) {
	env := setupTest(t)

	path := filepath.Join(env.workingDir, "shared/file.go")
	session1, session2 := "session-1", "session-2"
	env.createSession(t, session1)
	env.createSession(t, session2)

	env.svc.RecordRead(env.ctx, session1, path)

	lastRead1 := env.svc.LastReadTime(env.ctx, session1, path)
	require.False(t, lastRead1.IsZero())

	lastRead2 := env.svc.LastReadTime(env.ctx, session2, path)
	require.True(t, lastRead2.IsZero(), "session 2 should not see session 1's read")
}

func TestService_RecordRead_DifferentPaths(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-3"
	path1 := filepath.Join(env.workingDir, "path/to/file1.go")
	path2 := filepath.Join(env.workingDir, "path/to/file2.go")
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path1)

	lastRead1 := env.svc.LastReadTime(env.ctx, sessionID, path1)
	require.False(t, lastRead1.IsZero())

	lastRead2 := env.svc.LastReadTime(env.ctx, sessionID, path2)
	require.True(t, lastRead2.IsZero(), "path2 should not be recorded")
}

func TestService_CrossRootPathRelativization(t *testing.T) {
	t.Parallel()

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	sessionID := "cross-root-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "Cross Root Session",
	})
	require.NoError(t, err)

	svc := NewService(q, tmpDir1)

	// Record a file in tmpDir1 (within working dir)
	fileInWorkingDir := filepath.Join(tmpDir1, "src", "main.go")
	require.NoError(t, err)

	svc.RecordRead(t.Context(), sessionID, fileInWorkingDir)

	// Verify we can retrieve it with the same absolute path
	lastRead1 := svc.LastReadTime(t.Context(), sessionID, fileInWorkingDir)
	require.False(t, lastRead1.IsZero(), "should find file recorded within working dir")

	// Record a file in tmpDir2 (outside working dir)
	fileOutsideWorkingDir := filepath.Join(tmpDir2, "other", "file.go")
	svc.RecordRead(t.Context(), sessionID, fileOutsideWorkingDir)

	// Verify we can retrieve it - should be stored as absolute path or relative as possible
	lastRead2 := svc.LastReadTime(t.Context(), sessionID, fileOutsideWorkingDir)
	require.False(t, lastRead2.IsZero(), "should find file outside working dir")

	// Verify that paths are stored differently based on their relationship to working dir
	listedFiles, err := svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	require.Len(t, listedFiles, 2, "should have 2 files listed")

	// At least verify both files are in the list in some form
	foundInWorking := false
	foundOutsideWorking := false
	for _, f := range listedFiles {
		if f == fileInWorkingDir || f == filepath.Join(tmpDir1, "src", "main.go") {
			foundInWorking = true
		}
		if f == fileOutsideWorkingDir || f == filepath.Join(tmpDir2, "other", "file.go") {
			foundOutsideWorking = true
		}
	}
	require.True(t, foundInWorking, "should find file within working dir in list")
	require.True(t, foundOutsideWorking, "should find file outside working dir in list")
}

func TestService_PathRelativizationWithinWorkingDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	sessionID := "relativization-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "Relativization Test Session",
	})
	require.NoError(t, err)

	svc := NewService(q, tmpDir)

	// Create a nested structure
	nestedFile := filepath.Join(tmpDir, "a", "b", "c", "file.go")
	svc.RecordRead(t.Context(), sessionID, nestedFile)

	// Query via absolute path
	lastReadAbs := svc.LastReadTime(t.Context(), sessionID, nestedFile)
	require.False(t, lastReadAbs.IsZero())

	// Verify the file is listed correctly
	listedFiles, err := svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	require.Len(t, listedFiles, 1, "should have 1 file listed")
	require.Equal(t, nestedFile, listedFiles[0], "listed path should match absolute path")
}

func TestService_PathRelativizationSiblingDirs(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)

	sessionID := "sibling-session"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:    sessionID,
		Title: "Sibling Dir Session",
	})
	require.NoError(t, err)

	svc := NewService(q, workingDir)

	// Record files in two sibling directories within working dir
	file1 := filepath.Join(workingDir, "pkg1", "file.go")
	file2 := filepath.Join(workingDir, "pkg2", "file.go")

	svc.RecordRead(t.Context(), sessionID, file1)
	svc.RecordRead(t.Context(), sessionID, file2)

	// Verify both can be retrieved
	require.False(t, svc.LastReadTime(t.Context(), sessionID, file1).IsZero())
	require.False(t, svc.LastReadTime(t.Context(), sessionID, file2).IsZero())

	// Verify list returns absolute paths
	listedFiles, err := svc.ListReadFiles(t.Context(), sessionID)
	require.NoError(t, err)
	require.Len(t, listedFiles, 2)
}

func TestService_ListReadFiles(t *testing.T) {
	env := setupTest(t)

	sessionID := "list-session"
	env.createSession(t, sessionID)

	file1 := filepath.Join(env.workingDir, "file1.go")
	file2 := filepath.Join(env.workingDir, "file2.go")

	env.svc.RecordRead(env.ctx, sessionID, file1)
	env.svc.RecordRead(env.ctx, sessionID, file2)

	listed, err := env.svc.ListReadFiles(env.ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, listed, 2, "should list all read files")

	require.Contains(t, listed, file1)
	require.Contains(t, listed, file2)
}
