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
	"unicode/utf8"
)

// DefaultObservationTokenThreshold is the token count at which the observer
// triggers an observation cycle. 30 000 tokens matches the design spec.
const DefaultObservationTokenThreshold = 30_000

// Observation priority thresholds. These define the numeric boundaries for each
// priority level: critical is the highest, info is the lowest.
const (
	PriorityCritical float64 = 0.9
	PriorityHigh     float64 = 0.7
	PriorityMedium   float64 = 0.4
	PriorityLow      float64 = 0.15
	PriorityInfo     float64 = 0.0
)

// Observation is a single (event, context, implication) tuple extracted by the
// observer agent from the conversation history.
type Observation struct {
	Event       string  `json:"event"`
	Context     string  `json:"context"`
	Implication string  `json:"implication"`
	TokenCount  int64   `json:"token_count"`
	Priority    float64 `json:"priority"`
}

// ObservationResult is the output of a single observation cycle.
type ObservationResult struct {
	Observations []Observation
	Error        error
}

// degenerateRingSize is the number of recent observation summaries tracked for
// degenerate detection.
const degenerateRingSize = 5

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
	// recentObs tracks the last N observation summaries per session for
	// degenerate detection. Keyed by session ID.
	recentObs map[string]*degenerateRing
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
		recentObs: make(map[string]*degenerateRing),
	}
}

// degenerateRing is a fixed-size ring buffer that tracks the most recent
// observation summaries for a session.
type degenerateRing struct {
	buf  [degenerateRingSize]string
	pos  int
	full bool
}

func (r *degenerateRing) push(summary string) {
	r.buf[r.pos] = summary
	r.pos = (r.pos + 1) % degenerateRingSize
	if r.pos == 0 {
		r.full = true
	}
}

func (r *degenerateRing) items() []string {
	if !r.full {
		return r.buf[:r.pos]
	}
	all := make([]string, degenerateRingSize)
	copy(all, r.buf[r.pos:])
	copy(all[degenerateRingSize-r.pos:], r.buf[:r.pos])
	return all
}

func (r *degenerateRing) reset() {
	r.pos = 0
	r.full = false
}

func (r *degenerateRing) len() int {
	if r.full {
		return degenerateRingSize
	}
	return r.pos
}

// observationSummary produces a short fingerprint string for degenerate
// comparison. Two observations with the same summary are considered identical
// for degenerate detection purposes.
func observationSummary(obs Observation) string {
	return obs.Event + "|" + obs.Context
}

// isDegenerate returns true when the new observations represent a degenerate
// case: repeated identical observations, all-empty observations, or an
// observation loop (A→B→A→B pattern).
func isDegenerate(ring *degenerateRing, newObs []Observation) bool {
	if len(newObs) == 0 {
		return true
	}

	summaries := make([]string, len(newObs))
	for i, obs := range newObs {
		s := observationSummary(obs)
		if s == "|" {
			return true
		}
		summaries[i] = s
	}

	if ring != nil && ring.len() > 0 {
		allSame := true
		for _, s := range summaries[1:] {
			if s != summaries[0] {
				allSame = false
				break
			}
		}
		if allSame {
			past := ring.items()
			for _, ps := range past {
				if ps == summaries[0] {
					return true
				}
			}
		}

		if ring.len() >= 4 {
			past := ring.items()
			// Detect A→B→A→B loop: last 4 past entries form a 2-cycle.
			n := len(past)
			if past[n-1] == past[n-3] && past[n-2] == past[n-4] {
				// Only degenerate if new observations repeat the same pattern.
				for _, s := range summaries {
					if s == past[n-1] || s == past[n-2] {
						return true
					}
				}
			}
		}
	}

	return false
}

func (oc *ObservationCoordinator) recordObservations(sessionID string, observations []Observation) {
	ring, ok := oc.recentObs[sessionID]
	if !ok {
		ring = &degenerateRing{}
		oc.recentObs[sessionID] = ring
	}
	for _, obs := range observations {
		ring.push(observationSummary(obs))
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
		tokenCount := EstimateTokens(raw)
		observations = []Observation{{
			Event:       "raw_observation",
			Context:     truncateObservationField(raw, 2000),
			Implication: "",
			TokenCount:  tokenCount,
		}}
	}

	// Check for degenerate observations.
	var degenerate bool
	func() {
		oc.mu.Lock()
		defer oc.mu.Unlock()
		ring := oc.recentObs[sessionID]
		degenerate = isDegenerate(ring, observations)
	}()

	if degenerate {
		slog.Warn("Skipping degenerate observation cycle",
			"session_id", sessionID,
			"observation_count", len(observations),
		)
		return ObservationResult{}
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

	// Record successful observations for future degenerate detection.
	func() {
		oc.mu.Lock()
		defer oc.mu.Unlock()
		oc.recordObservations(sessionID, stored)
	}()

	return ObservationResult{Observations: stored}
}

// insertObservationBuffer persists a single observation into the
// lcm_observation_buffer table.
func (oc *ObservationCoordinator) insertObservationBuffer(ctx context.Context, sessionID string, obs Observation) error {
	id := generateObservationID(sessionID, obs)
	content := oc.strategy.FormatObservation(obs)
	tokenCount := EstimateTokens(string(content))

	_, err := oc.store.rawDB.ExecContext(ctx,
		`INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content, token_count, priority)
		 VALUES (?, ?, 'observation', ?, ?, ?)`,
		id, sessionID, string(content), tokenCount, observationPriorityText(obs.Priority),
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
		 ORDER BY CASE priority
		     WHEN 'critical' THEN 0
		     WHEN 'high' THEN 1
		     WHEN 'medium' THEN 2
		     WHEN 'low' THEN 3
		     WHEN 'info' THEN 4
		     ELSE 2
		 END ASC, created_at ASC`,
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
		Event       string  `json:"event"`
		Context     string  `json:"context"`
		Implication string  `json:"implication"`
		Priority    float64 `json:"priority"`
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
			Priority:    p.Priority,
		}
		observations = append(observations, obs)
	}
	return observations, nil
}

// truncateObservationField truncates a field to maxLen characters.
func truncateObservationField(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen])
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
- "priority": Importance of this observation as a number from 0.0 to 1.0 (>=0.7 is high, >=0.3 is medium, <0.3 is low)

Return a JSON array of observation objects. Each object must have exactly these four fields: "event", "context", "implication", "priority".

Example format:
[
  {
    "event": "User decided to use PostgreSQL instead of SQLite for the database",
    "context": "Discussed in context of internal/db/store.go, affects migration strategy in internal/db/migrations/",
    "implication": "Future migrations need to account for PostgreSQL-specific syntax, connection pooling changes needed",
    "priority": 0.8
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

// formatObservationForPrompt formats a single Observation as structured text
// for injection into the system prompt.
func formatObservationForPrompt(obs Observation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- [%s] %s\n", observationPriorityText(obs.Priority), obs.Event)
	if obs.Context != "" {
		fmt.Fprintf(&sb, "  Context: %s\n", obs.Context)
	}
	if obs.Implication != "" {
		fmt.Fprintf(&sb, "  Implication: %s\n", obs.Implication)
	}
	return sb.String()
}
