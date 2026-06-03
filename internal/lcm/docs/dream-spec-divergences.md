# DREAM Specification: Architectural Divergences

This document catalogs intentional divergences from the DREAM architecture
specification (`DREAM.md`, `DREAM_IMPLEMENTATION_SPEC.md`). Each divergence is
a deliberate design choice with documented rationale.

The DREAM specification synthesizes the most complementary features from 9
coding agent projects (Cline, Dirac, OpenCode, Volt, DCP, Mastra, Aider,
Serena, Claude Code). Crush implements most of these features as specified, but
diverges on three architectural points where the spec's choices don't fit a
single-user terminal tool.

---

## Divergence 1: B.5 Summary DAG Storage — SQLite instead of PostgreSQL

**Spec reference**: `DREAM.md` §B.5, `DREAM_IMPLEMENTATION_SPEC.md` §4.B.5
(lines 604-683)

**What the spec says**: "13-table embedded PostgreSQL v17.7" for summary DAG
storage. The spec calls for downloading and running an embedded PostgreSQL
instance (`LCM_POSTGRES_VERSION = "17.7"`) with pgx driver, GIN-indexed
tsvector full-text search, and a 13-table schema covering conversations,
messages, summaries, lineage pointers, and context assembly.

**What Crush does instead**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
and `github.com/ncruces/go-sqlite3`. The schema is managed through sqlc with
migrations in `internal/db/migrations/`.

**Rationale**:

1. **Zero external dependencies**. Embedded PostgreSQL means downloading,
   extracting, and running a ~500MB binary. For a terminal tool that users
   launch with `go install`, this is a non-starter. SQLite is a single file.
2. **No CGO requirement**. `modernc.org/sqlite` is a pure Go translation of
   SQLite. Crush builds with `CGO_ENABLED=1` for tree-sitter, but the database
   layer doesn't force that dependency. `ncruces/go-sqlite3` uses WASM and
   doesn't need CGO either.
3. **Sufficient for single-user workloads**. PostgreSQL's concurrent write
   performance and query planner sophistication matter for multi-user services.
   Crush is a single-user terminal application. SQLite handles this workload
   without breaking a sweat.
4. **sqlc migrations already work**. Crush's migration infrastructure
   (`internal/db/sql/`, `internal/db/migrations/`) targets SQLite. Switching
   to PostgreSQL would mean rewriting the entire persistence layer for marginal
   benefit.
5. **Startup time**. The spec itself notes "~2s first start, ~500ms cached"
   for embedded PostgreSQL. SQLite opens instantly.

**Tradeoff**: SQLite lacks GIN-indexed tsvector full-text search. Crush
compensates with application-level text search where needed. For a terminal
tool's data volumes, this is acceptable.

---

## Divergence 2: G.2 Structured Subagents — In-process instead of gRPC

**Spec reference**: `DREAM.md` §G.2, `DREAM_IMPLEMENTATION_SPEC.md` §9.G.2
(lines 1396-1441)

**What the spec says**: "gRPC-based communication between main agent and
subagents." The spec defines a `SubagentService` protobuf with `Spawn`,
`SendMessage`, `Stop`, and `HealthCheck` RPCs. SubagentRunner (1,007 lines),
AgentConfigLoader (355 lines), and SubagentBuilder (105 lines) manage agent
lifecycle over gRPC.

**What Crush does instead**: In-process Go function calls via
`internal/agent/structured_subagent.go`. Subagents run as goroutines within the
same process, communicating through Go channels and shared interfaces.

**Rationale**:

1. **Subagents share the same process**. Crush's subagents (coder, task,
   planner, editor) all operate on the same codebase within a single terminal
   session. There's no distribution boundary to justify RPC.
2. **Zero serialization overhead**. gRPC requires protobuf serialization for
   every message. In-process calls pass Go structs directly. For a tool that
   makes hundreds of subagent calls per session, this adds up.
3. **Simpler error propagation**. Go errors bubble up naturally through
   function calls. gRPC requires translating errors across process boundaries,
   handling transport failures, and managing reconnection logic.
4. **No port management**. gRPC needs a listening port, which means port
   allocation, conflict detection, and cleanup. A terminal tool shouldn't
   need to open network ports for internal communication.
5. **gRPC is for distributed systems**. The spec's gRPC choice comes from
   Dirac, a VS Code extension where the extension host and language servers
   may run in separate processes. Crush is a single binary. The overhead is
   unjustified.

**Tradeoff**: In-process subagents share memory space. A panic in one subagent
can crash the entire process. gRPC provides process isolation. In practice,
Crush's subagents are well-tested and the risk is minimal.

---

## Divergence 3: B.1 Cache-safe Compaction — In-process pipeline instead of
forked agents

**Spec reference**: `DREAM.md` §B.1, `DREAM_IMPLEMENTATION_SPEC.md` §4.B.1
(lines 279-391)

**What the spec says**: "Cache-safe forked compaction" via `runForkedAgent()`.
The spec describes forking the conversation, running compaction in the forked
agent, then replacing the original. This preserves Anthropic's prompt cache
prefix for ~98% token savings. The approach is explicitly noted as
"Anthropic-specific" (O6).

