package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools/types"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/lcm/nudge"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/session"
)

// CompactOption configures a compaction operation.
type CompactOption func(*compactConfig)

// compactConfig holds optional parameters for compaction.
type compactConfig struct {
	Pressure     string
	TargetTokens int64
}

// WithPressure sets the compaction pressure tier.
func WithPressure(p string) CompactOption {
	return func(c *compactConfig) { c.Pressure = p }
}

// WithTargetTokens sets the target token count override.
func WithTargetTokens(t int64) CompactOption {
	return func(c *compactConfig) { c.TargetTokens = t }
}

// OperationalMemoryStore is the subset of session.OperationalMemory needed by
// the LCM manager for session-lifecycle integration. Defined here to avoid a
// circular import of the session package.
type OperationalMemoryStore interface {
	// Get retrieves the value for a key within a session.
	Get(ctx context.Context, sessionID, key string) (string, bool, error)
	// Set upserts a key-value pair for the given session.
	Set(ctx context.Context, sessionID, key, value string) error
	// Delete removes a key from a session's operational memory.
	Delete(ctx context.Context, sessionID, key string) error
	// List returns all key-value pairs for a session.
	List(ctx context.Context, sessionID string) (map[string]string, error)
	// ListByPriority returns all entries for a session sorted by priority.
	ListByPriority(ctx context.Context, sessionID string) ([]session.OMEntry, error)
}

// Manager is the public interface for LCM operations.
type Manager interface {
	// ScheduleCompaction schedules an async soft-threshold compaction for a session.
	// Returns a channel that receives the result when done.
	ScheduleCompaction(ctx context.Context, sessionID string) <-chan CompactionResult

	// CompactUntilUnderLimit runs compaction until the session is under the hard limit.
	// Used for blocking hard-limit enforcement.
	CompactUntilUnderLimit(ctx context.Context, sessionID string) error

	// GetContextTokenCount returns the current token count for a session's context.
	GetContextTokenCount(ctx context.Context, sessionID string) (int64, error)

	// GetBudget returns the token budget for a session.
	GetBudget(ctx context.Context, sessionID string) (Budget, error)

	// IsOverSoftThreshold checks if a session is over the soft threshold.
	IsOverSoftThreshold(ctx context.Context, sessionID string) (ThresholdCheck, error)

	// IsOverHardLimit checks if a session is over the hard limit.
	IsOverHardLimit(ctx context.Context, sessionID string) (ThresholdCheck, error)

	// InitSession creates/ensures LCM session config exists.
	InitSession(ctx context.Context, sessionID string) error

	// SetDefaultContextWindow sets the context window for new sessions.
	SetDefaultContextWindow(contextWindow int64)

	// SetCutoffThreshold sets the cutoff fraction for budget computation
	// (default: 0.6). Values outside (0, 1] are ignored.
	SetCutoffThreshold(threshold float64)

	// SetLargeOutputThreshold sets the token count above which tool output
	// is stored in LCM (default: 50000). Values <= 0 are ignored.
	SetLargeOutputThreshold(threshold int64)

	// SetSessionBudget sets the maximum total auto-memory content per session
	// in characters. Values <= 0 keep the hardcoded default (60000).
	SetSessionBudget(budget int)

	// SetModelOutputLimit sets the model's max output token limit for budget computation.
	SetModelOutputLimit(limit int64)

	// SetRepoMapTokens sets repo map token overhead for session budget computation.
	SetRepoMapTokens(ctx context.Context, sessionID string, tokens int64) error

	// UpdateContextWindow updates context window for all tracked sessions.
	UpdateContextWindow(ctx context.Context, contextWindow int64) error

	// GetContextFiles returns LCM context files for injection into the system prompt.
	GetContextFiles() []ContextFile

	// Compact runs compaction for a session via layered compaction with
	// LLM summarization fallback. Options override pressure tier and
	// target token count.
	Compact(ctx context.Context, sessionID string, opts ...CompactOption) error

	// Subscribe returns a channel of compaction events.
	Subscribe(ctx context.Context) <-chan pubsub.Event[CompactionEvent]

	// GetFormattedContext returns the formatted context for a session.
	GetFormattedContext(ctx context.Context, sessionID string) ([]FormattedContextEntry, error)

	// CompactIfOverHardLimit checks the hard limit and runs blocking compaction
	// if needed. Errors are logged internally; the method always returns to the caller.
	CompactIfOverHardLimit(ctx context.Context, sessionID string)

	// ExtraAgentTools returns LCM-related agent tools for injection into the coder agent.
	ExtraAgentTools() []fantasy.AgentTool

	// SetOverheadTokens sets system prompt and tool token overhead for budget computation.
	SetOverheadTokens(systemPromptTokens, toolTokens int64)

	// SetLLMClient updates the LLM client used for LCM summary generation.
	SetLLMClient(llm LLMClient)

	// GetSummaryMentionedPaths extracts file paths mentioned in LCM
	// summaries for a session. Used as weak ranking hints for the repo map.
	GetSummaryMentionedPaths(ctx context.Context, sessionID string) ([]string, error)

	// SetActualPromptTokens records the provider-reported prompt token count
	// after an LLM call. Resets the pending-item delta. Used by the agent
	// to feed ground-truth token counts into LCM threshold checks.
	SetActualPromptTokens(sessionID string, tokens int64)

	// AddPendingItemTokens accumulates estimated tokens for messages created
	// since the last provider report. Added by messageDecorator on each Create.
	AddPendingItemTokens(sessionID string, tokens int64)

	// RunLayeredCompaction executes the multi-layer compaction framework for a
	// session. Layers run in priority order (micro-compaction, post-compact
	// restore, cache optimisation). Returns the aggregate layer result.
	RunLayeredCompaction(ctx context.Context, sessionID string, overrideTier ...*PressureTier) (*CompactionLayerResult, error)

	// InjectCuesIntoPrompt injects ghost cues into a prompt string, respecting
	// the given token budget. Cues are sorted by descending priority.
	InjectCuesIntoPrompt(prompt string, cues []GhostCue, tokenBudget int64) string

	// BuildCompactPrompt assembles the 9-section compact prompt for a session
	// using the cache optimizer. Sections are ordered by cache stability.
	BuildCompactPrompt(ctx context.Context, sessionID string) (string, error)

	// PostCompactionHook runs observation extraction, reflection, and post-
	// compact context restore after a compaction cycle completes.
	PostCompactionHook(ctx context.Context, sessionID string)

	// PostTurnHook runs auto-memory extraction after an agent turn completes.
	// Stores extracted memories in operational memory for cross-session recall.
	PostTurnHook(ctx context.Context, sessionID string)

	// SetProviderType updates the LLM provider type for cache-optimization
	// decisions (e.g. "anthropic" enables Anthropic prefix caching heuristics).
	SetProviderType(providerType string)

	// SetOperationalMemory connects the session's operational memory store so
	// that auto-memory and post-compaction hooks can persist key-value state.
	SetOperationalMemory(om OperationalMemoryStore)

	// SetOperationalMemoryEnabled controls whether operational memory features
	// are active. When false (default), OnSessionStart and OnSessionEnd are
	// no-ops even if an OperationalMemoryStore has been set.
	SetOperationalMemoryEnabled(enabled bool)

	// OnSessionStart initializes operational memory for a new session and
	// triggers an initial observation. It does not block session creation on
	// errors — failures are logged but not propagated.
	OnSessionStart(ctx context.Context, sessionID string) error

	// OnSessionEnd flushes operational memory for the session and triggers a
	// final observation. It does not block session deletion on errors.
	OnSessionEnd(ctx context.Context, sessionID string) error

	// SetHookRunners connects PreCompact and PostCompact hook runners so
	// compaction lifecycle events fire user-configured hooks.
	SetHookRunners(preCompact, postCompact *hooks.Runner)

	// SetAgentConfigRestorer connects the agent's config restorer so that
	// checkpointed session agent configuration (skills, tools, agents) is
	// restored after compaction.
	SetAgentConfigRestorer(restorer AgentConfigRestorer)

	// SetNudgeInjector connects the nudge injection system so that
	// context-limit warnings are appended to prompts when pressure is high.
	SetNudgeInjector(injector *nudge.NudgeInjector)

	// SetPostCompactConfig sets the post-compaction re-injection limits.
	// Zero values keep the hardcoded defaults (MaxFiles=5, TokenBudget=50000).
	SetPostCompactConfig(maxFiles int, tokenBudget int64)

	// SetDeduplicationEnabled controls whether the dedup compaction layer is
	// active (default: true).
	SetDeduplicationEnabled(enabled bool)

	// SetPurgeErrorsEnabled controls whether error entries are purged during
	// compaction (default: true).
	SetPurgeErrorsEnabled(enabled bool)

	// GetTurnCount returns the current turn count for a session. Returns 0
	// for unknown sessions.
	GetTurnCount(sessionID string) int64

	// GetIterationCount returns the current iteration count for a session.
	// Returns 0 for unknown sessions.
	GetIterationCount(sessionID string) int64

	// IncrementIteration atomically increments the iteration counter for a
	// session.
	IncrementIteration(sessionID string)

	// SetObservationConfig configures the observation coordinator with
	// strategy, threshold, and model overrides derived from the runtime
	// config. When strategyName is empty the default strategy is used.
	// When threshold is zero the built-in default is used.
	// observerBufferRatio controls the BufferingCoordinator interval fraction
	// (default 0.2). reflectorObservationTokens sets the reflector threshold
	// (default 40000). reflectorBufferActivation sets the reflector's buffer
	// activation percent (default 0.5).
	SetObservationConfig(strategyName string, threshold int64, observerModel string, reflectorModel string, observerBufferRatio float64, reflectorObservationTokens int64, reflectorBufferActivation float64)

	// CompressWith delegates to the configured Compressor strategy for
	// semantic compression of text. Returns an error when no Compressor
	// is configured.
	CompressWith(ctx context.Context, input string) (*CompressedOutput, error)

	// RetrieveSummary delegates to the retrieval store to fetch a formatted
	// summary by ID (Bindle).
	RetrieveSummary(ctx context.Context, summaryID string) (string, error)

	// ListObservations returns all stored observations for a session.
	ListObservations(ctx context.Context, sessionID string) ([]Observation, error)

	// GetObservationPrompt returns formatted observation text for injection
	// into the system prompt. Observations are sorted by priority descending
	// and truncated to fit within tokenBudget using the CueInjector greedy
	// pattern. Returns an empty string when no observations exist.
	GetObservationPrompt(ctx context.Context, sessionID string, tokenBudget int64) (string, error)

	// ListReflections returns all stored reflections for a session.
	ListReflections(ctx context.Context, sessionID string) ([]Reflection, error)

	// ListMemories returns all stored memories for a session.
	ListMemories(ctx context.Context, sessionID string) ([]ExtractedMemory, error)

	// ListOMEntries returns all operational memory entries for a session.
	ListOMEntries(ctx context.Context, sessionID string) ([]session.OMEntry, error)

	// RestoreReplacement restores a single content replacement by ID,
	// transitioning it from active to restored state. Returns
	// ErrNoActiveReplacement if the replacement is not in active state.
	RestoreReplacement(ctx context.Context, id int64) error

	// RestoreAllByRound restores all content replacements for a session in a
	// specific compaction round. Already-restored entries are skipped.
	RestoreAllByRound(ctx context.Context, sessionID string, round int) error

	// PinEntry pins a context entry by recording a pinned replacement at the
	// given position. Pinned entries are skipped by the MicroCompactor.
	PinEntry(ctx context.Context, sessionID string, position int64) error

	// CleanOrphanedReplacements removes replacement records whose referenced
	// context entry no longer exists. Should be called after compaction.
	CleanOrphanedReplacements(ctx context.Context, sessionID string) (int, error)
}

