package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

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
