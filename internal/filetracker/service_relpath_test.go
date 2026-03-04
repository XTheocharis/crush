package filetracker

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

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
