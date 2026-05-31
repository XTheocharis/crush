package lcm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// DefaultReflectionTokenThreshold is the token count at which the reflector
// triggers a reflection cycle. 40 000 tokens matches the design spec.
const DefaultReflectionTokenThreshold = 40_000

// BufferThresholdPercent is the interval (as a fraction) at which the
// BufferingCoordinator collects observations for the observer. 0.2 means
// observations are collected at 20%, 40%, 60%, 80% and 100% of the
// reflection threshold.
const BufferThresholdPercent = 0.2

// ReflectorThresholdPercent is the interval (as a fraction) at which the
// BufferingCoordinator triggers the reflector. 0.5 means the reflector
// fires at 50% and 100% of the reflection threshold, giving it two chances
// to produce insights before the session hits the full token budget.
const ReflectorThresholdPercent = 0.5

// Reflection is a single (insight, confidence, action_suggestion) tuple
// produced by the reflector agent from accumulated observations.
type Reflection struct {
	Insight          string  `json:"insight"`
	Confidence       float64 `json:"confidence"`
	ActionSuggestion string  `json:"action_suggestion"`
	TokenCount       int64   `json:"token_count"`
}

// ReflectionResult is the output of a single reflection cycle.
type ReflectionResult struct {
	Reflections []Reflection
	Error       error
}

// ReflectorAgent reflects on accumulated observations from the Observer. It
// triggers asynchronously when a session's token count crosses the configured
// threshold (default 40K) and produces (insight, confidence, action_suggestion)
// tuples via an LLM call.
type ReflectorAgent struct {
	store     *Store
	llm       LLMClient
	mu        sync.Mutex
	threshold int64
	// pending tracks session IDs with an in-flight reflection to avoid
	// stacking duplicate goroutines for the same session.
	pending map[string]struct{}
}

// NewReflectorAgent creates a reflector using the given store and LLM client.
// If llm is nil, Reflect will be a no-op. If threshold <= 0, the default of
// 40 000 is used.
func NewReflectorAgent(store *Store, llm LLMClient, threshold int64) *ReflectorAgent {
	if threshold <= 0 {
		threshold = DefaultReflectionTokenThreshold
	}
	return &ReflectorAgent{
		store:     store,
		llm:       llm,
		threshold: threshold,
		pending:   make(map[string]struct{}),
	}
}

// SetLLMClient updates the LLM client used for reflection analysis.
func (ra *ReflectorAgent) SetLLMClient(llm LLMClient) {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	ra.llm = llm
}

// Threshold returns the configured token threshold for triggering reflections.
func (ra *ReflectorAgent) Threshold() int64 {
	return ra.threshold
}

// ShouldReflect returns true when the session's current token count meets or
// exceeds the configured threshold.
func (ra *ReflectorAgent) ShouldReflect(ctx context.Context, sessionID string) (bool, error) {
	count, err := ra.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("checking token count for reflection: %w", err)
	}
	return count >= ra.threshold, nil
}

// Reflect launches an asynchronous reflection cycle for the given session.
// It returns immediately; the result is available through the returned channel.
// If a reflection is already running for this session, it returns nil.
func (ra *ReflectorAgent) Reflect(ctx context.Context, sessionID string) <-chan ReflectionResult {
	ch := make(chan ReflectionResult, 1)

	ra.mu.Lock()
	if ra.llm == nil {
		ra.mu.Unlock()
		ch <- ReflectionResult{Error: fmt.Errorf("reflection: %w", ErrLLMClientNil)}
		close(ch)
		return ch
	}
	if _, ok := ra.pending[sessionID]; ok {
		ra.mu.Unlock()
		return nil
	}
	ra.pending[sessionID] = struct{}{}
	llm := ra.llm
	ra.mu.Unlock()

	go func() {
		defer func() {
			ra.mu.Lock()
			delete(ra.pending, sessionID)
			ra.mu.Unlock()
		}()

		result := ra.reflectCycle(ctx, sessionID, llm)
		ch <- result
		close(ch)
	}()

	return ch
}

