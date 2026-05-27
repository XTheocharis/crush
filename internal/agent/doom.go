package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"

	"charm.land/fantasy"
)

// EscalationLevel represents the severity of a detected doom loop.
type EscalationLevel int

const (
	// EscalationNone means no loop detected.
	EscalationNone EscalationLevel = iota
	// EscalationSoft means a likely loop — the agent should be warned.
	EscalationSoft
	// EscalationMedium means a confirmed loop — the agent must change approach.
	EscalationMedium
	// EscalationHard means a severe loop — execution should halt.
	EscalationHard
)

func (l EscalationLevel) String() string {
	switch l {
	case EscalationNone:
		return "none"
	case EscalationSoft:
		return "soft"
	case EscalationMedium:
		return "medium"
	case EscalationHard:
		return "hard"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// DoomLoopThresholds configures when each escalation level triggers. Counts
// are the minimum number of semantically similar calls within the window that
// trigger the given level.
type DoomLoopThresholds struct {
	Soft   int
	Medium int
	Hard   int
}

// DefaultDoomLoopThresholds provides sensible defaults.
var DefaultDoomLoopThresholds = DoomLoopThresholds{
	Soft:   3,
	Medium: 5,
	Hard:   7,
}

// DoomLoopResult is returned by the detector after analyzing recent steps.
type DoomLoopResult struct {
	Level       EscalationLevel
	RepeatCount int
	ToolName    string
	Message     string
	Pattern     string
}

// DoomLoopDetector extends the basic loop detection with pattern-aware doom
// loop detection and escalation. It wraps the existing SHA-256 signature
// approach and adds semantic similarity heuristics.
type DoomLoopDetector struct {
	Thresholds   DoomLoopThresholds
	WindowSize   int
	SimilarityFn func(a, b fantasy.StepResult) bool
}

// NewDoomLoopDetector creates a detector with the given thresholds and window
// size. If thresholds is zero-valued, DefaultDoomLoopThresholds is used. If
// windowSize is 0, loopDetectionWindowSize is used.
func NewDoomLoopDetector(thresholds DoomLoopThresholds, windowSize int) *DoomLoopDetector {
	d := &DoomLoopDetector{
		Thresholds: thresholds,
		WindowSize: windowSize,
	}
	if d.Thresholds == (DoomLoopThresholds{}) {
		d.Thresholds = DefaultDoomLoopThresholds
	}
	if d.WindowSize == 0 {
		d.WindowSize = loopDetectionWindowSize
	}
	d.SimilarityFn = SemanticSimilar
	return d
}

// Detect analyzes recent steps and returns a DoomLoopResult. It first checks
// exact signature matches (via the existing SHA-256 approach), then falls back
// to semantic similarity detection.
func (d *DoomLoopDetector) Detect(steps []fantasy.StepResult) DoomLoopResult {
	if len(steps) < d.WindowSize {
		return DoomLoopResult{Level: EscalationNone}
	}

	window := steps[len(steps)-d.WindowSize:]

	exactResult := d.detectExactPattern(window)
	if exactResult.Level != EscalationNone {
		return exactResult
	}

	return d.detectSemanticPattern(window)
}

type patternInfo struct {
	count   int
	tool    string
	pattern string
}

func (d *DoomLoopDetector) detectExactPattern(window []fantasy.StepResult) DoomLoopResult {
	counts := make(map[string]*patternInfo)

	for _, step := range window {
		sig := getToolInteractionSignature(step.Content)
		if sig == "" {
			continue
		}
		normalized := normalizeSignature(sig)
		info, ok := counts[normalized]
		if !ok {
			info = &patternInfo{
				tool:    firstToolName(step.Content),
				pattern: normalized[:min(16, len(normalized))],
			}
			counts[normalized] = info
		}
		info.count++
	}

	return d.escalationFromCounts(counts)
}

func (d *DoomLoopDetector) detectSemanticPattern(window []fantasy.StepResult) DoomLoopResult {
	var groups []patternInfo
	assigned := make([]bool, len(window))

	for i, step := range window {
		if assigned[i] {
			continue
		}
		sig := getToolInteractionSignature(step.Content)
		if sig == "" {
			continue
		}

		g := patternInfo{
			tool:    firstToolName(step.Content),
			pattern: sig[:min(16, len(sig))],
			count:   1,
		}
		assigned[i] = true

		for j := i + 1; j < len(window); j++ {
			if assigned[j] {
				continue
			}
			if d.SimilarityFn(step, window[j]) {
				g.count++
				assigned[j] = true
			}
		}
		groups = append(groups, g)
	}

	highest := DoomLoopResult{Level: EscalationNone}
	for _, g := range groups {
		level, msg := d.classify(g.count, g.tool)
		if level > highest.Level {
			highest = DoomLoopResult{
				Level:       level,
				RepeatCount: g.count,
				ToolName:    g.tool,
				Message:     msg,
				Pattern:     g.pattern,
			}
		}
	}
	return highest
}

func (d *DoomLoopDetector) escalationFromCounts(counts map[string]*patternInfo) DoomLoopResult {
	highest := DoomLoopResult{Level: EscalationNone}
	for _, info := range counts {
		level, msg := d.classify(info.count, info.tool)
		if level > highest.Level {
			highest = DoomLoopResult{
				Level:       level,
				RepeatCount: info.count,
				ToolName:    info.tool,
				Message:     msg,
				Pattern:     info.pattern,
			}
		}
	}
	return highest
}

func (d *DoomLoopDetector) classify(count int, tool string) (EscalationLevel, string) {
	switch {
	case count >= d.Thresholds.Hard:
		return EscalationHard, fmt.Sprintf(
			"HARD LOOP: Tool %q repeated %d times. Halting — user intervention required.",
			tool, count,
		)
	case count >= d.Thresholds.Medium:
		return EscalationMedium, fmt.Sprintf(
			"MEDIUM LOOP: Tool %q repeated %d times. You must change your approach — try a different tool or strategy.",
			tool, count,
		)
	case count >= d.Thresholds.Soft:
		return EscalationSoft, fmt.Sprintf(
			"SOFT LOOP: Tool %q repeated %d times. Consider whether a different approach would be more effective.",
			tool, count,
		)
	default:
		return EscalationNone, ""
	}
}

// SemanticSimilar returns true if two steps are semantically similar: same
// tool name, similar arguments (prefix overlap ≥ 80%), and same error output
// (or both no error).
func SemanticSimilar(a, b fantasy.StepResult) bool {
	aCalls := a.Content.ToolCalls()
	bCalls := b.Content.ToolCalls()
	if len(aCalls) == 0 || len(bCalls) == 0 {
		return false
	}
	if len(aCalls) != len(bCalls) {
		return false
	}

	aResults := resultsByID(a.Content)
	bResults := resultsByID(b.Content)

	for i := range aCalls {
		if aCalls[i].ToolName != bCalls[i].ToolName {
			return false
		}
		if !argsSimilar(aCalls[i].Input, bCalls[i].Input) {
			return false
		}

		aOut := outputString(aResults, aCalls[i].ToolCallID)
		bOut := outputString(bResults, bCalls[i].ToolCallID)
		if aOut != bOut {
			return false
		}
	}
	return true
}

func resultsByID(content fantasy.ResponseContent) map[string]fantasy.ToolResultContent {
	m := make(map[string]fantasy.ToolResultContent)
	for _, tr := range content.ToolResults() {
		m[tr.ToolCallID] = tr
	}
	return m
}

func outputString(results map[string]fantasy.ToolResultContent, callID string) string {
	tr, ok := results[callID]
	if !ok {
		return ""
	}
	return toolResultOutputString(tr.Result)
}

// argsSimilar checks whether two tool argument strings are ≥80% similar using
// a character-level overlap ratio (Levenshtein-inspired distance heuristic).
func argsSimilar(a, b string) bool {
	if a == b {
		return true
	}
	la, lb := len(a), len(b)
	if la == 0 || lb == 0 {
		return la == lb
	}

	threshold := 0.8
	maxEdits := float64(max(la, lb)) * (1.0 - threshold)
	edits := editDistance(a, b)
	return float64(edits) <= maxEdits
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := range lb + 1 {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// normalizeSignature normalizes a tool interaction string for more robust
// pattern matching. It collapses whitespace, removes absolute paths (keeping
// basenames), and replaces numeric IDs with placeholders.
func normalizeSignature(sig string) string {
	// Collapse whitespace.
	sig = whitespaceRe.ReplaceAllString(sig, " ")
	sig = strings.TrimSpace(sig)

	// Replace absolute paths with basenames.
	sig = pathRe.ReplaceAllString(sig, "$1")

	// Replace numeric IDs (sequences of 3+ digits).
	sig = numIDRe.ReplaceAllString(sig, "<ID>")

	return sig
}

// NormalizeToolCall normalizes a tool call for pattern comparison. It
// concatenates the tool name and arguments, then applies path collapsing,
// whitespace normalization, and numeric ID replacement.
func NormalizeToolCall(toolName string, args string) string {
	return normalizeSignature(toolName + " " + args)
}

var (
	whitespaceRe = regexp.MustCompile(`\s+`)
	pathRe       = regexp.MustCompile(`(?:/[\w.-]+)+/([\w.-]+)`)
	numIDRe      = regexp.MustCompile(`\d{3,}`)
)

// extractToolSignature computes a lightweight signature from a step for
// grouping purposes. It uses the same SHA-256 approach as
// getToolInteractionSignature but only hashes tool name and output (not input),
// making it suitable for semantic grouping.
func extractToolSignature(content fantasy.ResponseContent) string {
	toolCalls := content.ToolCalls()
	if len(toolCalls) == 0 {
		return ""
	}

	resultsByIDMap := make(map[string]fantasy.ToolResultContent)
	for _, tr := range content.ToolResults() {
		resultsByIDMap[tr.ToolCallID] = tr
	}

	h := sha256.New()
	for _, tc := range toolCalls {
		io.WriteString(h, tc.ToolName)
		io.WriteString(h, "\x00")
		output := ""
		if tr, ok := resultsByIDMap[tc.ToolCallID]; ok {
			output = toolResultOutputString(tr.Result)
		}
		io.WriteString(h, output)
		io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func firstToolName(content fantasy.ResponseContent) string {
	calls := content.ToolCalls()
	if len(calls) == 0 {
		return ""
	}
	return calls[0].ToolName
}
