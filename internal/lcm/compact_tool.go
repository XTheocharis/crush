package lcm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"

	"github.com/charmbracelet/crush/internal/agent/tools/types"
)

// compactToolParams holds the optional parameters for the lcm_compact tool.
type compactToolParams struct {
	Pressure     string `json:"pressure"      description:"Compaction pressure level: low, medium, or high (default: auto-detected)"`
	TargetTokens int    `json:"target_tokens" description:"Target token count to compact down to (0 = use budget threshold)"`
}

const compactToolDescription = `Manually trigger LCM context compaction for the current session.

Compaction summarizes and condenses conversation history to free up token budget.
Use this when you notice the context is getting large or before starting a new
complex task.

Parameters:
- pressure: Optional compaction intensity. "low" for minimal compaction (micro-compaction only),
  "medium" for session-level compaction, "high" for full compaction with LLM summarization.
  When omitted, the system auto-detects pressure based on current token usage.
- target_tokens: Optional target token count. When set, compaction continues until
  the context is under this limit. When 0 (default), uses the session's soft threshold.`

// newCompactTool creates the lcm_compact agent tool.
func newCompactTool(mgr Manager) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"lcm_compact",
		compactToolDescription,
		func(ctx context.Context, params compactToolParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if mgr == nil {
				return fantasy.NewTextErrorResponse("LCM manager is not available"), nil
			}

			sessionID := types.SessionIDFromContext(ctx)
			if sessionID == "" {
				return fantasy.NewTextErrorResponse("Session ID not found in context"), nil
			}

			if params.Pressure != "" {
				p := strings.ToLower(params.Pressure)
				if p != "low" && p != "medium" && p != "high" {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("Invalid pressure %q: must be low, medium, or high", params.Pressure)), nil
				}
			}

			tokenCountBefore, _ := mgr.GetContextTokenCount(ctx, sessionID)
			budget, budgetErr := mgr.GetBudget(ctx, sessionID)

			if err := mgr.Compact(ctx, sessionID); err != nil {
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("Compaction failed: %v", err)), nil
			}

			tokenCountAfter, _ := mgr.GetContextTokenCount(ctx, sessionID)

			var b strings.Builder
			fmt.Fprintf(&b, "Compaction completed.\n")
			fmt.Fprintf(&b, "  Tokens before: %d\n", tokenCountBefore)
			fmt.Fprintf(&b, "  Tokens after:  %d\n", tokenCountAfter)

			if tokenCountBefore > 0 && tokenCountAfter > 0 && tokenCountAfter < tokenCountBefore {
				saved := tokenCountBefore - tokenCountAfter
				pct := float64(saved) / float64(tokenCountBefore) * 100
				fmt.Fprintf(&b, "  Tokens saved:  %d (%.1f%%)\n", saved, pct)
			}

			if budgetErr == nil {
				fmt.Fprintf(&b, "  Soft threshold: %d\n", budget.SoftThreshold)
				fmt.Fprintf(&b, "  Hard limit:     %d\n", budget.HardLimit)
				if tokenCountAfter > budget.SoftThreshold {
					fmt.Fprintf(&b, "  Status: still over soft threshold\n")
				} else {
					fmt.Fprintf(&b, "  Status: within budget\n")
				}
			}

			if params.TargetTokens > 0 && tokenCountAfter > int64(params.TargetTokens) {
				fmt.Fprintf(&b, "  Note: still above target of %d tokens\n", params.TargetTokens)
			}

			return fantasy.NewTextResponse(b.String()), nil
		})
}
