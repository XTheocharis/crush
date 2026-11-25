# UMCA implementation orchestrator

**Execution role:** lead coding agent, scheduler, integration maintainer and release decision owner. This is an instruction set for an autonomous coding-agent runner with Git, shell, file editing and child-agent execution available. It is not an implementation of an agent runner. Execute the work, not merely another planning exercise.

## 0. Mandatory operating documents and preflight

Read [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md), [UPSTREAM_PATCH_POLICY.md](UPSTREAM_PATCH_POLICY.md), [GATE_CONTRACTS.md](GATE_CONTRACTS.md), [REFERENCE_REPOSITORIES.md](REFERENCE_REPOSITORIES.md), the complete [GO_AGENT_CONTEXT.md](GO_AGENT_CONTEXT.md) and [GO_CONTEXT_READ_MAP.md](GO_CONTEXT_READ_MAP.md) before U00. The Go projection is supplied verbatim (including embedded packages/examples); use all of its relevant guidance within actual Crush repository constraints. ORCH reads all 6,630 logical lines, chunked and checkpointed if necessary, and verifies SHA-256 `ab11c958f30ca11198b0fc5d030668eb76e48111008bc735475b5e26121a4424`. Each child receives the original full document, exact role-specific full ranges/rule IDs from `GO_CONTEXT_INDEX.json`, agent plan, spec, contracts, gate contracts, reference acquisition manifest and repo instructions. Require actual reading evidence, not a summary of a summary. On resumption or changed scope, reload applicable source ranges and selected API contracts. [AUD](15-go-alignment-auditor.md) reads the full projection and independently reopens the applicable full sections at each handoff. Record any genuine reading/contract limitation; never count a truncated read as complete.

This package adds mandatory Go-context/footprint auditing as a cross-cutting execution and merge constraint; it does not reduce product scope. All 37 original work/gate nodes remain; dependencies, merge ordering and stage-specific acceptance have been corrected by the [review](AUDIT_REPORT.md). Dynamic audit nodes gate each unit, including shared slices and fixes. The original [UMCA spec](reference/umca.md) is bundled unchanged. Resolve authority by scope: product/security/wire requirements, applicable exact Go/API/driver/runtime/target contracts, compatible repository instructions/selected dependencies, then corpus scoped defaults. Where true conflict remains, record and block only the affected decision. Never upgrade/downgrade or redesign unrelated code to satisfy a generic rule.

## 1. Target and authority

