# XRUSH QA Gap Matrix

Traceability matrix mapping every XRUSH feature area to TUI-first test coverage.

## Schema

| Column | Description |
|--------|-------------|
| **TUI Scenario Path** | Path to the `test-*.sh` file (primary evidence) |
| **TUI Capture Artifact** | Expected capture file under `reports/` |
| **Secondary Evidence** | DB queries, log checks (supporting only) |
| **Status** | OPEN (test missing), PENDING (test exists, not yet run), RESOLVED (test exists AND passed with TUI evidence) |

## Rules

- Primary evidence is always the TUI scenario path + captured TUI output.
- `go test` commands and Go unit tests may appear as supporting unit evidence only.
- No row can be RESOLVED based solely on `go test` or DB/log checks.
- Stale paths (files that do not exist) must not be referenced.

## Matrix

| ID | Feature | TUI Scenario Path | TUI Capture Artifact | Secondary Evidence | Status |
|----|---------|-------------------|----------------------|--------------------|--------|
| F01 | Sessions | `wave1-session/test-session.sh` | `reports/wave1-session.txt` | test-session-resume.sh, test-file-tracking.sh | RESOLVED |
| F02 | Tree-sitter | `wave2-repomap/test-treesitter.sh` | `reports/wave2-repomap.txt` | — | RESOLVED |
| F03 | File-Type Explorer | `wave2-repomap/test-explorer.sh` | `reports/wave2-repomap.txt` | test-explorer-semantic.sh | PENDING |
| F04 | Repo-map | `wave2-repomap/test-repomap.sh` | `reports/wave2-repomap.txt` | test-rankings-imports.sh | RESOLVED |
| F05 | LCM Core | `wave3-lcm/test-lcm-compaction.sh` | `reports/wave3-lcm.txt` | test-lcm-basics.sh, test-lcm-compaction-retrieval.sh, test-auto-memory.sh, test-large-file-offload.sh, test-operational-memory.sh | PENDING |
| F06 | LCM Retrieval Tools | `wave3-lcm/test-lcm-retrieval-tools.sh` | `reports/wave3-lcm.txt` | test-map-tools.sh, test-reflector.sh | PENDING |
| F07 | Processor Pipeline | `wave5-tui/test-processors-live.sh` | `reports/wave5-tui.txt` | — | PENDING |
| F08 | Routing/Fallback | — | — | — | OPEN |
| F09 | Hooks | `wave5-tui/test-hooks.sh` | `reports/wave5-tui.txt` | test-post-tool-use.sh, test-compaction-hooks.sh | PENDING |
| F10 | Eval/Scorers | `wave5-tui/test-eval.sh` | `reports/wave5-tui.txt` | test-eval-pipeline.sh | PENDING |
| F11 | Config Loading | `wave5-tui/test-config-shell.sh` | `reports/wave5-tui.txt` | — | PENDING |
| F12 | TUI | `wave5-tui/test-tui-streaming.sh` | `reports/wave5-tui.txt` | — | RESOLVED |
| F13 | Architect Plan | `wave4-orchestration/test-architect.sh` | `reports/wave4-orchestration.txt` | test-architect-operator.sh | PENDING |
| F14 | AutoFix | `wave4-orchestration/test-autofix.sh` | `reports/wave4-orchestration.txt` | test-orchestration-autofix.sh | PENDING |
| F15 | Doom Loop | `wave4-orchestration/test-doom-loop.sh` | `reports/wave4-orchestration.txt` | test-doom-loop-intervention.sh | RESOLVED |
| F16 | Orchestration/Forked Agents | `wave4-orchestration/test-orchestration.sh` | `reports/wave4-orchestration.txt` | test-forked-agents.sh, test-orchestration-contract.sh | RESOLVED |
| F17 | Rewind | `wave4-orchestration/test-rewind.sh` | `reports/wave4-orchestration.txt` | test-rewind-restore.sh | PENDING |
| F18 | Extension Host | `wave5-tui/test-extensions-live.sh` | `reports/wave5-tui.txt` | — | PENDING |
| F19 | LSP Enhancements | — | — | — | OPEN |
| F20 | Tools Surface | `wave5-tui/test-edit-tools-live.sh` | `reports/wave5-tui.txt` | — | PENDING |
| F21 | Message Timestamps | `wave1-session/test-message-parts.sh` | `reports/wave1-session.txt` | — | RESOLVED |
| F22 | DB Migrations | `wave5-tui/test-db-migrations-live.sh` | `reports/wave5-tui.txt` | — | RESOLVED |
| F23 | Shell Enhancements | `wave5-tui/test-shell-enhancements.sh` | `reports/wave5-tui.txt` | — | PENDING |

## Summary

| Metric | Value |
|--------|-------|
| Total feature areas | 23 |
| RESOLVED (TUI evidence) | 8 (F01, F02, F04, F12, F15, F16, F21, F22) |
| PENDING (test exists, not yet run) | 12 |
| OPEN (no test file) | 3 (F08, F19) |


