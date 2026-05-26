package agent

import (
	"fmt"
	"slices"
)

// AgentConfig holds per-subagent configuration that constrains what a subagent
// can do. It is intentionally independent of config.Agent (which carries
// model/identity fields) to avoid circular imports and keep subagent concerns
// self-contained.
type AgentConfig struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	AgentType string `json:"agent_type,omitempty" yaml:"agent_type,omitempty"`

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Tools lists the tool names this subagent is allowed to use. Empty means
	// all tools are available.
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`

	// Permissions lists permission grants (e.g., "bash:allow", "edit:allow").
	Permissions []string `json:"permissions,omitempty" yaml:"permissions,omitempty"`

	// MaxTokens is the maximum output token budget for the subagent. Zero
	// means unlimited.
	MaxTokens int `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`

	// MaxSteps is the maximum number of tool-use steps the subagent may take.
	// Zero means unlimited.
	MaxSteps int `json:"max_steps,omitempty" yaml:"max_steps,omitempty"`

	// MaxTurns is the maximum number of conversation turns. Zero means
	// unlimited.
	MaxTurns int `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`

	// Model is the model identifier override for this agent.
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// PermMode is the permission mode (e.g., "ask", "auto").
	PermMode string `json:"perm_mode,omitempty" yaml:"perm_mode,omitempty"`

	// SystemPrompt is a custom system prompt template for this agent.
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`

	// Environment holds environment variables to set for the agent subprocess.
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`

	// AllowedMCP maps MCP server names to allowed tool names within that
	// server. An empty slice for a server means all tools are allowed. A nil
	// map means all MCPs are allowed.
	AllowedMCP map[string][]string `json:"allowed_mcp,omitempty" yaml:"allowed_mcp,omitempty"`
}

// DefaultAgentConfig returns a sensible default configuration for a subagent.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		MaxTokens: 4096,
		MaxSteps:  25,
	}
}

// Validate checks the AgentConfig for invalid values and returns an error
// describing the first issue found.
func (c AgentConfig) Validate() error {
	if c.MaxTokens < 0 {
		return fmt.Errorf("agent config: MaxTokens must be non-negative, got %d", c.MaxTokens)
	}
	if c.MaxSteps < 0 {
		return fmt.Errorf("agent config: MaxSteps must be non-negative, got %d", c.MaxSteps)
	}
	if c.MaxTurns < 0 {
		return fmt.Errorf("agent config: MaxTurns must be non-negative, got %d", c.MaxTurns)
	}
	if hasDuplicates(c.Tools) {
		return fmt.Errorf("agent config: Tools contains duplicates")
	}
	if hasDuplicates(c.Permissions) {
		return fmt.Errorf("agent config: Permissions contains duplicates")
	}
	for server, tools := range c.AllowedMCP {
		if hasDuplicates(tools) {
			return fmt.Errorf("agent config: AllowedMCP[%q] contains duplicate tools", server)
		}
	}
	return nil
}

// ToolAllowed returns true if the named tool is in the allowed list, or if
// the list is empty (meaning all tools are allowed).
func (c AgentConfig) ToolAllowed(toolName string) bool {
	if len(c.Tools) == 0 {
		return true
	}
	return slices.Contains(c.Tools, toolName)
}

func hasDuplicates(ss []string) bool {
	seen := make(map[string]bool, len(ss))
	for _, s := range ss {
		if seen[s] {
			return true
		}
		seen[s] = true
	}
	return false
}
