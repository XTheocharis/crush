package lcm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/stretchr/testify/require"
)

func TestNewObservationCoordinator_DefaultThreshold(t *testing.T) {
	t.Parallel()
	_, store := setupObservationTestDB(t)
	oc := NewObservationCoordinator(store, &mockLLMClient{}, 0, nil)
	require.Equal(t, int64(DefaultObservationTokenThreshold), oc.Threshold())
}

func TestNewObservationCoordinator_CustomThreshold(t *testing.T) {
	t.Parallel()
	_, store := setupObservationTestDB(t)
	oc := NewObservationCoordinator(store, &mockLLMClient{}, 50_000, nil)
	require.Equal(t, int64(50_000), oc.Threshold())
}

func TestObservationCoordinator_ShouldObserve_BelowThreshold(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-1"
	createTestSession(t, queries, sessionID)

	createTestMessage(t, queries, sessionID, "msg-1", "user", "hello")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "hi there")

	oc := NewObservationCoordinator(store, &mockLLMClient{}, DefaultObservationTokenThreshold, nil)
	should, err := oc.ShouldObserve(ctx, sessionID)
	require.NoError(t, err)
	require.False(t, should)
}

func TestObservationCoordinator_Observe_StoresObservations(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-2"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "I want to refactor the auth module to use JWT")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "Let's update internal/auth/jwt.go for that")

	obsJSON := `[{"event":"User decided to refactor auth to use JWT","context":"internal/auth/jwt.go","implication":"Need to update all auth middleware"}]`
	mockLLM := &mockLLMClient{response: obsJSON}

	oc := NewObservationCoordinator(store, mockLLM, 0, nil)

	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Observations, 1)
	require.Equal(t, "User decided to refactor auth to use JWT", result.Observations[0].Event)
	require.Equal(t, "internal/auth/jwt.go", result.Observations[0].Context)
	require.Equal(t, "Need to update all auth middleware", result.Observations[0].Implication)

	stored, err := oc.ListObservations(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "User decided to refactor auth to use JWT", stored[0].Event)
}

func TestObservationCoordinator_Observe_MultipleObservations(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-3"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "switching to postgres")
	createTestMessage(t, queries, sessionID, "msg-2", "assistant", "ok updating migrations")

	obsJSON := `[
		{"event":"Switched to PostgreSQL","context":"internal/db/migrations/","implication":"Migration syntax needs updating"},
		{"event":"Connection pooling discussed","context":"internal/db/connect.go","implication":"May need pgxpool"}
	]`
	mockLLM := &mockLLMClient{response: obsJSON}

	oc := NewObservationCoordinator(store, mockLLM, 0, nil)
	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Observations, 2)

	stored, err := oc.ListObservations(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, stored, 2)
}

func TestObservationCoordinator_Observe_MarkdownCodeBlockResponse(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-4"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "let's add caching")

	obsJSON := "```json\n[{\"event\":\"Caching added\",\"context\":\"internal/cache/\",\"implication\":\"Need cache invalidation strategy\"}]\n```"
	mockLLM := &mockLLMClient{response: obsJSON}

	oc := NewObservationCoordinator(store, mockLLM, 0, nil)
	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Observations, 1)
	require.Equal(t, "Caching added", result.Observations[0].Event)
}

func TestObservationCoordinator_Observe_FallbackOnBadJSON(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-5"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "refactor needed")

	mockLLM := &mockLLMClient{response: "This is not valid JSON at all!"}

	oc := NewObservationCoordinator(store, mockLLM, 0, nil)
	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Observations, 1)
	require.Equal(t, "raw_observation", result.Observations[0].Event)
}

func TestObservationCoordinator_Observe_NilLLMClient(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-nil-llm"
	createTestSession(t, queries, sessionID)

	oc := NewObservationCoordinator(store, nil, 0, nil)
	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
	require.True(t, errors.Is(result.Error, ErrLLMClientNil))
}

