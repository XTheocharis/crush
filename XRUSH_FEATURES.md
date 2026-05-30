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
- [10. Doom Loop Detection & Resource Limits](#10-doom-loop-detection-resource-limits)
- [11. AutoFix Loop & Go Linter](#11-autofix-loop-go-linter)
- [12. Architect Planning](#12-architect-planning)
- [13. LSP Enhancements](#13-lsp-enhancements)
- [14. Tools](#14-tools)
- [15. Config Enhancements](#15-config-enhancements)
- [16. Message Timestamps](#16-message-timestamps)
- [17. Database Migrations](#17-database-migrations)
- [18. Hooks Engine](#18-hooks-engine)
- [19. Shell Enhancements](#19-shell-enhancements)
- [20. TUI Enhancements](#20-tui-enhancements)
- [Appendix: Additional Components](#appendix-additional-components)

---

## 1. Lossless Context Management (LCM)

**Location**: `internal/lcm/`

LCM is a conversation context management system that summarizes, compacts, and
retrieves conversation history to stay within token budgets while preserving
critical information.

### Manager

The `Manager` (`manager.go`, ~1,706 lines) exposes 48 interface methods and 5 standalone functions (53 total) organized into
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

9-layer pipeline processes conversation content in priority order:

| Priority | Layer | Description |
|----------|-------|-------------|
| 1 | `micro-compactor` | Stores large content (tool output, file contents) in `lcm_large_files` table, replaces with compact references |
| 2 | `dedup-compaction` | SHA-256 deduplication of repeated content blocks |
| 3 | `stale-eviction` | Evicts tool output older than 30 minutes |
| 4 | `post-compact-cleanup` | Cleans orphaned references after compaction |
| 5 | `adjacent-condensation` | Merges adjacent summary blocks into unified summaries |
| 5 | `pressure-selector` | Selects compaction sub-layers based on memory pressure tier (see below) |
| 15 | `time-gap-compactor` | Detects time gaps >30s between messages; compacts older tool-result outputs in gap regions |
| 60 | `compact-prompt-structure` | 9-section prompt assembly optimization |
| 70 | `anthropic-cache-management` | Anthropic prefix cache optimization |

### Observation System

Observations carry priority tags (`high`, `medium`, `low`) and are bridged
into agent prompts with a dedicated token budget. High-priority observations
persist across compaction cycles.

### Memory System

**Location**: `internal/lcm/memory.go` (~812 lines)

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

### Supporting Files

Twelve additional LCM files supporting the core modules:

- **File**: `compactor.go` (~381 lines) — Core compaction orchestration
- **File**: `config.go` (~53 lines) — LCM configuration types
- **File**: `content_replacement_store.go` (~122 lines) — Content replacement persistence
- **File**: `context.go` (~53 lines) — Context management helpers
- **File**: `errors.go` (~41 lines) — Sentinel error definitions
- **File**: `id.go` (~25 lines) — LCM identifier generation
- **File**: `lcm_hooks.go` (~134 lines) — Hook registration for LCM lifecycle events
- **File**: `pressure.go` (~364 lines) — Graduated pressure system with tiered compaction strategies (see [Graduated Pressure System](#graduated-pressure-system) below)
- **File**: `retrieval.go` (~310 lines) — Retrieval orchestration logic
- **File**: `retrieval_tools.go` (~312 lines) — Retrieval tool constructor implementations
- **File**: `summarizer.go` (~243 lines) — LLM-powered conversation summarization
- **File**: `types.go` (~171 lines) — Shared LCM type definitions

### LCM Agent Tools (14)

5 tools via toolFactory (registered in standard tool surface):

| Tool | Description |
|------|-------------|
| `lcm_grep` | Search conversation history (full-text and regex) |
| `lcm_describe` | Describe a file or summary by LCM identifier |
| `lcm_expand` | Expand an LCM summary to its original messages |
| `llm_map` | Apply LLM transformation per JSONL item (read-only) |
| `agentic_map` | Run sub-agent on each JSONL item, write results |

9 retrieval tools via `ExtraAgentTools()` (injected directly into the coder agent):

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

### Reflector

**Location**: `internal/lcm/reflector.go` (601 lines)

The **ReflectorAgent** triggers asynchronous reflection when a session's token
count crosses the configured threshold (default 40K tokens). It loads
unreflected observations from the observation buffer, calls the LLM to produce
(insight, confidence, action_suggestion) tuples, and persists them as
reflections. The prompt adapts to context pressure via five compression levels
(Normal → Extractive → Aggressive → Skeleton → Deterministic).

The **BufferingCoordinator** collects observations at 20% intervals of the
reflection threshold (i.e. at 20%, 40%, 60%, 80%, 100%) and triggers the
reflector at 50% intervals (giving it two chances to produce insights before
the session hits the full token budget). A `Flush` method forces immediate
reflection for end-of-session cleanup. Controlled by the `reflector_enabled`
config key.

### Nudge System

**Location**: `internal/lcm/nudge/nudge.go` (196 lines)

Proactive context-management hints injected into the agent's prompt. Three
nudge types fire under different conditions:

- **context-limit**: when token usage exceeds `max_context_limit` and
  pressure is high (≥95% of context window). Warns the agent to be concise.
- **turn**: every `nudge_frequency` turns (default 5) when pressure is
  medium or higher. Suggests periodic summarization.
- **iteration**: when iteration count exceeds `iteration_nudge_threshold`
  (default 15) at medium+ pressure. Warns about potential loops.

Force levels: **soft** (advisory, default) or **hard** (directive). Configured
via the `nudge` block in LCM options.

#### Context-Limit Double-Gate

The context-limit nudge has an undocumented double-gate at
`nudge.go:141-146` that requires **all three** conditions to be true
simultaneously:

1. `currentTokens >= MinContextLimit` (default 50 000)
2. `currentTokens >= MaxContextLimit` (default 100 000)
3. `tier == PressureHigh` (≥95 % of context window)

Because condition (1) is always true whenever condition (2) is true
(the defaults satisfy 100 k ≥ 50 k), the `MinContextLimit` check is
effectively redundant with the current defaults. However, if
`MinContextLimit` is raised above `MaxContextLimit`, it would block all
context-limit nudges.

#### Production Caller Uses `Inject`, Not `InjectFull`

`CacheOptimizer` (`cache_optimizer.go:596`) calls `Inject`, which
delegates to `InjectFull` with **zero** `TurnCount` and
`IterationCount`. This means the turn and iteration nudge branches can
never fire from the production code path — only the context-limit nudge
is active in practice. For turn/iteration nudges to work, the caller
would need to invoke `InjectFull` directly with populated `TurnCount`
and `IterationCount` fields.

### Ghost Cues

**Location**: `internal/lcm/cue.go` (188 lines)

Transparent context markers injected into system prompts and tool results.
Cues are never shown to the user; they provide the LLM with lightweight
pointers into LCM-managed context. Three cue types:

- **CueTypeSummaryID** (`summary_id`): references a specific summary
  (`[Summary ID: {{.SummaryID}}]`).
- **CueTypeLineagePointer** (`lineage_pointer`): references a compaction
  lineage chain (`[Lineage: {{.ParentIDs}}, depth={{.Depth}}]`).
- **CueTypeArchiveStub** (`archive_stub`): references archived content
  (`[Archived: {{.FileID}}, tokens={{.TokenCount}}]`).

The **CueInjector** manages templates, renders cues via `{{.Var}}`
substitution, and injects them respecting priority ordering and a token
budget. Lower-priority cues are silently dropped when budget is constrained.

### Operational Memory

**Location**: `internal/session/om.go` (361 lines)

A per-session, per-thread key-value store backed by SQLite. Entries have
priority levels (**high**, **medium**, **low**) and support temporal relevance
decay via exponential half-life (1 hour). The **ThreadScopedOM** wrapper binds
a session+thread pair so callers do not repeat IDs on every call.

Key operations: `Set`, `Get`, `Delete`, `List`, `ListByPriority`,
`ListWithRelevance` (returns entries sorted by priority × relevance score).
`FormatThreadMemory` produces a human-readable summary with priority emojis
and relevance percentages. Controlled by the `operational_memory_enabled`
config key.

### Message Decorator

**Location**: `internal/lcm/message_decorator.go` (559 lines)

Wraps `message.Service` to intercept Create, Update, and List with LCM-aware
behaviour:

- **Large-output storage**: tool messages exceeding the token threshold
  (default 10K) are stored in `lcm_large_files` and replaced with a
  reference + 2000-char preview. Falls back to deterministic truncation
  (40K chars) when storage fails.
- **Explorer integration**: large tool outputs are explored via the
  RuntimeAdapter to generate structured summaries.
- **Token tracking**: persists per-message token counts and accumulates
  pending-item token deltas for threshold checks.
- **Compaction scheduling**: after each message creation, schedules async
  soft-threshold compaction.
- **Summary injection**: on List, rebuilds the message view from LCM context
  entries, injecting synthetic summary messages and preserving a stable
  prefix + append-only tail.

### Post-Compact Preservation

**Location**: `internal/lcm/post_compact.go` (403 lines)

**PreservedContext** captures operational context that must survive
compaction. The **PostCompactCleaner** (Layer 4) executes a 5-step restore
sequence after earlier layers free tokens:

1. Restore system prompt context.
2. Re-register active files with the LSP (via `FileRegistrar`).
3. Re-inject the repo map (via `MapInjector`).
4. Restore tool state to operational memory.
5. Restore agent configuration (skills, tools, agents) via
   `AgentConfigRestorer`.

A **PreservedContextStore** provides thread-safe save/load/delete keyed by
session ID, plus a separate "restored" key that survives the post-compact
clear for prompt assembly to read on the next cycle.

### Reversible Compaction

**Location**: `internal/lcm/reversible.go` (345 lines)

Saves original messages alongside compressed summaries so selected "anchor"
summaries can be decompressed on demand. Three detail levels:

- **full**: complete original messages.
- **partial**: first 200 characters of each message.
- **metadata**: only role, sequence number, and ID (no content).

Supports nested placeholder resolution: `(bN)` placeholders in compressed
content are recursively expanded via a `BlockResolver`, up to 5 levels deep
with cycle detection. **Recompress** reverses a decompression by clearing
stored original content while preserving the ancestry chain.

### Compressor

**Location**: `internal/lcm/compressor.go` (295 lines)

Defines the **CompressionStrategy** interface for LLM-based compression with
pluggable strategies:

- **RangeCompression** (ratio 0.3): extracts structured key-value ranges from
  line-oriented output.
- **MessageCompression** (ratio 0.4): compresses conversation messages while
  preserving decisions, technical details, and action items.
- **DedupCompression** (ratio 0.5): removes duplicate information, keeping
  the most complete or recent occurrence.
- **PurgeErrorsCompression** (ratio 0.6): removes resolved error output and
  debugging trails while retaining resolutions.

**ContextLimits** describes token budget constraints (max tokens, reserve,
summary budget) with pressure threshold calculations. Used by the graduated
pressure system to select strategies as the context window fills.

### Graduated Pressure System

**File**: `internal/lcm/pressure.go` (~364 lines) — Graduated pressure system
with tiered compaction strategies.

The graduated pressure system maps context-window usage to compaction
strategies, progressively escalating compression aggressiveness as tokens
approach the window limit.

**PressureTier** (`int`): exactly 3 tiers via iota:

| Tier | Value | Trigger | Strategies |
|------|-------|---------|------------|
| `PressureLow` | 0 | Minimal pressure (≥soft offset from limit) | `PurgeErrorsCompression` only |
| `PressureMedium` | 1 | Moderate pressure (≥compact offset from limit) | `PurgeErrorsCompression` + `DedupCompression` + `MessageCompression` |
| `PressureHigh` | 2 | Critical pressure (≥hard offset from limit) | All four strategies |

**PressureConfig** struct holds configurable thresholds that map context usage
to pressure tiers:

- `UseAbsoluteOffsets` (default `true`): absolute token offsets from context
  window limit vs legacy percentage mode
- `SoftOffset` (default 20000): tokens reserved below limit where Low pressure
  begins
- `CompactOffset` (default 13000): tokens reserved below limit where Medium
  begins
- `HardOffset` (default 3000, enforced floor 1000): tokens reserved below
  limit where High begins
- Legacy percentage fallback: `LowThreshold=70`, `MediumThreshold=85`,
  `HighThreshold=95`

Key functions and methods:

| Name | Description |
|------|-------------|
| `DefaultPressureConfig()` | Returns standard configuration with absolute offsets (SoftOffset=20000, CompactOffset=13000, HardOffset=3000) |
| `CalculatePressure(currentTokens, contextWindow)` | Computes raw pressure percentage (0–100) |
| `CalculatePressureTier(currentTokens, contextWindow, cfg)` | Determines tier from token usage, context window, and config; returns pressure % and tier |
| `NewGraduatedPressureSystem(cfg, limits, llm)` | Constructs system with default tier→strategy mapping |
| `StrategiesForTier(tier)` | Returns `[]CompressionStrategy` registered for the given tier |
| `StrategiesForTokens(currentTokens)` | Returns strategies for the tier corresponding to the given token count |
| `TierForTokens(currentTokens)` | Determines tier from raw token count using ContextLimits and PressureConfig |

Four compression strategies (defined in `compressor.go`):

- **RangeCompression** (ratio 0.3): extracts structured key-value ranges from
  line-oriented output
- **MessageCompression** (ratio 0.4): compresses messages preserving decisions
  and technical details
- **DedupCompression** (ratio 0.5): removes duplicate information, keeping most
  complete occurrence
- **PurgeErrorsCompression** (ratio 0.6): removes resolved error output while
  retaining resolutions

**PressureCompactionSelector** (priority 5) implements
`CompactionLayer`. It selects compaction sub-layers per tier rather than
individual strategies. Sub-layers are supplied at construction time via
`map[PressureTier][]CompactionLayer`.

Exact tier dispatch:
- **Low pressure**: runs `micro-compactor` only
- **Medium pressure**: runs `session-compactor` (priority 20) then `micro-compactor`
- **High pressure**: runs `full-compactor` (priority 30) then `micro-compactor`

### Provider Token Override

`SetActualPromptTokens(sessionID string, tokens int64)` on `compactionManager`
records the provider-reported prompt token count after each LLM call, resetting
the pending-item delta. This override is critical for accurate compaction
triggering: provider-reported counts replace local estimates, ensuring pressure
calculations reflect real token usage rather than approximations. Paired with
`AddPendingItemTokens` (which accumulates estimated tokens for messages created
since the last provider report), it maintains a live token estimate between
LLM calls.

### Observation Strategy

**Location**: `internal/lcm/observation_strategy.go` (112 lines)

Pluggable strategy interface controlling how observations are filtered and
formatted. Two implementations:

- **DefaultStrategy**: observes every event, JSON encoding, no compression.
- **ResourceScopedStrategy**: checks `runtime.MemStats` before allowing an
  observation. When `Alloc` exceeds `AllocFraction × Sys` (default 0.8 =
  80%), observations are skipped to avoid adding memory pressure. Uses mild
  compression (level 1).

Selected via the `observation.strategy` config key (`"default"` or
`"resource-scoped"`).

### LCM System Prompt

**Location**: `internal/lcm/prompt.go` (170 lines)

The system prompt injected when LCM is active. Instructs the LLM on silent
operation (never mention LCM internals), how hierarchical summaries work,
and when to use LCM retrieval tools (`lcm_grep`, `lcm_describe`,
`lcm_expand`). Documents ID types (`file_*`, `sum_*`) and provides guidance
on task sub-agent delegation (including infinite-recursion prevention).
Also documents `llm_map` and `agentic_map` tools when available.

### Time-Gap Compactor

**Location**: `internal/lcm/time_gap_compactor.go` (269 lines)

Sub-layer running after all priority-1–5 layers (priority 15). Detects time gaps >30 seconds between consecutive messages
and compacts older tool-result messages that fall before those gaps. Targets
scenarios where the user stepped away or context-switched, making prior tool
outputs less relevant. Replaces compacted content with archive stubs at ~10%
of the original token cost.

### Session Compactor

**Location**: `internal/lcm/session_compactor.go` (270 lines)

Sub-layer of PressureCompactionSelector (priority 20) that compiles the full session history into a
structured memory document via the LLM. Produces markdown with four sections:
**Decisions**, **Patterns**, **Errors**, **Current State**. Targets a token
range of 10K–40K output tokens, scaled by context pressure. Only activates
when the session has ≥50K context tokens and is under pressure. Skips if a
session-memory summary already exists. Caps prompt input at 200 context
entries to avoid exceeding LLM token limits.

### User-Facing Description

LCM keeps conversations from running out of memory. As your chat grows toward
the model's context window limit, LCM automatically triggers a 9-layer
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

### Additional Explorer Files

Five additional files supporting the explorer subsystem:

- **File**: `protocol_artifacts.go` (~399 lines) — Parity/conformance testing types for cross-platform consistency
- **File**: `extensions.go` (~97 lines) — File type classification maps and extension-to-handler dispatch
- **File**: `explorer_prompts.go` (~124 lines) — Agent exploration prompt templates for deep analysis tier
- **File**: `runtime.go` (~104 lines) — Runtime dependency detection helpers
- **File**: `tempfile.go` (~23 lines) — Temporary file management for explorer processing

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
28 languages have compiled CGO grammar packages (27 distinct imports; OCaml
provides two registrations). 10 entries are query-only with no compiled
grammar: Common Lisp, D, Emacs Lisp, Elm, Fortran, R, Racket, Solidity,
Swift, Zig. Note: 6 additional languages (Elixir, Gleam, Kotlin, MATLAB,
QL, Udev) have compiled grammars in `parser.go` but lack `grammar_module`
entries in `languages.json`.

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
- `swarm_execute` tool provided by `SwarmExtension` in `internal/extensions/swarm_ext.go` (~189 lines) via `ext.ToolProvider`

### Teammate Pattern

Persistent role-based agents that maintain state across tasks.

- Predefined roles: `researcher`, `tester`, `reviewer`
- Shared mailbox messaging between teammates
- Independent tool surfaces per role

### Forked Agent

**File**: `forked.go` (~523 lines)

Deep-copied parent context for parallel branching.

- Deep-copies parent session context
- Mailbox messaging (buffered channel, capacity 64)
- Turn-limited execution (max 10 turns)
- Tool filtering (subset of parent tools)

### Additional Agent Files

Seven additional agent files not listed in the primary subsections above:

- **File**: `coordinator_xrush_recovery.go` (~72 lines) — Coordinator recovery: post-compaction config restore and interrupted session recovery. `RestoreAgentConfig()` restores checkpointed agent configuration (skills, tools, agents) after LCM compaction events. `RecoverSession()` finishes interrupted thinking blocks or tool calls after mid-stream disruptions (network error, crash). These are recovery hooks ensuring the multi-agent system resumes cleanly after disruptions.
- **File**: `usage_fallback.go` (~176 lines) — Token usage fallback heuristics when primary tracking is unavailable
- **File**: `agentic_fetch_tool.go` (~206 lines) — Unregistered tool for LLM-powered URL content analysis (see §14)
- **File**: `agentconfig.go` (~108 lines) — Per-subagent configuration struct
- **File**: `agent_tool.go` (~68 lines) — "agent" tool for spawning sub-agents (see §14)
- **File**: `event.go` (~51 lines) — Telemetry event helpers
- **File**: `orchestration_types.go` (~40 lines) — Shared types for Operator, Parallel, and Swarm orchestration patterns
- **File**: `errors.go` (~10 lines) — Sentinel error definitions

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
| `treesitter-validation` | Tree-sitter parsing and analysis |
| `processor` | Processor pipeline management |
| `orchestration` | Multi-agent coordination |
| `model_router` | Model routing and tier selection |
| `autofix` | Auto-fix loop integration |
| `diag-gate` | Diagnostic quality gates |
| `edit-advanced` | Enhanced edit operations (anchors, fuzzy, batch) |
| `rewind` | Turn-based snapshot and rewind |
| `doom-loop` | Doom loop detection |
| `swarm` | Swarm pattern coordination |
| `tool-surface` | Tool registry and surface descriptions |
| `resource-limits` | Per-agent resource budgets |
| `xrush-sessions` | Extended session management |
| `prompt-assembly` | System prompt assembly (see details below) |
| `lsp-tools` | LSP-powered code intelligence tools |
| `StepAdapter` | Step-level adapter for agent coordination *(Infrastructure Adapter — bridges `PrepareStepHook` message mutation with extension host lifecycle, not a feature extension)* |

### Prompt Assembly (`prompt-assembly`) — Implementation Details

**Location**: `internal/extensions/prompt_assembly_ext.go` (119 lines)

`PromptAssemblyExtension` implements `ext.Extension` + `ext.PromptHookProvider`.
It mutates the system prompt via a `PromptHook.SystemPromptModifier` callback
before each LLM step, appending context blocks as structured XML.

#### Thread Safety

- Protected by `sync.RWMutex` — concurrent prompt assembly is safe.
- `SetLCMExtension()` and `SetRepomapExtension()` acquire write locks for
  dependency injection at startup.
- `PromptHook()` acquires a read lock to check `active` status; the inner
  `SystemPromptModifier` closure acquires a separate read lock.

#### Bootstrap Wiring

- `SetLCMExtension(lcm *LCMExtension)` — injects the LCM extension for
  accessing context files and observation prompts.
- `SetRepomapExtension(ext *RepomapExtension)` — injects the repo map
  extension for cached map injection.
- Both are called during `app.go` startup, before any agent runs.

#### Context Injection Pipeline

The `systemPromptModifier` appends XML blocks in order:

1. **LCM context files** — `mgr.GetContextFiles()` returns named context
   files (e.g., AGENTS.md content). Each is wrapped as:
   ```xml
   <context name="...">
   file content here
   </context>
   ```

2. **Observation prompt** — `mgr.GetObservationPrompt(ctx, sessionID, budget)`
   returns formatted observation text (see below). Injected as:
   ```xml
   <context name="observations">
   observation text here
   </context>
   ```
   Budget controlled by `defaultObservationTokenBudget = 2000` (overridable
   via config).

3. **Repo map** — conditionally injected only when `repomap.isActive()` and
   `repomap.ShouldInjectMap(ctx, sessionID)` are true. Uses
   `LoadCachedMap(sessionID)` to retrieve the pre-generated map string and
   its token count. Wrapped as:
   ```xml
   <context name="repo-map">
   map content here
   </context>
   ```

If no blocks are appended (no LCM, no observations, no repo map), the
original system prompt is returned unmodified.

#### Observation Prompt Injection

**Source**: `internal/lcm/manager.go` → `GetObservationPrompt()`

`GetObservationPrompt(ctx, sessionID, tokenBudget)` retrieves stored
observations for the session and formats them within the token budget:

- **Priority-based selection**: observations are pre-sorted by the database
  query as `high → medium → low → info` (descending priority), then by
  creation time ascending within each priority tier.
- **Greedy token budget**: observations are included greedily until the
  cumulative token cost exceeds `tokenBudget`. Lower-priority observations
  are truncated first.
- **Default budget**: `defaultObservationTokenBudget = 2000` tokens.
- **Zero observations**: returns empty string (no XML block appended).

### Safety Features

- Panic-safe execution with recovery
- Tool deduplication across extensions
- Hook collection and aggregation
- `Bootstrap` / `Shutdown` lifecycle management
- **Lifecycle hooks** (`internal/agent/ext_hooks.go`): `agentHookMediator` bridges
  agent events to extensions, invoking `RunHook` (run start/end), `StepHook`
  (prepare step, step finish, stop condition), and `PromptHook` (prepare prompt,
  system prompt modifier) callbacks with panic-safe execution

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

### Loop Detection Module

**File**: `internal/agent/loop_detection.go` (~92 lines) -- SHA-256 hash-based tool call repetition detection

A supporting module that provides the low-level repetition-checking primitives
used by the doom loop system. It does not implement escalation or recovery
itself; it answers the question "is the agent repeating itself?"

**Constants**:

| Constant | Value | Purpose |
|----------|-------|---------|
| `loopDetectionWindowSize` | 10 | Number of recent steps examined |
| `loopDetectionMaxRepeats` | 5 | Repetition count that triggers a positive |

**Functions**:

- **`hasRepeatedToolCalls(steps []fantasy.StepResult, windowSize, maxRepeats int) bool`**
  Examines the last `windowSize` steps and returns `true` when any single
  tool-call signature appears more than `maxRepeats` times within that window.
  Returns `false` when there are fewer than `windowSize` steps.

- **`getToolInteractionSignature(content fantasy.ResponseContent) string`**
  Computes a stable, hex-encoded SHA-256 hash for all tool interactions in a
  single step. Pairs each tool call with its result (matched by `ToolCallID`)
  and hashes the concatenation of tool name `\x00` input `\x00` output `\x00`.
  Returns `""` for steps with no tool calls.

- **`toolResultOutputString(result fantasy.ToolResultOutputContent) string`**
  Converts a `ToolResultOutputContent` to a stable string for hashing. Handles
  text, error, and media result types via fantasy's `AsToolResultOutputType`
  type switch.

**Relationship to agent loop**: `hasRepeatedToolCalls` is called from the agent
loop (in `agent.go`) as a stop condition — when the agent detects that recent
steps contain repeated tool-call signatures beyond the configured threshold, it
halts the loop. `DoomLoopDetector.Detect()` has its own independent detection
chain using `detectExactPattern()` → `getToolInteractionSignature()`.
`getToolInteractionSignature` is also reused by `ProductiveLoopDetector`
(see below) to capture the interaction pattern for groups that pass the
output-diversity check.

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
a forced tool switch at 5 repeats, and agent termination at 7 repeats.

**File**: `internal/agent/productive.go` (~150 lines) -- Productive-loop awareness layer

A **"productive loop" exception** prevents false positives when the agent is
making meaningful progress through repeated similar operations. The
`ProductiveLoopDetector` wraps `DoomLoopDetector` and adds a second-pass
output-diversity scan that can downgrade severity when repeated tool calls
produce genuinely different results.

**Key types**:

- **`ProductiveLoopDetector`** -- Embeds `*DoomLoopDetector`. Its `Detect()`
  method runs two passes: first the underlying doom-loop classification, then
  an independent output-diversity scan over the same window.

- **`ProductiveLoopResult`** -- Extends `DoomLoopResult` with two additional
  fields: `IsProductive bool` and `UniqueOutputCnt int`.

**Detection algorithm**:

1. `Detect()` calls `DoomLoopDetector.Detect()` for the base classification.
2. `groupByToolOutputHash()` groups the window's steps by tool name, tracking
   unique output hashes via `hashOutput()` (SHA-256 of `toolName \x00 output`).
   Note: unlike `getToolInteractionSignature` which hashes inputs, the
   productive-loop check hashes **outputs** to measure result diversity.
3. For any tool with `count >= SoftThreshold` **and** `uniqueOutputs / count >= 0.5`
   (50% uniqueness threshold), the loop is considered productive.
4. When productive and the doom classifier returned `EscalationHard`, severity
   is **downgraded from hard to medium** (execution continues instead of
   terminating). The message is replaced with a productive-loop advisory.
   Soft and medium warnings still reach the LLM as normal.
5. `groupByToolOutputHash()` also captures the first 16 characters of
   `getToolInteractionSignature()` as `pattern` for result reporting.

**DoomExtension integration**: `ProductiveLoopDetector` satisfies the
`DoomExtension` interface (embedding `*DoomLoopDetector`), allowing it to be
plugged into the doom-loop escalation pipeline alongside other extensions.

Resource limits enforce per-subagent token, step, and duration budgets with
soft warnings at 80%.

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

**File**: `internal/agent/architect_plan.go` (187 lines)

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

### New LSP Agent Tools (11 fork-new; 12 total built)

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

> **Note**: `LSPToolsExtension.buildLSPTools()` creates 17 tools total (12 original + 4 added in T16 + 3 added in T17). `lsp_restart` is also registered as an upstream tool (in `buildTools()`), so only 16 are fork-new additions.

Supporting files: `lsp_symbolic.go` (shared symbol operations), `lsp_helpers.go`
(shared utilities). These are not standalone tools but are used by the tools above.

### Fork-Specific LSP Additions

Three additional files extend the upstream LSP manager and client with
fork-specific functionality.

**`manager_xrush_methods.go`** — Manager-level enhancements:

| Addition | Purpose |
|----------|---------|
| Priority system (`PriorityCritical`…`PriorityLow`) | Controls server startup order; critical servers (gopls, rust-analyzer, etc.) start first |
| `sortServersByPriority()` | Sorts server map by priority for ordered startup |
| `resolveAutoDownload()` | Resolves auto-download paths for LSP server binaries at startup |
| `userMatchPatterns()` | Returns user-configured glob patterns for file-to-server matching |
| `Close()` | Graceful shutdown: stops health checker, task executor, and all clients |
| `GetDiagnosticsForServer()` | Per-server diagnostics, serialised through the task executor |
| `FindReferencesForServer()` | Per-server find-references, serialised through the task executor |
| `StartAll()` | Concurrent startup of all configured servers via errgroup, sorted by priority |
| `SaveAllCaches()` | Persists document symbol caches from all running clients |
| `RestartLanguageServer()` | Restarts a specific server by name |
| `RequestFullSymbolTree()` | Returns recursive document symbols for a URI from the first matching client |
| `startCrashRecovery()` | Background goroutine that monitors for crashes and auto-recreates the client |
| `handleServerReadyFailure()` / `handleServerReadySuccess()` | Post-readiness hooks for cleanup or crash recovery activation |

**`manager_handles_xrush.go`** — File-matching with user patterns:

Extends upstream `handles()` logic with support for user-configured
`match_patterns`. `handlesWithPatterns()` and `handleFiletypeWithPatterns()`
check both traditional file-type suffixes and glob-based patterns via
`NamePathMatcher`, allowing more flexible file-to-server routing.

**`client_xrush_methods.go`** — Client-level LSP protocol methods:

Adds direct LSP protocol call support (`callLSP` field) and seven standard
LSP methods on the `Client` struct:

| Method | LSP Request |
|--------|-------------|
| `Definition()` | `textDocument/definition` |
| `Rename()` | `textDocument/rename` |
| `CodeAction()` | `textDocument/codeAction` |
| `Hover()` | `textDocument/hover` |
| `DocumentSymbols()` | `textDocument/documentSymbol` |
| `Completion()` | `textDocument/completion` |
| `Formatting()` | `textDocument/formatting` |

Also adds `IsAlive()` (process liveness check via `healthCheck` or
`client.IsRunning()`), helper types (`xrushClientFields`), and response
parsing utilities (`remarshal`, `parseLocationArray`, `parseCompletionResult`).

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

> **Note**: `web_search` and `web_fetch` are listed as upstream tools. Their fork-point classification could not be independently verified from the current codebase state.

> **Overlap note**: 4 of these 23 tools — `multiedit`, `sourcegraph`, `list_mcp_resources`, `read_mcp_resource` — also appear in `xrushToolNames()` (the 16-entry fork-specific tool list). They are genuinely upstream tools (registered in `buildTools()`) but were included in the fork configuration list so that the fork's ordering and permission logic can reference them uniformly. They appear in exactly one place in each classification; the dual listing reflects their dual role as upstream implementations with fork-specific configuration.

### Fork-New Tools

> **Tool surface**: The fork registers 56 tools via `tool_surface.go:registerDefaults()`. Of these, 23 are inherited from upstream. The fork adds tools via three mechanisms: `xrushToolNames()` (25 standard), `LSPToolsExtension.buildLSPTools()` (17 built; 16 fork-new as `lsp_restart` is also upstream), and `ExtraAgentTools()` (14: 5 via toolFactory + 9 retrieval). Additional tools `agent` and `agentic_fetch` extend the base tool set, and 4 orchestration tools (`send_message`, `task_stop`, `team_create`, `team_delete`) are registered outside registerDefaults. In total, the full tool surface across all registration mechanisms comprises 60 unique tool names.

#### Standard Registry Tools (25)

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
| `agentic_fetch` | Network | LLM-powered URL content analysis and extraction |
| `agent` | Orchestration | Spawn sub-agents for task delegation |

#### LCM Retrieval Tools (9 retrieval, via `ExtraAgentTools()`)

`lcm_bindle`, `lcm_ancestry`, `lcm_dolt`, `lcm_archive`, `lcm_sprig`,
`lcm_time_query`, `lcm_file_search`, `lcm_active_context`, `lcm_lineage`

#### Not Agent Tools (Internal Helpers)

`diag_autofix` and `diag_gate` are internal helper types used by the
AutoFix and DiagGate extension systems. They have no `New*Tool`
constructors and are not registered in the agent tool surface.

`diag_cascade` (304L) provides diagnostic cascading: after editing a
file, it runs LSP diagnostics and recursively checks importing files
(up to `defaultCascadeMaxDepth=3`). The `CascadeResult` struct tracks
`FilesChecked`, `FileDiagnostics` (map of file path → diagnostics),
and `HasWarnings` (whether any severity ≥ Warning was found). The
`findImporters()` method discovers importing files via LSP
`textDocument/references` across all registered LSP clients (best-effort;
querying at line 1, col 1 as a heuristic for the package declaration).
`FormatCascadeResult()` produces `<diagnostic_cascade>` XML output
listing file paths, severities, and messages. The helper
`cascadeDiagnosticsConcurrent()` runs diagnostics for multiple files in
parallel using `sync.WaitGroup`. It has no `New*Tool` constructor and is
called internally by edit/write tools via `runDiagnosticCascade()`.

`search` (219L) provides DuckDuckGo HTML scraping via
`lite.duckduckgo.com` with randomized headers and rate limiting. It has
no `New*Tool` constructor; the `web_search` tool (`NewWebSearchTool`)
delegates to it.

`safe` (90L) provides a whitelist of read-only shell commands (e.g.
`git status`, `ls`, `ps`) and command-chaining detection. It has no
`New*Tool` constructor; the `bash` tool (`NewBashTool`) uses it to
auto-approve safe commands.

### Fork-New Edit Support

| Component | Lines | Description |
|-----------|-------|-------------|
| `edit_anchors` | 193 + 116 | FNV-1a hash-based content-addressed anchors for drift-tolerant edits |
| `anchor_state_manager` | 237 | Myers diff algorithm for tracking anchor hash map drift across file modifications; `CaptureState()` snapshots anchors per file, `DetectDrift()` compares current vs. captured state via Myers diff returning `AnchorDrift` entries, `Reconcile()` produces updated anchor map from drift; `AnchorDrift` struct with `DiffOp` enum (`Keep`/`Insert`/`Delete`) and line-number `Shift`; `//go:build treesitter` with stub fallback |
| `edit_fuzzy` | 307 | Whitespace-normalized fuzzy string matching for approximate edit targets |
| `rollback` | 220 | File snapshot and rollback on failure |
| `validate` | 832 | Tree-sitter syntax validation after edits |
| `validation_handler` | 228 | Post-edit validation pipeline (orchestrates validate + rollback) |
| `view_xrush` | 176 | Enhanced batch file reading with LCM context awareness. Runs up to `batchMaxWorkers=8` concurrent goroutines via semaphore, tracks cumulative output with `atomic.Int64` against a `batchDefaultTokenBudget=200_000` (×4 chars/token = 800K char budget). Performs path deduplication (`dedupBatchPaths`) to avoid redundant reads. Graceful budget exhaustion: once the atomic counter reaches the budget, in-flight and pending reads are skipped without error, returning whatever content was collected so far. Supports offset/limit slicing and line numbering. |

### User-Facing Description

The fork provides 60 unique tools across all registration mechanisms: 56 via registerDefaults (23 inherited from upstream), plus 4 orchestration tools registered outside registerDefaults (`send_message`, `task_stop`, `team_create`, `team_delete`), and extension-provided tools (e.g., swarm_execute). Tools with overlapping names (e.g., lsp_restart) are counted once. The
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
| Config Store | 761 | Main `ConfigStore` implementation: owns pure-data `Config`, runtime state (working dir, resolver, known providers), file-change snapshots, auto-reload, and persistence to global/workspace config files |
| Docker MCP | 134 | Auto-detect Docker MCP gateway (`docker mcp version`), 10 s TTL cache, enable/disable methods on `ConfigStore` that persist to global config |
| Hyper Provider | 124 | Charm Hyper provider auto-configuration: fetches provider metadata from `/api/v1/provider`, ETag-based caching, embedded fallback, `sync.Once` init |
| Atomic Writes | 38 | Safe config file writes via temp-file + rename, preventing concurrent readers from seeing partial writes |
| Xrush Types | 61 | Fork-specific config types: `RoutingTier`, `ArchitectOptions`, `ValidationOptions`, `ProcessorsOptions`, `SnapshotConfig`, `AutoDownloadConfig` |
| Xrush Tools Registry | 95 | Fork-only tool name registry (`xrushToolNames`, `xrushReadOnlyTools`) merged into sorted `allToolNames` alongside extension-contributed tools |
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

The `ConfigStore` is the single entry point for all config access — it owns
the pure-data `Config`, runtime state (working directory, variable resolver,
known providers), and persistence to both global and workspace config files.
It supports auto-reload on file changes (with snapshot-based change detection)
and safe writes via atomic temp-file + rename.

Docker MCP auto-detection probes `docker mcp version` and caches the result
for 10 seconds. When available, the Docker MCP gateway can be enabled or
disabled through the config store, persisting to the global config file.

The Hyper provider module fetches Charm Hyper provider metadata from
`/api/v1/provider` with ETag-based conditional requests and a local cache
file. It falls back to an embedded provider definition when offline or
timed out, and initializes once via `sync.Once`.

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

18 new migrations added by the fork:

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
| `20260519000000_lcm_observation_priority_info.sql` | Add 'info' priority level to observation buffer |

### User-Facing Description

25 SQLite migrations are applied automatically on startup using the Goose
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

## 18. Hooks Engine

**Location**: `internal/hooks/`

The hooks engine runs user-defined shell commands on agent lifecycle events
(PreToolUse, PostToolUse, PreCompact, PostCompact), returning decisions that
control agent behavior. It builds on Crush's upstream hook infrastructure (see
`docs/hooks/`) with additional fork-specific extensions: PostToolUse output
rewriting, compaction hooks, and deeper integration with the coordinator via
the `HookedTool` decorator.

### Hook Events & Decision Types

**Location**: `internal/hooks/hooks.go` (207 lines)

Four event constants trigger hook execution at different agent lifecycle
points:

| Event | Description |
|-------|-------------|
| `PreToolUse` | Fires before every tool call. Can block, allow, or rewrite tool input. |
| `PostToolUse` | Fires after every tool call (fork addition). Non-blocking; primary use is output rewriting/sanitization. |
| `PreCompact` | Fires before LCM compaction (fork addition). |
| `PostCompact` | Fires after LCM compaction (fork addition). |

Three decision types control how hooks influence agent behavior:

| Decision | Effect |
|----------|--------|
| `DecisionNone` | Hook expressed no opinion. Falls through to normal flow. |
| `DecisionAllow` | Explicitly allows the action. For PreToolUse, pre-approves the permission prompt. |
| `DecisionDeny` | Blocks the action. For PreToolUse, prevents the tool call. |

Key types:

- `HookResult` — parsed output of a single hook: `Decision`, `Halt`, `Reason`,
  `Context`, `UpdatedInput`, `UpdatedOutput` (fork: PostToolUse rewriting).
- `AggregateResult` — combined outcome of all hooks for an event: merged
  decisions, concatenated reasons/context, shallow-merged input patches,
  last-writer-wins output replacement.
- `HookMetadata` / `HookInfo` — embedded in tool response metadata for UI
  display.
- `HaltExitCode` (49) — exit code that halts the whole turn; chosen to avoid
  collisions with generic-error (1–30), BSD sysexits (64–78), and signal ranges
  (128+).

Aggregation rules (in `aggregate()`): deny wins over allow, allow wins over
none. Halt is sticky. Reasons and context concatenate in config order.
`updated_input` patches shallow-merge sequentially. `updated_output` uses
last-writer-wins semantics (fork addition for PostToolUse).

### Hook Runner

**Location**: `internal/hooks/runner.go` (265 lines)

The `Runner` executes hook commands and aggregates results:

1. **Construction** (`NewRunner`): accepts `[]HookConfig`, compiles each
   `Matcher` regex at construction time. Hooks with invalid matchers are
   skipped with a warning (defense in depth beyond `ValidateHooks`).
2. **Matching** (`matchingHooks`): filters hooks whose matcher matches the
   tool name; nil matcher matches everything.
3. **Deduplication**: by command string so identical commands run once.
4. **Parallel execution** (`Run`): all matching hooks run concurrently via
   goroutines with a `sync.WaitGroup`. Each hook receives environment variables
   and a JSON stdin payload.
5. **Single-hook execution** (`runOne`): runs a hook through Crush's embedded
   POSIX shell (`shell.Run`). Uses context-based timeout with an
   `abandonGrace` (1 second) period for non-yielding goroutines. Exit codes:
   0 = parse stdout JSON, 2 = deny (stderr = reason), 49 = halt turn, other =
   non-blocking warning.

### Input Builder

**Location**: `internal/hooks/input.go` (209 lines)

Constructs the data provided to hook commands via two channels:

**Environment variables** (`BuildEnv`): extends `os.Environ()` with
hook-specific variables:

| Variable | Description |
|----------|-------------|
| `CRUSH_EVENT` | Hook event name (e.g. `PreToolUse`) |
| `CRUSH_TOOL_NAME` | Tool being called (e.g. `bash`) |
| `CRUSH_SESSION_ID` | Current session ID |
| `CRUSH_CWD` | Working directory |
| `CRUSH_PROJECT_DIR` | Project root directory |
| `CRUSH_TOOL_INPUT_COMMAND` | For `bash` calls: the shell command |
| `CRUSH_TOOL_INPUT_FILE_PATH` | For file tools: the target file path |

**JSON stdin payload** (`BuildPayload`): structured JSON with `event`,
`session_id`, `cwd`, `tool_name`, and `tool_input` fields.

**Output parsing** (`parseStdout`): handles both Crush format and Claude Code
compatibility (`hookSpecificOutput` wrapper). Supports envelope versioning
(`version` field, default 1). Parses `decision`, `halt`, `reason`, `context`
(string or string array), and `updated_input` from hook stdout.

### Fork Additions: PostToolUse & Output Rewriting

**Location**: `internal/hooks/runner_xrush.go` (138 lines) +
`internal/hooks/input_xrush.go` (68 lines)

The fork extends the hooks engine with PostToolUse support and additional
lifecycle events:

**PostToolUse runner** (`RunPostToolUse`): executes matching hooks after a
tool completes. Unlike PreToolUse, PostToolUse hooks are non-blocking — exit
codes 2 and 49 do NOT halt or deny (the tool already ran). The primary use
case is output rewriting/sanitization via the `UpdatedOutput` field.

**PostToolUse payload** (`PostPayload`): extends the base `Payload` with:

| Field | Description |
|-------|-------------|
| `tool_output` | The tool's output text |
| `duration_ms` | Tool execution duration in milliseconds |

**PostToolUse environment** (`BuildPostEnv`): extends `BuildEnv` with:

| Variable | Description |
|----------|-------------|
| `CRUSH_TOOL_OUTPUT` | The tool's output text |
| `CRUSH_TOOL_DURATION_MS` | Execution time in milliseconds |

**PostToolUse output parsing** (`parsePostStdout`): extracts
`modified_output` from hook stdout. If the hook returns valid JSON with a
`modified_output` field, that text replaces the tool's original output. If
stdout is valid JSON without the field, no replacement occurs. If stdout is
not valid JSON, it is used as-is (plain text replacement).

### HookedTool Decorator

**Location**: `internal/agent/hooked_tool.go` (163 lines)

The `hookedTool` struct wraps `fantasy.AgentTool` to inject hook execution
into the tool call pipeline at the coordinator level:

1. **PreToolUse**: runs PreToolUse hooks. If deny/halt, returns error response
   with `StopTurn` set for halts. If allow, injects `permission.WithHookApproval`
   into context to bypass the permission prompt. If `UpdatedInput`, rewrites
   the tool call input.
2. **Tool execution**: delegates to the inner tool with (possibly rewritten)
   input.
3. **PostToolUse**: runs PostToolUse hooks. If `UpdatedOutput` is returned,
   replaces the tool response content.
4. **Metadata**: merges `HookMetadata` into the tool response so the UI can
   display hook indicators.

`wrapToolsWithHooks` wraps all tools for the top-level agent. Sub-agents are
explicitly excluded (`isSubAgent` check) so hooks only fire once per
top-level invocation, not N times for N sub-agent calls.

### User-Facing Description

Hooks let you run custom shell scripts at key points in the agent lifecycle.
Configure them in `crush.json` to block dangerous commands, rewrite tool
input/output, auto-approve safe tools, inject context reminders, or sanitize
tool results. Hooks run in parallel for speed, but their results compose in
config order for determinism. The fork extends upstream hooks with PostToolUse
(output rewriting/sanitization) and compaction hooks (PreCompact/PostCompact).

### Configuration

```jsonc
{
  "hooks": {
    // Fires before every tool call. Can block, allow, or rewrite input.
    "PreToolUse": [
      {
        // Regex matched against tool name. Omit to match all tools.
        "matcher": "^bash$",
        // Shell command to run. Scripts use embedded POSIX shell.
        "command": "./hooks/my-hook.sh",
        // Timeout in seconds (default: 30).
        "timeout": 10
      }
    ],
    // Fires after every tool call. Non-blocking; for output rewriting.
    "PostToolUse": [
      {
        "matcher": "bash",
        "command": "./hooks/sanitize-output.sh"
      }
    ],
    // Fires before LCM compaction.
    "PreCompact": [],
    // Fires after LCM compaction.
    "PostCompact": []
  }
}
```

Hooks can be added at both project-level (`crush.json`) and global-level
(`~/.config/crush/crush.json`), with project hooks taking precedence. See
`docs/hooks/README.md` for the full hook guide including input/output formats,
exit codes, aggregation rules, and Claude Code compatibility.

### Usage

Hooks are configured entirely through `crush.json`. No CLI commands are
needed. When a tool is called, the `HookedTool` decorator automatically runs
matching PreToolUse hooks before the tool executes and PostToolUse hooks after.
The hook indicator appears in the TUI via response metadata. Sub-agents do not
fire hooks independently — only the top-level agent's tool calls are hooked.

### Default Behavior

When no hooks are configured in `crush.json`, the hooks engine is completely
inactive. All tool calls proceed through the normal permission flow without
any hook interception. The `HookedTool` decorator passes through to the inner
tool unchanged when both `preRunner` and `postRunner` are nil.

---

## 19. Shell Enhancements

**Location**: `internal/shell/`

Cross-platform shell execution enhancements providing embedded `jq`, shell-style
variable expansion, background job management, platform-aware command dispatch,
and a core utilities flag -- all operating within the embedded POSIX shell
interpreter (mvdan/sh).

### Embedded jq

**File**: `jq.go` (342 lines)

Embedded gojq (v0.12.19) providing full `jq` functionality within the shell
without requiring an external binary. Available as a shell builtin in all
command execution contexts: interactive commands, hooks, and command
substitution.

Supported flags: `-r` (raw output), `-j` (join output), `-c` (compact output),
`-s` (slurp), `-n` (null input), `-e` (exit status), `-R` (raw input),
`--arg name value`, `--argjson name value`.

Context-aware cancellation: long-running `jq` queries are interruptible via
context cancellation at each output iteration and each reader boundary, so hook
timeouts never block indefinitely.

### Variable Expansion

**File**: `expand.go` (145 lines)

Shell-style variable expansion for configuration values (`crush.json`,
environment files, hook inputs). Supports:

- `$VAR` and `${VAR}` -- basic variable substitution
- `${VAR:-default}` -- default value when unset or empty
- `${VAR:+alt}` -- alternative value when set and non-empty
- `${VAR:?message}` -- error with message when unset or empty
- `$(command)` -- command substitution with full quoting and nesting
- Escaped and quoted strings (`"..."`, `'...'`)

Expansion is used across MCP server configuration, provider credentials, and
hook payloads. No field splitting, no globbing, no pathname generation --
output is preserved verbatim.

### Background Jobs

**File**: `background.go` (254 lines)

Singleton `BackgroundShellManager` manages up to 50 concurrent background shell
instances. Each job runs in its own shell with independent stdout/stderr buffers
(thread-safe). Features:

- Start, monitor, and kill background commands
- Retrieve partial output while running, full output on completion
- Automatic cleanup of completed jobs after 8 hours
- Graceful kill with 5-second timeout
- `KillAll` for batch shutdown with context-bound waits

### Platform-Aware Command Dispatch

**File**: `dispatch.go` (423 lines)

Exec-handler middleware that intercepts path-prefixed commands (e.g.,
`./foo.sh`, `/opt/bin/tool`, `C:\script.ps1`) and dispatches based on file
contents:

1. **Shebang** (`#!...`) -- Resolves the interpreter, with permissive PATH
   fallback (e.g., `#!/bin/bash` works on Windows via Git for Windows PATH).
   Supports `/usr/bin/env` rewriting with `-S` flag for tokenized argument
   splitting.
2. **Binary** (MZ, ELF, Mach-O magic bytes or NUL in probe window) -- Passes
   through to the default exec handler.
3. **Shell source** -- Parses as POSIX shell and runs in-process via a nested
   interpreter, reusing the same handler stack so builtins and block rules apply
   recursively. Positional parameters (`$1`, `$2`, ...) are forwarded from the
   invoking command.

Only the first 128 bytes are probed; the full file is read only for the
shell-source branch, keeping I/O bounded for binaries and shebang scripts.

### Core Utilities Flag

**File**: `coreutils.go` (19 lines)

The `CRUSH_CORE_UTILS` environment variable controls whether Go-based core
utilities are used instead of system binaries. Defaults to enabled on Windows,
disabled on all other platforms. Set `CRUSH_CORE_UTILS=true` to force Go
coreutils, or `CRUSH_CORE_UTILS=false` to disable them.

### User-Facing Description

The shell enhancements provide three user-visible capabilities. First, `jq` is
available as a builtin command in all shell contexts -- no external `jq`
binary needed. This means hooks, command substitution, and the agent's bash
tool can pipe JSON through `jq` on any platform, including Windows. Second,
`crush.json` values support shell-style variable expansion (`$VAR`,
`${VAR:-default}`, `$(command)`) for credentials, API keys, and dynamic
configuration -- the same syntax works identically across macOS, Linux, and
Windows. Third, long-running commands can run as background jobs that are
started, monitored, and killed independently, with output available for
retrieval at any time.

### Configuration

```jsonc
// Shell enhancements are controlled via environment variables:

// Force Go-based core utilities (default: true on Windows, false elsewhere).
// Set to "true" or "false" to override the platform default.
// CRUSH_CORE_UTILS=true

// Strict unset-variable mode for variable expansion.
// Default: unset variables expand to "" (bash behavior).
// When enabled via internal escape hatch, unset $VAR is an error.
// Not exposed in crush.json.
```

No `crush.json` configuration section is required. The `CRUSH_CORE_UTILS`
environment variable is the only user-facing configuration knob.

### Usage

Shell enhancements are transparent. Use `jq` in any command context (hooks,
bash tool, command substitution) as you would externally:

```bash
# jq builtin -- no external binary needed
echo '{"name": "crush"}' | jq '.name'

# Variable expansion in crush.json
// "api_key": "$MY_API_KEY"
// "api_key": "${MY_API_KEY:?set MY_API_KEY}"
// "api_key": "$(cat /path/to/key)"
```

Background jobs are managed programmatically by the agent and hook system. The
dispatch handler activates automatically for path-prefixed commands (scripts).

### Default Behavior

All shell enhancements are active by default. `jq` is always available as a
builtin. Variable expansion applies to all `crush.json` values, MCP
configuration, provider credentials, and hook payloads. Background jobs allow
up to 50 concurrent instances with 8-hour auto-cleanup. Command dispatch
activates automatically for any command referencing a file path. Go coreutils
are enabled by default on Windows and disabled on Unix; override via
`CRUSH_CORE_UTILS`.

---

## 20. TUI Enhancements

**Location**: `internal/ui/`

User-visible TUI additions for fork features: compaction status, message
actions, repo map refresh, and message routing.

### Components

| File | Lines | Description | User-Visible Behavior |
|------|-------|-------------|-----------------------|
| `diffview/` | 1076 | Unified and split diff rendering | Fluent-builder component rendering inline or side-by-side diffs with syntax highlighting, line numbers, scroll, and file-name headers; used in tool permission denied flow |
| `xchroma/chroma.go` | 52 | Custom Chroma syntax highlighting formatter | Wraps Chroma tokenization with Lip Gloss styling and forced background; used by DiffView and markdown renderer |
| `model/compaction.go` | 69 | LCM compaction status pill | Animated "⟳ Compacting" pill with elapsed time in the status bar while LCM is compacting |
| `model/xrush_routing.go` | 169 | Message routing and dialog actions | Routes rewind results, compaction events, edit-message results, and delayed clicks through the main update loop |
| `model/repomap_xrush.go` | 42 | Repo map refresh from command palette | Triggers async repo map refresh; shows success or error notification |
| `dialog/actions_xrush.go` | 29 | Extended action menu entries | Adds Rewind, Fork, Edit Message, and Message Options actions to the per-message action dialog |
| `chat/user_xrush.go` | 6 | User message sequence accessor | Exposes message sequence number for rewind/fork/edit targeting on user messages |

### DiffView

**File**: `internal/ui/diffview/` (~1076 lines across 4 files) -- Unified and
split diff rendering for TUI

| File | Lines | Description |
|------|-------|-------------|
| `diffview.go` | 823 | Core `DiffView` component: computes diffs via `go-udiff`, renders unified and split layouts with optional syntax highlighting, line numbers, file-name headers, and scroll |
| `split.go` | 73 | `hunkToSplit` conversion: aligns before/after lines from a unified hunk into paired `splitLine` entries for side-by-side rendering |
| `style.go` | 141 | `Style` and `LineStyle` structs with `DefaultDarkStyle()` / `DefaultLightStyle()` using charmtone palette colors for divider, equal, insert, delete, missing, and filename lines |
| `util.go` | 39 | Small helpers: `pad`, `isOdd`, `btoi`, `ternary` |

The `DiffView` component renders file differences in two layouts:

- **Unified** (default): traditional `+`/`-` inline diff with before and after
  line numbers side by side.
- **Split** (side-by-side): two-column layout where deletions appear on the
  left and insertions on the right, with missing lines rendered as empty
  placeholders.

Key features:

- Configurable context lines (defaults to `udiff.DefaultContextLines`).
- Optional line numbers with auto-detected digit width.
- Infinite Y-scroll mode (`InfiniteYScroll`) or clamped scrolling that prevents
  scrolling past the last line.
- X-offset scrolling for long lines, with leading ellipsis indicator.
- Configurable tab width (default 8).
- File-name header displayed before the first hunk.
- Syntax highlighting via `xchroma` with per-line caching (xxh3 hash key).
- Truncation ellipsis when the viewport height is reached before all hunks are
  displayed.

The component uses a fluent builder API (`New().Before(...).After(...).Split()`)
and computes lazily on `String()`. It is invoked from the tool permission
denied flow to show the user a diff of what the tool intended to do.

### xchroma Syntax Highlighting

**File**: `internal/ui/xchroma/chroma.go` (~52 lines) -- Custom Chroma syntax
highlighting formatter for TUI

`xchroma.Formatter` returns a `chroma.FormatterFunc` that wraps the Chroma
tokenization pipeline with Lip Gloss styling. For each token it resolves the
Chroma style entry and maps bold, underline, italic, and foreground colour to
the corresponding Lip Gloss style methods, while forcing a caller-specified
background colour. An optional `processValue` callback allows post-processing of
token values (e.g., ANSI escaping) before rendering.

Used by `DiffView` for syntax-highlighted code lines and by the markdown
renderer for fenced code blocks.

### User-Facing Description

Six TUI enhancements are visible to users:

1. **DiffView**: When a tool call is denied, a diff view shows what the tool
   intended to change. The view supports both unified (inline) and split
   (side-by-side) layouts, toggled by `options.tui.diff_mode` in `crush.json`.

2. **Syntax highlighting**: Code in diffs and markdown fenced blocks is
   highlighted using a custom Chroma formatter (`xchroma`) that integrates with
   Lip Gloss terminal styling.

3. **Compaction pill**: When LCM starts compacting the conversation, an animated
   "⟳ Compacting" pill with elapsed seconds appears in the status bar. It
   disappears when compaction completes.

4. **Message actions**: Press `o` on any user message to open an action menu
   with four new options: **Rewind** (restore code, conversation, or both to
   that point), **Fork** (branch the conversation from that message), **Edit
   Message** (load the original text into the editor for re-submission), and
   **Message Options** (opens a detailed dialog for the selected message).

5. **Repo map refresh**: The command palette (Ctrl+P) includes a "Refresh
   Repository Map" entry that triggers an asynchronous repo map rebuild and
   shows a success or error notification.

6. **Delayed click handling**: Click handling on chat messages is deferred to
   ensure the correct message is targeted, improving reliability of click-based
   interactions in the message list.

### Configuration

No configuration is required. All TUI enhancements are active by default. The
compaction pill appears automatically when LCM is configured and compaction is
running. Message actions require the rewind system to be enabled via
`options.snapshot` in `crush.json`.

The diff view layout can be configured via `options.tui.diff_mode` in
`crush.json`:

```jsonc
{
  "options": {
    "tui": {
      // "unified" (default) for inline +/- diff, "split" for side-by-side.
      "diff_mode": "unified"
    }
  }
}
```

### Usage

- **DiffView**: Automatically shown when a tool call is denied, displaying what
  the tool intended to change. Set `"diff_mode": "split"` in
  `options.tui` for side-by-side layout.
- **Compaction pill**: Visible automatically in the status bar when LCM
  compaction is running. No user action needed.
- **Message actions**: Navigate to a user message in the chat and press `o` to
  open the action menu. Select Rewind, Fork, Edit Message, or Message Options.
- **Repo map refresh**: Open the command palette with Ctrl+P and search for
  "Refresh Repository Map".
- **Message click**: Single-click on user messages to open the message options
  dialog directly.

### Default Behavior

The compaction pill is visible whenever LCM compaction is active (requires LCM
to be configured). Message actions are available on all user messages when the
rewind system is enabled (`options.snapshot` in `crush.json`). The repo map
refresh command is always available in the command palette. If rewind is not
enabled, pressing `o` shows a "Rewind is not available" warning. The diff view
defaults to unified layout; set `"diff_mode": "split"` to switch to
side-by-side. Syntax highlighting in diffs and markdown is always active.

---

## Appendix: Additional Components

| Component | Description |
|-----------|-------------|
| `CacheShare` (128L) | Cross-agent cache sharing via colon-separated string keys |
| `RateLimiting` (180L) | Reactive 429-backoff rate limit coordination |
| `ModelRouter` + `TierRouter` | Model routing with tier-based selection; `ModelRouter` deprecated, `TierRouter` active |
| `ConfigLoader` (253L) | Dynamic agent configuration from `crush.json` |
| `ToolSurface` (414L) | Tool registry with 6-capability bitmask, 6 behavioral markers, dynamic visibility, and phase filtering; 56 tools registered |
| `Session` (349L) | Session management (`session/session.go`) |
| `Completer` (88L) | Shell command completion |
| `WrittenFiles` (79L) | Track files written during a session |
| Background shell timeout | Timeout for background shell command execution |
| `regexp_modernc` / `ncruces` adapters | Pure-Go regexp adapters (no CGO dependency for regex) |
| `vacuum_guard` | Prevent SQLite vacuum during active operations |
| `app_xrush_wiring` (~400L) | Top-level fork wiring: `initRewindService`, `wireAgentConfigRestorer` — connects rewind, LCM, extensions, hooks, and explorer subsystems at app startup |
| `filetracker/service_xrush_lcm.go` (~35L) | LCM integration for file tracking service |
| `skills/tracker_xrush.go` (~17L) | Skill discovery tracking for xrush extensions |
| `workspace/app_workspace_xrush.go` (~7L) | App workspace LCM integration hooks |
| `workspace/client_workspace_xrush.go` (~10L) | Client workspace LCM integration hooks |

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
- **ToolSurface** maintains the tool registry (56 built-in tools) and
  generates surface descriptions for agent prompts. Each tool carries a
  6-bit capability bitmask (`FS`, `Network`, `CodeIntelligence`, `Execution`,
  `Memory`, `Observation`) and optional behavioral markers (`CanEdit`,
  `SymbolicRead`, `SymbolicEdit`, `Optional`, `Beta`,
  `DoesNotRequireActiveProject`). Dynamic visibility hides tools whose
  runtime dependencies are unsatisfied (e.g., LSP tools when no LSP is
  running, LCM tools when LCM is disabled). Phase filtering further hides
  write tools (`edit`, `multiedit`, `write`) during the Planning phase,
  classifying phases by keyword analysis of the current prompt.
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
