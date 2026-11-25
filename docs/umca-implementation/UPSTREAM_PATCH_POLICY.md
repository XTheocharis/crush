# Upstream footprint, delimiter and sync policy

Mandatory for all units, shared wiring, tests, repairs, and integration. The goal is to make future upstream merges small and intelligible without weakening UMCA correctness. Read the Go projection `implementation/04-architecture-and-apis.md`, `policy/architecture-and-module-layout.md`, `policy/generated-code.md`, `policy/documentation.md`, `policy/validation.md`, decision-existing-repository, decision-module-boundary, decision-package-boundaries, decision-type-abstraction, decision-generators and decision-modernization in full.

## Establish a clean comparison base

At U00 record `UMCA_SPEC_BASE=0b0c79e32ada5c1e5ba26180023be740f0530f7b` and `UMCA_BASE=308d108d50b0f55bea8ad246180dc958a7dde692` (or explicit accepted replacement). Keep the inherited fork changes intact. `git ls-tree -r --name-only "$UMCA_BASE"` is the inherited path inventory for this run; `git diff --name-status "$UMCA_SPEC_BASE" "$UMCA_BASE"` is existing drift, NOT this implementation's footprint. Do not tag inherited unrelated changes as UMCA. Record new file additions separately; files originally absent in the base are UMCA-owned additions even after later work units edit them. During sync update the reference with an explicit decision, never rebase the footprint silently.

Before editing, propose a file-level patch budget: each inherited file path, upstream symbol/boundary, why a change is necessary, new helper file/package considered, expected marker IDs, planned owner, and validation. The owner and orchestrator approve changes to shared files. No hard arbitrary line-count cap: target the fewest necessary inherited files, regions and changed lines. Compare actual to proposed footprint and justify any expansion; a smaller patch is not preferable if it bypasses required boundaries or violates Go contracts.

## Placement order

1. Prefer new files in existing packages (`umca_projection.go`, `umca_capture.go`, `umca_rewind_test.go`, etc. where consistent with project names). Same package can extend methods and use package internals without broad exports or import changes. Respect OS suffixes/build tags; avoid exposing new globals or init side effects.
2. Use a cohesive new `internal/` package for genuinely separate ownership, pure computation, provider/storage adapter or test boundary, staying in the existing Go module. Separate Go modules need an actual independent release/minimum/dependency reason plus orchestrator approval and consumer tests; do not split for cosmetic diff stats.
3. Modify inherited files only for composition/lifecycle, hook invocation, field/interface/registration, unavoidable correctness fix, schema/input or dependency wiring. Preserve native code paths, disabled behavior and adjacent upstream style. Put long helpers, tests, docs and optional logic in new files. Use narrow explicit functions/consumer-owned interfaces, not a giant hook table, duplicate core, copy of upstream files, `go:linkname`, reflection/unsafe, global monkeypatch, new executor, or broad refactor to dodge a one-line hook.
4. Avoid formatting, moving, renaming or deleting inherited files or unrelated cleanup. Only format actual edited Go files and review incidental formatting; use new test files rather than rewriting upstream fixtures unless the boundary requires it. Shared migrations/SQL, schemas and go.mod/go.sum remain with owners; prefer additive migration and query files when sqlc includes them. Verify generator naming/aggregation before changing generator config.

## Exact inline delimiter convention

All implementation-introduced changes in inherited comment-capable files must be enclosed in paired, standalone comments at legal syntax locations. Use stable human-readable IDs `<owner>.<boundary>[.<purpose>]` (ASCII alphanumerics, underscore, dot, hyphen; unique within a file, not recycled for another purpose). Place comments immediately around the smallest coherent changed region, not a whole function or file to hide unrelated changes. Preserve native code outside. Opening/closing tokens are exact and case sensitive; repeat same ID. Period is part of comment syntax, not the ID. Examples (illustrative, not API signatures):

```go
// UMCA-BEGIN: HST.dispatch.admission.
if err := a.admitUMCA(ctx, projection); err != nil {
    return err
}
// UMCA-END: HST.dispatch.admission.
```

```go
import (
    "context"

    // UMCA-BEGIN: HST.imports.projection.
    "github.com/charmbracelet/crush/internal/umca"
    // UMCA-END: HST.imports.projection.
)
```

