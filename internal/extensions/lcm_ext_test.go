package extensions

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

type dbHostContext struct {
	mockHostContext
	db *sql.DB
}

func (d *dbHostContext) DB() *sql.DB { return d.db }

func setupTestDBForLCM(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, sqlDB.PingContext(context.Background()))
	goose.SetBaseFS(db.FS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(sqlDB, "migrations"))
	return sqlDB
}

func TestLCMRetrievalTools(t *testing.T) {
	t.Parallel()

	sqlDB := setupTestDBForLCM(t)
	host := &dbHostContext{
		mockHostContext: mockHostContext{cfg: &config.Config{}},
		db:              sqlDB,
	}

	e := &LCMExtension{}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)
	defer e.Shutdown(context.Background())

	tools, err := e.Tools(context.Background())
	require.NoError(t, err)

	expectedToolNames := []string{
		"lcm_grep",
		"lcm_describe",
		"lcm_expand",
		"lcm_bindle",
		"lcm_ancestry",
		"lcm_dolt",
		"lcm_archive",
		"lcm_sprig",
		"lcm_time_query",
		"lcm_file_search",
		"lcm_active_context",
		"lcm_lineage",
	}

	var gotNames []string
	for _, tool := range tools {
		gotNames = append(gotNames, tool.Info().Name)
	}

	for _, name := range expectedToolNames {
		require.Contains(t, gotNames, name, "missing tool: %s", name)
	}

	names := e.ToolNames()
	require.Equal(t, len(tools), len(names), "ToolNames() length must match Tools() length")
	for _, name := range expectedToolNames {
		require.Contains(t, names, name, "ToolNames() missing: %s", name)
	}
}

func TestLCMExtension_InactiveWithoutDB(t *testing.T) {
	t.Parallel()

	host := &mockHostContext{cfg: &config.Config{}}
	e := &LCMExtension{}
	err := e.Init(context.Background(), host)
	require.NoError(t, err)

	tools, err := e.Tools(context.Background())
	require.NoError(t, err)
	require.Nil(t, tools, "extension should be inactive when DB is nil")

	names := e.ToolNames()
	require.Nil(t, names, "ToolNames should be nil when inactive")
}

var _ ext.HostContext = (*dbHostContext)(nil)
