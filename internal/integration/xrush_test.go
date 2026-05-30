// Package integration provides end-to-end integration tests that verify all
// fork-ported extensions register correctly, tools are available, config flags
// work, DB migrations apply cleanly, and both CGO/non-CGO builds work.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/ext"

	// Trigger init() registration of all extensions.
	_ "github.com/charmbracelet/crush/internal/extensions"

	"github.com/stretchr/testify/require"
)

var knownToolsFromExtensions = []string{"batch_edit"}

// TestExtensionRegistrationAndTools verifies that extensions from register.go
// bootstrap successfully, contribute expected tools, and each tool satisfies
// the AgentTool interface. This combines registration and tool checks into
// one test because init()-registered extensions can only be consumed once.
func TestExtensionRegistrationAndTools(t *testing.T) {
	host := ext.NewExtensionHost(ext.HostDeps{
		Config: config.NewTestStore(&config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}),
		WorkingDir: t.TempDir(),
	})

	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx), "Bootstrap should succeed")
	t.Cleanup(func() {
		_ = host.Shutdown(ctx)
		ext.ResetForTesting()
	})

	require.True(t, host.IsBootstrapped())

	tools := host.ContributedTools()
	names := host.ContributedToolNames()
	t.Logf("Contributed tool names (%d): %v", len(names), names)

	for _, expected := range knownToolsFromExtensions {
		require.True(t, slices.Contains(names, expected),
			"expected tool %q in contributed tools", expected)
	}

	require.Equal(t, len(tools), len(names))

	nameSet := make(map[string]bool, len(names))
	for i, tool := range tools {
		info := tool.Info()
		require.NotEmpty(t, info.Name, "tool %d must have a name", i)
		require.Equal(t, names[i], info.Name)
		require.NotEmpty(t, info.Description, "tool %q must have a description", info.Name)
		nameSet[info.Name] = true
	}
	require.Len(t, nameSet, len(names), "all tool names must be unique")

	stepHooks := host.StepHooks()
	t.Logf("Step hooks: %d", len(stepHooks))
	require.NotEmpty(t, stepHooks, "expected step hooks from doom-loop, step-adapter, or lcm extensions")

	runHooks := host.RunHooks()
	t.Logf("Run hooks: %d", len(runHooks))
}

// TestConfigFlags verifies config correctly enables/disables features.
func TestConfigFlags(t *testing.T) {
	t.Parallel()

	t.Run("disabled_tools_filters_tools", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options: &config.Options{
				DisabledTools: []string{"bash", "grep"},
			},
		}
		config.NewTestStore(cfg).SetupAgents()

		coder := cfg.Agents[config.AgentCoder]
		require.NotContains(t, coder.AllowedTools, "bash")
		require.NotContains(t, coder.AllowedTools, "grep")
		require.Contains(t, coder.AllowedTools, "view")
		require.Contains(t, coder.AllowedTools, "edit")
	})

	t.Run("empty_disabled_tools_keeps_all", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		config.NewTestStore(cfg).SetupAgents()

		coder := cfg.Agents[config.AgentCoder]
		require.Contains(t, coder.AllowedTools, "bash")
		require.Contains(t, coder.AllowedTools, "grep")
	})

	t.Run("lcm_options_default_nil", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		require.Nil(t, cfg.Options.LCM)
	})

	t.Run("doom_loop_intervention_default_empty", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		require.Empty(t, cfg.Options.DoomLoopIntervention)
	})

	t.Run("validation_options_default_nil", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		require.Nil(t, cfg.Options.Validation)
	})

	t.Run("task_agent_readonly_tools", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		config.NewTestStore(cfg).SetupAgents()

		task := cfg.Agents[config.AgentTask]
		readOnly := map[string]bool{
			"glob": true, "grep": true, "ls": true,
			"sourcegraph": true, "view": true,
			"lcm_grep": true, "lcm_describe": true, "lcm_expand": true,
			"lcm_bindle": true, "lcm_ancestry": true, "lcm_dolt": true,
			"lcm_archive": true, "lcm_sprig": true, "lcm_time_query": true,
			"lcm_file_search": true, "lcm_active_context": true, "lcm_lineage": true,
		}
		for _, tool := range task.AllowedTools {
			require.True(t, readOnly[tool],
				"task agent should only have read-only tools, got %q", tool)
		}
	})

	t.Run("extension_tool_names_registered", func(t *testing.T) {
		// Not parallel: mutates global extensionToolNames.
		config.ResetExtensionToolNamesForTesting()
		config.RegisterExtensionToolNames(func() []string {
			return []string{"batch_edit", "synthetic_output"}
		})
		defer config.ResetExtensionToolNamesForTesting()

		cfg := &config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}
		config.NewTestStore(cfg).SetupAgents()

		coder := cfg.Agents[config.AgentCoder]
		require.Contains(t, coder.AllowedTools, "batch_edit")
		require.Contains(t, coder.AllowedTools, "synthetic_output")
	})
}

