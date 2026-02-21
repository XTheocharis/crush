# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Crush is a terminal-based AI coding assistant built in Go on the Charm ecosystem (Bubble Tea, Lipgloss, Glamour). It supports multiple LLM providers via the Fantasy abstraction, session-based conversations, LSP integration, MCP extensibility, and token-aware context management (LCM).

## Build Commands

```bash
task build          # Compile binary (CGO_ENABLED=0, GOEXPERIMENT=greenteagc)
task test           # Run tests with race detector (CGO_ENABLED=1)
task lint           # Run golangci-lint + log capitalization check
task lint:fix       # Run linters with auto-fix
task fmt            # Format with gofumpt (stricter than gofmt)
task dev            # Run with pprof profiling on :6060
task install        # Install binary
task modernize      # Run go modernize pass
```

Single test: `go test ./internal/config -run TestConfigMerging`
Update golden files: `go test ./internal/tui/components/core -update`
Record VCR cassettes: `task test:record` (re-records all HTTP fixtures in `internal/agent/testdata`)

## Architecture

### Core Flow

```
CLI (cmd/) → App (app/) → Coordinator (agent/) → SessionAgent → Fantasy LLM
                                                      ↓
                                                 Tools (agent/tools/)
                                                      ↓
                                              Permission Service
```

### Key Packages

- **`internal/agent/`** — Core orchestration. `Coordinator` manages agents per session. `SessionAgent` runs prompt→tool loops against Fantasy LLMs. Tool implementations live in `agent/tools/` (40+ tools including bash, edit, grep, web search, MCP).
- **`internal/ui/`** — Bubble Tea v2 TUI. Main model in `ui/model/ui.go` owns state and routes messages. Components in `chat/`, `dialog/`, `list/` are dumb renderers. Styles in `styles/styles.go`.
- **`internal/config/`** — Multi-source config loading (`.crush.json` > `crush.json` > `~/.config/crush/crush.json`). Each field type has explicit merge rules (booleans OR, slices append+dedup, maps merge, timeouts max, pointers last-non-nil). **Adding a config field requires updating the struct's `merge()` method and `merge_test.go`.**
- **`internal/lcm/`** — Language Context Management. Token-aware context compaction with soft/hard thresholds, async summarization, and SQLite-backed state.
- **`internal/db/`** — SQLite via goose migrations + sqlc-generated queries. Schema in `db/sql/*.sql`.
- **`internal/lsp/`** — LSP client integration via powernap for code diagnostics.
- **`internal/app/`** — Wires services together: sessions, messages, permissions, history, filetracker, LSP, LCM, agent coordinator.
- **`internal/pubsub/`** — Generic typed pub/sub broker. Services publish Created/Updated/Deleted events consumed by the UI via Tea commands.

### Service Pattern

Major subsystems expose `Service` interfaces (session, message, permission, history, filetracker). Interfaces are defined in consuming packages. LCM decorates the message service to intercept and manage token budgets.

### Provider Abstraction

Fantasy (`charm.land/fantasy`) abstracts LLM providers (Anthropic, OpenAI, Google, Azure, Bedrock, OpenRouter, Vercel, OpenAI-compatible). Catwalk (`charm.land/catwalk`) provides model metadata and pricing. Provider configs live in `internal/agent/hyper/`.

## Code Style

- **Formatting**: gofumpt (always format before committing; `task fmt`)
- **Imports**: goimports grouping — stdlib, external, internal
- **Log messages**: Must start with a capital letter (enforced by `task lint:log`)
- **Comments**: Own-line comments start capitalized, end with period, wrap at 78 columns
- **JSON tags**: snake_case
- **File permissions**: Octal notation (0o755, 0o644)
- **Commits**: Semantic prefixes (`fix:`, `feat:`, `chore:`, `refactor:`, `docs:`, `sec:`)
- **Testing**: `testify/require`, `t.Parallel()`, `t.SetEnv()`, `t.TempDir()` (auto-cleaned)
- **Mock providers**: Set `config.UseMockProviders = true` and call `config.ResetProviders()` in tests to avoid API calls

## Subsystem Guides

Read these before working in the corresponding areas:
- **Config changes**: `internal/config/AGENTS.md` — merge rules and how to add fields
- **UI changes**: `internal/ui/AGENTS.md` — Bubble Tea patterns, component design, styling, gotchas
