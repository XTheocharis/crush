package lcm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultObservationTokenThreshold is the token count at which the observer
// triggers an observation cycle. 30 000 tokens matches the design spec.
const DefaultObservationTokenThreshold = 30_000

// Observation is a single (event, context, implication) tuple extracted by the
// observer agent from the conversation history.
type Observation struct {
	Event       string `json:"event"`
	Context     string `json:"context"`
	Implication string `json:"implication"`
	TokenCount  int64  `json:"token_count"`
}

// ObservationResult is the output of a single observation cycle.
type ObservationResult struct {
	Observations []Observation
	Error        error
}

// ObservationCoordinator manages the observer agent lifecycle. It triggers
// asynchronous observation cycles when a session's token count crosses the
// configured threshold.
type ObservationCoordinator struct {
	store     *Store
	llm       LLMClient
	mu        sync.Mutex
	threshold int64
	strategy  ObservationStrategy
	// pending tracks session IDs with an in-flight observation to avoid
	// stacking duplicate goroutines for the same session.
	pending map[string]struct{}
}

// NewObservationCoordinator creates a coordinator using the given store and LLM
// client. If llm is nil, Observe will be a no-op. If strategy is nil,
// DefaultStrategy is used.
func NewObservationCoordinator(store *Store, llm LLMClient, threshold int64, strategy ObservationStrategy) *ObservationCoordinator {
	if threshold <= 0 {
		threshold = DefaultObservationTokenThreshold
	}
	if strategy == nil {
		strategy = DefaultStrategy{}
	}
	return &ObservationCoordinator{
		store:     store,
		llm:       llm,
		threshold: threshold,
		strategy:  strategy,
		pending:   make(map[string]struct{}),
	}
}

// SetLLMClient updates the LLM client used for observation extraction.
func (oc *ObservationCoordinator) SetLLMClient(llm LLMClient) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.llm = llm
}

// Threshold returns the configured token threshold for triggering observations.
func (oc *ObservationCoordinator) Threshold() int64 {
	return oc.threshold
}

// ShouldObserve returns true when the session's current token count meets or
// exceeds the configured threshold.
func (oc *ObservationCoordinator) ShouldObserve(ctx context.Context, sessionID string) (bool, error) {
	count, err := oc.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("checking token count for observation: %w", err)
	}
	return count >= oc.threshold, nil
}

// Observe launches an asynchronous observation cycle for the given session.
// It returns immediately; the result is available through the returned channel.
// If an observation is already running for this session, it returns nil.
func (oc *ObservationCoordinator) Observe(ctx context.Context, sessionID string) <-chan ObservationResult {
	ch := make(chan ObservationResult, 1)

	oc.mu.Lock()
	if oc.llm == nil {
		oc.mu.Unlock()
		ch <- ObservationResult{Error: fmt.Errorf("observation: %w", ErrLLMClientNil)}
		// closed in Observe
		close(ch)
		return ch
	}
	if _, ok := oc.pending[sessionID]; ok {
		oc.mu.Unlock()
		// Already running — return nil channel signals "skipped".
		return nil
	}
	oc.pending[sessionID] = struct{}{}
	llm := oc.llm
	oc.mu.Unlock()

	go func() {
		defer func() {
			oc.mu.Lock()
			delete(oc.pending, sessionID)
			oc.mu.Unlock()
		}()

		result := oc.observeCycle(ctx, sessionID, llm)
		ch <- result
		// closed in Observe
		close(ch)
	}()

	return ch
}

// observeCycle runs a single observation cycle: fetch messages, call the LLM,
// parse results, and persist observations.
func (oc *ObservationCoordinator) observeCycle(ctx context.Context, sessionID string, llm LLMClient) ObservationResult {
	// Check strategy before proceeding.
	if !oc.strategy.ShouldObserve(ctx, "token_threshold_crossed") {
		return ObservationResult{}
	}

	// Fetch recent conversation messages.
	messages, err := oc.store.GetMessages(ctx, sessionID)
	if err != nil {
		return ObservationResult{Error: fmt.Errorf("fetching messages for observation: %w", err)}
	}
	if len(messages) == 0 {
		return ObservationResult{}
	}

	// Build user prompt from conversation.
	userPrompt := formatMessagesForObservation(messages)

	// Call the LLM to extract observations.
	raw, err := llm.Complete(ctx, observationSystemPrompt, userPrompt)
	if err != nil {
		return ObservationResult{Error: fmt.Errorf("LLM observation call: %w", err)}
	}

	// Parse the LLM response into observations.
	observations, err := parseObservations(raw)
	if err != nil {
		slog.Warn("Failed to parse observations, storing raw content",
			"session_id", sessionID,
			"error", err,
		)
		// Store the raw response as a single observation as a fallback.
		tokenCount := EstimateTokens(raw)
		observations = []Observation{{
			Event:       "raw_observation",
			Context:     truncateObservationField(raw, 2000),
			Implication: "",
			TokenCount:  tokenCount,
		}}
	}

	// Persist each observation to the buffer table.
	var stored []Observation
	for _, obs := range observations {
		if err := oc.insertObservationBuffer(ctx, sessionID, obs); err != nil {
			slog.Warn("Failed to store observation",
				"session_id", sessionID,
				"error", err,
			)
			continue
		}
		stored = append(stored, obs)
	}

	return ObservationResult{Observations: stored}
}