Deliver the entire [UMCA architecture, implementation contract and acceptance specification](https://github.com/XTheocharis/crush/blob/308d108d50b0f55bea8ad246180dc958a7dde692/docs/umca.md), including §§2–7 and §11 A–I. The native local vertical slice is a release gate; it is not the final deliverable. Treat §11 and the completion section as authoritative over the stray reference to “eight streams” in §7. §§8–10 provide reference patterns and known failures, not permission to copy code or reintroduce XRush behavior.

| Evidence | Pin / interpretation |
|---|---|
| Specification target (upstream) | `charmbracelet/crush@0b0c79e32ada5c1e5ba26180023be740f0530f7b` |
| Observed implementation branch | `XTheocharis/crush@308d108d50b0f55bea8ad246180dc958a7dde692` (`umca`); [compare](https://github.com/XTheocharis/crush/compare/0b0c79e32ada5c1e5ba26180023be740f0530f7b...308d108d50b0f55bea8ad246180dc958a7dde692) |
| Specification blob | `a6f7095bb1de8d8dac24302a3b4820232c844cb2`; identical in `crush-umca.zip` |
| Historical fork | `XTheocharis/crush@dc96baac1c4c25bd3bcd164cea030d90fc3977e9` |
| Reference corpus | [Clone manifest](REFERENCE_REPOSITORIES.md): all 20 exact commits/trees and 81 distinct indexed file blobs verified. Archives are not required. |
| Upstream at preceding review | `charmbracelet/crush@6e54ac296b159b282433193d2dd21db51b0f2365`; [compare](https://github.com/XTheocharis/crush/compare/6e54ac296b159b282433193d2dd21db51b0f2365...308d108d50b0f55bea8ad246180dc958a7dde692) |
| Observed checkout tools | Go directive `1.27.4`, Fantasy `v0.48.2`, sqlc, CI golangci-lint `v2.15.0`; latest listed migration `20261109000000_add_skill_overrides.sql`. Static archive observations, not executed tools. Verify actual selected versions/generator/build context. |

The fork is 33 commits ahead of the historical specification target; at review it was 10 commits ahead and 0 behind current upstream main, with only `docs/umca.md` changed versus that main. Safety/permission/CLI changes are already inherited from upstream, not UMCA implementation. Default target is the observed fork commit; refresh both remote refs at U00, record the actual ancestry, tree and diff, and explicitly approve any moved target before freezing UMCA_BASE. Do not reset to upstream or overwrite inherited changes. Distinguish historical specification pin, current upstream comparison pin, and actual implementation/marker baseline. No Crush code was implemented or run in planning.

Final currency check (2026-11-25): fork `umca` remains `308d108d50b0f55bea8ad246180dc958a7dde692`; upstream `main` advanced to `535dad0ba50cd57e46a17907a76cb861081e1244` (one commit after `6e54ac296b159b282433193d2dd21db51b0f2365`). Compare: ahead 10, behind 1, merge base `6e54ac296b159b282433193d2dd21db51b0f2365`; the compare API lists only the spec on the fork side, **not** the full endpoint tree diff. Upstream-only changes are README.md and docs/channels/README.md. Keep the existing target pin for reproducibility; U00 explicitly chooses whether to sync and revalidates both endpoints, the actual tree diff and any changed documentation/contracts.


## 2. Required invariants (apply to every agent)

- Crush owns messages, accepted-run lifecycle, dispatch, tool effects and completion. No second transcript, synthetic continuation turn, duplicate executor, synthetic provider signatures or replay of unknown external effects.
- Stable versioned source identity is not a text hash or timestamp. Scope is host-derived; handles are not grants. Every read, join, ranking, expansion, cached result, export, and model egress is checked against current authority. The dispatch admission claim is the ordering point for revocation versus irreversible sends.
- Keep an assistant emission plus all required tool results indivisible; protect outstanding calls, signed/opaque continuations and applicable instructions. Check actual adapter attempts, not just a planner or PrepareStep. One smaller overflow replan at most, and none after output/effect escapes.
- One authoritative transactional frontier per writable lineage. Finalize after message flush; release connections during model/filesystem I/O; publish with CAS, exact ordered coverage and current policy. A stale candidate is never blindly rebased. Revoke dependent closure before edges disappear; deny access immediately, cleanup later. Fail closed for inaccessible origin-dependent material.
- Consent is independent of capability. Default native behavior stays intact; capture, auto compaction/model spending, learning, recall, remote egress/storage, personal grants and MCP exposure are independently controlled. No live credentials in tests; no real user content in fixtures or default logs.
- All optional features must have enabled, disabled and failure-path evidence. Incomplete capture, estimated usage, uncertain claims, ambiguous external effects and partial restore must be labeled truthfully. Do not claim remote erasure, exact token counts, or atomic filesystem/database rollback where not established.

- All agents obey Go context authority/version and explicit lifetime/resource/validation contracts; preserve compatible local conventions. Prefer same-package new files and cohesive new internal packages, not a new module/duplicate core. Every necessary inherited-file hunk uses paired UMCA markers or exact independently approved exception per patch policy. No broad style/format/modernization/dependency churn. No acceptance without independent Go alignment and footprint audit.

## 3. Agent roster and ownership

Read the named Markdown file and assign the work-unit ID, input commit, accepted dependency evidence and output location. Agents must implement and test, then return a structured report. All proposed package/type names are provisional; match actual host interfaces during U00.

| Agent | File | Work |
|---|---|---|
| HST | [01-host-projection.md](01-host-projection.md) | Accepted run, grouping, planner, checkpoint orchestration, actual-attempt admission |
| STO | [02-storage-provenance.md](02-storage-provenance.md) | Source/version/edges, schema/sqlc, CAS, legacy, deletion and fences |
| CAP | [03-artifact-capture.md](03-artifact-capture.md) | Producer capture, bounded artifacts, exact expand and cleanup |
| REC | [04-scoped-recall.md](04-scoped-recall.md) | Lexical, cursor, native read-only tools, lineage/attribution |
| MEM | [05-memory-evidence.md](05-memory-evidence.md) | Manual and automatic memory, task notes, observations/reflection, historical evidence (B) |
| HIE | [06-hierarchy-rollover.md](06-hierarchy-rollover.md) | Hierarchical checkpoint, pressure, rollover (A) |
| VEC | [07-hybrid-retrieval.md](07-hybrid-retrieval.md) | Embedding, vector generations, hybrid/reranker (C) |
| HEL | [08-helper-mcp.md](08-helper-mcp.md) | Prefetch, read-only helper, MCP server (D) |
| PRV | [09-provider-catalog.md](09-provider-catalog.md) | Catalog/cache/cost, adapter matrix and conformance (E/G) |
| PER | [10-personal-memory.md](10-personal-memory.md) | Personal namespace/grants/import/export (F) |
| DST | [11-distributed-durability.md](11-distributed-durability.md) | PostgreSQL, outbox, RabbitMQ, local/remote worker, migration and repair (H) |
| REW | [12-rewind.md](12-rewind.md) | Mutation capture, all four rewind modes, crash recovery (I) |
| OPS | [13-workspace-controls.md](13-workspace-controls.md) | Workspace/API/server/client/CLI/TUI, config, status, docs |
| QA | [14-verification-release.md](14-verification-release.md) | Independent fixtures, fault matrix, regression, measurements and release audit |
| AUD | [15-go-alignment-auditor.md](15-go-alignment-auditor.md) | Independent per-unit Go-context, exact version/API, tests, scope, minimal footprint and delimiters audit (candidate and integrated) |

**Single writers:** HST owns edits to `internal/agent/agent.go`, dispatch/model wrapper integration and coordinator wiring; PRV, HIE, HEL and other agents submit hook/interface requests to HST. STO owns migration allocation, `internal/db/sql/`, generated `internal/db/*.sql.go`, `models.go`, `querier.go`, shared db schema and source/session deletion plumbing; all other agents request schema slices from STO. OPS owns shared config, JSON schema, workspace/server/proto/client/backend endpoints and application wiring, CLI/TUI controls. CAP owns `internal/shell/` capture paths and Bash producer instrumentation; REW owns file mutation hooks and asks CAP/HST/OPS for shared wiring. REC owns native context search/expand tool implementations, HST owns resolver integration. PRV owns catalog/adapter-specific packages; coordinate MCP registration with HEL/HST. QA owns independent test harnesses/ledger, not product fixes. Agents can edit private feature packages and their tests in parallel. An owner may grant an explicit scoped write lease (paths, work ID, base SHA, duration until result) and review that diff; no overlapping leases. `go.mod`/`go.sum` changes require orchestrator review and one writer. No mass formatting of unrelated files. AUD owns review reports, not implementation. ORCH approves exact patch budgets and exception ledger serially. Require separate reviewer for QA/ORCH or AUD-authored changes; no author approves their own work. Prefer new `umca_*.go` files in existing packages; use cohesive new internal packages only for real boundaries. Existing module stays intact unless separately justified. Each lease includes expected inherited paths/markers and new owned paths. Review all added paths too.

## 4. Executable dependency graph and gates

Each row is a schedulable work unit; the referenced agent document contains implementation steps and evidence. Dependencies are AND; completed means accepted and merged locally, not “agent said done.” The orchestrator may parallelize design and private-package tests before dependencies land, but integration and gate acceptance wait. `G*` are virtual gate nodes verified by QA; a blocked gate does not make a feature optional. U00 establishes invariant contracts and provisional responsibilities (see §5); U01 confirms host signatures and versions before U02 is accepted. Review order preserves the six core increments in spec §6. Advanced feature work may develop on private branches before G6 if its actual dependencies have landed; do not advance final release gates prematurely.

| Work unit | Agent | Depends on | Gate/required result |
|---|---|---|---|
| U00 | ORCH + HST/STO/CAP/OPS/QA | — | Preflight, pristine native baseline and drift, provisional contracts, capability/permission model |
| U01 | HST | U00 | Record real multi-step call & attempt baseline, stable step mapping spike |
| U02 | STO | U01 | Foundational source/artifact/edge/admission schema, minimal transactional native/artifact deletion, transaction contracts |
| G1 | QA | U01, U02 | Baseline regression and measurements; no native behavior change |
| U03 | CAP | G1 | Earliest producer capture, truthful bounded artifacts and recoverable storage |
| U04 | OPS | U03 | Exact-handle inspect/expand/delete through workspace, CLI and configuration; capture off default |
| G2 | QA | U03, U04 | Capture fault/scope tests; first durable artifact includes minimal transactional deletion through all paths |
| U05 | STO | U02, G1 | Checkpoint CAS, flush, source deletion closure, legacy validation & migration compatibility |
| U06 | OPS | G2, U05 | Extend inspect/delete to checkpoints and compatibility paths |
| G3 | QA | G2, U05, U06 | Publication/deletion race, restart and one-connection tests |
| U07 | HST | G3, U01 | Legal cut, same-run projection, manual/auto compact, output reserve, per-attempt admission |
| U08 | OPS | U07 | Local first-slice project switch, manual controls, status and mode transition |
| G4 | QA | U07, U08 | Real next-request release fixture, first named adapter & native fallback |
| U09 | REC | G4 | Scoped lexical search, exact handles/cursors, receipt/provenance, native context tools |
| U10 | OPS | U09 | Full inspector, search/expand and hide/pin UI/CLI; native/connected parity |
| G5 | QA | U09, U10 | FTS & bounded fallback, ordinary writes/backfill, deny cross-session |
| U11 | MEM | G5 | Manual memory, task continuity, opted-in extraction & local job contracts |
| U12 | OPS | U11 | Review/edit/forget, learning and recall controls |
| G6 | QA | U11, U12 | Core memory and all flag combinations; functional foundation complete |
| A1 | HIE | G4, U11 | Hierarchical coverage/pressure/idle work and local rollover |
| B1 | MEM | G6 | Observations, reflection, bitemporal evidence and verification |
| C1 | VEC | G5 | Local scoped embeddings/exact vectors, fusion, reranking |
| D1 | HEL | G5, U07 | Prefetch and isolated read-only helper; cancellation/egress |
| D2 | HEL | D1, U10 | Opt-in MCP server & native parity |
| E1 | PRV | G4, U09 | Manifest/dynamic catalog, stable cache and usage ledger |
| G7P | PRV | G4, E1 | All provider rows and conformance (spec §11G; not gate G7) |
| H1 | DST | G3, U09 | PostgreSQL store, origin outbox, authority/fence, local jobs |
| H2 | DST | H1, U11 | RabbitMQ/worker, recovery, cutover & backup/repair fixtures |
| C2 | VEC | C1, H2 | pgvector partitioned backend, external projection/reindex/revocation |
| F1 | PER | B1, H2 | Personal memory/grants, offline denial & import/export |
| A2 | HIE | A1, H2 | Cross-store rollover activation/revocation/retry |
| I1 | REW | G4, U03, U05 | Rewind points/mutation journal and workspace fences |
| I2 | REW | I1, U07 | Four local modes, safe apply/recover/compensate; active lineage invalidation |
| I3 | REW | I2, H2 | Connected/multi-store receipt & recovery semantics |
| U13 | OPS | A2, B1, C2, D2, E1, G7P, F1, H2, I3 | All advanced switches/status, real CLI/TUI/API entry points and docs; incrementally wire per stage as APIs land |
| G7 | QA | G6, A2, B1, C2, D2, E1, G7P, F1, H2, I3, U13 | All A–I enabled/disabled/failure/rollback gates, cross-boundary acceptance |
| U14 | QA + ORCH | G7 | Full release evidence, matched measurements, rollback runbook, final audit |

**Dynamic schema/wiring subunits:** whenever a unit needs storage or shared integration, request `S.<unit>.<n>` (STO), `H.<unit>.<n>` (HST), or `O.<unit>.<n>` (OPS). Record requested types/DDL, consumer, tests, base and ordering. QA fixture additions use `Q.<gate>.<n>` as owner units with separate independent audit; ORCH-authored executable changes likewise get a scoped unit. Virtual gates themselves remain read-only. Do not require the consuming feature to be complete before its prerequisite schema/contract lands; accept small additive schema/API slices with fixtures, then integrate dependent behavior. The owner reviews migrations and regenerated diffs even when delegating a scoped lease. Add these nodes to the ledger as dependencies of the requesting unit; a unit cannot pass its gate with a pending subunit. Validate expanded graph after each insertion. A prerequisite cannot depend on acceptance of its requesting unit: split an independent contract/fixture from later consumer wiring. Owner/wiring work lands before each applicable acceptance test; U13 is final parity, not the first advanced wiring. Merge U05 after G2 even if its development is concurrent. Never hold shared-file, migration allocation, runner slot or integration leases while waiting on a prerequisite that needs them. Freeze/park private work, release the contested lease, then revalidate/reacquire after dependency acceptance. Validate the resource wait graph as well as the DAG; no nested blocking owner requests. Schedule OPS slices as advanced APIs land rather than waiting for every feature to complete.

**Mandatory dynamic audit expansion (all units):** model each implementation/review-ready candidate as `CAND.<unit>.<attempt>`, followed by `AUD.<unit>.<attempt>.candidate`, then serialized `MERGE.<unit>.<attempt>`, then `AUD.<unit>.<attempt>.integrated` plus independent affected QA checks. A unit becomes ACCEPTED only after both audit decisions PASS and required checks pass on the integrated SHA. Original dependency edges reference ACCEPTED unit, never candidate. Each audit depends on a frozen candidate/merge SHA, not acceptance of its own unit; this avoids a cycle. Apply this to U00/U01 investigative outputs (patch N/A only if read-only), all U/A–I implementation nodes, G7P, each dynamic S/H/O unit, U13 increments, QA/ORCH-generated test/tool changes, rework and conflict fixes. G1–G7 and U14 additionally inspect cumulative audit coverage. QA-authored code gets independent AUD, and a separate reviewer for AUD-authored work. No bypass for tiny change, disabled path or previously approved file. A failed/blocked audit returns the unit to REWORK/BLOCKED; keep independent units schedulable. Record audit attempt and evidence paths in ledger. For read-only/no-diff tasks, audit the exact inspected SHA and evidence twice as candidate/integrated (same SHA if unchanged), mark merge N/A, never manufacture a commit. A candidate uses its own SHA-bound exception manifest copied from its accepted base; serialize reconciliation into the global integration manifest at merge, re-approve changed hash-bound records. If a downstream commit changes an accepted contract or reviewed lines, invalidate affected prior evidence and re-run the required audits/tests before dependent acceptance.

## 5. Establish invariants at U00; version actual interfaces after U01 and before consumption

Keep `contracts.md` in UMCA_STATE (outside Crush); commit only necessary product contract documentation. U00 records provisional responsibilities, U01 validates actual host signatures, each consumer freezes the minimum applicable version before work; change by reviewed migration/compatibility decision. Name exact consuming package(s), owner, concrete Go signatures, status codes, schema invariants and fixtures. Resolve actual selected APIs first with go doc/source, repository and domain overlays. Document values/zero/nil/units/encoding/order, input and result aliasing/ownership, simultaneous callers, lifetime/cancel/close/join, stable error identities/translation, compatibility and test boundary per exported operation. Start concrete; narrow consumer-owned interfaces only where actual substitution or transaction/adapter boundary exists. Avoid speculative types/interface factories. Record proposed interfaces below as responsibilities; prefer host types. No Fantasy types in pure context domain. Keep authoritative transaction-bound queries behind storage interfaces.

| Contract | Minimum data / operations | Provider → consumer |
|---|---|---|
| Scope & source | Host principal/resource set, session/workspace/lineage, source ID + immutable version, representation + digest/ranges, authority class, order/group/ordinal, finalization, current policy and generation | STO/HST → all |
| Source resolution | `Finalize` expected version/idempotency, `Resolve` scoped handle/version, `Delete` closure and cleanup journal; ambiguous/unknown distinct from confirmed | STO → all |
| Projection | ordered immutable groups + required segments, current live tail, coverage, protected input, optional items, tool catalog generation, effective model/wire options and policy, estimate/reserve, receipt | HST/STO/PRV → OPS/REC/HIE |
| Publication | `Snapshot`, `PublishCheckpoint` expected revision+predecessor+digests+policy/fence, append-only revalidation; transaction-scoped querier; revoke before closure edges removed | STO → HST/HIE/REW |
| Admission | purpose, accepted model pin, request/attempt key, eligible source versions and policy epoch; storage-checked claim, output/tool-escape bits, replan allowance | HST/STO → PRV/MEM/HEL/DST/VEC/PER |
| Artifact | producer/capture ID, scoped representation, observed/retained ranges and verified digest, status/reason, opaque locator, bounded expand/cleanup; reserve temp and recovery capacity | CAP/STO → REC/REW/OPS |
| Recall | query, scope derived by host, kinds, bounded limit, cursor; handle/version, representation, snippets, completeness, evidence; expansion range/cursor tied to source and policy | REC → HEL/VEC/MEM/OPS/PER |
| Derived writes | reads-from edges + origin generation; purpose and operation key; duplicate same payload idempotent, conflicting replay rejected; current eligibility required | STO/DST → all |
| Job | durable input refs, purpose, op key, policy generation, attempts, lease owner/fence/expiry, cancel; claim/heartbeat/commit result/outbox; no re-execution of unknown effects | DST/STO → MEM/HIE/VEC/HEL/PER |
| Rewind | point ID/frontier, workspace identity, capture/preview token+fingerprints, mutation intents and outcomes, idempotent operation/receipt, generation fence, resolution statuses | REW/STO → HST/OPS/DST |
| Capability / settings | implemented vs enabled, available reason, last validated version, retention/consent/cost; common typed errors `busy`, `stale`, `unavailable`, `partial`, `needs_resolution`, `incompatible`, `limit` (proposed names) | OPS → all |

Document admission ordering: revocation wins before claim → deny; claim wins before revocation → in-flight, best-effort cancel, no new read/publication. A DB commit cannot atomically send a network call. A generation check before the claim is not equivalent. Record non-content dependency receipts even with diagnostics disabled. Cross-store references are provisional until live origin admission. For active recovery, revocation overrides payload pins.

## 6. Bootstrap and runner protocol

Use a clean clone or existing clone; do not overwrite a dirty user checkout. Set `PLAN_DIR` to the absolute path of the extracted plan documents. Run commands locally; do not push, merge a remote PR, deploy services, release/tag, spend on live providers, or transmit data without explicit authorization. Implementation can complete locally. If authorized later, use draft PRs with evidence; never force push.

```sh
# Start in the approved clean clone. Read this whole section before running.
# The POSIX shell snippets apply to the orchestration host, not Crush's target OS.
set -eu
umask 077
export UMCA_BASE="${UMCA_BASE:-308d108d50b0f55bea8ad246180dc958a7dde692}"
export UMCA_SPEC_BASE=0b0c79e32ada5c1e5ba26180023be740f0530f7b
export UMCA_RUN_ID="${UMCA_RUN_ID:?Choose a unique run ID; reuse only for verified resume}"
case "$UMCA_RUN_ID" in *[!a-zA-Z0-9_-]*|'') exit 2;; esac
: "${PLAN_DIR:?Set PLAN_DIR to the extracted plan directory}"
export PLAN_DIR
python3 "$PLAN_DIR/tools/validate_package.py" --root "$PLAN_DIR"
ROOT=$(git rev-parse --show-toplevel)
export UMCA_STATE="$(dirname "$ROOT")/$UMCA_RUN_ID-state"
export UMCA_WT="$(dirname "$ROOT")/$UMCA_RUN_ID-worktrees"
export UMCA_REFS="$UMCA_STATE/references"
export UMCA_ACCEPTED_REF="refs/heads/umca/impl/$UMCA_RUN_ID/integration"
# ORCH first verifies paths are private, owner-controlled, outside all worktrees,
# not symlink redirects, and acquires a single exclusive run-owner lock at a separate owner-controlled sibling path (not a file inside the yet-uncreated state directory).
# FIRST RUN ONLY, after explicit inspection of refs/paths and drift:
mkdir "$UMCA_STATE" "$UMCA_WT"
mkdir "$UMCA_REFS"
# Verify approved remote URL; fetch missing exact objects only with authority.
git remote get-url origin
git cat-file -e "$UMCA_BASE^{commit}" || git fetch https://github.com/XTheocharis/crush.git "$UMCA_BASE"
git cat-file -e "$UMCA_BASE^{commit}"
git merge-base --is-ancestor "$UMCA_SPEC_BASE" "$UMCA_BASE"
test "$(git rev-parse "$UMCA_BASE:docs/umca.md")" = a6f7095bb1de8d8dac24302a3b4820232c844cb2
git diff --name-status "$UMCA_SPEC_BASE" "$UMCA_BASE" > "$UMCA_STATE/baseline-drift.txt"
git ls-tree -r --name-only "$UMCA_BASE" > "$UMCA_STATE/inherited-paths.txt"
printf '[]\n' > "$UMCA_STATE/patch-exceptions.json"
# The accepted integration branch is never checked out. A zero old value is CAS-create.
git update-ref "$UMCA_ACCEPTED_REF" "$UMCA_BASE" 0000000000000000000000000000000000000000
git worktree add --detach "$UMCA_WT/baseline" "$UMCA_BASE"
cd "$UMCA_WT/baseline"
# Record each command/result individually; a failure is a blocker, not a pass.
GOTOOLCHAIN=local go version
go version
go env -json GOMOD GOWORK GOVERSION GOTOOLCHAIN GOOS GOARCH CGO_ENABLED GOFLAGS GOEXPERIMENT
go mod edit -json
go list -mod=readonly -m -json all
sed -n '1,14p' go.mod
ls internal/db/migrations | sort | tail -5
```

The setup above is intentionally **first-run only**. On resume do not rerun mkdir, initialize manifests, or recreate the accepted ref. Verify the recorded run identity, package/spec digest, base, registered worktrees, exclusive owner lock and existing accepted ref; reconcile any pending promotion before dispatch. Existing runs made with the older checked-out integration-branch protocol require explicit migration with preserved commits, not an automatic rewrite. A new branch/spec blob requires a reviewed plan/input update, not disabling the check. Set the run lock using an atomic exclusive creation on a trusted local filesystem; bind it to an owner identity and process/runner incarnation, never break it solely by elapsed wall time. On abrupt shutdown confirm no live owner before authorized recovery. If the initial bootstrap fails partway, reconcile the exact created files/ref and resume or start a fresh run ID, without deleting user files. Capture failures and continue independent inspections; do not let shell `set -e` silently skip evidence. Preserve accepted ref and state outside task worktrees.


U00: run all shipped tooling tests and package validator, then read `AGENTS.md`, nested guides, `Taskfile.yaml`, actual affected source, `docs/umca.md` and the diffs for touched files. Map all truncating producers, dispatch construction/retry paths, native and connected admin paths, SQLite variants, adapter families, native mutation points; write inventory and baseline blockers. Use [REFERENCE_REPOSITORIES.md](REFERENCE_REPOSITORIES.md) and acquire pinned Git repositories per active unit before choosing its design. Read source anchors and surrounding implementation/tests, verify exact blobs/license, prefer clean implementation. No archives or latest-branch substitution. Save acquisition/reading receipts. Run the pristine native baseline at UMCA_BASE before any schema/feature commits; U01 records instrumented baseline before U02 migration acceptance. Record actual baseline toolchain/dependency availability. No implicit dependency upgrades. Before implementation save `project-facts.md` (source language/stdlib API/effective toolchain/deployed runtime+GODEBUG/selected dependencies/tools/build matrix independently), `go-context-read.json`, selected dependency/domain overlays, actual Go API and contract evidence, inherited patch budget/marker policy and initial clean mechanical audit. Use exact selected sqlc/driver/Fantasy/provider/OS/protocol sources; read canonical scripts first. Record `.golangci.yml` and CI-pinned v2.15.0 rather than corpus versions or the unpinned Taskfile install command. If Go or service availability blocks tests, report exact command/error; keep dependent gates unverified. Use synthetic test fixture data and disposable databases/worktrees only. Isolate HOME/config/cache/data, remove ambient provider credentials from child test environments and use explicit network policy/allowlisted local fixture addresses; inspect VCR behavior for missing cassettes before any suite. No implicit live calls on missing fixtures. Dependency fetches and local test services use separately approved network paths; no user project state or credentials.

```sh
# Core baseline and subsequent full checks. Run touched-package tests first.
CGO_ENABLED=0 GOEXPERIMENT=greenteagc go test ./...
CGO_ENABLED=0 GOEXPERIMENT=greenteagc go build .
# Run a separate race test in supported CGO/race environment (not pure-Go gate).
CGO_ENABLED=1 go test -race ./internal/agent ./internal/message ./internal/session ./internal/db ./internal/workspace
# Release: reproduce selected CI on supported native target/toolchain and compiler.
CGO_ENABLED=1 go build -race ./...
CGO_ENABLED=1 go test -race -failfast ./...
# For SQL/config changes (owners only) regenerate, then inspect diff:
task sqlc
task schema
# Explicit vet (test vet subset alone is insufficient). Scope per unit, full at release.
go vet ./...
# Nonmutating consistency check when modules/imports change (verify selected help).
go mod tidy -diff
# Format only changed Go files with repository gofumpt (or approved fallback),
# assert the formatter check has no findings. Check pinned canonical lint.
task lint
# Run actual CI race build/test matrix where supported, with compatible CGO/runtime.
# Follow current risk ladder: bounded fuzz, real driver/broker/platform/transport tests,
# generated drift check, selected govulncheck finding gate and representative metrics.
```

No need to run the race command on an unsupported platform; record it as blocked, not passed. Do not run `task test:record` against live providers; deterministic mock transports and existing recorded fixtures only. `task test` sets `CGO_ENABLED=0` but invokes `-race`; use the explicit separate commands above. Do not change the repo toolchain or race settings silently to hide a failure. Check `AGENTS.md` for formatting/semantic commit rules and `internal/ui/AGENTS.md` for UI work. Use canonical CI as required, compare its actual clean module diff and native/race behavior. All snippet commands are plans, verify exact supported flags and capture commands/exit status/output; run selected environment and scope rather than blindly applying every command.

### U00 mechanical audit bootstrap

In the clean detached baseline checkout, with the selected base/HEAD and the initialized exception manifest, run `python3 "$PLAN_DIR/tools/test_audit_upstream_patch.py" -v` (disposable Git fixtures) then `python3 "$PLAN_DIR/tools/audit_upstream_patch.py" --repo . --base "$UMCA_BASE" --head HEAD --exceptions "$UMCA_STATE/patch-exceptions.json" --output "$UMCA_STATE/baseline-upstream-audit.json"`. Verify the initial inherited change count is zero, unless explicitly resuming a documented accepted implementation. Use manual equivalent Git checks if Python unavailable, disclose limitation. Tools run outside the application module; no added Go dependencies. Record tool version, command output and actual base. Do not confuse fixture PASS with Crush implementation correctness.

### Dispatch a work unit

For each READY unit, create a separate worktree based on the current accepted integration commit. Avoid executing the same unit concurrently. A task runner may have its own spawn mechanism; call it with the full agent document and work order. If no spawn facility exists, one implementer may execute nonconflicting implementation roles sequentially, but cannot self-approve. A genuinely separate reviewer/session/authorized human must perform each independent AUD and QA decision; if unavailable, mark the relevant gate BLOCKED and proceed only with independent unblocked work. A second pass by the author is not independent review. Example commands (substitute task ID and actual accepted SHA):

```sh
# New attempt only; ORCH holds run-owner lock and selects accepted evidence.
UNIT=U03
ATTEMPT=1
BASE=$(git rev-parse --verify "$UMCA_ACCEPTED_REF^{commit}")
# Verify accepted ledger snapshot matches BASE and dependency evidence first.
ATTEMPT_DIR="$UMCA_STATE/$UNIT/attempt-$ATTEMPT"
mkdir -p "$UMCA_STATE/$UNIT"
mkdir "$ATTEMPT_DIR"
BRANCH="umca/impl/$UMCA_RUN_ID/$UNIT-$ATTEMPT"
WT="$UMCA_WT/$UNIT-$ATTEMPT"
# Snapshot the exact accepted manifest for BASE; never overwrite on resume.
cp "$UMCA_STATE/patch-exceptions.json" "$ATTEMPT_DIR/patch-exceptions-candidate.json"
printf '%s\n' "$BASE" > "$ATTEMPT_DIR/base-sha"
git worktree add -b "$BRANCH" "$WT" "$BASE"
```

On resume use the attempt's recorded base, branch, worktree and manifest, not today's integration HEAD or accepted ref. Verify branch registration and ancestry from that recorded base; if integration advanced, independently check dependency/contract drift and refresh/re-audit as required. Keep old attempts immutable; a new attempt gets new paths/branch, not overwritten logs or manifests. All `$UNIT` evidence examples in this package refer to the attempt-specific directory (`$ATTEMPT_DIR`) for candidate files; the unit ledger points to it.


Dispatch implementation prompt (fill placeholders, include paths to full documents; not abbreviated excerpts):

> You are `<AGENT>`. Read `<PLAN_DIR>/<agent-file>`, `00-orchestrator.md`, `IMPLEMENTATION_GUIDE.md`, `UPSTREAM_PATCH_POLICY.md`, `GO_CONTEXT_READ_MAP.md`, and the complete assigned portions of `GO_AGENT_CONTEXT.md` using `GO_CONTEXT_INDEX.json`. Read `GATE_CONTRACTS.md`, `REFERENCE_REPOSITORIES.md`, acquire your relevant pinned sources and read `docs/umca.md`, repo AGENTS, selected exact APIs, contract decisions and accepted dependency evidence. Record actual read ranges and SHA-256, rule IDs/applicability, project fact sheet, relevant dependency/domain overlays, and unknowns. Execute work unit `<UNIT>` in `<WORKTREE>` at `<BASE_SHA>`. Dependencies: `<IDs + commits + evidence paths>`. Lease: `<paths, owner, expected inherited hooks/marker IDs, new paths>`. Prefer new same-package/cohesive internal files; minimal necessary marked inherited hooks; approved exact sidecars only where inline comments cannot safely be used. Request owner subunits before shared edits. Implement, format only intended files, test according to real repository/version/risk, commit locally, save `report.json`, `go-alignment.md`, patch budget/diff and sanitized command logs. Do not claim tests you did not run, modify unrelated files, call live services, push or ship. On handoff request independent AUD; do not claim ACCEPTED. Continue independent work if blocked and report exact evidence. Do not spawn recursive unconstrained agents.

Dispatch audit prompt immediately after each candidate handoff and after each actual merge:

> You are an independent `<AUD reviewer ID>`, not the author. Read `15-go-alignment-auditor.md` and applicable full Go context sections and owner plan. Review `<UNIT>` attempt `<N>`, stage `<candidate|integrated>`, actual SHA `<SHA>`, frozen UMCA_BASE, dependencies, report, patch budget, code/consumer diff, exact selected API contracts and actual test evidence. Run marker/footprint helper and independent targeted validation. Inspect every inherited hunk and exact exception; verify minimized footprint and full applicable Go rules. Return PASS/REWORK/BLOCKED, rule IDs, file:line evidence and precise fixes in `<STATE>/<UNIT>/audit/<N>/<stage>/audit.json` and `audit.md`; never fix candidate yourself. Reject stale inputs, unapproved exceptions, missing checks or unresolved contract violations. A passing audit is commit-specific, not a waiver of later QA gates.

### Persistent orchestration state

Store outside the repo in `UMCA_STATE/`. Per-unit author evidence is nested under `attempt-<n>/`; audit stages are separately SHA-bound; earlier reports remain immutable. Required records: `ledger.json`, `decisions.md`, `baseline-drift.txt`, `contracts.md` (copy of accepted version), per-unit `report.json`, `go-alignment.md`, `patch-budget.md`, `commands.log`, `test-results/`, `review.md`, `audit/<attempt>/{candidate,integrated}/audit.json`, `audit.md`, `audit-commands.log`, `upstream-audit.json`, and `gate-G*.md`; `project-facts.md`, `go-context-read.json`, `inherited-paths.txt`, domain/dependency overlays, `patch-exceptions.json`, delimited patch evidence, cumulative footprint manifest and audit ledger. Store confidential data only as allowed; no raw payloads/secrets. Only ORCH edits the ledger. Each unit record:

```json
{"id":"U03","agent":"CAP","deps":["G1"],"state":"PENDING","attempt":0,"base_sha":null,"commit_sha":null,"lease":[],"tests":[],"blockers":[],"gate_ids":[],"report":null,"go_context_sha256":null,"reading_manifest":null,"patch_budget":null,"audit_attempts":[],"candidate_audit":null,"integrated_audit":null,"integrated_sha":null}
```

States: `PENDING`, `READY`, `RUNNING`, `REVIEW_READY`, `AUDIT_CANDIDATE`, `MERGE_READY`, `INTEGRATED_REVIEW`, `REWORK`, `ACCEPTED`, `BLOCKED`. Audit state `PENDING`, `RUNNING`, `PASS`, `REWORK`, `BLOCKED`; bind stage, author/reviewer, SHA and dependency/context hash. Only ACCEPTED releases dependencies. Gate states `PENDING`, `PASS`, `FAIL`, `BLOCKED`. On runner restart reconcile worktree HEAD/status, report and accepted integration commit; never create duplicate branches or repeat irreversible effects. Review evidence before treating any prior unit as accepted; verify actual read coverage, candidate and integrated audit decisions and approved exception hashes. Revalidate changed dependencies, marker manifests and affected fixtures when integration moves. Serialize ledger writes, use temporary files + checked flush/sync/close/rename and parent-directory persistence where supported for crash recovery; record durability limits and preserve append-only decisions. The accepted Git ref is the sole acceptance pointer, not the ledger or a trial worktree HEAD. A filesystem manifest update and a Git ref update are not one atomic transaction; follow the promotion journal below.

### Scheduler loop

1. Calculate READY nodes from [execution-graph.json](execution-graph.json), the table and recorded dynamic subunits and leases; validate acyclicity of actual acceptance/merge prerequisites on each insertion. Use [GATE_CONTRACTS.md](GATE_CONTRACTS.md) for staged fixture ownership, not later-feature dependencies. Prioritize core merge gates, then critical paths H2 → C2/F1/A2/I3 and G7P; run nonconflicting work in parallel up to actual runner capacity. No assumed fixed concurrency or runtime estimates.
2. Dispatch with full context; record worktree/base/attempt. Require a checkpoint report after each meaningful implementation slice and before context reset. If agent loses context, restart from commit and report, not transcript assumptions.
3. Transition to REVIEW_READY, freeze exact candidate/dependencies and dispatch independent `AUD.<unit>.<attempt>.candidate` immediately. AUD reads actual source and full applicable Go blocks, tests and every inherited diff hunk, verifies minimal footprint/markers/approved exceptions and issues PASS/REWORK/BLOCKED. ORCH checks ownership, spec, permissions, migration IDs and actual results; QA independently runs applicable tests. Reject TODO/stubs as complete or stale approval. If rebase/conflict, author resolves on task branch with owner review, rerun affected tests and candidate audit. No self-review, no silent security-sensitive conflict resolution.
4. Only after candidate audit PASS and required tests, transition MERGE_READY. Hold the single integration lease through trial creation, merge, integrated audit/QA and promotion or rejection. For U05 merge only after G2. Read accepted parent from `$UMCA_ACCEPTED_REF`; freeze the exact candidate SHA (never merge a moving branch tip), verify it against the audit and dependency versions. Create a new trial worktree and branch based on the accepted parent; `git merge --no-ff <candidate-sha>` there. If the merge conflicts, preserve evidence and have the owner resolve in a new candidate attempt; re-audit. On success record trial merge SHA and both parents, reconcile exact exception records from that tree, run tests and independent integrated AUD there. No accepted ref moves yet. Read-only investigations/gates use no-op merge and exact accepted evidence; tests or fixes authored by QA/ORCH are separate implementation units with both audits. If an unrelated accepted commit advances while candidate work is underway, evaluate affected dependencies/leases and re-audit changed assumptions. A trial is rejected without reverting or changing accepted history; retain its branch and failed evidence. A fresh repair branch from the accepted parent must include the complete intended diff, not just a fix on top of a rejected merge. Never dispatch from trial HEAD.

5. For fixable failures return minimal repro and exact gate to owner; allow up to two rework cycles before re-triage (proposed scheduling policy, not product retry behavior). For persistent blocker, record diagnosis, missing prerequisite, affected gates and workaround if safe. Keep independent nodes moving. Never weaken a release criterion, applicable Go contract or marker requirement to unblock scheduling; only exact justified exceptions per policy, not broad waivers.
6. On full release gate verify every unit/shared slice/repair has candidate and integrated audit tied to accepted tree/contract versions, all Go read manifests and decisions, cumulative footprint and exact exceptions; rerun tests against the exact final commit; verify scope flags and documented fallback. Stop if unresolved safety defect, data leak, duplicate effect, unverified required adapter/stream, or missing recovery evidence. Return a partial-completion report if environment prevents full execution; do not report full completion.

### Trial and promotion protocol (mandatory)

Under the run-owner and integration leases, use a unique trial branch/worktree per unit attempt. Verify the integration branch is not checked out (inspect `git worktree list --porcelain`) before updating it. The integration branch is an acceptance ref only. Example (validated variables and exact audited SHA from ledger, no shell substitution from agent text):

```sh
PARENT=$(git rev-parse --verify "$UMCA_ACCEPTED_REF^{commit}")
# CANDIDATE_SHA, UNIT, ATTEMPT already validated against candidate PASS.
TRIAL="$UMCA_WT/trial-$UNIT-$ATTEMPT"
git worktree add -b "umca/impl/$UMCA_RUN_ID/trial-$UNIT-$ATTEMPT" "$TRIAL" "$PARENT"
git -C "$TRIAL" merge --no-ff -m "UMCA trial $UNIT attempt $ATTEMPT" "$CANDIDATE_SHA"
TRIAL_SHA=$(git -C "$TRIAL" rev-parse HEAD)
# Stop here until independent integrated AUD + QA pass on TRIAL_SHA.
# Confirm first parent == PARENT, candidate is an ancestor, and tree clean.
# Persist immutable prepared promotion journal and SHA-bound manifest BEFORE CAS.
# Only after the above, and only under the exclusive integration lease:
git update-ref "$UMCA_ACCEPTED_REF" "$TRIAL_SHA" "$PARENT"
```

ORCH writes a durable prepared promotion record containing run/unit/attempt, expected old SHA, new SHA, candidate/merge parents, exact audit/QA IDs and hashes, exception manifest snapshot/digest, dependency versions and intended ledger changes. Once the prepared record is persisted, compare-and-swap the accepted ref. Verify the ref after the command, including on error. If it equals the prepared new SHA, finalize the ledger and publish the corresponding accepted manifest idempotently; if it equals old SHA, no promotion happened and a revalidated retry is allowed. If neither, stop dispatch and investigate unexpected writer/recovery state. A crash between ref CAS and manifest/ledger publication is resolved by that journal, not by using stale mutable files. Do not release the integration lease or start new units until all accepted evidence points to the same SHA. No merge/revert of a rejected trial is needed. If a prior protocol already recorded a revert of a rejected merge, reapply the full reviewed intended change on the accepted tree (not merely merge the same branch, which Git considers merged). No destructive reset of user work. A later *accepted* defect is repaired through a separately reviewed fix; invalidate affected gates and evidence rather than rewriting accepted history. Run the disposable orchestration fixture tests before using the protocol.

### Agent handoff schema (required for every unit)

```json
{
  "unit":"U03", "agent":"CAP", "status":"REVIEW_READY",
  "target_sha":"<base>", "commit_sha":"<head>",
  "files_changed":[], "contract_versions":[], "dependencies":[],
  "migration_ids":[], "generated_commands":[], "settings_and_default":[],
  "reference_evidence":[{"repository":"<id>","commit":"<sha>","tree":"<sha>","read_anchors":[],"receipt":"...","license_review":"...","adopted_or_rejected":"..."}],
  "entry_points":[], "tests":[{"id":"CAP-01","command":"...","exit_code":0,"result":"PASS","log":"..."}],
  "rollback":"...", "decisions":[], "limitations":[], "blockers":[], "next_consumers":[],
  "go_context":{"sha256":"<exact>","role":"CAP","required_ranges":[],"read_ranges":[],"rule_ids":[],"extra_task_tags":[],"limitations":[]},
  "project_facts":"...", "dependency_domain_overlays":[], "go_alignment":"go-alignment.md",
  "ownership_lifetime":[], "api_contract_evidence":[], "go_rule_deviations":[],
  "patch_budget":"patch-budget.md", "inherited_paths":[], "new_paths":[], "marker_ids":[],
  "patch_exceptions":[], "upstream_audit":"upstream-audit.json",
  "audit_requested":true, "candidate_audit":null, "integrated_audit":null
}
```

Use `BLOCKED` or `PARTIAL` as status if not ready; no fake commit/exit code. For manual cases include procedure, observed output, fixture and environment. Include source anchors and exact package/test names. Include cost/retention policy, no secrets/raw prompts. QA's evidence and AUD's candidate/integrated decisions must be separate from implementer's claim. ORCH checks actual read ranges cover assigned ranges, commit IDs match, required evidence is complete and accepted exceptions are exact. Each unit report also has a compact resource ownership/stop/join and exported API/wire/error review (or concrete N/A).

## 7. Release criteria and verification map

| Required evidence | Owner/gate |
|---|---|
| Native behavior parity, accepted run + folded prompt + actual adapter call | HST, QA / G1,G4 |
| Producer capture fidelity, quotas, failures, marker expansion, cross-scope denial | CAP, STO, QA / G2 |
| Transaction flush/CAS, legacy pointers, concurrent revoke, one conn, restart, deletion descendants | STO, QA / G3 |
| Smaller second request within one accepted run, live tool group integrity, output reserve and safe overflow | HST, PRV, QA / G4 |
| FTS on each supported build, ordinary insert/delete and preexisting-row backfill, bounded fallback | REC, QA / G5 |
| Manual memory review/forget, task notes, stale worker denial, consent matrix | MEM, OPS, QA / G6 |
| A: 3 levels, branch isolation, hysteresis, rollover activation; B: as-of and evidence deletion | HIE/MEM / G7 |
| C: retrieval quality, scope before candidate selection and at use, revoked vectors; D: read-only helper and MCP protocol/authorization | VEC/HEL / G7 |
| E/G: dynamic schemas, cache metrics, all real provider rows, attempts/usage | PRV/HST / G7 |
| F/H: grants offline, PG/RabbitMQ crash/partition/lease/cutover/restore, no resurrection | PER/DST / G7 |
| I: 4 rewind modes, path/worktree conflicts, partial failure, compensation, lost response and real next provider call | REW/OPS/HST / G7 |
| All stages: real workspace/CLI/TUI enabled/disabled/outage and rollback, metadata-only diagnostics, secure egress/deletion, matched workloads | QA/OPS / U14 |
| Every unit: source reading, selected-version facts, Go applicability, real validation, minimal inherited footprint and exact markers/exceptions | AUD / candidate and integrated audits, cumulative G1–G7/U14 |

Use §6 release fixture as a mandatory cross-agent integration fixture, §7 checklist and §11 boundary table as the detailed acceptance source. QA writes exact test IDs and commands into the release ledger, links the commit and logs. No performance promises before matched baselines; collect sizes, fidelity, duplicate calls, DB reads, disk, p50/p95 prep, time to first output, retrieval success/quality, total usage including retries/rejected summaries/background. Report sample count, hardware, workload, estimates/unknown costs, confidence and known fallback wins. Set justified thresholds after measuring baseline; do not move them post hoc to pass. If a capability is not implemented or verified, leave it open even if default-off. Rollback disables new jobs/publications, preserves authorized read/delete, validates transition and revocations; migrations remain unless explicit data-owner approval. Never downgrade to an older binary without proving its replay cannot bypass tombstones; otherwise provide filtered export and mark unsupported.

## 8. Final deliverables from implementation run

Return the local integration branch/commit, ordered commit list, requirement coverage ledger, generated schema/API diffs, exact build/test commands and observations, feature/consent matrix, operator docs and rollback/recovery runbook. Include Go context hash/range coverage, project and exact dependency/domain overlays, rule/decision ledger, all candidate/integrated audit records, cumulative new/inherited path and marker counts, per-file hook manifest and approved delimited exceptions, before/after diff stats, upstream sync procedure and validation/limitations. Confirm every inherited file modification is safely marked or has a specific exact approved exception; do not hide footprint by moving upstream code to new files. Include residual risks, failed/unrun provider/platform tests, unverified costs, open nodes and next safe action. Offer remote PR/push only if the user authorizes it. A finished plan is not an implemented or validated UMCA.
