# Crush Development Guide

## Project Overview

Crush is a terminal-based AI coding assistant built in Go by
[Charm](https://charm.land). It connects to LLMs and gives them tools to read,
write, and execute code. It supports multiple providers (Anthropic, OpenAI,
Gemini, Bedrock, Copilot, Hyper, MiniMax, Vercel, and more), integrates with
LSPs for code intelligence, and supports extensibility via MCP servers and
agent skills.

The module path is `github.com/charmbracelet/crush`.

This fork branch also carries a large squashed delta centered on:

- **`internal/lcm/`**: lossless context management, summaries, large-output
  storage, and extra LCM tools.
- **`internal/repomap/`**: repository indexing, ranking, rendering, caching,
  and refresh/reset control paths.
- **`internal/treesitter/`**: parser pool, grammar/query loading, and shared
  code analysis used by repo-map and LCM exploration.
- **`internal/app/`**: app wiring for LCM and repo-map initialization.
- **`internal/agent/`**: coordinator hooks, prepare-step injection, and tool
  wiring for repo-map/LCM behavior.
- **`internal/db/`**: goose migrations and sqlc queries for LCM and repo-map
  persistence.
- **`internal/filetracker/`**: session read-path tracking used by repo-map
  ranking and exclusion logic.

Keep this file focused on stable workflow and architecture notes. Track open
branch issues in `TODO.md`.

## Architecture

```
main.go                            CLI entry point (cobra via internal/cmd)
internal/
  app/app.go                       Top-level wiring: DB, config, agents, LSP, MCP, events
  cmd/                             CLI commands (root, run, login, models, stats, sessions)
  config/
    config.go                      Config struct, context file paths, agent definitions
    load.go                        crush.json loading and validation
    store.go                       ConfigStore runtime wrapper and persistence
  agent/
    agent.go                       SessionAgent: runs LLM conversations per session
    coordinator.go                 Coordinator: manages named agents ("coder", "task")
    coordinator_opts.go            Fork hook plumbing for repo-map and LCM
    prompts.go                     Loads Go-template system prompts
    templates/                     System prompt templates
    tools/                         Built-in tools and MCP integration
  session/session.go               Session CRUD backed by SQLite
  message/                         Message model and content types
  db/                              SQLite via sqlc, with migrations
    sql/                           Raw SQL queries (consumed by sqlc)
    migrations/                    Schema migrations
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

- **Config is accessed through `ConfigStore`**: the store owns the pure-data
  config, runtime state, and config persistence.
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

## Build/Test/Lint Commands

- **Build**: `go build .` or `go run .`
- **Source build requirement**: This branch requires `CGO_ENABLED=1` and a
  working C compiler because tree-sitter support is mandatory.
- **Test**: `task test` or `go test ./...`
  - Run single test: `go test ./internal/config -run TestConfigMerging`
- **Focused fork packages**:
  `go test ./internal/agent ./internal/app ./internal/lcm ./internal/repomap ./internal/treesitter`
- **Race suite for fork additions**:
  `go test -race ./internal/agent ./internal/app ./internal/lcm ./internal/repomap ./internal/treesitter`
- **Update Golden Files**: `go test ./... -update` (regenerates `.golden`
  files when test output changes)
  - Update specific package:
    `go test ./internal/tui/components/core -update`
- **Lint**: `task lint:fix`
- **Format**: `task fmt` (`gofumpt -w .`)
- **Modernize**: `task modernize`
- **Dev**: `task dev`
- **Additional fork-only tasks**: see `Taskfile.xrush.yaml`

## Branch-Specific Wiring

- **LCM activation**: `cfg.Options.LCM != nil` enables LCM. The app wraps the
  message service in `internal/app/app_lcm.go`.
- **Repo-map activation**: `cfg.Options.RepoMap` enables repo-map setup in
  `internal/app/repomap.go`.
- **Repo-map injection**: the coordinator injects repo-map context via
  prepare-step hooks in `internal/agent/coordinator_opts.go`.
- **Filetracker matters**: repo-map uses tracked read paths as ranking and
  exclusion hints; changes in `internal/filetracker/` affect prompt context.
- **DB work spans four layers**: migration in `internal/db/migrations/`, SQL in
  `internal/db/sql/`, generated sqlc files in `internal/db/*.sql.go`, and
  tests covering both read and write paths.

## Code Style Guidelines

- **Imports**: Use `goimports` formatting, group stdlib, external, internal
  packages.
- **Formatting**: Use gofumpt (stricter than gofmt), enabled in
  golangci-lint.
- **Naming**: Standard Go conventions. PascalCase for exported, camelCase for
  unexported.
- **Types**: Prefer explicit types and small, clear aliases where they help.
- **Error handling**: Return errors explicitly and wrap with `fmt.Errorf`.
- **Context**: Always pass `context.Context` as first parameter for operations.
- **Interfaces**: Define interfaces in consuming packages, keep them small and
  focused.
- **Structs**: Use struct embedding for composition, group related fields.
- **Constants**: Use typed constants with iota for enums, group in const
  blocks.
- **Testing**: Use testify's `require` package, `t.Parallel()`, `t.SetEnv()`,
  and `t.TempDir()`.
- **JSON tags**: Use snake_case for JSON field names.
- **File permissions**: Use octal notation (0o755, 0o644) for file
  permissions.
- **Log messages**: Log messages must start with a capital letter.
  `task lint:log` enforces this.
- **Comments**: End comments in periods unless comments are at the end of the
  line.

## Testing with Mock Providers

When writing tests that involve provider configurations, use the mock
providers to avoid API calls:

```go
func TestYourFunction(t *testing.T) {
    originalUseMock := config.UseMockProviders
    config.UseMockProviders = true
    defer func() {
        config.UseMockProviders = originalUseMock
        config.ResetProviders()
    }()

    config.ResetProviders()

    providers := config.Providers()
    _ = providers
}
```

## Formatting

- ALWAYS format any Go code you write.
  - First, try `gofumpt -w .`.
  - If `gofumpt` is unavailable, use `goimports`.
  - If `goimports` is unavailable, use `gofmt`.

## Comments

- Own-line comments should start with capital letters and end with periods.
- Wrap comments at 78 columns.

## Committing

- ALWAYS use semantic commits (`fix:`, `feat:`, `chore:`, `refactor:`,
  `docs:`, `sec:`, etc).
- Try to keep commits to one line unless extra context is genuinely necessary.

## Working on Configuration

Read `internal/config/AGENTS.md` before changing config loading, merge rules,
or schema-backed options.

## Working on the TUI (UI)

Read `internal/ui/AGENTS.md` before changing Bubble Tea models, components, or
styles.

## Working on LCM, Repo Map, or Tree-Sitter

Start with these files:

- `internal/app/app_lcm.go`
- `internal/app/repomap.go`
- `internal/agent/coordinator.go`
- `internal/agent/coordinator_opts.go`
- `internal/lcm/`
- `internal/repomap/`
- `internal/treesitter/`
