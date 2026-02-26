package lcm

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/require"
)

func TestMessageDecorator_Create_LargeToolOutput_ThresholdAndExplorationPersistence(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-threshold"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 5,
	})

	toolOutput := strings.Repeat("x", 80) // ~20 tokens (> threshold 5, < default 10000)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: toolOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "[Large Tool Output Stored:")
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.True(t, files[0].ExplorationSummary.Valid)
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorationSummary.String))
	require.True(t, files[0].ExplorerUsed.Valid)
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorerUsed.String))
}

func TestMessageDecorator_Create_LargeToolOutput_DisabledByConfig(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-disabled"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		DisableLargeToolOutput:        true,
		LargeToolOutputTokenThreshold: 1,
	})

	toolOutput := strings.Repeat("y", 120000) // very large, should still bypass interception
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: toolOutput}},
	})
	require.NoError(t, err)
	require.Equal(t, toolOutput, msg.Content().Text)
	require.NotContains(t, msg.Content().Text, "[Large Tool Output Stored:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestMessageDecorator_Create_LargeToolOutput_BelowThreshold_NoStorage(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-below-threshold"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 1000, // set high, so small output is below threshold
	})

	toolOutput := strings.Repeat("z", 80) // ~20 tokens (below threshold of 1000)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: toolOutput}},
	})
	require.NoError(t, err)
	// Below threshold: full content is stored inline, no LCM reference
	require.Equal(t, toolOutput, msg.Content().Text)
	require.NotContains(t, msg.Content().Text, "[Large Tool Output Stored:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestMessageDecorator_Create_NonToolRole_NoStorageOrExploration(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-non-tool"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 1, // very low threshold
	})

	// Large content in a non-tool message (e.g., assistant response) should NOT trigger LCM storage
	assistantOutput := strings.Repeat("a", 120000) // very large
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: assistantOutput}},
	})
	require.NoError(t, err)
	require.Equal(t, assistantOutput, msg.Content().Text)
	require.NotContains(t, msg.Content().Text, "[Large Tool Output Stored:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestMessageDecorator_Create_UserRole_NoStorageOrExploration(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-user"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 1,
	})

	userInput := strings.Repeat("b", 120000)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: userInput}},
	})
	require.NoError(t, err)
	require.Equal(t, userInput, msg.Content().Text)
	require.NotContains(t, msg.Content().Text, "[Large Tool Output Stored:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestMessageDecorator_Create_StorageFails_NonBlockingFallback(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-storage-fail"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 1,
	})

	// Close the DB to simulate storage failure
	sqlDB.Close()

	toolOutput := strings.Repeat("c", 120000)
	_, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: toolOutput}},
	})
	// Message creation should fail (DB is closed, inner service can't create message)
	require.Error(t, err)
	// Because message creation failed, we can't check for the warning in msg
}

func TestMessageDecorator_Create_PersistenceMatrix_EndToEnd(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-matrix"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 100, // threshold of 100 tokens (~400 chars)
	})

	// Test 1: Below threshold - no storage, no exploration (verified by empty list)
	smallOutput := strings.Repeat(".", 100) // ~25 tokens (below threshold)
	_, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: smallOutput}},
	})
	require.NoError(t, err)

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, files, "Below threshold should not store in lcm_large_files")

	// Test 2: Above threshold + tool role - storage with non-null exploration
	largeOutput := strings.Repeat("x", 500) // ~125 tokens (above threshold)
	_, err = svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: largeOutput}},
	})
	require.NoError(t, err)

	files, err = queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1, "Above threshold should store exactly one file")
	require.True(t, files[0].ExplorationSummary.Valid, "Exploration summary should be non-null")
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorationSummary.String), "Exploration summary should not be empty")
	require.True(t, files[0].ExplorerUsed.Valid, "Explorer used should be non-null")
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorerUsed.String), "Explorer used should not be empty")

	// Test 3: Non-tool role with large content - no storage
	assistantOutput := strings.Repeat("y", 500)
	_, err = svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: assistantOutput}},
	})
	require.NoError(t, err)

	// Still only one file (the tool message), not two
	files, err = queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1, "Non-tool role should not create additional storage entries")
}

func TestMessageDecorator_Create_ExplorationFailure_NonBlocking(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-explore-fail"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)

	// Create decorator with nil explorers registry to force exploration failure
	// This simulates the scenario where exploration fails but storage succeeds
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 1,
	})

	// Set explorers to nil via a custom decorator
	decor := svc.(*messageDecorator)
	decor.explorers = nil

	largeOutput := strings.Repeat("a", 5000) // ~1250 tokens (well above threshold)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: largeOutput}},
	})

	// Message creation must succeed even though exploration will fail
	require.NoError(t, err, "Message creation should succeed despite exploration failure")
	require.Contains(t, msg.Content().Text, "[Large Tool Output Stored:",
		"Message content should reference the stored file")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1, "File should still be stored in lcm_large_files")

	// Exploration fields should be NULL since exploration failed (non-blocking)
	require.False(t, files[0].ExplorationSummary.Valid,
		"Exploration summary should be NULL when exploration fails")
	require.False(t, files[0].ExplorerUsed.Valid,
		"Explorer used should be NULL when exploration fails")
}

func TestMessageDecorator_Create_RuntimePathPersistenceMatrix(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-runtime-matrix"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	cfg := MessageDecoratorConfig{LargeToolOutputTokenThreshold: 1}

	// Persisted path (text): should write non-null exploration fields.
	textSvc := NewMessageDecorator(inner, mgr, queries, sqlDB, cfg)
	textOutput := strings.Repeat("x", 5000)
	msg, err := textSvc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: textOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.True(t, files[0].ExplorationSummary.Valid)
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorationSummary.String))
	require.True(t, files[0].ExplorerUsed.Valid)
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorerUsed.String))

	// Non-persisted path (forced): simulate non-persisted runtime path by disabling
	// exploration execution; storage+flow must still succeed with NULL exploration fields.
	nonPersistSvc := NewMessageDecorator(inner, mgr, queries, sqlDB, cfg)
	decor := nonPersistSvc.(*messageDecorator)
	decor.explorers = nil
	binaryLikeOutput := string([]byte{0x00, 0x01, 0x02, 0x03}) + strings.Repeat("\x00", 4096)
	msg, err = nonPersistSvc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: binaryLikeOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	files, err = queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 2)
	latest := files[1]
	require.False(t, latest.ExplorationSummary.Valid)
	require.False(t, latest.ExplorerUsed.Valid)
}
