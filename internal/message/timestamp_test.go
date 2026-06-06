package message

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTimestamp_LifecycleMonotonicity verifies that the four lifecycle
// timestamps advance monotonically through the message lifecycle:
//
//	submitted_at <= sent_to_llm_at <= first_token_at <= completed_at
//
// We simulate the full lifecycle: create (submitted), send to LLM, first
// token, and completion. Zero-debounce mode ensures every Update lands
// synchronously so we can read back from the DB immediately.
func TestTimestamp_LifecycleMonotonicity(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(0))

	now := time.Now().Unix()

	// Step 1: Create message with submitted_at set.
	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:        Assistant,
		SubmittedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, now, msg.SubmittedAt, "submitted_at should be set at creation")
	require.Zero(t, msg.SentToLLMAt, "sent_to_llm_at should be zero before send")
	require.Zero(t, msg.FirstTokenAt, "first_token_at should be zero before first token")
	require.Zero(t, msg.CompletedAt, "completed_at should be zero before completion")

	// Step 2: Simulate send to LLM, set sent_to_llm_at.
	sentAt := now + 1
	msg.SentToLLMAt = sentAt
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)
	require.Equal(t, sentAt, got.SentToLLMAt, "sent_to_llm_at should be persisted")

	// Step 3: Simulate first token arrival.
	firstTokenAt := now + 2
	msg.FirstTokenAt = firstTokenAt
	msg.AppendContent("Hello")
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err = svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)
	require.Equal(t, firstTokenAt, got.FirstTokenAt, "first_token_at should be persisted")

	// Step 4: Complete the message.
	completedAt := now + 3
	msg.CompletedAt = completedAt
	msg.AddFinish(FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err = svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)
	require.Equal(t, completedAt, got.CompletedAt, "completed_at should be persisted")

	// Assert monotonicity: submitted_at <= sent_to_llm_at <= first_token_at <= completed_at.
	require.True(t, got.SubmittedAt <= got.SentToLLMAt,
		"submitted_at (%d) must be <= sent_to_llm_at (%d)", got.SubmittedAt, got.SentToLLMAt)
	require.True(t, got.SentToLLMAt <= got.FirstTokenAt,
		"sent_to_llm_at (%d) must be <= first_token_at (%d)", got.SentToLLMAt, got.FirstTokenAt)
	require.True(t, got.FirstTokenAt <= got.CompletedAt,
		"first_token_at (%d) must be <= completed_at (%d)", got.FirstTokenAt, got.CompletedAt)
}

// TestTimestamp_AllFieldsNonZeroAfterCompletion asserts that after a
// message has gone through the full lifecycle all four timestamp fields
// are non-zero in the persisted DB row.
func TestTimestamp_AllFieldsNonZeroAfterCompletion(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(0))

	now := time.Now().Unix()

	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:        Assistant,
		SubmittedAt: now,
	})
	require.NoError(t, err)

	msg.SentToLLMAt = now + 1
	msg.FirstTokenAt = now + 2
	msg.CompletedAt = now + 3
	msg.AppendContent("response")
	msg.AddFinish(FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)

	require.NotZero(t, got.SubmittedAt, "submitted_at must be non-zero after completion")
	require.NotZero(t, got.SentToLLMAt, "sent_to_llm_at must be non-zero after completion")
	require.NotZero(t, got.FirstTokenAt, "first_token_at must be non-zero after completion")
	require.NotZero(t, got.CompletedAt, "completed_at must be non-zero after completion")
}

