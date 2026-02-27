# Tree-Sitter + RepoMap Parity Closure Supplement

## Purpose

This document is a **supplement** to `docs/PLAN-treesitter-repomap.md`.
It defines the remaining work needed to reach **100% implementation and parity closure** for Gate A (Aider) and Gate B (Volt), based on currently observed gaps.

Use this as an execution playbook: complete each step in order, and do not skip gate checks.

## Mandatory Execution Constraints

1. Preserve all constraints from `docs/PLAN-treesitter-repomap.md` (especially upstream-minimal-touch and profile-scoped parity/exceed behavior).
2. Do not weaken parity by silently changing denominators, fixture scope, or required language sets.
3. Any exclusion or protocol change requires:
   - explicit version bump,
   - artifact update,
   - full parity + exceed rerun.

---

## Scope of Outstanding Gaps This Supplement Closes

The following gaps must be closed:

1. **Language runtime mismatch**
   - `languages.json` and vendored queries include more languages than runtime parser activation currently supports.
2. **TreeContext not integrated into production render path**
   - TreeContext renderer exists, but main repo-map render path does not consume it.
3. **Gate A parity checks partially fixture/simulation-driven**
   - Some checks are not tied to real production ranking/comparator execution.
4. **Gate B B3 runtime discovery weakness**
   - Runtime path validation currently derives discovered paths from inventory itself in at least one path.
5. **B1/B5 enforcement not fully equivalent to declared threshold rigor**
   - Not all protocol dimensions are hard-failed in scored runs.
6. **Exceed gate weak enforcement**
   - `test:exceed` exists but needs explicit, non-empty, thresholded exceed test coverage.
7. **Sign-off bundle can report pass without proving same-run gate execution provenance**
   - Bundle and conformance must be tied to concrete gate run outputs.

---

## Non-Negotiable Exit Criteria (Supplement Completion)

This supplement is complete only when all are true:

1. Canonical language parity policy is explicit and enforced: every manifest language is either runtime-activated or explicitly classified as parity-nonruntime via versioned protocol exception, and all vendored queries/manifests/tests are consistent with that policy.
2. Production repo-map render path provides TreeContext-equivalent scope-aware behavior for parity mode (either by using TreeContext directly or by proving comparator-visible output equivalence).
3. Gate A checks (A1-A6) are run against real production outputs and comparator inputs, not fixture-only proxies.
4. Gate B checks (B1-B5) fail closed on runtime-discovered inventory drift and full threshold matrix.
5. `task test:exceed` fails if no exceed tests are executed, and passes only with explicit exceed assertions.
6. Sign-off bundle is generated from **the same scoring run** with immutable evidence links.

---

## Execution Order (Do Not Reorder)

1. **S0** Baseline lock + reproducibility
2. **S1** Runtime language parity closure
3. **S2** Production TreeContext integration
4. **S3** Gate A hardening (A1-A6)
5. **S4** Gate B hardening (B1-B5)
6. **S5** Exceed profile hardening
7. **S6** Atomic sign-off bundle hard binding
8. **S7** Final full sweep + closure record

---

## S0 — Baseline Lock and Reproducibility

### S0.1 Freeze comparator/runtime inputs

**Actions**
1. Freeze and record:
   - comparator SHAs,
   - fixture corpus hashes,
   - tokenizer id/version,
   - active profile object.
2. Persist this freeze in parity artifacts before any code changes.

**Files**
- `internal/repomap/testdata/parity_aider/*.json`
- `internal/lcm/explorer/testdata/parity_volt/*.json`

**Acceptance**
- One reproducibility artifact exists with all tuple members populated and non-placeholder values.

---

## S1 — Runtime Language Parity Closure

### S1.1 Create canonical language parity checker

**Actions**
1. Add/strengthen a canonical comparison test that computes:
   - manifest keys from `languages.json`,
   - runtime-supported parser keys,
   - vendored query keys,
   - alias-normalized canonical sets.
2. Fail if canonical sets diverge.

**Files**
- `internal/treesitter/runtime_activation_test.go` (or new `manifest_runtime_parity_test.go`)
- `internal/treesitter/languages.go`

**Acceptance**
- Test prints canonical diff and fails on any mismatch.

### S1.2 Close grammar activation gaps

**Actions**
1. For each canonical language in manifest, ensure runtime parser activation exists.
2. Add missing grammar module imports and parser switch entries.
3. If a language truly cannot be runtime-activated, handle it only via an explicit protocol/version exception process (documented rationale + artifact/version updates). Do not silently drop it from parity-required scope.

