package lcm

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/lcm/explorer"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/treesitter"
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
	decor.runtimeAdapter = nil

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
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, cfg)

	// Persisted path (text): should write non-null exploration fields.
	textOutput := strings.Repeat("x", 5000)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: textOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	textFileIDs := ExtractFileIDs(msg.Content().Text)
	require.Len(t, textFileIDs, 1)
	textFile, err := queries.GetLcmLargeFile(ctx, textFileIDs[0])
	require.NoError(t, err)
	require.True(t, textFile.ExplorationSummary.Valid)
	require.NotEmpty(t, strings.TrimSpace(textFile.ExplorationSummary.String))
	require.True(t, textFile.ExplorerUsed.Valid)
	require.NotEmpty(t, strings.TrimSpace(textFile.ExplorerUsed.String))

	// Binary-like payload still resolves through ingestion persistence policy in
	// the B3 matrix, so exploration fields remain populated.
	binaryLikeOutput := string([]byte{0x7f, 0x45, 0x4c, 0x46}) + strings.Repeat("\x00", 4096)
	msg, err = svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: binaryLikeOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	binaryFileIDs := ExtractFileIDs(msg.Content().Text)
	require.Len(t, binaryFileIDs, 1)
	binaryFile, err := queries.GetLcmLargeFile(ctx, binaryFileIDs[0])
	require.NoError(t, err)
	require.True(t, binaryFile.ExplorationSummary.Valid)
	require.NotEmpty(t, strings.TrimSpace(binaryFile.ExplorationSummary.String))
	require.True(t, binaryFile.ExplorerUsed.Valid)
	require.NotEmpty(t, strings.TrimSpace(binaryFile.ExplorerUsed.String))
}

// mockTreeSitterParser is a minimal test implementation of treesitter.Parser.
type mockTreeSitterParser struct{}

func (m *mockTreeSitterParser) Analyze(_ context.Context, _ string, content []byte) (*treesitter.FileAnalysis, error) {
	return &treesitter.FileAnalysis{
		Language: "go",
		Symbols: []treesitter.SymbolInfo{
			{Name: "main", Kind: "function", Line: 1},
		},
		Imports: []treesitter.ImportInfo{
			{Path: "fmt", Category: treesitter.ImportCategoryStdlib},
		},
	}, nil
}

func (m *mockTreeSitterParser) Languages() []string {
	return []string{"go", "python", "javascript", "typescript", "rust", "java"}
}

func (m *mockTreeSitterParser) SupportsLanguage(lang string) bool {
	switch lang {
	case "go", "python", "javascript", "typescript", "rust", "java":
		return true
	default:
		return false
	}
}

func (m *mockTreeSitterParser) HasTags(lang string) bool {
	return m.SupportsLanguage(lang)
}

func (m *mockTreeSitterParser) Close() error {
	return nil
}

func TestMessageDecorator_Create_TreeSitterPath_WithParser_EndToEnd(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-treesitter"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)

	// Configure decorator with a tree-sitter parser to enable TreeSitterExplorer path.
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 10,
		Parser:                        &mockTreeSitterParser{},
	})

	// Create large Go tool output that will use TreeSitterExplorer.
	goCode := strings.Repeat(`package main

import "fmt"

type Point struct {
	X, Y int
}

func (p Point) String() string {
	return fmt.Sprintf("(%d,%d)", p.X, p.Y)
}

func main() {
	p := Point{X: 1, Y: 2}
	fmt.Println(p)
}
`, 100) // ~2500 tokens (well above threshold)

	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: goCode}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "[Large Tool Output Stored:")
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Verify exploration is persisted for tree-sitter path.
	require.True(t, files[0].ExplorationSummary.Valid, "Exploration summary should be non-null for tree-sitter path")
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorationSummary.String), "Exploration summary should not be empty")
	require.True(t, files[0].ExplorerUsed.Valid, "Explorer used should be non-null for tree-sitter path")
	require.NotEmpty(t, strings.TrimSpace(files[0].ExplorerUsed.String), "Explorer used should not be empty")

	// Verify tree-sitter is the explorer used (not go, treesitter).
	require.Contains(t, strings.ToLower(files[0].ExplorerUsed.String), "treesitter",
		"Explorer used should contain 'treesitter' when parser is available")

	require.NotContains(t, strings.ToLower(files[0].ExplorerUsed.String), "go",
		"Explorer used should not contain 'go' when tree-sitter catches Go code")
}