type compactionManager struct {
	store      *Store
	querier    db.Querier
	queries    *db.Queries
	sqlDB      *sql.DB
	broker     *pubsub.Broker[CompactionEvent]
	summarizer *Summarizer

	contentReplacements ContentReplacementStore

	inFlight      sync.Map // sessionID -> struct{} (deduplication)
	budgetCache   sync.Map // sessionID -> Budget
	repoMapTokens sync.Map // sessionID -> int64
	sessionMu     sync.Map // sessionID -> *sync.Mutex (per-session compaction lock)
	providerState sync.Map // sessionID -> *providerTokenState

	defaultContextWindow      int64
	defaultCutoff             float64
	defaultModelOutputLimit   int64
	defaultSystemPromptTokens int64
	defaultToolTokens         int64
	largeOutputThreshold      int64

	// Compressor strategy for LLM-based semantic compression during
	// compaction. When set, the manager delegates to the Compressor for
	// applying a compression pass on summary content.
	compressor CompressionStrategy

	// retrieval wraps the Store methods for summary retrieval (Bindle,
	// Ancestry, Dolt, Archive, Sprig). The manager delegates context-fetch
	// calls to retrieval so that higher layers stay decoupled from the
	// Store.
	retrieval *Store

	// Reversible compaction: saves original messages alongside summaries
	// so that compressed content can be decompressed later.
	reversibleCompactor *ReversibleCompactor
	blockTracker        *BlockIDTracker

	// Layer system components (Tasks 28-42 integration).
	cueInjector           *CueInjector
	observer              *ObservationCoordinator
	reflector             *ReflectorAgent
	bufferingCoordinator  *BufferingCoordinator
	preservedContextStore *PreservedContextStore
	providerType          string
	opMemory              OperationalMemoryStore
	operationalMemEnabled bool
	autoMemoryExtractor   *AutoMemoryExtractor
	turnCounter           sync.Map // sessionID -> *atomic.Int64
	iterationCounter      sync.Map // sessionID -> *atomic.Int64
	preCompactRunner      *hooks.Runner
	postCompactRunner     *hooks.Runner

	// DeduplicationEnabled controls whether the dedup compaction layer is
	// created. Defaults to true.
	deduplicationEnabled bool

	// purgeErrorsEnabled controls whether the purge-errors compression
	// strategy is included in the graduated pressure system. Defaults to true.
	purgeErrorsEnabled bool

	// agentConfigRestorer restores checkpointed session agent configuration
	// (skills, tools, agents) after compaction.
	agentConfigRestorer AgentConfigRestorer

	postCompactMaxFiles    int
	postCompactTokenBudget int64

	// toolFactory creates LCM agent tools. Injected via constructor to
	// avoid importing the concrete tools package (layering violation).
	toolFactory types.LCMToolFactory

	// nudgeInjector appends context-limit nudges to prompts when pressure
	// is high. Nil means no nudges are injected.
	nudgeInjector *nudge.NudgeInjector
}

type providerTokenState struct {
	mu                sync.Mutex
	promptTokens      int64
	pendingItemTokens int64
}

