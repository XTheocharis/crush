package lcm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestNewReflectorAgent_DefaultThreshold(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 0)
	require.Equal(t, int64(DefaultReflectionTokenThreshold), ra.Threshold())
}

func TestNewReflectorAgent_CustomThreshold(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 50_000)
	require.Equal(t, int64(50_000), ra.Threshold())
}

func TestReflectorAgent_ShouldReflect_BelowThreshold(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-1"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "hello")

	ra := NewReflectorAgent(store, &mockLLMClient{}, DefaultReflectionTokenThreshold)
	should, err := ra.ShouldReflect(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, should)
}

func TestReflectorAgent_Reflect_NoObservations(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-no-obs"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "hello")

	mockLLM := &mockLLMClient{response: "[]"}
	ra := NewReflectorAgent(store, mockLLM, 0)

	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Empty(t, result.Reflections)
	require.Equal(t, 0, mockLLM.callCount, "LLM should not be called with no observations")
}

func TestReflectorAgent_Reflect_StoresReflections(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-2"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "I want to switch to PostgreSQL")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "Let's update internal/db/migrations/")

	// Insert observations using the ObservationCoordinator.
	obsJSON := `[{"event":"Switched to PostgreSQL","context":"internal/db/migrations/","implication":"Migration syntax needs updating"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	obsResult := <-obsCh
	require.NoError(t, obsResult.Error)
	require.Len(t, obsResult.Observations, 1)

	// Now reflect on those observations.
	reflJSON := `[{"insight":"Database migration strategy is shifting to PostgreSQL","confidence":0.9,"action_suggestion":"Review all migration files for SQLite-specific syntax"}]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)

	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)
	require.Equal(t, "Database migration strategy is shifting to PostgreSQL", result.Reflections[0].Insight)
	require.InDelta(t, 0.9, result.Reflections[0].Confidence, 0.01)
	require.Equal(t, "Review all migration files for SQLite-specific syntax", result.Reflections[0].ActionSuggestion)

	// Verify reflections are persisted.
	stored, err := ra.ListReflections(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "Database migration strategy is shifting to PostgreSQL", stored[0].Insight)
}

func TestReflectorAgent_Reflect_MultipleReflections(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-multi"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "add caching and refactor auth")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "updating internal/cache/ and internal/auth/")

	// Insert multiple observations.
	obsJSON := `[
		{"event":"Added caching layer","context":"internal/cache/","implication":"Need invalidation strategy"},
		{"event":"Refactored auth middleware","context":"internal/auth/","implication":"JWT tokens need rotation"}
	]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	obsResult := <-obsCh
	require.NoError(t, obsResult.Error)
	require.Len(t, obsResult.Observations, 2)

	// Reflect on them.
	reflJSON := `[
		{"insight":"Performance and security are both being addressed","confidence":0.85,"action_suggestion":"Prioritize cache invalidation before auth token rotation"},
		{"insight":"Middleware refactoring may affect existing tests","confidence":0.7,"action_suggestion":"Run full test suite after auth changes"}
	]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)

	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 2)
}

func TestReflectorAgent_Reflect_MarksObservationsReflected(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-mark"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "switch to go modules")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "updating go.mod")

	// Insert observations.
	obsJSON := `[{"event":"Switched to Go modules","context":"go.mod","implication":"Dependency management changed"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	// Verify observations are in 'observation' buffer_type.
	unreflected, err := store.rawDB.QueryContext(ctx,
		`SELECT COUNT(*) FROM lcm_observation_buffer WHERE session_id = ? AND buffer_type = 'observation'`,
		sessionID,
	)
	require.NoError(t, err)
	defer unreflected.Close()
	var count int
	require.True(t, unreflected.Next())
	require.NoError(t, unreflected.Scan(&count))
	require.NoError(t, unreflected.Err())
	require.Equal(t, 1, count)

	// Reflect.
	reflJSON := `[{"insight":"Module system migration complete","confidence":0.8,"action_suggestion":"Verify all imports"}]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)
	<-ch

	// Verify observations are now marked as 'summary'.
	unreflected, err = store.rawDB.QueryContext(ctx,
		`SELECT COUNT(*) FROM lcm_observation_buffer WHERE session_id = ? AND buffer_type = 'observation'`,
		sessionID,
	)
	require.NoError(t, err)
	defer unreflected.Close()
	require.True(t, unreflected.Next())
	require.NoError(t, unreflected.Scan(&count))
	require.NoError(t, unreflected.Err())
	require.Equal(t, 0, count, "observations should be marked as reflected")
}

