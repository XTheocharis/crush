# Crush Development Guide

## Project Overview

Crush is a terminal-based AI coding assistant built in Go by
[Charm](https://charm.land). It connects to LLMs and gives them tools to read,
write, and execute code. It supports multiple providers (Anthropic, OpenAI,
Gemini, Bedrock, Copilot, Hyper, MiniMax, Vercel, and more), integrates with
LSPs for code intelligence, and supports extensibility via MCP servers and
agent skills.

The module path is `github.com/charmbracelet/crush`.

## Architecture

```
main.go                            CLI entry point (cobra via internal/cmd)
internal/
  app/app.go                       Top-level wiring: DB, config, agents, LSP, MCP, events
  cmd/                             CLI commands (root, run, login, models, stats, sessions)
  config/
    config.go                      Config struct, context file paths, agent definitions
    load.go                        crush.json loading and validation
    provider.go                    Provider configuration and model resolution
  agent/                           (see internal/agent/AGENTS.md)
    agent.go                       SessionAgent: runs LLM conversations per session
    coordinator.go                 Coordinator: manages named agents ("coder", "task")
    coordinator_xrush_recovery.go  Recover from interrupted streaming LLM responses
    coordinator_xrush_repomap.go   Inject repo-map context into coordinator loop
    hooked_tool.go                 Decorator that runs PreToolUse hooks before tool execution
    prompts.go                     Loads Go-template system prompts
    operator.go                    Operator: recursive task decomposition via DAG
    parallel.go                    Parallel: bounded fan-out concurrent execution
    swarm.go                       Swarm: multi-agent parallel coordination
    productive.go                  Productive: constrained high-output agent loop
    doom.go                        Doom-loop detection and auto-recovery
    architect_plan.go              Structured planning before code changes
    autofix.go                     Iterative lint → fix → test → reflect cycle
    go_linter.go                   Go-specific vet/staticcheck integration
    model_router.go                Deprecated model router; kept for compatibility. Active routing via router_tier.go TierRouter, with ModelRouter as runtime fallback when no RouterTiers configured
    router_tier.go                 Tier definitions for model routing
    ratelimit.go                      Reactive 429-backoff rate limit coordination
    resource_limits.go             Concurrency caps, token budgets, escalation
    cache_share.go                    Cross-agent cache sharing via colon-separated string keys
    config_loader.go               Dynamic agent configuration from crush.json
    prompt_assembly.go             → moved to internal/extensions/prompt_assembly_ext.go (PromptAssemblyExtension)
    structured_subagent.go         Typed-output forked child agents
    structured_types.go            Shared types for structured sub-agents
    forked.go                      Forked session support for parallel branches
    tool_surface.go                Tool registry and surface description for prompts
    ext_hooks.go                   External lifecycle hook callbacks
    lcm_client.go                  LCM adapter for agent-level integration
    loop_detection.go              Repetition detection in agent tool calls
    prompt/                        Prompt assembly sub-package
      context.go                   Context file loading (AGENTS.md, CRUSH.md, etc.)
      prompt.go                    Prompt construction and template execution
      prompt_extra.go              Extra prompt assembly helpers
    templates/                     System prompt templates (*.md.tpl)
    tools/                         Built-in tools
      mcp/                         MCP client integration
      agentic_map.go               Sub-agent on each JSONL item, write results
      llm_map.go                   LLM transformation on each JSONL item (read-only)
      map_refresh.go               Force repo-map cache invalidation
      lcm_describe.go              Describe file/summary by LCM identifier
      lcm_expand.go                Expand LCM summary to original messages
      lcm_grep.go                  Search conversation history (full-text/regex)
      diag_autofix.go              Iterative diagnostic auto-fix
      diag_gate.go                 Quality gate via LSP diagnostics
      edit_batch_tool.go           Atomic multi-file batch edits with rollback
      edit_anchors.go              Content-addressed hash anchors for drift-tolerant edits
      edit_anchor_ops.go           Anchor operations (insert_before, replace_range, etc.)
      edit_anchors_cache.go        Anchor hash map cache
      edit_fuzzy.go                Fuzzy string matching for approximate edit targets
      orchestration_types.go       Shared types for operator/parallel/swarm
      send_message.go              Inter-agent mailbox messaging
      task_stop.go                 Stop a running forked sub-agent
      team_create.go               Create named agent team
      team_delete.go               Delete team and stop members
      synthetic_output.go          Synthetic output for testing/simulation
      crush_logs.go                Read Crush internal logs from session
      view_xrush.go                Enhanced view with LCM context awareness
      rollback.go                  Revert files to prior state on failure
      validate.go                  Tree-sitter validation (build tag: treesitter)
      validate_stub.go             No-op validation fallback
      validation_handler.go        Post-edit validation pipeline
  lcm/                             Lossless Context Management (see internal/lcm/AGENTS.md)
    manager.go                     Manager: LCM lifecycle, 37 methods
    compactor.go                   Core compaction logic
    store.go                       LCM persistent storage (SQLite-backed)
    compaction_layers.go           8-layer compaction pipeline
    summarizer.go                  LLM-powered conversation summarization
    memory.go                      Auto-memory: persist insights across sessions
    explorer/                      File-type exploration (see internal/lcm/explorer/AGENTS.md)
      explorer.go                  Explorer interface, Registry, dispatch
      code_treesitter.go           Tree-sitter code explorer
      stdlib/                      Per-language stdlib membership (15 languages)
  repomap/                         Repository map (see internal/repomap/AGENTS.md)
    repomap.go                     Service struct, lifecycle, Generate(), PreIndex
    tags.go                        Tree-sitter tag extraction with DB caching
    graph.go                       FileGraph from def/ref/import edges
    pagerank.go                    PageRank over FileGraph with personalization
    render.go                      RenderRepoMap: scope-aware tree-context rendering
    tiktoken.go                    cl100k_base BPE tokenizer (embedded ~1.6 MB)
  treesitter/                      Tree-sitter integration (CGO, see internal/treesitter/AGENTS.md)
    treesitter.go                  Core types: Tag, SymbolInfo, FileAnalysis, Parser
    parser.go                      ParserPool (channel-based), Analyze(), ParseTree()
    query.go                       QueryLoader: compiles and caches .scm queries
    languages.go                   Extension-to-language mapping (28 languages)
    imports.go                     Per-language import extraction
  processor/                       Message intercept pipeline (see internal/processor/AGENTS.md)
    types.go                       Processor interface, ProcessorPhase, ProcessorContext
    runner.go                      ProcessorRunner: chains processors per phase
  eval/                            Agent evaluation harness (see internal/eval/AGENTS.md)
    harness.go                     EvalHarness: parallel scorer registration and execution
    runner.go                      EvalRunner: loads JSON datasets, runs through harness
    scorers/                       Scorer sub-packages (metric, judge, mastra)
  extensions/                      Runtime extension packages
    prompt_assembly_ext.go         PromptAssemblyExtension for system prompt assembly
  rewind/                          Turn-based snapshot and rewind (see internal/rewind/AGENTS.md)
    snapshot.go                    Snapshotter: capture and retrieve turn snapshots
    rewind.go                      Rewinder: rewind code, conversation, or both
    service.go                     Service composing Snapshotter + Rewinder + Forker + Editor
  hooks/                           Hook engine: runs user shell commands on hook events
    hooks.go                       Decision types, aggregation logic, event constants
    runner.go                      Parallel hook execution, timeout, dedup
    input.go                       Stdin payload builder, env vars, stdout parsing (Crush + Claude Code compat)
  session/session.go               Session CRUD backed by SQLite
  message/                         Message model and content types
  db/                              SQLite via sqlc, with migrations
    sql/                           Raw SQL queries (consumed by sqlc)
    migrations/                    Schema migrations (lcm, repo_map, xrush_dag, turn_snapshots, etc.)
  lsp/                             LSP client manager, auto-discovery, on-demand startup
  ui/                              Bubble Tea v2 TUI (see internal/ui/AGENTS.md)
  permission/                      Tool permission checking and allow-lists
  skills/                          Skill file discovery and loading
  shell/                           Bash command execution with background job support
  event/                           Telemetry (PostHog)
  pubsub/                          Internal pub/sub for cross-component messaging
  filetracker/                     Tracks files touched per session
  history/                         Prompt history
```

### Key Dependency Roles

- **`charm.land/fantasy`**: LLM provider abstraction layer. Handles protocol
  differences between Anthropic, OpenAI, Gemini, etc. Used in `internal/app`
  and `internal/agent`.
- **`charm.land/bubbletea/v2`**: TUI framework powering the interactive UI.
- **`charm.land/lipgloss/v2`**: Terminal styling.
- **`charm.land/glamour/v2`**: Markdown rendering in the terminal.
- **`charm.land/catwalk`**: Snapshot/golden-file testing for TUI components.
- **`sqlc`**: Generates Go code from SQL queries in `internal/db/sql/`.

### Key Patterns

- **Config is a Service**: accessed via `config.Service`, not global state.
- **Tools are self-documenting**: each tool has a `.go` implementation and a
  `.md` description file in `internal/agent/tools/`.
- **System prompts are Go templates**: `internal/agent/templates/*.md.tpl`
  with runtime data injected.
- **Context files**: Crush reads AGENTS.md, CRUSH.md, CLAUDE.md, GEMINI.md
  (and `.local` variants) from the working directory for project-specific
  instructions.
- **Persistence**: SQLite + sqlc. All queries live in `internal/db/sql/`,
  generated code in `internal/db/`. Migrations in `internal/db/migrations/`.
- **Pub/sub**: `internal/pubsub` for decoupled communication between agent,
  UI, and services.
- **Hooks**: User-defined shell commands in `crush.json` that fire before
  tool execution. The engine (`internal/hooks/`) is independent of fantasy
  and agent — it takes inputs, runs commands, returns decisions. The
  `hookedTool` decorator in `internal/agent/hooked_tool.go` wraps tools at
  the coordinator level. Hooks run before permission checks. See
  `HOOKS.md` for the user-facing protocol.
- **LCM (Lossless Context Management)**: conversation summarization and
  compaction via an 8-layer pipeline. Manages token budgets, large-output
  storage, and auto-memory across sessions. See `internal/lcm/AGENTS.md`.
- **Repository Map**: scope-aware code outlines for LLM context. Uses
  PageRank over a def/ref/import graph, rendered within a token budget.
  See `internal/repomap/AGENTS.md`.
- **Tree-sitter**: code parsing and analysis via a channel-based parser
  pool with 28 grammars. Provides tag extraction, import resolution, and
  AST scope walking. Requires `CGO_ENABLED=1`.
  See `internal/treesitter/AGENTS.md`.
- **Processor Pipeline**: message intercept pipeline with four sequential
  phases (input, output stream, output result, API error). 16 processor
  implementations; 3 active by default (TokenLimiter, SystemPromptScrubber, PIIDetector), with 10 activatable via config and 6 never-wirable.
  See `internal/processor/AGENTS.md`.
- **Eval**: agent evaluation harness. Scorers assess output quality across
  metric, LLM-judge, and Mastra dimensions. Invoked via
  `crush eval --dataset <path> --scorer <name>`.
  See `internal/eval/AGENTS.md`.
- **Rewind**: turn-based snapshot and rewind. After each agent turn, files
  are snapshotted so users can rewind to any previous state. Supports
  code-only, conversation-only, or combined rewind modes.
  See `internal/rewind/AGENTS.md`.
- **Orchestration**: multi-agent coordination via operator, parallel, and
  swarm modes. Operator orchestrates sub-agents through a DAG. Parallel
  fans out concurrent execution. Swarm coordinates multiple agents toward
  a shared goal. See `internal/agent/AGENTS.md`.
- **CGO disabled**: builds with `CGO_ENABLED=1` and
  `GOEXPERIMENT=greenteagc`.

## Build/Test/Lint Commands

- **Build**: `go build .` or `go run .`
- **Test**: `task test` or `go test ./...` (run single test:
  `go test ./internal/llm/prompt -run TestGetContextFromPaths`)
- **Update Golden Files**: `go test ./... -update` (regenerates `.golden`
  files when test output changes)
  - Update specific package:
    `go test ./internal/tui/components/core -update` (in this case,
    we're updating "core")
- **Lint**: `task lint:fix`
- **Format**: `task fmt` (`gofumpt -w .`)
- **Modernize**: `task modernize` (runs `modernize` which makes code
  simplifications)
- **Dev**: `task dev` (runs with profiling enabled)

## Code Style Guidelines

- **Imports**: Use `goimports` formatting, group stdlib, external, internal
  packages.
- **Formatting**: Use gofumpt (stricter than gofmt), enabled in
  golangci-lint.
- **Naming**: Standard Go conventions — PascalCase for exported, camelCase
  for unexported.
- **Types**: Prefer explicit types, use type aliases for clarity (e.g.,
  `type AgentName string`).
- **Error handling**: Return errors explicitly, use `fmt.Errorf` for
  wrapping.
- **Context**: Always pass `context.Context` as first parameter for
  operations.
- **Interfaces**: Define interfaces in consuming packages, keep them small
  and focused.
- **Structs**: Use struct embedding for composition, group related fields.
- **Constants**: Use typed constants with iota for enums, group in const
  blocks.
- **Testing**: Use testify's `require` package, parallel tests with
  `t.Parallel()`, `t.SetEnv()` to set environment variables. Always use
  `t.Tempdir()` when in need of a temporary directory. This directory does
  not need to be removed.
- **JSON tags**: Use snake_case for JSON field names.
- **File permissions**: Use octal notation (0o755, 0o644) for file
  permissions.
- **Log messages**: Log messages must start with a capital letter (e.g.,
  "Failed to save session" not "failed to save session").
  - This is enforced by `task lint:log` which runs as part of `task lint`.
- **Comments**: End comments in periods unless comments are at the end of the
  line.

## Testing with Mock Providers

When writing tests that involve provider configurations, use the mock
providers to avoid API calls:

```go
func TestYourFunction(t *testing.T) {
    // Enable mock providers for testing
    originalUseMock := config.UseMockProviders
    config.UseMockProviders = true
    defer func() {
        config.UseMockProviders = originalUseMock
        config.ResetProviders()
    }()

    // Reset providers to ensure fresh mock data
    config.ResetProviders()

    // Your test code here - providers will now return mock data
    providers := config.Providers()
    // ... test logic
}
```

## Formatting

- ALWAYS format any Go code you write.
  - First, try `gofumpt -w .`.
  - If `gofumpt` is not available, use `goimports`.
  - If `goimports` is not available, use `gofmt`.
  - You can also use `task fmt` to run `gofumpt -w .` on the entire project,
    as long as `gofumpt` is on the `PATH`.

## Comments

- Comments that live on their own lines should start with capital letters and
  end with periods. Wrap comments at 78 columns.

## Committing

- ALWAYS use semantic commits (`fix:`, `feat:`, `chore:`, `refactor:`,
  `docs:`, `sec:`, etc).
- Try to keep commits to one line, not including your attribution. Only use
  multi-line commits when additional context is truly necessary.

## Working on the TUI (UI)

Anytime you need to work on the TUI, read `internal/ui/AGENTS.md` before
starting work.