func TestObservationCoordinator_Observe_DoesNotBlock(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-async"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "test async")

	slowLLM := &slowMockLLMClient{delay: 500 * time.Millisecond, response: `[]`}

	oc := NewObservationCoordinator(store, slowLLM, 0, nil)

	start := time.Now()
	ch := oc.Observe(ctx, sessionID)
	elapsed := time.Since(start)

	require.NotNil(t, ch)
	require.Less(t, elapsed, 50*time.Millisecond, "Observe should return immediately")

	select {
	case result := <-ch:
		require.NoError(t, result.Error)
	case <-time.After(2 * time.Second):
		t.Fatal("observation did not complete within timeout")
	}
}

func TestObservationCoordinator_Observe_DeduplicatesSessions(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-dedup"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "test dedup")

	var callCount atomic.Int32
	countingLLM := &countingMockLLMClient{response: `[]`, callCount: &callCount}

	oc := NewObservationCoordinator(store, countingLLM, 0, nil)

	ch1 := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch1)

	ch2 := oc.Observe(ctx, sessionID)
	require.Nil(t, ch2)

	<-ch1
	require.Equal(t, int32(1), callCount.Load())
}

func TestObservationCoordinator_Observe_LLMError(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-llm-err"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg-1", "user", "trigger error")

	mockLLM := &mockLLMClient{err: fmt.Errorf("LLM unavailable")}

	oc := NewObservationCoordinator(store, mockLLM, 0, nil)
	ch := oc.Observe(ctx, sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
	require.Contains(t, result.Error.Error(), "LLM unavailable")
}

func TestObservationCoordinator_SetLLMClient(t *testing.T) {
	t.Parallel()
	_, store := setupObservationTestDB(t)

	oc := NewObservationCoordinator(store, nil, 0, nil)
	require.Nil(t, oc.llm)

	newLLM := &mockLLMClient{}
	oc.SetLLMClient(newLLM)
	require.NotNil(t, oc.llm)
}

func TestObservationCoordinator_ListObservations_Empty(t *testing.T) {
	t.Parallel()
	queries, store := setupObservationTestDB(t)
	ctx := context.Background()

	sessionID := "obs-test-session-empty"
	createTestSession(t, queries, sessionID)

	oc := NewObservationCoordinator(store, &mockLLMClient{}, 0, nil)
	stored, err := oc.ListObservations(ctx, sessionID)
	require.NoError(t, err)
	require.Empty(t, stored)
}

func TestParseObservations_ValidJSON(t *testing.T) {
	t.Parallel()
	raw := `[{"event":"e1","context":"c1","implication":"i1"},{"event":"e2","context":"c2","implication":"i2"}]`
	obs, err := parseObservations(raw)
	require.NoError(t, err)
	require.Len(t, obs, 2)
	require.Equal(t, "e1", obs[0].Event)
	require.Equal(t, "c2", obs[1].Context)
}

func TestParseObservations_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := parseObservations("not json")
	require.Error(t, err)
}

func TestParseObservations_EmptyArray(t *testing.T) {
	t.Parallel()
	obs, err := parseObservations("[]")
	require.NoError(t, err)
	require.Empty(t, obs)
}

func TestParseObservations_CodeBlock(t *testing.T) {
	t.Parallel()
	raw := "```json\n[{\"event\":\"e\",\"context\":\"c\",\"implication\":\"i\"}]\n```"
	obs, err := parseObservations(raw)
	require.NoError(t, err)
	require.Len(t, obs, 1)
	require.Equal(t, "e", obs[0].Event)
}

func setupObservationTestDB(t *testing.T) (*db.Queries, *Store) {
	t.Helper()
	queries, sqlDB := setupTestDB(t)
	store := newStore(queries, sqlDB)
	return queries, store
}

type slowMockLLMClient struct {
	delay    time.Duration
	response string
}

func (m *slowMockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	time.Sleep(m.delay)
	return m.response, nil
}

type countingMockLLMClient struct {
	response  string
	callCount *atomic.Int32
}

func (m *countingMockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	m.callCount.Add(1)
	return m.response, nil
}