// TestTimestamp_DebouncedUpdatesPreserveFinalState verifies that rapid
// updates to timestamp fields under debouncing preserve the final values.
// Multiple updates to SentToLLMAt, FirstTokenAt, etc. within a debounce
// window should coalesce, and the final flush must persist the last value.
func TestTimestamp_DebouncedUpdatesPreserveFinalState(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(50*time.Millisecond))

	now := time.Now().Unix()

	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:        Assistant,
		SubmittedAt: now,
	})
	require.NoError(t, err)

	// Rapidly update timestamp fields with intermediate values.
	// Only the last value for each field should survive.
	for i := range 5 {
		msg.SentToLLMAt = now + int64(i+1)
		msg.FirstTokenAt = now + int64(i+10)
		msg.AppendContent("a")
		require.NoError(t, svc.Update(t.Context(), msg))
	}

	// Set final timestamp and finish the message (terminal → sync flush).
	finalSentAt := now + 100
	finalFirstTokenAt := now + 200
	finalCompletedAt := now + 300
	msg.SentToLLMAt = finalSentAt
	msg.FirstTokenAt = finalFirstTokenAt
	msg.CompletedAt = finalCompletedAt
	msg.AddFinish(FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)

	// The final values must be preserved regardless of intermediate updates.
	require.Equal(t, finalSentAt, got.SentToLLMAt,
		"sent_to_llm_at must reflect final value after debounced updates")
	require.Equal(t, finalFirstTokenAt, got.FirstTokenAt,
		"first_token_at must reflect final value after debounced updates")
	require.Equal(t, finalCompletedAt, got.CompletedAt,
		"completed_at must reflect final value after debounced updates")
	require.Equal(t, now, got.SubmittedAt,
		"submitted_at must remain unchanged from creation")
}

// TestTimestamp_DefaultZeroOnCreate verifies that when a message is created
// without explicit SubmittedAt, the field defaults to zero, and the other
// lifecycle timestamps are also zero.
func TestTimestamp_DefaultZeroOnCreate(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(0))

	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role: Assistant,
		// SubmittedAt intentionally left zero.
	})
	require.NoError(t, err)

	require.Zero(t, msg.SubmittedAt, "submitted_at should default to 0")
	require.Zero(t, msg.SentToLLMAt, "sent_to_llm_at should be 0 on creation")
	require.Zero(t, msg.FirstTokenAt, "first_token_at should be 0 on creation")
	require.Zero(t, msg.CompletedAt, "completed_at should be 0 on creation")

	// Verify after round-trip to DB.
	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)
	require.Zero(t, got.SubmittedAt)
	require.Zero(t, got.SentToLLMAt)
	require.Zero(t, got.FirstTokenAt)
	require.Zero(t, got.CompletedAt)
}

// TestTimestamp_UserMessageHasSubmittedAt verifies that user messages
// (non-assistant) get a Finish part auto-added by Create, and their
// submitted_at is correctly persisted.
func TestTimestamp_UserMessageHasSubmittedAt(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(0))

	now := time.Now().Unix()
	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:        User,
		Parts:       []ContentPart{TextContent{Text: "hello"}},
		SubmittedAt: now,
	})
	require.NoError(t, err)

	require.Equal(t, now, msg.SubmittedAt)
	require.True(t, msg.IsFinished(), "user messages should be auto-finished on creation")

	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)
	require.Equal(t, now, got.SubmittedAt)
	require.True(t, got.IsFinished())
}

// TestTimestamp_EqualTimestampsAllowed verifies that the monotonicity
// constraint allows equal values (submitted_at == sent_to_llm_at is
// valid when send happens in the same second).
func TestTimestamp_EqualTimestampsAllowed(t *testing.T) {
	t.Parallel()

	svc, sessionID := newTestService(t, WithDebounce(0))

	ts := time.Now().Unix()
	msg, err := svc.Create(t.Context(), sessionID, CreateMessageParams{
		Role:        Assistant,
		SubmittedAt: ts,
	})
	require.NoError(t, err)

	// All timestamps set to the same value is technically valid.
	msg.SentToLLMAt = ts
	msg.FirstTokenAt = ts
	msg.CompletedAt = ts
	msg.AddFinish(FinishReasonEndTurn, "", "")
	require.NoError(t, svc.Update(t.Context(), msg))

	got, err := svc.Get(t.Context(), msg.ID)
	require.NoError(t, err)

	require.Equal(t, ts, got.SubmittedAt)
	require.Equal(t, ts, got.SentToLLMAt)
	require.Equal(t, ts, got.FirstTokenAt)
	require.Equal(t, ts, got.CompletedAt)

	// Monotonicity with equal values: <= holds.
	require.True(t, got.SubmittedAt <= got.SentToLLMAt)
	require.True(t, got.SentToLLMAt <= got.FirstTokenAt)
	require.True(t, got.FirstTokenAt <= got.CompletedAt)
}
