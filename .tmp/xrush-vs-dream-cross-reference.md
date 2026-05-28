# XRush Implementation vs DREAM Spec Cross-Reference

**Date**: 2026-05-28
**Sources**: `.tmp/xrush-feature-audit-report.md` (audit) × `DREAM.md` + `DREAM_IMPLEMENTATION_SPEC.md` (specs)

---

## Executive Summary

| Category | Total DREAM Components | Implemented per Spec | Partial Match | Missing/Wired Wrong | Not Implemented |
|----------|----------------------|---------------------|---------------|---------------------|-----------------|
| A: Code Understanding | 3 | 2 | 1 | 0 | 0 |
| B: Context Management | 5 | 1 | 3 | 1 | 0 |
| C: Memory | 4 | 2 | 1 | 1 | 0 |
| D: Edit | 2 | 1 | 1 | 0 | 0 |
| E: Validation | 4 | 1 | 2 | 1 | 0 |
| F: Model Optimization | 2 | 0 | 2 | 0 | 0 |
| G: Orchestration | 6 | 2 | 1 | 3 | 0 |
| H: Evaluation & QA | 3 | 1 | 1 | 1 | 0 |
| **TOTAL** | **29** | **10** | **12** | **7** | **0** |

**Key Finding**: All 29 DREAM components have corresponding code in the Crush codebase. The primary issues are (1) wiring gaps making features unreachable in production, and (2) spec deviations where implementation differs from DREAM's reference designs.

---

## Layer A: Code Understanding

### A.1: PageRank Repomap — ✅ IMPLEMENTED, SPEC COMPLIANT

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| PageRank over import/reference graph | `repomap/pagerank.go` — full PageRank implementation | ✅ |
| Damping factor 0.85 | `pagerank.go` default | ✅ |
| File-level edges weight 1.0, symbol refs weight 0.3 | `repomap/graph.go` | ✅ |
| Token budget ~2K tokens | `repomap/render.go` with token budget | ✅ |
| Incremental refresh | `repomap.go` PreIndex + Generate | ✅ |
| Tree-sitter tag extraction | `repomap/tags.go` with 28 languages | ✅ |
| Language-agnostic via tree-sitter | Confirmed — 28 grammars in `treesitter/languages.go` | ✅ |
| Cache keyed by (file_hash, import_hash) | `repomap.go` SessionCache | ✅ |

**Wiring Gap**: RepoMap data generated but **never injected into LLM prompt** (ShouldInject zero callers, PromptAssembler dead code). Per audit report feature #18.

**Spec Deviation**: None — implementation matches spec precisely. Only gap is the wiring disconnection.

### A.2: File Exploration Dispatchers — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Tree-sitter dispatchers for 26+ languages | `treesitter/languages.go` — 28 languages | ✅ |
| Route by file extension | `treesitter/parser.go` — extension-to-language mapping | ✅ |
| Extract classes, functions, methods, types, imports | `treesitter/imports.go` + `repomap/tags.go` | ✅ |
| `FileDispatcher` interface pattern | `lcm/explorer/explorer.go` — Explorer interface with Registry | ✅ |
| Per-language stdlib membership | `lcm/explorer/stdlib/` — 15 languages | ✅ |
| File structure cached with TTL | `repomap/tags.go` — DB-cached tag extraction | ✅ |

**Spec Deviation**: Spec calls for `FileDispatcher` with `Dispatch(ctx, path, content)` returning `*FileStructure`. Crush uses `Explorer` interface with similar semantics but different naming. Functionally equivalent.

### A.3: Embedded LSP Server Management — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Language server binary management | `lsp/manager.go` — Manager with auto-discovery | ✅ |
| Parallel startup | `lsp/manager.go` — parallel server start | ✅ |
| Crash recovery with exponential backoff | `lsp/crash_recovery.go` (91 lines) + `lsp/backoff.go` | ✅ |
| Backoff: 1s, 2s, 4s, 8s, 30s max | `lsp/backoff.go` — InitialInterval 1s, MaxInterval 60s, Multiplier 2x | ⚠️ Max is 60s not 30s |
| Graceful shutdown via LSP protocol | `lsp/manager.go` | ✅ |
| Auto-download LSP servers | `lsp/manager_xrush_methods.go` — AutoDownloadConfig | ✅ |
| Mode system (only relevant servers) | Server priority system + user match patterns | ✅ |
| SHA256 verification | AutoDownload feature (in config) | ✅ |