// reflectCycle runs a single reflection cycle: load observations from the
// buffer, call the LLM to analyze them, parse results, persist reflections,
// and mark observations as reflected.
func (ra *ReflectorAgent) reflectCycle(ctx context.Context, sessionID string, llm LLMClient) ReflectionResult {
	// Fetch unreflected observations.
	observations, err := ra.listUnreflectedObservations(ctx, sessionID)
	if err != nil {
		return ReflectionResult{Error: fmt.Errorf("fetching observations for reflection: %w", err)}
	}
	if len(observations) == 0 {
		return ReflectionResult{}
	}

	// Build user prompt from observations.
	userPrompt := formatObservationsForReflection(observations)

	// Determine compression level based on current session token count.
	tokenCount, _ := ra.store.GetContextTokenCount(ctx, sessionID)
	level := determineCompressionLevel(int(tokenCount))
	systemPrompt := reflectionPromptForLevel(level)

	// Call the LLM to produce reflections.
	raw, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return ReflectionResult{Error: fmt.Errorf("LLM reflection call: %w", err)}
	}

	// Parse the LLM response into reflections.
	reflections, err := parseReflections(raw)
	if err != nil {
		slog.Warn("Failed to parse reflections, storing raw content",
			"session_id", sessionID,
			"error", err,
		)
		tokenCount := EstimateTokens(raw)
		reflections = []Reflection{{
			Insight:          truncateObservationField(raw, 2000),
			Confidence:       0.5,
			ActionSuggestion: "",
			TokenCount:       tokenCount,
		}}
	}

	// Persist each reflection.
	var stored []Reflection
	for _, ref := range reflections {
		if err := ra.insertReflectionBuffer(ctx, sessionID, ref); err != nil {
			slog.Warn("Failed to store reflection",
				"session_id", sessionID,
				"error", err,
			)
			continue
		}
		stored = append(stored, ref)
	}

	// Mark observations as reflected (update buffer_type to 'summary').
	if err := ra.markObservationsReflected(ctx, sessionID); err != nil {
		slog.Warn("Failed to mark observations as reflected",
			"session_id", sessionID,
			"error", err,
		)
	}

	return ReflectionResult{Reflections: stored}
}

// determineCompressionLevel returns the CompressionLevel appropriate for the
// given token count. Higher token counts trigger more aggressive compression
// to keep reflection output within reasonable bounds.
func determineCompressionLevel(tokenCount int) CompressionLevel {
	switch {
	case tokenCount < 30_000:
		return LevelNormal
	case tokenCount < 40_000:
		return LevelExtractive
	case tokenCount < 50_000:
		return LevelAggressive
	case tokenCount < 60_000:
		return LevelSkeleton
	default:
		return LevelDeterministic
	}
}

// reflectionPromptForLevel returns the system prompt for the reflector based on
// the compression level. Higher levels produce more concise reflection prompts.
func reflectionPromptForLevel(lvl CompressionLevel) string {
	switch lvl {
	case LevelExtractive:
		return extractiveReflectionPrompt
	case LevelAggressive:
		return aggressiveReflectionPrompt
	case LevelSkeleton:
		return skeletonReflectionPrompt
	case LevelDeterministic:
		return deterministicReflectionPrompt
	default:
		return reflectionSystemPrompt
	}
}

