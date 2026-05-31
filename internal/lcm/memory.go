package lcm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Memory type constants matching the CHECK constraint in lcm_auto_memory.
const (
	MemoryFact       = "fact"
	MemoryDecision   = "decision"
	MemoryPreference = "preference"
	MemoryLesson     = "lesson"
)

// Memory priority thresholds for TF-IDF based priority classification.
const (
	MemoryPriorityCritical float64 = 0.9
	MemoryPriorityHigh     float64 = 0.7
	MemoryPriorityMedium   float64 = 0.3
	MemoryPriorityLow      float64 = 0.0
)

// Memory extraction limits.
const (
	// MemoryMaxLines is the maximum number of lines per memory content.
	MemoryMaxLines = 200
	// MemoryMaxChars is the maximum size in characters per memory content (4 KB).
	MemoryMaxChars = 4096
	// MemorySessionMaxChars is the maximum total memory size per session (60 KB).
	MemorySessionMaxChars = 60000
	// DefaultMemoryInterval is the default number of turns between extraction triggers.
	DefaultMemoryInterval = 5
	// MemoryMaxTurnsPerTrigger is the maximum number of recent conversation turns
	// examined per extraction trigger.
	MemoryMaxTurnsPerTrigger = 5
)

// ExtractedMemory is a single structured memory extracted from conversation.
type ExtractedMemory struct {
	Type       string  `json:"type"`       // fact, decision, preference, lesson
	Content    string  `json:"content"`    // the memory text
	Confidence float64 `json:"confidence"` // 0.0–1.0
	Priority   float64 `json:"priority"`   // TF-IDF based priority score
}

// AutoMemoryExtractor extracts structured memories from conversation turns
// using an LLM and persists them in the lcm_auto_memory table.
type AutoMemoryExtractor struct {
	store    *Store
	llm      LLMClient
	mu       sync.Mutex
	interval int // turns between extractions
	// pending tracks session IDs with an in-flight extraction to prevent
	// stacking duplicate goroutines.
	pending map[string]struct{}
	// lastProcessedIndex is a cursor tracking how many messages have been
	// examined by extractCycle. Only messages after this index are considered
	// in subsequent extractions, preventing redundant LLM calls on already-
	// processed conversation turns.
	lastProcessedIndex int
}

// NewAutoMemoryExtractor creates a new extractor. If llm is nil, Extract is a
// no-op. If interval <= 0, DefaultMemoryInterval is used.
func NewAutoMemoryExtractor(store *Store, llm LLMClient, interval int) *AutoMemoryExtractor {
	if interval <= 0 {
		interval = DefaultMemoryInterval
	}
	return &AutoMemoryExtractor{
		store:    store,
		llm:      llm,
		interval: interval,
		pending:  make(map[string]struct{}),
	}
}

// SetLLMClient updates the LLM client used for memory extraction.
func (e *AutoMemoryExtractor) SetLLMClient(llm LLMClient) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llm = llm
}

// Interval returns the configured turn interval.
func (e *AutoMemoryExtractor) Interval() int {
	return e.interval
}

// LastProcessedIndex returns the cursor position for incremental extraction.
func (e *AutoMemoryExtractor) LastProcessedIndex() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastProcessedIndex
}

// ExtractResult is the output of an asynchronous extraction cycle.
type ExtractResult struct {
	Memories []ExtractedMemory
	Error    error
}

// ShouldExtract returns true when the given turn count is a multiple of the
// configured interval (and is non-zero).
func (e *AutoMemoryExtractor) ShouldExtract(turnCount int) bool {
	return turnCount > 0 && turnCount%e.interval == 0
}