// sessionMutex returns the per-session mutex, creating it lazily.
func (m *compactionManager) sessionMutex(sessionID string) *sync.Mutex {
	actual, _ := m.sessionMu.LoadOrStore(sessionID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// NewManager creates a new LCM manager.
func NewManager(queries *db.Queries, rawDB *sql.DB, toolFactory ...types.LCMToolFactory) Manager {
	var factory types.LCMToolFactory
	if len(toolFactory) > 0 {
		factory = toolFactory[0]
	}
	store := newStore(queries, rawDB)
	preservedStore := NewPreservedContextStore()
	cueInjector := NewCueInjector()

	return &compactionManager{
		store:                 store,
		querier:               queries,
		queries:               queries,
		sqlDB:                 rawDB,
		broker:                pubsub.NewBroker[CompactionEvent](),
		summarizer:            NewSummarizer(nil),
		defaultContextWindow:  128000,
		defaultCutoff:         0.6,
		retrieval:             store,
		cueInjector:           cueInjector,
		preservedContextStore: preservedStore,
		observer:              NewObservationCoordinator(store, nil, 0, nil),
		reflector:             NewReflectorAgent(store, nil, 0),
		bufferingCoordinator:  NewBufferingCoordinator(store, nil),
		autoMemoryExtractor:   NewAutoMemoryExtractor(store, nil, 0),
		reversibleCompactor:   NewReversibleCompactor(store),
		blockTracker:          NewBlockIDTracker("blk"),
		toolFactory:           factory,
		contentReplacements:   newContentReplacementStore(queries, rawDB),
		deduplicationEnabled:  true,
		purgeErrorsEnabled:    true,
	}
}

// NewManagerWithLLM creates a new LCM manager with an LLM client for summarization.
func NewManagerWithLLM(queries *db.Queries, rawDB *sql.DB, llm LLMClient, toolFactory ...types.LCMToolFactory) Manager {
	var factory types.LCMToolFactory
	if len(toolFactory) > 0 {
		factory = toolFactory[0]
	}
	store := newStore(queries, rawDB)
	preservedStore := NewPreservedContextStore()
	cueInjector := NewCueInjector()
	observer := NewObservationCoordinator(store, llm, 0, nil)
	reflector := NewReflectorAgent(store, llm, 0)

	return &compactionManager{
		store:                 store,
		querier:               queries,
		queries:               queries,
		sqlDB:                 rawDB,
		broker:                pubsub.NewBroker[CompactionEvent](),
		summarizer:            NewSummarizer(llm),
		defaultContextWindow:  128000,
		defaultCutoff:         0.6,
		compressor:            NewMessageCompression(llm),
		retrieval:             store,
		cueInjector:           cueInjector,
		preservedContextStore: preservedStore,
		observer:              observer,
		reflector:             reflector,
		bufferingCoordinator:  NewBufferingCoordinator(store, reflector),
		autoMemoryExtractor:   NewAutoMemoryExtractor(store, llm, 0),
		reversibleCompactor:   NewReversibleCompactor(store),
		blockTracker:          NewBlockIDTracker("blk"),
		toolFactory:           factory,
		contentReplacements:   newContentReplacementStore(queries, rawDB),
		deduplicationEnabled:  true,
		purgeErrorsEnabled:    true,
	}
}

// Subscribe returns a channel of compaction events.
func (m *compactionManager) Subscribe(ctx context.Context) <-chan pubsub.Event[CompactionEvent] {
	return m.broker.Subscribe(ctx)
}

// InitSession creates or ensures an LCM session config exists for the session.
func (m *compactionManager) InitSession(ctx context.Context, sessionID string) error {
	repoMapTokens := m.repoMapTokensForSession(sessionID)
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:      m.defaultContextWindow,
		CutoffThreshold:    m.defaultCutoff,
		RepoMapTokens:      repoMapTokens,
		ModelOutputLimit:   m.defaultModelOutputLimit,
		SystemPromptTokens: m.defaultSystemPromptTokens,
		ToolTokens:         m.defaultToolTokens,
	})

	tx, err := m.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning init transaction: %v: %w", ErrStorageTransaction, err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := m.queries.WithTx(tx)

	err = qtx.UpsertLcmSessionConfig(ctx, db.UpsertLcmSessionConfigParams{
		SessionID:           sessionID,
		ModelName:           "",
		ModelCtxMaxTokens:   m.defaultContextWindow,
		CtxCutoffThreshold:  m.defaultCutoff,
		SoftThresholdTokens: budget.SoftThreshold,
		HardThresholdTokens: budget.HardLimit,
	})
	if err != nil {
		return fmt.Errorf("upserting session config: %w", err)
	}

	if err := m.bootstrapLegacyContext(ctx, qtx, sessionID); err != nil {
		return fmt.Errorf("bootstrapping legacy context: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing init transaction: %v: %w", ErrStorageTransaction, err)
	}

	m.budgetCache.Store(sessionID, budget)
	return nil
}

func (m *compactionManager) bootstrapLegacyContext(ctx context.Context, q db.Querier, sessionID string) error {
	sessionRow, err := q.GetSessionByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("getting session: %w", err)
	}

	items, err := q.ListLcmContextItems(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("listing context items: %w", err)
	}

	// Legacy summary boundaries are only safe to clear once LCM owns a stable
	// ordered context prefix for the session.
	if len(items) > 0 {
		if sessionRow.SummaryMessageID.Valid && hasStableContextItems(items) {
			if err := q.ClearSessionSummaryMessageID(ctx, sessionID); err != nil {
				return fmt.Errorf("clearing migrated summary boundary: %w", err)
			}
		}
		return nil
	}

	dbMsgs, err := q.ListMessagesBySessionSeq(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("listing messages by seq: %w", err)
	}

	visibleMsgs := legacyVisibleMessages(dbMsgs, sessionRow.SummaryMessageID.String)
	for i, msg := range visibleMsgs {
		if err := q.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
			SessionID:  sessionID,
			Position:   int64(i),
			ItemType:   "message",
			MessageID:  sql.NullString{String: msg.ID, Valid: true},
			TokenCount: legacyMessageTokenCount(msg),
		}); err != nil {
			return fmt.Errorf("inserting bootstrap context item for message %s: %w", msg.ID, err)
		}
	}

	if sessionRow.SummaryMessageID.Valid {
		if err := q.ClearSessionSummaryMessageID(ctx, sessionID); err != nil {
			return fmt.Errorf("clearing legacy summary boundary: %w", err)
		}
	}

	return nil
}

func legacyVisibleMessages(msgs []db.Message, summaryMessageID string) []db.Message {
	if summaryMessageID == "" {
		return msgs
	}
	for i, msg := range msgs {
		if msg.ID == summaryMessageID {
			return msgs[i:]
		}
	}
	return msgs
}

func legacyMessageTokenCount(msg db.Message) int64 {
	if msg.TokenCount > 0 {
		return msg.TokenCount
	}
	return EstimateTokens(extractTextFromParts(msg.Parts))
}

func hasStableContextItems(items []db.LcmContextItem) bool {
	for _, item := range items {
		if item.Position >= 0 {
			return true
		}
	}
	return false
}

// SetDefaultContextWindow sets the context window for new sessions.
func (m *compactionManager) SetDefaultContextWindow(contextWindow int64) {
	m.defaultContextWindow = contextWindow
}

func (m *compactionManager) SetCutoffThreshold(threshold float64) {
	if threshold <= 0 || threshold > 1 {
		return
	}
	m.defaultCutoff = threshold
}

func (m *compactionManager) SetLargeOutputThreshold(threshold int64) {
	if threshold <= 0 {
		return
	}
	m.largeOutputThreshold = threshold
}

func (m *compactionManager) SetSessionBudget(budget int) {
	if budget <= 0 || m.autoMemoryExtractor == nil {
		return
	}
	m.autoMemoryExtractor.SetSessionBudget(budget)
}

// SetModelOutputLimit sets the model's max output token limit for budget computation.
func (m *compactionManager) SetModelOutputLimit(limit int64) {
	m.defaultModelOutputLimit = limit
}

// SetOverheadTokens sets system prompt and tool token overhead for budget computation.
func (m *compactionManager) SetOverheadTokens(systemPromptTokens, toolTokens int64) {
	m.defaultSystemPromptTokens = systemPromptTokens
	m.defaultToolTokens = toolTokens
}

// SetLLMClient updates the LLM client used for LCM summary generation.
func (m *compactionManager) SetLLMClient(llm LLMClient) {
	m.summarizer.SetLLM(llm)
	if m.compressor == nil && llm != nil {
		m.compressor = NewMessageCompression(llm)
	}
	if m.observer != nil {
		m.observer.SetLLMClient(llm)
	}
	if m.reflector != nil {
		m.reflector.SetLLMClient(llm)
	}
	if m.bufferingCoordinator == nil && m.reflector != nil {
		m.bufferingCoordinator = NewBufferingCoordinator(m.store, m.reflector)
	}
	if m.autoMemoryExtractor != nil {
		m.autoMemoryExtractor.SetLLMClient(llm)
	}
	if m.reversibleCompactor != nil {
		m.reversibleCompactor.SetSummarizer(m.summarizer)
	}
}

// CompressWith delegates to the configured Compressor strategy. Returns
// the compressed output or an error when no Compressor is set.
func (m *compactionManager) CompressWith(ctx context.Context, input string) (*CompressedOutput, error) {
	if m.compressor == nil {
		return nil, fmt.Errorf("CompressWith: %w", ErrNoCompressor)
	}
	return m.compressor.Compress(ctx, input)
}

// RetrieveSummary delegates to the retrieval store to fetch a formatted
// summary by ID (Bindle).
func (m *compactionManager) RetrieveSummary(ctx context.Context, summaryID string) (string, error) {
	if m.retrieval == nil {
		return "", fmt.Errorf("RetrieveSummary: %w", ErrNoRetrievalStore)
	}
	return m.retrieval.Bindle(ctx, summaryID)
}

func (m *compactionManager) getOrCreateProviderState(sessionID string) *providerTokenState {
	actual, _ := m.providerState.LoadOrStore(sessionID, &providerTokenState{})
	return actual.(*providerTokenState)
}

// SetActualPromptTokens records the provider-reported prompt token count
// after an LLM call and resets the pending-item delta.
func (m *compactionManager) SetActualPromptTokens(sessionID string, tokens int64) {
	state := m.getOrCreateProviderState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.promptTokens = tokens
	state.pendingItemTokens = 0
}

// AddPendingItemTokens accumulates estimated tokens for messages created
// since the last provider report.
func (m *compactionManager) AddPendingItemTokens(sessionID string, tokens int64) {
	state := m.getOrCreateProviderState(sessionID)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.pendingItemTokens += tokens
}

var filePathPattern = regexp.MustCompile(
	`(?:^|[\s` + "`" + `"'(])([a-zA-Z0-9_./\-]+\.(?:go|ts|tsx|js|jsx|py|rs|rb|java|c|cpp|h|hpp|css|html|sql|yaml|yml|json|toml|md|sh))\b`,
)

