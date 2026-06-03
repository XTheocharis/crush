package lcm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestAutoMemoryShouldExtract(t *testing.T) {
	t.Parallel()

	extractor := NewAutoMemoryExtractor(nil, nil, 5)

	require.False(t, extractor.ShouldExtract(0), "turn 0 should not trigger")
	require.False(t, extractor.ShouldExtract(3), "turn 3 with interval 5 should not trigger")
	require.True(t, extractor.ShouldExtract(5), "turn 5 with interval 5 should trigger")
	require.False(t, extractor.ShouldExtract(6), "turn 6 with interval 5 should not trigger")
	require.True(t, extractor.ShouldExtract(10), "turn 10 with interval 5 should trigger")
}

func TestAutoMemoryDefaultInterval(t *testing.T) {
	t.Parallel()

	e1 := NewAutoMemoryExtractor(nil, nil, 0)
	require.Equal(t, DefaultMemoryInterval, e1.Interval())

	e2 := NewAutoMemoryExtractor(nil, nil, -1)
	require.Equal(t, DefaultMemoryInterval, e2.Interval())

	e3 := NewAutoMemoryExtractor(nil, nil, 3)
	require.Equal(t, 3, e3.Interval())
}

func TestAutoMemoryExtractSyncSuccess(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_mem_1"
	createTestSession(t, queries, sessionID)

	for i := range 5 {
		createTestMessage(t, queries, sessionID, "msg_mem_1_"+itoa(i), "user", "Hello from message "+itoa(i))
	}

	// Prepare LLM mock response.
	memories := []map[string]any{
		{"type": "fact", "content": "The project uses SQLite via sqlc", "confidence": 0.95},
		{"type": "decision", "content": "Chose sha256 for ID generation", "confidence": 0.9},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Len(t, result.Memories, 2)
	require.Equal(t, "fact", result.Memories[0].Type)
	require.Equal(t, "The project uses SQLite via sqlc", result.Memories[0].Content)
	require.InDelta(t, 0.95, result.Memories[0].Confidence, 0.001)
	require.Equal(t, "decision", result.Memories[1].Type)

	// Verify stored in DB.
	stored, err := extractor.ListMemories(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	require.Equal(t, "fact", stored[0].Type)
}

func TestAutoMemoryExtractSyncNoLLM(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	extractor := NewAutoMemoryExtractor(store, nil, 5)
	result := extractor.ExtractSync(context.Background(), "sess_x")
	require.Error(t, result.Error)
	require.True(t, errors.Is(result.Error, ErrLLMClientNil))
}

func TestAutoMemoryExtractSyncEmptySession(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_empty"
	createTestSession(t, queries, sessionID)

	mock := &mockLLMClient{response: "[]"}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Empty(t, result.Memories)
	require.Equal(t, 0, mock.callCount)
}

func TestAutoMemoryExtractSyncLLMError(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_err"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_err_0000000000", "user", "test")

	mock := &mockLLMClient{err: errTestLLM}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.Error(t, result.Error)
	require.Contains(t, result.Error.Error(), "LLM memory extraction call")
}

func TestAutoMemoryParseMemoriesValid(t *testing.T) {
	t.Parallel()

	input := `[{"type":"fact","content":"Uses Go 1.22","confidence":0.9}]`
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, MemoryFact, memories[0].Type)
	require.Equal(t, "Uses Go 1.22", memories[0].Content)
	require.InDelta(t, 0.9, memories[0].Confidence, 0.001)
}

func TestAutoMemoryParseMemoriesMarkdown(t *testing.T) {
	t.Parallel()

	input := "```json\n[{\"type\":\"lesson\",\"content\":\"Always check errors\",\"confidence\":0.8}]\n```"
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, MemoryLesson, memories[0].Type)
}

func TestAutoMemoryParseMemoriesInvalidType(t *testing.T) {
	t.Parallel()

	input := `[{"type":"invalid","content":"should be skipped","confidence":0.5}]`
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Empty(t, memories)
}