// Extract launches an asynchronous memory extraction for the given session,
// examining up to MemoryMaxTurnsPerTrigger recent turns. It returns
// immediately; results are available through the returned channel. If an
// extraction is already in-flight for this session, it returns nil.
func (e *AutoMemoryExtractor) Extract(ctx context.Context, sessionID string) <-chan ExtractResult {
	ch := make(chan ExtractResult, 1)

	e.mu.Lock()
	if e.llm == nil {
		e.mu.Unlock()
		ch <- ExtractResult{Error: fmt.Errorf("auto-memory: %w", ErrLLMClientNil)}
		close(ch)
		return ch
	}
	if _, ok := e.pending[sessionID]; ok {
		e.mu.Unlock()
		return nil
	}
	e.pending[sessionID] = struct{}{}
	llm := e.llm
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.pending, sessionID)
			e.mu.Unlock()
		}()

		result := e.extractCycle(ctx, sessionID, llm)
		ch <- result
		close(ch)
	}()

	return ch
}

// ExtractSync is the synchronous version of Extract, useful for testing.
func (e *AutoMemoryExtractor) ExtractSync(ctx context.Context, sessionID string) ExtractResult {
	e.mu.Lock()
	llm := e.llm
	e.mu.Unlock()

	if llm == nil {
		return ExtractResult{Error: fmt.Errorf("auto-memory: %w", ErrLLMClientNil)}
	}
	return e.extractCycle(ctx, sessionID, llm)
}

// extractCycle fetches recent messages, calls the LLM, parses memories,
// enforces limits, and persists them. It uses lastProcessedIndex as a cursor
// to skip messages that were already examined in a previous extraction cycle.
func (e *AutoMemoryExtractor) extractCycle(ctx context.Context, sessionID string, llm LLMClient) ExtractResult {
	// Fetch recent conversation messages.
	messages, err := e.store.GetMessages(ctx, sessionID)
	if err != nil {
		return ExtractResult{Error: fmt.Errorf("fetching messages for memory extraction: %w", err)}
	}
	if len(messages) == 0 {
		return ExtractResult{}
	}

	e.mu.Lock()
	cursor := e.lastProcessedIndex
	e.mu.Unlock()

	if cursor >= len(messages) {
		return ExtractResult{}
	}

	remaining := messages[cursor:]
	start := max(len(remaining)-MemoryMaxTurnsPerTrigger, 0)
	recent := remaining[start:]

	// Build user prompt from recent conversation.
	userPrompt := formatMessagesForMemory(recent)

	// Call the LLM to extract memories.
	raw, err := llm.Complete(ctx, memorySystemPrompt, userPrompt)
	if err != nil {
		return ExtractResult{Error: fmt.Errorf("LLM memory extraction call: %w", err)}
	}

	e.mu.Lock()
	e.lastProcessedIndex = len(messages)
	e.mu.Unlock()

	// Parse the LLM response.
	memories, err := parseMemories(raw)
	if err != nil {
		slog.Warn("Failed to parse memories, skipping",
			"session_id", sessionID,
			"error", err,
		)
		return ExtractResult{}
	}

	// Check session memory budget before storing.
	currentSize, err := e.getSessionMemorySize(ctx, sessionID)
	if err != nil {
		return ExtractResult{Error: fmt.Errorf("checking session memory size: %w", err)}
	}

	// Collect source message IDs from the recent turns.
	sourceIDs := make([]string, 0, len(recent))
	for _, m := range recent {
		sourceIDs = append(sourceIDs, m.ID)
	}

	// Persist each memory within budget.
	var stored []ExtractedMemory
	for _, mem := range memories {
		content := truncateMemoryContent(mem.Content)
		mem.Content = content
		memSize := len(content)

		if currentSize+memSize > MemorySessionMaxChars {
			break
		}

		if err := e.insertMemory(ctx, sessionID, mem, sourceIDs); err != nil {
			slog.Warn("Failed to store memory",
				"session_id", sessionID,
				"error", err,
			)
			continue
		}
		currentSize += memSize
		stored = append(stored, mem)
	}

	return ExtractResult{Memories: stored}
}

// LLMClientFactory creates a new LLM client for forked extraction.
type LLMClientFactory func() LLMClient