SQL: `-- UMCA-BEGIN: STO.query.source.` and `-- UMCA-END: STO.query.source.`; place outside sqlc name annotations and check generator behavior. Shell/YAML: `# UMCA-BEGIN: OPS.config.setting.` / `# UMCA-END: OPS.config.setting.`. Markdown/HTML: `<!-- UMCA-BEGIN: OPS.docs.context. -->` / `<!-- UMCA-END: OPS.docs.context. -->` where comments are safe. Do not introduce invalid syntax, split a doc comment from its declaration, alter build tag/Go directive placement, break `go:embed`/generator annotations, or inject comments into provider prompt text or data. Where possible wrap the whole attached directive/declaration without separating them. For deleted/replaced original code, keep a surviving paired region containing the replacement or an explicit zero-content marker pair at the former anchor; document the old behavior and reason in ledger. No orphan, crossed, nested or broad catch-all markers; no marker-only churn. Do not mark whole new files; they are recorded as UMCA-owned paths. Add an ordinary explanatory comment only when helpful for maintainers; tags are diff anchors, not runtime guards.

## Exceptions: generated, commentless, binary and syntax-sensitive changes

A literal inline marker is not legal in strict JSON (`schema.json`), `go.sum` (canonical), many machine-owned outputs, generated Go/SQL, binary fixtures, and some wire/directive-sensitive areas. For go.mod prefer paired comments only if the selected tool preserves them across canonical generation/tidy; otherwise record an exception. Never hand-edit generated files or change JSON format simply to add a comment. Mark editable authoritative inputs where legal, regenerate using pinned canonical commands and assert stable output. For every unmarkable **specific hunk** create an external record in `$UMCA_STATE/patch-exceptions.json` (accepted integration manifest; use a SHA-bound candidate copy for parallel branches) with path, stable exception ID, category, reason, owner, independent reviewer, authoritative marked input(s) (or dependency decision), exact base/head file SHA-256, exact diff SHA-256 and uncovered hunk coordinates (report produced by the audit helper). File deletes, mode/type changes and inherited renames are structural changes, require explicit justification plus exact diff evidence; treat rename as delete + add. No blanket exemption by file extension or directory, no generated exception without verified provenance, no exception for convenience. A mixed comment-capable file may only exempt the specific syntax/formatter-constrained hunks, never all changes. Any changed diff/hash invalidates exception for reapproval. Keep `UMCA-BEGIN: <exception-id>.` / `UMCA-END: <exception-id>.` in a sidecar review document around the exact diff excerpt or link to the hash-bound patch and coordinate list; use `git diff --binary --full-index` for binary evidence and hash both blobs. A marker in a sidecar cannot replace safe inline markers. Exceptions cannot waive required behavior or tests. See auditor instructions; report these separately in final review.

### Sidecar evidence example (illustrative; substitute actual hashes and diff)

```text
UMCA-BEGIN: OPS.schema.generated.
Path: schema.json
Category: generated (strict JSON, no inline comments)
Authority: actual marked config input(s), generator and selected version
Owner: OPS; independent reviewer: AUD:<identity>
Base/head SHA-256: <actual> / <actual>
Patch SHA-256: <actual>; path to exact git patch: <evidence path>
Uncovered hunk coordinates: <actual old/new start/count list>
Why inline markers are invalid; alternative considered; regeneration and tests.
UMCA-END: OPS.schema.generated.
```

The JSON approval record contains `id`, `path`, `kind`, `reason`, `owner`, `reviewer`, `evidence`, `base_sha256`, `head_sha256`, `patch_sha256`, `uncovered_hunks`, `structural`. One path record covers exactly the listed uncovered hunks and structural changes, with individual reason/anchor evidence for each; never blanket-approve the file. Exact helper evidence can seed coordinates/hashes but not approval. Reviewer and author identities must differ; ORCH verifies that in runner state. Save delimited evidence in the maintained hook/exception manifest accompanying release/sync docs, with hash-bound diff available to reviewers.

## Review commands (run from a clean committed candidate)

