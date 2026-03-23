# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Crush is a terminal-based AI coding assistant built in Go on the Charm ecosystem (Bubble Tea, Lipgloss, Glamour). It supports multiple LLM providers via the Fantasy abstraction, session-based conversations, LSP integration, MCP extensibility, and token-aware context management (LCM).

## Build Commands

```bash
task build          # Compile binary (CGO_ENABLED=1, GOEXPERIMENT=greenteagc)
task test           # Run tests with race detector (CGO_ENABLED=1)
task lint           # Run golangci-lint + log capitalization check
task lint:fix       # Run linters with auto-fix
task fmt            # Format with gofumpt (stricter than gofmt)
task dev            # Run with pprof profiling on :6060
task install        # Install binary
task modernize      # Run go modernize pass
```

Source builds on this branch require `CGO_ENABLED=1` and a working C compiler
because tree-sitter support is mandatory.

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

## CLI

```
crush [command] [--flags]
```

### Commands

| Command | Description |
|---------|-------------|
| `run [prompt...] [--flags]` | Single non-interactive prompt (`-m model`, `-q` quiet, `--small-model`) |
| `models [filter]` | List available models from configured providers |
| `stats` | Show usage statistics (tokens, costs, activity) |
| `projects [--json]` | List known project directories |
| `dirs [config\|data]` | Print config/data directories |
| `logs [-f] [-t N]` | View crush logs (`-f` follow, `-t` tail N lines) |
| `login [hyper\|copilot]` | Authenticate with a platform |
| `completion [bash\|fish\|zsh\|powershell]` | Generate shell completions |
| `update-providers [path-or-url]` | Update provider metadata (`--source=catwalk\|hyper`) |

### Global Flags

`-c` cwd, `-D` data-dir, `-d` debug, `-y` yolo (auto-accept all permissions), `-v` version.

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `CRUSH_GLOBAL_CONFIG` | Override global config directory path |
| `CRUSH_GLOBAL_DATA` | Override global data directory path |
| `CRUSH_SKILLS_DIR` | Custom skills directory |
| `CRUSH_DISABLE_PROVIDER_AUTO_UPDATE` | Disable automatic provider metadata updates |
| `CRUSH_DISABLE_DEFAULT_PROVIDERS` | Disable merging with embedded providers |
| `CRUSH_DISABLE_METRICS` | Disable telemetry (also respects `DO_NOT_TRACK`) |
| `CRUSH_DISABLE_ANTHROPIC_CACHE` | Disable Anthropic prompt caching |
| `CRUSH_ANTHROPIC_API_KEY`, `CRUSH_OPENAI_API_KEY`, etc. | Provider-specific API keys |
| `CATWALK_URL` | Override provider metadata source |
| `HYPER_URL` / `HYPER_ENABLE` | Hyper provider integration |

## Configuration

Config files are loaded and merged in order: `~/.config/crush/crush.json` → `crush.json` / `.crush.json` (project) → data-dir config. Merge rules: booleans OR, slices append+dedup, maps merge, timeouts max, pointers last-non-nil.

### Config Schema

```jsonc
{
  "$schema": "https://charm.land/crush.json",
  "models": {
    "large": { "model": "id", "provider": "id", "max_tokens": N, "temperature": F, "top_p": F, "reasoning_effort": "low|medium|high", "think": bool },
    "small": { "model": "id", "provider": "id" }
  },
  "providers": {
    "<id>": { "id": "", "name": "", "base_url": "", "type": "openai|openai-compat|anthropic|gemini|azure|vertexai", "api_key": "", "disable": bool, "extra_headers": {}, "extra_body": {}, "system_prompt_prefix": "", "models": [...] }
  },
  "mcp": {
    "<id>": { "type": "stdio|sse|http", "command": "", "args": [], "env": {}, "url": "", "headers": {}, "disabled": bool, "disabled_tools": [], "timeout": N }
  },
  "lsp": {
    "<id>": { "disabled": bool, "command": "", "args": [], "env": {}, "filetypes": [], "root_markers": [], "options": {}, "timeout": N }
  },
  "options": {
    "context_paths": [],          // Context files (CLAUDE.md, AGENTS.md, .cursorrules, etc.)
    "skills_paths": [],           // Skill directories
    "debug": bool,                // Debug logging
    "debug_lsp": bool,            // LSP debug logging
    "disable_auto_summarize": bool,
    "data_directory": ".crush",
    "disabled_tools": [],         // Globally disable tools by name
    "disable_provider_auto_update": bool,
    "disable_default_providers": bool,
    "disable_metrics": bool,
    "initialize_as": "AGENTS.md", // Context file name for project init
    "auto_lsp": true,             // Auto-setup LSPs
    "progress": true,             // Show progress updates
    "tui": {
      "compact_mode": bool, "diff_mode": "unified|split", "transparent": bool,
      "completions": { "max_depth": N, "max_items": N }
    },
    "lcm": {
      "ctx_cutoff_threshold": 0.6,               // Soft compact trigger (fraction of context window)
      "large_tool_output_token_threshold": 10000, // Store outputs above this in SQLite
      "explorer_output_profile": "enhancement|parity"
    },
    "repo_map": {
      "disabled": bool, "max_tokens": N, "exclude_globs": [], "refresh_mode": "auto|files|manual|always",
      "map_mul_no_files": 2.0, "parser_pool_size": N
    },
    "attribution": { "trailer_style": "none|co-authored-by|assisted-by", "generated_with": bool }
  },
  "permissions": { "allowed_tools": [] },
  "tools": {
    "ls": { "max_depth": N, "max_items": N },
    "grep": { "timeout": "5s" }
  }
}
```