// ExtractForked launches a memory extraction in a separate goroutine using an
// LLM client produced by the factory. This isolates the extraction's LLM call
// from the main conversation's client. It returns immediately; results are
// available through the returned channel. If extraction is already in-flight
// for this session, it returns nil.
func (e *AutoMemoryExtractor) ExtractForked(ctx context.Context, sessionID string, factory LLMClientFactory) <-chan ExtractResult {
	ch := make(chan ExtractResult, 1)

	e.mu.Lock()
	if _, ok := e.pending[sessionID]; ok {
		e.mu.Unlock()
		return nil
	}
	e.pending[sessionID] = struct{}{}
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.pending, sessionID)
			e.mu.Unlock()
		}()

		forkedLLM := factory()
		if forkedLLM == nil {
			ch <- ExtractResult{Error: fmt.Errorf("auto-memory: %w", ErrLLMClientNil)}
			close(ch)
			return
		}

		result := e.extractCycle(ctx, sessionID, forkedLLM)

		if len(result.Memories) > 0 {
			corpus := e.buildCorpus(ctx, sessionID, result.Memories)
			result.Memories = RankMemoriesByTFIDF(result.Memories, corpus)
		}

		ch <- result
		close(ch)
	}()

	return ch
}

// buildCorpus assembles the document corpus for TF-IDF from existing session
// memories and the newly extracted ones.
func (e *AutoMemoryExtractor) buildCorpus(ctx context.Context, sessionID string, newMemories []ExtractedMemory) []string {
	existing, err := e.ListMemories(ctx, sessionID)
	if err != nil {
		existing = nil
	}

	corpus := make([]string, 0, len(existing)+len(newMemories))
	for _, m := range existing {
		corpus = append(corpus, m.Content)
	}
	for _, m := range newMemories {
		corpus = append(corpus, m.Content)
	}
	return corpus
}

// tokenize splits text into lowercase terms for TF-IDF computation.
func tokenize(text string) []string {
	var tokens []string
	var sb strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(unicode.ToLower(r))
		} else {
			if sb.Len() > 0 {
				tokens = append(tokens, sb.String())
				sb.Reset()
			}
		}
	}
	if sb.Len() > 0 {
		tokens = append(tokens, sb.String())
	}
	return tokens
}

// termFrequency returns a map of term → frequency for the given text.
// TF = count of term in document / total terms in document.
func termFrequency(text string) map[string]float64 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return map[string]float64{}
	}

	counts := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		counts[tok]++
	}

	total := float64(len(tokens))
	tf := make(map[string]float64, len(counts))
	for term, count := range counts {
		tf[term] = float64(count) / total
	}
	return tf
}

// documentFrequency counts how many documents in the corpus contain each term.
func documentFrequency(corpus []string) map[string]int {
	df := make(map[string]int)
	for _, doc := range corpus {
		seen := make(map[string]struct{})
		for _, tok := range tokenize(doc) {
			if _, ok := seen[tok]; !ok {
				seen[tok] = struct{}{}
				df[tok]++
			}
		}
	}
	return df
}

// tfidfScore computes the aggregate TF-IDF score for all terms in text against
// the given corpus. IDF = log(total documents / documents containing term).
func tfidfScore(text string, corpus []string) float64 {
	if len(corpus) == 0 {
		return 0
	}

	tf := termFrequency(text)
	df := documentFrequency(corpus)

	totalDocs := float64(len(corpus))
	var score float64
	for term, freq := range tf {
		docFreq := float64(df[term])
		if docFreq == 0 {
			continue
		}
		idf := math.Log(totalDocs / docFreq)
		score += freq * idf
	}
	return score
}

// RankMemoriesByTFIDF computes TF-IDF priority scores for each memory against
// the corpus and returns them sorted descending by priority.
func RankMemoriesByTFIDF(memories []ExtractedMemory, corpus []string) []ExtractedMemory {
	if len(memories) == 0 {
		return nil
	}

	for i := range memories {
		memories[i].Priority = tfidfScore(memories[i].Content, corpus)
	}

	sort.SliceStable(memories, func(i, j int) bool {
		return memories[i].Priority > memories[j].Priority
	})

	return memories
}

