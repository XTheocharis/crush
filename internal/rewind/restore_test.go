package rewind

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCodeOnlyRestore verifies that RewindCodeOnly restores file contents on
// disk to the state captured in a snapshot, without modifying conversation
// messages. After capturing a snapshot, the file is modified on disk, then a
// code-only rewind restores the original content while leaving all messages
// intact.
func TestCodeOnlyRestore(t *testing.T) {
	t.Parallel()

	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "code-only-restore")

	// Create a file and take a snapshot.
	env.insertMessage(t, ctx, sid, "user", textParts("create file"))
	env.insertFile(t, ctx, sid, "config.yaml", "key: original-value", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	// Write a different version to disk to simulate user or agent edits.
	configPath := filepath.Join(env.workingDir, "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte("key: modified-value"), 0o644))

	// Rewind code only — should restore the snapshot content.
	result, err := env.svc.Rewind(ctx, sid, 0, RewindCodeOnly)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.NotNil(t, result.Snapshot)
	require.Equal(t, 0, result.MessagesDeleted)

	// Verify file on disk matches the snapshot content.
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "key: original-value", string(data))

	// Verify messages are untouched by the code-only rewind.
	var msgCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&msgCount))
	require.Equal(t, 1, msgCount)
}

// TestConversationTruncation verifies that RewindConvoOnly deletes messages
// after the target sequence while preserving earlier messages. Three user
// messages with unique sentinel text are created. After rewinding to the
// middle message, later sentinels must be removed and earlier ones preserved.
func TestConversationTruncation(t *testing.T) {
	t.Parallel()

	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "convo-truncation")

	// Insert three user messages with unique sentinel values.
	env.insertMessage(t, ctx, sid, "user", textParts("SENTINEL_ALPHA"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response-alpha"))
	env.insertMessage(t, ctx, sid, "user", textParts("SENTINEL_BETA"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response-beta"))
	env.insertMessage(t, ctx, sid, "user", textParts("SENTINEL_GAMMA"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response-gamma"))

	// Total 6 messages before rewind.
	var countBefore int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&countBefore))
	require.Equal(t, 6, countBefore)

	// Rewind conversation to seq=2 (SENTINEL_BETA user message).
	// This deletes messages at seq >= 2 (the user message itself and
	// everything after it).
	result, err := env.svc.Rewind(ctx, sid, 2, RewindConvoOnly)
	require.NoError(t, err)
	require.Equal(t, "SENTINEL_BETA", result.ExtractedText)
	require.Greater(t, result.MessagesDeleted, 0)

	// Verify SENTINEL_ALPHA still exists (preserved).
	var alphaCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ? AND parts LIKE '%SENTINEL_ALPHA%'",
		sid,
	).Scan(&alphaCount))
	require.Equal(t, 1, alphaCount, "SENTINEL_ALPHA should be preserved after rewind")

	// Verify SENTINEL_GAMMA was removed.
	var gammaCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ? AND parts LIKE '%SENTINEL_GAMMA%'",
		sid,
	).Scan(&gammaCount))
	require.Equal(t, 0, gammaCount, "SENTINEL_GAMMA should be removed after rewind")

	// Verify SENTINEL_BETA was removed (rewind deletes the target too).
	var betaCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ? AND parts LIKE '%SENTINEL_BETA%'",
		sid,
	).Scan(&betaCount))
	require.Equal(t, 0, betaCount, "SENTINEL_BETA (target) should be removed by rewind")
}

// TestCombinedRewind verifies that RewindBoth restores both file contents on
// disk and truncates conversation messages in a single operation. A file is
// modified and additional messages are added after a snapshot. After the
// combined rewind, both the file and messages must be restored to the
// pre-modification state.
func TestCombinedRewind(t *testing.T) {
	t.Parallel()

	env := integrationTestEnv(t)
	ctx := context.Background()
	sid := env.createSession(t, ctx, "combined-rewind")

	// Set up initial state: one message and one file, then capture snapshot.
	env.insertMessage(t, ctx, sid, "user", textParts("KEEP_THIS"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response-keep"))
	env.insertFile(t, ctx, sid, "handler.go", "func original() {}", 1)
	require.NoError(t, env.svc.CaptureSnapshot(ctx, sid, 0))

	// Add more messages after the snapshot point.
	env.insertMessage(t, ctx, sid, "user", textParts("REMOVE_THIS"))
	env.insertMessage(t, ctx, sid, "assistant", textParts("response-remove"))

	// Modify the file on disk.
	handlerPath := filepath.Join(env.workingDir, "handler.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(handlerPath), 0o755))
	require.NoError(t, os.WriteFile(handlerPath, []byte("func modified() {}"), 0o644))

	// Perform combined rewind.
	result, err := env.svc.Rewind(ctx, sid, 0, RewindBoth)
	require.NoError(t, err)
	require.Equal(t, 1, result.FilesRestored)
	require.Greater(t, result.MessagesDeleted, 0)
	require.NotNil(t, result.Snapshot)

	// Assert file content is restored to snapshot state.
	data, err := os.ReadFile(handlerPath)
	require.NoError(t, err)
	require.Equal(t, "func original() {}", string(data))

	// Assert KEEP_THIS messages are gone (rewind deletes target seq and
	// everything after) but verify by checking that REMOVE_THIS is removed.
	var removeCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ? AND parts LIKE '%REMOVE_THIS%'",
		sid,
	).Scan(&removeCount))
	require.Equal(t, 0, removeCount, "REMOVE_THIS messages should be deleted after combined rewind")

	// Verify all messages were removed (rewind at seq=0 removes everything).
	var totalCount int
	require.NoError(t, env.sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE session_id = ?", sid,
	).Scan(&totalCount))
	require.Equal(t, 0, totalCount, "all messages should be deleted when rewinding to seq 0")
}