Use the included read-only Python 3 stdlib helper (outside Crush; no app dependency). It checks the cumulative inherited-file diff relative to UMCA_BASE, not merely the most recent agent diff. Git must be installed. No Python runtime is required in Crush. The helper isolates ambient Git overrides, disables replacement refs, requires matching comment styles, records Git version, and checks balanced pairs and changed-line/deletion anchors, exact exception matching and footprint counts; it cannot decide whether an architecture is appropriate, comments are legal in actual grammar, the scope is minimal, generator output is reproducible, or Go behavior is correct. Independent human/agent review and actual tests remain required. No marker injection or source mutation by script.

```sh
# PLAN_DIR is absolute path to extracted package. Run from clean candidate repo.
python3 "$PLAN_DIR/tools/audit_upstream_patch.py" --repo . \
  --base "$UMCA_BASE" --head HEAD \
  --exceptions "$ATTEMPT_DIR/patch-exceptions-candidate.json" \
  --output "$ATTEMPT_DIR/upstream-audit.json"
git diff --check "$UNIT_BASE" HEAD
git diff --stat "$UNIT_BASE" HEAD
git diff --stat "$UMCA_BASE" HEAD
git diff --name-status --no-renames "$UMCA_BASE" HEAD
git diff --no-ext-diff --no-renames "$UNIT_BASE" HEAD
```

Initialize the accepted exception file to `[]` at U00. For concurrent candidate branches, snapshot the accepted manifest at the branch base into `$ATTEMPT_DIR/patch-exceptions-candidate.json`, update only its affected path records and run the helper with that file. Do not use a newer global manifest for an older candidate. During serialized integration, reconcile candidate exceptions with the latest accepted manifest, re-review any changed exact diff/hash, then publish the integrated manifest only after integrated PASS. Preserve prior snapshots; use attempt directories and immutable SHA-bound manifests. Use the accepted-ref promotion journal, never a trial tree or premature manifest replacement. If script fails, read report, correct misplaced/missing markers or obtain an exact independent exception. `proposed_exception` in report is only evidence for review; do not automatically promote it. If any script/tool unavailable, use manual exact zero-context Git hunk and marker audit with equivalent signed-off evidence; tooling unavailable is disclosed, not automatic pass. Reviewer also checks **per-unit** delta for edits outside lease, existing marker region reused too broadly, upstream code touched unnecessarily, baseline prior changes preserved, no swallowed errors/import cycles/compatibility churn, no new unnecessary upstream paths, new-file placement and genuine new functionality. Compare base/head and merge parents; integrated audit required after conflict resolution or dependency changes. Whole-file count, changed lines (+/-), new paths, marker block count, exceptions, cumulative diff and per-unit diff retained; do not use a numerical score as correctness evidence.

## Future upstream sync procedure

Use dedicated clean sync branch/worktree. Record old base, new upstream SHA, accepted UMCA head, native upstream drift and pre-sync manifest. Inventory marker IDs and unmarkable exact exceptions. Preview upstream merge/rebase with three-way conflict review: align upstream changed symbols and semantics first, reapply only the necessary thin hooks, retain marker IDs or record a rename mapping. Move substantive logic into UMCA-owned files, not a copied upstream implementation. If upstream supersedes a hook, remove the obsolete UMCA hook/markers with recorded reason and functional coverage; never blindly strip all marker ranges (replacement/deletion blocks may contain native behavior). Reallocate migration order or regenerate output only under owners, preserve existing deployed migration IDs and approved compatibility/migration plan. Reverify provider/Go/tool versions, selected APIs, config, replay, deletion, rewind and disable paths. Run per-unit/integrated audits against the accepted updated base; reconcile source lineage and exception records; full regression/required enabled and disabled gates before merging. Keep changes local absent explicit remote authority. Preserve old manifests and hashes for review.

The committed diff is not a grammar parser; inspect actual comment syntax and distinguish marker-like literals in raw strings/fixtures. Diff attributes/local Git configuration and Git version can affect exact patch bytes, so record the helper version, Git version and exact patch recipe; re-run on the frozen clean tree and reapprove changed evidence, never force a pass. Treat an untrusted cache or Git directory as out of scope for this helper; use a trusted private worktree/object store.