func TestReflectorAgent_Reflect_DoesNotDiscardObservations(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-keep"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "add test coverage")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "updating internal/coverage/")

	// Insert observations.
	obsJSON := `[{"event":"Added test coverage","context":"internal/coverage/","implication":"CI pipeline updated"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	// Reflect.
	reflJSON := `[{"insight":"Test coverage improved","confidence":0.95,"action_suggestion":"Add coverage threshold to CI"}]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)
	<-ch

	// Verify original observations still exist (now as 'insight' buffer_type).
	total, err := store.rawDB.QueryContext(ctx,
		`SELECT COUNT(*) FROM lcm_observation_buffer WHERE session_id = ?`,
		sessionID,
	)
	require.NoError(t, err)
	defer total.Close()
	var count int
	require.True(t, total.Next())
	require.NoError(t, total.Scan(&count))
	require.NoError(t, total.Err())
	require.Equal(t, 2, count, "should have 1 observation (marked insight) + 1 reflection")
}

func TestReflectorAgent_Reflect_MarkdownCodeBlockResponse(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-codeblock"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "add retry logic")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "updating internal/retry/")

	// Insert observations.
	obsJSON := `[{"event":"Added retry logic","context":"internal/retry/","implication":"Need backoff config"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	reflJSON := "```json\n[{\"insight\":\"Retry mechanism uses exponential backoff\",\"confidence\":0.88,\"action_suggestion\":\"Configure max retry count\"}]\n```"
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)
	require.Equal(t, "Retry mechanism uses exponential backoff", result.Reflections[0].Insight)
}

func TestReflectorAgent_Reflect_FallbackOnBadJSON(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-badjson"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "something happened")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok noted")

	// Insert observations.
	obsJSON := `[{"event":"Something happened","context":"somewhere","implication":"some implication"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	// LLM returns invalid JSON for reflection.
	reflMockLLM := &mockLLMClient{response: "This is not JSON at all!"}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)
	require.Contains(t, result.Reflections[0].Insight, "This is not JSON")
	require.InDelta(t, 0.5, result.Reflections[0].Confidence, 0.01)
}

func TestReflectorAgent_Reflect_NilLLMClient(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-nil-llm"
	createTestSession(t, queries, sessionID)

	ra := NewReflectorAgent(store, nil, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
	require.True(t, errors.Is(result.Error, ErrLLMClientNil))
}

func TestReflectorAgent_Reflect_DoesNotBlock(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-async"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "async test message")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok noted")

	// Insert observations.
	obsJSON := `[{"event":"async test","context":"test","implication":"test"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	slowLLM := &slowMockLLMClient{delay: 500 * time.Millisecond, response: `[]`}
	ra := NewReflectorAgent(store, slowLLM, 0)

	start := time.Now()
	ch := ra.Reflect(ctx, sessionID)
	elapsed := time.Since(start)

	require.NotNil(t, ch)
	require.Less(t, elapsed, 50*time.Millisecond, "Reflect should return immediately")

	select {
	case result := <-ch:
		require.NoError(t, result.Error)
	case <-time.After(2 * time.Second):
		t.Fatal("reflection did not complete within timeout")
	}
}

func TestReflectorAgent_Reflect_DeduplicatesSessions(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-dedup"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "dedup test")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok")

	// Insert observations.
	obsJSON := `[{"event":"dedup test","context":"test","implication":"test"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	var callCount atomic.Int32
	countingLLM := &countingMockLLMClient{response: `[]`, callCount: &callCount}
	ra := NewReflectorAgent(store, countingLLM, 0)

	ch1 := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch1)

	ch2 := ra.Reflect(ctx, sessionID)
	require.Nil(t, ch2, "second call should return nil channel")

	<-ch1
	require.Equal(t, int32(1), callCount.Load())
}

func TestReflectorAgent_Reflect_LLMError(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-llm-err"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "trigger error")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok")

	// Insert observations.
	obsJSON := `[{"event":"error test","context":"test","implication":"test"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	reflMockLLM := &mockLLMClient{err: fmt.Errorf("LLM unavailable")}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
	require.Contains(t, result.Error.Error(), "LLM unavailable")
}

func TestReflectorAgent_SetLLMClient(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)

	ra := NewReflectorAgent(store, nil, 0)
	require.Nil(t, ra.llm)

	newLLM := &mockLLMClient{}
	ra.SetLLMClient(newLLM)
	require.NotNil(t, ra.llm)
}

func TestReflectorAgent_ListReflections_Empty(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-empty"
	createTestSession(t, queries, sessionID)

	ra := NewReflectorAgent(store, &mockLLMClient{}, 0)
	stored, err := ra.ListReflections(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, stored)
}

func TestParseReflections_ValidJSON(t *testing.T) {
	t.Parallel()
	raw := `[{"insight":"pattern found","confidence":0.85,"action_suggestion":"do X"},{"insight":"another pattern","confidence":0.6,"action_suggestion":"do Y"}]`
	refs, err := parseReflections(raw)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.Equal(t, "pattern found", refs[0].Insight)
	require.InDelta(t, 0.85, refs[0].Confidence, 0.01)
	require.Equal(t, "do Y", refs[1].ActionSuggestion)
}

func TestParseReflections_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := parseReflections("not json")
	require.Error(t, err)
}

func TestParseReflections_EmptyArray(t *testing.T) {
	t.Parallel()
	refs, err := parseReflections("[]")
	require.NoError(t, err)
	require.Empty(t, refs)
}

func TestParseReflections_CodeBlock(t *testing.T) {
	t.Parallel()
	raw := "```json\n[{\"insight\":\"test\",\"confidence\":0.5,\"action_suggestion\":\"test\"}]\n```"
	refs, err := parseReflections(raw)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, "test", refs[0].Insight)
}

