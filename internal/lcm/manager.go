package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
)

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

	// SetModelOutputLimit sets the model's max output token limit for budget computation.
	SetModelOutputLimit(limit int64)

	// SetRepoMapTokens sets repo map token overhead for session budget computation.
	SetRepoMapTokens(ctx context.Context, sessionID string, tokens int64) error

	// UpdateContextWindow updates context window for all tracked sessions.
	UpdateContextWindow(ctx context.Context, contextWindow int64) error

	// GetContextFiles returns LCM context files for injection into the system prompt.
	GetContextFiles() []ContextFile

	// Compact runs compaction for a session (delegates to CompactOnce with force=true).
	Compact(ctx context.Context, sessionID string) error

	// Subscribe returns a channel of compaction events.
	Subscribe(ctx context.Context) <-chan pubsub.Event[CompactionEvent]

	// GetFormattedContext returns the formatted context for a session.
	GetFormattedContext(ctx context.Context, sessionID string) ([]FormattedContextEntry, error)

	// CompactIfOverHardLimit checks the hard limit and runs blocking compaction
	// if needed. Errors are logged internally; the method always returns to the caller.
	CompactIfOverHardLimit(ctx context.Context, sessionID string)

	// ExtraAgentTools returns LCM-related agent tools for injection into the coder agent.
	ExtraAgentTools() []fantasy.AgentTool
}

type compactionManager struct {
	store      *Store
	querier    db.Querier
	queries    *db.Queries
	sqlDB      *sql.DB
	broker     *pubsub.Broker[CompactionEvent]
	summarizer *Summarizer

	inFlight      sync.Map // sessionID -> struct{} (deduplication)
	budgetCache   sync.Map // sessionID -> Budget
	repoMapTokens sync.Map // sessionID -> int64
	sessionMu     sync.Map // sessionID -> *sync.Mutex (per-session compaction lock)

	defaultContextWindow    int64
	defaultCutoff           float64
	defaultModelOutputLimit int64
}

