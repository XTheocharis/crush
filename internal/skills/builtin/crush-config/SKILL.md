---
name: crush-config
description: Use when the user needs help configuring Crush — working with crush.json, setting up providers, configuring LSPs, adding MCP servers, managing skills or permissions, or changing Crush behavior.
---

# Crush Configuration

Crush uses JSON configuration files with deep-merge priority (lowest to
highest):

1. `$XDG_CONFIG_HOME/crush/crush.json` or `$HOME/.config/crush/crush.json` (global)
2. `$HOME/.local/share/crush/crush.json` (global data)
3. Project-local configs (root→cwd walk: `.crush.json` and `crush.json` at each level)
4. `.xrush/config.yml` (project-local YAML config)
5. `.crush/<workspace>.json` (workspace-specific config)

When multiple config files exist, values are **deep-merged**: scalar values use
last-wins priority, slices accumulate from all sources, and nested objects
recurse. `disabled` uses OR-latch (either source set to `true` disables the
feature); `exclude_globs` accumulate from both locations.

## Basic Structure

```json
{
  "$schema": "https://charm.land/crush.json",
  "models": {},
  "providers": {},
  "mcp": {},
  "lsp": {},
  "hooks": {},
  "options": {},
  "permissions": {},
  "tools": {}
}
```

The `$schema` property enables IDE autocomplete but is optional.

## Config File Formats

### JSON (primary)

`.crush.json` or `crush.json` in the project root and parent directories.

### YAML (alternative)

`.xrush/config.yml` in the project root:

```yaml
options:
  auto_lsp: true
  debug_lsp: false
  validation:
    enabled: true
    auto_fix: true
```

### Include Directives in Context Files

`@include` directives work in context files (AGENTS.md, CRUSH.md, CLAUDE.md,
GEMINI.md, and `.local` variants):

```markdown
// @include path/to/file.md

// @if language=go
// @include go-conventions.md
// @endif

// @if file=go.mod
// @include go-rules.md
// @endif

// @if env=CI
// @include ci-rules.md
// @endif
```

Cycle detection prevents infinite includes.

## Shell Expansion

Crush runs selected string fields through an embedded bash-compatible
shell at load time, so values can pull from env vars, files, or helper
commands.

Supported constructs (match the `bash` tool):

- `$VAR` and `${VAR}`
- `${VAR:-default}`, `${VAR:+alt}`, `${VAR:?message}`
- `$(command)` with full quoting and nesting
- Single- and double-quoted strings, escapes

Default semantics match bash: an unset variable expands to an empty
string, no error. A failing `$(command)` is always a hard error. For
required credentials, use `${VAR:?message}` so a missing variable
fails loudly at load time with your message.

```json
{ "api_key": "${CODEBERG_TOKEN:?set CODEBERG_TOKEN}" }
```

### Which fields expand

| Surface                                             | Expansion |
| --------------------------------------------------- | --------- |
| Provider `api_key`, `base_url`, `api_endpoint`      | yes       |
| Provider `extra_headers`                            | yes       |
| Provider `extra_body`                               | **no**    |
| MCP `command`, `args`, `env`, `headers`, `url`      | yes       |
| LSP `command`, `args`, `env`                        | yes       |
| Hook `command`                                      | runs via `sh -c`, not the resolver |

`extra_body` is a JSON passthrough. If you need env-driven values in
a request body, put them in `extra_headers`, `api_key`, or
`base_url` instead.

### Empty-resolved headers are dropped

When a header value resolves to the empty string (unset variable,
`$(echo)`, or literal `""`), the header is omitted from the
outgoing request. This keeps optional env-gated headers like
`"OpenAI-Organization": "$OPENAI_ORG_ID"` working cleanly when the
var isn't set. Applies to MCP `headers` and provider `extra_headers`.

### Security note

`crush.json` is trusted code. Any `$(...)` in it runs at load time
with the invoking user's shell privileges, before the UI appears.
Don't launch Crush in a directory whose `crush.json` you haven't
reviewed.

## Common Tasks

- Add a custom provider: add an entry under `providers` with `type`, `base_url`, `api_key`, and `models`.
- Disable a builtin or local skill: add the skill name to `options.disabled_skills`.
- Add an MCP server: add an entry under `mcp` with `type` and either `command` (stdio) or `url` (http/sse).
- Hide a tool from the agent: add its name to `options.disabled_tools`.