func TestParseReflections_ConfidenceClamping(t *testing.T) {
	t.Parallel()
	raw := `[{"insight":"high","confidence":1.5,"action_suggestion":"test"},{"insight":"low","confidence":-0.3,"action_suggestion":"test"}]`
	refs, err := parseReflections(raw)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.InDelta(t, 1.0, refs[0].Confidence, 0.01)
	require.InDelta(t, 0.0, refs[1].Confidence, 0.01)
}

func TestFormatObservationsForReflection(t *testing.T) {
	t.Parallel()
	observations := []Observation{
		{Event: "switched DB", Context: "internal/db/", Implication: "migration needed"},
		{Event: "added caching", Context: "internal/cache/", Implication: "invalidation needed"},
	}
	result := formatObservationsForReflection(observations)
	require.Contains(t, result, "<observations>")
	require.Contains(t, result, "switched DB")
	require.Contains(t, result, "internal/cache/")
	require.Contains(t, result, "</observations>")
}

// --- BufferingCoordinator tests ---

func TestBufferingCoordinator_CurrentInterval(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	require.Equal(t, -1, bc.CurrentInterval(0))
	require.Equal(t, -1, bc.CurrentInterval(-1))

	// 20% of 40K = 8000 tokens per interval.
	require.Equal(t, 0, bc.CurrentInterval(1))      // 1/8000 = 0
	require.Equal(t, 0, bc.CurrentInterval(7999))   // 7999/8000 = 0
	require.Equal(t, 1, bc.CurrentInterval(8000))   // 8000/8000 = 1
	require.Equal(t, 1, bc.CurrentInterval(15999))  // 15999/8000 = 1
	require.Equal(t, 2, bc.CurrentInterval(16000))  // 16000/8000 = 2
	require.Equal(t, 3, bc.CurrentInterval(24000))  // 24000/8000 = 3
	require.Equal(t, 4, bc.CurrentInterval(32000))  // 32000/8000 = 4
	require.Equal(t, 4, bc.CurrentInterval(100000)) // capped at 4
}

func TestBufferingCoordinator_ShouldCollect(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-1"
	createTestSession(t, queries, sessionID)

	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	// Initially, no context items means token count is 0.
	should, err := bc.ShouldCollect(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, should)
}

func TestBufferingCoordinator_Collect_BelowThreshold(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-below"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "hello")

	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	ch, err := bc.Collect(ctx, sessionID)
	require.NoError(t, err)
	require.Nil(t, ch, "should not trigger reflection below threshold")
}

func TestBufferingCoordinator_Collect_AtThreshold(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-at"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "threshold test")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok")

	// Insert observations first.
	obsJSON := `[{"event":"threshold test","context":"test","implication":"test"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	// Insert a context item so GetContextTokenCount returns a non-zero value.
	err := queries.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  sessionID,
		Position:   0,
		ItemType:   "message",
		MessageID:  sql.NullString{String: "msg-1", Valid: true},
		TokenCount: 50,
	})
	require.NoError(t, err)

	// Use a very small threshold so the context item's token count exceeds it.
	reflJSON := `[{"insight":"small threshold test","confidence":0.8,"action_suggestion":"test"}]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 10)
	bc := NewBufferingCoordinator(store, ra)

	ch, err := bc.Collect(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, ch, "should trigger reflection at threshold")

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)
}

