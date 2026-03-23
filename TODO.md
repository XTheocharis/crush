# TODO

## Scope And Context

This file tracks open branch issues and review follow-ups. There were no
explicit outstanding work items in `AGENTS.md` or `CLAUDE.md`; this file now
holds the current branch-specific issues instead.

On 2026-03-22, the branch moved explicitly to CGO-required source builds.
Non-CGO compatibility is no longer a tracked work item.

As of 2026-03-22:

- Current branch: `fork/xrush`
- Upstream comparison target: `https://github.com/charmbracelet/crush`
- Branch-only commit reviewed: `a2b5f64d`
  `feat: squashed fork changes (RepoMap, LCM, tree-sitter, explorer, integration tests)`
- Merge-base with `upstream/main`:
  `9ec46b8d439646a505018576e7197b2daacfb7b3`

Validation run during review:

- `go test ./...` passed
- `go test -race ./internal/agent ./internal/app ./internal/lcm ./internal/repomap ./internal/treesitter` passed
- `CGO_ENABLED=0 go build .` failed

## Open Issues

### 1. Wire LCM compaction to a real LLM summarizer

Status: Open.

Context:

- `internal/app/app_lcm.go` initializes LCM with `lcm.NewManager(q, conn)`.
- `internal/lcm/manager.go` shows that `NewManager` uses
  `NewSummarizer(nil)`.
- In `internal/lcm/summarizer.go`, a nil LLM causes summarization and
  condensation to fall back to deterministic truncation.

Observed behavior:

- LCM compaction works structurally, but summary quality falls back to
  truncating raw conversation text instead of model-backed summarization.

Impact:

- Long-session context quality degrades sharply.
- The branch adds LCM machinery but does not wire its highest-value path in
  production app initialization.

Relevant files:

- `internal/app/app_lcm.go`
- `internal/lcm/manager.go`
- `internal/lcm/summarizer.go`
- `internal/lcm/compactor.go`

Possible directions:

- Build an `lcm.LLMClient` from the configured small or large model and switch
  app initialization to `NewManagerWithLLM`, or
- explicitly scope LCM to deterministic-only compaction and document that
  behavior.