**What Crush does instead**: A 9-layer in-process compaction pipeline in
`internal/lcm/compaction_layers.go`. The pipeline processes messages through
sequential layers (tool result clearing, session memory, summarization, cache
optimization, cleanup, etc.) without spawning separate agents.

**Rationale**:

1. **Works across ALL LLM providers**. The forked-agent approach depends on
   Anthropic's prompt caching behavior. Crush supports Anthropic, OpenAI,
   Gemini, Bedrock, Copilot, Hyper, MiniMax, Vercel, and more. The compaction
   pipeline needs to work regardless of provider.
2. **Lower latency**. Forking an agent means spawning a new LLM conversation,
   waiting for the model to process the summary, then reconciling results. The
   in-process pipeline executes layers sequentially with no agent spawn
   overhead.
3. **Deterministic layer ordering**. Each layer in the 9-layer pipeline has a
   clear responsibility and runs in a fixed order. This makes debugging and
   testing straightforward compared to the non-deterministic behavior of a
   forked agent producing a summary.
4. **CacheOptimizer handles cache awareness**. One of the 9 layers
   (`CacheOptimizer`) provides cache-aware compression within the pipeline.
   It's not identical to Anthropic's prompt cache prefix preservation, but it
   optimizes for the general case across providers.

**Tradeoff**: The in-process approach doesn't achieve the ~98% token savings
from Anthropic's cache prefix sharing. For Anthropic users specifically, this
means slightly higher API costs during compaction. The benefit is that
compaction works correctly for every supported provider.

---

## Features Fully Implemented Per Spec

The following features match or exceed the DREAM specification:

| Feature | Spec Section | Crush Implementation |
|---------|-------------|---------------------|
| 12-Step Validation Pipeline | E.1 | Fully implemented. 12 validation stages in `internal/agent/tools/validate.go` (tree-sitter build tag) and `validate_stub.go` (fallback). |
| ReadCoordinator with token-budgeted reads | H.3 | Fully implemented. 8 parallel workers, 200K token budget in `internal/agent/tools/view_xrush.go`. |
| Architect/Editor Model Split | F.1 | Fully implemented via `internal/agent/router_tier.go` TierRouter. Planning models at 3x cost multiplier, editing models at 0.5x. |
| Forward Import Resolution | T13 | Fully implemented via tree-sitter in `internal/treesitter/imports.go`. Handles 28 languages with per-language import extraction. |
| PageRank Repomap | A.1 | Fully implemented in `internal/repomap/`. PageRank over def/ref/import graph with personalization vectors. |
| Hash-Anchored Editing | D.1 | Fully implemented in `internal/agent/tools/edit_anchors.go`. FNV-1a hashing, position-independent anchors, batch support. |
| Doom Loop Detection | G.5 | Fully implemented in `internal/agent/doom.go`. Detects repetitive action cycles and forces strategy changes. |
| Coordinator/Worker + Swarm | G.1 | Fully implemented in `internal/agent/coordinator.go` and `internal/agent/swarm.go`. |
| Operator Recursion | G.3 | Fully implemented in `internal/agent/operator.go`. Recursive task decomposition via DAG. |
| Parallel Subagents | G.4 | Fully implemented in `internal/agent/parallel.go`. Bounded fan-out concurrent execution. |
| Auto LSP Diagnostics | E.2 | Fully implemented. Auto-triggered post-write via `internal/lsp/` client manager. |
| Auto-Lint/Commit/Test/Reflect | E.3 | Fully implemented in `internal/agent/autofix.go`. Iterative lint-fix-test-reflect cycle. |
| Hierarchical Context Files | C.1 | Fully implemented. Walks CWD to root reading AGENTS.md, CRUSH.md, CLAUDE.md, GEMINI.md (and `.local` variants). |
| Dynamic Tool Surface | G.6 | Fully implemented in `internal/agent/tool_surface.go`. Tool registry with capability descriptions for prompts. |
| 16-Processor Pipeline | H.2 | Fully implemented in `internal/processor/`. 16 processors across 4 phases, 3 active by default. |
| Eval Framework | H.1 | Fully implemented in `internal/eval/`. Parallel scorer registration, dataset-driven testing, LLM-as-judge. |

---

## Features Partially Implemented

| Feature | Spec Section | Status | Notes |
|---------|-------------|--------|-------|
| Embedded LSP Server Management | A.3 | Partial | LSP client manager and auto-discovery implemented in `internal/lsp/`. 55+ servers via Serena's solidlsp not fully ported; expanding via catalog.json. |
| Compaction Pipeline | B.1 | Functional | 9-layer pipeline fully operational in `internal/lcm/compaction_layers.go`. Uses in-process approach instead of forked agents (see Divergence 3). |
| LLM-as-Compressor | B.2 | Partial | Graduated pressure and compression strategies partially implemented through the compaction pipeline. Full DCP-style deduplication and purge-errors strategies not yet separate. |
| Ghost-Cue Injection | B.4 | Partial | Summary references injected into context via LCM. Full lineage pointer traversal and archive stub retrieval still maturing. |
| Auto-Memory Extraction | C.3 | Partial | Memory persistence across sessions implemented. 613-line forked agent pipeline from Claude Code simplified to in-process extraction. |