// GetSummaryMentionedPaths extracts file paths mentioned in LCM summaries
// for a session. Used as weak ranking hints for the repo map.
func (m *compactionManager) GetSummaryMentionedPaths(ctx context.Context, sessionID string) ([]string, error) {
	summaries, err := m.querier.ListLcmSummariesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var paths []string
	for _, s := range summaries {
		matches := filePathPattern.FindAllStringSubmatch(s.Content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				p := match[1]
				if _, ok := seen[p]; !ok {
					seen[p] = struct{}{}
					paths = append(paths, p)
				}
			}
		}
	}
	return paths, nil
}

// SetRepoMapTokens sets repo map token overhead for a session and atomically
// updates both in-memory cache and persisted thresholds.
func (m *compactionManager) SetRepoMapTokens(ctx context.Context, sessionID string, tokens int64) error {
	if tokens < 0 {
		tokens = 0
	}

	mu := m.sessionMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	return m.setRepoMapTokensLocked(ctx, sessionID, tokens)
}

// UpdateContextWindow updates context window for all tracked sessions.
func (m *compactionManager) UpdateContextWindow(ctx context.Context, contextWindow int64) error {
	m.defaultContextWindow = contextWindow

	// Update all cached budgets.
	m.budgetCache.Range(func(key, _ any) bool {
		sessionID := key.(string)
		repoMapTokens := m.repoMapTokensForSession(sessionID)
		budget := ComputeBudget(BudgetConfig{
			ContextWindow:      contextWindow,
			CutoffThreshold:    m.defaultCutoff,
			RepoMapTokens:      repoMapTokens,
			ModelOutputLimit:   m.defaultModelOutputLimit,
			SystemPromptTokens: m.defaultSystemPromptTokens,
			ToolTokens:         m.defaultToolTokens,
		})

		err := m.querier.UpdateLcmSessionConfig(ctx, db.UpdateLcmSessionConfigParams{
			SessionID:           sessionID,
			ModelName:           "",
			ModelCtxMaxTokens:   contextWindow,
			CtxCutoffThreshold:  m.defaultCutoff,
			SoftThresholdTokens: budget.SoftThreshold,
			HardThresholdTokens: budget.HardLimit,
		})
		if err != nil {
			return false
		}

		m.budgetCache.Store(sessionID, budget)
		return true
	})
	return nil
}

func (m *compactionManager) repoMapTokensForSession(sessionID string) int64 {
	if tokens, ok := m.repoMapTokens.Load(sessionID); ok {
		return tokens.(int64)
	}
	return 0
}

func (m *compactionManager) setRepoMapTokensLocked(ctx context.Context, sessionID string, tokens int64) error {
	tx, err := m.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %v: %w", ErrStorageTransaction, err)
	}
	defer tx.Rollback()

	qtx := m.queries.WithTx(tx)
	config, err := qtx.GetLcmSessionConfig(ctx, sessionID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("getting session config: %w", err)
		}

		budget := ComputeBudget(BudgetConfig{
			ContextWindow:      m.defaultContextWindow,
			CutoffThreshold:    m.defaultCutoff,
			RepoMapTokens:      tokens,
			ModelOutputLimit:   m.defaultModelOutputLimit,
			SystemPromptTokens: m.defaultSystemPromptTokens,
			ToolTokens:         m.defaultToolTokens,
		})
		if err := qtx.UpsertLcmSessionConfig(ctx, db.UpsertLcmSessionConfigParams{
			SessionID:           sessionID,
			ModelName:           "",
			ModelCtxMaxTokens:   m.defaultContextWindow,
			CtxCutoffThreshold:  m.defaultCutoff,
			SoftThresholdTokens: budget.SoftThreshold,
			HardThresholdTokens: budget.HardLimit,
		}); err != nil {
			return fmt.Errorf("upserting session config: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing transaction: %v: %w", ErrStorageTransaction, err)
		}
		m.repoMapTokens.Store(sessionID, tokens)
		m.budgetCache.Store(sessionID, budget)
		return nil
	}

	budget := ComputeBudget(BudgetConfig{
		ContextWindow:      config.ModelCtxMaxTokens,
		CutoffThreshold:    config.CtxCutoffThreshold,
		RepoMapTokens:      tokens,
		ModelOutputLimit:   m.defaultModelOutputLimit,
		SystemPromptTokens: m.defaultSystemPromptTokens,
		ToolTokens:         m.defaultToolTokens,
	})

	if err := qtx.UpdateLcmSessionConfig(ctx, db.UpdateLcmSessionConfigParams{
		SessionID:           sessionID,
		ModelName:           config.ModelName,
		ModelCtxMaxTokens:   config.ModelCtxMaxTokens,
		CtxCutoffThreshold:  config.CtxCutoffThreshold,
		SoftThresholdTokens: budget.SoftThreshold,
		HardThresholdTokens: budget.HardLimit,
	}); err != nil {
		return fmt.Errorf("updating session config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %v: %w", ErrStorageTransaction, err)
	}

	m.repoMapTokens.Store(sessionID, tokens)
	m.budgetCache.Store(sessionID, budget)
	return nil
}

// GetBudget returns the token budget for a session.
func (m *compactionManager) GetBudget(ctx context.Context, sessionID string) (Budget, error) {
	// Check cache first.
	if cached, ok := m.budgetCache.Load(sessionID); ok {
		return cached.(Budget), nil
	}

	// Load from DB.
	config, err := m.querier.GetLcmSessionConfig(ctx, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return default budget.
			repoMapTokens := m.repoMapTokensForSession(sessionID)
			budget := ComputeBudget(BudgetConfig{
				ContextWindow:      m.defaultContextWindow,
				CutoffThreshold:    m.defaultCutoff,
				RepoMapTokens:      repoMapTokens,
				ModelOutputLimit:   m.defaultModelOutputLimit,
				SystemPromptTokens: m.defaultSystemPromptTokens,
				ToolTokens:         m.defaultToolTokens,
			})
			return budget, nil
		}
		return Budget{}, fmt.Errorf("getting session config: %w", err)
	}

	repoMapTokens := m.repoMapTokensForSession(sessionID)
	budget := ComputeBudget(BudgetConfig{
		ContextWindow:      config.ModelCtxMaxTokens,
		CutoffThreshold:    config.CtxCutoffThreshold,
		RepoMapTokens:      repoMapTokens,
		ModelOutputLimit:   m.defaultModelOutputLimit,
		SystemPromptTokens: m.defaultSystemPromptTokens,
		ToolTokens:         m.defaultToolTokens,
	})
	m.budgetCache.Store(sessionID, budget)
	return budget, nil
}

// GetContextTokenCount returns the current token count for a session's context.
// When provider-reported prompt tokens are available (from SetActualPromptTokens),
// returns those plus any pending-item tokens added since the last provider report.
// Falls back to the lcm_context_items sum otherwise.
func (m *compactionManager) GetContextTokenCount(ctx context.Context, sessionID string) (int64, error) {
	if state, ok := m.providerState.Load(sessionID); ok {
		s := state.(*providerTokenState)
		s.mu.Lock()
		total := s.promptTokens + s.pendingItemTokens
		s.mu.Unlock()
		if total > 0 {
			return total, nil
		}
	}
	return m.store.GetContextTokenCount(ctx, sessionID)
}

// IsOverSoftThreshold checks if a session is over the soft threshold.
func (m *compactionManager) IsOverSoftThreshold(ctx context.Context, sessionID string) (ThresholdCheck, error) {
	budget, err := m.GetBudget(ctx, sessionID)
	if err != nil {
		return ThresholdCheck{}, err
	}

	tokenCount, err := m.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return ThresholdCheck{}, err
	}

	return ThresholdCheck{
		CurrentTokens: tokenCount,
		SoftLimit:     budget.SoftThreshold,
		HardLimit:     budget.HardLimit,
		OverSoft:      tokenCount > budget.SoftThreshold,
		OverHard:      tokenCount > budget.HardLimit,
	}, nil
}

// IsOverHardLimit checks if a session is over the hard limit.
func (m *compactionManager) IsOverHardLimit(ctx context.Context, sessionID string) (ThresholdCheck, error) {
	budget, err := m.GetBudget(ctx, sessionID)
	if err != nil {
		return ThresholdCheck{}, err
	}

	tokenCount, err := m.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return ThresholdCheck{}, err
	}

	return ThresholdCheck{
		CurrentTokens: tokenCount,
		SoftLimit:     budget.SoftThreshold,
		HardLimit:     budget.HardLimit,
		OverSoft:      tokenCount > budget.SoftThreshold,
		OverHard:      tokenCount > budget.HardLimit,
	}, nil
}

