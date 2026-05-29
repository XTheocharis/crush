# XRush Fork Feature Documentation

> **Fork**: `XTheocharis/crush` (parent: `charmbracelet/crush`)
> **Divergence**: `d14f3b1b` ("feat(tools): add diff view for denied tools", May 13, 2026)
> **Fork activity**: 41 commits, May 26–28, 2026 (~50.5 hours)
> **Scale**: 742 files changed, 244,900 insertions, 386 deletions
> **Commit types**: feat: 20, fix: 11, refactor: 2, build: 2, chore: 2, docs: 2, style: 1, test: 1

---

## Table of Contents

- [1. Lossless Context Management (LCM)](#1-lossless-context-management-lcm)
- [2. File-Type Explorer](#2-file-type-explorer)
- [3. Tree-sitter Integration](#3-tree-sitter-integration)
- [4. Repository Map](#4-repository-map)
- [5. Processor Pipeline](#5-processor-pipeline)
- [6. Multi-Agent Orchestration](#6-multi-agent-orchestration)
- [7. Extension Host](#7-extension-host)
- [8. Rewind System](#8-rewind-system)
- [9. Eval Framework](#9-eval-framework)
- [10. Doom Loop Detection & Resource Limits](#10-doom-loop-detection--resource-limits)
- [11. AutoFix Loop & Go Linter](#11-autofix-loop--go-linter)
- [12. Architect Planning](#12-architect-planning)
- [13. LSP Enhancements](#13-lsp-enhancements)
- [14. Tools](#14-tools)
- [15. Config Enhancements](#15-config-enhancements)
- [16. Message Timestamps](#16-message-timestamps)
- [17. Database Migrations](#17-database-migrations)

---

## 1. Lossless Context Management (LCM)

**Location**: `internal/lcm/`

LCM is a conversation context management system that summarizes, compacts, and
retrieves conversation history to stay within token budgets while preserving
critical information.

### Manager

The `Manager` (`manager.go`, 1,633 lines) exposes 57 methods and 8 standalone functions (65 total) organized into
functional groups:

- **Compaction**: trigger compaction, query compaction state
- **Budget**: token budget tracking and enforcement
- **Lifecycle**: initialize, start, stop, shutdown
- **LLM Client**: configured LLM for summarization
- **Agent Integration**: hooks for agent turn boundaries
- **Post-Hooks**: callbacks after compaction or retrieval
- **Token Tracking**: per-turn and cumulative token counts
- **Cross-Subsystem**: integration with repomap, explorer, extension host
- **Retrieval**: search and expand compacted content
- **Observation/Reflection/Memory**: observation priority tagging, reflection
  triggers, auto-memory extraction
- **Content Replacement**: reversible content substitution
- **Hook/Config**: configuration hot-reload, hook registration
- **Events**: pub/sub events for state changes

### Compaction Pipeline

8-stage pipeline processes conversation content in priority order:

| Priority | Layer | Description |
|----------|-------|-------------|
| P1 | `MicroCompactor` | Stores large content (tool output, file contents) in `lcm_large_files` table, replaces with compact references |
| P2 | `DedupCompactionLayer` | SHA-256 deduplication of repeated content blocks |
| P3 | `StaleEvictionLayer` | Evicts tool output older than 30 minutes |
| P4 | `post-compact-cleaner` | Cleans orphaned references after compaction |
| P5 | `AdjacentCondensationLayer` | Merges adjacent summary blocks into unified summaries |
| P5b | `pressure-compaction-selector` | Selects compaction strategy based on memory pressure |
| P6–P7 | `CacheOptimizer` | P6: `compact-prompt-structure` (9-section prompt assembly); P7: `anthropic-cache-management` (Anthropic prefix cache optimization) |

### Observation System

Observations carry priority tags (`high`, `medium`, `low`) and are bridged
into agent prompts with a dedicated token budget. High-priority observations
persist across compaction cycles.

### Memory System

**Location**: `internal/lcm/memory.go` (792 lines)

- `AutoMemoryExtractor` fires every 5 turns
- Extracts 4 memory types: **fact**, **decision**, **preference**, **lesson**
- TF-IDF ranking for relevance scoring
- ~60 KB per-session memory budget (`MemorySessionMaxChars=60000`;
  `MemoryFileConfig.SessionBudget=61440` default exists but is unenforced)
- Persists across sessions to `CRUSH.memory.md`

### Content Replacement

Reversible content substitutions with state machine tracking. Supports
replacing verbose content with compact references and restoring original
content on demand.

### Store

**Location**: `internal/lcm/store.go`

- FTS5 full-text search tables: `lcm_summaries_fts` (bm25 ranking + snippet
  generation), `lcm_large_files_fts`
- Session ancestry access for cross-session context
- Recursive CTE expansion for nested compaction chains
- SQLite-backed persistence

### LCM Agent Tools (14)

3 LCM tools via tool factory (registered in standard tool surface):

| Tool | Description |
|------|-------------|
| `lcm_grep` | Search conversation history (full-text and regex) |
| `lcm_describe` | Describe a file or summary by LCM identifier |
| `lcm_expand` | Expand an LCM summary to its original messages |

11 tools via `ExtraAgentTools()` (injected directly into the coder agent):

| Tool | Description |
|------|-------------|
| `lcm_bindle` | Bundle and retrieve compacted conversation bundles |
| `lcm_ancestry` | Trace session ancestry for cross-session context |
| `lcm_dolt` | Query conversation history with diff semantics |
| `lcm_archive` | Archive and retrieve conversation snapshots |
| `lcm_sprig` | Retrieve targeted conversation segments |
| `lcm_time_query` | Time-based conversation history queries |
| `lcm_file_search` | Search files referenced in conversation history |
| `lcm_active_context` | Show currently active LCM context window |
| `lcm_lineage` | Trace compaction lineage for a content block |
| `llm_map` | Apply LLM transformation per JSONL item (read-only) |
| `agentic_map` | Run sub-agent on each JSONL item, write results |

### User-Facing Description

LCM keeps conversations from running out of memory. As your chat grows toward
the model's context window limit, LCM automatically triggers an 8-layer
compaction pipeline that summarizes older content, stores large outputs
separately, and evicts stale data -- all without losing critical information.
Extracted insights persist across sessions in `CRUSH.memory.md`. When
compaction is running, the TUI displays a "Compacting" pill indicator so
you always know when context management is active.

### Configuration

```jsonc
{
  "options": {
    "lcm": {
      // Soft threshold ratio for triggering compaction (default: 0.6 = 60%)
      "ctx_cutoff_threshold": 0.6,

      // Model used for summarization (defaults to session model)
      "summarizer_model": null,

      // Disable storing large tool output in separate table
      "disable_large_tool_output": false,

      // Token count above which tool output is offloaded (default: 10000)
      "large_tool_output_token_threshold": 10000,

      // Explorer profile: "enhancement" (structured + LLM) or "parity" (structured only)
      "explorer_output_profile": "enhancement",

      // Enable operational memory for persistent cross-turn state
      "operational_memory_enabled": false,

      // Observation settings: what the agent notices and remembers
      "observation": {
        "strategy": "default",      // "default" or "resource-scoped"
        "token_budget": 2000        // token budget for observation content
      },

      // Nudge: proactive context management hints to the agent
      "nudge": {
        "min_context_limit": 0,     // minimum context usage to start nudging
        "max_context_limit": 0,     // maximum context before forced nudge
        "nudge_frequency": 5,       // how often to nudge (in turns, default 5)
        "nudge_force": "soft"       // "soft" (nudge) or "hard" (force), default "soft"
      }
    }
  }
}
```

Token budget formula:

```
overhead = systemPrompt + tools + repoMap + 200
outputReserve = min(20000, contextWindow * 0.25, modelOutputLimit)
hardLimit = contextWindow - overhead - outputReserve
softThreshold = clamp(contextWindow * cutoff - overhead, 0, hardLimit)
```

### Usage

LCM is always active and requires no CLI commands. When the conversation
approaches the soft threshold, compaction begins automatically. The TUI shows
a "Compacting" pill during the process. The `CRUSH.memory.md` file is written
to your project root and can be edited by hand to adjust persisted memories.
The agent uses LCM tools (`lcm_grep`, `lcm_expand`, `lcm_describe`, etc.)
automatically when it needs to search or retrieve prior conversation content.

### Default Behavior

LCM auto-activates on every session. The default soft threshold is 60% of the
effective context window. Auto-memory extraction runs every 5 turns, extracting
facts, decisions, preferences, and lessons learned. The default explorer profile
is "enhancement", which combines structured extraction with LLM-powered analysis.
No configuration is required for LCM to function.

---

## 2. File-Type Explorer

**Location**: `internal/lcm/explorer/`

A dispatch-based file analysis system that provides intelligent content
extraction for 20+ file types.

### Architecture

Three-tier dispatch per file:

1. **Static** (always runs): Structured extraction without LLM
2. **LLM** (optional): LLM-powered analysis, 50 KB char truncation
3. **Agent** (optional): Language-specific agent prompts

### File-Type Handlers

Handlers are registered in priority order. The first handler that accepts a
file type processes it:

| Handler | File Types | Lines |
|---------|-----------|-------|
| `Archive` | zip, tar, gz, bz2, xz, 7z, rar | 955 |
| `PDF` | .pdf | 160 |
| `Image` | png, jpg, gif, svg, webp, bmp, ico | 441 |
| `Executable` | ELF, Mach-O, PE binaries | 630 |
| `Binary` | Generic binary files | 169 |
| `JSON` | .json, .jsonl | (in Data) |
| `CSV` | .csv, .tsv | (in Data) |
| `YAML` | .yaml, .yml | (in Data) |
| `TOML` | .toml | (in Data) |
| `INI` | .ini, .cfg, .conf | (in Data) |
| `XML` | .xml, .xsl, .xsd | (in Data) |
| `HTML` | .html, .htm | (in Data) |
| `Markdown` | .md, .mdx | 420 |
| `LaTeX` | .tex, .latex | 456 |
| `SQLite` | .db, .sqlite, .sqlite3 | 627 |
| `Logs` | .log, structured logs | 724 |
| `TreeSitter` | 38 programming languages (see §3) | 161 |
| `Shell` | .sh, .bash, .zsh | 80 |
| `Text` | .txt, .rst, .adoc | (fallback) |
| `Fallback` | Any unrecognized file | (fallback) |

### Supporting Components

| Component | Lines | Description |
|-----------|-------|-------------|
| `Formatter` | 188 | Output formatting for explorer results |
| `Heuristic` | 503 | File-type detection heuristics |
| `FileStructure` | 167 | Directory structure analysis |
| `LLMExplorer` | 165 | LLM-based fallback exploration |
| `RuntimeInventory` | 692 | Runtime dependency detection |
| `ParityFixtures` | — | Test fixture SHA-256 checksums for cross-platform consistency |
| `ParityProvenance` | — | Track provenance of ported code for attribution |
| `Conformance` | 300 | Validation of handler output format |

### Language Stdlib Mappings

14 language-specific stdlib membership checkers in `stdlib/`:

C, C++, C#, Go, Haskell, Java, Kotlin, Node.js, PHP, Python, Ruby, Rust,
Scala, Swift (plus `common.go` for shared logic).

### User-Facing Description

The Explorer transparently analyzes files viewed by the agent across 20+ file
types. It uses a three-tier dispatch system: static structured extraction
(always), LLM-powered analysis (for content exceeding ~10K tokens), and
agent-based deep exploration (for complex code). Python files skip the LLM
tier and go directly to agent-based analysis for better accuracy. Users never
invoke the Explorer directly -- it activates automatically when the agent
uses the `view` tool or processes large tool output.

### Configuration

```jsonc
{
  "options": {
    "lcm": {
      // Controls how much analysis the explorer performs per file.
      // "enhancement" = structured + LLM analysis (default, more detail).
      // "parity" = structured extraction only (faster, less token usage).
      "explorer_output_profile": "enhancement"
    }
  }
}
```

### Usage

The Explorer activates automatically when the agent views files or processes
large tool output. There are no CLI commands or keybindings for the Explorer.
It runs transparently as part of LCM's content analysis pipeline.

### Default Behavior

The default explorer profile is "enhancement", which runs all three tiers of
analysis. All file-type handlers are active by default. Tree-sitter-based code
analysis requires `CGO_ENABLED=1`; without CGO, the Explorer falls back to
text-based extraction.

---

## 3. Tree-sitter Integration

**Location**: `internal/treesitter/`

Code parsing and analysis via channel-based parser pools with CGO-compiled
grammars for 38 programming languages (entries in `languages.json`; ~28 with
compiled grammar imports).

### Core Components

| Component | File | Lines | Description |
|-----------|------|-------|-------------|
| ParserPool | `parser.go` | 634 | Channel-based pool, sized to `runtime.NumCPU()` |
| QueryLoader | `query.go` | 579 | Compiles and caches `.scm` query files |
| Language Map | `languages.go` | 177 | File extension → language mapping (38 language entries) |
| Import Resolution | `imports.go` | 965 | Per-language import path extraction |
| Cache | `cache.go` | 180 | LRU cache (5,000 entries, 256 MB max) |

### Supported Languages (38 entries in languages.json)

Arduino, C, C++, C#, Chatito, Common Lisp, D, Dart, Emacs Lisp, Elixir,
Elm, Fortran, Gleam, Go, Haskell, HCL, Java, JavaScript, Julia, Kotlin,
Lua, MATLAB, OCaml, OCaml Interface, PHP, Properties, Python, QL, R,
Racket, Ruby, Rust, Scala, Solidity, Swift, TypeScript, Udev, Zig.
Approximately 28 have compiled CGO grammar imports; the remaining entries
use query-only or basic parsing.

### Query Files

38 `.scm` query files in `queries/` for structured AST pattern matching
across all supported languages.

### Build Configuration

- Requires `CGO_ENABLED=1` (fork switched from `CGO_ENABLED=0`)
- `treesitter` build tag enabled by default
- Grammars compiled as CGO native extensions

### User-Facing Description

Tree-sitter provides deep code structure understanding for 38 programming
languages. It powers symbol extraction, import resolution, scope-aware
analysis, and syntax validation across the codebase. When CGO is enabled,
compiled grammars deliver fast, accurate AST parsing. Without CGO, the system
falls back to text-based analysis. Tree-sitter operates transparently --
users never invoke it directly, but benefit from more accurate repository
maps, better code exploration, and reliable post-edit validation.

### Configuration

```jsonc
{
  "options": {
    "repo_map": {
      // Parser pool size. 0 = runtime.NumCPU() (default).
      // Increase for very large codebases; decrease to reduce memory.
      "parser_pool_size": 0
    }
  }
}
```

Tree-sitter requires `CGO_ENABLED=1` and the `treesitter` build tag at compile
time. There is no runtime toggle to enable or disable it -- the binary either
includes compiled grammars or it does not.

### Usage

Tree-sitter operates transparently and has no user-facing commands or
keybindings. It feeds data into four subsystems: repository map generation
(symbol definitions and references), file-type explorer (code structure
analysis), import resolution (dependency graphs), and post-edit validation
(syntax checking after edits).

### Default Behavior

When built with `CGO_ENABLED=1`, Tree-sitter is active by default. The parser
pool is sized to `runtime.NumCPU()`. The LRU cache holds up to 5,000 entries
with a 256 MB maximum. Without CGO, all Tree-sitter-dependent features fall
back to text-based alternatives.

---

## 4. Repository Map

**Location**: `internal/repomap/`

Scope-aware code outline generation for LLM context, using PageRank over a
definition/reference/import graph.

### Generation Pipeline

```
extractTags → buildGraph → BuildPersonalization → Rank(PageRank) → AggregateRankedFiles → BuildSpecialPrelude → AssembleStageEntries → FitToBudget → RenderRepoMap
```

### PageRank

- **Damping factor**: 0.85
- **Tolerance**: 1e-6
- **Max iterations**: 100
- **Personalization**: Blends chat-mentioned files + explicitly mentioned
  filenames and identifiers

### FileGraph Edge Weighting

Edges between files are weighted by multiple factors:

| Factor | Multiplier | Description |
|--------|-----------|-------------|
| Chat origin | 50x | Files discussed in conversation |
| Mentioned | 10x | Identifiers explicitly referenced in code |
| Long names | 10x | Identifiers with descriptive/structured names |
| >5 definitions | 0.1x | Dampen multiply-defined identifiers |
| Underscore prefix | 0.1x | Internal/private identifiers |
| Reference count | `sqrt(count)` | Square-root scaling for connectivity |

### Rendering

`TreeContext` renders AST-driven, scope-aware line selections within a token
budget. Only the most relevant lines are included based on PageRank scores.

### Refresh Modes

| Mode | Trigger |
|------|---------|
| `manual` | Explicit tool invocation only |
| `files` | When watched files change |
| `auto` | Heuristic-based automatic refresh |
| `always` | Refresh on every turn |

### Additional Components

| Component | Lines | Description |
|-----------|-------|-------------|
| Tag Extraction | 473 | Tree-sitter tag extraction for definitions/references (`tags.go`) |
| Blame | 198 | Git blame integration with 7-day half-life decay |
| Proximity | 266 | Test file co-location scoring |
| Staging | 140 | 4-stage file staging for incremental updates |
| Budget | 226 | Token budget management for map rendering |
| DiffWatch | 202 | Watch file diffs for incremental updates |
| Mentions | 260 | Extract file/identifier mentions from conversation |
| Caching | 225 | Persistent cache for computed map data |
| Tokenizer | 367 | Embedded `cl100k_base` BPE tokenizer (~1.6 MB) |

### User-Facing Description

The repository map provides a compressed project outline in every conversation,
giving the LLM project-wide code understanding without reading every file. It
uses PageRank ranking personalized to your current chat -- files you have
discussed or referenced are ranked higher. The token budget is allocated
dynamically based on the model's context window. When LCM is active, the map
budget doubles to compensate for compacted context.

### Configuration

```jsonc
{
  "options": {
    "repo_map": {
      // Disable repository map generation entirely
      "disabled": false,

      // Maximum tokens for the map output.
      // Default: min(max(contextWindow/8, 1024), 4096), capped at 8192 with LCM.
      "max_tokens": 4096,

      // Glob patterns to exclude from the map
      "exclude_globs": ["vendor/**", "node_modules/**", "*.generated.go"],

      // Refresh mode: "auto", "files", "manual", "always"
      "refresh_mode": "auto",

      // Multiplier for map size when no files are open (default: 2.0)
      "map_mul_no_files": 2.0,

      // Parser pool size for tree-sitter (0 = NumCPU)
      "parser_pool_size": 0
    }
  },
  "tools": {
    "repo_map": {
      // Tool-level overrides (merged with options.repo_map)
      // "disabled" uses OR-latch: true in either location disables the map.
      // "exclude_globs" accumulates from both locations.
      // Scalar fields use last-wins priority.
    }
  }
}
```

Merge rules: `disabled` uses OR-latch (either source set to `true` disables the
map), `exclude_globs` accumulate from both locations, and scalar fields use
last-wins priority (tools > options).

### Usage

- **Ctrl+P** -> "Refresh Repository Map" to manually trigger a refresh.
- The agent can invoke the `map_refresh` tool to force a cache invalidation.
- In `"files"` refresh mode, the DiffWatcher polls for file changes every 30
  seconds and triggers incremental updates.

### Default Behavior

The repository map is enabled by default. The token budget is calculated
dynamically as `min(max(contextWindow/8, 1024), 4096)`, capped at 8192 when
LCM is active. The default refresh mode is `"auto"`, which uses heuristics to
decide when to regenerate the map based on conversation context changes.

---

## 5. Processor Pipeline

**Location**: `internal/processor/`

Message intercept pipeline with four sequential phases. 19 production Go
files in this package (entire package added by the fork).

### Phases

| Phase | When |
|-------|------|
| `InputPhase` | Before messages are sent to the LLM |
| `OutputStreamPhase` | As streaming tokens arrive |
| `OutputResultPhase` | After the complete response is received |
| `APIErrorPhase` | When an API error occurs |

### Processor Implementations (16 total)

#### Pipeline Configuration

The pipeline requires explicit `processors` configuration in `crush.json` to
activate. When `ProcessorsOptions` is nil (the default), the entire processor
extension is inactive. When configured, **3 processors are active by default**
(`TokenLimiter`, `SystemPromptScrubber`, `PIIDetector` — marked with \* below).
The remaining 7 configurable processors must be explicitly listed to activate.

#### Configurable Processors (10)

| Processor | Phase | Default | Description |
|-----------|-------|---------|-------------|
| `TokenLimiter` | Input | true | Enforces token budget per request |
| `SystemPromptScrubber` | Output Stream | true | LLM-based detection of system prompt leaks |
| `PIIDetector` | Output Result | true | Regex + optional LLM PII redaction |
| `UnicodeNormalizer` | Input | — | NFKC Unicode normalization |
| `BatchParts` | Output Result | — | Merges adjacent same-role message parts |
| `MessageSelection` | Input | — | Recency/relevance-based message pruning strategies |
| `ToolCallFilter` | Input | — | Allow/deny lists for tool calls |
| `ToolSearch` | Input | — | BM25 search over tool registry |
| `Skills` | Input | — | Load skill definitions into pipeline State |
| `SkillSearch` | Input | — | BM25 search over available skills |

#### Never-Wirable (6)

Implemented but not available for configuration.

`ModerationProcessor`, `PromptInjectionDetector`, `LanguageDetector`,
`StructuredOutput`, `WorkspaceInstructions`, `MessageHistory`

### User-Facing Description

The processor pipeline is an optional message interception layer that can
redact PII, enforce token budgets, prevent system prompt leaks, filter tool
calls, and prune conversation history. It operates in four phases: input
(before sending to the LLM), output stream (during streaming), output result
(after completion), and API error handling. Three processors are active by
default; the remaining seven must be explicitly enabled in configuration.

### Configuration

```jsonc
{
  "options": {
    "processors": {
      // Master switch: set to false to disable the entire pipeline.
      // Default: pipeline is ACTIVE with 3 processors (TokenLimiter, SystemPromptScrubber, PIIDetector).
      "enabled": true,

      // Ordered list of processor names to activate.
      // "skills" must precede "skill_search" if both are used.
      "list": [
        "TokenLimiter",
        "SystemPromptScrubber",
        "PIIDetector"
      ],

      // Per-processor configuration.
      "config": {
        "pii_detector": {
          // Sensitivity level: "low", "medium", "high"
          "sensitivity": "medium"
        },
        "message_selection": {
          // Maximum messages to keep in conversation
          "max_messages": 50,
          // Strategy: "recency" (keep latest) or "relevance" (score-based)
          "strategy": "recency"
        },
        "tool_call_filter": {
          // Allow list: only these tools are permitted (mutually exclusive with deny_list)
          "allow_list": ["view", "edit", "bash"],
          // Deny list: these tools are blocked (mutually exclusive with allow_list)
          "deny_list": ["web_search"]
        }
      }
    }
  }
}
```

Minimal example (3 default processors only):

```jsonc
{
  "options": {
    "processors": {
      "enabled": true,
      "list": ["TokenLimiter", "SystemPromptScrubber", "PIIDetector"]
    }
  }
}
```

PII protection example:

```jsonc
{
  "options": {
    "processors": {
      "enabled": true,
      "list": ["TokenLimiter", "PIIDetector", "SystemPromptScrubber"],
      "config": {
        "pii_detector": { "sensitivity": "high" }
      }
    }
  }
}
```

Conversation trimming example:

```jsonc
{
  "options": {
    "processors": {
      "enabled": true,
      "list": ["TokenLimiter", "MessageSelection"],
      "config": {
        "message_selection": { "max_messages": 30, "strategy": "relevance" }
      }
    }
  }
}
```

Disable the pipeline:

```jsonc
{
  "options": {
    "processors": {
      "enabled": false
    }
  }
}
```

### Usage

The processor pipeline is configured exclusively through `crush.json`. There
are no CLI commands or keybindings. Once configured, processors run
automatically on every LLM request and response. The `skills` processor must
precede `skill_search` in the list to ensure skill definitions are loaded
before search is attempted.

### Default Behavior

The pipeline is **active by default** with 3 processors (`TokenLimiter`,
`SystemPromptScrubber`, `PIIDetector`). When `options.processors` is present
in `crush.json`, only the processors explicitly listed in the `list` array
will run. Set `enabled: false` to disable the entire pipeline.

---

## 6. Multi-Agent Orchestration

**Location**: `internal/agent/`

A suite of multi-agent coordination patterns for task decomposition,
parallel execution, and team-based workflows.

### Operator

**File**: `operator.go` (510 lines)

Decomposes tasks via recursive decomposition with cycle detection.

- **4 strategies**: `LLMMap`, `AgenticMap`, `Batch`, `Sequential`
- Auto-selects strategy based on task characteristics
- Maximum decomposition depth: 3
- Maximum concurrent workers: 16
- Cycle detection via SHA-256 hashing of task descriptors

### Parallel Controller

**File**: `parallel.go` (211 lines)

Bounded fan-out concurrent execution.

- Semaphore-based concurrency control (default: 5 concurrent)
- Focus-area serialization (same-area tasks run sequentially)
- Future-based result collection

### Swarm Pattern

**File**: `swarm.go` (303 lines)

Multi-agent parallel coordination toward a shared goal.

- Parallel subagent fan-out with shared caching
- Configurable result synthesis strategy
- Shared workspace for inter-agent context

### Teammate Pattern

Persistent role-based agents that maintain state across tasks.

- Predefined roles: `researcher`, `tester`, `reviewer`
- Shared mailbox messaging between teammates
- Independent tool surfaces per role

### Forked Agent

**File**: `forked.go` (528 lines)

Deep-copied parent context for parallel branching.

- Deep-copies parent session context
- Mailbox messaging (buffered channel, capacity 64)
- Turn-limited execution (max 10 turns)
- Tool filtering (subset of parent tools)

### Structured Subagent

**File**: `structured_subagent.go` (193 lines)

Typed-output child agents with coordinator backing.

- Depth-limited (max 3 levels of nesting)
- Per-request tool filtering
- Typed output schemas for result validation

### Inter-Agent Communication

| Mechanism | Description |
|-----------|-------------|
| `send_message` tool | Send messages between agents via mailbox |
| `task_stop` tool | Stop a running forked sub-agent |
| `team_create` tool | Create a named agent team |
| `team_delete` tool | Delete a team and stop all members |
| `agentic_map` tool | Run a sub-agent on each item in a JSONL dataset |
| `llm_map` tool | Apply LLM transformation to each JSONL item (read-only) |
| `map_refresh` tool | Force repo-map cache invalidation |
| `synthetic_output` tool | Generate synthetic output for testing/simulation |

### User-Facing Description

The orchestration system enables the agent to break complex tasks into
parallel sub-tasks. Six patterns are available: **Operator** decomposes tasks
into recursively decomposed sub-tasks, **Parallel** fans out concurrent execution with
bounded semaphores, **Swarm** coordinates multiple agents toward a shared
goal, **Teammates** maintain persistent role-based agents (researcher, tester,
reviewer), **Forked** creates parallel branches with deep-copied context, and
**Structured** produces typed output from child agents. All patterns include
inter-agent mailbox messaging for coordination.

### Configuration

There is no user-facing configuration for orchestration. All limits are
hardcoded:

| Parameter | Value |
|-----------|-------|
| Maximum decomposition depth | 3 |
| Maximum concurrent workers | 16 |
| Parallel semaphore | 5 concurrent |
| Mailbox channel capacity | 64 messages |
| Forked agent turn limit | 10 turns |
| Structured subagent nesting depth | 3 levels |

### Usage

Orchestration patterns are triggered automatically by the agent when it
determines a task would benefit from decomposition or parallelization. Users
can encourage the agent to use these patterns with phrases like "break this
into steps", "work on these files in parallel", or "use sub-agents for this".
No explicit commands or keybindings are required.

### Default Behavior

All orchestration patterns are available but not active until the agent needs
them. The agent automatically selects the appropriate pattern based on task
characteristics. No configuration is required.

---

## 7. Extension Host

**Location**: `internal/ext/host.go` (329 lines) + `internal/extensions/`

Runtime extension system with plugin-like capabilities.

### Architecture

- Base `Extension` interface with lifecycle hooks
- 4 capability interfaces:
  - `ToolProvider` -- Register agent-facing tools
  - `RunHookProvider` -- Hooks before/after agent runs
  - `StepHookProvider` -- Hooks per agent step
  - `PromptHookProvider` -- Hooks for prompt assembly

### Registered Extensions (18)

| Extension | Description |
|-----------|-------------|
| `lcm` | Lossless Context Management integration |
| `repomap` | Repository map generation and caching |
| `treesitter` | Tree-sitter parsing and analysis |
| `processor` | Processor pipeline management |
| `orchestration` | Multi-agent coordination |
| `model-router` | Model routing and tier selection |
| `autofix` | Auto-fix loop integration |
| `diag-gate` | Diagnostic quality gates |
| `edit` | Enhanced edit operations (anchors, fuzzy, batch) |
| `rewind` | Turn-based snapshot and rewind |
| `doom` | Doom loop detection |
| `swarm` | Swarm pattern coordination |
| `tool-surface` | Tool registry and surface descriptions |
| `resource-limits` | Per-agent resource budgets |
| `xrush-sessions` | Extended session management |
| `prompt-assembly` | System prompt assembly |
| `lsp-tools` | LSP-powered code intelligence tools |
| `StepAdapter` | Step-level adapter for agent coordination |

### Safety Features

- Panic-safe execution with recovery
- Tool deduplication across extensions
- Hook collection and aggregation
- `Bootstrap` / `Shutdown` lifecycle management

### User-Facing Description

The Extension Host is Crush's internal plugin system. It manages 18
compile-time extensions that provide capabilities like LCM integration,
repository maps, tree-sitter parsing, the processor pipeline, orchestration,
model routing, auto-fix, rewind, doom loop detection, and LSP tools. Users
do not interact with the extension host directly -- it runs at startup,
registers all extensions, and coordinates their lifecycle.

### Configuration

There is no user-facing configuration for the extension host itself. Individual
extensions check their own configuration sections during `Init()` and
deactivate silently if their required config is absent. Control individual
extensions via their own config sections:

- **Processor pipeline**: `options.processors`
- **Rewind**: `options.snapshot`
- **Doom loop**: `options.doom_loop_intervention`
- **Model router**: `options.router_tiers`
- **Tree-sitter**: `options.repo_map.parser_pool_size`
- **Auto-fix**: `options.validation`

### Usage

The extension host operates entirely internally. Users control individual
extensions through their respective configuration sections in `crush.json`.
There are no CLI commands or keybindings for the host itself.

### Default Behavior

All 18 extensions are registered at startup. Panic-safe execution ensures one
crashing extension does not affect others. Tool deduplication prevents
conflicts when multiple extensions register the same tool. Extensions follow
an ordered `Bootstrap` / `Shutdown` lifecycle.

---

## 8. Rewind System

**Location**: `internal/rewind/`

Turn-based snapshot and rewind for code and conversation state.

### Components

| Component | Lines | Description |
|-----------|-------|-------------|
| `Snapshotter` | 131 | Capture file and conversation state per turn |
| `Rewinder` | 151 | Restore to any previous snapshot |
| `Service` | 39 | Composes Snapshotter + Rewinder + Forker + Editor |
| Types | 117 | Shared types for snapshot and rewind operations |

### Rewind Modes

| Mode | What is Restored |
|------|-----------------|
| Code-only | File system state only |
| Conversation-only | Chat history only |
| Combined | Both files and conversation |

### Technical Configuration

- 50 snapshots retained per session
- `PostRewindHook` for post-rewind callbacks
- Migration: `20260515000000_turn_snapshots.sql`

### User-Facing Description

The rewind system lets you undo agent work at any point in the conversation.
Navigate to any user message in the TUI, press `o`, and choose from six
options: three rewind modes (code-only, conversation-only, or combined),
edit and resubmit the message, fork the conversation from that point, or
cancel. Conversation-only rewind loads the original message text back into
the editor for re-submission.

### Configuration

```jsonc
{
  "options": {
    // Enable rewind/snapshot system.
    // {} = enabled with defaults (max 50 snapshots per session).
    // null or omitted = disabled.
    "snapshot": {
      "max_per_session": 50
    }
  }
}
```

Minimal enable:

```jsonc
{
  "options": {
    "snapshot": {}
  }
}
```

### Usage

The rewind system is TUI-only. Navigate to any user message in the
conversation, press `o` to open the action menu, and select:

1. **Rewind code** -- restore files to that point in time
2. **Rewind conversation** -- restore chat history to that point
3. **Rewind both** -- restore both files and conversation
4. **Edit and resubmit** -- load the original message into the editor
5. **Fork** -- create a new branch from that point
6. **Cancel** -- close the menu

### Default Behavior

The rewind system is **disabled** unless `options.snapshot` is present in
`crush.json`. When enabled with an empty object `{}`, the default is 50
snapshots per session. Setting the key to `null` or omitting it disables
snapshots entirely.

---

## 9. Eval Framework

**Location**: `internal/eval/`

Agent evaluation harness for measuring output quality across multiple
dimensions.

### Components

| Component | Lines | Description |
|-----------|-------|-------------|
| Harness | 296 | Parallel scorer registration and execution |
| Runner | 200 | Loads JSON datasets, runs through harness |
| Report | 222 | Evaluation report generation |
| Storage | 321 | Persistent storage for evaluation results |

### Scorer Types

| Type | Description |
|------|-------------|
| **Metric** | 10 built-in metric types (build success, test pass rate, syntax validity, lint score, edit distance, coverage, typecheck, etc.) |
| **Judge** | LLM-based quality assessment (193 lines) |
| **Mastra** | Mastra framework integration for agent evaluation (236 lines) |

### Technical Usage

```bash
crush eval --dataset <path> --scorer <name>
```

### User-Facing Description

The eval framework measures agent output quality with automated scoring. Three
scorer types are available: **Metric** scorers use code-analysis checks (build
success, test pass rate, syntax validity, lint score, edit distance, coverage,
typecheck, and more), **Judge** scorers use an LLM to assess quality dimensions
like correctness, completeness, and style, and **Mastra** scorers integrate
with the Mastra framework for agent evaluation. Results are stored
persistently for comparison across runs.

### Configuration

There is no `crush.json` configuration for the eval framework. All settings
are controlled through CLI flags.

### Usage

```bash
# List all available scorers (19 total)
crush eval

# Run a specific scorer against a dataset
crush eval --dataset path/to/dataset.jsonl --scorer build_success

# Output results to a JSON report
crush eval --dataset path/to/dataset.jsonl --scorer build_success --output report.json
```

### Default Behavior

No configuration is needed. The eval framework is entirely CLI-driven and
does not affect normal Crush operation. It activates only when the `crush eval`
command is invoked.

---

## 10. Doom Loop Detection & Resource Limits

### Doom Loop Detection

**File**: `internal/agent/doom.go` (394 lines)

Detects when the agent is stuck in repetitive tool-call cycles.

**3-tier escalation**:

| Tier | Trigger | Response |
|------|---------|----------|
| 1 | 3 consecutive repeats | Warning + context hint |
| 2 | 5 consecutive repeats | Forced strategy change |
| 3 | 7 consecutive repeats | Agent termination |

**Detection methods**:

- **Exact match**: SHA-256 hash of tool name + input + output
- **Semantic match**: 80% argument overlap threshold

### Resource Limits

**File**: `internal/agent/resource_limits.go` (333 lines)

Per-subagent-type resource budgets:

- Token limits (input + output)
- Step limits (tool calls per turn)
- Duration limits (wall-clock time)
- **Soft warning** at 80% usage
- **Hard cancellation** at 100% usage

### User-Facing Description

The doom loop detector recognizes when the agent is stuck repeating the same
tool calls and intervenes with three escalation tiers: a warning at 3 repeats,
a forced tool switch at 5 repeats, and agent termination at 7 repeats. A
"productive loop" exception prevents false positives when the agent is making
meaningful progress through repeated similar operations. Resource limits enforce
per-subagent token, step, and duration budgets with soft warnings at 80%.

### Configuration

```jsonc
{
  "options": {
    // Doom loop intervention mode.
    // "warn" = show warnings and forced tool switches (default).
    // "full" = aggressive: warnings + forced switches + termination.
    // "none" = disable doom loop detection entirely.
    "doom_loop_intervention": "warn"
  }
}
```

Resource limits have no user-facing configuration. Budgets are hardcoded per
subagent type.

### Usage

Doom loop detection and resource limits are fully automatic. Visual cues
appear in the TUI: a warning indicator at 3+ consecutive repeats, a forced
tool switch notification at 5+ repeats, and agent halt at 7+ repeats. No user
action is required.

### Default Behavior

The default intervention mode is `"warn"`, which shows warnings and forces
tool strategy changes without terminating the agent. Resource limits issue a
soft warning at 80% usage and hard cancellation at 100% usage.

---

## 11. AutoFix Loop & Go Linter

### AutoFix

**File**: `internal/agent/autofix.go` (420 lines)

Iterative lint -> fix -> test -> reflect cycle:

1. Run linter on changed files
2. If errors found, generate fixes
3. Apply fixes and re-run tests
4. Reflect on results
5. **Max 3 retry cycles**
6. Automatic rollback on persistent failure
- Optional auto-commit after successful fixes

### Go Linter

**File**: `internal/agent/go_linter.go` (177 lines)

Go-specific linting integration:

- **Preferred**: `golangci-lint` (comprehensive checks)
- **Fallback**: `go vet` (basic checks)
- **Timeout**: 60 seconds per run

### User-Facing Description

AutoFix is a built-in QA loop that runs after edits: lint changed files,
generate fixes for any errors, apply fixes, re-test, and reflect on results.
It retries up to 3 times and automatically rolls back changes if errors
persist. The Go linter prefers `golangci-lint` for comprehensive checks and
falls back to `go vet` for basic analysis.

### Configuration

```jsonc
{
  "options": {
    "validation": {
      // Enable post-edit validation (syntax checking).
      // Default: false.
      "enabled": false,

      // Automatically attempt to fix validation errors.
      // Default: false.
      "auto_fix": false,

      // Enable the iterative autofix loop (lint -> fix -> test -> reflect).
      // Requires both "enabled" and "auto_fix" to be true.
      // Default: false.
      "autofix_loop_enabled": false
    }
  }
}
```

All three settings default to `false`. The autofix loop requires both
`enabled: true` and `auto_fix: true` before it will activate.

### Usage

AutoFix is entirely config-driven. There are no CLI commands or keybindings.
Enable the three layers in order:

1. Set `validation.enabled: true` for post-edit syntax validation.
2. Set `validation.auto_fix: true` to automatically fix errors.
3. Set `validation.autofix_loop_enabled: true` to enable the full iterative
   lint-fix-test-reflect cycle.

### Default Behavior

All three validation settings default to `false`. AutoFix does not run unless
explicitly enabled. When enabled, the Go linter prefers `golangci-lint` if
available on `$PATH`, falling back to `go vet` with a 60-second timeout.

---

## 12. Architect Planning

**File**: `internal/agent/architect_plan.go` (131 lines)

Structured planning before code changes:

- JSON-structured steps with dependency ordering
- Heuristic prompt classification (bug fix, feature, refactor)
- Step-level validation criteria
- Dependency graph for execution ordering

### User-Facing Description

The architect system introduces a two-phase workflow for complex tasks: plan
first, then execute. When the agent determines a prompt describes a complex
change, it first generates a structured plan with ordered steps, dependency
graphs, and validation criteria before writing any code. A heuristic classifier
categorizes prompts as bug fixes, features, or refactors to tailor the
planning approach.

### Configuration

```jsonc
{
  "options": {
    "architect": {
      // Require user approval before executing the plan.
      // Default: false (plans execute automatically).
      "approval_required": false
    }
  }
}
```

### Usage

The architect is agent-driven. It activates automatically when the agent
classifies a prompt as a complex task. Users can encourage plan-first behavior
with phrases like "plan and implement this feature" or "break this into a
plan first". If `approval_required` is set to `true`, the plan is displayed
for user review before execution begins.

### Default Behavior

The architect is disabled by default. It activates only when the agent's
heuristic classifier determines a prompt warrants planning. No configuration
is required for the agent to begin using it when needed.

---

## 13. LSP Enhancements

**Location**: `internal/lsp/`

Enhanced LSP client with crash recovery, auto-download, and health monitoring.

### New Components

| Component | Lines | Description |
|-----------|-------|-------------|
| CrashRecovery | 91 | Automatic recovery from LSP server crashes |
| Download | 171 | On-demand LSP server binary download |
| Executor | 221 | Enhanced LSP request execution with retries |
| Health | 127 | LSP server health monitoring and restart |
| Backoff | 70 | Exponential backoff for failed requests |
| NamePath | 117 | Language name to server path resolution |

### New LSP Agent Tools (11)

| Tool | LSP Method |
|------|-----------|
| `lsp_definition` | `textDocument/definition` |
| `lsp_hover` | `textDocument/hover` |
| `lsp_rename` | `textDocument/rename` |
| `lsp_replace_symbol` | Custom symbol replacement |
| `lsp_insert_before` | Insert text before a symbol |
| `lsp_insert_after` | Insert text after a symbol |
| `lsp_safe_delete` | Safe symbol deletion with reference checking |
| `lsp_completion` | `textDocument/completion` |
| `lsp_formatting` | `textDocument/formatting` |
| `lsp_signature_help` | `textDocument/signatureHelp` |
| `lsp_code_action` | `textDocument/codeAction` |

Supporting files: `lsp_symbolic.go` (shared symbol operations), `lsp_helpers.go`
(shared utilities). These are not standalone tools but are used by the tools above.

### User-Facing Description

The enhanced LSP integration provides robust language server connectivity with
automatic crash recovery (always-on, exponential backoff from 1s to 60s, up to
5 retries), on-demand server binary download with SHA256 verification, and
health monitoring. 11 new agent tools expose LSP capabilities: go-to-definition,
hover, rename, symbol replacement, insertion, deletion with reference checking,
completion, formatting, signature help, and code actions.

### Configuration

```jsonc
{
  "lsp": {
    // Per-language LSP server configuration.
    // Key is the language name (e.g., "go", "typescript", "python").
    "go": {
      // Command to start the LSP server
      "command": "gopls",

      // Arguments to pass to the server command
      "args": [],

      // Environment variables for the server process
      "env": {
        "GOTOOLCHAIN": "go1.24.5"
      },

      // File types this server handles (auto-detected if omitted)
      "filetypes": [".go"],

      // Patterns for matching files to this server
      "match_patterns": [],

      // Root markers to identify project root (e.g., ["go.mod"])
      "root_markers": [],

      // Initialization options sent to the server
      "init_options": {},

      // Server-specific settings
      "options": {},

      // Request timeout in seconds (default: 30)
      "timeout": 30,

      // Disable this server
      "disabled": false,

      // Auto-download server binary
      "auto_download": {
        "url": "https://example.com/gopls-linux-amd64",
        "sha256": "abc123..."
      }
    }
  },
  "options": {
    // Automatically start LSP servers when relevant files are opened.
    // Default: true.
    "auto_lsp": true,

    // Enable LSP debug logging.
    // Default: false.
    "debug_lsp": false
  }
}
```

### Usage

LSP servers are configured exclusively through `crush.json`. Servers
auto-start when the agent opens a file matching a configured language. If a
server crashes, it is automatically restarted with exponential backoff. The
11 LSP tools are available to the agent automatically when a matching server
is running.

### Default Behavior

`auto_lsp` defaults to `true`, meaning LSP servers start automatically when
needed. Crash recovery is always-on with no configuration option to disable
it. The default request timeout is 30 seconds. Auto-download requires
explicit `url` and `sha256` configuration per server.

---

## 14. Tools

### Upstream Tools (23 inherited)

`bash`, `view`, `edit`, `write`, `multiedit`, `glob`, `grep`, `ls`, `fetch`,
`download`, `sourcegraph`, `lsp_diagnostics`, `lsp_references`, `lsp_restart`,
`web_search`, `web_fetch`, `job_output`, `job_kill`, `list_mcp_resources`,
`read_mcp_resource`, `todos`, `crush_logs`, `crush_info`

### Fork-New Tools (23 in standard registry + 9 LCM retrieval = 32)

#### Standard Registry Tools (23)

| Tool | Category | Description |
|------|----------|-------------|
| `agentic_map` | Orchestration | Run sub-agent on each JSONL item |
| `llm_map` | Orchestration | LLM transformation per JSONL item |
| `map_refresh` | Orchestration | Force repo-map cache invalidation |
| `lcm_describe` | LCM | Describe file/summary by LCM identifier |
| `lcm_expand` | LCM | Expand LCM summary to original messages |
| `lcm_grep` | LCM | Search conversation history |
| `batch_edit` | Edit | Atomic multi-file batch edits with rollback |
| `send_message` | Communication | Inter-agent mailbox messaging |
| `task_stop` | Communication | Stop a running forked sub-agent |
| `team_create` | Communication | Create named agent team |
| `team_delete` | Communication | Delete team and stop members |
| `synthetic_output` | Testing | Synthetic output for testing/simulation |
| `lsp_definition` | LSP | Go to definition |
| `lsp_hover` | LSP | Hover information |
| `lsp_rename` | LSP | Rename symbol |
| `lsp_replace_symbol` | LSP | Replace symbol implementation |
| `lsp_insert_before` | LSP | Insert text before symbol |
| `lsp_insert_after` | LSP | Insert text after symbol |
| `lsp_safe_delete` | LSP | Safe deletion with reference check |
| `lsp_completion` | LSP | Code completion |
| `lsp_formatting` | LSP | Document formatting |
| `lsp_signature_help` | LSP | Function signature help |
| `lsp_code_action` | LSP | Code actions |

#### LCM Retrieval Tools (9, via `ExtraAgentTools()`)

`lcm_bindle`, `lcm_ancestry`, `lcm_dolt`, `lcm_archive`, `lcm_sprig`,
`lcm_time_query`, `lcm_file_search`, `lcm_active_context`, `lcm_lineage`

#### Not Agent Tools (Internal Helpers)

`diag_autofix` and `diag_gate` are internal helper types used by the
AutoFix and DiagGate extension systems. They have no `New*Tool`
constructors and are not registered in the agent tool surface.

### Fork-New Edit Support

| Component | Lines | Description |
|-----------|-------|-------------|
| `edit_anchors` | 136 + 116 | FNV-1a hash-based content-addressed anchors for drift-tolerant edits |
| `edit_fuzzy` | 307 | Whitespace-normalized fuzzy string matching for approximate edit targets |
| `rollback` | 220 | File snapshot and rollback on failure |
| `validate` | 832 | Tree-sitter syntax validation after edits |
| `validation_handler` | 228 | Post-edit validation pipeline (orchestrates validate + rollback) |
| `view_xrush` | — | Enhanced batch file reading with LCM context awareness |

### User-Facing Description

The fork provides 55 total tools: 23 inherited from upstream plus 32 new. The
enhanced edit subsystem introduces anchor-based edits (content-addressed hashes
that survive minor file changes), fuzzy string matching for approximate edit
targets, atomic multi-file batch editing with rollback, and a 12-stage
post-edit validation pipeline. Users benefit from more reliable edits, atomic
multi-file changes, and automatic syntax validation after every edit.

### Configuration

```jsonc
{
  "permissions": {
    // Tools that can run without user confirmation.
    "allowed_tools": ["view", "ls", "grep", "edit"]
  },
  "options": {
    // Tools to completely hide from the agent.
    "disabled_tools": ["bash", "sourcegraph"],

    // Post-edit validation and auto-fix settings.
    "validation": {
      "enabled": false,
      "auto_fix": false,
      "autofix_loop_enabled": false
    }
  }
}
```

### Usage

All tools are agent-facing. Users do not invoke tools directly. The benefits
are indirect: more reliable edits that handle minor file drift via anchors,
fuzzy matching that tolerates whitespace differences, atomic multi-file
batch operations that roll back on failure, and automatic syntax validation
after changes.

### Default Behavior

All tools are registered and available to the agent by default. Tree-sitter
syntax validation requires `CGO_ENABLED=1`. Fuzzy string matching is always
active for edit operations. The `disabled_tools` option can hide specific
tools from the agent's surface.

---

## 15. Config Enhancements

### Include Directives

**File**: `internal/config/include.go` (231 lines)

- `@include` directives in `crush.json` for modular configuration
- Conditional blocks (environment-based)
- Cycle detection to prevent infinite includes

### Deep Merge

**File**: `internal/config/merge.go` (276 lines)

- Overlay-wins strategy for scalar values
- Accumulation strategy for slices
- Recursive merge for nested objects

### Additional Config Components

| Component | Lines | Description |
|-----------|-------|-------------|
| Migration | 44 | Config schema migration |
| Walking | 134 | Walk config tree for validation |
| YAML | 345 | YAML config file support |

Note: `schema.json` exists at the project root (1,073 lines) as a general
config schema, not within `internal/config/`.

### User-Facing Description

Two config enhancements: `@include` directives allow modular configuration in
context files (AGENTS.md, CRUSH.md, etc.) with conditional blocks based on
language, file, or environment variables. Deep merge combines multiple config
sources with a clear priority ordering: `$HOME/.config/crush/crush.json` <
`$HOME/.local/share/crush/crush.json` < project-local configs (root→cwd walk,
`.crush.json` and `crush.json` at each level) < `.xrush/config.yml` <
`.crush/<workspace>.json`. YAML config is supported as an alternative via
`.xrush/config.yml`.

### Configuration

```jsonc
// @include in context files (e.g., AGENTS.md, CRUSH.md):
// Include another file's contents.
// @include path/to/file.md

// Conditional includes.
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

YAML config alternative (`.xrush/config.yml`):

```yaml
options:
  auto_lsp: true
  debug_lsp: false
  validation:
    enabled: true
    auto_fix: true
```

Deep merge priority (lowest to highest):

1. `$HOME/.config/crush/crush.json` (global config)
2. `$HOME/.local/share/crush/crush.json` (global data config)
3. Project-local configs (root→cwd walk: `.crush.json` and `crush.json` at each level)
4. `.xrush/config.yml` (YAML config)
5. `.crush/<workspace>.json` (workspace-specific config)

### Usage

All config enhancements are file-based. `@include` directives work in context
files (AGENTS.md, CRUSH.md, CLAUDE.md, GEMINI.md, and their `.local`
variants). YAML config requires a `.xrush/config.yml` file in the project
root. Deep merge applies automatically when multiple config files exist.

### Default Behavior

`@include` is always active in context files. Deep merge is always active
when multiple config files are present. YAML config requires `.xrush/config.yml`
to exist; no fallback or auto-creation.

---

## 16. Message Timestamps

Full-stack implementation of message timing:

| Column | Description |
|--------|-------------|
| `submitted_at` | When the user submitted the message |
| `sent_to_llm_at` | When the message was sent to the LLM |
| `first_token_at` | When the first streaming token arrived |
| `completed_at` | When the response was fully received |

Wiring spans: DB migration -> `Message` struct -> agent integration -> UI display.

### User-Facing Description

Every message in a session tracks four lifecycle timestamps: when it was
submitted, when it was sent to the LLM, when the first streaming token
arrived, and when the response completed. These timestamps enable latency
analysis -- you can see exactly how long each phase of the LLM interaction
takes. Timestamps are displayed in the TUI and queryable via the SQLite
database.

### Configuration

There is no configuration for message timestamps. The feature is always
enabled.

### Usage

Timestamps are transparent. They are displayed automatically in the TUI next
to each message. For programmatic access, query the `messages` table in the
session database:

```sql
SELECT submitted_at, sent_to_llm_at, first_token_at, completed_at
FROM messages
WHERE session_id = ?;
```

### Default Behavior

Always on. The four timestamp columns default to `0` (not yet recorded) and
are populated as the message progresses through each lifecycle stage. No
configuration or opt-in required.

---

## 17. Database Migrations

17 new migrations added by the fork:

| Migration | Description |
|-----------|-------------|
| `20260219000000_lcm.sql` | LCM initial schema (7 tables + 2 FTS5 + triggers) |
| `20260222000000_repo_map.sql` | Repository map cache storage |
| `20260501000000_xrush_dag.sql` | LCM infrastructure (reversible state, observation buffer, auto-memory) |
| `20260506000000_session_om.sql` | Session observation/memory system |
| `20260507000000_lcm_block_ids.sql` | LCM content block identifiers |
| `20260508000000_rename_session_om.sql` | Rename session OM tables |
| `20260509000000_om_priority.sql` | Observation priority field |
| `20260510000000_scorer_storage.sql` | Eval scorer result storage |
| `20260511000000_repo_map_imports.sql` | Import graph for repository map |
| `20260512000000_lcm_auto_memory_confidence.sql` | Auto-memory confidence scoring |
| `20260512000001_session_om_thread_id.sql` | Thread ID for session observations |
| `20260513000000_scorer_mastra.sql` | Mastra scorer integration |
| `20260514000000_add_written_files_table.sql` | Track files written per session |
| `20260515000000_turn_snapshots.sql` | Turn-based snapshot storage |
| `20260516000000_message_timestamps.sql` | Message timestamp columns |
| `20260517000000_lcm_gaps.sql` | Content replacements + large files FTS |
| `20260518000000_lcm_observation_priority.sql` | Observation priority support |

### User-Facing Description

24 SQLite migrations are applied automatically on startup using the Goose
format with Up and Down support. Each migration adds schema for a specific
fork feature: LCM tables, repository map cache, LCM infrastructure (reversible state, observation buffer, auto-memory),
session observations, eval scorer storage, turn snapshots, message timestamps,
and more. A `vacuum_guard` prevents SQLite VACUUM operations during active
agent work.

### Configuration

There is no configuration for database migrations. All migrations are embedded
in the binary and applied automatically.

### Usage

Migrations run automatically on startup. To inspect migration history:

```bash
crush logs --tail 100 | grep -i migrat
```

### Default Behavior

All migrations are applied automatically on every startup. There is no opt-out.
The `vacuum_guard` component prevents VACUUM operations from running during
active agent work to avoid data corruption.

---

## Appendix: Additional Components

| Component | Description |
|-----------|-------------|
| `CacheShare` (128L) | Cross-agent cache sharing via colon-separated string keys |
| `RateLimiting` (180L) | Reactive 429-backoff rate limit coordination |
| `ModelRouter` + `TierRouter` | Model routing with tier-based selection; `ModelRouter` deprecated, `TierRouter` active |
| `ConfigLoader` (253L) | Dynamic agent configuration from `crush.json` |
| `ToolSurface` (343L) | Tool registry and surface description for prompts |
| `Session` (349L) | Session management (`session/session.go`) |
| `Completer` (88L) | Shell command completion |
| `WrittenFiles` (79L) | Track files written during a session |
| Background shell timeout | Timeout for background shell command execution |
| `regexp_modernc` / `ncruces` adapters | Pure-Go regexp adapters (no CGO dependency for regex) |
| `vacuum_guard` | Prevent SQLite vacuum during active operations |

### User-Facing Description

Several internal components support the fork's features:

- **CacheShare** enables cross-agent cache sharing using colon-separated string
  keys, allowing sub-agents to reuse expensive computations.
- **RateLimiting** provides reactive 429-backoff coordination to handle API
  rate limits gracefully.
- **TierRouter** replaces the deprecated `ModelRouter` for model selection
  based on configurable tiers (e.g., route simple tasks to a cheaper model).
- **ConfigLoader** dynamically loads agent configuration from `crush.json`,
  enabling per-agent settings.
- **ToolSurface** maintains the tool registry and generates surface
  descriptions for agent prompts.
- **WrittenFiles** tracks which files were written during a session for
  cleanup and rollback.
- **regexp adapters** (`regexp_modernc`, `ncruces`) provide pure-Go regexp
  support without requiring CGO.
- **vacuum_guard** prevents SQLite VACUUM from running during active agent
  operations.

### Configuration

```jsonc
{
  "options": {
    // Tier-based model routing configuration.
    // Array of tiers, evaluated in order. First matching tier wins.
    "router_tiers": [
      {
        "up_to_tokens": 1000,
        "model_type": "small"
      },
      {
        "up_to_tokens": 100000,
        "model_type": "large"
      }
    ]
  }
}
```

### Usage

All appendix components operate internally. There are no user-facing commands
or keybindings. Users configure `router_tiers` to control model selection;
all other components run transparently.

### Default Behavior

All components are enabled by default. Cache sharing, rate limiting, tool
surface generation, file tracking, and regexp adapters require no
configuration. TierRouter activates only when `options.router_tiers` is
configured; otherwise, the session model is used for all requests.