func TestAutoMemoryParseMemoriesEmptyContent(t *testing.T) {
	t.Parallel()

	input := `[{"type":"fact","content":"","confidence":0.5}]`
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Empty(t, memories)
}

func TestAutoMemoryParseMemoriesInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := parseMemories("not json at all")
	require.Error(t, err)
}

func TestAutoMemoryParseMemoriesAllTypes(t *testing.T) {
	t.Parallel()

	input := `[
		{"type":"fact","content":"f1","confidence":0.9},
		{"type":"decision","content":"d1","confidence":0.8},
		{"type":"preference","content":"p1","confidence":0.7},
		{"type":"lesson","content":"l1","confidence":0.6}
	]`
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Len(t, memories, 4)
	require.Equal(t, MemoryFact, memories[0].Type)
	require.Equal(t, MemoryDecision, memories[1].Type)
	require.Equal(t, MemoryPreference, memories[2].Type)
	require.Equal(t, MemoryLesson, memories[3].Type)
}

func TestAutoMemoryParseMemoriesConfidenceClamping(t *testing.T) {
	t.Parallel()

	input := `[
		{"type":"fact","content":"high","confidence":2.0},
		{"type":"fact","content":"low","confidence":-1.0}
	]`
	memories, err := parseMemories(input)
	require.NoError(t, err)
	require.Len(t, memories, 2)
	require.InDelta(t, 1.0, memories[0].Confidence, 0.001)
	require.InDelta(t, 0.0, memories[1].Confidence, 0.001)
}

func TestAutoMemoryTruncateContent(t *testing.T) {
	t.Parallel()

	// Within limits.
	content := "short content"
	require.Equal(t, content, truncateMemoryContent(content))

	// Over byte limit.
	longContent := strings.Repeat("x", MemoryMaxChars+100)
	result := truncateMemoryContent(longContent)
	require.Equal(t, MemoryMaxChars, utf8.RuneCountInString(result))

	// Over line limit.
	manyLines := strings.Repeat("line\n", MemoryMaxLines+10)
	result = truncateMemoryContent(manyLines)
	lines := strings.Split(result, "\n")
	require.LessOrEqual(t, len(lines), MemoryMaxLines+1) // trailing newline adds one
}

func TestAutoMemoryTruncateContent_Multibyte(t *testing.T) {
	t.Parallel()

	// Japanese characters are 3 bytes each in UTF-8.
	// Build a string longer than MemoryMaxChars in character count.
	input := strings.Repeat("日本語テスト", MemoryMaxChars/5+100)
	result := truncateMemoryContent(input)

	// Verify truncation happened and produces valid UTF-8.
	require.LessOrEqual(t, utf8.RuneCountInString(result), MemoryMaxChars)
	require.True(t, utf8.ValidString(result))
	// Verify no partial rune — each rune should be a complete character.
	for _, r := range result {
		require.NotEqual(t, utf8.RuneError, r)
	}
}

func TestAutoMemorySessionBudget(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_budget"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_budg_00000000", "user", "test message for budget")

	// Pre-fill the table to near the 60 KB limit.
	largeContent := strings.Repeat("a", MemorySessionMaxChars-100)
	_, err := rawDB.ExecContext(context.Background(),
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids)
		 VALUES (?, ?, ?, ?, ?)`,
		"mem_prefill_0000000", sessionID, MemoryFact, largeContent, "[]",
	)
	require.NoError(t, err)

	// LLM returns a memory that would push past the limit.
	memories := []map[string]any{
		{"type": "fact", "content": strings.Repeat("b", 200), "confidence": 0.9},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	// Memory should not be stored because budget is exhausted.
	require.Empty(t, result.Memories)
}

func TestSessionBudgetConfigOverride(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_cfg_override"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_override_000", "user", "test override budget")

	budget := 500
	largeContent := strings.Repeat("a", budget-50)
	_, err := rawDB.ExecContext(context.Background(),
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids)
		 VALUES (?, ?, ?, ?, ?)`,
		"mem_override_00000", sessionID, MemoryFact, largeContent, "[]",
	)
	require.NoError(t, err)

	memories := []map[string]any{
		{"type": "fact", "content": strings.Repeat("b", 100), "confidence": 0.9},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	extractor := NewAutoMemoryExtractor(store, mock, 5)
	extractor.SetSessionBudget(budget)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Empty(t, result.Memories)
}