## Model Selection

```json
{
  "models": {
    "large": {
      "model": "claude-sonnet-4-20250514",
      "provider": "anthropic",
      "max_tokens": 16384
    },
    "small": {
      "model": "claude-haiku-4-20250514",
      "provider": "anthropic"
    }
  }
}
```

- `large` is the primary coding model; `small` is for summarization.
- Only `model` and `provider` are required.
- Optional tuning: `reasoning_effort`, `think`, `max_tokens`, `temperature`, `top_p`, `top_k`, `frequency_penalty`, `presence_penalty`, `provider_options`.

### Tier-Based Model Routing

Route different token counts to different model tiers:

```jsonc
{
  "options": {
    "router_tiers": [
      { "up_to_tokens": 1000, "model_type": "small" },
      { "up_to_tokens": 100000, "model_type": "large" }
    ]
  }
}
```

Tiers are evaluated in order; first match wins. Without `router_tiers`, the
session model is used for all requests.

## Custom Providers

```json
{
  "providers": {
    "deepseek": {
      "type": "openai-compat",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "$DEEPSEEK_API_KEY",
      "models": [
        {
          "id": "deepseek-chat",
          "name": "Deepseek V3",
          "context_window": 64000
        }
      ]
    }
  }
}
```

- `type` (required): `openai`, `openai-compat`, or `anthropic`
- `api_key`, `base_url`, `api_endpoint`, and `extra_headers` are shell-expanded (see [Shell Expansion](#shell-expansion)).
- `extra_body` is a JSON passthrough and is **not** expanded.
- Additional fields: `disable`, `system_prompt_prefix`, `extra_headers`, `extra_body`, `provider_options`.

## LSP Configuration

```json
{
  "lsp": {
    "go": {
      "command": "gopls",
      "env": { "GOPATH": "$HOME/go" }
    },
    "typescript": {
      "command": "typescript-language-server",
      "args": ["--stdio"]
    }
  }
}
```

- `command` (required), `args`, `env` cover most setups.
- `command`, `args`, and `env` values are shell-expanded (see [Shell Expansion](#shell-expansion)).
- Additional fields: `disabled`, `filetypes`, `root_markers`, `init_options`, `options`, `timeout`.

### Extended LSP Options

```jsonc
{
  "lsp": {
    "go": {
      "command": "gopls",
      "args": [],
      "env": {},
      "filetypes": [".go"],
      "match_patterns": ["*.go", "go.mod"],
      "root_markers": ["go.mod"],
      "init_options": {},
      "options": {},
      "timeout": 30,
      "disabled": false,
      "auto_download": {
        "url": "https://example.com/gopls-linux-amd64",
        "sha256": "abc123..."
      }
    }
  },
  "options": {
    "auto_lsp": true,
    "debug_lsp": false
  }
}
```

- `match_patterns`: glob patterns for flexible file-to-server routing.
- `auto_download`: on-demand LSP server binary download with SHA256
  verification. An embedded catalog provides zero-config auto-download for
  ~15 servers when no user config exists.
- `auto_lsp` (default `true`): auto-start LSP servers when relevant files
  are opened.
- `debug_lsp` (default `false`): enable LSP debug logging.
- Crash recovery is always-on (exponential backoff 1s–60s, up to 5 retries).

## MCP Servers

```json
{
  "mcp": {
    "filesystem": {
      "type": "stdio",
      "command": "node",
      "args": ["/path/to/mcp-server.js"]
    },
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/",
      "headers": {
        "Authorization": "Bearer $GH_PAT"
      }
    }
  }
}
```

- `type` (required): `stdio`, `sse`, or `http`
- `command`, `args`, `env`, `headers`, and `url` are shell-expanded (see [Shell Expansion](#shell-expansion)).
- Additional fields: `env`, `disabled`, `disabled_tools`, `timeout`.

## Options

```json
{
  "options": {
    "skills_paths": ["./skills"],
    "disabled_tools": ["bash", "sourcegraph"],
    "disabled_skills": ["crush-config"],
    "beta_tools": false,
    "tui": {
      "compact_mode": false,
      "diff_mode": "unified",
      "transparent": false
    },
    "auto_lsp": true,
    "debug": false,
    "debug_lsp": false,
    "attribution": {
      "trailer_style": "assisted-by",
      "generated_with": true
    }
  }
}
```

> [!IMPORTANT]
> The following skill paths are loaded by default and DO NOT NEED to be added to `skills_paths`:
> `.agents/skills`, `.crush/skills`, `.claude/skills`, `.cursor/skills`

Other options: `context_paths`, `progress`, `disable_notifications`, `disable_auto_summarize`, `disable_metrics`, `disable_provider_auto_update`, `disable_default_providers`, `data_directory`, `initialize_as`.

### `disabled_tools`

Array of tool names to completely hide from the agent. Hidden tools are not
registered in the tool surface and cannot be called.

### `beta_tools`

Set to `true` to enable tools marked as beta. Beta tools are hidden from the
agent's tool list unless explicitly enabled.

## Lossless Context Management (LCM)

LCM is always active. It automatically compacts conversation history when the
token budget is approached. The TUI shows a "Compacting" pill during the
process. Extracted insights persist across sessions in `CRUSH.memory.md`.

```jsonc
{
  "options": {
    "lcm": {
      // Soft threshold ratio for triggering compaction (default: 0.6 = 60%).
      "ctx_cutoff_threshold": 0.6,

      // Model used for summarization (defaults to session model).
      "summarizer_model": null,

      // Disable storing large tool output in separate table.
      "disable_large_tool_output": false,

      // Token count above which tool output is offloaded (default: 10000).
      "large_tool_output_token_threshold": 10000,

      // Explorer profile: "enhancement" (structured + LLM) or "parity" (structured only).
      "explorer_output_profile": "enhancement",

      // Enable operational memory for persistent cross-turn state.
      "operational_memory_enabled": false,

      // Observation settings.
      "observation": {
        "strategy": "default",      // "default" or "resource-scoped"
        "token_budget": 2000
      },

      // Proactive context management hints to the agent.
      "nudge": {
        "min_context_limit": 0,
        "max_context_limit": 0,
        "nudge_frequency": 5,       // turns between nudges (default 5)
        "nudge_force": "soft"       // "soft" or "hard"
      }
    }
  }
}
```

## Repository Map

The repository map provides a compressed project outline in every conversation,
personalized to the current chat via PageRank. It is enabled by default.

```jsonc
{
  "options": {
    "repo_map": {
      // Disable repository map generation entirely.
      "disabled": false,

      // Maximum tokens for the map output (default: dynamic based on context window).
      "max_tokens": 4096,

      // Glob patterns to exclude from the map.
      "exclude_globs": ["vendor/**", "node_modules/**"],

      // Refresh mode: "auto", "files", "manual", "always".
      "refresh_mode": "auto",

      // Multiplier when no files are open (default: 2.0).
      "map_mul_no_files": 2.0,

      // Parser pool size for tree-sitter (0 = NumCPU).
      "parser_pool_size": 0
    }
  }
}
```

Merge rules for `tools.repo_map` vs `options.repo_map`: `disabled` uses
OR-latch, `exclude_globs` accumulate, scalar fields use last-wins.

## Processor Pipeline

An optional message interception layer with four phases. Three processors are
active by default; the rest must be explicitly enabled.

```jsonc
{
  "options": {
    "processors": {
      // Master switch (default: active with 3 default processors).
      "enabled": true,

      // Ordered list of processor names to activate.
      "list": [
        "TokenLimiter",
        "SystemPromptScrubber",
        "PIIDetector"
      ],

      // Per-processor configuration.
      "config": {
        "pii_detector": {
          "sensitivity": "medium"    // "low", "medium", "high"
        },
        "message_selection": {
          "max_messages": 50,
          "strategy": "recency"      // "recency" or "relevance"
        },
        "tool_call_filter": {
          "allow_list": ["view", "edit", "bash"],  // mutually exclusive with deny_list
          "deny_list": ["web_search"]
        }
      }
    }
  }
}
```

Configurable processors (15): `TokenLimiter`, `SystemPromptScrubber`,
`PIIDetector`, `UnicodeNormalizer`, `BatchParts`, `MessageSelection`,
`ToolCallFilter`, `ToolSearch`, `Skills`, `SkillSearch`,
`ModerationProcessor`, `PromptInjectionDetector`, `LanguageDetector`,
`WorkspaceInstructions`, `MessageHistory`.

The `skills` processor must precede `skill_search` in the list.

## Validation & AutoFix

Post-edit validation and iterative auto-fix for changed files.

```jsonc
{
  "options": {
    "validation": {
      // Enable post-edit validation (syntax checking). Default: false.
      "enabled": false,

      // Automatically attempt to fix validation errors. Default: false.
      "auto_fix": false,

      // Enable the iterative autofix loop (lint -> fix -> test -> reflect).
      // Requires both "enabled" and "auto_fix" to be true. Default: false.
      "autofix_loop_enabled": false
    }
  }
}
```

All three default to `false`. The autofix loop requires both `enabled` and
`auto_fix` to be `true` before it activates.

## Snapshot & Rewind

Turn-based snapshot and rewind for code and conversation state.

```jsonc
{
  "options": {
    // Enable rewind/snapshot system.
    // {} = enabled with defaults. null or omitted = disabled.
    "snapshot": {
      "max_per_session": 50   // default: 50
    }
  }
}
```

When enabled, press `o` on any user message in the TUI to rewind code,
conversation, both, edit & resubmit, or fork from that point.

## Doom Loop Detection

Detects repetitive tool-call cycles with three escalation tiers.

```jsonc
{
  "options": {
    // "warn" = warnings + forced tool switches (default).
    // "full" = aggressive: warnings + forced switches + termination.
    // "none" = disable entirely.
    "doom_loop_intervention": "warn"
  }
}
```

## Architect Planning

Structured planning before complex code changes.

```jsonc
{
  "options": {
    "architect": {
      // Require user approval before executing the plan. Default: false.
      "approval_required": false
    }
  }
}
```

The architect activates automatically when the agent's heuristic classifier
determines a prompt warrants planning. Requires `architect_model` config key
to be set.

## User-Invocable Skills

Skills can be made invocable as commands from the commands palette. Add `user-invocable: true` to the skill's YAML frontmatter:

```yaml
---
name: my-skill
description: A skill that can be invoked as a command.
user-invocable: true
---
```

User-invocable skills appear in the commands palette with a prefix:
- Skills from global directories: `user:skill-name`
- Skills from project directories: `project:skill-name`

When invoked, the skill's instructions are loaded into the conversation context.

To prevent the model from auto-triggering a skill (while still allowing user invocation), add `disable-model-invocation: true`:

```yaml
---
name: my-skill
description: Only invocable by users, not the model.
user-invocable: true
disable-model-invocation: true
---
```

Skills with `disable-model-invocation` won't appear in the model's available skills list but can still be invoked manually by users.

## Hooks

Hooks are user-defined shell commands that fire on agent events. Four events
are supported:

| Event | Description |
|---|---|
| `PreToolUse` | Fires before every tool call. Can block, allow, or rewrite tool input. |
| `PostToolUse` | Fires after every tool call. Non-blocking; for output rewriting/sanitization. |
| `PreCompact` | Fires before LCM compaction. |
| `PostCompact` | Fires after LCM compaction. |

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "^(edit|write|multiedit)$",
        "command": ".crush/hooks/protect-files.sh"
      },
      {
        "matcher": "^bash$",
        "command": ".crush/hooks/no-haskell.sh"
      }
    ],
    "PostToolUse": [
      {
        "matcher": "bash",
        "command": "./hooks/sanitize-output.sh"
      }
    ],
    "PreCompact": [],
    "PostCompact": []
  }
}
```

### Hook Properties

- `command` (required): Shell command to execute. Runs via `sh -c`.
- `matcher` (optional): Regex pattern tested against the tool name. Empty or absent means match all tools.
- `timeout` (optional): Timeout in seconds. Defaults to 30.

### Event Name Normalization

Event names are case-insensitive and accept snake_case variants: `PreToolUse`, `pretooluse`, `pre_tool_use`, and `PRE_TOOL_USE` all work.

### How Hooks Work

1. When a tool is about to be called, all `PreToolUse` hooks with a matching `matcher` (or no matcher) run in parallel.
2. Duplicate commands are deduplicated — each unique command runs at most once.
3. The hook receives JSON on **stdin** and hook-specific **environment variables**.
4. `PostToolUse` hooks run after tool completion. Exit codes 2 and 49 are **non-blocking** (the tool already ran). The primary use is output rewriting via `modified_output`.

### Hook Input (stdin)

**PreToolUse payload:**

```json
{
  "event": "PreToolUse",
  "session_id": "abc-123",
  "cwd": "/path/to/project",
  "tool_name": "bash",
  "tool_input": {"command": "ls -la"}
}
```

**PostToolUse payload** (extends base with tool output):

```json
{
  "event": "PostToolUse",
  "session_id": "abc-123",
  "cwd": "/path/to/project",
  "tool_name": "bash",
  "tool_input": {"command": "ls -la"},
  "tool_output": "file1.txt\nfile2.txt",
  "duration_ms": 150
}
```

### Hook Environment Variables

| Variable | Description |
|---|---|
| `CRUSH_EVENT` | Event name (e.g. `PreToolUse`) |
| `CRUSH_TOOL_NAME` | Name of the tool being called |
| `CRUSH_SESSION_ID` | Current session ID |
| `CRUSH_CWD` | Current working directory |
| `CRUSH_PROJECT_DIR` | Project root directory |
| `CRUSH_TOOL_INPUT_COMMAND` | Value of `command` from tool input (if present) |
| `CRUSH_TOOL_INPUT_FILE_PATH` | Value of `file_path` from tool input (if present) |

PostToolUse adds:

| Variable | Description |
|---|---|
| `CRUSH_TOOL_OUTPUT` | The tool's output text |
| `CRUSH_TOOL_DURATION_MS` | Execution time in milliseconds |

### Hook Output

**Exit code 0** — the hook succeeded. Stdout is parsed as JSON:

```json
{"decision": "allow", "context": "optional context appended to tool result"}
```

- `decision`: `allow` to explicitly allow, `deny` to block, `none` (or omit) for no opinion.
- `reason`: Explanation text (used when denying).
- `context`: Extra context appended to the tool result. String or array of strings.
- `updated_input`: Shallow-merge patch against `tool_input`. Keys you include overwrite; keys you omit are preserved. Nested objects replaced wholesale. Ignored on deny/halt.

**Exit code 2** — the tool call is blocked (PreToolUse) or non-blocking warning (PostToolUse). Stderr is used as the deny reason.

**Exit code 49** — halt the whole turn. Ends the current agent turn immediately.

```bash
echo "No Haskell allowed" >&2
exit 2
```

**Any other exit code** — non-blocking error. The tool call proceeds as normal.

### PostToolUse Output Rewriting

PostToolUse hooks can replace tool output via `modified_output`:

```json
{"modified_output": "sanitized output here"}
```

- If stdout is valid JSON with a `modified_output` field, that text replaces the tool's original output.
- If stdout is valid JSON without the field, no replacement occurs.
- If stdout is not valid JSON, it is used as-is (plain text replacement).

### Claude Code Compatibility

Crush also supports the Claude Code hook output format:

```json
{
  "hookSpecificOutput": {
    "permissionDecision": "allow",
    "permissionDecisionReason": "Auto-approved",
    "updatedInput": {"command": "echo rewritten"}
  }
}
```

Existing Claude Code hooks should work without modification. One divergence:
Crush treats `updated_input` as shallow-merge; Claude Code replaces wholesale.

### Decision Aggregation

When multiple hooks match, their decisions are aggregated:

- **Deny wins over allow** — if any hook denies, the tool call is blocked.
- **Allow wins over none** — if no hook denies but at least one allows, the call proceeds.
- **Halt is sticky** — any hook halting ends the turn.
- All deny reasons are concatenated (newline-separated).
- All context strings are concatenated (newline-separated).
- For `updated_input`, patches shallow-merge sequentially; later patches win on colliding keys.
- For `modified_output` (PostToolUse), last-writer-wins semantics.

## Tool Permissions

```json
{
  "permissions": {
    "allowed_tools": ["view", "ls", "grep", "edit"]
  }
}
```

Tools listed in `allowed_tools` can run without user confirmation.

## Environment Variables

- `CRUSH_GLOBAL_CONFIG` - Override global config location
- `CRUSH_GLOBAL_DATA` - Override data directory location
- `CRUSH_SKILLS_DIR` - Override default skills directory
- `CRUSH_CORE_UTILS` - Force Go-based core utilities (`true`/`false`). Default: `true` on Windows, `false` elsewhere.
