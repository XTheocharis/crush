package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	softLimitWarningRatio = 0.8
	// DefaultCharsPerToken is the default character-to-token estimation ratio
	// used when no LCMDataProvider is configured.
	DefaultCharsPerToken = 4
)

// LCMDataProvider provides LCM data for resource limit tracking.
// The concrete implementation is provided by the LCM extension (Wave 4).
type LCMDataProvider interface {
	CharsPerToken() int
}

// ResourceLimit defines the soft and hard limits for a single resource
// dimension. Soft limit triggers a warning; hard limit terminates the
// subagent.
type ResourceLimit struct {
	Soft int
	Hard int
}

// Exceeded returns true when usage has reached or surpassed the hard limit.
func (r ResourceLimit) Exceeded(usage int) bool {
	return r.Hard > 0 && usage >= r.Hard
}

// Approaching returns true when usage has reached or surpassed the soft limit
// (which defaults to 80% of the hard limit when Soft is zero).
func (r ResourceLimit) Approaching(usage int) bool {
	soft := r.softThreshold()
	return soft > 0 && usage >= soft
}

func (r ResourceLimit) softThreshold() int {
	if r.Soft > 0 {
		return r.Soft
	}
	if r.Hard > 0 {
		return int(float64(r.Hard) * softLimitWarningRatio)
	}
	return 0
}

// SubagentLimits holds the resource limits for a specific subagent type.
// Zero-valued fields mean no limit for that dimension.
type SubagentLimits struct {
	MaxTokens    ResourceLimit
	MaxSteps     ResourceLimit
	MaxDuration  time.Duration
	SoftDuration time.Duration
}

// DurationExceeded returns true when the elapsed time has reached the hard
// duration limit.
func (l SubagentLimits) DurationExceeded(elapsed time.Duration) bool {
	return l.MaxDuration > 0 && elapsed >= l.MaxDuration
}

// DurationApproaching returns true when the elapsed time has reached the soft
// duration threshold (defaults to 80% of MaxDuration).
func (l SubagentLimits) DurationApproaching(elapsed time.Duration) bool {
	soft := l.softDurationThreshold()
	return soft > 0 && elapsed >= soft
}

func (l SubagentLimits) softDurationThreshold() time.Duration {
	if l.SoftDuration > 0 {
		return l.SoftDuration
	}
	if l.MaxDuration > 0 {
		return time.Duration(float64(l.MaxDuration) * softLimitWarningRatio)
	}
	return 0
}

// LimitsProfile maps subagent types to their resource limits.
type LimitsProfile map[string]SubagentLimits

// DefaultLimitsProfile returns a sensible default profile with limits for
// common subagent types.
func DefaultLimitsProfile() LimitsProfile {
	return LimitsProfile{
		"task": {
			MaxTokens:   ResourceLimit{Soft: 32000, Hard: 40000},
			MaxSteps:    ResourceLimit{Soft: 40, Hard: 50},
			MaxDuration: 5 * time.Minute,
		},
		"structured": {
			MaxTokens:   ResourceLimit{Soft: 16000, Hard: 20000},
			MaxSteps:    ResourceLimit{Soft: 20, Hard: 25},
			MaxDuration: 2 * time.Minute,
		},
	}
}

// Get returns the limits for a given subagent type, falling back to the
// "task" profile if the type is not found.
func (p LimitsProfile) Get(subagentType string) SubagentLimits {
	if limits, ok := p[subagentType]; ok {
		return limits
	}
	if limits, ok := p["task"]; ok {
		return limits
	}
	return SubagentLimits{}
}

// ResourceUsage tracks real-time resource consumption for a single subagent.
type ResourceUsage struct {
	TokensUsed  atomic.Int64
	StepsTaken  atomic.Int32
	StartTime   time.Time
	mu          sync.Mutex
	warnedToken bool
	warnedStep  bool
	warnedDur   bool
	charsPerTok int
}

func NewResourceUsage() *ResourceUsage {
	return &ResourceUsage{
		StartTime:   time.Now(),
		charsPerTok: DefaultCharsPerToken,
	}
}

// NewResourceUsageWithProvider creates a ResourceUsage that uses the given
// LCMDataProvider for character-to-token estimation.
func NewResourceUsageWithProvider(p LCMDataProvider) *ResourceUsage {
	cpt := DefaultCharsPerToken
	if p != nil {
		cpt = p.CharsPerToken()
	}
	if cpt <= 0 {
		cpt = DefaultCharsPerToken
	}
	return &ResourceUsage{
		StartTime:   time.Now(),
		charsPerTok: cpt,
	}
}

// AddTokens increments the token counter by the estimated token count for
// the given text (using CharsPerToken ceiling division).
func (u *ResourceUsage) AddTokens(text string) {
	chars := len(text)
	tokens := int64((chars + u.charsPerTok - 1) / u.charsPerTok)
	u.TokensUsed.Add(tokens)
}

// AddStep increments the step counter by one.
func (u *ResourceUsage) AddStep() {
	u.StepsTaken.Add(1)
}

// Elapsed returns the duration since tracking started.
func (u *ResourceUsage) Elapsed() time.Duration {
	return time.Since(u.StartTime)
}