// insertObservationBuffer persists a single observation into the
// lcm_observation_buffer table.
func (oc *ObservationCoordinator) insertObservationBuffer(ctx context.Context, sessionID string, obs Observation) error {
	id := generateObservationID(sessionID, obs)
	content := oc.strategy.FormatObservation(obs)
	tokenCount := EstimateTokens(string(content))

	_, err := oc.store.rawDB.ExecContext(ctx,
		`INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content, token_count)
		 VALUES (?, ?, 'observation', ?, ?)`,
		id, sessionID, string(content), tokenCount,
	)
	if err != nil {
		return fmt.Errorf("inserting observation buffer: %w", err)
	}
	return nil
}

// ListObservations returns all stored observations for a session from the
// lcm_observation_buffer table.
func (oc *ObservationCoordinator) ListObservations(ctx context.Context, sessionID string) ([]Observation, error) {
	rows, err := oc.store.rawDB.QueryContext(ctx,
		`SELECT content, token_count FROM lcm_observation_buffer
		 WHERE session_id = ? AND buffer_type = 'observation'
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing observations: %w", err)
	}
	defer rows.Close()

	var observations []Observation
	for rows.Next() {
		var content string
		var tokenCount int64
		if err := rows.Scan(&content, &tokenCount); err != nil {
			return nil, fmt.Errorf("scanning observation: %w", err)
		}
		var obs Observation
		if err := json.Unmarshal([]byte(content), &obs); err != nil {
			// Skip malformed entries.
			continue
		}
		obs.TokenCount = tokenCount
		observations = append(observations, obs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating observations: %w", err)
	}
	return observations, nil
}

// generateObservationID creates a deterministic ID for an observation based on
// session ID and content.
func generateObservationID(sessionID string, obs Observation) string {
	input := fmt.Sprintf("%s:%s:%s:%d", sessionID, obs.Event, obs.Context, time.Now().UnixNano())
	h := sha256.Sum256([]byte(input))
	return "obs_" + hex.EncodeToString(h[:])[:16]
}

// parseObservations parses the LLM response into a slice of Observation
// structs. The expected format is a JSON array of objects with "event",
// "context", and "implication" fields.
func parseObservations(raw string) ([]Observation, error) {
	raw = strings.TrimSpace(raw)

	// Try to extract JSON from markdown code blocks if present.
	if idx := strings.Index(raw, "```json"); idx != -1 {
		raw = raw[idx+7:]
		if end := strings.Index(raw, "```"); end != -1 {
			raw = raw[:end]
		}
	} else if idx := strings.Index(raw, "```"); idx != -1 {
		raw = raw[idx+3:]
		if end := strings.Index(raw, "```"); end != -1 {
			raw = raw[:end]
		}
	}

	raw = strings.TrimSpace(raw)

	var parsed []struct {
		Event       string `json:"event"`
		Context     string `json:"context"`
		Implication string `json:"implication"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing observations JSON: %w", err)
	}

	var observations []Observation
	for _, p := range parsed {
		obs := Observation{
			Event:       truncateObservationField(p.Event, 500),
			Context:     truncateObservationField(p.Context, 2000),
			Implication: truncateObservationField(p.Implication, 2000),
		}
		observations = append(observations, obs)
	}
	return observations, nil
}

// truncateObservationField truncates a field to maxLen characters.
func truncateObservationField(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// formatMessagesForObservation formats conversation messages into the user
// prompt for the observer LLM call.
func formatMessagesForObservation(messages []MessageForSummary) string {
	var sb strings.Builder
	sb.WriteString("<conversation>\n")
	for _, m := range messages {
		fmt.Fprintf(&sb, "--- Message (seq: %d, role: %s) ---\n%s\n\n", m.Seq, m.Role, m.Content)
	}
	sb.WriteString("</conversation>")
	return sb.String()
}

// observationSystemPrompt is the system prompt for the observer LLM call.
const observationSystemPrompt = `You are an observer agent analyzing a coding conversation. Your task is to extract structured observations as JSON.

Analyze the conversation and extract key observations. Each observation should capture:
- "event": What happened or what was discussed (brief, 1-2 sentences)
- "context": The relevant context or background (file paths, function names, technical details)
- "implication": What this means for future work or potential issues to watch for

Return a JSON array of observation objects. Each object must have exactly these three fields: "event", "context", "implication".

Example format:
[
  {
    "event": "User decided to use PostgreSQL instead of SQLite for the database",
    "context": "Discussed in context of internal/db/store.go, affects migration strategy in internal/db/migrations/",
    "implication": "Future migrations need to account for PostgreSQL-specific syntax, connection pooling changes needed"
  }
]

Focus on:
- Decisions made and their rationale
- Technical details mentioned (file paths, function names, APIs)
- Problems encountered and solutions applied
- Patterns or conventions established
- Dependencies and their impact
- Unresolved issues or TODOs mentioned

Be specific and factual. Do not hallucinate details not present in the conversation.`

func BuildObservationContextPrompt(entries map[string]string) string {
	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Operational Memory\n\n")

	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(&sb, "- **%s**: %s\n", k, entries[k])
	}

	return sb.String()
}