// getSessionMemorySize returns the total byte size of all memory content for a
// session.
func (e *AutoMemoryExtractor) getSessionMemorySize(ctx context.Context, sessionID string) (int, error) {
	var totalSize int
	rows, err := e.store.rawDB.QueryContext(ctx,
		`SELECT COALESCE(LENGTH(content), 0) FROM lcm_auto_memory WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("querying session memory size: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var size int
		if err := rows.Scan(&size); err != nil {
			return 0, fmt.Errorf("scanning memory size: %w", err)
		}
		totalSize += size
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating memory sizes: %w", err)
	}
	return totalSize, nil
}

// priorityToText converts a numeric TF-IDF priority score to the text bucket
// stored in the database ('critical', 'high', 'medium', 'low').
func priorityToText(p float64) string {
	switch {
	case p >= MemoryPriorityCritical:
		return "critical"
	case p >= MemoryPriorityHigh:
		return "high"
	case p >= MemoryPriorityMedium:
		return "medium"
	default:
		return "low"
	}
}

// textToPriority converts a text priority bucket back to a representative
// numeric value. The original TF-IDF score is not preserved; callers that
// need the exact score should recompute it.
func textToPriority(s string) float64 {
	switch s {
	case "critical":
		return MemoryPriorityCritical
	case "high":
		return MemoryPriorityHigh
	case "medium":
		return MemoryPriorityMedium
	default:
		return MemoryPriorityLow
	}
}

// insertMemory persists a single extracted memory into lcm_auto_memory.
func (e *AutoMemoryExtractor) insertMemory(ctx context.Context, sessionID string, mem ExtractedMemory, sourceIDs []string) error {
	id := generateMemoryID(sessionID, mem)
	sourceJSON, err := json.Marshal(sourceIDs)
	if err != nil {
		return fmt.Errorf("marshaling source message IDs: %w", err)
	}

	_, err = e.store.rawDB.ExecContext(ctx,
		`INSERT INTO lcm_auto_memory (id, session_id, memory_type, content, source_message_ids, confidence, priority)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, mem.Type, mem.Content, string(sourceJSON), mem.Confidence, priorityToText(mem.Priority),
	)
	if err != nil {
		return fmt.Errorf("inserting auto memory: %w", err)
	}
	return nil
}

// ListMemories returns all stored memories for a session, ordered by
// confidence descending (highest confidence first).
func (e *AutoMemoryExtractor) ListMemories(ctx context.Context, sessionID string) ([]ExtractedMemory, error) {
	rows, err := e.store.rawDB.QueryContext(ctx,
		`SELECT memory_type, content, confidence, priority FROM lcm_auto_memory
		 WHERE session_id = ?
		 ORDER BY confidence DESC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()

	var memories []ExtractedMemory
	for rows.Next() {
		var memType, content, priorityStr string
		var confidence float64
		if err := rows.Scan(&memType, &content, &confidence, &priorityStr); err != nil {
			return nil, fmt.Errorf("scanning memory: %w", err)
		}
		memories = append(memories, ExtractedMemory{
			Type:       memType,
			Content:    content,
			Confidence: confidence,
			Priority:   textToPriority(priorityStr),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating memories: %w", err)
	}
	return memories, nil
}

// generateMemoryID creates a deterministic-enough ID for a memory entry.
func generateMemoryID(sessionID string, mem ExtractedMemory) string {
	input := fmt.Sprintf("%s:%s:%s:%d", sessionID, mem.Type, mem.Content, time.Now().UnixNano())
	h := sha256.Sum256([]byte(input))
	return "mem_" + hex.EncodeToString(h[:])[:16]
}

// truncateMemoryContent enforces the per-memory line and character limits.
func truncateMemoryContent(content string) string {
	// Enforce line limit.
	lines := strings.Split(content, "\n")
	if len(lines) > MemoryMaxLines {
		lines = lines[:MemoryMaxLines]
		content = strings.Join(lines, "\n")
	}
	// Enforce character limit.
	if utf8.RuneCountInString(content) > MemoryMaxChars {
		content = string([]rune(content)[:MemoryMaxChars])
	}
	return content
}

// parseMemories parses the LLM response into a slice of ExtractedMemory.
// Expected format is a JSON array of objects with "type", "content", and
// "confidence" fields.
func parseMemories(raw string) ([]ExtractedMemory, error) {
	raw = strings.TrimSpace(raw)

	// Extract JSON from markdown code blocks if present.
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
		Type       string  `json:"type"`
		Content    string  `json:"content"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing memories JSON: %w", err)
	}

	// Validate memory types.
	validTypes := map[string]bool{
		MemoryFact:       true,
		MemoryDecision:   true,
		MemoryPreference: true,
		MemoryLesson:     true,
	}

	var memories []ExtractedMemory
	for _, p := range parsed {
		if !validTypes[p.Type] {
			continue // skip invalid types.
		}
		if p.Content == "" {
			continue // skip empty content.
		}
		confidence := p.Confidence
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		memories = append(memories, ExtractedMemory{
			Type:       p.Type,
			Content:    p.Content,
			Confidence: confidence,
		})
	}
	return memories, nil
}