// TestDBMigrations verifies all migrations apply cleanly from scratch.
func TestDBMigrations(t *testing.T) {
	dataDir := t.TempDir()

	ctx := context.Background()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)
	require.NotNil(t, conn)

	assertTableExists(t, ctx, conn, "sessions")
	assertTableExists(t, ctx, conn, "messages")
	assertTableExists(t, ctx, conn, "files")
	assertTableExists(t, ctx, conn, "turn_snapshots")
	assertTableExists(t, ctx, conn, "turn_snapshot_files")
	assertTableExists(t, ctx, conn, "written_files")
	assertTableExists(t, ctx, conn, "read_files")

	assertTableExists(t, ctx, conn, "lcm_context_items")
	assertTableExists(t, ctx, conn, "lcm_summaries")
	assertTableExists(t, ctx, conn, "lcm_summary_messages")
	assertTableExists(t, ctx, conn, "lcm_summary_parents")
	assertTableExists(t, ctx, conn, "lcm_large_files")
	assertTableExists(t, ctx, conn, "lcm_session_config")
	assertTableExists(t, ctx, conn, "lcm_map_runs")
	assertTableExists(t, ctx, conn, "lcm_map_items")

	assertTableExists(t, ctx, conn, "repo_map_file_cache")
	assertTableExists(t, ctx, conn, "repo_map_tags")
	assertTableExists(t, ctx, conn, "repo_map_session_rankings")
	assertTableExists(t, ctx, conn, "repo_map_session_read_only")

	queries := db.New(conn)

	sess, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Title: "test",
	})
	require.NoError(t, err)
	require.NotEmpty(t, sess.ID)

	fetched, err := queries.GetSessionByID(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, "test", fetched.Title)

	msg, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		Role:      "user",
		Parts:     `[{"type":"text","text":"hello"}]`,
	})
	require.NoError(t, err)

	err = queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sess.ID,
		Position:   1,
		ItemType:   "message",
		MessageID:  sql.NullString{String: msg.ID, Valid: true},
		TokenCount: 100,
	})
	require.NoError(t, err)

	items, err := queries.ListLcmContextItems(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	err = queries.UpsertRepoMapFileCache(ctx, db.UpsertRepoMapFileCacheParams{
		RepoKey:  "test-repo",
		RelPath:  "test.go",
		Mtime:    1000,
		Language: "go",
		TagCount: 5,
	})
	require.NoError(t, err)

	cached, err := queries.GetRepoMapFileCacheByPath(ctx, db.GetRepoMapFileCacheByPathParams{
		RepoKey: "test-repo",
		RelPath: "test.go",
	})
	require.NoError(t, err)
	require.Equal(t, "go", cached.Language)
	require.Equal(t, int64(5), cached.TagCount)

	require.NoError(t, db.Release(dataDir))
}

