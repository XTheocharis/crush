# Agent Package

The `agent` package implements Crush's LLM conversation loop, multi-agent
orchestration, and all built-in tools. A `SessionAgent` handles a single
conversation; a `Coordinator` manages named agents and wires in hooks,
permissions, and the LCM subsystem. The fork adds orchestration primitives
(operator, parallel, swarm), context management (LCM, repo map), and
diagnostic tooling (autofix, diagnostics gate).

## Architecture

```
agent.go                          SessionAgent: one LLM conversation per session
agentconfig.go                    AgentConfig type, defaults, validation
coordinator.go                    Coordinator: manages named agents ("coder", "task")
coordinator_xrush_recovery.go     Recover from interrupted/streaming LLM responses
coordinator_xrush_repomap.go      Inject repo-map context into coordinator loop
hooked_tool.go                    PreToolUse hook decorator around tool execution
prompts.go                        System prompt template loading

operator.go                       Operator: recursive task decomposition via DAG
parallel.go                       Parallel: bounded fan-out concurrent execution
swarm.go                          Swarm: multi-agent parallel coordination
productive.go                     Productive: constrained high-output agent loop
doom.go                           Doom-loop detection and auto-recovery

architect_plan.go                 Structured planning before code changes
autofix.go                        Iterative lint → fix → test → reflect cycle
go_linter.go                      Go-specific vet/staticcheck integration

model_router.go                   Deprecated model router; fallback for TierRouter when no RouterTiers configured
router_tier.go                    Tier definitions for model routing
ratelimit.go                      Reactive 429-backoff rate limit coordination
resource_limits.go                Concurrency caps, token budgets, escalation

cache_share.go                    Cross-agent cache sharing via colon-separated string keys
config_loader.go                  Dynamic agent configuration from crush.json
prompt_assembly.go                → moved to internal/extensions/prompt_assembly_ext.go (PromptAssemblyExtension)
structured_subagent.go            Typed-output forked child agents
structured_types.go               Shared types for structured sub-agents
forked.go                         Forked session support for parallel branches
tool_surface.go                   Tool registry and surface description for prompts
ext_hooks.go                      External lifecycle hook callbacks
lcm_client.go                     LCM adapter for agent-level integration
loop_detection.go                 Repetition detection in agent tool calls

prompt/                           Prompt assembly sub-package
  context.go                      Context file loading (AGENTS.md, CRUSH.md, etc.)
  prompt.go                       Prompt construction and template execution
  prompt_extra.go                 Extra prompt assembly helpers

templates/                        System prompt Go templates (*.md.tpl)

tools/                            Built-in tools (see tools/ section below)
tools/mcp/                        MCP client integration
```

## Orchestration

### operator.go

Recursive task decomposition engine. Breaks a top-level task into subtasks
using four strategies: `LLMMap`, `AgenticMap`, `Batch`, and `Sequential`.
Each subtask can itself spawn further subtasks up to a configurable depth
limit. Cycle detection prevents infinite recursion. Supports up to 16
parallel workers with future-based result collection.

Key types:
- `Operator` — orchestrates subtask DAG execution.
- `OperatorConfig` — depth limits, strategy selection, worker count.

### parallel.go

Concurrent task controller with bounded semaphores. Serializes access to
the same focus area to avoid conflicting writes. Tracks resource usage per
task and collects results via futures.

Key types:
- `Parallel` — fan-out execution manager.
- `ParallelTask` — individual concurrent task descriptor.

### swarm.go

Decomposes a task across multiple sub-agents that run in parallel, then
synthesizes their results into a single answer. Uses shared caching and
configurable concurrency limits.

Key types:
- `Swarm` — multi-agent swarm coordinator.

### productive.go

Constrained agent loop that maximizes output quality under resource
limits. Tracks a content-addressed fingerprint of the conversation state
to detect progress stalls.

Key types:
- `Productive` — productive-mode agent loop.

### doom.go

Detects doom loops (repeated failures with no forward progress) and
escalates through soft, medium, and hard recovery levels. Hard recovery
may restart the agent with a simplified prompt.

Key types:
- `DoomDetector` — doom-loop detector with escalation.

## Planning and Fix

### architect_plan.go

Generates a structured execution plan before making code changes. The plan
is parsed from LLM JSON output and includes file-level steps with
dependencies.

Key types:
- `ArchitectPlan` — parsed plan with ordered steps.
- `PlanStep` — single step with target file and action.

### autofix.go

Iterative auto-fix loop: run diagnostics → apply fix → test → reflect.
Rolls back file changes if the fix introduces new errors. Tracks fix
attempts per diagnostic to avoid infinite loops.

Key types:
- `AutoFixer` — manages the fix cycle.
- `FixResult` — outcome of a single fix attempt.

### go_linter.go

Runs Go-specific linting (`go vet`, `staticcheck`) and returns structured
diagnostics. Integrates with the autofix cycle for Go projects.

Key types:
- `GoLinter` — Go linter integration.

## Routing and Limits

### model_router.go

Deprecated model router; kept for compatibility. Active routing via
`router_tier.go` `TierRouter`, with `ModelRouter` used as runtime fallback
via `ModelRouterExtension` when no `RouterTiers` are configured.

Key types:
- `TierRouter` — N-tier model routing with configurable thresholds.
- `RoutingTier` — tier definition with model ID and token thresholds.
- `ModelRouter` — deprecated model selection logic (binary threshold).

### router_tier.go

Defines model tiers (small, default, large) used by `ModelRouter`.

Key types:
- `RouterTier` — tier definition with model ID and token thresholds.

