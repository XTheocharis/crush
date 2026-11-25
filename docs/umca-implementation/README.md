# UMCA — final reviewed autonomous implementation package

This package supersedes all earlier plan revisions. Read [FINAL_REVIEW.md](FINAL_REVIEW.md) for the final correction pass. It preserves all 37 original work/gate nodes, six core increments, and §11 A–I of the full UMCA architecture. It adds a mandatory, independent Go-context and upstream-footprint sweep after every implementation unit, including incremental owner wiring and repairs. No implementation has been performed by preparing these documents.

## Start here

- [00-orchestrator.md](00-orchestrator.md) — primary execution runbook, role ownership, scheduling graph, per-unit and integrated audit gates, dispatch prompts and persistent handoff state.
- [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md) — Go alignment and existing Crush conventions, exact version/API/target selection and validation, cross-stream requirements.
- [GO_AGENT_CONTEXT.md](GO_AGENT_CONTEXT.md) — complete supplied document, byte-identical, including its reference package code, policy/rule catalog, version data, tools, package landscape, workflows and playbooks.
- [GO_CONTEXT_READ_MAP.md](GO_CONTEXT_READ_MAP.md) and [GO_CONTEXT_INDEX.json](GO_CONTEXT_INDEX.json) — exact full-source reading assignments for every agent, including full rule ranges. ORCH and AUD read the entire original; every specialist reads common and assigned full sections plus newly applicable blocks. Use complete chunked reads if context capacity requires it.
- [UPSTREAM_PATCH_POLICY.md](UPSTREAM_PATCH_POLICY.md) — new-files-first policy, paired markers, exact exceptions, cumulative diff and future upstream sync process.
- [15-go-alignment-auditor.md](15-go-alignment-auditor.md) — independent per-candidate and integrated audit role; no self-review and no acceptance if required checks fail or are blocked.
- [reference/umca.md](reference/umca.md) and [reference/INPUTS.md](reference/INPUTS.md) — unchanged product specification and exact input manifest.

## Review and execution evidence

- [AUDIT_REPORT.md](AUDIT_REPORT.md) — findings, source evidence, corrections and unresolved implementation verification.
- [ACCEPTANCE_TRACEABILITY.md](ACCEPTANCE_TRACEABILITY.md) — original requirements mapped to gates and fixtures, including all 31 checklist items.
- [GATE_CONTRACTS.md](GATE_CONTRACTS.md) and [execution-graph.json](execution-graph.json) — stage-specific acceptance and scheduling, including merge constraints.
- [REFERENCE_REPOSITORIES.md](REFERENCE_REPOSITORIES.md), [reference/repos.json](reference/repos.json) and [tools/acquire_references.py](tools/acquire_references.py) — clone-based reproducible reference acquisition.

## Agent documents

| Agent | Plan |
|---|---|
| HST | [01-host-projection.md](01-host-projection.md) |
| STO | [02-storage-provenance.md](02-storage-provenance.md) |
| CAP | [03-artifact-capture.md](03-artifact-capture.md) |
| REC | [04-scoped-recall.md](04-scoped-recall.md) |
| MEM | [05-memory-evidence.md](05-memory-evidence.md) |
| HIE | [06-hierarchy-rollover.md](06-hierarchy-rollover.md) |
| VEC | [07-hybrid-retrieval.md](07-hybrid-retrieval.md) |
| HEL | [08-helper-mcp.md](08-helper-mcp.md) |
| PRV | [09-provider-catalog.md](09-provider-catalog.md) |
| PER | [10-personal-memory.md](10-personal-memory.md) |
| DST | [11-distributed-durability.md](11-distributed-durability.md) |
| REW | [12-rewind.md](12-rewind.md) |
| OPS | [13-workspace-controls.md](13-workspace-controls.md) |
| QA | [14-verification-release.md](14-verification-release.md) |
| AUD | [15-go-alignment-auditor.md](15-go-alignment-auditor.md) |

## Execute

Extract outside the target Git worktree. No source archives are required: acquire pinned relevant Git repositories on demand as described in [REFERENCE_REPOSITORIES.md](REFERENCE_REPOSITORIES.md). Set `PLAN_DIR` to this directory's absolute path. Give the runner the following instruction (requires shell, Git, file editing, and for true independent parallel execution, child-agent facility):

> Read 00-orchestrator.md, IMPLEMENTATION_GUIDE.md, UPSTREAM_PATCH_POLICY.md, GATE_CONTRACTS.md, REFERENCE_REPOSITORIES.md, and the complete GO_AGENT_CONTEXT.md, including every section and embedded example. Checkpoint read ranges if context is limited; do not count truncated output. Act as the implementation orchestrator. Execute U00 on verified target, establish pristine baseline before schema changes, acquire required references by pinned Git commit, resolve repository and exact Go/dependency facts, initialize persistent state and dispatch agents according to the dependency graph from the accepted integration ref, never an unapproved trial HEAD. Give each child its plan and complete role-specific reading assignment; require actual selected API and test evidence. Prefer new files in existing packages and cohesive new internal packages, with only minimal marked inherited hooks. For every implementation handoff run independent AUD candidate review and integrated review before ACCEPTED, in addition to existing QA gates. Repair/re-audit failures, preserve partial state and independent scheduling, and continue through all A–I stages and U14. Do not modify the Go directive or existing upstream conventions without explicit justified approval, push/deploy/spend, or declare untested work complete.

Tooling: [tools/acquire_references.py](tools/acquire_references.py) acquires exact reference Git objects and verifies source identities; [tools/test_acquire_references.py](tools/test_acquire_references.py) exercises disposable Git fixtures. [tools/audit_upstream_patch.py](tools/audit_upstream_patch.py) is a read-only Python stdlib/Git helper to check inherited diff markers, exact exception matching and footprint; [tools/test_audit_upstream_patch.py](tools/test_audit_upstream_patch.py) exercises it in temporary Git repositories. This is orchestration tooling outside Crush; no new application dependency. Mechanical PASS is necessary where available, not proof of code semantics, generator correctness or audit independence. See [VALIDATION.md](VALIDATION.md) for the checks actually performed in packaging.

Before starting, run `python3 tools/validate_package.py --root .` from the extracted package. Run `python3 -m unittest discover -s tools -p "test_*.py" -v` for local helper fixtures. These check plan/tooling, not Crush. The accepted integration branch is never checked out; trial merges and promotion use the runbook journal/CAS protocol.
