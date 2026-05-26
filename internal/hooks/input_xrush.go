package hooks

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PostPayload extends Payload with tool output and duration for PostToolUse.
type PostPayload struct {
	Payload
	ToolOutput string `json:"tool_output"`
	DurationMs int64  `json:"duration_ms"`
}

// BuildPostPayload constructs the JSON stdin payload for a PostToolUse hook.
func BuildPostPayload(eventName, sessionID, cwd, toolName, toolInputJSON, toolOutput string, durationMs int64) []byte {
	toolInput := json.RawMessage(toolInputJSON)
	if !json.Valid(toolInput) {
		toolInput = json.RawMessage("{}")
	}
	p := PostPayload{
		Payload: Payload{
			Event:     eventName,
			SessionID: sessionID,
			CWD:       cwd,
			ToolName:  toolName,
			ToolInput: toolInput,
		},
		ToolOutput: toolOutput,
		DurationMs: durationMs,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// BuildPostEnv extends BuildEnv with CRUSH_TOOL_OUTPUT and
// CRUSH_TOOL_DURATION_MS for PostToolUse hooks.
func BuildPostEnv(eventName, toolName, sessionID, cwd, projectDir, toolInputJSON, toolOutput string, durationMs int64) []string {
	env := BuildEnv(eventName, toolName, sessionID, cwd, projectDir, toolInputJSON)
	env = append(env,
		fmt.Sprintf("CRUSH_TOOL_OUTPUT=%s", toolOutput),
		fmt.Sprintf("CRUSH_TOOL_DURATION_MS=%d", durationMs),
	)
	return env
}

// parsePostStdout parses PostToolUse hook stdout for modified_output field.
// Returns the replacement text, or empty string if no modification.
func parsePostStdout(stdout string) string {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return ""
	}
	var parsed struct {
		ModifiedOutput string `json:"modified_output"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err == nil && parsed.ModifiedOutput != "" {
		return parsed.ModifiedOutput
	}
	if json.Valid([]byte(stdout)) {
		return ""
	}
	return stdout
}
