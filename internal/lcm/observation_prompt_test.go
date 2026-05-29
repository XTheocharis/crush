package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetObservationPromptEmpty(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "obs-prompt-empty"
	createTestSession(t, queries, sessionID)

	mgr := NewManager(queries, sqlDB)
	prompt, err := mgr.GetObservationPrompt(ctx, sessionID, 2000)
	require.NoError(t, err)
	require.Empty(t, prompt)
}

func TestGetObservationPromptBudget(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "obs-prompt-budget"
	createTestSession(t, queries, sessionID)

	// Insert 5 observations: 2 high, 2 medium, 1 low with large content.
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "high-1: user decided to use PostgreSQL",
		Context:     strings.Repeat("A", 50),
		Implication: "Migration syntax needs updating for PostgreSQL",
		Priority:    0.9,
	}, "high")
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "high-2: auth module refactored to JWT",
		Context:     "internal/auth/jwt.go",
		Implication: "All middleware must verify JWT tokens",
		Priority:    0.8,
	}, "high")
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "medium-1: caching strategy discussed",
		Context:     "internal/cache/redis.go",
		Implication: "Need TTL configuration for cache entries",
		Priority:    0.5,
	}, "medium")
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "medium-2: error handling pattern established",
		Context:     "pkg/errors/errors.go",
		Implication: "Wrap errors with fmt.Errorf throughout",
		Priority:    0.4,
	}, "medium")
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "low-1: minor variable naming preference",
		Context:     "various files",
		Implication: "Prefer full words over abbreviations",
		Priority:    0.1,
	}, "low")

	mgr := NewManager(queries, sqlDB)

	// Tight budget: only fits the two high-priority observations.
	smallBudget := int64(80)
	prompt, err := mgr.GetObservationPrompt(ctx, sessionID, smallBudget)
	require.NoError(t, err)
	require.NotEmpty(t, prompt)
	require.Contains(t, prompt, "high-1")
	require.Contains(t, prompt, "high-2")
	// Medium and low should be excluded by budget truncation.
	require.NotContains(t, prompt, "medium-1")
	require.NotContains(t, prompt, "medium-2")
	require.NotContains(t, prompt, "low-1")

	// With zero budget, returns empty.
	prompt, err = mgr.GetObservationPrompt(ctx, sessionID, 0)
	require.NoError(t, err)
	require.Empty(t, prompt)
}

func TestGetObservationPromptAllIncluded(t *testing.T) {
	t.Parallel()
	queries, sqlDB := setupTestDB(t)
	ctx := context.Background()

	sessionID := "obs-prompt-all"
	createTestSession(t, queries, sessionID)

	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "switched to module approach",
		Context:     "go.mod",
		Implication: "Need to update import paths",
		Priority:    0.7,
	}, "high")
	insertTestObservation(t, sqlDB, sessionID, Observation{
		Event:       "added logging middleware",
		Context:     "internal/middleware/logging.go",
		Implication: "Log all HTTP requests",
		Priority:    0.3,
	}, "medium")

	mgr := NewManager(queries, sqlDB)

	// Large budget fits everything.
	prompt, err := mgr.GetObservationPrompt(ctx, sessionID, 10000)
	require.NoError(t, err)
	require.NotEmpty(t, prompt)
	require.Contains(t, prompt, "switched to module approach")
	require.Contains(t, prompt, "added logging middleware")
}

func insertTestObservation(t *testing.T, db *sql.DB, sessionID string, obs Observation, priority string) {
	t.Helper()
	content, err := json.Marshal(obs)
	require.NoError(t, err)
	tokenCount := EstimateTokens(string(content))
	id := fmt.Sprintf("test_obs_%s_%d", priority, tokenCount)
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content, token_count, priority)
		 VALUES (?, ?, 'observation', ?, ?, ?)`,
		id, sessionID, string(content), tokenCount, priority,
	)
	require.NoError(t, err)
}