func TestBufferingCoordinator_Collect_NoProgress(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-noprogress"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "hi")

	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	// First collect sets the interval.
	_, err := bc.Collect(ctx, sessionID)
	require.NoError(t, err)

	// Second collect with no progress returns nil.
	ch, err := bc.Collect(ctx, sessionID)
	require.NoError(t, err)
	require.Nil(t, ch)
}

func TestBufferingCoordinator_Flush(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-flush"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "flush test")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok")

	// Insert observations.
	obsJSON := `[{"event":"flush test","context":"test","implication":"test"}]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	<-obsCh

	reflJSON := `[{"insight":"flushed","confidence":0.9,"action_suggestion":"done"}]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	// Force flush regardless of threshold.
	ch := bc.Flush(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)
	require.Equal(t, "flushed", result.Reflections[0].Insight)
}

func TestBufferingCoordinator_Reset(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "buf-test-session-reset"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "hello")

	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	// Simulate progress.
	_, err := bc.Collect(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, 0, bc.intervals[sessionID])

	// Reset.
	bc.Reset(sessionID)
	_, exists := bc.intervals[sessionID]
	require.False(t, exists)
}

func TestBufferingCoordinator_Collect_IntervalProgression(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	// Simulate interval progression.
	require.Equal(t, 0, bc.CurrentInterval(1000))   // interval 0
	require.Equal(t, 1, bc.CurrentInterval(8000))   // interval 1
	require.Equal(t, 2, bc.CurrentInterval(16000))  // interval 2
	require.Equal(t, 3, bc.CurrentInterval(24000))  // interval 3
	require.Equal(t, 4, bc.CurrentInterval(32000))  // interval 4
	require.Equal(t, 4, bc.CurrentInterval(40000))  // capped at 4
	require.Equal(t, 4, bc.CurrentInterval(100000)) // capped at 4
}

func TestBufferingCoordinator_NilReflector(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	bc := NewBufferingCoordinator(store, nil)
	require.Equal(t, -1, bc.CurrentInterval(1000))
}

// --- Reflection quality test ---

func TestReflectorAgent_ReflectionQuality(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-session-quality"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "migrating to PostgreSQL")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "updating store and migrations")

	// Insert diverse observations.
	obsJSON := `[
		{"event":"Switched to PostgreSQL from SQLite","context":"internal/db/store.go, internal/db/migrations/","implication":"Migration files need PostgreSQL syntax review"},
		{"event":"Added connection pooling via pgxpool","context":"internal/db/connect.go","implication":"Connection lifecycle management changed"},
		{"event":"Updated test suite for new DB driver","context":"internal/db/store_test.go","implication":"Tests now require PostgreSQL test container"}
	]`
	obsMockLLM := &mockLLMClient{response: obsJSON}
	oc := NewObservationCoordinator(store, obsMockLLM, 0, nil)
	obsCh := oc.Observe(ctx, sessionID)
	require.NotNil(t, obsCh)
	obsResult := <-obsCh
	require.NoError(t, obsResult.Error)
	require.Len(t, obsResult.Observations, 3)

	// Reflect with a quality response.
	reflJSON := `[
		{
			"insight": "The database layer is undergoing a complete migration from SQLite to PostgreSQL, affecting store, migrations, and connection management",
			"confidence": 0.92,
			"action_suggestion": "Review all migration files for SQLite-specific syntax, update connection pooling configuration, and ensure test infrastructure supports PostgreSQL containers"
		}
	]`
	reflMockLLM := &mockLLMClient{response: reflJSON}
	ra := NewReflectorAgent(store, reflMockLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Reflections, 1)

	ref := result.Reflections[0]
	require.NotEmpty(t, ref.Insight)
	require.InDelta(t, 0.92, ref.Confidence, 0.01)
	require.NotEmpty(t, ref.ActionSuggestion)
	require.Contains(t, ref.Insight, "PostgreSQL")
	require.Contains(t, ref.ActionSuggestion, "migration files")
}

// --- CompressionLevel tests ---

func TestDetermineCompressionLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		tokenCount int
		want       CompressionLevel
	}{
		{"zero tokens", 0, LevelNormal},
		{"below 30K", 29_999, LevelNormal},
		{"at 30K", 30_000, LevelExtractive},
		{"below 40K", 39_999, LevelExtractive},
		{"at 40K", 40_000, LevelAggressive},
		{"below 50K", 49_999, LevelAggressive},
		{"at 50K", 50_000, LevelSkeleton},
		{"below 60K", 59_999, LevelSkeleton},
		{"at 60K", 60_000, LevelDeterministic},
		{"well above 60K", 100_000, LevelDeterministic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := determineCompressionLevel(tt.tokenCount)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReflectionPromptForLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		lvl  CompressionLevel
		want string
	}{
		{"normal returns default prompt", LevelNormal, reflectionSystemPrompt},
		{"extractive", LevelExtractive, extractiveReflectionPrompt},
		{"aggressive", LevelAggressive, aggressiveReflectionPrompt},
		{"skeleton", LevelSkeleton, skeletonReflectionPrompt},
		{"deterministic", LevelDeterministic, deterministicReflectionPrompt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := reflectionPromptForLevel(tt.lvl)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReflectorAgent_Reflect_UsesCompressionLevel(t *testing.T) {
	t.Parallel()
	queries, store := setupReflectorTestDB(t)
	ctx := context.Background()

	sessionID := "refl-test-compression"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "test compression")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok")

	// Insert observations directly into the buffer instead of using
	// ObservationCoordinator, which has a known goroutine leak in tests.
	obs := Observation{Event: "compression test", Context: "test", Implication: "test"}
	obsJSON, err := json.Marshal(obs)
	require.NoError(t, err)
	_, err = store.rawDB.ExecContext(ctx,
		`INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content, token_count, priority)
		 VALUES (?, ?, 'observation', ?, ?, 'medium')`,
		"obs-compression-1", sessionID, string(obsJSON), 10,
	)
	require.NoError(t, err)

	var capturedSystemPrompt string
	captureLLM := &capturingMockLLMClient{
		response: `[{"insight":"captured","confidence":0.8,"action_suggestion":"test"}]`,
		onComplete: func(systemPrompt string) {
			capturedSystemPrompt = systemPrompt
		},
	}
	ra := NewReflectorAgent(store, captureLLM, 0)
	ch := ra.Reflect(ctx, sessionID)
	require.NotNil(t, ch)
	result := <-ch
	require.NoError(t, result.Error)
	require.NotEmpty(t, capturedSystemPrompt)
	require.Equal(t, reflectionSystemPrompt, capturedSystemPrompt,
		"with low token count, should use normal (default) prompt")
}

// --- ReflectorThresholdPercent tests ---

func TestBufferingCoordinator_50PercentIntervals(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinatorWithPercent(store, ra, ReflectorThresholdPercent)

	require.Equal(t, -1, bc.CurrentInterval(0))
	require.Equal(t, 0, bc.CurrentInterval(1))
	require.Equal(t, 0, bc.CurrentInterval(19_999))
	require.Equal(t, 1, bc.CurrentInterval(20_000))
	require.Equal(t, 1, bc.CurrentInterval(39_999))
	require.Equal(t, 1, bc.CurrentInterval(40_000))
	require.Equal(t, 1, bc.CurrentInterval(100_000))
}

func TestBufferingCoordinator_20PercentIntervals(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)
	bc := NewBufferingCoordinator(store, ra)

	require.Equal(t, 0, bc.CurrentInterval(1))
	require.Equal(t, 1, bc.CurrentInterval(8000))
	require.Equal(t, 2, bc.CurrentInterval(16000))
	require.Equal(t, 3, bc.CurrentInterval(24000))
	require.Equal(t, 4, bc.CurrentInterval(32000))
	require.Equal(t, 4, bc.CurrentInterval(100_000))
}

func TestNewBufferingCoordinatorWithPercent(t *testing.T) {
	t.Parallel()
	_, store := setupReflectorTestDB(t)
	ra := NewReflectorAgent(store, &mockLLMClient{}, 40_000)

	bc := NewBufferingCoordinatorWithPercent(store, ra, 0.5)
	require.Equal(t, 0.5, bc.thresholdPercent)

	bcDefault := NewBufferingCoordinator(store, ra)
	require.Equal(t, BufferThresholdPercent, bcDefault.thresholdPercent)
}

// --- Helper ---

type capturingMockLLMClient struct {
	response   string
	err        error
	onComplete func(systemPrompt string)
	callCount  int
}

func (m *capturingMockLLMClient) Complete(_ context.Context, systemPrompt, _ string) (string, error) {
	m.callCount++
	if m.onComplete != nil {
		m.onComplete(systemPrompt)
	}
	return m.response, m.err
}

func setupReflectorTestDB(t *testing.T) (*db.Queries, *Store) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	return queries, store
}
