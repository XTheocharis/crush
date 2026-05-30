package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"charm.land/fantasy"
)

// ProductiveLoopResult extends DoomLoopResult with a productivity assessment.
// When IsProductive is true, the repeated tool calls are producing different
// outputs — the agent is making progress even though it appears to be looping.
type ProductiveLoopResult struct {
	DoomLoopResult
	IsProductive    bool
	UniqueOutputCnt int
}

// outputHashGroup tracks output hashes for a single tool-name group.
type outputHashGroup struct {
	tool    string
	hashes  map[string]int
	count   int
	pattern string
}

// ProductiveLoopDetector extends DoomLoopDetector with productive-loop
// awareness. It compares output hashes across repeated tool calls: if the
// same tool produces different outputs each time, the loop is productive and
// should not be escalated (though it is still tracked).
type ProductiveLoopDetector struct {
	*DoomLoopDetector
}

// NewProductiveLoopDetector wraps an existing DoomLoopDetector.
func NewProductiveLoopDetector(d *DoomLoopDetector) *ProductiveLoopDetector {
	return &ProductiveLoopDetector{DoomLoopDetector: d}
}

// Detect analyzes recent steps and returns a ProductiveLoopResult. It runs
// two passes: first the underlying DoomLoopDetector to classify the loop, then
// an independent output-diversity scan. If any tool appears frequently enough
// to meet the soft threshold AND produces diverse outputs (>=50% unique hashes),
// the loop is considered productive: hard escalation is downgraded to medium
// (execution continues) while soft/medium warnings still reach the LLM.
func (p *ProductiveLoopDetector) Detect(steps []fantasy.StepResult) ProductiveLoopResult {
	doomResult := p.DoomLoopDetector.Detect(steps)

	if len(steps) < p.WindowSize {
		return ProductiveLoopResult{DoomLoopResult: doomResult}
	}

	window := steps[len(steps)-p.WindowSize:]
	groups := p.groupByToolOutputHash(window)

	for _, g := range groups {
		if g.count < p.Thresholds.Soft {
			continue
		}
		uniqueOutputs := len(g.hashes)
		if uniqueOutputs > 1 && float64(uniqueOutputs)/float64(g.count) >= 0.5 {
			level, msg := p.classify(g.count, g.tool)
			if level == EscalationHard {
				level = EscalationMedium
				msg = fmt.Sprintf(
					"PRODUCTIVE LOOP: Tool %q repeated %d times (%d unique outputs). Progress detected but consider consolidating your approach.",
					g.tool, g.count, uniqueOutputs,
				)
			}
			return ProductiveLoopResult{
				DoomLoopResult: DoomLoopResult{
					Level:       level,
					RepeatCount: g.count,
					ToolName:    g.tool,
					Message:     msg,
					Pattern:     g.pattern,
				},
				IsProductive:    true,
				UniqueOutputCnt: uniqueOutputs,
			}
		}
	}

	return ProductiveLoopResult{DoomLoopResult: doomResult}
}

// groupByToolOutputHash groups steps by tool name and tracks unique output
// hashes within each group.
func (p *ProductiveLoopDetector) groupByToolOutputHash(window []fantasy.StepResult) []outputHashGroup {
	groupMap := make(map[string]*outputHashGroup)

	for _, step := range window {
		calls := step.Content.ToolCalls()
		if len(calls) == 0 {
			continue
		}

		resultsByID := make(map[string]fantasy.ToolResultContent)
		for _, tr := range step.Content.ToolResults() {
			resultsByID[tr.ToolCallID] = tr
		}

		for _, tc := range calls {
			output := ""
			if tr, ok := resultsByID[tc.ToolCallID]; ok {
				output = toolResultOutputString(tr.Result)
			}
			hash := hashOutput(tc.ToolName, output)

			g, ok := groupMap[tc.ToolName]
			if !ok {
				g = &outputHashGroup{
					tool:   tc.ToolName,
					hashes: make(map[string]int),
				}
				groupMap[tc.ToolName] = g
			}
			g.hashes[hash]++
			g.count++
			if g.pattern == "" {
				sig := getToolInteractionSignature(step.Content)
				if sig != "" {
					g.pattern = sig[:min(16, len(sig))]
				}
			}
		}
	}

	groups := make([]outputHashGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].tool < groups[j].tool
	})
	return groups
}

// hashOutput computes a SHA-256 hash of a tool name + output pair for
// deduplication.
func hashOutput(toolName, output string) string {
	h := sha256.New()
	io.WriteString(h, toolName)
	io.WriteString(h, "\x00")
	io.WriteString(h, output)
	return hex.EncodeToString(h.Sum(nil))
}

// ---------------------------------------------------------------------------
// Productive orchestration pattern
// ---------------------------------------------------------------------------

const (
	productiveCachePrefix   = "productive"
	defaultMaxIterations    = 10
	defaultStallThreshold   = 2 // consecutive identical fingerprints → stall
)

// ProductiveConfig configures a Productive orchestration loop.
type ProductiveConfig struct {
	MaxIterations   int
	StallThreshold  int           // consecutive identical fingerprints → stall
	CacheTTL        time.Duration
	SubagentTools   []string
	SubagentSteps   int
	SubagentTimeout time.Duration
}

func (c ProductiveConfig) maxIterations() int {
	if c.MaxIterations <= 0 {
		return defaultMaxIterations
	}
	return c.MaxIterations
}

func (c ProductiveConfig) stallThreshold() int {
	if c.StallThreshold <= 0 {
		return defaultStallThreshold
	}
	return c.StallThreshold
}

func (c ProductiveConfig) cacheTTL() time.Duration {
	if c.CacheTTL <= 0 {
		return DefaultRepoMapTTL
	}
	return c.CacheTTL
}

