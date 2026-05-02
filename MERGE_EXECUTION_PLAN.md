# Merge Execution Plan

## Goal

Sync `fork/xrush` with `origin/main` / `upstream/main` using a normal merge
commit, not a rebase. Preserve fork-specific LCM, repo-map, tree-sitter,
filetracker, and large-output storage behavior while adopting upstream
implementations that supersede fork-adjacent work.

## Known State

- PR: `XTheocharis/crush#1`.
- Head: `fork/xrush` at `860c445ca9dc9062da07672375166707322d4753`.
- Base: `origin/main` and `upstream/main` at
  `56b192e35e8cdaa63f44318ecd13da418e790582`.
- Merge base: `a495a042aa95f03f290447cdc33e77130efb3319`.
- Branch delta: `297` behind, `8` ahead.
- `TODO.md` contains stale merge-base notes and should be updated or removed
  after merge validation.

## Conflict Inventory

Actual content conflicts:

- `AGENTS.md`
- `go.mod`
- `go.sum`
- `internal/agent/agent.go`
- `internal/agent/coordinator.go`
- `internal/app/app.go`
- `internal/config/config.go`
- `internal/config/load_test.go`
- `internal/db/connect.go`
- `internal/ui/model/ui.go`
- `schema.json`

Same-path files that auto-merge but still need semantic review:

- `README.md`
- `Taskfile.yaml`
- `internal/agent/agent_tool.go`
- `internal/agent/agentic_fetch_tool.go`
- `internal/agent/common_test.go`
- `internal/agent/prompt/prompt.go`
- `internal/agent/tools/edit.go`
- `internal/agent/tools/multiedit.go`
- `internal/config/load.go`
- `internal/db/db.go`
- `internal/ui/dialog/commands.go`
- `internal/ui/model/pills.go`
- `internal/ui/model/session.go`

## Merge Decisions

- Adopt upstream `internal/hooks/`, `internal/agent/hooked_tool.go`, hook
  config validation, hook metadata, and permission pre-approval.
- Keep fork `PrepareStepHook` behavior for LCM/repo-map injection. It is model
  prepare-step plumbing, not a replacement for upstream user-configured
  `PreToolUse` hooks.
- Adopt upstream skills discovery/filtering, `disabled_skills`, `crush_info`,
  `crush_logs`, short tool descriptions, runtime overrides, config reload
  tracking, app event broker, workspace/backend/client architecture, and
  `StopTurn` permission denial behavior.
- Preserve fork `Options.LCM`, `Options.RepoMap`, LCM compaction, LCM extra
  tools, repo-map refresh/reset, repo-map prepare-step injection, tree-sitter
  dependencies, and `recursive_triggers = ON`.
- Treat `Tools.RepoMap` as stale unless tests prove it is still intentionally
  supported. Prefer `Options.RepoMap` as the single runtime config surface.

## Implementation Order

1. Start clean: confirm `git status --short --branch` is clean.
2. Run `git merge origin/main`.
3. Resolve config first:
   `internal/config/config.go`, `internal/config/load.go`,
   `internal/config/merge.go`, and `internal/config/load_test.go`.
   Ensure `Hooks`, `DisabledSkills`, runtime overrides, loaded paths, reload
   behavior, LCM, and repo-map all survive merging.
4. Resolve DB next:
   keep upstream goose init/test logger/embed cleanup; retain fork
   `recursive_triggers = ON`; regenerate sqlc later instead of hand-finalizing
   generated files.
5. Resolve app/coordinator/agent wiring:
   layer fork LCM/repo-map services and coordinator options onto upstream event
   broker, workspace, skills, hooks, and tool wrapping.
6. Resolve UI:
   route repo-map refresh/reset and LCM UI events through upstream
   workspace/backend/client APIs, not direct `m.com.App` access.
7. Resolve docs/tasks:
   keep upstream README hook/skills/provider updates and fork CGO/tree-sitter
   notes. Keep upstream `swag` task and fork `Taskfile.xrush.yaml`
   include/CGO/sqlc source metadata.
8. Resolve derived artifacts last:
   run `go mod tidy`, `task sqlc`, regenerate `schema.json`, then format.

## Validation

- Confirm `git merge-tree --write-tree --name-only HEAD origin/main` no longer
  reports conflicts after resolution.
- Run `go mod tidy`.
- Run `task sqlc`.
- Run `go run main.go schema > schema.json`.
- Run `task fmt`.
- Run focused tests:
  `go test ./internal/config ./internal/hooks ./internal/skills ./internal/agent ./internal/app ./internal/db ./internal/filetracker ./internal/lcm ./internal/repomap ./internal/treesitter`.
- Run UI/workspace tests:
  `go test ./internal/ui/... ./internal/workspace ./internal/backend ./internal/server ./internal/client`.
- Run the full suite:
  `CGO_ENABLED=1 go test ./...`.
- Smoke-test manually or with targeted tests:
  hook-denied tool call, hook allow pre-approval, permission denial StopTurn,
  skill discovery/disable, repo-map refresh/reset, LCM compaction event UI,
  session recovery, and client/server workspace mode.

## Assumptions

- The merge is performed as a normal merge commit because `fork/xrush` is
  published.
- Upstream implementations should win where they reduce fork maintenance
  burden, unless they remove explicit fork LCM/repo-map behavior.
- Generated artifacts are not trusted until regenerated after code conflicts
  are resolved.
