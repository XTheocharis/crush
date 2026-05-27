package lcm

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/charmbracelet/crush/internal/hooks"
)

// CompactHookInput is passed to PreCompact hooks as tool_input.
type CompactHookInput struct {
	SessionID     string `json:"session_id"`
	TokenCount    int64  `json:"token_count"`
	SoftThreshold int64  `json:"soft_threshold"`
	HardLimit     int64  `json:"hard_limit"`
	OverSoft      bool   `json:"over_soft"`
	OverHard      bool   `json:"over_hard"`
	Blocking      bool   `json:"blocking"`
}

// CompactHookOutput is passed to PostCompact hooks as tool_input.
type CompactHookOutput struct {
	SessionID       string `json:"session_id"`
	Success         bool   `json:"success"`
	Rounds          int    `json:"rounds"`
	TokenCountAfter int64  `json:"token_count_after"`
	Blocking        bool   `json:"blocking"`
	DurationMs      int64  `json:"duration_ms"`
}

// CompactHookDecision is the outcome of running PreCompact hooks.
type CompactHookDecision struct {
	Skip      bool // Hook denied compaction.
	ForceFull bool // Hook requested skipping layers, going straight to LLM summarization.
	Reason    string
}

const lcmCompactTool = "lcm_compact"

func runPreCompactHooks(
	ctx context.Context,
	runner *hooks.Runner,
	sessionID string,
	input CompactHookInput,
) CompactHookDecision {
	if runner == nil {
		return CompactHookDecision{}
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		slog.Warn("LCM hooks: failed to marshal PreCompact input", "error", err)
		return CompactHookDecision{}
	}

	result, err := runner.Run(ctx, hooks.EventPreCompact, sessionID, lcmCompactTool, string(inputJSON))
	if err != nil {
		slog.Warn("LCM hooks: PreCompact hook execution error", "error", err)
		return CompactHookDecision{}
	}

	decision := CompactHookDecision{
		Reason: result.Reason,
	}

	if result.Decision == hooks.DecisionDeny || result.Halt {
		decision.Skip = true
		return decision
	}

	// Hook requested force-full via updated_input patch.
	if result.UpdatedInput != "" {
		var patch struct {
			ForceFull bool `json:"force_full"`
		}
		if err := json.Unmarshal([]byte(result.UpdatedInput), &patch); err == nil && patch.ForceFull {
			decision.ForceFull = true
		}
	}

	return decision
}

func runPostCompactHooks(
	ctx context.Context,
	runner *hooks.Runner,
	sessionID string,
	output CompactHookOutput,
) {
	if runner == nil {
		return
	}

	inputJSON, err := json.Marshal(output)
	if err != nil {
		slog.Warn("LCM hooks: failed to marshal PostCompact input", "error", err)
		return
	}

	result, err := runner.Run(ctx, hooks.EventPostCompact, sessionID, lcmCompactTool, string(inputJSON))
	if err != nil {
		slog.Warn("LCM hooks: PostCompact hook execution error", "error", err)
		return
	}

	// XRUSH: log error before discarding
	slog.Debug("LCM hooks: PostCompact result discarded", "result", result)
	_ = result
}

func buildPreCompactInput(sessionID string, tokenCount int64, budget Budget, blocking bool) CompactHookInput {
	return CompactHookInput{
		SessionID:     sessionID,
		TokenCount:    tokenCount,
		SoftThreshold: budget.SoftThreshold,
		HardLimit:     budget.HardLimit,
		OverSoft:      tokenCount > budget.SoftThreshold,
		OverHard:      tokenCount > budget.HardLimit,
		Blocking:      blocking,
	}
}

func buildPostCompactOutput(sessionID string, result CompactionResult, blocking bool, start time.Time) CompactHookOutput {
	return CompactHookOutput{
		SessionID:       sessionID,
		Success:         result.ActionTaken,
		Rounds:          result.Rounds,
		TokenCountAfter: result.TokenCount,
		Blocking:        blocking,
		DurationMs:      time.Since(start).Milliseconds(),
	}
}