// GetContextFiles returns LCM context files for injection into the system prompt.
func (m *compactionManager) GetContextFiles() []ContextFile {
	return []ContextFile{{Name: "LCM Instructions", Content: LCMSystemPrompt}}
}

// CompactUntilUnderLimit runs compaction until the session is under the hard
// limit. Acquires a per-session mutex to prevent concurrent compactions and
// publishes compaction events for the UI.
func (m *compactionManager) CompactUntilUnderLimit(ctx context.Context, sessionID string) error {
	mu := m.sessionMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	budget, _ := m.GetBudget(ctx, sessionID)
	tokenCount, _ := m.store.GetContextTokenCount(ctx, sessionID)
	hookInput := buildPreCompactInput(sessionID, tokenCount, budget, true)
	hookDecision := runPreCompactHooks(ctx, m.preCompactRunner, sessionID, hookInput)

	if hookDecision.Skip {
		slog.Info("LCM hard-limit compaction skipped by hook",
			"session_id", sessionID, "reason", hookDecision.Reason)
		return nil
	}

	start := time.Now()
	m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
		Type:      CompactionStarted,
		SessionID: sessionID,
		Blocking:  true,
	})

	err := m.compactUntilUnderLimitLocked(ctx, sessionID, hookDecision)

	if err != nil {
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionFailed,
			SessionID: sessionID,
			Error:     err.Error(),
		})
	} else {
		tokenCountAfter, _ := m.store.GetContextTokenCount(ctx, sessionID)
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionCompleted,
			SessionID: sessionID,
			Success:   true,
		})
		postOutput := CompactHookOutput{
			SessionID:       sessionID,
			Success:         true,
			TokenCountAfter: tokenCountAfter,
			Blocking:        true,
			DurationMs:      time.Since(start).Milliseconds(),
		}
		postHookDecision := runPostCompactHooks(ctx, m.postCompactRunner, sessionID, postOutput)
		if postHookDecision.Halt {
			slog.Warn("LCM: PostCompact hook requested halt after blocking compaction",
				"session_id", sessionID, "reason", postHookDecision.Reason)
		}
		go m.PostCompactionHook(ctx, sessionID)
	}
	return err
}

// Compact runs compaction for a session and publishes events. It delegates to
// compactLocked which performs the two-phase approach (layered compaction first,
// then LLM summarization fallback if still over the soft threshold).
func (m *compactionManager) Compact(ctx context.Context, sessionID string, opts ...CompactOption) error {
	cfg := &compactConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	mu := m.sessionMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	budget, _ := m.GetBudget(ctx, sessionID)
	tokenCount, _ := m.store.GetContextTokenCount(ctx, sessionID)
	hookInput := buildPreCompactInput(sessionID, tokenCount, budget, false)
	hookDecision := runPreCompactHooks(ctx, m.preCompactRunner, sessionID, hookInput)

	if hookDecision.Skip {
		slog.Info("LCM compaction skipped by hook",
			"session_id", sessionID, "reason", hookDecision.Reason)
		return nil
	}

	start := time.Now()
	m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
		Type:      CompactionStarted,
		SessionID: sessionID,
	})

	result, err := m.compactLocked(ctx, sessionID, hookDecision, cfg)
	if err != nil {
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionFailed,
			SessionID: sessionID,
			Error:     err.Error(),
		})
		return err
	}

	m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
		Type:      CompactionCompleted,
		SessionID: sessionID,
		Rounds:    result.Rounds,
		Success:   result.ActionTaken,
	})

	postOutput := buildPostCompactOutput(sessionID, result, false, start)
	postHookDecision := runPostCompactHooks(ctx, m.postCompactRunner, sessionID, postOutput)
	if postHookDecision.Halt {
		slog.Warn("LCM: PostCompact hook requested halt",
			"session_id", sessionID, "reason", postHookDecision.Reason)
	}

	go m.PostCompactionHook(ctx, sessionID)

	return nil
}

// compactLocked performs the two-phase compaction without publishing events.
// Phase 1: Run layered compaction (8 optimization layers), unless
// hookDecision.ForceFull is set.
// Phase 2: If still over soft threshold, fall back to LLM summarization +
// condensation (trySummarize/tryCondense).
// Caller must hold the per-session mutex.
func (m *compactionManager) compactLocked(ctx context.Context, sessionID string, hookDecision CompactHookDecision, cfg *compactConfig) (CompactionResult, error) {
	var actionTaken bool
	var rounds int

	var overrideTier *PressureTier
	if cfg.Pressure != "" {
		if tier, ok := parsePressureTier(cfg.Pressure); ok {
			overrideTier = &tier
		}
	}

	if !hookDecision.ForceFull {
		// Phase 1: Run layered compaction.
		layerResult, err := m.RunLayeredCompaction(ctx, sessionID, overrideTier)
		if err != nil {
			return CompactionResult{}, err
		}

		actionTaken = layerResult != nil && layerResult.ActionTaken
		if actionTaken {
			rounds = 1
		}
	}

	// Phase 2: If still over soft threshold, fall back to LLM summarization.
	budget, budgetErr := m.GetBudget(ctx, sessionID)
	tokensAfter, tokenErr := m.store.GetContextTokenCount(ctx, sessionID)
	softThreshold := budget.SoftThreshold
	if cfg.TargetTokens > 0 {
		softThreshold = cfg.TargetTokens
	}
	stillOver := budgetErr != nil || tokenErr != nil || tokensAfter > softThreshold
	if stillOver || hookDecision.ForceFull {
		onceResult, err := m.runLLMSummarization(ctx, sessionID)
		if err != nil {
			return CompactionResult{}, err
		}
		if onceResult.ActionTaken {
			actionTaken = true
			rounds += onceResult.Rounds
		}
	}

	tokenCount, _ := m.store.GetContextTokenCount(ctx, sessionID)
	return CompactionResult{
		Rounds:      rounds,
		ActionTaken: actionTaken,
		TokenCount:  tokenCount,
	}, nil
}

// runLLMSummarization performs a single round of LLM summarization and
// condensation. This is the fallback path when layered compaction alone
// doesn't bring tokens below the soft threshold.
func (m *compactionManager) runLLMSummarization(ctx context.Context, sessionID string) (CompactionResult, error) {
	budget, err := m.GetBudget(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("getting budget: %w", err)
	}

	var tokenCount int64

	// Try leaf summarization first.
	summarized, err := m.trySummarize(ctx, sessionID, budget)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("summarization: %w", err)
	}

	if summarized {
		tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
		if err != nil {
			return CompactionResult{}, fmt.Errorf("getting token count after summarization: %w", err)
		}

		// If still over threshold, try condensation.
		if tokenCount > budget.SoftThreshold {
			condensed, err := m.tryCondense(ctx, sessionID, budget)
			if err != nil {
				return CompactionResult{}, fmt.Errorf("condensation: %w", err)
			}
			if condensed {
				tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
				if err != nil {
					return CompactionResult{}, fmt.Errorf("getting token count after condensation: %w", err)
				}
			}
		}

		return CompactionResult{
			Rounds:      1,
			ActionTaken: true,
			TokenCount:  tokenCount,
		}, nil
	}

	// No messages to summarize, try condensation directly.
	condensed, err := m.tryCondense(ctx, sessionID, budget)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("condensation: %w", err)
	}

	tokenCount, err = m.store.GetContextTokenCount(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("getting token count after condensation: %w", err)
	}

	return CompactionResult{
		Rounds:      1,
		ActionTaken: condensed,
		TokenCount:  tokenCount,
	}, nil
}