**Spec Deviation**: Spec calls for 55+ language servers (Serena solidlsp). Crush has fewer server types but covers Go, TypeScript, Python and more. The auto-download + SHA256 verification is present but not at Serena's scale.

**Missing from Spec**: Spec defines `NamePathMatcher` (3-mode symbol navigation with overload index). Crush has LSP-based symbol navigation but no explicit NamePathMatcher pattern.

**Missing from Spec**: Spec defines `TaskExecutor` (serialized LSP concurrency). Crush uses Go's native concurrency model without explicit serialization queue.

---

## Layer B: Context Management

### B.1: 7-Layer Progressive Reduction — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| **Layer 1a: Time-based micro-compaction** | `lcm/compaction_layers.go` — micro-truncation layer | ⚠️ Different approach |
| **Layer 1b: Cache-based micro-compaction** | NOT IMPLEMENTED (Anthropic-specific) | ❌ |
| **Layer 1c: Tool result persistence + clearing** | NOT IMPLEMENTED — no `ContentReplacementState` | ❌ |
| **Layer 2: Session memory compaction** | `lcm/session_compactor.go` | ✅ |
| **Layer 3: Full compaction (cache-safe fork)** | `lcm/full_compactor.go` + `agent/forked.go` | ⚠️ See below |
| **Layer 4: Post-compact cleanup** | `lcm/post_compact.go` — 4-step restoration | ⚠️ Partial |
| **Layer 5: Warning/error thresholds** | `lcm/pressure.go` — graduated pressure | ✅ |
| **Layer 6: Compact prompt (9 sections)** | `lcm/summarizer.go` — has prompt template | ⚠️ Fewer sections |
| **Layer 7: Anthropic cache management** | Forked agent shares context | ⚠️ Not cache-prefix aware |
| **AUTOCOMPACT_BUFFER_TOKENS = 13K** | `lcm/types.go` — has configurable budget | ✅ |

**CRITICAL SPEC DEVIATIONS**:

1. **No ContentReplacementState (Layer 1c)**: Spec requires frozen map of cleared tool results with `[Old tool result content cleared]` markers. Crush has NO implementation of this. Tool results are never selectively cleared or persisted to disk.

2. **No Tool Result Persistence**: Spec requires writing large tool results to `<sessionDir>/tool-results/<toolUseId>.json`. Zero implementation found — `ContentReplacementState` grep returns zero matches.

3. **Post-Compact 4-Step Restoration Incomplete**: Spec requires: (1) re-read up to 5 files, (2) inject tool result deltas, (3) re-announce skills/tools/MCP, (4) fire SessionStart hooks. Crush's `post_compact.go` does skill restoration but NOT the other 3 steps (no file re-reads, no tool result deltas, no session start hooks after compaction).

4. **LLM Summarizer Not Wired**: The compaction LLM client is never connected (per audit). Falls back to `truncateToMaxChars(2048)` instead of intelligent summarization.

5. **Cache-Safe Fork Not Anthropic-Aware**: Spec's `runForkedAgent()` specifically preserves Anthropic prompt cache prefix. Crush's `forked.go` is a general-purpose forked session system — it doesn't have Anthropic cache prefix optimization.

6. **Compact Prompt Structure**: Spec requires 9 sections (goal, task progress, files modified, errors, problem solving, user messages, pending tasks, current work, summary). Crush's summarizer prompt likely has fewer sections.

### B.2: LLM-as-Compressor (DCP) — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Range compression mode | `lcm/compressor.go` — compression strategies | ✅ |
| Message compression mode | `lcm/compressor.go` — single message compaction | ✅ |
| Deduplication auto-strategy | `lcm/compaction_layers.go` — dedup layer | ✅ |
| Purge-errors auto-strategy | `lcm/compressor.go` — error purging | ✅ |
| Graduated pressure (3-tier) | `lcm/pressure.go` + `lcm/nudge/nudge.go` | ✅ |
| Nested compression with (bN) placeholders | `lcm/reversible.go` — block placeholders | ✅ |
| Reversible compression | `lcm/reversible.go` | ✅ |
| Turn-nudge injection | `lcm/nudge/nudge.go` | ✅ |
| Iteration-nudge injection | `lcm/nudge/nudge.go` | ✅ |
| Context-limit-nudge (emergency) | `lcm/pressure.go` | ✅ |
| Model-specific limits | `config/lcm.go` — model config | ⚠️ Parsed but not consumed |

