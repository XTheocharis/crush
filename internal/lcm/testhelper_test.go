package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

// gooseOnce ensures goose global state is initialized exactly once.
var gooseOnce sync.Once

func initGoose() {
	gooseOnce.Do(func() {
		goose.SetBaseFS(db.FS)
		if err := goose.SetDialect("sqlite3"); err != nil {
			panic(fmt.Sprintf("goose.SetDialect: %v", err))
		}
	})
}

// setupTestDB creates a temporary SQLite database with all migrations applied.
// It returns the db.Queries and the raw *sql.DB.
//
// recursive_triggers is ON to match production (see connect.go). The LCM
// migration replaces the pre-existing updated_at triggers with recursion-safe
// versions that use a WHEN guard, so this is safe.
func setupTestDB(t *testing.T) (*db.Queries, *sql.DB) {
	t.Helper()
	initGoose()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(ON)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	err = sqlDB.PingContext(context.Background())
	require.NoError(t, err)

	err = goose.Up(sqlDB, "migrations")
	require.NoError(t, err)

	queries := db.New(sqlDB)
	return queries, sqlDB
}

// createTestSession inserts a test session and returns its ID.
func createTestSession(t *testing.T, queries *db.Queries, sessionID string) {
	t.Helper()
	ctx := context.Background()
	_, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    sessionID,
		Title: "test session",
	})
	require.NoError(t, err)
}

// createTestMessage inserts a test message and returns its ID.
func createTestMessage(t *testing.T, queries *db.Queries, sessionID, msgID, role, textContent string) {
	t.Helper()
	ctx := context.Background()
	parts := fmt.Sprintf(`[{"type":"text","data":{"text":%q}}]`, textContent)
	_, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:        msgID,
		SessionID: sessionID,
		Role:      role,
		Parts:     parts,
	})
	require.NoError(t, err)
}

// mockLLMClient is a mock LLM client for testing.
type mockLLMClient struct {
	response  string
	err       error
	callCount int
}

func (m *mockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	m.callCount++
	return m.response, m.err
}
