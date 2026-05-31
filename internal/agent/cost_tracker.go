package agent

import (
	"sync"

	"github.com/charmbracelet/crush/internal/config"
)

const (
	// downgradeRatio triggers a downgrade suggestion for simple/medium tasks
	// when cumulative cost reaches this fraction of the budget.
	downgradeRatio = 0.8

	// forceLowTierRatio forces all tasks to the lowest-cost tier when
	// cumulative cost reaches this fraction of the budget.
	forceLowTierRatio = 0.95

	// DefaultCostBudget is the default session cost budget (in USD) used
	// when no explicit budget is configured. Zero means unlimited.
	DefaultCostBudget float64 = 0
)

// CostTracker tracks cumulative LLM spending within a session and provides
// cost-aware routing hints. All operations are O(1) and safe for concurrent
// use. The tracker is session-scoped and not persisted.
type CostTracker struct {
	mu     sync.Mutex
	budget float64
	used   float64

	perModel            map[string]float64
	lowestTierModelType config.SelectedModelType
}

// NewCostTracker creates a CostTracker with the given budget. A budget of 0
// means unlimited (no downgrade or force-low-tier will trigger).
func NewCostTracker(budget float64) *CostTracker {
	return &CostTracker{
		budget:              budget,
		perModel:            make(map[string]float64),
		lowestTierModelType: config.SelectedModelTypeSmall,
	}
}

// RecordCost records a cost event for the given model and updates cumulative
// totals. Token counts are informational; cost is the authoritative dollar
// amount.
func (t *CostTracker) RecordCost(model string, tokensIn, tokensOut int, cost float64) {
	if cost <= 0 {
		return
	}

	t.mu.Lock()
	t.used += cost
	t.perModel[model] += cost
	t.mu.Unlock()
}

// TotalCost returns the cumulative session cost in USD.
func (t *CostTracker) TotalCost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.used
}

// RemainingBudget returns the remaining budget in USD. Returns
// math.MaxFloat64 when the budget is unlimited (0).
func (t *CostTracker) RemainingBudget() float64 {
	if t.budget <= 0 {
		return float64(1<<63 - 1)
	}
	t.mu.Lock()
	rem := t.budget - t.used
	t.mu.Unlock()
	if rem < 0 {
		return 0
	}
	return rem
}

// ShouldDowngrade returns true when the session has consumed at least 80% of
// its budget. The caller should prefer a lower-tier model for simple/medium
// complexity tasks when this returns true.
func (t *CostTracker) ShouldDowngrade() bool {
	if t.budget <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.used >= t.budget*downgradeRatio
}

// ForceLowTier returns true when the session has consumed at least 95% of its
// budget. When true, all routing decisions should use the lowest-cost tier
// regardless of complexity.
func (t *CostTracker) ForceLowTier() bool {
	if t.budget <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.used >= t.budget*forceLowTierRatio
}

// CostForModel returns the cumulative cost attributed to a specific model.
func (t *CostTracker) CostForModel(model string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.perModel[model]
}

// SetLowestTierModelType configures which model type is considered the
// cheapest. This is used by ResolveWithCost to determine the fallback when
// ForceLowTier is true. Defaults to SelectedModelTypeSmall.
func (t *CostTracker) SetLowestTierModelType(mt config.SelectedModelType) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lowestTierModelType = mt
}

// Budget returns the configured session cost budget.
func (t *CostTracker) Budget() float64 {
	return t.budget
}

// ResolveWithCost wraps a TierRouter routing decision with cost-aware
// adjustments. When ForceLowTier is true it returns the lowest-tier model
// type. When ShouldDowngrade is true and the base result is not already the
// lowest tier, it downgrades to the lowest tier. Otherwise it returns the
// base result unchanged.
func (t *CostTracker) ResolveWithCost(
	base config.SelectedModelType,
	lowest config.SelectedModelType,
) config.SelectedModelType {
	if t.ForceLowTier() {
		return lowest
	}
	if t.ShouldDowngrade() && base != lowest {
		return lowest
	}
	return base
}
