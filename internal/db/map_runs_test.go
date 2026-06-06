package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func mapTestOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	return dagTestOpenDB(t)
}

func mapTestSetup(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	sqlDB := mapTestOpenDB(t)

	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err, "all migrations should apply cleanly")

	queries := New(sqlDB)
	ctx := context.Background()

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('map-test-session', 'test', 0, 0)",
	)
	require.NoError(t, err)

	return sqlDB, queries
}

func TestMapRun_InsertAndGet(t *testing.T) {
	t.Parallel()
	_, queries := mapTestSetup(t)
	ctx := context.Background()

	for _, toolType := range []string{"agentic_map", "llm_map"} {
		runID := fmt.Sprintf("run-%s", toolType)
		err := queries.InsertMapRun(ctx, InsertMapRunParams{
			RunID:      runID,
			SessionID:  "map-test-session",
			InputPath:  "/input",
			OutputPath: "/output",
			SchemaJson: "{}",
			ToolType:   toolType,
		})
		require.NoError(t, err, "InsertMapRun with tool_type=%q should succeed", toolType)

		run, err := queries.GetMapRun(ctx, runID)
		require.NoError(t, err)
		require.Equal(t, toolType, run.ToolType)
		require.Equal(t, "RUNNING", run.Status)
	}
}

func TestMapRun_UpdateStatus(t *testing.T) {
	t.Parallel()
	_, queries := mapTestSetup(t)
	ctx := context.Background()

	err := queries.InsertMapRun(ctx, InsertMapRunParams{
		RunID:      "run-update",
		SessionID:  "map-test-session",
		InputPath:  "/in",
		OutputPath: "/out",
		SchemaJson: "{}",
		ToolType:   "agentic_map",
	})
	require.NoError(t, err)

	err = queries.UpdateMapRunStatus(ctx, UpdateMapRunStatusParams{
		Status: "DONE",
		RunID:  "run-update",
	})
	require.NoError(t, err)

	run, err := queries.GetMapRun(ctx, "run-update")
	require.NoError(t, err)
	require.Equal(t, "DONE", run.Status)
}

func TestMapRun_GetItems(t *testing.T) {
	t.Parallel()
	_, queries := mapTestSetup(t)
	ctx := context.Background()

	err := queries.InsertMapRun(ctx, InsertMapRunParams{
		RunID:      "run-items",
		SessionID:  "map-test-session",
		InputPath:  "/in",
		OutputPath: "/out",
		SchemaJson: "{}",
		ToolType:   "llm_map",
	})
	require.NoError(t, err)

	err = queries.InsertLcmMapItem(ctx, InsertLcmMapItemParams{
		ItemID:    "item-1",
		RunID:     "run-items",
		InputJson: `{"path":"a.go"}`,
	})
	require.NoError(t, err)

	err = queries.InsertLcmMapItem(ctx, InsertLcmMapItemParams{
		ItemID:    "item-2",
		RunID:     "run-items",
		InputJson: `{"path":"b.go"}`,
	})
	require.NoError(t, err)

	items, err := queries.GetMapRunItems(ctx, "run-items")
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "item-1", items[0].ItemID)
	require.Equal(t, "item-2", items[1].ItemID)
	require.Equal(t, "PENDING", items[0].Status)
}

func TestMapRun_ToolTypeConstraint(t *testing.T) {
	t.Parallel()
	_, queries := mapTestSetup(t)
	ctx := context.Background()

	err := queries.InsertMapRun(ctx, InsertMapRunParams{
		RunID:      "run-bad",
		SessionID:  "map-test-session",
		InputPath:  "/in",
		OutputPath: "/out",
		SchemaJson: "{}",
		ToolType:   "invalid_type",
	})
	require.Error(t, err, "invalid tool_type should be rejected by CHECK constraint")
}

func TestMapRun_DefaultToolType(t *testing.T) {
	t.Parallel()
	sqlDB := mapTestOpenDB(t)

	err := goose.Up(sqlDB, "migrations")
	require.NoError(t, err)
	ctx := context.Background()

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO sessions (id, title, updated_at, created_at) VALUES ('map-default-session', 'test', 0, 0)",
	)
	require.NoError(t, err)

	_, err = sqlDB.ExecContext(ctx,
		"INSERT INTO lcm_map_runs (run_id, session_id, input_path, output_path, schema_json) VALUES ('run-default', 'map-default-session', '/in', '/out', '{}')",
	)
	require.NoError(t, err)

	var toolType string
	err = sqlDB.QueryRowContext(ctx,
		"SELECT tool_type FROM lcm_map_runs WHERE run_id = 'run-default'",
	).Scan(&toolType)
	require.NoError(t, err)
	require.Equal(t, "agentic_map", toolType, "default tool_type should be agentic_map")
}