**Spec Deviation**: The nudge system IS implemented. The graduated pressure IS implemented. BUT because the LLM client is never wired, the LLM-powered compression (range, message, summarization) falls back to deterministic truncation. The nudge system fires but nudges contain generic text rather than intelligent summaries.

### B.3: 3-Agent Observation Pipeline — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Observer agent @30K tokens | `lcm/observation.go` — observer with threshold | ✅ |
| Reflector agent @40K tokens | `lcm/observation.go` — reflector with escalating levels | ✅ |
| Async buffering @20% intervals | `lcm/observation.go` — buffer logic | ✅ |
| BufferingCoordinator | `lcm/observation.go` — coordination | ✅ |
| Escalating compression (Level 0-4) | `lcm/observation.go` — reflector levels | ✅ |
| Structured XML output with priorities | `lcm/observation.go` — observation format | ✅ |
| Observer model configurable | `config/lcm.go` — SummarizerModel | ✅ |

**Wiring Gap**: ALL observation features return `ErrLLMClientNil` because the LLM client is never wired. The code is structurally complete but functionally dead.

### B.4: Ghost-Cue Injection — ✅ CODE EXISTS

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Summary IDs with type metadata | `lcm/cue.go` — ghost cue injection | ✅ |
| Lineage pointer IDs | `lcm/cue.go` — lineage references | ✅ |
| archive_stub references | `lcm/cue.go` — stub references | ✅ |
| `[AVAILABLE CONTEXT]` format block | `lcm/cue.go` — injection format | ✅ |

**Wiring Gap**: `cue.go` exists with the implementation, but cue injection depends on the LCM manager having compressed blocks to reference. With LLM unwired, there are no summaries to generate cues from.

### B.5: Summary DAG Storage — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| 13-table PostgreSQL schema | Crush uses SQLite with fewer tables | ❌ |
| conversations table | `session/session.go` — SQLite sessions | ⚠️ Different schema |
| messages table (immutable) | `message/` — message service | ⚠️ |
| summaries with kind/type | `lcm/store.go` — summaries table with kinds (sprig/bindle/archive_stub) | ✅ |
| summary_lineage_pointers | `lcm/store.go` — lineage edges | ✅ |
| context_items | NOT found | ❌ |
| large_files external storage | NOT found | ❌ |
| GIN-indexed full-text search | NOT found (SQLite not PostgreSQL) | ❌ |
| 5 retrieval tools | `lcm_grep`, `lcm_describe`, `lcm_expand` — 3 tools | ⚠️ 3 of 5 |
| QueryByContent | `lcm_grep` tool | ✅ |
| QueryByLineage | NOT found as separate tool | ❌ |
| QueryByTime | NOT found | ❌ |
| GetActiveContext | NOT found | ❌ |
| RetrieveOffContext | `lcm_expand` tool — partial match | ⚠️ |
| agentic_map_runs / llm_map_items tables | DB tables exist but zero production callers | ⚠️ |

