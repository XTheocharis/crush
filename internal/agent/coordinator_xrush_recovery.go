// xrush session recovery for interrupted messages
package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/crush/internal/message"
)

// RestoreAgentConfig restores checkpointed session agent configuration (skills,
// tools, agents) after compaction. It is called by the LCM post-compact
// cleaner to re-mark loaded skills in the tracker.
//
// The payload is a map from category to list of names:
//   - "skills" -> loaded skill names
//   - "tools_delta" -> loaded MCP tool names
//   - "agents" -> active agent names
func (c *coordinator) RestoreAgentConfig(_ context.Context, payload map[string][]string) error {
	if payload == nil {
		return nil
	}

	// Restore loaded skills in the tracker.
	if c.skillTracker != nil {
		if skills, ok := payload["skills"]; ok && len(skills) > 0 {
			c.skillTracker.RestoreLoadedSkills(skills)
			slog.Debug("Restored loaded skills after compaction",
				"skills", skills,
				"count", len(skills),
			)
		}
	}

	return nil
}

// RecoverSession recovers corrupt sessions by finishing interrupted messages.
// It iterates through all messages in the session and finishes any that have
// unfinished thinking, tool calls, or no finish marker.
func (c *coordinator) RecoverSession(ctx context.Context, sessionID string) error {
	if c.currentAgent != nil && c.currentAgent.IsSessionBusy(sessionID) {
		return nil
	}

	msgs, err := c.messages.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	for i := range msgs {
		msg := &msgs[i]
		if msg.IsFinished() {
			continue
		}

		msg.FinishThinking()
		for _, tc := range msg.ToolCalls() {
			if !tc.Finished {
				msg.FinishToolCall(tc.ID)
			}
		}

		msg.AddFinish(message.FinishReasonError, "Session interrupted", "The session was previously interrupted")
		if updateErr := c.messages.Update(ctx, *msg); updateErr != nil {
			slog.Error("Failed to recover message", "message_id", msg.ID, "error", updateErr)
		}
	}

	return nil
}