### ratelimit.go

Reactive 429-backoff coordination for LLM API calls. When one call
receives a 429, all concurrent calls to the same provider respect the
shared backoff rather than independently racing.

Key types:
- `RateLimitCoordinator` — per-provider backoff coordination.

### resource_limits.go

Enforces per-session resource consumption: concurrent tool calls, total
token budget, and maximum conversation turns. Escalates from warning to
hard limit.

Key types:
- `ResourceUsage` — real-time resource consumption tracker.
- `ResourceLimit` — soft/hard limit for a single resource dimension.
- `SubagentLimits` — per-subagent-type resource limits.
- `ResourceLimitedTask` — task wrapper with limit enforcement.

## Context and Caching

### cache_share.go

Thread-safe, TTL-aware key-value store shared across agent sessions.
Keys are colon-separated strings with a category prefix (e.g.,
`"diagnostics:session-abc"`) supporting wildcard invalidation.

Key types:
- `SharedCache` — shared cache instance.

### config_loader.go

Loads and validates dynamic agent configuration from `crush.json`.
Supports per-agent overrides, tool allow/deny lists, and model routing
configuration.

Key types:
- `AgentConfigLoader` — configuration loading and validation.

### prompt_assembly.go

Assembles the full system prompt at runtime from template, context files,
tool descriptions, and dynamic instructions. Handles token budget
truncation. Now located at `internal/extensions/prompt_assembly_ext.go`.

Key types:
- `PromptAssemblyExtension` — prompt composition engine.

### lcm_client.go

Thin adapter that connects the agent to the LCM subsystem for context
summarization, compaction, and retrieval.

Key types:
- `LCMClient` — LCM integration adapter.

## Session Management

### forked.go

Supports forking a session into parallel branches. Each fork gets its own
message history but shares the underlying file state.

Key types:
- `ForkedSession` — forked session handle.

### structured_subagent.go

Forks a child agent with its own message history, runs it to completion,
and returns typed output. Supports mailbox-based inter-agent
communication and per-agent tool permissions.

Key types:
- `StructuredSubagent` — typed-output child agent.
- `SubagentConfig` — configuration for child agent.

### structured_types.go

Shared types for structured sub-agent output schemas.

### coordinator_xrush_recovery.go

Recovers from interrupted or malformed LLM streaming responses. Detects
partial tool calls and either retries or falls back to a simplified
prompt.

### coordinator_xrush_repomap.go

Injects repository map context into the coordinator's agent loop before
each LLM call.

### loop_detection.go

Detects repetitive tool-call patterns in the conversation (e.g., the same
edit applied twice). Signals the agent to change strategy.

## Tool Surface

### tool_surface.go

Builds the tool description surface exposed to the LLM. Filters available
tools by agent name, permissions, and configuration. Generates the tool
schema passed in the API request.

Key types:
- `ToolSurface` — tool registry and description builder.

### ext_hooks.go

External lifecycle hooks that fire on agent events (tool start, tool end,
session start, etc.). Runs user-defined shell commands.

Key types:
- `ExtHooks` — external hook manager.

## Tools Sub-Package (`tools/`)

All built-in tools. Each tool has a Go implementation and an embedded `.md`
description file. The fork adds the following tools:

### Map and Context

- `agentic_map.go` — Run a sub-agent on each item in a JSONL file and
  write results to another JSONL file. Supports schema validation and
  configurable concurrency.
- `llm_map.go` — Apply an LLM transformation to each item in a JSONL
  file. Read-only; no tool access for sub-agents.
- `map_refresh.go` — Force invalidation and regeneration of the
  repository map cache.
- `lcm_describe.go` — Describe a file or summary by its LCM identifier.
  Returns content preview and metadata.
- `lcm_expand.go` — Expand an LCM summary to its original messages.
- `lcm_grep.go` — Search conversation history with full-text or regex
  search.

### Diagnostics

- `diag_autofix.go` — Iterative diagnostic auto-fix tool. Runs
  diagnostics, applies fixes, tests, and reflects.
- `diag_gate.go` — Quality gate that checks LSP diagnostics before
  allowing a task to complete.

### Editing

- `edit_batch_tool.go` — Apply multiple string-replacement edits across
  files in a single atomic batch with rollback on failure.
- `edit_anchors.go` — Content-addressed hash anchors for drift-tolerant
  edit targeting. Anchors survive minor file changes.
- `edit_anchor_ops.go` — Anchor edit operations: insert_before,
  insert_after, replace_range, delete_range.
- `edit_anchors_cache.go` — Cache for anchor hash maps to avoid
  re-scanning files.
- `edit_fuzzy.go` — Fuzzy string matching for approximate edit targets.

### Orchestration

- `orchestration_types.go` — Shared types for operator, parallel, and
  swarm coordination.
- `send_message.go` — Send a message to another agent via the mailbox
  system.
- `task_stop.go` — Stop a running forked sub-agent.
- `team_create.go` — Create a named team of agents for coordinated
  execution.
- `team_delete.go` — Delete a team and stop its member agents.
- `synthetic_output.go` — Generate synthetic output for testing and
  simulation.

### Inspection

- `crush_logs.go` — Read Crush's internal application logs.
- `view_xrush.go` — Enhanced view tool with LCM context awareness.

### Validation

- `rollback.go` — Revert files to their prior state on failure.
- `validate.go` — Tree-sitter-based validation of file edits (build tag:
  `treesitter`).
- `validate_stub.go` — No-op validation fallback when tree-sitter is
  unavailable.
- `validation_handler.go` — Post-edit validation pipeline.