// formatMessagesForMemory formats recent conversation messages into the user
// prompt for the memory extraction LLM call.
func formatMessagesForMemory(messages []MessageForSummary) string {
	var sb strings.Builder
	sb.WriteString("<conversation>\n")
	for _, m := range messages {
		fmt.Fprintf(&sb, "--- Message (seq: %d, role: %s) ---\n%s\n\n", m.Seq, m.Role, m.Content)
	}
	sb.WriteString("</conversation>")
	return sb.String()
}

// MemoryFileConfig controls the CRUSH.memory.md output behavior.
type MemoryFileConfig struct {
	// FilePath is the path to write. Defaults to "CRUSH.memory.md".
	FilePath string
	// MaxLines is the maximum number of lines per file. Default: 200.
	MaxLines int
	// MaxChars is the maximum file size in characters. Default: 4096.
	MaxChars int
	// SessionBudget is the maximum total memory content per session in characters.
	// Default: 61440 (60 KB).
	SessionBudget int
}

// formatMemoryFrontmatter produces a YAML frontmatter block for a single memory.
func formatMemoryFrontmatter(memType string, priority float64) string {
	return fmt.Sprintf("---\ntype: %s\npriority: %.2f\n---", memType, priority)
}

// WriteMemoryFile writes extracted memories to a markdown file in the working
// directory. It respects line and byte limits, truncating if necessary. If the
// file already contains a manual-edit marker ("<!-- manual -->"), it is not
// overwritten. Returns the number of bytes written and any error.
func WriteMemoryFile(ctx context.Context, memories []ExtractedMemory, workingDir string, cfg MemoryFileConfig) (int, error) {
	if cfg.FilePath == "" {
		cfg.FilePath = "CRUSH.memory.md"
	}
	if cfg.MaxLines == 0 {
		cfg.MaxLines = MemoryMaxLines
	}
	if cfg.MaxChars == 0 {
		cfg.MaxChars = MemoryMaxChars
	}

	fullPath := filepath.Join(workingDir, cfg.FilePath)

	if existing, err := os.ReadFile(fullPath); err == nil {
		if strings.Contains(string(existing), "<!-- manual -->") {
			return 0, nil
		}
	}

	var sb strings.Builder
	sb.WriteString("# Session Memory\n\n")
	sb.WriteString("<!-- auto-generated by Crush -->\n\n")

	for _, mem := range memories {
		priority := mem.Priority
		if priority == 0 {
			priority = mem.Confidence
		}
		fmt.Fprintf(&sb, "%s\n\n- **[%s]** %s\n", formatMemoryFrontmatter(mem.Type, priority), mem.Type, mem.Content)
	}

	content := sb.String()

	lines := strings.Split(content, "\n")
	if len(lines) > cfg.MaxLines {
		lines = lines[:cfg.MaxLines]
		lines = append(lines, "... (truncated)")
		content = strings.Join(lines, "\n")
	}

	if utf8.RuneCountInString(content) > cfg.MaxChars {
		content = string([]rune(content)[:cfg.MaxChars])
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return 0, fmt.Errorf("writing memory file: %w", err)
	}

	return len(content), nil
}

