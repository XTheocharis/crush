# TODO

## Scope And Context

This file tracks open branch issues and review follow-ups. There were no
explicit outstanding work items in `AGENTS.md` or `CLAUDE.md`; this file now
holds the current branch-specific issues instead.

On 2026-03-22, the branch moved explicitly to CGO-required source builds.
Non-CGO compatibility is no longer a tracked work item.

On 2026-03-22, LCM compaction was wired to a real LLM summarizer. It now uses
`options.lcm.summarizer_model` when configured with enough context window, and
otherwise falls back to the configured large model.

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

No open branch issues are currently tracked here.
