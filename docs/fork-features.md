# Fork Feature Configuration

This document covers configuration and usage for features added by this fork
beyond upstream Crush. All configuration lives in `crush.json` (project-level
or global `~/.config/crush/crush.json`).

## Table of Contents

- [Lossless Context Management (LCM)](#lossless-context-management-lcm)
- [Repository Map](#repository-map)
- [Model Routing](#model-routing)
- [Validation Pipeline](#validation-pipeline)
- [Architect Planning](#architect-planning)
- [Doom Loop Intervention](#doom-loop-intervention)
- [Processor Pipeline](#processor-pipeline)
- [Snapshots and Rewind](#snapshots-and-rewind)
- [Agent Configuration](#agent-configuration)
- [Auto-Memory](#auto-memory)
- [Evaluation CLI](#evaluation-cli)

## Lossless Context Management (LCM)

LCM keeps conversations within token budgets through an 8-layer compaction
pipeline. When enabled, it automatically summarizes old messages, stores
large tool outputs out-of-line, and injects context-limit nudges.

Enable by adding an `lcm` key to `crush.json`:

```json
{
  "lcm": {}
}
```

This activates LCM with defaults. To customize:

```json
{
  "lcm": {
    "ctx_cutoff_threshold": 0.6,
    "large_tool_output_token_threshold": 10000,
    "explorer_output_profile": "enhancement",
    "operational_memory_enabled": false,
    "summarizer_model": {
      "model": "gpt-4o-mini",
      "provider": "openai"
    },
    "observation": {
      "strategy": "default"
    },
    "nudge": {
      "min_context_limit": 50000,
      "max_context_limit": 100000,
      "nudge_frequency": 5,
      "nudge_force": "soft"
    }
  }
}
```

### LCM Options

| Field | Type | Default | Description |
|---|---|---|---|
| `ctx_cutoff_threshold` | float | `0.6` | Fraction of context window at which soft compaction triggers (0.6 = 60%) |
| `summarizer_model` | object | _large model_ | Dedicated model for LCM summarization calls. Must have a context window at least as large as the large model, otherwise ignored |
| `disable_large_tool_output` | bool | `false` | Disable automatic storage of large tool outputs in LCM |
| `large_tool_output_token_threshold` | int | `10000` | Token count above which tool output is stored in LCM instead of inline |
| `explorer_output_profile` | string | `"enhancement"` | Formatter profile for exploration summaries: `"enhancement"` or `"parity"` |
| `operational_memory_enabled` | bool | `false` | Persist extracted observations across sessions via LCM lifecycle hooks |
| `observation.strategy` | string | `"default"` | Observation strategy: `"default"` (always observe) or `"resource-scoped"` (skip under memory pressure) |
| `nudge.min_context_limit` | int | `50000` | Minimum context tokens below which nudges are never injected |
| `nudge.max_context_limit` | int | `100000` | Context tokens above which nudges inject when pressure is high |
| `nudge.nudge_frequency` | int | `5` | Inject a nudge every N turns |
| `nudge.nudge_force` | string | `"soft"` | Nudge intensity: `"soft"` or `"hard"` |

### LCM Tools

When LCM is active, three tools become available to the agent:

- **`lcm_describe`** — Describe a file or summary by its LCM identifier.
  Returns content preview and metadata.
- **`lcm_expand`** — Expand an LCM summary to its original messages.
- **`lcm_grep`** — Search conversation history with full-text or regex
  search.

## Repository Map

Generates scope-aware code outlines ranked by PageRank, giving the LLM
project-wide understanding without reading every file. Uses tree-sitter for
tag extraction and import analysis.

Configured under the `repo_map` key:

```json
{
  "repo_map": {
    "disabled": false,
    "max_tokens": 0,
    "refresh_mode": "auto",
    "exclude_globs": ["vendor/**", "node_modules/**"],
    "map_mul_no_files": 2.0,
    "parser_pool_size": 0
  }
}
```

### Repo Map Options

| Field | Type | Default | Description |
|---|---|---|---|
| `disabled` | bool | `false` | Disable repo map generation entirely |
| `max_tokens` | int | _dynamic_ | Override token budget for rendered map. Zero uses `min(max(contextWindow/8, 1024), 4096)`, or `8192` when LCM is active |
| `exclude_globs` | string[] | `[]` | Additional glob patterns to exclude from scanning |
| `refresh_mode` | string | `"auto"` | When to regenerate: `"auto"`, `"files"`, `"manual"`, or `"always"` |
| `map_mul_no_files` | float | `2.0` | Budget multiplier when no files are in chat |
| `parser_pool_size` | int | _runtime default_ | Tree-sitter parser pool capacity |

## Model Routing

Routes LLM requests to different models based on input size. This replaces
the binary small/large model split with configurable tiers.

### Binary Routing (simple)

```json
{
  "router_token_limit": 4000,
  "editor_model": {
    "model": "gpt-4o-mini",
    "provider": "openai"
  },
  "architect_model": {
    "model": "gpt-4o",
    "provider": "openai"
  }
}
```

| Field | Type | Description |
|---|---|---|
| `router_token_limit` | int | Token count threshold. Inputs at or below this go to the editor model |
| `editor_model` | object | Model override for editor/coding calls (defaults to small model) |
| `architect_model` | object | Model override for architect/planning calls (defaults to large model) |

### Multi-Tier Routing (advanced)

```json
{
  "router_tiers": [
    { "up_to_tokens": 2000, "model_type": "small" },
    { "up_to_tokens": 8000, "model_type": "large" },
    { "up_to_tokens": 100000, "model_type": "large" }
  ]
}
```

Tiers are sorted ascending by `up_to_tokens`. The first matching tier wins.
`model_type` is `"small"` or `"large"`, referring to the corresponding
provider model configuration.

## Validation Pipeline

Post-edit validation using tree-sitter parsing and LSP diagnostics.

```json
{
  "validation": {
    "enabled": false,
    "auto_fix": false,
    "autofix_loop_enabled": false
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable post-edit validation pipeline |
| `auto_fix` | bool | `false` | Automatically attempt fixes when validation fails |
| `autofix_loop_enabled` | bool | `false` | Enable post-turn auto-fix quality cycle |

When enabled, the agent runs diagnostics after edits and can automatically
attempt fixes in a lint → fix → test → reflect loop with rollback on
failure.

## Architect Planning

Controls the two-phase architect → editor planning flow. The architect
creates a structured plan; the editor executes it.

```json
{
  "architect": {
    "approval_required": false
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `approval_required` | bool | `false` | Require user approval before executing architect plans |

## Doom Loop Intervention

Detects when the agent is stuck in a loop (repeated failures with no
progress) and escalates intervention.

```json
{
  "doom_loop_intervention": "warn"
}
```

| Value | Behavior |
|---|---|
| `"none"` | Log message only, no intervention |
| `"warn"` | Soft intervention (default) |
| `"full"` | Soft + medium intervention, may restart with simplified prompt |

## Processor Pipeline

Message intercept pipeline that processes LLM input and output across four
sequential phases: input, output stream, output result, and API error.

```json
{
  "processors": {
    "enabled": true,
    "list": ["token_limiter", "system_prompt_scrubber", "pii_detector"]
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Enable the processor pipeline |
| `list` | string[] | `[]` | Processor names to enable. Safe defaults: `token_limiter`, `system_prompt_scrubber`, `pii_detector` |

## Snapshots and Rewind

After each agent turn, files are snapshotted so you can rewind to any
previous state.

```json
{
  "snapshot": {
    "max_per_session": 50
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `max_per_session` | int | `50` | Maximum snapshots retained per session. Older ones are cleaned up |

## Agent Configuration

Per-agent configuration overrides can be placed in YAML files under
`.crush/agents/project/` in your project root. These override the built-in
defaults for the `coder` and `task` agents, or define entirely new agents.

Example `.crush/agents/project/coder.yaml`:

```yaml
name: coder
tools:
  - bash
  - edit
  - view
  - grep
  - glob
  - ls
permissions:
  - bash:allow
  - edit:allow
max_tokens: 8192
max_steps: 50
max_turns: 100
model: gpt-4o
perm_mode: auto
system_prompt: |
  You are a senior Go developer.
environment:
  GOFLAGS: "-mod=readonly"
allowed_mcp:
  firecrawl:
    - firecrawl_scrape
    - firecrawl_search
```

### Agent Config Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | _agent name_ | Agent identifier |
| `agent_type` | string | _agent name_ | Internal type (usually same as name) |
| `tools` | string[] | _all tools_ | Allowed tool names. Empty = all tools |
| `permissions` | string[] | _none_ | Permission grants (e.g., `"bash:allow"`, `"edit:allow"`) |
| `max_tokens` | int | `4096` | Maximum output token budget. Zero = unlimited |
| `max_steps` | int | `25` | Maximum tool-use steps. Zero = unlimited |
| `max_turns` | int | `0` | Maximum conversation turns. Zero = unlimited |
| `model` | string | _default_ | Model identifier override |
| `perm_mode` | string | _default_ | Permission mode: `"ask"` or `"auto"` |
| `system_prompt` | string | _template_ | Custom system prompt template |
| `environment` | map | `{}` | Environment variables for the agent subprocess |
| `allowed_mcp` | map | _all_ | MCP server → allowed tool names. Empty array = all tools for that server |

Precedence: runtime overrides > project YAML > built-in defaults.

## Auto-Memory

When LCM is active with `operational_memory_enabled: true`, the agent
persists insights to `CRUSH.memory.md` in the project root. This file is
automatically loaded as context in future sessions, carrying learned project
knowledge forward.

## Evaluation CLI

The `crush eval` command runs evaluation scorers to measure agent output
quality.

```bash
# List available scorers
crush eval

# Run a specific scorer
crush eval --scorer build --dataset ./eval-dataset.json

# Run with output report
crush eval --scorer quality --dataset ./eval-dataset.json --output report.json
```

### Flags

| Flag | Description |
|---|---|
| `--dataset <path>` | Path to JSON dataset file (required when running a scorer) |
| `--scorer <name>` | Scorer to run |
| `--input <path>` | Input file or directory (default: `.`) |
| `--output <path>` | Output report file path (JSON format) |

### Available Scorers

**LLM Judge Scorers** (require an LLM client):

| Scorer | Assesses |
|---|---|
| `code_quality` | Overall code quality |
| `correctness` | Logical correctness of implementation |
| `completeness` | Whether requirements are fully addressed |
| `clarity` | Code clarity and readability |
| `safety` | Security and safety concerns |
| `performance` | Performance characteristics |
| `maintainability` | Long-term maintainability |
| `error_handling` | Error handling coverage |
| `documentation` | Documentation quality |
| `conventions` | Adherence to language conventions |
| `testing_quality` | Test coverage and quality |
| `edge_cases` | Edge case handling |

**Metric Scorers** (deterministic, no LLM needed):

| Scorer | Assesses |
|---|---|
| `build_success` | Whether the project builds successfully |
| `test_pass_rate` | Percentage of passing tests |
| `syntax_validity` | Syntax correctness |
| `lint_score` | Static analysis score |
| `edit_distance` | Minimal edit distance |
| `coverage_score` | Code coverage percentage |
| `type_check_score` | Type checking passes |