**Files**
- `internal/treesitter/parser.go`
- `go.mod`, `go.sum`
- `internal/treesitter/languages.json`

**Acceptance**
- Canonical manifest/runtime/query parity test passes.

### S1.3 Re-verify tooling integrity

**Actions**
1. Run and fix failures in:
   - `task tsaudit`
   - `task tsaudit:verify`
2. If manifests are updated, regenerate through approved path and re-verify.

**Acceptance**
- `task tsaudit:verify` is green and no unresolved runtime language drift remains.

---

## S2 — Production TreeContext Integration

### S2.1 Integrate TreeContext into active render path

**Actions**
1. Wire TreeContext rendering into the production repo-map render pipeline for parity profile behavior.
2. Preserve stage semantics:
   - stage 0 prelude,
   - stage 1 ranked defs,
   - stage 2/3 filename-only entries.
3. Ensure parity/enhancement profile toggles are explicit and artifact-recorded.

**Files**
- `internal/repomap/repomap.go`
- `internal/repomap/render.go` and/or `internal/repomap/budget.go`
- `internal/repomap/treecontext.go`

**Acceptance**
- Production map output path exercises TreeContext in parity mode.
- Existing stage invariants still pass.

### S2.2 Add production-path tests (not only unit tests)

**Actions**
1. Add tests proving TreeContext is used from service generation path, not only direct function tests.
2. Verify output contracts under parity and enhancement profiles.

**Files**
- `internal/repomap/parity_test.go`
- `internal/repomap/vertical_slice_test.go`

**Acceptance**
- Failing test if TreeContext integration is removed from production path.

---

## S3 — Gate A Hardening (A1–A6)

### S3.1 Replace fixture-only A1 with real production ranking comparison

**Actions**
1. Compute Crush ranking from actual production ranking pipeline (`extract -> graph -> rank`).
2. Compare against pinned comparator outputs/artifacts for same fixture repo and profile.
3. Keep thresholds as declared by plan.

**Files**
- `internal/repomap/parity_test.go`
- `internal/repomap/parity_fixtures.go`
- (optional helper) `internal/repomap/comparator_runner.go`

**Acceptance**
- A1 fails if production ranking deviates below threshold; no fixture-to-fixture shortcut path remains.

### S3.2 Enforce tokenizer-backed parity counting for scored parity runs

**Actions**
1. Ensure parity-mode score path requires tokenizer-backed parity count.
2. If tokenizer unavailable, parity run is invalid (hard fail), not heuristic pass.

**Files**
- `internal/repomap/tokens.go`
- `internal/repomap/budget.go`
- `internal/repomap/parity_provenance.go`

**Acceptance**
- A3 cannot pass with fake/synthetic parity counter in scored run mode.

### S3.3 Implement explicit comparator normalization contract

**Actions**
1. Centralize comparator normalization function used by parity gates:
   - path separator normalization,
   - line ending normalization,
   - stage-3 ordering normalization only.
2. Use same normalization in all Gate A parity assertions.

**Files**
- `internal/repomap/metrics.go` (or new `normalization.go`)
- `internal/repomap/parity_test.go`

**Acceptance**
- Normalization behavior is test-covered and artifact-documented.

### S3.4 Bind conformance to actual gate execution

**Actions**
1. `GateAPassed` must derive from actual A1-A6 execution results from current run.
2. Remove any unconditional/assumed pass assignment in snapshot build path.

**Files**
- `internal/repomap/conformance.go`
- `internal/repomap/conformance_bundle.go`

**Acceptance**
- Snapshot/bundle generation fails if current-run gate evidence is missing.

---

## S4 — Gate B Hardening (B1–B5)

### S4.1 B1 full metric matrix enforcement

**Actions**
1. Enforce all declared B1 metrics as hard thresholds in scored parity/exceed runs:
   - symbol recall/precision,
   - import category accuracy,
   - visibility accuracy,
   - micro + macro + per-language floors,
   - denominator/capability handling.
2. Ensure fixture corpus breadth is adequate for per-language claims.

**Files**
- `internal/lcm/explorer/parity_test.go`
- `internal/lcm/explorer/protocol_artifacts.go`
- `internal/lcm/explorer/testdata/parity_volt/b1_scoring_protocol.v1.json`

**Acceptance**
- Any single threshold miss fails B1.

### S4.2 B3 runtime discovery must be truly runtime-derived

**Actions**
1. Replace inventory-to-inventory projection checks with:
   - static registration export +
   - runtime instrumentation discovery.