// ScheduleCompaction schedules an async soft-threshold compaction for a session.
// Returns a channel that receives the result when done. Uses sync.Map for
// deduplication to prevent concurrent compactions on the same session.
func (m *compactionManager) ScheduleCompaction(ctx context.Context, sessionID string) <-chan CompactionResult {
	resultCh := make(chan CompactionResult, 1)

	// Deduplicate: skip if already in flight.
	if _, loaded := m.inFlight.LoadOrStore(sessionID, struct{}{}); loaded {
		resultCh <- CompactionResult{}
		return resultCh
	}

	// Detach from parent context so compaction can complete even if the
	// request context is canceled.
	detachedCtx := context.WithoutCancel(ctx)

	go func() {
		defer m.inFlight.Delete(sessionID)
		// closed in ScheduleCompaction
		defer close(resultCh)

		mu := m.sessionMutex(sessionID)
		mu.Lock()
		defer mu.Unlock()

		budget, budgetErr := m.GetBudget(detachedCtx, sessionID)
		tokenCount, tokenErr := m.store.GetContextTokenCount(detachedCtx, sessionID)

		// Check soft threshold before running compaction (force=false behavior).
		if budgetErr == nil && tokenErr == nil && tokenCount <= budget.SoftThreshold {
			m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
				Type:      CompactionCompleted,
				SessionID: sessionID,
				Success:   false,
			})
			resultCh <- CompactionResult{TokenCount: tokenCount}
			return
		}

		var hookInput CompactHookInput
		if budgetErr == nil && tokenErr == nil {
			hookInput = buildPreCompactInput(sessionID, tokenCount, budget, false)
		}
		hookDecision := runPreCompactHooks(detachedCtx, m.preCompactRunner, sessionID, hookInput)

		if hookDecision.Skip {
			slog.Info("LCM scheduled compaction skipped by hook",
				"session_id", sessionID, "reason", hookDecision.Reason)
			m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
				Type:      CompactionCompleted,
				SessionID: sessionID,
				Success:   false,
			})
			resultCh <- CompactionResult{TokenCount: tokenCount}
			return
		}

		start := time.Now()
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionStarted,
			SessionID: sessionID,
		})

		result, err := m.compactLocked(detachedCtx, sessionID, hookDecision, &compactConfig{})
		if err != nil {
			m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
				Type:      CompactionFailed,
				SessionID: sessionID,
				Error:     err.Error(),
			})
			resultCh <- CompactionResult{}
			return
		}

		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionCompleted,
			SessionID: sessionID,
			Rounds:    result.Rounds,
			Success:   result.ActionTaken,
		})

		postOutput := buildPostCompactOutput(sessionID, result, false, start)
		postHookDecision := runPostCompactHooks(detachedCtx, m.postCompactRunner, sessionID, postOutput)
		if postHookDecision.Halt {
			slog.Warn("LCM: PostCompact hook requested halt after scheduled compaction",
				"session_id", sessionID, "reason", postHookDecision.Reason)
		}

		go m.PostCompactionHook(ctx, sessionID)

		resultCh <- result
	}()

	return resultCh
}

// CompactIfOverHardLimit checks if the session is over the hard limit and runs
// blocking compaction if so. Errors are logged and suppressed; callers always
// continue regardless of compaction outcome.
func (m *compactionManager) CompactIfOverHardLimit(ctx context.Context, sessionID string) {
	check, err := m.IsOverHardLimit(ctx, sessionID)
	if err != nil {
		slog.Warn("Failed to check hard limit", "session_id", sessionID, "error", err)
		return
	}
	if !check.OverHard {
		return
	}
	slog.Info("Context over hard limit, running compaction", "session_id", sessionID)
	if err := m.CompactUntilUnderLimit(ctx, sessionID); err != nil {
		slog.Warn("Hard-limit compaction failed", "session_id", sessionID, "error", err)
	}
}

// CompactIfOverHardLimit is a nil-safe wrapper: it is a no-op when mgr is nil.
func CompactIfOverHardLimit(ctx context.Context, mgr Manager, sessionID string) {
	if mgr != nil {
		mgr.CompactIfOverHardLimit(ctx, sessionID)
	}
}

// PostTurnHookSafe is a nil-safe wrapper: it is a no-op when mgr is nil.
func PostTurnHookSafe(ctx context.Context, mgr Manager, sessionID string) {
	if mgr != nil {
		mgr.PostTurnHook(ctx, sessionID)
	}
}

// ExtraAgentTools returns LCM-related agent tools for injection into the coder agent.
func (m *compactionManager) ExtraAgentTools() []fantasy.AgentTool {
	var base []fantasy.AgentTool
	if m.toolFactory != nil {
		base = []fantasy.AgentTool{
			m.toolFactory.NewLcmGrepTool(m.sqlDB),
			m.toolFactory.NewLcmDescribeTool(m.sqlDB),
			m.toolFactory.NewLcmExpandTool(m.sqlDB),
			m.toolFactory.NewLlmMapTool(m.sqlDB),
			m.toolFactory.NewAgenticMapTool(m.sqlDB),
		}
	}
	base = append(base,
		newBindleTool(m.store),
		newAncestryTool(m.store),
		newDoltTool(m.store),
		newArchiveTool(m.store),
		newSprigTool(m.store),
		newTimeQueryTool(m.store),
		newFileSearchTool(m.store),
		newActiveContextTool(m.store),
		newLineageTool(m.store),
		newCompactTool(m),
	)
	return base
}

// ExtraAgentTools is a nil-safe wrapper: returns nil when mgr is nil.
func ExtraAgentTools(mgr Manager) []fantasy.AgentTool {
	if mgr == nil {
		return nil
	}
	return mgr.ExtraAgentTools()
}

// ---------------------------------------------------------------------------
// Layer integration methods (Tasks 28-42)
// ---------------------------------------------------------------------------

// tokenUsageForSession returns a TokenUsageFunc bound to the given session.
func (m *compactionManager) tokenUsageForSession(sessionID string) TokenUsageFunc {
	return func(ctx context.Context) (int64, int64, error) {
		budget, err := m.GetBudget(ctx, sessionID)
		if err != nil {
			return 0, 0, err
		}
		tokens, err := m.GetContextTokenCount(ctx, sessionID)
		if err != nil {
			return 0, 0, err
		}
		return tokens, budget.ContextWindow, nil
	}
}