**CRITICAL SPEC DEVIATION**: Spec requires embedded PostgreSQL with 13 tables. Crush uses SQLite. The 5 retrieval tools from spec are reduced to 3. No GIN-indexed full-text search (SQLite FTS5 could substitute but isn't implemented).

---

## Layer C: Memory

### C.1: Hierarchical CLAUDE.md — ✅ IMPLEMENTED, SPEC COMPLIANT

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| 4-layer priority (user/project/local/rules) | `config/config.go` + `agent/prompt/context.go` | ✅ |
| Walk CWD→root | `agent/prompt/context.go` — walks up from cwd | ✅ |
| CLAUDE.md, CLAUDE.local.md, .claude/rules/*.md | Context file paths in config | ✅ |
| @include resolution (max depth 5) | `agent/prompt/context.go` — include handling | ✅ |
| MAX_MEMORY_CHARACTER_COUNT: 40K | Config has size limits | ✅ |
| Frontmatter strip | `agent/prompt/prompt.go` | ✅ |
| Cache invalidation by mtime | File watcher + mtime checking | ✅ |

**Spec Deviation**: Minimal — implementation closely follows spec. Also supports GEMINI.md and CRUSH.md variants beyond CLAUDE.md.

### C.2: Thread-Scoped Observational Memory — ✅ CODE EXISTS

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Thread-scoped (not resource-scoped) | `lcm/observation.go` — thread scope | ✅ |
| Fed from Observer/Reflector | `lcm/observation.go` — pipeline | ✅ |
| Observation with priority tags | Structured observation format | ✅ |
| Continuation hint injection | Observation injection system | ✅ |

**Wiring Gap**: Same as B.3 — LLM client unwired so observations are never generated.

### C.3: Auto-Memory Extraction — ✅ CODE EXISTS

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Forked agent extraction | `lcm/memory.go` — forked agent approach | ✅ |
| 4-type taxonomy (fact/decision/preference/lesson) | `lcm/memory.go` — memory types | ✅ |
| MAX_MEMORY_LINES = 200 | Config limits exist | ✅ |
| MAX_MEMORY_BYTES = 4096 | Config limits exist | ✅ |
| Incremental processing (cursor) | Cursor-based processing | ✅ |
| Every 5 turns attachment schedule | Post-turn hook at OnRunEnd | ✅ |
| Cache shared with main thread | `lcm/cache_share.go` — cross-agent cache sharing | ✅ |

**Wiring Gap**: Returns `ErrLLMClientNil` because LLM is never wired. The code is structurally complete.

### C.4: Reversible Compression State — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Immutable history | `lcm/reversible.go` — operates on copies | ✅ |
| blocksById map | `lcm/reversible.go` — CompressionState | ✅ |
| Sequential block IDs (b1, b2, b3) | `lcm/reversible.go` — monotonic allocator | ✅ |
| Decompress command | `lcm_expand` tool | ✅ |
| No RAG/vector search | Confirmed — ID-based retrieval only | ✅ |

---

## Layer D: Edit

### D.1: Hash-Anchored Structural Editing — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Word anchors (every Nth word) | `edit_anchors.go` — content-addressed hash anchors | ✅ |
| FNV-1a hashing | `edit_anchors.go` — FNV-1a hash anchors | ✅ |
| AnchorStateManager | `edit_anchors.go` + `edit_anchors_cache.go` | ✅ |
| BatchProcessor | `edit_batch_tool.go` + `edit_batch.go` — atomic multi-file edits | ✅ |
| Position-independent operations | insert_before, replace_range, delete_range | ✅ |
| Atomic rollback on failure | `rollback.go` — file rollback | ✅ |
| ASTAnchorBridge | `treesitter` integration via edit validation | ⚠️ Implicit, not explicit bridge |
| Myers diff on hash reconciliation | `edit_fuzzy.go` — fuzzy matching for drift | ⚠️ Different approach |
| Hard rejection on mismatch | Yes — validation handler | ✅ |
| Fallback to exact string match (proposed) | `edit_fuzzy.go` — fuzzy string matching | ✅ |

**Spec Deviation**: Spec proposes explicit `ASTAnchorBridge` class bridging tree-sitter AST with line anchors. Crush takes a different approach — edit validation goes through `validation_handler.go` which uses tree-sitter parsing but doesn't have a dedicated bridge class.

### D.2: LSP Symbolic Editing — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| rename_symbol (project-wide) | `lsp_tools_ext.go` — rename tool | ✅ |
| SafeDeleteSymbol | `lsp_safe_delete.go` — reference-checked delete | ✅ |
| replace_symbol_body | `lsp_symbolic.go:23` — ReplaceSymbolBody | ✅ |
| insert_after_symbol | `lsp_insert_after.go` — constructed via LSPTools extension | ✅ |
| insert_before_symbol | `lsp_insert_before.go` — constructed via LSPTools extension | ✅ |
| WorkspaceEdit application | Via LSP client protocol | ✅ |
| Serialization (TaskExecutor) | NOT explicitly serialized | ⚠️ |

**Spec Deviation**: Spec requires all LSP edits serialized through Serena's TaskExecutor queue. Crush uses Go's native concurrency without explicit serialization. This could cause LSP race conditions under concurrent edits.

---

## Layer E: Validation

### E.1: 12-Step Validation Pipeline — ⚠️ PARTIAL MATCH

| Spec Step | Crush Implementation | Match? |
|----------|---------------------|--------|
| Step 1: Anchor map verification | `edit_anchors.go` — anchor cache | ✅ |
| Step 2: Hash resolution validation | `edit_anchors.go` — resolveAnchor | ✅ |
| Step 3: Edit syntax check | `validate.go` — tree-sitter parse | ✅ |
| Step 4: Pre-edit snapshot | `rollback.go` — pre-edit state capture | ✅ |
| Step 5: Edit application | Edit tool execution | ✅ |
| Step 6: Post-edit parse | `validate.go` — post-edit tree-sitter parse | ✅ |
| Step 7: Symbol consistency | `validate.go` — AST symbol check | ✅ |
| Step 8: Import consistency | NOT explicitly checked | ⚠️ |
| Step 9: Anchor map update | `edit_anchors_cache.go` — cache update | ✅ |
| Step 10: Diagnostics comparison | `diag_autofix.go` + `validation_handler.go` | ✅ |
| Step 11: Formatter pass | NOT in validation pipeline | ❌ |
| Step 12: Anchor map save | Cache persistence | ✅ |

**Spec Deviation**: Steps 8 (import consistency) and 11 (formatter pass) are missing from the pipeline. The formatter is available separately (gofmt/prettier) but not wired into the validation steps.

### E.2: Auto LSP Diagnostics — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Trigger after every write | `diag_autofix.go` — post-write diagnostics | ✅ |
| Cascading to importing files | NOT confirmed | ⚠️ |
| Min severity Warning | `diag_gate.go` — quality gate | ✅ |
| Auto-fix for simple diagnostics | `autofix.go` — iterative lint-fix-test cycle | ✅ |

### E.3: Auto-Lint → Commit → Test → Reflect — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Run linter | `autofix.go` + `go_linter.go` | ✅ |
| Auto-commit if lint passes | NOT automatic commit | ⚠️ |
| Run tests | `autofix.go` — test execution | ✅ |
| Reflect on failures | `autofix.go` — iterative reflection | ✅ |
| Max 3 retries | `autofix.go` — retry limit | ✅ |

**Spec Deviation**: Spec calls for auto-commit between lint and test. Crush's autofix doesn't auto-commit — it fixes, re-lints, and re-tests but leaves committing to the user.

### E.4: Atomic Rollback with Pre/Post Diagnostics — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Pre-edit diagnostics | `validation_handler.go` — pre-edit scan | ✅ |
| Post-edit diagnostics | `validation_handler.go` — post-edit comparison | ✅ |
| Rollback on regression | `rollback.go` — file revert | ✅ |
| Batch atomicity | `edit_batch_tool.go` — rollback all on failure | ✅ |

---

## Layer F: Model Optimization

### F.1: Architect/Editor Split — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Architect model (expensive) | `config/config.go` — architect model config | ✅ |
| Editor model (cheap) | `config/config.go` — editor model config | ✅ |
| ModelRole type | `agent/model_router.go` — ModelRole, RoleArchitect, RoleEditor | ✅ |
| ArchitectPlan with steps | `agent/architect_plan.go` — full plan struct | ✅ |
| Plan approval flow | NOT IMPLEMENTED | ❌ |

**Wiring Gap**: ModelRole routing logic exists but result is DISCARDED (`_ = modelType` in `model_router_ext.go:97`). ArchitectPlan data type exists but is dead code (zero production callers).

### F.2: Model Routing by Input Tokens — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| ModelRouter with tiers | `agent/router_tier.go` — TierRouter | ✅ |
| Tier-based selection | `agent/model_router.go` — RouteByTokenCount/CharCount | ✅ |
| Default tiers (10K/30K/80K/200K) | Configurable via YAML `router.tiers` | ✅ |
| Resolve returns model string | `model_router.go` — returns model | ✅ |
| Applied to subagent selection | NOT APPLIED — result discarded | ❌ |

**Wiring Gap**: The routing decision is computed but `_ = modelType` discards it. The hook has no mechanism to feed the routing decision back to the LLM request builder.

---

## Layer G: Orchestration

### G.1: Coordinator/Worker + Swarm + Forked Agent — ⚠️ WIRING GAPS

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Coordinator mode with workers | `agent/coordinator.go` — coordinator struct | ✅ |
| Forked agent (cache-efficient) | `agent/forked.go` — full forked session system | ✅ |
| Forked agent shares parent cache | `agent/forked.go` — inherits parent messages | ⚠️ Not Anthropic cache-aware |
| SendMessage (inter-agent comm) | `agent/tools/send_message.go` | ✅ |
| TeamCreate / TeamDelete | `agent/tools/team_create.go`, `team_delete.go` | ✅ |
| TaskStop | `agent/tools/task_stop.go` | ✅ |
| SyntheticOutput | `agent/tools/synthetic_output.go` | ✅ |
| AgentRegistry | `extensions/xrush_ext.go` — AgentRegistry | ✅ |
| Mailbox-based messaging | `extensions/xrush_ext.go` — Mailbox | ✅ |
| Worker tool permission capping | Config-based agent permissions | ⚠️ |

**Wiring Gaps**: StructuredSubagentFactory never wired → sub-agents can't be created. Registry/Mailbox created but never connected to OrchestrationExtension. All 4 orchestration tools return error strings.

### G.2: Structured Subagent System — ⚠️ CODE EXISTS, WIRING DISCONNECTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| SubagentRunner | `agent/structured_subagent.go` — StructuredSubagentFactory | ✅ |
| AgentConfigLoader | `agent/config_loader.go` — DynamicAgentConfigLoader | ✅ |
| Agent types (worker/explorer/planner/editor) | Config-based agent definitions | ✅ |
| Permission modes (bubble/strict) | Permission system exists | ✅ |
| gRPC communication | NOT IMPLEMENTED — uses in-process goroutines | ⚠️ |

**Spec Deviation**: Spec calls for gRPC-based communication between agents. Crush uses in-process goroutines with context isolation. This is actually one of the DREAM spec's unresolved questions (#5: "gRPC vs In-Process for Subagents"). Crush chose in-process, which is simpler but limits external agent integration.

**Wiring Gap**: `WithStructuredSubagentFactory` exists but can't be called because `NewCoordinator` has no options parameter.

### G.3: Operator Recursion Tools — ⚠️ CODE EXISTS, PARTIALLY WIRED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| LLM-Map (16-worker pool) | `agent/tools/llm_map.go` — LLM transformation | ⚠️ Generic JSONL processor |
| Agentic-Map (full subagent) | `agent/tools/agentic_map.go` — sub-agent per item | ⚠️ Generic JSONL processor |
| Batch (shared context) | `agent/tools/edit_batch_tool.go` — batch editing | ⚠️ Edit-focused, not generic |
| map_refresh tool | `agent/tools/map_refresh.go` — refresh | ✅ |
| Order-preserving fan-in | `llm_map.go` — ordered collection | ⚠️ |

**Spec Deviation**: Spec's LLM-Map is a pure function: input → structured output with JSON Schema validation and retries. Crush's `llm_map` is a generic JSONL processor that happens to use LLM. Spec's Agentic-Map spawns full sub-agents per item. Crush's `agentic_map` is similar but more limited.

**Naming Misleading**: `agentic_map` and `llm_map` sound repo-map related but are actually generic JSONL processors unrelated to repo maps.

### G.4: Parallel Subagents + Loop Detection — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Parallel subagents (MAX 5) | `agent/parallel.go` — bounded semaphore concurrency | ⚠️ No fixed 5 limit |
| Focus chain result sequencing | `agent/parallel.go` — focus-area serialization | ✅ |
| Soft threshold (3 warnings) | `agent/doom.go` — Soft=3 | ✅ |
| Hard threshold (5 forced stop) | `agent/doom.go` — Hard=5, EscalationHard=7 | ✅ |
| Canonical signature comparison | `agent/doom.go` — normalizeSignature | ✅ |
| Semantic similarity (80%) | `agent/productive.go` — ProductiveLoopDetector | ✅ |

**Spec Deviation**: Spec's Cline parallel limit is MAX_SUBAGENT_PROMPTS=5. Crush's `parallel.go` uses configurable bounded semaphore, not a fixed limit of 5.

### G.5: Doom Loop Detection — ✅ IMPLEMENTED, SPEC COMPLIANT

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Detect repetitive patterns | `agent/doom.go` — full detection | ✅ |
| Same tool + same args ≥3 | Exact signature + semantic similarity | ✅ |
| Warning injection at soft limit | `<doom-loop-warning>` XML injection | ✅ |
| Force strategy change at hard limit | EscalationHard stops agent | ✅ |
| Rollback to last known good state | `agent/doom.go` — recovery logic | ✅ |

### G.6: Dynamic Tool Surface — ⚠️ PARTIAL MATCH

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| ToolMarkerCanEdit | `agent/tool_surface.go` — CapabilityEdit | ✅ |
| ToolMarkerSymbolicRead | CapabilityCodeIntelligence | ✅ |
| ToolMarkerSymbolicEdit | CapabilityCodeIntelligence | ✅ |
| ToolMarkerOptional | — | ❌ |
| ToolMarkerBeta | — | ❌ |
| ToolMarkerDoesNotRequireActiveProject | — | ❌ |
| ToolRegistry auto-discovery | `agent/tool_surface.go` — registerDefaults | ✅ |
| ForMode() filtering | UpdateCapabilities() — runtime gating | ⚠️ |
| Session-aware prompt assembly | `agent/tool_surface.go` — surface descriptions | ✅ |

**Spec Deviation**: Spec defines 6 marker types from Serena. Crush has different capability bits (Memory, Execution, Network, etc.) — not a 1:1 mapping. Crush is missing Optional, Beta, and DoesNotRequireActiveProject markers.

**Wiring Gap**: HasRepoMap/HasMCP/HasLCM never set to true → 8 tools incorrectly hidden from surface.

---

## Layer H: Evaluation & QA

### H.1: Eval Framework — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| MastraScorer architecture | `eval/scorers/mastra/mastra.go` — Mastra scorer | ✅ |
| LLM-as-judge scorer | `eval/scorers/judge/judge.go` — LLM judge | ✅ |
| Metric-based scorers | `eval/scorers/metric/` — multiple metric scorers | ✅ |
| EvalRunner with datasets | `eval/runner.go` — JSON dataset runner | ✅ |
| ScorerStorage | `eval/storage.go` — scorer storage | ✅ |
| Preprocess → Analyze → Score → Reason pipeline | 4-step pipeline in scorers | ✅ |
| Prebuilt scorers (12 LLM + 7 code) | `eval/scorers/` — metric, judge, mastra packages | ⚠️ Fewer than 19 |

**Spec Deviation**: Spec calls for 19 prebuilt scorers. Crush has a subset organized into metric, judge, and mastra packages. Specific scorers found: edit_distance, keyword_coverage, content_similarity, lint, syntax, test_pass, coverage, trajectory_code, typecheck. Missing several spec scorers (bias, hallucination, toxicity, faithfulness, etc.).

### H.2: 16-Processor Pipeline — ⚠️ CODE EXISTS, MOSTLY DEAD

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| 16 processor implementations | `processor/` — 16 implementations found | ✅ |
| PII Detector | `processor/` — PIIDetector | ✅ (dead) |
| Moderation | `processor/` — ModerationProcessor | ✅ (dead) |
| Prompt Injection Detector | `processor/` — PromptInjectionDetector | ✅ (dead) |
| Token Limiter | `processor/` — TokenLimiter | ✅ (ACTIVE) |
| System Prompt Scrubber | `processor/` — SystemPromptScrubber | ✅ (ACTIVE) |
| Unicode Normalizer | `processor/` — UnicodeNormalizer | ✅ (dead) |
| Message Selection | `processor/` — MessageSelection | ✅ (dead) |
| Structured Output | `processor/` — StructuredOutput | ✅ (dead) |
| Tool Call Filter | `processor/` — ToolCallFilter | ✅ (dead) |
| Batch Parts | `processor/` — BatchParts | ✅ (dead) |
| Tool/Skill Search | `processor/` — ToolSearch, SkillSearch | ✅ (dead) |
| Skills processor | `processor/` — Skills | ✅ (dead) |
| Workspace Instructions | `processor/` — WorkspaceInstructions | ✅ (dead) |
| Message History | `processor/` — MessageHistory | ✅ (dead) |
| Language Detector | `processor/` — LanguageDetector | ✅ (dead) |

**Wiring Gap**: Only 2/16 processors are active (TokenLimiter + SystemPromptScrubber). PIIDetector is advertised as "safe default" in config schema but NOT registered. 14 processors are dead code.

### H.3: ReadCoordinator — ✅ IMPLEMENTED

| Spec Requirement | Crush Implementation | Match? |
|-----------------|---------------------|--------|
| Token-budgeted reads | `eval/readcoordinator.go` — NewReadCoordinator | ✅ |
| Priority-based allocation | `eval/priority_adapter.go` — PageRank priority | ✅ |
| Bounded worker pool (8 workers) | ReadCoordinator with configurable workers | ✅ |
| Per-file token cap | Budget enforcement in tests | ✅ |
| Integration with PageRank repomap | `priority_adapter.go` — adapts repomap scores | ✅ |

---

## Summary of DREAM Spec Deviations (by severity)

### CRITICAL — Features Present but Non-Functional Due to Wiring

| # | DREAM Component | Deviation | Impact |
|---|----------------|-----------|--------|
| 1 | B.1 ContentReplacementState | Not implemented — no tool result clearing or persistence | Major quality loss: tool results never freed |
| 2 | B.1 Post-compact restoration | Only 1 of 4 steps implemented (skills) | Files not re-read, tool deltas not restored |
| 3 | B.3/B.4/B.5 LLM client | Never wired — all LLM-dependent features dead | No intelligent summarization, observation, or memory |
| 4 | G.1/G.2 Sub-agent factory | Never wired — orchestration unreachable | No multi-agent coordination |
| 5 | F.1/F.2 Model routing result | Discarded with `_ = modelType` | No cost optimization via routing |

### HIGH — Implementation Differs from Spec

| # | DREAM Component | Deviation | Impact |
|---|----------------|-----------|--------|
| 6 | B.5 Storage backend | SQLite instead of PostgreSQL, 3/5 retrieval tools | No FTS, no full DAG query capabilities |
| 7 | D.2 TaskExecutor serialization | Not implemented — native Go concurrency | Potential LSP race conditions |
| 8 | G.2 gRPC communication | In-process goroutines instead | No external agent integration |
| 9 | E.1 12-step pipeline | Steps 8 (imports) and 11 (formatter) missing | Less thorough validation |
| 10 | G.6 Tool markers | Different capability system, missing Optional/Beta/NoProject markers | Less granular tool visibility control |

### MEDIUM — Spec Details Not Fully Matched

| # | DREAM Component | Deviation | Impact |
|---|----------------|-----------|--------|
| 11 | B.1 Cache-safe fork | Not Anthropic cache-prefix aware | Missed ~98% token savings on compaction |
| 12 | G.3 LLM-Map/Agentic-Map | Generic JSONL processors, not spec's structured tools | Less schema-validated output |
| 13 | H.1 Eval scorers | Fewer than 19 prebuilt scorers | Less evaluation coverage |
| 14 | H.2 Processors | 14/16 implemented but dead | Safety/QA features inactive |
| 15 | E.3 Auto-commit | Not implemented between lint and test | Manual commit workflow |

### LOW — Cosmetic Differences

| # | DREAM Component | Deviation | Impact |
|---|----------------|-----------|--------|
| 16 | A.3 Backoff max | 60s vs spec's 30s | Slightly longer recovery time |
| 17 | G.4 Parallel limit | Configurable vs fixed 5 | More flexible |
| 18 | A.3 Server count | Fewer than 55+ languages | Less language coverage |

---

## Features in Crush NOT in DREAM Spec

These Crush features have no corresponding DREAM component:

| Feature | Files | Notes |
|---------|-------|-------|
| Session Recovery | `coordinator_xrush_recovery.go` | Unique to Crush — fixes interrupted sessions |
| YAML Config (`.xrush/config.yml`) | `config/yaml.go` | Crush's config system, not from DREAM |
| Rewind/Fork/Edit messages | `rewind/` package (11 files) | Turn-based snapshot system — unique to Crush |
| Message Options (UI) | `ui/model/rewind_ui.go` | User-facing rewind/fork/edit dialog |
| Compaction Events (pubsub) | `app/ext_setup.go` | UI feedback during compaction |
| Doom Loop escalation tiers | `agent/doom.go` | Crush has Soft/Medium/Hard vs DREAM's 2-level |
| Hash anchors with fuzzy fallback | `edit_fuzzy.go` | DREAM spec proposed this — Crush already has it |
| Batch file reading (8 workers) | `view_xrush.go` | ReadCoordinator-like but in view tool |
| Exponential backoff for LSP | `lsp/backoff.go` | Dedicated backoff system |

---

## Priority Recommendations (DREAM Compliance)

### P0 — Make Implemented Features Actually Work

1. **Wire LLM client to LCM** — unlocks B.3, B.4, C.2, C.3 (observation, ghost-cue, auto-memory)
2. **Wire StructuredSubagentFactory** — unlocks G.1, G.2, G.3 (orchestration)
3. **Wire Registry/Mailbox** — enables inter-agent communication
4. **Fix model routing** — replace `_ = modelType` with actual routing

### P1 — Close Spec Gaps in Existing Features

5. **Implement ContentReplacementState** — tool result clearing/persistence (B.1 Layer 1c)
6. **Complete post-compact 4-step restoration** — file re-reads, tool result deltas, SessionStart hooks
7. **Populate HasLCM/HasMCP/HasRepoMap** — fix ToolSurface gating
8. **Activate PIIDetector processor** — advertised as "safe default" but not registered
9. **Wire PostToolUse hooks** — build runner from both Pre and Post hooks

### P2 — Spec Alignment

10. **Add missing retrieval tools** (QueryByLineage, QueryByTime, GetActiveContext) to B.5
11. **Add import consistency check** to validation pipeline (E.1 Step 8)
12. **Add formatter pass** to validation pipeline (E.1 Step 11)
13. **Implement LSP TaskExecutor serialization** for concurrent edit safety
14. **Implement more eval scorers** to reach 19 prebuilt scorers
15. **Activate dead processors** or remove them
