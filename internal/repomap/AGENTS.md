# Repository Map Subsystem

Indexes, ranks, and renders repository maps for LLM context. Provides
scope-aware code outlines that fit within a token budget.

## Structure

- `repomap.go` - Service struct, lifecycle, Generate(), PreIndex
- `tags.go` - Tree-sitter tag extraction with DB caching
- `graph.go` - FileGraph from def/ref/import edges
- `pagerank.go` - PageRank over FileGraph with personalization
- `stage.go` - AssembleStageEntries (4-stage priority)
- `budget.go` - FitToBudget: binary-search token fitting
- `render.go` - RenderRepoMap: scope-aware tree-context rendering
- `treecontext.go` - AST-driven scope-aware line selection
- `cache.go` - SessionCache + SessionRenderCacheSet
- `diffwatch.go` - Polls git diff, invalidates caches
- `blame.go` - Git-log recency metadata per file
- `proximity.go` - Test-file co-location heuristics
- `mentions.go` - Extract mentions from LLM messages
- `special.go` - Root config/doc files for stage-0 prelude
- `tiktoken.go` - cl100k_base BPE tokenizer (embedded ~1.6 MB)
- `conformance.go` - Aider parity sign-off snapshots
- `parity_fixtures.go`, `parity_provenance.go` - Parity test infra
- `tokens.go`, `normalization.go`, `metrics.go` - Token utilities

## Pipeline

```
PreIndex -> extractTags -> buildGraph -> BuildPersonalization
  -> Rank (PageRank) -> BuildSpecialPrelude -> AssembleStageEntries
  -> FitToBudget -> RenderRepoMap -> post-render trim -> cache store
```

Stages:
- 0: Special prelude (root config files like AGENTS.md, go.mod)
- 1: Ranked definitions (PageRank-scored, scope-rendered)
- 2: Remaining graph nodes (bare filenames)
- 3: Remaining repo files (bare filenames)

## Ranking

PageRank: damping=0.85, tol=1e-6, 100 iterations max.
Personalization blends chat files, mentioned filenames/idents, blame
recency (7-day half-life, 0.15 weight), proximity (0.10 weight).

## Caching

Two-tier: SessionCache (one map+token pair per session) and
SessionRenderCacheSet (per-session, keyed by opts hash). DiffWatcher
invalidates both on git diff every 30s. Singleflight groups concurrent runs.

## Agent Tools

- `agentic_map`: Full Generate() pipeline, agent-initiated
- `llm_map`: Read-only cached map for LLM context injection
- `map_refresh`: Force invalidation and regeneration

Registered in coordinator.buildTools(). Coordinator mediates with LCM;
this package never imports LCM directly.

## Parity Mode

Mirrors Aider's repomap output. Uses git-tracked files (not walker),
deterministic tokenization, conformance snapshots, fixture hashing.
One-way disable latch on resource exhaustion.

## Dependencies

- `internal/treesitter`: Tag extraction, Parser, AST scope walking
- `internal/db`: SQLite tag cache (sqlc)
- `internal/config`: RepoMapOptions (budget, refresh mode)

## Anti-Patterns

- Never call `runtime/debug.SetLimit(0)` here. Deadlocks with tree-sitter CGO.
- Never import `internal/lcm`. Coordinator handles mediation.
- Never VACUUM databases with FTS5 tables.
- Parity mode must not fall back to estimation when a tokenizer is configured.