// ProductiveResult holds the outcome of a productive loop run.
type ProductiveResult struct {
	Success    bool
	Result     string
	Iterations int
	Stalled    bool // true if loop exited due to progress stall
	Error      string
}

// Productive runs a constrained iterative agent loop that maximizes output
// quality under resource limits. Each iteration spawns a subagent with the
// task and accumulated context. The loop tracks content-addressed
// fingerprints of intermediate outputs to detect progress stalls and uses
// ProductiveLoopDetector for doom-loop awareness.
//
// It is designed for tasks that benefit from iterative refinement rather
// than decomposition: code review passes, incremental refactoring, or
// exploratory research where each step builds on the previous one.
type Productive struct {
	cfg      ProductiveConfig
	cache    *SharedCache
	factory  StructuredSubagentFactory
	detector *ProductiveLoopDetector
}

// NewProductive creates a Productive orchestration pattern. The detector may
// be nil (a default is created). The cache and factory must be non-nil.
func NewProductive(cfg ProductiveConfig, cache *SharedCache, factory StructuredSubagentFactory, detector *ProductiveLoopDetector) *Productive {
	p := &Productive{
		cfg:      cfg,
		cache:    cache,
		factory:  factory,
		detector: detector,
	}
	if p.detector == nil {
		p.detector = NewProductiveLoopDetector(
			NewDoomLoopDetector(DefaultDoomLoopThresholds, loopDetectionWindowSize),
		)
	}
	return p
}

// Run executes the productive loop. It iteratively runs a subagent on the
// task, accumulating results until: the subagent reports success, the
// maximum iteration count is reached, progress stalls (same output
// fingerprint repeated), or a doom loop is detected.
func (p *Productive) Run(ctx context.Context, parentSessionID, task string) ProductiveResult {
	if p.factory == nil {
		return ProductiveResult{Error: "productive: no structured subagent factory"}
	}
	if task == "" {
		return ProductiveResult{Error: "productive: empty task"}
	}
	if err := ctx.Err(); err != nil {
		return ProductiveResult{Error: fmt.Sprintf("productive: context cancelled: %v", err)}
	}

	cacheKey := CacheKey(productiveCachePrefix, parentSessionID, task)
	if cached, ok := p.cache.Get(cacheKey); ok {
		if r, ok := cached.(ProductiveResult); ok {
			return r
		}
	}

	var (
		accumulated     string
		lastFingerprint string
		stallCount      int
	)

	for i := range p.cfg.maxIterations() {
		if err := ctx.Err(); err != nil {
			return ProductiveResult{
				Result:     accumulated,
				Iterations: i,
				Error:      fmt.Sprintf("productive: context cancelled at iteration %d: %v", i, err),
			}
		}

		subagent, err := p.factory.NewStructuredSubagent(ctx, parentSessionID)
		if err != nil {
			return ProductiveResult{
				Result:     accumulated,
				Iterations: i,
				Error:      fmt.Sprintf("productive: create subagent at iteration %d: %v", i, err),
			}
		}

		prompt := task
		if accumulated != "" {
			prompt = fmt.Sprintf("%s\n\nPrevious progress:\n%s\n\nContinue from where you left off.", task, accumulated)
		}

		resp, err := subagent.Execute(ctx, StructuredRequest{
			Task:     prompt,
			Tools:    p.cfg.SubagentTools,
			MaxSteps: p.cfg.SubagentSteps,
			Timeout:  p.cfg.SubagentTimeout,
			Context: map[string]string{
				"iteration":    fmt.Sprintf("%d/%d", i+1, p.cfg.maxIterations()),
				"accumulated":  accumulated,
			},
		})
		if err != nil {
			return ProductiveResult{
				Result:     accumulated,
				Iterations: i + 1,
				Error:      fmt.Sprintf("productive: execute iteration %d: %v", i, err),
			}
		}

			fp := fingerprintOutput(resp.Result)
		if fp == lastFingerprint && fp != "" {
			stallCount++
			if stallCount >= p.cfg.stallThreshold() {
				slog.Debug("Productive: stall detected",
					"iteration", i+1,
					"stall_count", stallCount,
				)
				result := ProductiveResult{
					Success:    resp.Success,
					Result:     accumulated,
					Iterations: i + 1,
					Stalled:    true,
				}
				p.cache.Set(cacheKey, result, p.cfg.cacheTTL())
				return result
			}
		} else {
			stallCount = 0
		}
		lastFingerprint = fp

		if accumulated == "" {
			accumulated = resp.Result
		} else if resp.Result != "" {
			accumulated = accumulated + "\n" + resp.Result
		}

		if !resp.Success && i > 0 {
			slog.Debug("Productive: subagent failed, continuing",
				"iteration", i+1,
			)
		}

		if resp.Success {
			result := ProductiveResult{
				Success:    true,
				Result:     accumulated,
				Iterations: i + 1,
			}
			p.cache.Set(cacheKey, result, p.cfg.cacheTTL())
			return result
		}
	}

	result := ProductiveResult{
		Success:    false,
		Result:     accumulated,
		Iterations: p.cfg.maxIterations(),
		Error:      fmt.Sprintf("productive: reached max iterations (%d) without completion", p.cfg.maxIterations()),
	}
	p.cache.Set(cacheKey, result, p.cfg.cacheTTL())
	return result
}

// fingerprintOutput computes a SHA-256 fingerprint of a string for stall
// detection. Empty strings produce a distinct fingerprint so they are not
// confused with each other.
func fingerprintOutput(s string) string {
	h := sha256.New()
	io.WriteString(h, s)
	return hex.EncodeToString(h.Sum(nil))
}