// listUnreflectedObservations returns observations that have not yet been
// reflected on. Unreflected observations have buffer_type = 'observation'.
func (ra *ReflectorAgent) listUnreflectedObservations(ctx context.Context, sessionID string) ([]Observation, error) {
	rows, err := ra.store.rawDB.QueryContext(ctx,
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
		return nil, fmt.Errorf("querying unreflected observations: %w", err)
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

// markObservationsReflected updates all unreflected observations for a session
// by setting buffer_type to 'summary'. This marks them as having been processed
// by the reflector without discarding them.
func (ra *ReflectorAgent) markObservationsReflected(ctx context.Context, sessionID string) error {
	_, err := ra.store.rawDB.ExecContext(ctx,
		`UPDATE lcm_observation_buffer
		 SET buffer_type = 'summary'
		 WHERE session_id = ? AND buffer_type = 'observation'`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("marking observations as reflected: %w", err)
	}
	return nil
}

// insertReflectionBuffer persists a single reflection into the
// lcm_observation_buffer table with buffer_type = 'insight'.
func (ra *ReflectorAgent) insertReflectionBuffer(ctx context.Context, sessionID string, ref Reflection) error {
	id := generateReflectionID(sessionID, ref)
	content, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("marshaling reflection: %w", err)
	}
	tokenCount := EstimateTokens(string(content))

	_, err = ra.store.rawDB.ExecContext(ctx,
		`INSERT INTO lcm_observation_buffer (id, session_id, buffer_type, content, token_count, priority)
		 VALUES (?, ?, 'insight', ?, ?, 'medium')`,
		id, sessionID, string(content), tokenCount,
	)
	if err != nil {
		return fmt.Errorf("inserting reflection buffer: %w", err)
	}
	return nil
}

// ListReflections returns all stored reflections for a session.
func (ra *ReflectorAgent) ListReflections(ctx context.Context, sessionID string) ([]Reflection, error) {
	rows, err := ra.store.rawDB.QueryContext(ctx,
		`SELECT content, token_count FROM lcm_observation_buffer
		 WHERE session_id = ? AND buffer_type = 'insight'
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing reflections: %w", err)
	}
	defer rows.Close()

	var reflections []Reflection
	for rows.Next() {
		var content string
		var tokenCount int64
		if err := rows.Scan(&content, &tokenCount); err != nil {
			return nil, fmt.Errorf("scanning reflection: %w", err)
		}
		var ref Reflection
		if err := json.Unmarshal([]byte(content), &ref); err != nil {
			continue
		}
		ref.TokenCount = tokenCount
		reflections = append(reflections, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating reflections: %w", err)
	}
	return reflections, nil
}

// generateReflectionID creates a deterministic ID for a reflection based on
// session ID and content.
func generateReflectionID(sessionID string, ref Reflection) string {
	input := fmt.Sprintf("%s:%s:%f:%d", sessionID, ref.Insight, ref.Confidence, time.Now().UnixNano())
	h := sha256.Sum256([]byte(input))
	return "ref_" + hex.EncodeToString(h[:])[:16]
}

// parseReflections parses the LLM response into a slice of Reflection structs.
// The expected format is a JSON array of objects with "insight", "confidence",
// and "action_suggestion" fields.
func parseReflections(raw string) ([]Reflection, error) {
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
		Insight          string  `json:"insight"`
		Confidence       float64 `json:"confidence"`
		ActionSuggestion string  `json:"action_suggestion"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing reflections JSON: %w", err)
	}

	var reflections []Reflection
	for _, p := range parsed {
		ref := Reflection{
			Insight:          truncateObservationField(p.Insight, 2000),
			Confidence:       p.Confidence,
			ActionSuggestion: truncateObservationField(p.ActionSuggestion, 2000),
		}
		// Clamp confidence to [0, 1].
		if ref.Confidence < 0 {
			ref.Confidence = 0
		}
		if ref.Confidence > 1 {
			ref.Confidence = 1
		}
		reflections = append(reflections, ref)
	}
	return reflections, nil
}

// observationPriorityText converts a numeric priority score to a text label.
func observationPriorityText(p float64) string {
	switch {
	case p >= PriorityCritical:
		return "critical"
	case p >= PriorityHigh:
		return "high"
	case p >= PriorityMedium:
		return "medium"
	case p >= PriorityLow:
		return "low"
	default:
		return "info"
	}
}

// formatObservationsForReflection formats observations into the user prompt
// for the reflector LLM call.
func formatObservationsForReflection(observations []Observation) string {
	var sb strings.Builder
	sb.WriteString("<observations>\n")
	for i, obs := range observations {
		fmt.Fprintf(&sb, "--- Observation #%d ---\n", i+1)
		fmt.Fprintf(&sb, "Event: %s\n", obs.Event)
		fmt.Fprintf(&sb, "Context: %s\n", obs.Context)
		fmt.Fprintf(&sb, "Implication: %s\n", obs.Implication)
		fmt.Fprintf(&sb, "Priority: %s\n\n", observationPriorityText(obs.Priority))
	}
	sb.WriteString("</observations>")
	return sb.String()
}

// reflectionSystemPrompt is the system prompt for the reflector LLM call.
const reflectionSystemPrompt = `You are a reflector agent analyzing accumulated observations from a coding conversation. Your task is to produce structured reflections as JSON.

Analyze the observations and produce insights. Each reflection should capture:
- "insight": A synthesized understanding or pattern you discovered from the observations (be specific and actionable)
- "confidence": A number from 0.0 to 1.0 indicating how confident you are in this insight
- "action_suggestion": A recommended action or next step based on this insight

Return a JSON array of reflection objects. Each object must have exactly these three fields: "insight", "confidence", "action_suggestion".

Example format:
[
  {
    "insight": "The codebase is migrating from SQLite to PostgreSQL, requiring syntax changes in 5 migration files",
    "confidence": 0.9,
    "action_suggestion": "Review all migration files for SQLite-specific syntax and update connection pooling configuration"
  }
]

Focus on:
- Patterns and trends across multiple observations
- Decisions that impact future work
- Technical debt or risks identified
- Relationships between different parts of the codebase
- Unresolved issues that need attention

Be specific and actionable. Do not hallucinate details not present in the observations.`

const extractiveReflectionPrompt = `You are a reflector agent. Extract key points from the observations as structured JSON reflections. Target 8/10 detail — preserve important sentences verbatim, remove filler.

Each reflection must have: "insight", "confidence" (0.0-1.0), "action_suggestion".
Return a JSON array. Preserve file paths and function names verbatim.`

const aggressiveReflectionPrompt = `You are a reflector agent. Produce very concise reflections from the observations as JSON. Focus on the most critical patterns and risks only. Target 6/10 detail.

Each reflection must have: "insight", "confidence" (0.0-1.0), "action_suggestion".
Return a JSON array. Be brief but precise.`

const skeletonReflectionPrompt = `You are a reflector agent. Reduce observations to headers and key terms as JSON. Target 4/10 detail — output structured outlines with identifiers, file paths, and decisions only. Omit prose.

Each reflection must have: "insight", "confidence" (0.0-1.0), "action_suggestion".
Return a JSON array.`

const deterministicReflectionPrompt = `You are a reflector agent. Produce bullet-point reflections from the observations as JSON. Each insight must be a single bullet point. No prose. Target 2/10 detail.

Each reflection must have: "insight", "confidence" (0.0-1.0), "action_suggestion".
Return a JSON array with short, factual bullet points only.`

// BufferingCoordinator collects observations at configurable interval thresholds
// and passes them to the ReflectorAgent when the buffer is full. It tracks the
// session's progress through the threshold intervals and flushes accumulated
// observations to the reflector at each boundary crossing.
type BufferingCoordinator struct {
	store            *Store
	reflector        *ReflectorAgent
	mu               sync.Mutex
	thresholdPercent float64
	intervals        map[string]int
}

// NewBufferingCoordinator creates a coordinator that buffers observations and
// flushes to the given reflector at threshold intervals. The observer intervals
// use BufferThresholdPercent (20%) by default; the reflector uses
// ReflectorThresholdPercent (50%).
func NewBufferingCoordinator(store *Store, reflector *ReflectorAgent) *BufferingCoordinator {
	return &BufferingCoordinator{
		store:            store,
		reflector:        reflector,
		thresholdPercent: BufferThresholdPercent,
		intervals:        make(map[string]int),
	}
}

// NewBufferingCoordinatorWithPercent creates a coordinator with a custom
// threshold percent for interval calculation.
func NewBufferingCoordinatorWithPercent(store *Store, reflector *ReflectorAgent, percent float64) *BufferingCoordinator {
	return &BufferingCoordinator{
		store:            store,
		reflector:        reflector,
		thresholdPercent: percent,
		intervals:        make(map[string]int),
	}
}

// CurrentInterval returns the current interval index (0-based) for a session
// based on its token count. Returns -1 if the token count is below the first
// threshold.
func (bc *BufferingCoordinator) CurrentInterval(tokenCount int64) int {
	if bc.reflector == nil || bc.reflector.Threshold() <= 0 {
		return -1
	}
	threshold := bc.reflector.Threshold()
	if tokenCount <= 0 {
		return -1
	}
	intervalSize := int64(float64(threshold) * bc.thresholdPercent)
	if intervalSize <= 0 {
		return -1
	}
	maxIdx := int(1.0/bc.thresholdPercent) - 1
	if maxIdx < 1 {
		maxIdx = 1
	}
	idx := min(int(tokenCount/intervalSize), maxIdx)
	return idx
}

// ShouldCollect returns true when the session's token count has crossed into a
// new 20% threshold interval since the last collection.
func (bc *BufferingCoordinator) ShouldCollect(ctx context.Context, sessionID string) (bool, error) {
	tokenCount, err := bc.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("checking token count for buffer collection: %w", err)
	}

	currentInterval := bc.CurrentInterval(tokenCount)

	bc.mu.Lock()
	lastInterval := bc.intervals[sessionID]
	bc.mu.Unlock()

	return currentInterval > lastInterval, nil
}

