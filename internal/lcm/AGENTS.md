# LCM -- Lossless Context Management

Manages conversation context for LLM sessions: summaries, large-output
storage, multi-layer compaction, and agent tools. Must be invisible to
users -- no LCM internals in tool outputs.

## Structure

Core: manager.go (Manager, 37 methods), compactor.go, store.go, config.go,
types.go. Layers: compaction_layers.go, full_compactor.go, session_compactor.go,
cache_optimizer.go, pressure.go, post_compact.go. LLM: summarizer.go,
compressor.go, reversible.go. Intelligence: observation.go, reflector.go,
memory.go, cue.go. Tools: retrieval_tools.go. Analysis: explorer/.

## Manager Interface (key groups)

- **Compaction**: Compact, ScheduleCompaction, CompactUntilUnderLimit,
  RunLayeredCompaction, CompactIfOverHardLimit
- **Budget**: GetBudget, GetContextTokenCount, SetOverheadTokens,
  SetRepoMapTokens, IsOverSoftThreshold, IsOverHardLimit
- **Lifecycle**: InitSession, OnSessionStart, OnSessionEnd, SetLLMClient
- **Agent integration**: ExtraAgentTools, BuildCompactPrompt,
  GetFormattedContext, GetContextFiles
- **Post-hooks**: PostCompactionHook, PostTurnHook (auto-memory)
- **Token tracking**: SetActualPromptTokens, AddPendingItemTokens

## Compaction Pipeline (8 layers)

Phase 1: layers in priority order. Phase 2: LLM summarization fallback
if still over soft threshold.

1. MicroCompactor -- truncate large tool outputs
2. DedupCompactionLayer -- remove duplicate messages
3. StaleEvictionLayer -- evict stale tool outputs
4. PostCompactCleaner -- restore preserved context
5. AdjacentCondensationLayer -- merge adjacent summaries
5b. PressureCompactionSelector -- Low/Medium/High pressure tiers
6-7. CacheOptimizer -- memory pruning, emergency truncation

## Budget Formula

```
overhead = systemPrompt + tools + repoMap + 200 (per-step injection)
outputReserve = min(20000, contextWindow*0.25, modelOutputLimit)
hardLimit = contextWindow - overhead - outputReserve
softThreshold = clamp(contextWindow*cutoff - overhead, 0, hardLimit)
```

Defaults: cutoff=0.6, contextWindow=128000.

## Agent Tools (10)

lcm_grep, lcm_describe, lcm_expand, llm_map, agentic_map, lcm_bindle,
lcm_ancestry, lcm_dolt, lcm_archive, lcm_sprig.

## Explorer Subsystem (lcm/explorer/)

~20 file-type explorers: code (tree-sitter), markdown, binary, images,
PDF, LaTeX, shell, SQLite, logs, executables, data. Registry dispatches
on extension + content heuristics. Interface: CanHandle / Explore.

## Cross-Subsystem

Coordinator mediates between LCM and repomap -- never import each other.
Coordinator calls SetRepoMapTokens (budget), GetSummaryMentionedPaths
(weak ranking hints for repo-map).

## Anti-Patterns

- Never expose LCM internals (summary IDs, token counts) in user output.
- internal/lcm must not import internal/repomap.
- Never call runtime/debug/SetLimit(0) -- deadlocks with tree-sitter CGO.
- Always call SetActualPromptTokens after each LLM call.
- Never call CompactUntilUnderLimit while holding another session lock.