2. Diff merged discovered paths against versioned inventory; fail closed on drift.
3. Keep success-path and explicit failure-path assertions per path id.

**Files**
- `internal/lcm/explorer/runtime_inventory.go`
- `internal/lcm/explorer/parity_test.go`
- `internal/lcm/explorer/testdata/parity_volt/runtime_ingestion_paths.v1.json`

**Acceptance**
- B3 fails if discovered != inventory.

### S4.3 B5 deterministic end-to-end behavior strengthening

**Actions**
1. Add end-to-end scored runs that execute real ingest + retrieval paths under parity profile.
2. Confirm deterministic mode toggles and disallow LLM/agent tier leakage.

**Files**
- `internal/lcm/explorer/parity_test.go`
- `internal/lcm/message_decorator_test.go`
- `internal/agent/tools/lcm_describe.go` and corresponding package-level tests that exercise describe-path persistence/retrieval semantics
- `internal/agent/tools/lcm_expand.go` and corresponding package-level tests that exercise expand-path authorization/retrieval semantics

**Acceptance**
- B5 fails if deterministic guarantees are violated or retrieval semantics drift.

---

## S5 — Exceed Profile Hardening

### S5.1 Make exceed tests explicit and mandatory

**Actions**
1. Add concrete exceed-profile suites for repomap + explorer paths.
2. Ensure `task test:exceed` cannot silently pass with zero matching tests.
   - Add pre-check or post-check that at least one exceed-profile test executed.

**Files**
- `internal/repomap/*_test.go`
- `internal/lcm/explorer/*_test.go`
- `Taskfile.yaml`

**Acceptance**
- `task test:exceed` fails if zero Exceed tests are run.
- Exceed thresholds are hard-gated.

---

## S6 — Atomic Sign-Off Bundle Hard Binding

### S6.1 Bundle must be same-run evidence only

**Actions**
1. Require bundle to include direct references to gate outputs from the same run id/timestamp/hash.
2. Validate:
   - A1-A6 outcomes,
   - B1-B5 outcomes,
   - profile object,
   - comparator tuple,
   - fixture/inventory/matrix hashes,
   - protocol versions.
3. Reject stale artifacts and placeholder values.

**Files**
- `internal/repomap/conformance_bundle.go`
- `internal/repomap/parity_provenance.go`
- `internal/lcm/explorer/parity_provenance.go`

**Acceptance**
- Bundle generation fails when any component is stale/missing/mismatched.

---

## S7 — Final Verification Sweep and Closure Record

Run this exact sequence after all implementation steps:

1. `task fmt`
2. `task lint:fix`
3. `task lint`
4. `task test`
5. `task tsaudit:verify`
6. `task test:parity`
7. `task test:exceed`
8. `task parity:bundle`

Then produce closure artifact summary containing:
- commit SHA,
- command outputs,
- A1-A6 and B1-B5 pass/fail table,
- tokenizer/comparator tuple,
- manifest/query/runtime language parity evidence,
- inventory drift result,
- exceed test execution count.

---

## Implementation Checklist (Operator-Friendly)

Status legend: [x] fully implemented and verified in code/tests; [ ] still outstanding or only partially implemented.

- [x] S0 reproducibility artifact created and validated
- [x] S1 canonical language parity test added and passing
- [x] S1 runtime parser activation closure policy enforced via explicit exceptions artifact
- [ ] S2 TreeContext integrated into production render path
- [ ] S2 production-path integration tests added
- [ ] S3 A1 switched to real production ranking comparison
- [ ] S3 tokenizer-backed parity counting required in scored runs
- [ ] S3 comparator normalization centralized and enforced
- [x] S3 GateA pass derives from actual gate run outputs
- [ ] S4 B1 full metric matrix hard-gated
- [ ] S4 B2 progressive disclosure behavior is explicitly hard-gated
- [x] S4 B3 real runtime discovery + drift fail-closed
- [ ] S4 B4 data-format coverage/depth thresholds are explicitly hard-gated
- [ ] S4 B5 deterministic end-to-end checks strengthened
- [x] S5 Exceed tests added and `task test:exceed` hardened against zero-match pass
- [x] S6 sign-off bundle bound to same-run immutable evidence (run_id + command + output_sha256)
- [ ] S7 full sweep run and closure report generated

---

## Notes

- This supplement does not replace the original plan; it closes verified deltas.
- If any step requires changing protocol schema or artifact versioning, bump version explicitly and rerun full parity/exceed cycles.
- Do not mark parity complete until all checklist items above are checked and verified from command outputs.
