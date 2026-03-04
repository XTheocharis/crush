package lcm

import (
	"context"
	"testing"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestSetOverheadTokens(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB).(*compactionManager)

	mgr.SetOverheadTokens(7000, 12000)
	require.Equal(t, int64(7000), mgr.defaultSystemPromptTokens)
	require.Equal(t, int64(12000), mgr.defaultToolTokens)
}

func TestSetOverheadTokens_AffectsBudget(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-overhead"
	createTestSession(t, queries, sessionID)
	require.NoError(t, mgr.InitSession(ctx, sessionID))

	budgetBefore, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	// Set overhead and recompute via UpdateContextWindow (which recomputes all budgets).
	mgr.SetOverheadTokens(5000, 10000)
	require.NoError(t, mgr.UpdateContextWindow(ctx, 128000))

	budgetAfter, err := mgr.GetBudget(ctx, sessionID)
	require.NoError(t, err)

	// Budget should be smaller after setting overhead.
	require.Less(t, budgetAfter.HardLimit, budgetBefore.HardLimit)
	require.Less(t, budgetAfter.SoftThreshold, budgetBefore.SoftThreshold)
}

func TestGetSummaryMentionedPaths_NoPaths(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-no-paths"
	createTestSession(t, queries, sessionID)

	paths, err := mgr.GetSummaryMentionedPaths(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, paths)
}

func TestGetSummaryMentionedPaths_ExtractsPaths(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-extract-paths"
	createTestSession(t, queries, sessionID)

	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  "sum_pathtest1234567",
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "User edited internal/lcm/manager.go and internal/agent/coordinator.go to add overhead tracking.",
		TokenCount: 20,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	paths, err := mgr.GetSummaryMentionedPaths(ctx, sessionID)
	require.NoError(t, err)
	require.Contains(t, paths, "internal/lcm/manager.go")
	require.Contains(t, paths, "internal/agent/coordinator.go")
}

func TestGetSummaryMentionedPaths_Deduplicates(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-dedup-paths"
	createTestSession(t, queries, sessionID)

	// Two summaries mentioning the same file.
	for i, id := range []string{"sum_dedup00000000001", "sum_dedup00000000002"} {
		err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
			SummaryID:  id,
			SessionID:  sessionID,
			Kind:       KindLeaf,
			Content:    "Modified config.go for budget changes.",
			TokenCount: int64(10 + i),
			FileIds:    "[]",
		})
		require.NoError(t, err)
	}

	paths, err := mgr.GetSummaryMentionedPaths(ctx, sessionID)
	require.NoError(t, err)

	// Should have config.go only once.
	count := 0
	for _, p := range paths {
		if p == "config.go" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestGetSummaryMentionedPaths_MultipleExtensions(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	mgr := NewManager(queries, sqlDB)
	ctx := context.Background()

	sessionID := "sess-multi-ext"
	createTestSession(t, queries, sessionID)

	err := queries.InsertLcmSummary(ctx, db.InsertLcmSummaryParams{
		SummaryID:  "sum_multiext000000001",
		SessionID:  sessionID,
		Kind:       KindLeaf,
		Content:    "Files: src/app.ts, styles/main.css, db/schema.sql, config.yaml, Makefile",
		TokenCount: 15,
		FileIds:    "[]",
	})
	require.NoError(t, err)

	paths, err := mgr.GetSummaryMentionedPaths(ctx, sessionID)
	require.NoError(t, err)
	require.Contains(t, paths, "src/app.ts")
	require.Contains(t, paths, "styles/main.css")
	require.Contains(t, paths, "db/schema.sql")
	require.Contains(t, paths, "config.yaml")
	// "Makefile" has no recognized extension, should NOT be matched.
	for _, p := range paths {
		require.NotEqual(t, "Makefile", p)
	}
}

func TestFilePathPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantNot []string
	}{
		{
			name:  "basic go file",
			input: "edited internal/lcm/manager.go",
			want:  []string{"internal/lcm/manager.go"},
		},
		{
			name:  "multiple files",
			input: "Changed foo.py and bar.rs",
			want:  []string{"foo.py", "bar.rs"},
		},
		{
			name:    "no extensions",
			input:   "Modified Makefile and README",
			want:    nil,
			wantNot: []string{"Makefile", "README"},
		},
		{
			name:  "path in backticks",
			input: "See `internal/config.go` for details",
			want:  []string{"internal/config.go"},
		},
		{
			name:  "path in quotes",
			input: `Changed "src/app.ts" and 'lib/util.js'`,
			want:  []string{"src/app.ts", "lib/util.js"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			matches := filePathPattern.FindAllStringSubmatch(tt.input, -1)
			var found []string
			for _, m := range matches {
				if len(m) > 1 {
					found = append(found, m[1])
				}
			}
			for _, w := range tt.want {
				require.Contains(t, found, w)
			}
			for _, w := range tt.wantNot {
				require.NotContains(t, found, w)
			}
		})
	}
}