func filterNilLayers(layers ...CompactionLayer) []CompactionLayer {
	filtered := make([]CompactionLayer, 0, len(layers))
	for _, l := range layers {
		if l != nil {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

// newSessionLayerManager builds a CompactionLayerManager for the given session
// with all currently available layers wired in.
func (m *compactionManager) newSessionLayerManager(sessionID string, overrideTier ...*PressureTier) *CompactionLayerManager {
	microCompactor := NewMicroCompactor(MicroCompactorConfig{
		Store:     m.store,
		SessionID: sessionID,
	})

	maxFiles := m.postCompactMaxFiles
	if maxFiles == 0 {
		maxFiles = 5
	}
	tokenBudget := m.postCompactTokenBudget
	if tokenBudget == 0 {
		tokenBudget = 50000
	}
	postCompactCleaner := NewPostCompactCleaner(PostCompactCleanerConfig{
		Store:               m.preservedContextStore,
		SessionID:           sessionID,
		AgentConfigRestorer: m.agentConfigRestorer,
		OMStore:             m.opMemory,
		MaxFiles:            maxFiles,
		TokenBudget:         tokenBudget,
	})

	cacheOpt := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType:       m.providerType,
		Store:              m.store,
		SessionID:          sessionID,
		NudgeInjector:      m.nudgeInjector,
		TurnCountFunc:      func() int64 { return m.GetTurnCount(sessionID) },
		IterationCountFunc: func() int64 { return m.GetIterationCount(sessionID) },
		ContextWindowFunc:  func() int64 { return m.defaultContextWindow },
	})

	microCompactor.cfg.CacheAware = true
	microCompactor.cfg.ProviderType = m.providerType
	microCompactor.cfg.CacheOptimizer = cacheOpt

	llm := m.summarizer.llmClient()

	sessionCompactor := NewSessionCompactor(SessionCompactorConfig{
		Store:     m.store,
		LLM:       llm,
		SessionID: sessionID,
	})

	fullCompactor := NewFullCompactor(FullCompactorConfig{
		LLM:       llm,
		Store:     m.store,
		SessionID: sessionID,
	})

	var dedupLayer CompactionLayer
	if m.deduplicationEnabled {
		dedupLayer = NewDedupCompactionLayer(m.store, sessionID)
	}
	staleLayer := NewStaleEvictionLayer(m.store, sessionID, 0)
	adjacentLayer := NewAdjacentCondensationLayer(m.store, sessionID, 0)
	timeGapLayer := NewTimeGapCompactor(TimeGapCompactorConfig{
		Store:     m.store,
		SessionID: sessionID,
	})

	usageFn := m.tokenUsageForSession(sessionID)
	pressureSelector := NewPressureCompactionSelector(
		DefaultPressureConfig(),
		usageFn,
		map[PressureTier][]CompactionLayer{
			PressureLow:    {microCompactor},
			PressureMedium: {sessionCompactor, microCompactor},
			PressureHigh:   {fullCompactor, microCompactor},
		},
	)

	if len(overrideTier) > 0 && overrideTier[0] != nil {
		pressureSelector.SetOverrideTier(*overrideTier[0])
	}

	return NewCompactionLayerManager(filterNilLayers(
		microCompactor,
		timeGapLayer,
		dedupLayer,
		staleLayer,
		postCompactCleaner,
		adjacentLayer,
		pressureSelector,
		cacheOpt.Layer6(),
		cacheOpt.Layer7(),
	)...)
}

// RunLayeredCompaction executes the multi-layer compaction framework for a
// session. It builds a session-specific CompactionLayerManager with all wired
// layers and runs them in priority order.
func (m *compactionManager) RunLayeredCompaction(ctx context.Context, sessionID string, overrideTier ...*PressureTier) (*CompactionLayerResult, error) {
	budget, err := m.GetBudget(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("layered compaction: getting budget: %w", err)
	}

	lm := m.newSessionLayerManager(sessionID, overrideTier...)
	return lm.RunAll(ctx, budget)
}

// InjectCuesIntoPrompt injects ghost cues into a prompt string using the
// CueInjector, respecting the given token budget.
func (m *compactionManager) InjectCuesIntoPrompt(prompt string, cues []GhostCue, tokenBudget int64) string {
	if m.cueInjector == nil {
		return prompt
	}
	return m.cueInjector.InjectIntoPrompt(prompt, cues, tokenBudget)
}

// BuildCompactPrompt assembles the compact prompt for a session using the
// CacheOptimizer's BuildPrompt method.
func (m *compactionManager) BuildCompactPrompt(ctx context.Context, sessionID string) (string, error) {
	opt := NewCacheOptimizer(CacheOptimizerConfig{
		ProviderType:       m.providerType,
		Store:              m.store,
		SessionID:          sessionID,
		NudgeInjector:      m.nudgeInjector,
		TurnCountFunc:      func() int64 { return m.GetTurnCount(sessionID) },
		IterationCountFunc: func() int64 { return m.GetIterationCount(sessionID) },
		ContextWindowFunc:  func() int64 { return m.defaultContextWindow },
	})

	entries, err := m.store.GetContextEntries(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("building compact prompt: %w", err)
	}

	return opt.BuildPrompt(ctx, entries)
}

// PostCompactionHook runs observation extraction, reflection buffering, and
// post-compact context restore after a compaction cycle completes. Errors are
// logged and do not propagate to avoid disrupting the compaction flow.
func (m *compactionManager) PostCompactionHook(ctx context.Context, sessionID string) {
	// Step 1: Run observation if threshold is met.
	if m.observer != nil {
		shouldObserve, err := m.observer.ShouldObserve(ctx, sessionID)
		if err != nil {
			slog.Warn("PostCompactionHook: failed to check observation threshold",
				"session_id", sessionID, "error", err)
		} else if shouldObserve {
			resultCh := m.observer.Observe(ctx, sessionID)
			if resultCh != nil {
				result := <-resultCh
				if result.Error != nil {
					slog.Warn("PostCompactionHook: observation failed",
						"session_id", sessionID, "error", result.Error)
				}
			}
		}
	}

	// Step 2: Run buffering coordinator to check if reflection is needed.
	if m.bufferingCoordinator != nil {
		resultCh, err := m.bufferingCoordinator.Collect(ctx, sessionID)
		if err != nil {
			slog.Warn("PostCompactionHook: buffer collection failed",
				"session_id", sessionID, "error", err)
		} else if resultCh != nil {
			result := <-resultCh
			if result.Error != nil {
				slog.Warn("PostCompactionHook: reflection failed",
					"session_id", sessionID, "error", result.Error)
			}
		}
	}

	// Step 3: Run post-compact restore (Layer 4) if preserved context exists.
	if m.preservedContextStore != nil {
		cleaner := NewPostCompactCleaner(PostCompactCleanerConfig{
			Store:               m.preservedContextStore,
			SessionID:           sessionID,
			AgentConfigRestorer: m.agentConfigRestorer,
			OMStore:             m.opMemory,
		})
		budget, err := m.GetBudget(ctx, sessionID)
		if err != nil {
			slog.Warn("PostCompactionHook: failed to get budget for restore",
				"session_id", sessionID, "error", err)
			return
		}
		if cleaner.ShouldCompact(ctx, budget) {
			if _, err := cleaner.Compact(ctx, budget); err != nil {
				slog.Warn("PostCompactionHook: post-compact restore failed",
					"session_id", sessionID, "error", err)
			}
		}
	}
}

// PostTurnHook runs auto-memory extraction after an agent turn completes.
// It persists extracted memories in operational memory for cross-session recall.
func (m *compactionManager) PostTurnHook(ctx context.Context, sessionID string) {
	if m.autoMemoryExtractor == nil {
		return
	}

	counterPtr, _ := m.turnCounter.LoadOrStore(sessionID, new(atomic.Int64))
	counter := counterPtr.(*atomic.Int64)
	newCount := counter.Add(1)

	// Iterations accumulate within a single turn, not across turns.
	if iterPtr, ok := m.iterationCounter.Load(sessionID); ok {
		iterPtr.(*atomic.Int64).Store(0)
	}

	if !m.autoMemoryExtractor.ShouldExtract(int(newCount)) {
		return
	}

	resultCh := m.autoMemoryExtractor.Extract(ctx, sessionID)
	if resultCh == nil {
		return
	}
	go func() {
		result := <-resultCh
		if result.Error != nil {
			slog.Warn("Auto-memory extraction failed",
				"session_id", sessionID, "error", result.Error)
		}
		if m.opMemory != nil && m.operationalMemEnabled {
			for _, mem := range result.Memories {
				key := fmt.Sprintf("memory:%s:%s", mem.Type, truncateString(mem.Content, 60))
				// XRUSH: log error before discarding
				if err := m.opMemory.Set(ctx, sessionID, key, mem.Content); err != nil {
					slog.Warn("Auto-memory store failed", "session_id", sessionID, "key", key, "error", err)
				}
			}
		}
	}()
}

// SetProviderType updates the LLM provider type for cache-optimization.
func (m *compactionManager) SetProviderType(providerType string) {
	m.providerType = providerType
}

// SetOperationalMemory connects the session's operational memory store.
func (m *compactionManager) SetOperationalMemory(om OperationalMemoryStore) {
	m.opMemory = om
}

func (m *compactionManager) SetOperationalMemoryEnabled(enabled bool) {
	m.operationalMemEnabled = enabled
}

func (m *compactionManager) SetDeduplicationEnabled(enabled bool) {
	m.deduplicationEnabled = enabled
}

func (m *compactionManager) SetPurgeErrorsEnabled(enabled bool) {
	m.purgeErrorsEnabled = enabled
}

func (m *compactionManager) SetHookRunners(preCompact, postCompact *hooks.Runner) {
	m.preCompactRunner = preCompact
	m.postCompactRunner = postCompact
}

// SetAgentConfigRestorer connects the agent's config restorer so that
// checkpointed session agent configuration (skills, tools, agents) is
// restored after compaction.
func (m *compactionManager) SetAgentConfigRestorer(restorer AgentConfigRestorer) {
	m.agentConfigRestorer = restorer
}

func (m *compactionManager) SetNudgeInjector(injector *nudge.NudgeInjector) {
	m.nudgeInjector = injector
}

func (m *compactionManager) SetPostCompactConfig(maxFiles int, tokenBudget int64) {
	m.postCompactMaxFiles = maxFiles
	m.postCompactTokenBudget = tokenBudget
}

// GetTurnCount returns the current turn count for a session. Returns 0 for
// unknown sessions.
func (m *compactionManager) GetTurnCount(sessionID string) int64 {
	val, ok := m.turnCounter.Load(sessionID)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}

// GetIterationCount returns the current iteration count for a session. Returns
// 0 for unknown sessions.
func (m *compactionManager) GetIterationCount(sessionID string) int64 {
	val, ok := m.iterationCounter.Load(sessionID)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}

// IncrementIteration atomically increments the iteration counter for a session.
func (m *compactionManager) IncrementIteration(sessionID string) {
	counterPtr, _ := m.iterationCounter.LoadOrStore(sessionID, new(atomic.Int64))
	counterPtr.(*atomic.Int64).Add(1)
}

func (m *compactionManager) SetObservationConfig(strategyName string, threshold int64, observerModel string, reflectorModel string, observerBufferRatio float64, reflectorObservationTokens int64, reflectorBufferActivation float64) {
	strategy := NewObservationStrategyFromConfig(strategyName)
	m.observer.SetStrategy(strategy)
	m.observer.SetThreshold(threshold)
	m.observer.SetModelOverrides(observerModel, reflectorModel)
	if reflectorObservationTokens > 0 {
		m.reflector.SetThreshold(reflectorObservationTokens)
	}
	// observerBufferRatio controls the BufferingCoordinator's interval
	// fraction (default 0.2). reflectorBufferActivation is a distinct
	// concept (when async reflection starts relative to observationTokens,
	// default 0.5) and is NOT wired through SetThresholdPercent, which
	// would overwrite the observer ratio.
	if observerBufferRatio > 0 {
		m.bufferingCoordinator.SetThresholdPercent(observerBufferRatio)
	}
	_ = reflectorBufferActivation
}

func (m *compactionManager) OnSessionStart(ctx context.Context, sessionID string) error {
	if m.opMemory == nil || !m.operationalMemEnabled {
		return nil
	}
	if err := m.opMemory.Set(ctx, sessionID, "session_started_at", time.Now().Format(time.RFC3339)); err != nil {
		slog.Warn("LCM lifecycle: failed to set session start marker",
			"session_id", sessionID, "error", err)
	}
	return nil
}

func (m *compactionManager) OnSessionEnd(ctx context.Context, sessionID string) error {
	if m.opMemory == nil || !m.operationalMemEnabled {
		return nil
	}
	entries, err := m.opMemory.List(ctx, sessionID)
	if err != nil {
		slog.Warn("LCM lifecycle: failed to list OM entries for session end",
			"session_id", sessionID, "error", err)
		return nil
	}
	if len(entries) > 0 {
		if err := m.opMemory.Set(ctx, sessionID, "session_ended_at", time.Now().Format(time.RFC3339)); err != nil {
			slog.Warn("LCM lifecycle: failed to set session end marker",
				"session_id", sessionID, "error", err)
		}
	}
	return nil
}

// PreserveContext saves operational context for a session before compaction
// runs. The PostCompactionHook will restore it after compaction completes.
func (m *compactionManager) PreserveContext(sessionID string, pc *PreservedContext) {
	if m.preservedContextStore != nil {
		m.preservedContextStore.Save(sessionID, pc)
	}
}

// GetPressureTier returns the current pressure tier for a session.
func (m *compactionManager) GetPressureTier(ctx context.Context, sessionID string) (PressureTier, error) {
	usageFn := m.tokenUsageForSession(sessionID)
	currentTokens, contextWindow, err := usageFn(ctx)
	if err != nil {
		return PressureLow, fmt.Errorf("getting pressure tier: %w", err)
	}
	_, tier := CalculatePressureTier(currentTokens, contextWindow, DefaultPressureConfig())
	return tier, nil
}

// Observations returns stored observations for a session (delegates to the
// ObservationCoordinator).
func (m *compactionManager) Observations(ctx context.Context, sessionID string) ([]Observation, error) {
	if m.observer == nil {
		return nil, nil
	}
	return m.observer.ListObservations(ctx, sessionID)
}

// Reflections returns stored reflections for a session (delegates to the
// ReflectorAgent).
func (m *compactionManager) Reflections(ctx context.Context, sessionID string) ([]Reflection, error) {
	if m.reflector == nil {
		return nil, nil
	}
	return m.reflector.ListReflections(ctx, sessionID)
}

func (m *compactionManager) ListObservations(ctx context.Context, sessionID string) ([]Observation, error) {
	if m.observer == nil {
		return nil, nil
	}
	return m.observer.ListObservations(ctx, sessionID)
}

// GetObservationPrompt returns formatted observation text for injection into
// the system prompt. Observations are already sorted by priority descending
// from the database. A greedy token budget is applied to truncate at the
// budget limit, following the CueInjector pattern.
func (m *compactionManager) GetObservationPrompt(ctx context.Context, sessionID string, tokenBudget int64) (string, error) {
	if tokenBudget <= 0 {
		return "", nil
	}

	observations, err := m.ListObservations(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("listing observations for prompt: %w", err)
	}
	if len(observations) == 0 {
		return "", nil
	}

	// Format each observation as structured text and accumulate within budget.
	var sb strings.Builder
	remaining := tokenBudget
	for _, obs := range observations {
		text := formatObservationForPrompt(obs)
		cost := EstimateTokens(text)
		if cost > remaining {
			break
		}
		sb.WriteString(text)
		remaining -= cost
	}

	return sb.String(), nil
}

func (m *compactionManager) ListReflections(ctx context.Context, sessionID string) ([]Reflection, error) {
	if m.reflector == nil {
		return nil, nil
	}
	return m.reflector.ListReflections(ctx, sessionID)
}

func (m *compactionManager) ListMemories(ctx context.Context, sessionID string) ([]ExtractedMemory, error) {
	if m.autoMemoryExtractor == nil {
		return nil, nil
	}
	return m.autoMemoryExtractor.ListMemories(ctx, sessionID)
}

func (m *compactionManager) ListOMEntries(ctx context.Context, sessionID string) ([]session.OMEntry, error) {
	if m.opMemory == nil || !m.operationalMemEnabled {
		return nil, nil
	}
	return m.opMemory.ListByPriority(ctx, sessionID)
}

// FlushReflections forces an immediate reflection cycle for a session,
// useful for end-of-session cleanup.
func (m *compactionManager) FlushReflections(ctx context.Context, sessionID string) {
	if m.bufferingCoordinator != nil {
		resultCh := m.bufferingCoordinator.Flush(ctx, sessionID)
		if resultCh != nil {
			result := <-resultCh
			if result.Error != nil {
				slog.Warn("FlushReflections: reflection failed",
					"session_id", sessionID, "error", result.Error)
			}
		}
	}
}

// RestoreReplacement restores a single content replacement by ID.
func (m *compactionManager) RestoreReplacement(ctx context.Context, id int64) error {
	if m.contentReplacements == nil {
		return fmt.Errorf("RestoreReplacement: %w", ErrNoActiveReplacement)
	}
	row, err := m.querier.GetContentReplacement(ctx, id)
	if err != nil {
		return fmt.Errorf("RestoreReplacement: getting replacement %d: %w", id, err)
	}
	if ReplacementState(row.State) != ReplacementActive {
		return ErrNoActiveReplacement
	}
	return m.contentReplacements.UpdateState(ctx, id, ReplacementRestored)
}

// RestoreAllByRound restores all content replacements for a session round.
func (m *compactionManager) RestoreAllByRound(ctx context.Context, sessionID string, round int) error {
	if m.contentReplacements == nil {
		return nil
	}
	replacements, err := m.contentReplacements.ListByRound(ctx, sessionID, round)
	if err != nil {
		return fmt.Errorf("RestoreAllByRound: listing round %d: %w", round, err)
	}
	for _, r := range replacements {
		if r.State != ReplacementActive {
			continue
		}
		if err := m.contentReplacements.UpdateState(ctx, r.ID, ReplacementRestored); err != nil {
			slog.Warn("RestoreAllByRound: failed to restore",
				"session_id", sessionID, "id", r.ID, "error", err)
		}
	}
	return nil
}

// PinEntry pins a context entry at the given position.
func (m *compactionManager) PinEntry(ctx context.Context, sessionID string, position int64) error {
	if m.contentReplacements == nil {
		return fmt.Errorf("PinEntry: content replacement store not configured")
	}
	id, err := m.contentReplacements.RecordReplacement(ctx, ContentReplacement{
		SessionID:             sessionID,
		Position:              position,
		State:                 ReplacementActive,
		Round:                 0,
		OriginalTokenCount:    0,
		ReplacementTokenCount: 0,
	})
	if err != nil {
		return fmt.Errorf("PinEntry: recording pin: %w", err)
	}
	return m.contentReplacements.UpdateState(ctx, id, ReplacementPinned)
}

// CleanOrphanedReplacements removes replacement records whose referenced
// context entry no longer exists.
func (m *compactionManager) CleanOrphanedReplacements(ctx context.Context, sessionID string) (int, error) {
	if m.sqlDB == nil {
		return 0, nil
	}
	rows, err := m.sqlDB.QueryContext(ctx,
		`SELECT cr.id FROM lcm_content_replacements cr
		 LEFT JOIN lcm_context_items ci ON cr.session_id = ci.session_id AND cr.position = ci.position
		 WHERE cr.session_id = ? AND ci.session_id IS NULL`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("CleanOrphanedReplacements: querying orphans: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("CleanOrphanedReplacements: scanning: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("CleanOrphanedReplacements: iterating: %w", err)
	}

	for _, id := range ids {
		if _, err := m.sqlDB.ExecContext(ctx,
			"DELETE FROM lcm_content_replacements WHERE id = ?", id,
		); err != nil {
			slog.Warn("CleanOrphanedReplacements: failed to delete",
				"session_id", sessionID, "id", id, "error", err)
		}
	}
	return len(ids), nil
}