// Snapshot returns a point-in-time copy of the resource usage.
func (u *ResourceUsage) Snapshot() UsageSnapshot {
	return UsageSnapshot{
		TokensUsed: u.TokensUsed.Load(),
		StepsTaken: u.StepsTaken.Load(),
		Elapsed:    u.Elapsed(),
	}
}

// WarnTokensOnce logs a soft-limit warning for token usage once.
func (u *ResourceUsage) WarnTokensOnce(limit ResourceLimit) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.warnedToken {
		return
	}
	usage := u.TokensUsed.Load()
	if limit.Approaching(int(usage)) && !limit.Exceeded(int(usage)) {
		slog.Warn("Subagent approaching token limit",
			"tokens_used", usage,
			"soft_limit", limit.softThreshold(),
			"hard_limit", limit.Hard,
		)
		u.warnedToken = true
	}
}

// WarnStepsOnce logs a soft-limit warning for step usage once.
func (u *ResourceUsage) WarnStepsOnce(limit ResourceLimit) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.warnedStep {
		return
	}
	usage := u.StepsTaken.Load()
	if limit.Approaching(int(usage)) && !limit.Exceeded(int(usage)) {
		slog.Warn("Subagent approaching step limit",
			"steps_taken", usage,
			"soft_limit", limit.softThreshold(),
			"hard_limit", limit.Hard,
		)
		u.warnedStep = true
	}
}

// WarnDurationOnce logs a soft-limit warning for duration once.
func (u *ResourceUsage) WarnDurationOnce(limits SubagentLimits) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.warnedDur {
		return
	}
	elapsed := u.Elapsed()
	if limits.DurationApproaching(elapsed) && !limits.DurationExceeded(elapsed) {
		slog.Warn("Subagent approaching duration limit",
			"elapsed", elapsed,
			"soft_duration", limits.softDurationThreshold(),
			"hard_duration", limits.MaxDuration,
		)
		u.warnedDur = true
	}
}

// UsageSnapshot is a point-in-time copy of resource usage.
type UsageSnapshot struct {
	TokensUsed int64
	StepsTaken int32
	Elapsed    time.Duration
}

// ResourceLimitResult describes why a subagent was terminated.
type ResourceLimitResult struct {
	HardLimit    string
	Usage        UsageSnapshot
	PartialValue any
}

// Error implements the error interface.
func (r ResourceLimitResult) Error() string {
	return fmt.Sprintf("subagent terminated: %s exceeded (tokens=%d, steps=%d, elapsed=%s)",
		r.HardLimit, r.Usage.TokensUsed, r.Usage.StepsTaken, r.Usage.Elapsed,
	)
}

// ResourceLimitedTask wraps a Task with resource limit enforcement. It tracks
// usage via a ResourceUsage instance and checks limits after each token
// addition or step. On hard limit breach, the task's context is cancelled.
type ResourceLimitedTask struct {
	usage  *ResourceUsage
	limits SubagentLimits
	cancel context.CancelFunc
}

// NewResourceLimitedTask creates a wrapper that enforces the given limits. The
// returned context is derived from parent and will be cancelled on hard limit
// breach.
func NewResourceLimitedTask(parent context.Context, limits SubagentLimits) (*ResourceLimitedTask, context.Context) {
	ctx, cancel := context.WithCancel(parent)
	return &ResourceLimitedTask{
		usage:  NewResourceUsage(),
		limits: limits,
		cancel: cancel,
	}, ctx
}

// Usage returns the underlying ResourceUsage tracker.
func (t *ResourceLimitedTask) Usage() *ResourceUsage {
	return t.usage
}

// Check evaluates all resource limits and returns a ResourceLimitResult if a
// hard limit has been breached. On soft-limit breach, it logs a warning.
func (t *ResourceLimitedTask) Check() *ResourceLimitResult {
	snapshot := t.usage.Snapshot()

	if t.limits.MaxTokens.Exceeded(int(snapshot.TokensUsed)) {
		t.cancel()
		return &ResourceLimitResult{
			HardLimit: "tokens",
			Usage:     snapshot,
		}
	}
	t.usage.WarnTokensOnce(t.limits.MaxTokens)

	if t.limits.MaxSteps.Exceeded(int(snapshot.StepsTaken)) {
		t.cancel()
		return &ResourceLimitResult{
			HardLimit: "steps",
			Usage:     snapshot,
		}
	}
	t.usage.WarnStepsOnce(t.limits.MaxSteps)

	if t.limits.DurationExceeded(snapshot.Elapsed) {
		t.cancel()
		return &ResourceLimitResult{
			HardLimit: "duration",
			Usage:     snapshot,
		}
	}
	t.usage.WarnDurationOnce(t.limits)

	return nil
}

// Cancel releases the context resources.
func (t *ResourceLimitedTask) Cancel() {
	t.cancel()
}

// TrackStep increments the step counter and checks limits.
func (t *ResourceLimitedTask) TrackStep() *ResourceLimitResult {
	t.usage.AddStep()
	return t.Check()
}

// TrackTokens estimates tokens from text, increments the counter, and checks
// limits.
func (t *ResourceLimitedTask) TrackTokens(text string) *ResourceLimitResult {
	t.usage.AddTokens(text)
	return t.Check()
}