// LoadMemoryFile reads CRUSH.memory.md from the working directory and parses
// it back into structured memories. Returns os.ErrNotExist if the file does
// not exist.
func LoadMemoryFile(workingDir string) ([]ExtractedMemory, error) {
	fullPath := filepath.Join(workingDir, "CRUSH.memory.md")
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("reading memory file: %w", err)
	}
	return parseMemoryMarkdown(string(data)), nil
}

// parseMemoryMarkdown parses the markdown content of a memory file into
// structured memories. It skips headers, markers, empty lines, and the
// auto-generated marker. Each memory entry may be preceded by YAML frontmatter
// containing type and priority. Lines matching "- **[type]** content" are
// parsed.
func parseMemoryMarkdown(content string) []ExtractedMemory {
	if strings.Contains(content, "<!-- manual -->") {
		return nil
	}
	var memories []ExtractedMemory
	var currentPriority float64
	inFrontmatter := false

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<!--") {
			continue
		}

		if line == "---" {
			if inFrontmatter {
				inFrontmatter = false
			} else {
				inFrontmatter = true
				currentPriority = 0
			}
			continue
		}

		if inFrontmatter {
			if after, ok := strings.CutPrefix(line, "priority: "); ok {
				currentPriority = parsePriority(after)
			}
			continue
		}

		if !strings.HasPrefix(line, "- **[") {
			continue
		}
		closeIdx := strings.Index(line, "]**")
		if closeIdx == -1 {
			continue
		}
		memType := line[5:closeIdx]
		memContent := strings.TrimSpace(line[closeIdx+3:])
		if memType != MemoryFact && memType != MemoryDecision &&
			memType != MemoryPreference && memType != MemoryLesson {
			continue
		}

		priority := currentPriority
		currentPriority = 0

		memories = append(memories, ExtractedMemory{
			Type:       memType,
			Content:    memContent,
			Confidence: 0.5,
			Priority:   priority,
		})
	}
	return memories
}

// parsePriority parses a priority value from a YAML frontmatter line.
func parsePriority(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"")
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return val
}

// MemoryTriggerConfig controls when auto-memory extraction fires.
type MemoryTriggerConfig struct {
	// MessageInterval is the number of messages between extractions. Default: 5.
	MessageInterval int
}

// ShouldTrigger reports whether memory extraction should fire given the current
// message count since last extraction.
func ShouldTrigger(messagesSinceLastExtraction int, cfg MemoryTriggerConfig) bool {
	interval := cfg.MessageInterval
	if interval <= 0 {
		interval = DefaultMemoryInterval
	}
	return messagesSinceLastExtraction >= interval
}

// memorySystemPrompt is the system prompt for the memory extraction LLM call.
const memorySystemPrompt = `You are a memory extraction agent analyzing a coding conversation. Your task is to extract structured memories as JSON.

Analyze the conversation and extract key information as memories. Each memory has:
- "type": One of "fact", "decision", "preference", "lesson"
- "content": A concise, self-contained description of the memory (1-3 sentences)
- "confidence": Your confidence in this memory being useful (0.0 to 1.0)

Memory types:
- "fact": Objective information about the codebase, architecture, or dependencies (e.g., "The project uses SQLite via sqlc for persistence")
- "decision": A deliberate choice made during the conversation (e.g., "Decided to use sha256 for ID generation instead of UUID")
- "preference": A user-stated or inferred preference (e.g., "User prefers table-driven tests over subtest assertions")
- "lesson": A lesson learned from errors, debugging, or exploration (e.g., "Tree-sitter requires CGO_ENABLED=1 and a working C compiler")

Return a JSON array of memory objects. Do NOT include raw conversation text — only structured, condensed facts.

Example:
[
  {"type": "fact", "content": "The LCM package uses a Store struct wrapping db.Querier for all DB operations", "confidence": 0.95},
  {"type": "decision", "content": "Chose to use sync.Mutex instead of sync.RWMutex for observation coordinator", "confidence": 0.9}
]

Focus on information that would be valuable in future sessions. Be specific and factual. Do not hallucinate.`