func TestSessionBudgetFallbackToConstant(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_fallback"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_fallback_000", "user", "test fallback")

	largeContent := strings.Repeat("a", MemorySessionMaxChars-100)
	_, err := rawDB.ExecContext(context.Background(),
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids)
		 VALUES (?, ?, ?, ?, ?)`,
		"mem_fallback_00000", sessionID, MemoryFact, largeContent, "[]",
	)
	require.NoError(t, err)

	memories := []map[string]any{
		{"type": "fact", "content": strings.Repeat("b", 200), "confidence": 0.9},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Empty(t, result.Memories)
}

func TestAutoMemoryExtractAsync(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_async"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_async_0000000", "user", "async test")

	memories := []map[string]any{
		{"type": "preference", "content": "User prefers table-driven tests", "confidence": 0.85},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	ch := extractor.Extract(context.Background(), sessionID)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Memories, 1)
	require.Equal(t, MemoryPreference, result.Memories[0].Type)
}

func TestAutoMemoryExtractAsyncPending(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_pend"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_pend_00000000", "user", "pending test")

	// LLM that never returns — extraction stays in-flight.
	mock := &mockLLMClient{response: `[]`}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	// First call starts extraction.
	ch1 := extractor.Extract(context.Background(), sessionID)
	require.NotNil(t, ch1)

	// Manually mark as pending to simulate race.
	extractor.mu.Lock()
	extractor.pending[sessionID] = struct{}{}
	extractor.mu.Unlock()

	// Second call should return nil (already pending).
	ch2 := extractor.Extract(context.Background(), sessionID)
	require.Nil(t, ch2)

	// Clean up pending marker.
	extractor.mu.Lock()
	delete(extractor.pending, sessionID)
	extractor.mu.Unlock()

	// Drain first channel.
	<-ch1
}

func TestAutoMemoryExtractAsyncNoLLM(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	extractor := NewAutoMemoryExtractor(store, nil, 5)
	ch := extractor.Extract(context.Background(), "sess_nollm")
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
	require.True(t, errors.Is(result.Error, ErrLLMClientNil))
}

func TestAutoMemorySetLLMClient(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	extractor := NewAutoMemoryExtractor(store, nil, 5)

	// Initially no LLM.
	result := extractor.ExtractSync(context.Background(), "s1")
	require.Error(t, result.Error)

	// Set LLM client.
	mock := &mockLLMClient{response: `[]`}
	extractor.SetLLMClient(mock)

	// Now should not error on LLM check (but will fail on missing session).
	result = extractor.ExtractSync(context.Background(), "s_nonexist")
	// Should get past the LLM nil check (may still fail on DB query).
	// The point is it no longer says "no LLM client".
	if result.Error != nil {
		require.False(t, errors.Is(result.Error, ErrLLMClientNil))
	}
}

func TestAutoMemoryMaxTurnsPerTrigger(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_turns"
	createTestSession(t, queries, sessionID)

	for i := range 10 {
		createTestMessage(t, queries, sessionID, "msg_turn_"+itoa(i), "user", "message "+itoa(i))
	}

	mock := &mockLLMClient{response: `[]`}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Empty(t, result.Memories)
	require.Equal(t, 1, mock.callCount)
}

func TestAutoMemoryListMemories(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_list"
	createTestSession(t, queries, sessionID)

	// Insert memories directly.
	_, err := rawDB.ExecContext(context.Background(),
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids) VALUES (?, ?, ?, ?, ?)`,
		"mem_list_00000001", sessionID, MemoryFact, "fact 1", "[]",
	)
	require.NoError(t, err)
	_, err = rawDB.ExecContext(context.Background(),
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids) VALUES (?, ?, ?, ?, ?)`,
		"mem_list_00000002", sessionID, MemoryDecision, "decision 1", "[]",
	)
	require.NoError(t, err)

	extractor := NewAutoMemoryExtractor(store, nil, 5)
	memories, err := extractor.ListMemories(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, memories, 2)
	require.Equal(t, MemoryFact, memories[0].Type)
	require.Equal(t, "fact 1", memories[0].Content)
	require.Equal(t, MemoryDecision, memories[1].Type)
}

func TestAutoMemoryGenerateMemoryID(t *testing.T) {
	t.Parallel()

	mem := ExtractedMemory{Type: MemoryFact, Content: "test content", Confidence: 0.9}
	id := generateMemoryID("sess_1", mem)

	require.True(t, strings.HasPrefix(id, "mem_"))
	require.Equal(t, 20, len(id), "mem_ prefix + 16 hex chars = 20 chars")
}

func TestAutoMemoryCursorIncremental(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		initialMessages int
		secondBatch     int
		expectLLMCalls  int
	}{
		{
			name:            "no_new_messages_skips_llm",
			initialMessages: 5,
			secondBatch:     0,
			expectLLMCalls:  0,
		},
		{
			name:            "new_messages_triggers_llm",
			initialMessages: 5,
			secondBatch:     3,
			expectLLMCalls:  1,
		},
		{
			name:            "many_new_messages_still_one_call",
			initialMessages: 5,
			secondBatch:     8,
			expectLLMCalls:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			queries, rawDB := setupTestDB(t)
			store := newStore(queries, rawDB)

			sessionID := "sess_cur_" + strings.ReplaceAll(tc.name, "/", "_")
			createTestSession(t, queries, sessionID)

			for i := range tc.initialMessages {
				createTestMessage(t, queries, sessionID, "msg_init_"+itoa(i), "user", "initial "+itoa(i))
			}

			memories := []map[string]any{
				{"type": "fact", "content": "test fact", "confidence": 0.9},
			}
			respBytes, err := json.Marshal(memories)
			require.NoError(t, err)

			mock := &mockLLMClient{response: string(respBytes)}
			extractor := NewAutoMemoryExtractor(store, mock, 5)

			// First extraction: should process and advance cursor.
			result := extractor.ExtractSync(context.Background(), sessionID)
			require.NoError(t, result.Error)
			require.Equal(t, 1, mock.callCount)
			require.Equal(t, tc.initialMessages, extractor.LastProcessedIndex())

			// Add second batch of messages.
			for i := range tc.secondBatch {
				createTestMessage(t, queries, sessionID, "msg_b2_"+itoa(i), "user", "batch2 "+itoa(i))
			}

			// Second extraction: cursor-based.
			mock.callCount = 0
			result = extractor.ExtractSync(context.Background(), sessionID)
			require.NoError(t, result.Error)
			require.Equal(t, tc.expectLLMCalls, mock.callCount)
			require.Equal(t, tc.initialMessages+tc.secondBatch, extractor.LastProcessedIndex())

			// Third extraction with no new messages: LLM should not be called.
			mock.callCount = 0
			result = extractor.ExtractSync(context.Background(), sessionID)
			require.NoError(t, result.Error)
			require.Empty(t, result.Memories)
			require.Equal(t, 0, mock.callCount)
		})
	}
}

func TestAutoMemoryCursorStartsAtZero(t *testing.T) {
	t.Parallel()

	extractor := NewAutoMemoryExtractor(nil, nil, 5)
	require.Equal(t, 0, extractor.LastProcessedIndex())
}

func TestAutoMemoryExtractSyncBadParse(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_badparse"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_bp_0000000000", "user", "test")

	mock := &mockLLMClient{response: "this is not JSON"}
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	result := extractor.ExtractSync(context.Background(), sessionID)
	require.NoError(t, result.Error)
	require.Empty(t, result.Memories)
}

// --- TF-IDF Tests ---

func TestTFIDF_SingleDocument(t *testing.T) {
	t.Parallel()

	doc := "sqlite database query optimization"
	corpus := []string{doc}

	tf := termFrequency(doc)
	require.InDelta(t, 0.25, tf["sqlite"], 0.01)
	require.InDelta(t, 0.25, tf["database"], 0.01)

	// Single document: IDF = log(1/1) = 0, so TF-IDF is 0.
	score := tfidfScore(doc, corpus)
	require.InDelta(t, 0.0, score, 0.01, "single-doc corpus has IDF=0 for all terms")

	// Multi-doc corpus: non-zero IDF for terms appearing in subset.
	corpus2 := []string{doc, "unrelated document about networking"}
	score2 := tfidfScore(doc, corpus2)
	require.Greater(t, score2, 0.0, "multi-doc corpus should yield non-zero TF-IDF")
}

func TestTFIDF_CommonTermsHaveLowerScore(t *testing.T) {
	t.Parallel()

	corpus := []string{
		"the project uses sqlite for the database",
		"the database is queried via the orm",
		"sqlite handles the persistence layer",
	}

	scoreThe := tfidfScore("the", corpus)
	require.InDelta(t, 0.0, scoreThe, 0.01, "ubiquitous term should have ~0 IDF")

	scoreSqlite := tfidfScore("sqlite", corpus)
	require.Greater(t, scoreSqlite, 0.0, "selective term should have positive TF-IDF")
}

func TestTFIDF_RareTermHigherThanCommon(t *testing.T) {
	t.Parallel()

	corpus := []string{
		"go tests use testify require",
		"go modules use go.sum",
		"go build produces a binary",
	}

	scoreTestify := tfidfScore("testify", corpus)
	scoreGo := tfidfScore("go", corpus)

	require.Greater(t, scoreTestify, scoreGo,
		"rare term 'testify' should score higher than ubiquitous 'go'")
}

func TestRankMemoriesByTFIDF(t *testing.T) {
	t.Parallel()

	memories := []ExtractedMemory{
		{Type: MemoryFact, Content: "The project uses SQLite via sqlc for persistence", Confidence: 0.9},
		{Type: MemoryDecision, Content: "Chose sha256 for ID generation", Confidence: 0.8},
		{Type: MemoryLesson, Content: "SQLite requires foreign keys pragma", Confidence: 0.7},
	}
	corpus := []string{
		"The project uses SQLite via sqlc for persistence",
		"Chose sha256 for ID generation",
		"SQLite requires foreign keys pragma",
		"SQLite is embedded in the application",
	}

	ranked := RankMemoriesByTFIDF(memories, corpus)
	require.Len(t, ranked, 3)

	for _, m := range ranked {
		require.GreaterOrEqual(t, m.Priority, 0.0)
	}

	require.Equal(t, "Chose sha256 for ID generation", ranked[0].Content,
		"memory with rarest terms should rank first")
}

func TestRankMemoriesByTFIDF_EmptyCorpus(t *testing.T) {
	t.Parallel()

	memories := []ExtractedMemory{
		{Type: MemoryFact, Content: "Some fact", Confidence: 0.9},
	}
	ranked := RankMemoriesByTFIDF(memories, nil)
	require.Len(t, ranked, 1)
	require.Equal(t, 0.0, ranked[0].Priority, "empty corpus should yield zero priority")
}

func TestRankMemoriesByTFIDF_EmptyMemories(t *testing.T) {
	t.Parallel()

	ranked := RankMemoriesByTFIDF(nil, []string{"some corpus"})
	require.Empty(t, ranked)
}

// --- Forked Extraction Tests ---

func TestExtractForked_Success(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_fork_ok"
	createTestSession(t, queries, sessionID)
	for i := range 5 {
		createTestMessage(t, queries, sessionID, "msg_fork_ok_"+itoa(i), "user", "fork test "+itoa(i))
	}

	memories := []map[string]any{
		{"type": "fact", "content": "Forked extraction fact", "confidence": 0.9},
	}
	respBytes, err := json.Marshal(memories)
	require.NoError(t, err)

	mock := &mockLLMClient{response: string(respBytes)}
	factory := func() LLMClient { return mock }
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	ch := extractor.ExtractForked(context.Background(), sessionID, factory)
	require.NotNil(t, ch)

	result := <-ch
	require.NoError(t, result.Error)
	require.Len(t, result.Memories, 1)
	require.Equal(t, "Forked extraction fact", result.Memories[0].Content)
}

func TestExtractForked_FactoryError(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_fork_err"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_fork_err_0", "user", "test")

	factory := func() LLMClient { return nil }
	extractor := NewAutoMemoryExtractor(store, nil, 5)

	ch := extractor.ExtractForked(context.Background(), sessionID, factory)
	require.NotNil(t, ch)

	result := <-ch
	require.Error(t, result.Error)
}

func TestExtractForked_Deduplicates(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_fork_dedup"
	createTestSession(t, queries, sessionID)
	createTestMessage(t, queries, sessionID, "msg_fd_0000000000", "user", "test")

	mock := &mockLLMClient{response: `[]`}
	factory := func() LLMClient { return mock }
	extractor := NewAutoMemoryExtractor(store, mock, 5)

	ch1 := extractor.ExtractForked(context.Background(), sessionID, factory)
	require.NotNil(t, ch1)

	extractor.mu.Lock()
	extractor.pending[sessionID] = struct{}{}
	extractor.mu.Unlock()

	ch2 := extractor.ExtractForked(context.Background(), sessionID, factory)
	require.Nil(t, ch2)

	extractor.mu.Lock()
	delete(extractor.pending, sessionID)
	extractor.mu.Unlock()

	<-ch1
}

// --- YAML Frontmatter Tests ---

func TestFormatMemoryFrontmatter(t *testing.T) {
	t.Parallel()

	fm := formatMemoryFrontmatter(MemoryFact, 0.75)
	require.Contains(t, fm, "---")
	require.Contains(t, fm, "type: fact")
	require.Contains(t, fm, "priority: 0.75")
}

func TestWriteMemoryFile_WithFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	memories := []ExtractedMemory{
		{Type: MemoryFact, Content: "SQLite via sqlc", Confidence: 0.95, Priority: 0.8},
		{Type: MemoryDecision, Content: "Chose sha256", Confidence: 0.9, Priority: 0.6},
	}

	n, err := WriteMemoryFile(context.Background(), memories, dir, MemoryFileConfig{})
	require.NoError(t, err)
	require.Greater(t, n, 0)

	data, err := os.ReadFile(filepath.Join(dir, "CRUSH.memory.md"))
	require.NoError(t, err)
	content := string(data)

	require.Contains(t, content, "---")
	require.Contains(t, content, "type: fact")
	require.Contains(t, content, "priority: 0.80")
	require.Contains(t, content, "type: decision")
	require.Contains(t, content, "priority: 0.60")
}

func TestLoadMemoryFile_FrontmatterRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	original := []ExtractedMemory{
		{Type: MemoryFact, Content: "SQLite via sqlc", Confidence: 0.95, Priority: 0.8},
		{Type: MemoryLesson, Content: "CGO required", Confidence: 0.85, Priority: 0.5},
	}

	_, err := WriteMemoryFile(context.Background(), original, dir, MemoryFileConfig{})
	require.NoError(t, err)

	loaded, err := LoadMemoryFile(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	require.Equal(t, MemoryFact, loaded[0].Type)
	require.Equal(t, "SQLite via sqlc", loaded[0].Content)
	require.InDelta(t, 0.8, loaded[0].Priority, 0.01)

	require.Equal(t, MemoryLesson, loaded[1].Type)
	require.Equal(t, "CGO required", loaded[1].Content)
	require.InDelta(t, 0.5, loaded[1].Priority, 0.01)
}

func TestAutoMemoryConfidenceRoundTrip(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_conf_rt"
	createTestSession(t, queries, sessionID)

	mem := ExtractedMemory{
		Type:       MemoryFact,
		Content:    "Confidence round-trip test fact",
		Confidence: 0.85,
	}
	extractor := NewAutoMemoryExtractor(store, nil, 5)
	err := extractor.insertMemory(context.Background(), sessionID, mem, []string{"msg_src_001"})
	require.NoError(t, err)

	memories, err := extractor.ListMemories(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, "Confidence round-trip test fact", memories[0].Content)
	require.InDelta(t, 0.85, memories[0].Confidence, 0.001)
}

func TestAutoMemoryConfidenceOrdering(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_conf_order"
	createTestSession(t, queries, sessionID)

	extractor := NewAutoMemoryExtractor(store, nil, 5)

	memories := []ExtractedMemory{
		{Type: MemoryFact, Content: "low confidence fact", Confidence: 0.3, Priority: 0.2},
		{Type: MemoryDecision, Content: "high confidence decision", Confidence: 0.95, Priority: 0.8},
		{Type: MemoryLesson, Content: "mid confidence lesson", Confidence: 0.6, Priority: 0.5},
	}
	for _, mem := range memories {
		err := extractor.insertMemory(context.Background(), sessionID, mem, []string{"msg_src"})
		require.NoError(t, err)
	}

	result, err := extractor.ListMemories(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, result, 3)

	require.InDelta(t, 0.95, result[0].Confidence, 0.001, "highest confidence should be first")
	require.Equal(t, "high confidence decision", result[0].Content)

	require.InDelta(t, 0.6, result[1].Confidence, 0.001, "mid confidence should be second")
	require.Equal(t, "mid confidence lesson", result[1].Content)

	require.InDelta(t, 0.3, result[2].Confidence, 0.001, "lowest confidence should be last")
	require.Equal(t, "low confidence fact", result[2].Content)
}

func TestAutoMemoryPriorityRoundTrip(t *testing.T) {
	t.Parallel()

	queries, rawDB := setupTestDB(t)
	store := newStore(queries, rawDB)

	sessionID := "sess_pri_rt"
	createTestSession(t, queries, sessionID)

	extractor := NewAutoMemoryExtractor(store, nil, 5)

	cases := []struct {
		name     string
		priority float64
		bucket   string
	}{
		{"critical", 0.95, "critical"},
		{"high", 0.85, "high"},
		{"medium", 0.5, "medium"},
		{"low", 0.1, "low"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sid := sessionID + "_" + tc.name
			createTestSession(t, queries, sid)

			mem := ExtractedMemory{
				Type:       MemoryFact,
				Content:    tc.name + " priority fact",
				Confidence: 0.8,
				Priority:   tc.priority,
			}
			err := extractor.insertMemory(context.Background(), sid, mem, nil)
			require.NoError(t, err)

			result, err := extractor.ListMemories(context.Background(), sid)
			require.NoError(t, err)
			require.Len(t, result, 1)

			require.Equal(t, tc.bucket, priorityToText(tc.priority))
			require.Equal(t, tc.bucket, priorityToText(result[0].Priority))
		})
	}
}

func TestPriorityToTextBuckets(t *testing.T) {
	t.Parallel()

	require.Equal(t, "critical", priorityToText(0.95))
	require.Equal(t, "critical", priorityToText(0.9))
	require.Equal(t, "high", priorityToText(0.85))
	require.Equal(t, "high", priorityToText(0.7))
	require.Equal(t, "medium", priorityToText(0.5))
	require.Equal(t, "medium", priorityToText(0.3))
	require.Equal(t, "low", priorityToText(0.1))
	require.Equal(t, "low", priorityToText(0.0))

	require.InDelta(t, 0.9, textToPriority("critical"), 0.001)
	require.InDelta(t, 0.7, textToPriority("high"), 0.001)
	require.InDelta(t, 0.3, textToPriority("medium"), 0.001)
	require.InDelta(t, 0.0, textToPriority("low"), 0.001)
}

type testLLMError struct{}

func (e *testLLMError) Error() string { return "test LLM error" }

var errTestLLM error = &testLLMError{}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