## Agent Tools

| Tool | Type | Description |
|------|------|-------------|
| `bash` | Execution | Execute shell commands |
| `job_output` | Execution | Get background job output |
| `job_kill` | Execution | Kill background jobs |
| `edit` | Code | Edit files with LSP integration |
| `multiedit` | Code | Batch file editing |
| `write` | File I/O | Write new files |
| `download` | File I/O | Download files from URLs |
| `view` | Reading | Read file contents |
| `glob` | Search | File pattern matching |
| `grep` | Search | Regex file search |
| `ls` | Search | List directory contents |
| `todos` | Search | Find TODO comments |
| `fetch` / `agentic_fetch` | Web | Fetch HTTP content |
| `web_search` | Web | Search the web |
| `sourcegraph` | Search | Code search integration |
| `agent` | Delegation | Spawn sub-agents (`coder` or `task` type) |
| `lsp_diagnostics` | Inspection | LSP code diagnostics |
| `lsp_references` | Inspection | Find symbol references |
| `lsp_restart` | Admin | Restart LSP servers |
| `list_mcp_resources` | MCP | List MCP resources |
| `read_mcp_resource` | MCP | Read MCP resources |
| `lcm_grep` | LCM | Context-aware search |
| `lcm_describe` | LCM | Summarize context |
| `lcm_expand` | LCM | Expand context summaries |
| `agentic_map` / `llm_map` | RepoMap | Repo map generation |
| `map_refresh` | RepoMap | Refresh repo map cache |

Tools can be disabled globally (`options.disabled_tools`), per-agent (`Agent.allowed_tools`), or by permissions config. MCP tools are loaded dynamically from connected servers.

## TUI Keybindings

**Global**: `ctrl+c` quit, `ctrl+g` help, `ctrl+p` commands, `ctrl+m`/`ctrl+l` model selector, `ctrl+s` session switcher, `ctrl+n` new session, `ctrl+d` toggle compact, `ctrl+t`/`ctrl+space` toggle task pills, `tab` change focus.

**Chat navigation**: `↑`/`k` scroll up, `↓`/`j` scroll down, `shift+↑`/`K` jump up, `shift+↓`/`J` jump down, `u`/`d` half page, `b`/`f` full page, `g`/`G` top/bottom, `c`/`y` copy, `space` expand/collapse.

**Editor**: `enter` send, `shift+enter`/`ctrl+j` newline, `ctrl+o` external editor, `ctrl+f` add image, `ctrl+v` paste image, `/` commands (empty input) or add file (at start), `@` mention file, `↑`/`↓` history.

## Slash Commands

Custom commands are `.md` files loaded from: `~/.config/crush/commands/`, `~/.crush/commands/`, `.crush/commands/`. Variables (`$VARIABLE_NAME`) become arguments. Built-in: `/new`, `/model`, `/export`, `/clear`. MCP servers can also expose commands via `/prompts`.

## Dev Binary

`../xrush/` is the folder where the developer config (`crush.json`) lives for dev usage and live testing. After building, run with: `env CRUSH_GLOBAL_CONFIG=/home/user/working/xrush ./crush`

## Subsystem Guides

Read these before working in the corresponding areas:
- **Config changes**: `internal/config/AGENTS.md` — merge rules and how to add fields
- **UI changes**: `internal/ui/AGENTS.md` — Bubble Tea patterns, component design, styling, gotchas