func TestMessageDecorator_Create_TreeSitterPath_WithoutParser_UsesNative(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-no-treesitter"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)

	// Configure decorator WITHOUT tree-sitter parser (default behavior).
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 10,
	})

	goCode := strings.Repeat(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`, 100)

	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: goCode}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "[Large Tool Output Stored:")

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Without parser, should use GoExplorer (native path), not TreeSitterExplorer.
	require.True(t, files[0].ExplorerUsed.Valid)
	explorerUsed := strings.ToLower(files[0].ExplorerUsed.String)
	require.Contains(t, explorerUsed, "go", "Without parser, should use GoExplorer")
	require.NotContains(t, explorerUsed, "treesitter", "Without parser, should not use TreeSitterExplorer")
}

func TestMessageDecorator_Create_RuntimeMatrix_NonPersistedPaths(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-nonpersisted-paths"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)
	cfg := MessageDecoratorConfig{LargeToolOutputTokenThreshold: 10}
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, cfg)

	// Test 1: Binary paths should be stored and exploration persisted in enhancement profile.
	binaryOutput := string([]byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00}) + strings.Repeat("\x00", 5000)
	msg, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: binaryOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	binaryFileIDs := ExtractFileIDs(msg.Content().Text)
	require.Len(t, binaryFileIDs, 1)
	binaryFile, err := queries.GetLcmLargeFile(ctx, binaryFileIDs[0])
	require.NoError(t, err)
	// Binary path is persisted in enhancement profile.
	require.True(t, binaryFile.ExplorationSummary.Valid, "Binary path should persist exploration_summary")
	require.NotEmpty(t, strings.TrimSpace(binaryFile.ExplorationSummary.String))
	require.True(t, binaryFile.ExplorerUsed.Valid, "Binary path should persist explorer_used")
	require.NotEmpty(t, strings.TrimSpace(binaryFile.ExplorerUsed.String))

	// Test 2: FallbackExplorer (retrieval path) should also be persisted in enhancement profile.
	// Use content that falls through to FallbackExplorer - random bytes that aren't ASCII text.
	fallbackOutput := strings.Repeat(string([]byte{0x80, 0x81, 0x82, 0x83}), 200)
	msg, err = svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: fallbackOutput}},
	})
	require.NoError(t, err)
	require.Contains(t, msg.Content().Text, "LCM File ID:")

	fallbackFileIDs := ExtractFileIDs(msg.Content().Text)
	require.Len(t, fallbackFileIDs, 1)
	fallbackFile, err := queries.GetLcmLargeFile(ctx, fallbackFileIDs[0])
	require.NoError(t, err)
	// Fallback path is persisted in enhancement profile.
	require.True(t, fallbackFile.ExplorationSummary.Valid, "Fallback path should persist exploration_summary")
	require.NotEmpty(t, strings.TrimSpace(fallbackFile.ExplorationSummary.String))
	require.True(t, fallbackFile.ExplorerUsed.Valid, "Fallback path should persist explorer_used")
	require.NotEmpty(t, strings.TrimSpace(fallbackFile.ExplorerUsed.String))

	// Test 3: Verify persisted paths DO store exploration (e.g., text/code).
	textOutput := strings.Repeat("This is plain text content.\n", 200)
	msg, err = svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: textOutput}},
	})
	require.NoError(t, err)

	textFileIDs := ExtractFileIDs(msg.Content().Text)
	require.Len(t, textFileIDs, 1)
	textFile, err := queries.GetLcmLargeFile(ctx, textFileIDs[0])
	require.NoError(t, err)
	// Text path is persisted: exploration fields should be non-NULL.
	require.True(t, textFile.ExplorationSummary.Valid, "Text path should persist exploration_summary")
	require.True(t, textFile.ExplorerUsed.Valid, "Text path should persist explorer_used")
}

func TestMessageDecorator_Create_ParacyProfile_DisablesPersistence(t *testing.T) {
	t.Parallel()

	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()
	sessionID := "sess-msgdecorator-parity-profile"
	createTestSession(t, queries, sessionID)

	inner := message.NewService(queries)
	mgr := NewManager(queries, sqlDB)

	// Even with parser and large content, parity profile should disable persistence.
	svc := NewMessageDecorator(inner, mgr, queries, sqlDB, MessageDecoratorConfig{
		LargeToolOutputTokenThreshold: 10,
		Parser:                        &mockTreeSitterParser{},
		ExplorerOutputProfile:         explorer.OutputProfileParity,
	})

	goCode := strings.Repeat(`package main

func main() {}
`, 100)

	_, err := svc.Create(ctx, sessionID, message.CreateMessageParams{
		Role:  message.Tool,
		Parts: []message.ContentPart{message.TextContent{Text: goCode}},
	})
	require.NoError(t, err)

	files, err := queries.ListLcmLargeFilesBySession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Parity profile: exploration should NOT be persisted even though storage succeeds.
	require.False(t, files[0].ExplorationSummary.Valid, "Parity profile should not persist exploration_summary")
	require.False(t, files[0].ExplorerUsed.Valid, "Parity profile should not persist explorer_used")

	// File should still be stored (LCM storage is independent of persistence matrix).
	require.NotEmpty(t, files[0].FileID)
	require.NotZero(t, files[0].TokenCount)
}