// TestDBTurnSnapshotCRUD verifies turn_snapshots table and file tracking.
func TestDBTurnSnapshotCRUD(t *testing.T) {
	dataDir := t.TempDir()

	ctx := context.Background()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)

	queries := db.New(conn)

	sess, err := queries.CreateSession(ctx, db.CreateSessionParams{
		ID:    fmt.Sprintf("snap-%d", time.Now().UnixNano()),
		Title: "snap-test",
	})
	require.NoError(t, err)

	msg, err := queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		SessionID: sess.ID,
		Role:      "user",
		Parts:     `[{"type":"text","text":"hello"}]`,
	})
	require.NoError(t, err)

	snap, err := queries.CreateTurnSnapshot(ctx, db.CreateTurnSnapshotParams{
		ID:             fmt.Sprintf("snap-%d", time.Now().UnixNano()),
		SessionID:      sess.ID,
		UserMessageID:  msg.ID,
		UserMessageSeq: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, snap.ID)

	fileID := fmt.Sprintf("file-%d", time.Now().UnixNano())
	_, err = queries.CreateFile(ctx, db.CreateFileParams{
		ID:        fileID,
		SessionID: sess.ID,
		Path:      "main.go",
		Content:   "package main\nfunc main() {}\n",
		Version:   1,
	})
	require.NoError(t, err)

	err = queries.AddSnapshotFile(ctx, db.AddSnapshotFileParams{
		SnapshotID: snap.ID,
		FileID:     fileID,
		Path:       "main.go",
		Version:    1,
	})
	require.NoError(t, err)

	gotSnap, err := queries.GetTurnSnapshot(ctx, snap.ID)
	require.NoError(t, err)
	require.Equal(t, sess.ID, gotSnap.SessionID)
	require.Equal(t, int64(1), gotSnap.UserMessageSeq)

	files, err := queries.ListSnapshotFiles(ctx, snap.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.go", files[0].Path)

	require.NoError(t, db.Release(dataDir))
}

// TestDBMigrationCount verifies the expected number of migrations.
func TestDBMigrationCount(t *testing.T) {
	dataDir := t.TempDir()

	ctx := context.Background()
	conn, err := db.Connect(ctx, dataDir)
	require.NoError(t, err)

	var versionID int64
	err = conn.QueryRowContext(ctx,
		"SELECT max(version_id) FROM goose_db_version").Scan(&versionID)
	require.NoError(t, err)

	require.GreaterOrEqual(t, versionID, int64(21),
		"expected at least 21 migrations, got %d", versionID)
	t.Logf("Applied %d migrations", versionID)

	require.NoError(t, db.Release(dataDir))
}

// TestCGOBuild verifies CGO_ENABLED=1 go build ./... succeeds.
func TestCGOBuild(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("skipping CGO build test on %s", runtime.GOOS)
	}

	goBin, err := exec.LookPath("go")
	require.NoError(t, err)

	cmd := exec.CommandContext(context.Background(), goBin, "build", "./...")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Dir = findModuleRoot(t)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"CGO_ENABLED=1 go build ./... should succeed.\nOutput: %s", string(output))
}

// TestRaceDetector verifies extension host concurrent reads under -race.
func TestRaceDetector(t *testing.T) {
	host := ext.NewExtensionHost(ext.HostDeps{
		Config: config.NewTestStore(&config.Config{
			Providers: csync.NewMap[string, config.ProviderConfig](),
			Options:   &config.Options{},
		}),
		WorkingDir: t.TempDir(),
	})

	ctx := context.Background()
	require.NoError(t, host.Bootstrap(ctx))
	t.Cleanup(func() {
		_ = host.Shutdown(ctx)
		ext.ResetForTesting()
	})

	const goroutines = 50
	done := make(chan struct{}, goroutines)
	for range goroutines {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = host.ContributedTools()
			_ = host.ContributedToolNames()
			_ = host.RunHooks()
			_ = host.StepHooks()
			_ = host.IsBootstrapped()
		}()
	}
	for range goroutines {
		<-done
	}
}

func assertTableExists(t *testing.T, ctx context.Context, conn *sql.DB, table string) {
	t.Helper()
	var count int
	err := conn.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
	require.NoError(t, err, "checking table %q", table)
	require.Equal(t, 1, count, "table %q should exist", table)
}

func findModuleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(dir + "/go.mod"); statErr == nil {
			return dir
		}
		abs, err := exec.CommandContext(context.Background(), "realpath", dir+"/..").Output()
		if err != nil {
			t.Fatal("cannot find module root")
		}
		resolved := strings.TrimSpace(string(abs))
		if resolved == dir {
			t.Fatal("cannot find module root: reached filesystem root")
		}
		dir = resolved
	}
}