// Collect checks if the session has crossed a threshold boundary. If it has,
// it advances the interval tracker and triggers an asynchronous reflection
// flush at reflector intervals (50% and 100% of threshold by default).
func (bc *BufferingCoordinator) Collect(ctx context.Context, sessionID string) (<-chan ReflectionResult, error) {
	tokenCount, err := bc.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking token count for buffer collection: %w", err)
	}

	currentInterval := bc.CurrentInterval(tokenCount)

	bc.mu.Lock()
	lastInterval := bc.intervals[sessionID]

	if currentInterval <= lastInterval {
		bc.mu.Unlock()
		return nil, nil
	}

	bc.intervals[sessionID] = currentInterval
	threshold := bc.reflector.Threshold()
	bc.mu.Unlock()

	reflectorStep := int64(float64(threshold) * bc.thresholdPercent)
	if reflectorStep <= 0 {
		reflectorStep = threshold
	}
	if tokenCount%reflectorStep == 0 || tokenCount >= threshold {
		return bc.reflector.Reflect(ctx, sessionID), nil
	}

	return nil, nil
}

// Flush forces an immediate reflection cycle regardless of threshold state.
// Useful for end-of-session cleanup.
func (bc *BufferingCoordinator) Flush(ctx context.Context, sessionID string) <-chan ReflectionResult {
	return bc.reflector.Reflect(ctx, sessionID)
}

// Reset clears the interval tracking for a session. Typically called when a
// session is reset or when the buffer is cleared externally.
func (bc *BufferingCoordinator) Reset(sessionID string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	delete(bc.intervals, sessionID)
}