// sessionMutex returns the per-session mutex, creating it lazily.
func (m *compactionManager) sessionMutex(sessionID string) *sync.Mutex {
	actual, _ := m.sessionMu.LoadOrStore(sessionID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// NewManager creates a new LCM manager.
func NewManager(queries *db.Queries, rawDB *sql.DB) Manager {
	return &compactionManager{
		store:                newStore(queries, rawDB),
		querier:              queries,
		queries:              queries,
		sqlDB:                rawDB,
		broker:               pubsub.NewBroker[CompactionEvent](),
		summarizer:           NewSummarizer(nil),
		defaultContextWindow: 128000,
		defaultCutoff:        0.6,
	}
}

// NewManagerWithLLM creates a new LCM manager with an LLM client for summarization.
func NewManagerWithLLM(queries *db.Queries, rawDB *sql.DB, llm LLMClient) Manager {
	return &compactionManager{
		store:                newStore(queries, rawDB),
		querier:              queries,
		queries:              queries,
		sqlDB:                rawDB,
		broker:               pubsub.NewBroker[CompactionEvent](),
		summarizer:           NewSummarizer(llm),
		defaultContextWindow: 128000,
		defaultCutoff:        0.6,
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
		ContextWindow:    m.defaultContextWindow,
		CutoffThreshold:  m.defaultCutoff,
		RepoMapTokens:    repoMapTokens,
		ModelOutputLimit: m.defaultModelOutputLimit,
	})

	err := m.querier.UpsertLcmSessionConfig(ctx, db.UpsertLcmSessionConfigParams{
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

	// Clear stale summary_message_id from a previous session lifecycle so that
	// getSessionMessages does not truncate LCM's compacted context at the old
	// summary position.
	if err := m.querier.ClearSessionSummaryMessageID(ctx, sessionID); err != nil {
		slog.Warn("Failed to clear stale summary message ID", "session_id", sessionID, "error", err)
	}

	m.budgetCache.Store(sessionID, budget)
	return nil
}

// SetDefaultContextWindow sets the context window for new sessions.
func (m *compactionManager) SetDefaultContextWindow(contextWindow int64) {
	m.defaultContextWindow = contextWindow
}

// SetModelOutputLimit sets the model's max output token limit for budget computation.
func (m *compactionManager) SetModelOutputLimit(limit int64) {
	m.defaultModelOutputLimit = limit
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
			ContextWindow:    contextWindow,
			CutoffThreshold:  m.defaultCutoff,
			RepoMapTokens:    repoMapTokens,
			ModelOutputLimit: m.defaultModelOutputLimit,
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
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := m.queries.WithTx(tx)
	config, err := qtx.GetLcmSessionConfig(ctx, sessionID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("getting session config: %w", err)
		}

		budget := ComputeBudget(BudgetConfig{
			ContextWindow:    m.defaultContextWindow,
			CutoffThreshold:  m.defaultCutoff,
			RepoMapTokens:    tokens,
			ModelOutputLimit: m.defaultModelOutputLimit,
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
			return fmt.Errorf("committing transaction: %w", err)
		}
		m.repoMapTokens.Store(sessionID, tokens)
		m.budgetCache.Store(sessionID, budget)
		return nil
	}

	budget := ComputeBudget(BudgetConfig{
		ContextWindow:    config.ModelCtxMaxTokens,
		CutoffThreshold:  config.CtxCutoffThreshold,
		RepoMapTokens:    tokens,
		ModelOutputLimit: m.defaultModelOutputLimit,
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
		return fmt.Errorf("committing transaction: %w", err)
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
				ContextWindow:    m.defaultContextWindow,
				CutoffThreshold:  m.defaultCutoff,
				RepoMapTokens:    repoMapTokens,
				ModelOutputLimit: m.defaultModelOutputLimit,
			})
			return budget, nil
		}
		return Budget{}, fmt.Errorf("getting session config: %w", err)
	}

	budget := Budget{
		SoftThreshold: config.SoftThresholdTokens,
		HardLimit:     config.HardThresholdTokens,
		ContextWindow: config.ModelCtxMaxTokens,
	}
	m.budgetCache.Store(sessionID, budget)
	return budget, nil
}

// GetContextTokenCount returns the current token count for a session's context.
func (m *compactionManager) GetContextTokenCount(ctx context.Context, sessionID string) (int64, error) {
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
	return m.IsOverSoftThreshold(ctx, sessionID)
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

	m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
		Type:      CompactionStarted,
		SessionID: sessionID,
		Blocking:  true,
	})

	err := m.compactUntilUnderLimitLocked(ctx, sessionID)

	if err != nil {
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionFailed,
			SessionID: sessionID,
			Error:     err.Error(),
		})
	} else {
		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionCompleted,
			SessionID: sessionID,
			Success:   true,
		})
	}
	return err
}

// Compact runs compaction for a session with force=true and publishes events.
func (m *compactionManager) Compact(ctx context.Context, sessionID string) error {
	mu := m.sessionMutex(sessionID)
	mu.Lock()
	defer mu.Unlock()

	m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
		Type:      CompactionStarted,
		SessionID: sessionID,
	})

	result, err := m.CompactOnce(ctx, sessionID, true)
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

	return nil
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
		defer close(resultCh)

		mu := m.sessionMutex(sessionID)
		mu.Lock()
		defer mu.Unlock()

		m.broker.Publish(pubsub.CreatedEvent, CompactionEvent{
			Type:      CompactionStarted,
			SessionID: sessionID,
		})

		result, err := m.CompactOnce(detachedCtx, sessionID, false)
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

// ExtraAgentTools returns LCM-related agent tools for injection into the coder agent.
func (m *compactionManager) ExtraAgentTools() []fantasy.AgentTool {
	return []fantasy.AgentTool{
		tools.NewLcmGrepTool(m.sqlDB),
		tools.NewLcmDescribeTool(m.sqlDB),
		tools.NewLcmExpandTool(m.sqlDB),
		tools.NewLlmMapTool(),
		tools.NewAgenticMapTool(),
	}
}

// ExtraAgentTools is a nil-safe wrapper: returns nil when mgr is nil.
func ExtraAgentTools(mgr Manager) []fantasy.AgentTool {
	if mgr == nil {
		return nil
	}
	return mgr.ExtraAgentTools()
}
