# Crush Development Guide

## Project Snapshot

Crush is a terminal-based AI coding assistant built in Go on the Charm stack.
The main runtime path is:

`CLI -> App -> Coordinator -> SessionAgent -> Fantasy LLM -> Tools/Permissions`

This fork branch adds a large squashed delta centered on:

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

## Build/Test/Lint Commands

- **Build**: `go build .` or `go run .`
- **Test**: `task test` or `go test ./...`
  - Run single test: `go test ./internal/config -run TestConfigMerging`
- **Focused fork packages**:
  `go test ./internal/agent ./internal/app ./internal/lcm ./internal/repomap ./internal/treesitter`
- **Race suite for fork additions**:
  `go test -race ./internal/agent ./internal/app ./internal/lcm ./internal/repomap ./internal/treesitter`
- **Update Golden Files**: `go test ./... -update`
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

- **Imports**: Use `goimports` formatting, grouped as stdlib, external,
  internal.
- **Formatting**: Use gofumpt.
- **Naming**: Standard Go conventions. PascalCase for exported symbols,
  camelCase for unexported ones.
- **Types**: Prefer explicit types and small, clear aliases where they help.
- **Error handling**: Return errors explicitly and wrap with `fmt.Errorf`.
- **Context**: Pass `context.Context` as the first parameter for operations.
- **Interfaces**: Define interfaces in consuming packages and keep them small.
- **Structs**: Use composition/embedding where it improves clarity.
- **Constants**: Use typed constants and `iota` for enums.
- **Testing**: Use `testify/require`, `t.Parallel()`, `t.SetEnv()`, and
  `t.TempDir()`.
- **JSON tags**: Use snake_case.
- **File permissions**: Use octal notation like `0o755` and `0o644`.
- **Log messages**: Start with a capital letter. `task lint:log` enforces
  this.
- **Comments**: End comments in periods unless the comment is at end of line.

## Testing with Mock Providers

When tests depend on provider configs, use the mock providers to avoid API
calls:

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
  - First try `gofumpt -w .`.
  - If `gofumpt` is unavailable, use `goimports`.
  - If `goimports` is unavailable, use `gofmt`.

## Comments

- Own-line comments should start with capital letters and end with periods.
- Wrap comments at 78 columns.

## Committing

- ALWAYS use semantic commits such as `fix:`, `feat:`, `chore:`, `refactor:`,
  `docs:`, or `sec:`.
- Keep commits to one line unless extra context is genuinely necessary.

## Working on Configuration

Read `internal/config/AGENTS.md` before changing config loading, merge rules, or
schema-backed options.

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
