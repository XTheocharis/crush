# Tree-Sitter Foundation + Explorer Upgrade + Repo Map

## Implementation Directive

**Upstream sync priority**: This is a fork. Minimize modifications to files that exist on `upstream/main` to reduce merge conflicts during upstream sync. Prefer creating new files and using extension points (functional options, hook patterns, delegation) over inlining logic into existing files. When an existing file must be touched, keep the diff as small as possible — add a field, add a delegation call, nothing more.

**Comparator reference policy (normative)**: external line numbers and broad file pointers are informational only. Normative parity references are comparator commit SHAs plus symbol/behavior contracts recorded in parity artifacts. If line ranges drift but symbol behavior at the pinned comparator commit is unchanged, adjudicate by symbol behavior.
**All `file:line` anchors below are informational and non-normative; behavioral contracts are authoritative.**

**Reference implementations**: Two sibling repositories serve as reference sources:
- **`../aider/`** — Reference for the Repo Map implementation (PageRank ranking, tag extraction, budget fitting, scope-aware rendering via TreeContext, mention extraction, refresh modes, token estimation). Primary in-repo sources are `aider/repomap.py`, `aider/repo.py`, and `aider/coders/base_coder.py`. TreeContext internals come from the external `grep-ast` dependency used by Aider (pin comparator provenance in parity artifacts). Our implementation must meet or exceed Aider's functionality and capability: same algorithm fidelity, same personalization dimensions, same stage-0+1/2/3 output assembly semantics, and same progressive fallback chain.
- **`../volt/`** — Reference for the LCM + Explorer implementation (explorer registry, code explorers, heuristic enrichment, stdlib categorization, output formatting). In the current local snapshot, `../volt/packages/voltcode/src/session/lcm/explore` contains 36 explorer-related modules (~24,593 lines) extracting deeply categorized data. Our tree-sitter-based explorers must meet or exceed Volt's depth of analysis: comparable symbol extraction, import categorization, visibility inference, and progressive-disclosure formatting.

**Goal**: Replace Crush's current 20 lightweight regex/go-ast explorers with tree-sitter-based equivalents that match Volt's analytical depth, and implement Aider's repo map algorithm with full fidelity — while keeping the diff to upstream-tracked files as small as possible.

**Observational-count policy**: line/module/file counts and inventory tallies outside the explicit quantitative gate sections are contextual only and are not pass/fail criteria.

## Non-Negotiable Outcome Gates (Definition of Done)

This plan is only considered complete when **all** gates below pass. "Mostly works" is not sufficient.

### Gate A — Repo Map meets/exceeds Aider

1. **Algorithm parity**: personalization, weighted graph construction, PageRank, stage-0+1/2/3 assembly, progressive fallback chain, and refresh modes must match Aider semantics in `parity_mode=true`.
   - In parity mode, fallback attempts #2 and #3 must use `chat_files = empty` and `other_files = all_abs_files` (Aider disjoint/global and unhinted sequence semantics).
2. **Budget safety (profile-scoped)**:
   - `parity_mode=true`: budget-fit acceptance must follow Aider comparator semantics and be scored with `parityTokenCount`.
   - `parity_mode=false`: hard safety must hold with `safetyTokenCount <= TokenBudget`.
3. **Output fidelity**: scope-aware rendering and stage ordering must preserve the same prioritization behavior as Aider while allowing deterministic ordering improvements in enhancement profile.
4. **Parity tests required**: fixture-based tests comparing ranking/selection/render behavior against `../aider/` reference outputs for representative repos.

### Gate B — LCM + Explorer meets/exceeds Volt

1. **Explorer depth parity baseline**: tree-sitter + heuristics must provide Volt-comparable symbol/import/visibility metadata depth and progressive disclosure behavior in `parity_mode=true`.
2. **Runtime integration parity**: explorer summaries must be wired into the actual LCM large-content path (not just standalone explorer package tests).
3. **Data coverage split**:
   - **Parity baseline**: match Volt-observed format coverage/fields.
   - **Exceed track**: additional semantic extraction beyond Volt baseline is required in `parity_mode=false`.
4. **Parity tests required**: fixture-based output tests against `../volt/` expectations plus end-to-end LCM tests proving explorer summaries are persisted and retrievable.

### Execution Profiles (Mandatory)

To resolve parity-vs-enhancement ambiguity, every implementation and test run in this plan must declare one of two profiles:

1. **`parity_mode=true` (gate/comparator profile)**
   - Repo-map behavior follows Aider semantics for refresh, ranking assembly, mention extraction source text, and budget-fit acceptance logic.
   - Explorer scoring runs are deterministic static-only (LLM/agent enhancement tiers disabled).
   - Ordering differences intended as improvements are disabled unless explicitly allowed by a parity fixture rule.

2. **`parity_mode=false` (enhancement profile)**
   - Additional behavior that exceeds references is allowed (e.g., stronger safety trimming, deterministic ordering, extra capture handling), but must not be used for parity scoring.

Parity artifacts must record the active profile and the exact toggle set used.

**Profile source-of-truth (normative):**
- Active profile and toggles MUST come from a single runtime/test profile object recorded in artifacts as:
  `profile`, `parity_mode`, `deterministic_mode`, `enhancement_tiers_enabled`, `token_counter_mode`, `comparator_tuple`.
- Any parity run (`parity_mode=true`) with `enhancement_tiers_enabled != none` is an automatic hard failure.
- Any gate assertion produced without a recorded profile object is invalid.

### Quantitative Pass/Fail Thresholds (Mandatory)

All thresholds below are hard gates within their declared profile:
- `parity_mode=true` thresholds are hard blockers for parity sign-off.
- `parity_mode=false` thresholds are hard blockers for exceed certification.

#### Gate A thresholds (Aider parity)

- **A1 — Ranking concordance**: For each parity fixture and each budget profile `{1024, 2048, 4096}` tokens, compare Crush vs Aider top-file rankings (excluding chat files). Require:
  - Jaccard(top-30) `>= 0.85`
  - Spearman rank correlation on shared top-30 `>= 0.80`
  - If eligible files `< 30`, evaluate top-K where `K = eligibleFiles`; compute Spearman only when shared top-K `>= 3`.
  - If shared top-K `< 3`, mark Spearman as N/A for that fixture-profile pair and gate that pair on Jaccard only.
- **A2 — Stage correctness + render fidelity**: In 100% of parity fixtures, assembled output invariants hold:
  - stage 0 (optional): prepended special-file prelude from `special.py` parity set,
  - stage 1 contains ranked definitions only,
  - stage 2 contains remaining graph nodes as bare filename entries (**no trailing colon**),
  - stage 3 contains remaining repo files as bare filename entries (**no trailing colon**),
  - trim priority is stage 3 → stage 2 → stage 1, with stage 0 highest priority by prepend order,
  - parity artifacts include stage-membership traces for pre-fit and post-fit candidate lists.
  - parity checks evaluate selection/trim invariants on the assembled pre-render tag list; final rendered output parity is checked separately against Aider-equivalent render semantics (including sorted tag rendering behavior in `to_tree`).
  - comparator normalization in `parity_mode=true` is: path-separator normalization, line-ending normalization, and stage-3 ordering normalization only. No additional semantic/content normalization is allowed.
- **A3 — Budget safety and comparator accounting**:
  - Keep two counters in parity artifacts:
    - `parityTokenCount`: Aider-comparator token estimate path (Aider-style tokenizer-backed estimate/sampling semantics).
    - `safetyTokenCount`: conservative safety acceptance count used for hard safety (`max(parityTokenCount, ceil(heuristicCount*1.15))`).
  - In `parity_mode=true`, budget-fit acceptance logic matches Aider semantics: candidates may be accepted when within 15% absolute error of target budget, even if slightly above target.
  - In `parity_mode=false`, enforce strict safety: in 100% of runs, `safetyTokenCount <= TokenBudget`.
  - **Tokenizer availability rule (mandatory)**:
    - In `parity_mode=true`, Aider-comparator scoring requires tokenizer-backed `parityTokenCount`.
    - If tokenizer-backed parity counting is unavailable for the active comparator run, mark that parity run invalid (hard failure for parity adjudication), not pass-by-heuristic.
  - **Comparator tuple pin (mandatory for parity adjudication):** parity artifacts must include and pin:
    - `aider_commit_sha`
    - `grep_ast_provenance`
    - `tokenizer_id`
    - `tokenizer_version`
    Missing any tuple element invalidates the parity run.
  - Any hard under-budget assertions (`<= TokenBudget`) are enhancement-profile gates unless explicitly marked as comparator-only checks.
- **A4 — Refresh semantics**: 100% contract-test pass rate for `auto`, `files`, `manual`, `always` modes.
  - `parity_mode=true`: behavior must match Aider semantics for cache-hit/miss and mode behavior (including prompt-caching coercion `auto -> files`, and no extra staleness heuristics).
  - `parity_mode=false`: additional freshness/staleness triggers are allowed as enhancements.
- **A5 — Determinism (profile-scoped)**:
  - `parity_mode=true`: in `files` and `manual` modes, 10 repeated runs on unchanged inputs must produce identical comparator-normalized hashes. Comparator normalization is mandatory: compare stage-3 as an order-insensitive set by canonical lexical sort of normalized rel paths; stage-0/1/2 remain order-sensitive.
  - `parity_mode=false`: in `files` and `manual` modes, 10 repeated runs on unchanged inputs must produce byte-identical raw output hashes, with deterministic ordering enhancements enabled where configured.
- **A6 — Parity profile leakage guard**: in `parity_mode=true`, enhancement-only repo-map toggles are disabled; parity artifacts must record active toggles and fail if enhancement-only behavior is detected.

#### Gate B thresholds (Volt parity)

- **B1 — Symbol/import extraction quality** (on parity fixture corpus, scored by the protocol in Phase 5E):
  - `parity_mode=true`: hard parity thresholds (numeric and frozen for the scoring cycle):
    - global micro averages:
      - symbol recall `>= 0.93`
      - symbol precision `>= 0.93`
      - import category accuracy (stdlib/third_party/local) `>= 0.93`
      - visibility inference accuracy `>= 0.90` (only for languages with visibility capability `full` or `export-only`)
    - global macro averages:
      - symbol recall `>= 0.90`
      - symbol precision `>= 0.90`
      - import category accuracy `>= 0.90`
      - visibility inference accuracy `>= 0.87`
    - per-language floor (for languages with sufficient sample size per protocol):
      - symbol recall `>= 0.85`
      - symbol precision `>= 0.85`
      - import category accuracy `>= 0.85`
      - visibility inference accuracy `>= 0.80`
  - `parity_mode=false`: exceed-track thresholds:
    - global micro averages:
      - symbol recall `>= 0.95`
      - symbol precision `>= 0.95`
      - import category accuracy (stdlib/third_party/local) `>= 0.95`
      - visibility inference accuracy `>= 0.95`
    - global macro averages (same four metrics) `>= 0.93`
    - per-language floor (for languages with sufficient sample size per protocol): each metric `>= 0.90`
- **B2 — Progressive disclosure behavior**:
  - `parity_mode=true`: 100% of formatted outputs obey per-section caps, include an explicit truncation signal when truncated, and never silently truncate. Accept Volt-observed indicator variants by marker class:
    - **Counted markers**:
      - `... and N more`
      - `... and N more <category>` (for example, `... and N more keys`)
      - `(+N more)`
    - **Non-count markers**:
      - plain truncation marker `, ...`
      - bracketed truncation/sample markers observed in dispatcher/summary paths (for example, bracketed sampled/truncated indicators)
    For parity scoring, normalize accepted variants to a canonical comparator marker class before metric comparison, preserving numeric overflow counts when present.
  - `parity_mode=false`: require canonical counted overflow indicator form `"... and N more"` for capped sections (non-count markers are not allowed in enhancement scoring output).
- **B3 — Runtime-path parity and persistence coverage**:
  - Use a versioned runtime-path inventory artifact generated from deterministic path discovery; parity is fail-closed if runtime-discovered in-scope paths differ from inventory.
  - Runtime-path inventory is authoritative only when generated by both:
    1) versioned static registration export, and
    2) deterministic runtime instrumentation trace.
    The merged result is versioned at `internal/lcm/explorer/testdata/parity_volt/runtime_ingestion_paths.v1.json`.
    (Artifact filename is retained for compatibility, but inventory scope includes both ingestion and retrieval paths.)
  - For every inventoried path, include success-path and explicit failure-path assertions.
  - `parity_mode=true`: success assertions must match Volt-faithful per-path behavior matrix (`persists_exploration=true|false`). Paths expected to persist must assert non-null `exploration_summary` and `explorer_used`; paths expected not to persist must assert those fields remain null while request flow succeeds.
  - `parity_mode=false`: all inventoried success paths must persist non-null `exploration_summary` and `explorer_used`.
  - Include config-gating assertions per applicable path (`DisableLargeToolOutput`, `LargeToolOutputTokenThreshold`).
  - Canonical inventory artifact path: `internal/lcm/explorer/testdata/parity_volt/runtime_ingestion_paths.v1.json`.
  - `path_kind` is mandatory per entry: `ingestion` or `retrieval`.
  - Inventory schema is mandatory:
    - top-level: `version`, `generated_at`, `discovery_method`, `profile`, `paths[]`
    - each path entry: `id`, `path_kind`, `entrypoint`, `trigger`, `in_scope`, `persists_exploration_parity`, `persists_exploration_enhanced`, `config_gates[]`
  - Deterministic discovery method is mandatory: generate inventory from a versioned static registration table plus deterministic runtime instrumentation in parity tests; fail if either source produces entries not present in the artifact.
  - **Concrete in-scope path IDs are mandatory in the inventory artifact (v1):**
    - `lcm.tool_output.create` — Crush `message_decorator.Create` large tool-output interception path.
    - `lcm.describe.readback` — `lcm_describe` retrieval-path assertions for persisted and non-persisted exploration states.
    - `lcm.expand.readback` — `lcm_expand` retrieval-path and authorization assertions.
  - **Volt comparator path matrix anchoring (mandatory for parity fixtures):** `runtime_ingestion_paths.v1.json` must carry comparator-anchored path expectations for the current scoring cycle, including at minimum:
    - `volt.prompt.file.persist` — `session/prompt.ts` `insertLargeFileFromPath + updateLargeFileExploration` behavior class.
    - `volt.prompt.user_text.nonpersist` — `session/prompt.ts` large-user-text path with summary returned to caller but no DB exploration-field update.
    - `volt.tool.large_output.nonpersist` — `session/large-tool-output.ts` insert-only path (no exploration-field persistence in this path).
    - `volt.tool.read.nonpersist` — `tool/read.ts` path that explores for response output but does not persist exploration fields.
    - `volt.map_shared.persist` — `tool/map-shared.ts` path that persists exploration fields.
    The artifact stores behavior class and provenance pointers only (no cross-repo runtime execution dependency).
  - Any newly introduced in-scope runtime ingestion/retrieval path requires a protocol/version bump (`runtime_ingestion_paths.v{N+1}.json`) and full parity rerun before sign-off.
  - **Per-path retrieval assertions (mandatory):** for each path outcome, validate `lcm_describe` behavior for both persisted and non-persisted states.
  - **Retrieval-scope parity assertions (mandatory):**
    - `lcm_describe` (scoped-context path): self-session allowed, ancestor-session allowed, unrelated-session denied.
    - `lcm_expand`: `parity_mode=true` must use Volt-strict authorization semantics — lineage scope (`self + ancestors`) **and** sub-agent-only caller requirement. `parity_mode=false` may relax/extend agent-type gates as enhancement behavior, but active policy must be recorded in artifacts.
  - **Parity retrieval-mode caveat (mandatory):** if comparator behavior includes missing-conversation fallback or scoped-query field omissions (for example, unscoped lookup paths when conversation context is absent), parity fixtures must model that behavior explicitly and prevent false pass/fail caused by query-shape differences rather than true persistence/authorization state.
- **B4 — Data-format coverage and depth**:
  - `parity_mode=true`: Markdown, LaTeX, SQLite, Logs, and existing JSON/YAML/CSV/XML/HTML/TOML/INI explorers must pass golden tests and meet all of:
    - required-field coverage: `100%` for each format's required field set,
    - entity extraction quality: per-format micro F1 `>= 0.90` and macro F1 `>= 0.86`,
    - numeric/count fields (when present): MAPE `<= 0.10`.
  - `parity_mode=false`: same formats must meet all of:
    - required-field coverage: `100%`,
    - entity extraction quality: per-format micro F1 `>= 0.94` and macro F1 `>= 0.90`,
    - numeric/count fields (when present): MAPE `<= 0.05`.
- **B5 — End-to-end parity gate**: parity suite must pass end-to-end in deterministic scoring mode (LLM/agent tiers disabled for scoring runs); any threshold miss is a hard failure. This includes the scored explorer-family parity/exclusion matrix required in Phase 5E.

---

## Context

Crush currently has 20 lightweight explorers (~3,500 lines) using regex/go-ast and format-specific parsing that extract limited structural information compared to Volt (names/basic signatures for most code explorers). Note: the explorer system is pre-built infrastructure not yet wired into the LCM compaction pipeline — no **production** code outside `internal/lcm/explorer/` imports it today. In the current local Volt snapshot, `session/lcm/explore` contains 36 explorer-related modules (~24,593 lines) extracting deeply categorized data. Crush also has zero proactive codebase context — the agent starts every conversation blind, relying on tool calls to discover the repo.

This plan centralizes parsing on tree-sitter via `github.com/tree-sitter/go-tree-sitter` (CGO), upgrades all explorers to provide comparable coverage to Volt, and implements Aider's repo map algorithm (PageRank-ranked, token-budget-aware code outlines injected into every prompt). By sharing one tree-sitter parse cache across explorers and repo map, we avoid duplicate work and get uniform language coverage. Query files (`*-tags.scm`) are vendored from Aider query directories; grammar-bundled `queries/tags.scm` are tracked for drift/audit only. Grammar module versions are pinned to Aider's `tree-sitter-language-pack` v0.13.0 via `language_definitions.json`. The script `scripts/gen-treesitter-deps.sh` automates discovery and candidate dependency generation; authoritative pin validation is owned by `tsaudit` + manual exception manifest data (including unresolved-language exceptions like QL).

**Key decisions:**
- **Full CGO-only (local/dev scope)** — this fork requires `CGO_ENABLED=1` for tree-sitter functionality. No graceful degradation inside the tree-sitter/repo-map feature path. CGO is needed only for tree-sitter — both SQLite drivers (`modernc.org/sqlite` and `ncruces/go-sqlite3`) are pure Go and unaffected. Release/distribution matrix changes are out of scope for this document.
- **Build profile alignment requirement** — because tree-sitter/repo-map are default-enabled in this fork, `task build` and all gate tasks (`task test`, `task test:parity`, `task test:exceed`) must run with `CGO_ENABLED=1`. Any `build:nocgo`/`test:nocgo` compatibility tasks are non-signoff targets only; parity/exceed adjudication is invalid in a no-CGO profile.
- **Default enabled** — repo map is on by default. Users can disable via `options.repo_map.disabled: true` in config.

---

## Definitive Language Support

### Discovery Methodology

Language support is determined by `scripts/gen-treesitter-deps.sh` plus documented manual exception pins, which intersect three data sources:

1. **Aider's `*-tags.scm` query files** — from two directories in the [Aider repository](https://github.com/Aider-AI/aider), matching the priority order in Aider's `get_scm_fname()` (`repomap.py:805-829`):
   - **Primary**: `aider/queries/tree-sitter-language-pack/` — curated by Aider (30 languages; 28 with Go bindings). These provide modern `@name.definition.*` and `@name.reference.*` captures used for parity def/ref extraction. Battle-tested with Aider's repo map algorithm.
   - **Fallback**: `aider/queries/tree-sitter-languages/` — secondary Aider query source. The primary directory takes precedence when both have a `tags.scm` for the same language. The 11 unique languages are TypeScript, PHP, Kotlin, Haskell, Scala, Fortran, HCL, Julia, Zig, C# (as `c_sharp`), and QL. QL is not in `tree-sitter-language-pack`; it is handled as a documented manual exception pin (see fallback table below).
2. **Go grammar bindings** — the grammar repository must contain `bindings/go/` (required for `go get` integration). Grammar module versions are pinned to Aider's `tree-sitter-language-pack` v0.13.0 `language_definitions.json`. The script resolves modules from `language_definitions.json` first, then falls back to pip package metadata for languages like C# that are transitive dependencies. For multi-grammar repos (e.g. `tree-sitter-typescript` with `typescript/` and `tsx/` subdirectories), Go bindings are checked in both the subdirectory and repo root.

A language qualifies when it has (a) an Aider `*-tags.scm` file from either query directory AND (b) Go grammar bindings at the pinned revision. No dependency-only grammars or query inheritance chains are needed — each `tags.scm` file is self-contained.

### Capture Convention

`tags.scm` files follow two capture styles that must both be supported:

1. **Modern paired-name style** (common):
   - `@name.definition.<type>` — name text node for definition
   - `@name.reference.<type>` — name text node for reference
2. **Legacy paired-node style** (present in some languages, e.g. OCaml interface):
   - `@name` with sibling `@definition.<type>`
   - `@name` with sibling `@reference.<type>`

**Parity note**: Aider's current extractor emits tags only from `@name.definition.*` and `@name.reference.*` captures. Legacy paired-node handling in Crush is treated as an enhancement path (`exceed` behavior) and must not alter strict parity-mode results.

Additional captures:
- `@definition.<type>` — full definition node
- `@reference.<type>` — full reference node
- `@doc` — associated documentation comment

Where `<type>` includes: `function`, `method`, `class`, `interface`, `type`, `module`, `constant`, `variable`, `call`, `implementation`, `macro`, `enum`, `union`, `constructor`, `property`, `alias`, `slot`, `mixin`, `label`, and others.

**Parser contract**: query extraction must handle both modern and legacy styles. In `parity_mode=true`, emitted parity outputs must remain modern-capture-equivalent; legacy paired-node handling is enhancement-only. `tsaudit:verify` fails if a vendored query cannot be interpreted under this dual-style contract.

### Supported Languages (39)

Raw Aider query union is 41 language names (30 primary + 26 fallback − 15 overlap). Target support is 39 languages (37 unique Go modules — ocaml/ocaml_interface and csharp/c_sharp share modules). The excluded raw-union languages are `clojure` and `pony` (no Go bindings at pinned revisions). Rollout is incremental (Phase 1A bootstrap, then Phase 1B expansion), and parity gates are evaluated on the full target set. Modules and revisions are from `tree-sitter-language-pack` v0.13.0 `language_definitions.json` unless noted. The set comes from `scripts/gen-treesitter-deps.sh` output plus documented manual exception pins.

**Automation split (mandatory):** dependency commands are maintained in two explicit groups for auditability:
- **Auto-generated set**: languages resolved by script/manifest pipeline.
- **Manual-exception set**: languages intentionally pinned outside automated resolution (currently includes QL).
Both groups must be emitted in `tsaudit:update` output and preserved distinctly in plan artifacts.

**Primary source** — `tree-sitter-language-pack/` (curated by Aider, 28 languages):

| Language        | Go Grammar Module                                        | tags.scm | Notes                          |
|-----------------|----------------------------------------------------------|----------|--------------------------------|
| arduino         | `github.com/ObserverOfTime/tree-sitter-arduino`          | 5 lines  |                                |
| c               | `github.com/tree-sitter/tree-sitter-c`                   | 9 lines  | structs + functions + typedefs |
| chatito         | `github.com/tree-sitter-grammars/tree-sitter-chatito`    | 16 lines | NLU training data              |
| commonlisp      | `github.com/theHamsta/tree-sitter-commonlisp`            | 122 lines| most complex tags.scm          |
| cpp             | `github.com/tree-sitter/tree-sitter-cpp`                 | 15 lines | extends C tags with classes + namespaces |
| csharp          | `github.com/tree-sitter/tree-sitter-c-sharp`             | 26 lines | resolved via pip fallback (not in `language_definitions.json`) |
| d               | `github.com/gdamore/tree-sitter-d`                       | 26 lines |                                |
| dart            | `github.com/UserNobody14/tree-sitter-dart`               | 92 lines | rich class/mixin/extension captures |
| elisp           | `github.com/Wilfred/tree-sitter-elisp`                   | 5 lines  |                                |
| elixir          | `github.com/elixir-lang/tree-sitter-elixir`              | 54 lines | modules + functions + macros   |
| elm             | `github.com/razzeee/tree-sitter-elm`                     | 19 lines | functions + types              |
| gleam           | `github.com/gleam-lang/tree-sitter-gleam`                | 41 lines |                                |
| go              | `github.com/tree-sitter/tree-sitter-go`                  | 42 lines | 12 capture types               |
| java            | `github.com/tree-sitter/tree-sitter-java`                | 20 lines | classes + methods + interfaces |
| javascript      | `github.com/tree-sitter/tree-sitter-javascript`          | 88 lines | rich class/method/call captures|
| lua             | `github.com/MunifTanjim/tree-sitter-lua`                 | 34 lines | functions + local assignments  |
| matlab          | `github.com/acristoffers/tree-sitter-matlab`             | 10 lines |                                |
| ocaml           | `github.com/tree-sitter/tree-sitter-ocaml`               | 115 lines| rich module/type/value captures|
| ocaml_interface | `github.com/tree-sitter/tree-sitter-ocaml`               | 98 lines | shares module with ocaml       |
| properties      | `github.com/tree-sitter-grammars/tree-sitter-properties` | 5 lines  | Java .properties files         |
| python          | `github.com/tree-sitter/tree-sitter-python`              | 14 lines | defs + calls                   |
| r               | `github.com/r-lib/tree-sitter-r`                         | 21 lines | functions + assignments        |
| racket          | `github.com/6cdh/tree-sitter-racket`                     | 12 lines |                                |
| ruby            | `github.com/tree-sitter/tree-sitter-ruby`                | 64 lines | methods + aliases + modules    |
| rust            | `github.com/tree-sitter/tree-sitter-rust`                | 60 lines | ADTs + traits + impls          |
| solidity        | `github.com/JoranHonig/tree-sitter-solidity`             | 43 lines | contracts + events + modifiers |
| swift           | `github.com/alex-pinkus/tree-sitter-swift`               | 51 lines | classes + protocols + extensions|
| udev            | `github.com/tree-sitter-grammars/tree-sitter-udev`       | 20 lines | udev rules                     |

**Fallback source** — `tree-sitter-languages/` (from grammar repos' bundled `queries/tags.scm`, 11 entries yielding 10 additional languages plus the `c_sharp` naming-variant entry):

| Language        | Go Grammar Module                                        | Notes                          |
|-----------------|----------------------------------------------------------|--------------------------------|
| c_sharp         | `github.com/tree-sitter/tree-sitter-c-sharp`             | Same module as `csharp` above; separate tags.scm with `c_sharp` naming |
| fortran         | `github.com/stadelmanma/tree-sitter-fortran`             |                                |
| haskell         | `github.com/tree-sitter/tree-sitter-haskell`             |                                |
| hcl             | `github.com/MichaHoffmann/tree-sitter-hcl`               | Terraform uses same grammar (dir: `dialects/terraform`) |
| julia           | `github.com/tree-sitter/tree-sitter-julia`               |                                |
| kotlin          | `github.com/fwcd/tree-sitter-kotlin`                     |                                |
| php             | `github.com/tree-sitter/tree-sitter-php`                 | Multi-grammar repo (dir: `php`) |
| ql              | `github.com/tree-sitter/tree-sitter-ql`                  | Not in `language_definitions.json`; documented manual exception pin at `1fd627a4e8bff8c24c11987474bd33112bead857` (tag `v0.23.1`). Uses Aider's `tree-sitter-languages/ql-tags.scm` (modern `@name.definition.*` convention), not grammar repo's bundled `queries/tags.scm` (old `@name` convention). `scripts/gen-treesitter-deps.sh` currently resolves deferred languages via pip metadata and reports unresolved entries as NOT_FOUND; therefore QL is maintained in explicit manual-exception manifest data consumed by `tsaudit:update` and must not be treated as part of the auto-generated command set. |
| scala           | `github.com/tree-sitter/tree-sitter-scala`               |                                |
| typescript      | `github.com/tree-sitter/tree-sitter-typescript`          | Multi-grammar repo (dir: `typescript`); `.tsx` uses separate `tsx` grammar from same repo |
| zig             | `github.com/maxxnino/tree-sitter-zig`                    |                                |

Note: 15 languages overlap between the two directories (c, cpp, dart, elisp, elixir, elm, go, java, javascript, matlab, ocaml, ocaml_interface, python, ruby, rust). For overlapping languages, the primary `tree-sitter-language-pack/` tags.scm is used. The `c_sharp`/`csharp` pair is a naming variation — both resolve to the same grammar module and the pip fallback handles the name normalization. QL is not in `tree-sitter-language-pack`; its revision is a documented manual exception pin at `1fd627a4e8bff8c24c11987474bd33112bead857` (tag `v0.23.1`).

### Notable Unsupported Languages

These popular languages have no Aider `tags.scm` file in either query directory:

| Language | Notes |
|----------|-------|
| Bash/Shell | Handled by `ShellExplorer` (regex, survives tree-sitter migration). The `bash` grammar and Go bindings exist (`github.com/tree-sitter/tree-sitter-bash`), but Aider has no tree-sitter tags query for bash in either query directory. Bash files produce zero tags and appear only as bare filename entries in Stage 3 output. |

---

## Dynamic Language Support Tracking

### Problem

The set of languages with Go grammar bindings + `tags.scm` query files is a moving target. Aider adds new curated query files, grammar repos add Go bindings or update their bundled tags.scm, and `tree-sitter-language-pack` bumps revisions. Manual tracking is error-prone and will drift.

### Solution: `scripts/gen-treesitter-deps.sh` + `internal/cmd/tsaudit/`

Two complementary tools:

1. **`scripts/gen-treesitter-deps.sh`** (bash) — already implemented. Clones Aider + `tree-sitter-language-pack`, resolves grammar modules + revisions, checks Go bindings, outputs `go get` commands. Used for initial setup and major version bumps.
   - **Current contract note (mandatory):** JSON-resolved grammar binding checks are currently done from shallow default-branch clones (not pinned-revision clones), while pip-fallback/deferred languages are checked at pinned tag revisions. Treat script output as candidate commands; the pinned source-of-truth for implementation is `languages.json` + manual exception manifest generated/validated by `tsaudit`.

2. **`internal/cmd/tsaudit/`** (Go) — a dev tool that:
   1. **Fetches Aider's tags.scm directories** — lists `*-tags.scm` files from both `tree-sitter-language-pack/` (primary) and `tree-sitter-languages/` (fallback) via GitHub API, applying the same priority rules
   2. **Fetches grammar-bundled tags.scm** — checks each grammar repo for `queries/tags.scm` (audit/drift signal only)
   3. **Checks Go binding availability** — verifies `bindings/go/` availability against pinned manifest revisions (including subdirectories for multi-grammar repos). This pinned check is authoritative in `tsaudit`; script-only prechecks are advisory.
   4. **Compares against current support** — diffs against `languages.json` manifest
   5. **Reports drift** — new Aider-curated queries, new grammar-bundled queries, new Go bindings, updated revisions in `tree-sitter-language-pack`

**Vendoring precedence rule (mandatory):** `tsaudit:update` vendors query files only from Aider directories (`tree-sitter-language-pack/` first, `tree-sitter-languages/` fallback). Grammar-repo `queries/tags.scm` are never used as direct replacement sources in parity mode.

### `internal/treesitter/languages.json`

Source-of-truth manifest for supported languages. Generated by `tsaudit` and checked into the repo:

```json
{
  "generated": "2026-02-22T00:00:00Z",
  "language_pack_version": "0.13.0",
  "aider_commit": "7afaa26",
  "languages": [
    {
      "name": "go",
      "grammar_module": "github.com/tree-sitter/tree-sitter-go",
      "grammar_rev": "2346a3ab1bb3857b48b29d779a1ef9799a248cd7",
      "query_source": "pack"
    },
    {
      "name": "typescript",
      "grammar_module": "github.com/tree-sitter/tree-sitter-typescript",
      "grammar_rev": "75b3874edb2dc714fb1fd77a32013d0f8699989f",
      "grammar_dir": "typescript",
      "query_source": "langs"
    }
  ]
}
```

### `internal/cmd/tsaudit/` Commands

```bash
# Check for new/changed language support
task tsaudit

# Update languages.json + vendor new query files
task tsaudit:update

# Verify vendored queries match upstream
task tsaudit:verify
```

`internal/cmd/tsaudit/` command descriptions in this section are examples for local/manual verification only.
### Query Vendoring Structure

```
internal/treesitter/queries/
├── arduino-tags.scm        # pack
├── c-tags.scm              # pack
├── c_sharp-tags.scm        # langs (fallback)
├── chatito-tags.scm        # pack
├── commonlisp-tags.scm     # pack
├── cpp-tags.scm            # pack
├── csharp-tags.scm         # pack
├── d-tags.scm              # pack
├── dart-tags.scm           # pack
├── elisp-tags.scm          # pack
├── elixir-tags.scm         # pack
├── elm-tags.scm            # pack
├── fortran-tags.scm        # langs (fallback)
├── gleam-tags.scm          # pack
├── go-tags.scm             # pack
├── haskell-tags.scm        # langs (fallback)
├── hcl-tags.scm            # langs (fallback)
├── java-tags.scm           # pack
├── javascript-tags.scm     # pack
├── julia-tags.scm          # langs (fallback)
├── kotlin-tags.scm         # langs (fallback)
├── lua-tags.scm            # pack
├── matlab-tags.scm         # pack
├── ocaml-tags.scm          # pack
├── ocaml_interface-tags.scm# pack
├── php-tags.scm            # langs (fallback)
├── properties-tags.scm     # pack
├── python-tags.scm         # pack
├── ql-tags.scm             # langs (fallback) — from Aider (modern @name.definition.* convention)
├── r-tags.scm              # pack
├── racket-tags.scm         # pack
├── ruby-tags.scm           # pack
├── rust-tags.scm           # pack
├── scala-tags.scm          # langs (fallback)
├── solidity-tags.scm       # pack
├── swift-tags.scm          # pack
├── typescript-tags.scm     # langs (fallback)
├── udev-tags.scm           # pack
└── zig-tags.scm            # langs (fallback)
```

Target state: all 39 `*-tags.scm` files are vendored into a single flat directory, embedded via `//go:embed queries/*` and shipped in the binary. No runtime downloads. 28 files come from Aider's primary `tree-sitter-language-pack/` directory; 11 entries come from the fallback `tree-sitter-languages/` directory (10 additional languages plus `c_sharp` naming-variant coverage, only where not already present in the primary). The `tsaudit` tool manages the vendoring process; humans review the diff.

**No `; inherits:` handling needed (current snapshot)**: Aider's `tags.scm` files are self-contained — they do not use nvim-treesitter's `; inherits: <lang>` directive. The query runner simply loads and compiles each `*-tags.scm` file directly with no preprocessing. `tsaudit:verify` must include an explicit drift check that fails if any vendored query introduces `; inherits:` so preprocessing requirements cannot silently change.

---

## Phase 0: Infrastructure Scaffolding

No tree-sitter dependency yet. Sets up config, DB schema, and stub packages.

### 0C: Blocker Burn-Down Gate (Required Before 1A/2A/3A.0)

This gate establishes mandatory scaffolding and one verified runtime persistence bootstrap vertical slice before deeper implementation. It is intentionally narrow. Phase 2D remains required to complete full runtime wiring and inventory-wide parity-path assertions across all v1 in-scope paths.

Current codebase baseline (verified):
- `internal/repomap/` does not exist.
- `internal/treesitter/` does not exist.
- LCM large-output runtime path stores content but does not persist
  `exploration_summary` / `explorer_used` (DB columns/query already exist; missing piece is runtime wiring).

This gate exists to eliminate those three blockers before deeper implementation.
Do not proceed to Phase 1A/2A/3A.0 work until all checks below pass.

**Mandatory pass criteria:**
1. `internal/treesitter/` package exists with compilable stubs for parser/query/cache/types.
2. `internal/repomap/` package exists with compilable service skeleton and GenerateOpts.
3. Runtime exploration persistence bootstrap vertical slice is wired in `internal/lcm/message_decorator.go`:
   - after successful large-output store, explorer runtime executes,
   - `UpdateLcmLargeFileExploration` behavior is profile-scoped:
     - `parity_mode=true`: follow B3 per-path matrix (`persists_exploration=true|false`) for this path,
     - `parity_mode=false`: persist non-empty `exploration_summary` and `explorer_used` on success,
   - failure path remains non-blocking and preserves existing behavior.
4. Blocking proof tests pass:
   - parity profile fixture validates this path against the Volt-faithful persistence matrix,
   - enhancement profile fixture validates non-null `lcm_large_files.exploration_summary`
     and `lcm_large_files.explorer_used`.
5. LCM large-output config gates are enforced in this path:
   - `DisableLargeToolOutput=true` bypasses interception/storage,
   - `LargeToolOutputTokenThreshold` controls interception cutoff (no hardcoded-only behavior in gating path).

Note: 0C is a bootstrap gate for one verified vertical slice only. Full runtime-path parity behavior across all inventoried ingestion paths is enforced by Gate B3 and Phase 5E.

### 0A: Config + DB Migration

**Files to create:**

`internal/config/repomap.go`:
```go
// RepoMapOptions configures the repo map system.
// When non-nil, repo map generation is active.
type RepoMapOptions struct {
    // Disabled turns off repo map generation entirely (default: false = enabled).
    Disabled     bool     `json:"disabled,omitempty" jsonschema:"description=Disable repo map generation entirely"`
    // MaxTokens overrides the dynamic token budget for the rendered map.
    // If zero, uses dynamic formula: min(max(modelContextWindow/8, 1024), 4096).
    MaxTokens    int      `json:"max_tokens,omitempty" jsonschema:"description=Override max token budget for rendered map (0 = dynamic)"`
    // ExcludeGlobs are additional glob patterns to exclude from scanning.
    // Invalid or semantically inert patterns (containing `/` or `**`) are warned at startup
    // via slog.Warn in initRepoMap (see Phase 3A.1). Malformed bracket expressions are also
    // caught. No hard error — matching the codebase convention where shouldIgnore at ls.go:208
    // checks err == nil && matched. The warn-at-startup approach lives in initRepoMap
    // (internal/app/repomap.go), not in the config loading path, keeping it scoped to the
    // feature and avoiding modifications to upstream-tracked load.go.
    ExcludeGlobs []string `json:"exclude_globs,omitempty" jsonschema:"description=Additional glob patterns to exclude from repo map scanning"`
    // RefreshMode controls when the map is regenerated: "auto", "files", "manual", or "always"
    // (default: "auto").
    RefreshMode  string   `json:"refresh_mode,omitempty" jsonschema:"description=When to regenerate the repo map: auto files manual or always"`
    // MapMulNoFiles is the budget multiplier when no files are in chat (default: 2.0).
    // Expands the repo map token budget on the first turn to give the model broader
    // codebase context before files are added to the conversation.
    MapMulNoFiles float64 `json:"map_mul_no_files,omitempty" jsonschema:"description=Budget multiplier when no files are in chat (default 2.0)"`
}

// DefaultRepoMapMaxTokens computes the dynamic token budget based on the model's
// context window size, matching Aider's formula: min(max(contextWindow/8, 1024), 4096).
func DefaultRepoMapMaxTokens(modelContextWindow int) int {
    budget := modelContextWindow / 8
    if budget < 1024 { budget = 1024 }
    if budget > 4096 { budget = 4096 }
    return budget
}

func DefaultRepoMapOptions() RepoMapOptions {
    return RepoMapOptions{
        RefreshMode:   "auto",
        MapMulNoFiles: 2.0,
    }
}
```

**Files to modify (minimized — see Implementation Directive):**

`internal/config/config.go` (~5 lines):
- Add `RepoMap *RepoMapOptions `json:"repo_map,omitempty" jsonschema:"description=Repository map configuration"`` field to the `Options` struct (after the `LCM` field, before the closing `}`) — 1 line.
- Add merge delegation in `Options.merge()` (after the LCM merge block, before `return o`) — 4 lines, matching the `TUI` delegation pattern:
```go
if t.RepoMap != nil {
    if o.RepoMap == nil { o.RepoMap = &RepoMapOptions{} }
    *o.RepoMap = o.RepoMap.merge(*t.RepoMap)
}
```
  The field-level merge rules live on `RepoMapOptions.merge()` in the new `internal/config/repomap.go` file (see above), keeping the diff to `config.go` purely structural. The merge method implements these rules:
  - `Disabled`: OR merge (one-way latch — once any config source disables repo map, no lower-priority source can re-enable it, matching the codebase pattern for boolean flags like `Debug`, `DisableAutoSummarize`)
  - `MaxTokens`: `cmp.Or(t.MaxTokens, o.MaxTokens)` (last-non-zero wins)
  - `ExcludeGlobs`: `sortedCompact(append(o.ExcludeGlobs, t.ExcludeGlobs...))` (append + dedup)
  - `RefreshMode`: `cmp.Or(t.RefreshMode, o.RefreshMode)` (last-non-empty wins)
  - `MapMulNoFiles`: non-zero override (`if t.MapMulNoFiles != 0 { o.MapMulNoFiles = t.MapMulNoFiles }`)

`internal/config/load.go` (~4 lines):
- **Wire defaults** in `setDefaults()` (`load.go:341`), matching the `Attribution` pattern: after the existing `if c.Options.Attribution == nil` block, add:
  ```go
  if c.Options.RepoMap == nil {
      v := DefaultRepoMapOptions()
      c.Options.RepoMap = &v
  }
  ```
  This ensures repo map is enabled by default with a non-nil `*RepoMapOptions`. Users disable via `options: { repo_map: { disabled: true } }`. This differs from `LCMOptions` (where nil = disabled) because the features have different default-on/off semantics.

**Two-level RepoMap configuration:**

RepoMap is configured in two places with slightly different types:
- `Options.RepoMap` (`*RepoMapOptions`) — A pointer that gets defaults in `setDefaults()` when nil
- `Tools.RepoMap` (`RepoMapOptions`) — A value type within the `Tools` struct (defined in `internal/config/config.go`)

Both are mergeable via `RepoMapOptions.merge()` and participate in the full config merge cascade.

  **Why `setDefaults()` and not `newConfig()`**: `newConfig()` is called once per config source in `loadFromBytes()` before JSON unmarshaling. Pre-populating non-zero defaults (like `RefreshMode: "auto"`, `MapMulNoFiles: 2.0`) would cause every config source to start with those values, and json.Unmarshal would leave them intact for config files that never mention `repo_map`. The merge would then treat these as explicitly-set values, incorrectly overriding intentional settings from other config sources. `setDefaults()` runs once after all merges complete, so only truly-unset fields get defaults.

- `internal/config/merge_test.go` — add merge test cases for RepoMapOptions
- `internal/filetracker/service.go` — align path normalization with config working dir (see Phase 4B path-root consistency note).

**New migration** `internal/db/migrations/20260222000000_repo_map.sql`:

Tags and file cache are **not** session-scoped — they are properties of source files, identical
across sessions within the same repo. Because `data_directory` may be shared across repos, repo-map tables must be **repo-scoped** using a stable `repo_key` (derived from canonical working root).
Per-session state includes both PageRank rankings and session read-only rel-path sets.

```sql
-- +goose Up
-- +goose StatementBegin

-- File-level cache: tracks which files have been parsed and their mtime.
-- Used to skip re-parsing unchanged files.
CREATE TABLE IF NOT EXISTS repo_map_file_cache (
    repo_key TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    mtime INTEGER NOT NULL,
    language TEXT NOT NULL DEFAULT '',
    tag_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(repo_key, rel_path)
);

-- Tags: def/ref identifiers extracted from source files.
-- No UNIQUE constraint — same identifier can appear multiple times on the same line
-- (e.g., foo(data, data, data)). Use DELETE + batch INSERT per file instead.
CREATE TABLE IF NOT EXISTS repo_map_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_key TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('def', 'ref')),
    node_type TEXT NOT NULL DEFAULT '',
    line INTEGER NOT NULL,
    language TEXT NOT NULL,
    FOREIGN KEY(repo_key, rel_path) REFERENCES repo_map_file_cache(repo_key, rel_path) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_path ON repo_map_tags(repo_key, rel_path);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_name ON repo_map_tags(repo_key, name);
CREATE INDEX IF NOT EXISTS idx_rmt_repo_kind_name ON repo_map_tags(repo_key, kind, name);

-- Per-session PageRank rankings: which files are most relevant for each conversation.
-- This IS session-scoped because rankings depend on chat files and mentioned identifiers.
CREATE TABLE IF NOT EXISTS repo_map_session_rankings (
    repo_key TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    rank REAL NOT NULL,
    PRIMARY KEY(repo_key, session_id, rel_path)
);
CREATE INDEX IF NOT EXISTS idx_rmsr_repo_session ON repo_map_session_rankings(repo_key, session_id);

-- Session-scoped read-only rel-path set used for Aider parity semantics.
CREATE TABLE IF NOT EXISTS repo_map_session_read_only (
    repo_key TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    rel_path TEXT NOT NULL,
    PRIMARY KEY(repo_key, session_id, rel_path)
);
CREATE INDEX IF NOT EXISTS idx_rmsro_repo_session ON repo_map_session_read_only(repo_key, session_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_rmsro_repo_session;
DROP INDEX IF EXISTS idx_rmsr_repo_session;
DROP INDEX IF EXISTS idx_rmt_repo_kind_name;
DROP INDEX IF EXISTS idx_rmt_repo_name;
DROP INDEX IF EXISTS idx_rmt_repo_path;
DROP TABLE IF EXISTS repo_map_session_read_only;
DROP TABLE IF EXISTS repo_map_session_rankings;
DROP TABLE IF EXISTS repo_map_tags;
DROP TABLE IF EXISTS repo_map_file_cache;
-- +goose StatementEnd
```

Goose wraps each migration in a transaction, and SQLite supports transactional DDL (`CREATE TABLE`, `CREATE INDEX`), so a failure partway through rolls back cleanly with no partial schema state.

**New sqlc queries** `internal/db/sql/repo_map.sql`:

```sql
-- name: UpsertRepoMapFileCache :exec
INSERT INTO repo_map_file_cache (repo_key, rel_path, mtime, language, tag_count)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(repo_key, rel_path) DO UPDATE SET mtime = excluded.mtime, language = excluded.language, tag_count = excluded.tag_count;

-- name: GetRepoMapFileCache :many
SELECT repo_key, rel_path, mtime, language, tag_count FROM repo_map_file_cache WHERE repo_key = ?;

-- name: GetRepoMapFileCacheByPath :one
SELECT repo_key, rel_path, mtime, language, tag_count FROM repo_map_file_cache WHERE repo_key = ? AND rel_path = ?;

-- name: InsertRepoMapTag :exec
INSERT INTO repo_map_tags (repo_key, rel_path, name, kind, node_type, line, language)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: DeleteRepoMapTagsByPath :exec
DELETE FROM repo_map_tags WHERE repo_key = ? AND rel_path = ?;

-- name: ListRepoMapTags :many
SELECT repo_key, rel_path, name, kind, node_type, line, language FROM repo_map_tags WHERE repo_key = ?;

-- name: ListRepoMapDefsByName :many
SELECT repo_key, rel_path, name, node_type, line, language FROM repo_map_tags WHERE repo_key = ? AND kind = 'def' AND name = ?;

-- name: UpsertSessionRanking :exec
INSERT INTO repo_map_session_rankings (repo_key, session_id, rel_path, rank)
VALUES (?, ?, ?, ?)
ON CONFLICT(repo_key, session_id, rel_path) DO UPDATE SET rank = excluded.rank;

-- name: ListSessionRankings :many
SELECT repo_key, session_id, rel_path, rank FROM repo_map_session_rankings WHERE repo_key = ? AND session_id = ? ORDER BY rank DESC;

-- name: DeleteSessionRankings :exec
DELETE FROM repo_map_session_rankings WHERE repo_key = ? AND session_id = ?;

-- name: UpsertSessionReadOnlyPath :exec
INSERT INTO repo_map_session_read_only (repo_key, session_id, rel_path)
VALUES (?, ?, ?)
ON CONFLICT(repo_key, session_id, rel_path) DO NOTHING;

-- name: ListSessionReadOnlyPaths :many
SELECT rel_path FROM repo_map_session_read_only WHERE repo_key = ? AND session_id = ? ORDER BY rel_path;

-- name: DeleteSessionReadOnlyPaths :exec
DELETE FROM repo_map_session_read_only WHERE repo_key = ? AND session_id = ?;

-- name: DeleteRepoMapFileCache :exec
DELETE FROM repo_map_file_cache WHERE repo_key = ? AND rel_path = ?;

```

**Pruning stale cache entries**: No `PruneRepoMapFileCache` SQL query — pruning is implemented in Go. Fetch all cached paths via `GetRepoMapFileCache(repo_key)`, compute the set difference against the current repo file list, and batch-delete stale entries via `DeleteRepoMapFileCache(repo_key, rel_path)`. The `ON DELETE CASCADE` FK on `(repo_key, rel_path)` automatically removes associated tags when a file cache entry is deleted, eliminating the need for separate `DeleteRepoMapTagsByPath` calls during pruning. (The `DeleteRepoMapTagsByPath` query is retained for per-file re-extraction during incremental updates, where the file cache entry is updated rather than deleted.) This avoids `sqlc.slice()` which has known edge-case bugs and is not used elsewhere in the codebase (only `sqlc.arg()` is used).

**Session read-only lifecycle**: `project:map-reset` and session reset flows must call both `DeleteSessionRankings(repo_key, session_id)` and `DeleteSessionReadOnlyPaths(repo_key, session_id)` so personalization/read-only state is cleared atomically.

**Session read-only write-path contract (mandatory):** the service must provide explicit mutation APIs (`SetSessionReadOnlyPaths`, `AddSessionReadOnlyPath`, `RemoveSessionReadOnlyPath`) that write to `repo_map_session_read_only` and are the only source-of-truth for read-only set updates used by mention/addable logic. Parity fixtures must seed/read this table through service APIs only.

Run `task sqlc` to regenerate.

### 0B: Tree-Sitter Types Package

`internal/treesitter/treesitter.go` — public types (no CGO yet):

```go
type Tag struct {
    RelPath  string
    Name     string
    Kind     string // "def" or "ref"
    Line     int
    Language string
    NodeType string // from capture name: "function", "class", "method", "call", "type", etc.
    // NodeType is extracted from the @name.definition.<type> or @name.reference.<type>
    // capture name suffix in the tags.scm query file.
}

// String returns stable diagnostic text used by tests/artifacts.
// Format: "<rel_path>:<line> <kind> <name> [<node_type>]".
func (t Tag) String() string

type SymbolInfo struct {
    Name       string
    Kind       string // "function", "class", "method", "type", "interface", etc.
    Line       int
    EndLine    int
    Params     string
    ReturnType string
    Modifiers  []string // "async", "static", "public", "abstract", etc.
    Decorators []string
    Parent     string   // enclosing class/module name
    DocComment string
}

type FileAnalysis struct {
    Language string
    Tags     []Tag
    Symbols  []SymbolInfo
    Imports  []ImportInfo
}

type ImportInfo struct {
    Path     string
    Names    []string
    Category string // "stdlib", "third_party", "local", "unknown"
}

type Parser interface {
    Analyze(ctx context.Context, path string, content []byte) (*FileAnalysis, error)
    Languages() []string
    SupportsLanguage(lang string) bool
    HasTags(lang string) bool
    Close() error // releases CGO resources (parser pool, query cache)
}
```

**Testing** (`internal/treesitter/treesitter_test.go`):
- `TestTagString()` — verify `Tag.String()` output format
- `TestSymbolInfoFields()` — verify SymbolInfo struct field population
- `TestImportCategoryValues()` — verify ImportInfo category string constants

---

## Phase 1: Tree-Sitter Core

Introduce the CGO dependency and get parsing + querying working.

### 1A: Parser + Cache + Query Runner (Go + Python)

**Dependencies to add:**
- `github.com/tree-sitter/go-tree-sitter`
- `github.com/tree-sitter/tree-sitter-go@2346a3ab1bb3857b48b29d779a1ef9799a248cd7`
- `github.com/tree-sitter/tree-sitter-python@26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64`

(Revisions from `tree-sitter-language-pack` v0.13.0 `language_definitions.json`; authoritative validation is performed by `tsaudit` + manual exception manifest, while `scripts/gen-treesitter-deps.sh` remains advisory for candidate command generation.)

**Files to create:**

`internal/treesitter/parser.go`:
- `parser` struct implementing `Parser`
- Per-language `*tree_sitter.Language` registry (populated from `languages.json`)
- Bounded channel-based parser pool (`cap = runtime.NumCPU()`). A `chan *tree_sitter.Parser` acts as a semaphore.
  - **Deadlock-safe lifecycle contract (mandatory):**
    - `Acquire(ctx)` receives from the channel and returns `(p *tree_sitter.Parser, ok bool)`; cancellation/closed-pool returns `ok=false`.
    - `wg.Add(1)` happens **after successful acquire** (or immediately before returning success), never before a potentially blocking receive.
    - `Release(p)` sends parser back only when pool is open; always pairs with `wg.Done()` for successful acquires.
    - `Close()` first marks pool closed (atomic flag / close-once), then waits for outstanding holders (`wg.Wait()`), then drains and closes remaining parsers.
    - `Analyze()` must propagate context cancellation from `Acquire(ctx)` so shutdown cannot block on new waiters.
  This guarantees deterministic CGO cleanup and avoids blocked-acquire shutdown deadlocks. (`sync.Pool` is unsuitable here — it provides no eviction callback, so pool-evicted parsers would leak C memory silently.)
- `Analyze()` — parse file, run `.scm` queries, return `FileAnalysis`. Each `Analyze()` call must create its own `QueryCursor` (not thread-safe — must not be shared across goroutines) and `Close()` it before returning.
- **Tree-sitter trees are NOT thread-safe** — the C library makes no read vs write distinction. All concurrent access is unsafe. The LRU cache stores "master" trees; consumers receive clones (see cache.go below).

`internal/treesitter/cache.go`:
- In-memory LRU cache keyed by `(filepath, mtime)` → `cacheEntry{tree *tree_sitter.Tree,
  estimatedBytes int64}`
- Use `hashicorp/golang-lru/v2` (already in go.mod as indirect dep; will become direct)
- **Two-level eviction — entry count and byte budget**: entry-count cap (default 5000)
  AND byte-budget cap (default 256 MB). Both are internal constants
  (`defaultTreeCacheEntries = 5000`, `defaultTreeCacheMaxBytes = 256 * 1024 * 1024`) —
  not exposed in `RepoMapOptions` initially. If profiling or user reports surface OOM in
  constrained environments (e.g., containerized environments with ≤512 MB total RAM), add
  `CacheMaxMB int` to `RepoMapOptions` following the existing field pattern.

  **Size estimation**: `estimatedBytes = max(int64(32*1024), int64(len(content))*10)`.
  The 10× multiplier is a conservative lower bound estimated from the AST size ranges
  stated above (50–300 KB for median files against typical source file sizes); the 32 KB
  floor prevents under-accounting for trivially small files. At this estimate, the byte
  cap is the binding constraint for source files larger than ~5 KB (where estimated bytes
  exceed 256 MB / 5000 = 52.4 KB); the entry cap dominates only for sub-5 KB files
  (config stubs, tiny modules). Without the byte cap, 5000 entries × 2 MB each = ~10 GB
  uncapped for large-file repos.

  **Eviction mechanism**: maintain `totalBytes atomic.Int64` on the cache struct.
  After each `Add()`, increment `totalBytes` by `estimatedBytes`, then evict oldest
  entries until under budget:

  ```go
  c.totalBytes.Add(entry.estimatedBytes)
  for c.totalBytes.Load() > c.maxBytes {
      _, _, ok := c.lru.RemoveOldest()
      if !ok {
          break
      }
  }
  ```

  Under concurrent `Add()` calls, two goroutines may both observe
  `c.totalBytes.Load() > maxBytes` and both call `RemoveOldest()`, evicting more entries
  than strictly necessary. This is a benign race — it errs toward lower memory use, not
  data corruption, and resolves within the same eviction pass.

- **Critical**: all CGO objects (`Parser`, `Tree`, `TreeCursor`, `Query`, `QueryCursor`)
  require explicit `Close()` calls.
  - **Ownership rule (mandatory):** in repo-map integration, parser lifecycle is owned by
    `repomap.Service.Close()`, and `App.Shutdown()` must call `repoMapSvc.Close()` before
    DB close/cleanup fan-out. Do **not** additionally register parser close in
    `app.cleanupFuncs` for this path (avoids double-close and DB-close races).
  - For standalone parser usage outside repo-map service ownership, ensure explicit close
    at the owning lifecycle boundary.
  `Parser.Close()` must drain the parser pool (calling `tree_sitter.Parser.Close()` on
  each), iterate the query cache (calling `tree_sitter.Query.Close()` on each compiled
  query), and drain the tree cache via `lru.Purge()` — `Purge()` fires the eviction
  callback for every entry (outside the lock, same as `RemoveOldest()`), calling
  `tree_sitter.Tree.Close()` on each master tree and decrementing `totalBytes` to zero
  naturally. No separate tree-closing loop is needed.
- **Clone-and-own tree sharing**: The LRU cache stores a "master" `*tree_sitter.Tree`
  per file. `Get()` returns `tree.Clone()` — the caller owns the clone and must call
  `Close()` on it. Use `lru.NewWithEvict()` to register the eviction callback, which
  (a) calls `tree.Close()` to release C memory and (b) decrements `totalBytes` by the
  entry's `estimatedBytes` via `c.totalBytes.Add(-entry.estimatedBytes)`. In
  `hashicorp/golang-lru/v2`, the user-supplied eviction callback is called **after** the
  cache lock is released — the library buffers the evicted key/value inside the critical
  section and fires the callback outside it. Two consequences: (1) the callback may
  safely call back into the cache without deadlocking; (2) `RemoveOldest()`, `Add()`,
  and `Purge()` all return to the caller only after their callbacks have completed, so
  the subsequent `c.totalBytes.Load()` in the eviction loop always sees the decremented
  value. `tree.Clone()` is O(1) — it calls `ts_tree_copy()` in C which increments an
  internal atomic refcount and allocates a ~32-byte wrapper. All underlying tree data is
  immutable and shared. No Go-side refcounting or mutexes are needed.

`internal/treesitter/query.go`:
- Load `*-tags.scm` files from embedded FS (one file per language, no inheritance)
- Compile queries per language (cached after first compilation via `Query` objects). The query cache must implement `Close() error` that iterates all cached `*tree_sitter.Query` objects and calls `Close()` on each — this is called by `Parser.Close()` during shutdown.
- Run queries against trees, extract captures into `Tag` and `SymbolInfo` structs
- **Capture extraction**: implement two execution modes:
  - **Parity mode (required for Aider comparator tests):** emit tags only from modern captures:
    - `@name.definition.<type>` → `Tag{Kind: "def", NodeType: <type>}`
    - `@name.reference.<type>` → `Tag{Kind: "ref", NodeType: <type>}`
  - **Enhanced mode (allowed post-parity):** additionally support legacy paired-node captures:
    - `@name` paired with `@definition.<type>` → `Tag{Kind: "def", NodeType: <type>}`
    - `@name` paired with `@reference.<type>` → `Tag{Kind: "ref", NodeType: <type>}`
  - Shared:
    - `@definition.<type>` → full definition node (line range, params extraction)
    - `@reference.<type>` → full reference node
    - `@doc` → associated documentation comment (stored in `SymbolInfo.DocComment`)
  - Parse `NodeType` from the `<type>` suffix on `@*.definition.<type>` / `@*.reference.<type>` when present, or from paired `@definition.<type>` / `@reference.<type>` in enhanced mode.
  - Parity fixtures must run in parity mode; enhanced mode must not alter parity-suite expectations.
  - **Parity capture-loop policy (mandatory):** if comparator-visible behavior depends on Aider capture-loop quirks (including post-loop/last-node handling artifacts), parity mode must either (a) replicate the behavior exactly, or (b) declare an explicit normalized divergence with comparator-side normalization rules recorded in artifacts. Silent divergence is not allowed.
- **Predicate handling**: go-tree-sitter auto-evaluates text predicates (`#eq?`, `#match?`, `#any-of?`, `#not-eq?`, `#not-match?`, `#not-any-of?`) during match iteration — `QueryMatches.Next()` and `QueryCaptures.Next()` both call `satisfiesTextPredicate()` internally, looping until a match passes all predicates. No manual filtering is required. (Verified against go-tree-sitter source: `query.go`.) Aider's tags.scm files also use custom predicates (`#strip!`, `#set-adjacent!`, `#select-adjacent!`) — these are Aider-specific and not evaluated by go-tree-sitter. They are used for doc comment association and can be safely ignored (we associate docs via AST adjacency instead). Property predicates (`#set!`) are NOT auto-evaluated but are accessible via `query.PropertySettings(patternIndex)` if needed in the future.
- **Error handling policy**:
  - Parse errors: Log at debug level, return partial results (tree-sitter produces partial ASTs for malformed code). Never fatal.
  - Query compilation failure: Log at warn level, skip that query file. Handles malformed `.scm` files gracefully. Note: custom predicates like `#strip!` are silently accepted by go-tree-sitter — they produce no warnings and do not prevent query compilation; they are simply unrecognized and skipped during match iteration.
  - Unsupported language: `CanHandle()` returns false, next explorer in chain handles it. No error.
  - File read errors (permissions, symlinks): Skip file, log at debug level.
  - CGO crashes: Go's `recover()` does not catch C-level segfaults (SIGSEGV). A tree-sitter grammar crash will kill the process. Mitigation is preventive: pin grammar versions via `tree-sitter-language-pack`, run tag extraction tests per language in the validation suite. No runtime recovery is attempted.

`internal/treesitter/languages.go`:
- Load `languages.json` manifest
- Extension-to-language mapping (extend existing `internal/lcm/explorer/extensions.go`). **Critical**: several extensions require explicit overrides because the tree-sitter grammar name differs from the language name inferred by extension:

  | Extension | Tree-sitter Language | Notes |
  |-----------|---------------------|-------|
  | `.jsx`    | `javascript`        | JS grammar handles JSX natively |
  | `.tsx`    | `typescript` (parity key) | In `parity_mode=true`, use the `typescript` parser/query path (no `tsx-tags.scm` in Aider). `tsx`-specific parser behavior is enhancement-only. |
  | `.cs`     | `csharp`            | Aider primary uses `csharp` naming |
  | `.ml`     | `ocaml`             | Distinguish from `.mli` → `ocaml_interface` |
  | `.mli`    | `ocaml_interface`   | Separate tags.scm from ocaml |
  | `.kt`     | `kotlin`            | Kotlin (fallback source) |
  | `.tf`     | `hcl`               | Terraform files use HCL grammar |
  | `.hcl`    | `hcl`               | HCL (fallback source) |

  Store this as a `var extensionOverrides = map[string]string{...}` table. For query loading, add an explicit language alias table (`tsx -> typescript`) so `HasTags("tsx")` resolves to `typescript-tags.scm`. Test with a golden file of all supported extensions → expected parse language and query-key mapping.
- Language capability queries: `HasTags(lang string) bool` — returns true if a `<lang>-tags.scm` file exists in the embedded FS.

**Vendor query files (Phase 1A — bootstrap with Go + Python):**
```
internal/treesitter/queries/go-tags.scm       # from tree-sitter-language-pack/
internal/treesitter/queries/python-tags.scm    # from tree-sitter-language-pack/
```

**Embed directive:**
```go
//go:embed queries/* languages.json
var queriesFS embed.FS
```

**Testing:**
- Parse known Go and Python source files
- Verify correct def/ref tag extraction counts and names against Aider's capture convention
- Verify `@name.definition.*` → Tag{Kind: "def"} mapping
- Verify `@name.reference.*` → Tag{Kind: "ref"} mapping
- Verify SymbolInfo extraction (params, types from definition nodes)
- Verify custom predicates (`#strip!`, `#set-adjacent!`) are gracefully ignored
- Benchmark: parse time, cache hit/miss

### 1B: Expand to All Languages

Add the remaining 37 languages (all ship together, no priority ordering). Each needs:
1. Grammar Go module dependency in `go.mod` (revision pinned from `tree-sitter-language-pack` v0.13.0 plus manual exception manifest entries; `scripts/gen-treesitter-deps.sh` output is advisory, `tsaudit` output is authoritative)
2. Registration in the language registry
3. `*-tags.scm` file vendored from Aider (from whichever query directory has it)
4. Unit tests with representative source files

**Phase 1B grammar `go get` commands (auto-generated set)** (34 modules listed below; 36 auto-generated total with Phase 1A's Go and Python; 37 total including manual-exception QL):
```bash
# Primary (tree-sitter-language-pack) — 25 unique modules for 26 languages (27/28 total with Phase 1A)
go get github.com/ObserverOfTime/tree-sitter-arduino@53eb391da4c6c5857f8defa2c583c46c2594f565
go get github.com/tree-sitter/tree-sitter-c@ae19b676b13bdcc13b7665397e6d9b14975473dd
go get github.com/tree-sitter-grammars/tree-sitter-chatito@c0ed82c665b732395073f635c74c300f09530a7f
go get github.com/theHamsta/tree-sitter-commonlisp@32323509b3d9fe96607d151c2da2c9009eb13a2f
go get github.com/tree-sitter/tree-sitter-cpp@12bd6f7e96080d2e70ec51d4068f2f66120dde35
go get github.com/tree-sitter/tree-sitter-c-sharp@362a8a41b265056592a0c3771664a21d23a71392
go get github.com/gdamore/tree-sitter-d@fb028c8f14f4188286c2eef143f105def6fbf24f
go get github.com/UserNobody14/tree-sitter-dart@d4d8f3e337d8be23be27ffc35a0aef972343cd54
go get github.com/Wilfred/tree-sitter-elisp@951d802ee2b92f16f1091d91827655bf76afc076
go get github.com/elixir-lang/tree-sitter-elixir@d24cecee673c4c770f797bac6f87ae4b6d7ddec5
go get github.com/razzeee/tree-sitter-elm@6e3c6d51f13168f9d7794c8e8add7dfdd07d20b8
go get github.com/gleam-lang/tree-sitter-gleam@ec3c27c5eef20f48b17ee28152f521697df10312
go get github.com/tree-sitter/tree-sitter-javascript@58404d8cf191d69f2674a8fd507bd5776f46cb11
go get github.com/tree-sitter/tree-sitter-java@e10607b45ff745f5f876bfa3e94fbcc6b44bdc11
go get github.com/MunifTanjim/tree-sitter-lua@4fbec840c34149b7d5fe10097c93a320ee4af053
go get github.com/acristoffers/tree-sitter-matlab@1bccabdbd420a9c3c3f96f36d7f9e65b3d9c88ef
go get github.com/tree-sitter/tree-sitter-ocaml@3ef7c00b29e41e3a0c1d18e82ea37c64d72b93fc
go get github.com/tree-sitter-grammars/tree-sitter-properties@6310671b24d4e04b803577b1c675d765cbd5773b
go get github.com/6cdh/tree-sitter-racket@130e76536bd3a45df7b7fd71cfa3d0df25fcfe8e
go get github.com/r-lib/tree-sitter-r@0e6ef7741712c09dc3ee6e81c42e919820cc65ef
go get github.com/tree-sitter/tree-sitter-ruby@89bd7a8e5450cb6a942418a619d30469f259e5d6
go get github.com/tree-sitter/tree-sitter-rust@261b20226c04ef601adbdf185a800512a5f66291
go get github.com/JoranHonig/tree-sitter-solidity@4e938a46c7030dd001bc99e1ac0f0c750ac98254
go get github.com/alex-pinkus/tree-sitter-swift@78d84ef82c387fceeb6094038da28717ea052e39
go get github.com/tree-sitter-grammars/tree-sitter-udev@2fcb563a4d56a6b8e8c129252325fc6335e4acbf

# Fallback (tree-sitter-languages) — 9 additional unique modules for 10 languages
go get github.com/stadelmanma/tree-sitter-fortran@8334abca785db3a041292e3b3b818a82a55b238f
go get github.com/tree-sitter/tree-sitter-haskell@0975ef72fc3c47b530309ca93937d7d143523628
go get github.com/MichaHoffmann/tree-sitter-hcl@fad991865fee927dd1de5e172fb3f08ac674d914
go get github.com/tree-sitter/tree-sitter-julia@e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3
go get github.com/fwcd/tree-sitter-kotlin@57fb4560ba8641865bc0baa6b3f413b236112c4c
go get github.com/tree-sitter/tree-sitter-php@de11d0bcec62b8ed6b0c7edd55051042f37b8b05
go get github.com/tree-sitter/tree-sitter-scala@97aead18d97708190a51d4f551ea9b05b60641c9
go get github.com/tree-sitter/tree-sitter-typescript@75b3874edb2dc714fb1fd77a32013d0f8699989f
go get github.com/maxxnino/tree-sitter-zig@a80a6e9be81b33b182ce6305ae4ea28e29211bd5
```

**Phase 1B manual-exception `go get` commands** (outside auto-generated set; pinned in manual exception manifest):
```bash
go get github.com/tree-sitter/tree-sitter-ql@1fd627a4e8bff8c24c11987474bd33112bead857
```

Note: `ocaml` and `ocaml_interface` share the same grammar module (`tree-sitter-ocaml`). `csharp` and `c_sharp` share `tree-sitter-c-sharp`. `typescript` uses `tree-sitter-typescript` (dir: `typescript`). `ql` is a documented manual exception pin (`1fd627a4...`, tag `v0.23.1`); it is not in `tree-sitter-language-pack` and has no `language_definitions.json` entry.

### 1C: Local Build + Developer Validation (Fork Scope)

This fork explicitly prioritizes implementation parity and local verification over
release/distribution pipeline changes.

**Files to modify (local/dev focused):**

`Taskfile.yaml`:
- Set default build/test profile for this fork to `CGO_ENABLED=1`.
- Add explicit CGO-enabled targets for tree-sitter/repo-map validation and parity gates.
- Add optional non-signoff compatibility targets (`build:nocgo`, `test:nocgo`) only if needed for unrelated workflows.
- Add `tsaudit`, `tsaudit:update`, `tsaudit:verify` tasks.
- Add `test:parity` (parity_mode=true) and `test:exceed` (parity_mode=false) tasks (see Phase 5E).
- Parity/exceed tasks MUST assert `CGO_ENABLED=1` and fail fast otherwise.

`internal/treesitter/`:
- Add build-time CGO diagnostic files:
  - `cgo_check.go` (`//go:build cgo`)
  - `cgo_check_nocgo.go` (`//go:build !cgo`) with
    `panic("crush requires CGO_ENABLED=1 and a C compiler for tree-sitter support")`

**Out of scope for this fork plan:**
- GoReleaser target matrix redesign
- Packaging/distribution changes (AUR/nfpm/npm/apt/yum)
- Platform support/deprecation announcements

These may be maintained in a separate release-engineering document if needed.

**Read-only data contract (mandatory):**
- Add `repo_map_session_read_only` table keyed by `(repo_key, session_id, rel_path)`.
- Add sqlc queries: upsert/list/delete session read-only paths.
- `project:map-reset` clears both map caches and session read-only rows.
- Parity fixtures must provide explicit read-only sets through this table and assert mention/addable behavior against that source.

---

## Phase 2: Explorer Upgrade

Replace regex explorers with tree-sitter, add heuristic enrichment layer.

### 2A: Tree-Sitter Code Explorer

**Note on volatile counts**: module/file/line counts in this section are observational context only and are not gate criteria.

**Files to create:**

`internal/lcm/explorer/code_treesitter.go`:
```go
type TreeSitterExplorer struct {
    parser treesitter.Parser
}
```
- `CanHandle()` — in parity path, returns true only when both language and tags query are available (`parser.SupportsLanguage(lang) && parser.HasTags(lang)` equivalent contract).
- `Explore()` — guard: if `len(input.Content) > MaxFullLoadSize`, return a size-exceeded summary (matching existing explorers; count references are observational and may change with refactors). Otherwise calls `parser.Analyze()`, which runs the language's `*-tags.scm` query and extracts definition/reference captures. Applies heuristic enrichment, formats into `ExploreResult`. The extractor must support both modern and legacy capture styles described in the Capture Convention section, but parity scoring/output uses modern capture-derived emissions only.

**Files to modify:**

`internal/lcm/explorer/explorer.go`:
- Use functional options for backward-compatible parser injection:
```go
type RegistryOption func(*Registry)

func WithTreeSitter(parser treesitter.Parser) RegistryOption {
    return func(r *Registry) { r.tsParser = parser }
}

func NewRegistry(opts ...RegistryOption) *Registry {
    r := &Registry{}
    for _, opt := range opts {
        opt(r)
    }
    if r.tsParser != nil {
        // Data format explorers BEFORE TreeSitterExplorer (tree-sitter has grammars
        // for json/yaml/toml/html but we keep specialized parsers for these).
        r.explorers = []Explorer{
            &BinaryExplorer{},
            &JSONExplorer{}, &CSVExplorer{}, &YAMLExplorer{},
            &TOMLExplorer{}, &INIExplorer{}, &XMLExplorer{}, &HTMLExplorer{},
            &TreeSitterExplorer{parser: r.tsParser},
            // Existing regex code explorers retained as fallback for unsupported languages
            // ...
        }
    }
    // If no parser provided, use existing regex explorers as before
}
```
- **Runtime-path parity matrix (required for Volt gate B3):** exploration persistence coverage must be tested for every in-scope large-content ingestion/retrieval path in this fork:
  1. `lcm.tool_output.create` — tool-message large-output interception path (`message_decorator.Create`)
  2. any additional large-content insertion path introduced by this plan (must be assigned stable path IDs before parity scoring)
  3. success-path + explicit error-path behavior for each path.
  The parity harness must record which ingestion paths are in scope for this fork and fail if a declared in-scope path lacks persistence assertions.
- All existing `NewRegistry()` and `NewRegistryWithLLM()` call sites continue to work unchanged. (Current call-site counts are intentionally not hardcoded in this plan to avoid drift when tests are added.)
- **Integration requirement (moved from deferred to required)**: wire the explorer pipeline into LCM's large-content storage path in the message decorator so explorer output is actually persisted to `lcm_large_files.exploration_summary` and `lcm_large_files.explorer_used`.
  - Modification site: `internal/lcm/message_decorator.go` (`Create()` path where large tool output is stored).
  - After `InsertLargeTextContent`, call registry exploration for the stored content and persist via `UpdateLcmLargeFileExploration`.
  - Respect `cfg.Options.LCM.DisableLargeToolOutput` and `cfg.Options.LCM.LargeToolOutputTokenThreshold` as gating controls.
  - Use conservative failure behavior: if explorer analysis fails, keep existing storage behavior and leave exploration fields null.
- This converts explorer upgrades from package-internal improvements into end-to-end LCM functionality, which is required for Volt parity.
- **Update `NewRegistryWithLLM`** to accept and pass through `...RegistryOption` so that production code using `NewRegistryWithLLM` can compose tree-sitter + LLM:
```go
func NewRegistryWithLLM(llm LLMClient, agentFn AgentFunc, opts ...RegistryOption) *Registry {
    r := NewRegistry(opts...)
    // ... existing LLM tier wiring (agentFn must be preserved for tier-3 agent-based exploration)
}
```
- **Fallback chain**: `BinaryExplorer` → `JSONExplorer` → `CSVExplorer` → `YAMLExplorer` → `TOMLExplorer` → `INIExplorer` → `XMLExplorer` → `HTMLExplorer` → `TreeSitterExplorer` → `ShellExplorer` → `TextExplorer` → `FallbackExplorer`. **Critical ordering**: data format explorers (JSON, YAML, TOML, CSV, INI, XML, HTML) must precede `TreeSitterExplorer` because Phase 5B explicitly keeps these as regex/stdlib-parser based. `ShellExplorer` remains after `TreeSitterExplorer` because Bash/Shell has no tree-sitter tags.scm — shell files fall through tree-sitter's `CanHandle()` and are caught by `ShellExplorer`.
- **Volt parity delegation rule (mandatory):** `FallbackExplorer` parity behavior must include shebang/content-based delegation semantics equivalent to Volt (including script-type detection before generic fallback formatting). Add parity fixtures covering shebang-driven dispatch and ambiguous-content cases.
- **No language-specific exceptions**: all code languages with tree-sitter support use tree-sitter uniformly — including Go (replacing the existing `GoExplorer` which uses `go/ast`). This ensures standard support and consistent behavior.
- **Regex explorer deletion**: Delete regex code explorers from `code.go` only when their language has a validated tree-sitter equivalent. Specifically:
  - `GoExplorer`, `PythonExplorer`, `JavaScriptExplorer`, `RubyExplorer`, `RustExplorer`, `JavaExplorer`, `CExplorer`, `CppExplorer` — delete after Phase 1B validation. (Note: `SwiftExplorer` and `CSSExplorer` do not exist in Crush and are not listed here.)
  - `TypeScriptExplorer` — delete after TypeScript tree-sitter support is validated (TypeScript is in the fallback query directory). `.ts`/`.tsx`/`.mts`/`.cts` files will be handled by tree-sitter via extension overrides.
  - `ShellExplorer` — **keep permanently**. Bash/Shell has no tree-sitter tags.scm in Aider.

The three-tier pipeline (static → LLM → agent) is unchanged — tree-sitter replaces the tier 1 static layer for all code languages.

### 2B: Heuristic Enrichment Layer

**Files to create:**

`internal/lcm/explorer/heuristic.go`:
- `EnrichAnalysis(analysis *treesitter.FileAnalysis, content []byte) *EnrichedAnalysis`
- Categorize imports: stdlib vs third-party vs local (per language)
- Detect language idioms: React components (capitalized + JSX return), dataclasses, abstract classes, async generators
- Infer visibility from naming conventions (Go exported = capitalized, Python private = underscore prefix)
- Detect module patterns: `if __name__ == "__main__"`, `module.exports`, etc.

`internal/lcm/explorer/stdlib/` — per-language stdlib lists:

| File       | Contents                               | Source    |
|------------|----------------------------------------|-----------|
| `go.go`    | ~45 stdlib package prefixes            | Volt      |
| `python.go`| ~210 builtin modules                   | Volt      |
| `node.go`  | Node.js builtins (JS + TypeScript)     | Volt      |
| `rust.go`  | std/core/alloc crates                  | Volt      |
| `java.go`  | java./javax. packages (prefix match)   | Volt      |
| `csharp.go`| System.* namespaces (prefix match)     | Volt      |
| `ruby.go`  | stdlib gems (~85)                      | Volt      |
| `swift.go` | Framework categorization (~78)         | Volt      |
| `c.go`     | Standard headers (~29)                 | Volt      |
| `cpp.go`   | Standard + C headers (~67+21)          | Volt      |
| `kotlin.go`| kotlin. stdlib packages (prefix match) | New       |
| `scala.go` | scala. stdlib packages (prefix match)  | New       |
| `haskell.go`| base, containers, etc. (~30)          | New       |
| `php.go`   | PHP standard extensions (~50)          | New       |

`internal/lcm/explorer/heuristic_test.go`

### 2C: Output Formatting

Tree-sitter + heuristic explorer output should match or exceed Volt's depth:

```
## example.py (Python)

### Imports
- **stdlib**: os, sys, pathlib
- **third_party**: requests, flask
- **local**: .utils, .config

### Classes
- `AuthService(BaseService)` — abstract, 5 methods
  - `authenticate(user: str, password: str) -> Token`
  - `refresh_token(token: Token) -> Token` — async

### Functions
- `create_app(config: Config) -> Flask` — factory function
- `main()` — entry point (__main__ block)

### Constants
- `DEFAULT_TIMEOUT`, `MAX_RETRIES`
```

Progressive disclosure: per-section caps with required overflow-count indicator. For `parity_mode=true`, accept Volt-observed variants (including `"... and N more"`, `"(+N more)"`, plain `", ..."`, and bracketed sampled/truncation markers) and normalize them in comparator scoring. For `parity_mode=false`, emit canonical form `"... and N more"`.

**Testing:**
- Golden file tests comparing tree-sitter explorer output against expected format
- Parity fixtures for fallback shebang/content delegation and overflow-marker variant normalization (`... and N more`, `(+N more)`, `, ...`, bracketed sampled/truncation markers)
- Regression tests ensuring >= regex explorer information
- Run `go test ./internal/lcm/explorer -update` for golden files

### 2D: LCM Runtime Wiring (Required for Volt Parity)

Wire explorers into the real LCM large-content path so analysis is produced during normal operation, not only in unit tests. This phase lands runtime wiring and bootstrap end-to-end validation, but feature sign-off still requires full inventory-wide B3/5E closure.

**Files to modify:**

`internal/lcm/message_decorator.go`:
- In `Create()` tool-message large-output path, after `InsertLargeTextContent(...)` succeeds, run explorer analysis on the stored text.
- Persist results through existing SQLC query `UpdateLcmLargeFileExploration` (`internal/db/sql/lcm.sql`) with:
  - `exploration_summary`
  - `explorer_used`
- Respect LCM config switches:
  - `DisableLargeToolOutput`
  - `LargeToolOutputTokenThreshold`
- **Required config plumbing**: update `NewMessageDecorator` constructor and app wiring so effective LCM options are available in the decorator path (current code uses static `LargeOutputThreshold`).
- Failure behavior must remain non-blocking: if exploration fails, keep existing storage/reference flow and continue request handling.

`internal/agent/tools/lcm_describe.go`:
- Scope file/summary lookup to caller session lineage (self + ancestors) for scoped-context paths; parity fixtures must also model comparator missing-conversation/unscoped fallback behavior where applicable.
- Preserve explicit output behavior for non-persisted exploration (no-summary path).

`internal/agent/tools/lcm_expand.go`:
- Scope summary expansion to caller session lineage (self + ancestors), with unrelated-session deterministic denial.
- Authorization profile rule (single-source):
  - `parity_mode=true`: Volt-strict semantics are required for scoring: lineage scope **plus sub-agent-only caller restriction** (matching current comparator behavior).
  - `parity_mode=false`: additional policy variants are allowed as enhancements and must be recorded as active toggles.
- Do not mix parity and enhancement authorization paths in the same scored fixture run.

**Files to create:**

`internal/lcm/explorer/runtime.go`:
- Small runtime adapter that builds registry with optional tree-sitter parser and executes exploration for large stored content.
- Returns `(summary string, explorer string, err error)` for decorator persistence.

**Validation:**
- End-to-end test proving the large tool-output path writes non-null `lcm_large_files.exploration_summary` and `lcm_large_files.explorer_used` (bootstrap path).
- End-to-end parity-path tests proving each inventoried ingestion path satisfies its B3 matrix expectation in the active profile (`persists_exploration=true|false` in parity mode, universal persistence in enhancement mode).
- End-to-end tests proving `lcm_describe` reflects both persisted and non-persisted exploration outcomes.
- Retrieval-scope tests for `lcm_describe` and `lcm_expand`: self-session allowed, ancestor-session allowed, unrelated-session denied.

---

## Phase 3: Repo Map Core

Implement the ranking, budgeting, and rendering pipeline.

### 3A.0: Repo Map Vertical-Slice Gate (Required Before 3A.1+)

**Profile note:** `parity_mode=true` uses Aider comparator acceptance semantics for budget fit; strict `<= TokenBudget` hard-safety is enhancement-profile (`parity_mode=false`) unless explicitly marked as comparator-only.

Before full graph and parity implementation, land and validate one end-to-end
vertical slice (`extract -> graph -> rank -> render -> budget-fit`) on a small
fixture repo.

**Mandatory pass criteria:**
1. Running the vertical-slice test produces a map string where `strings.TrimSpace(map) != ""` and at least one rendered file entry exists.
2. Budget-fit assertions are profile-scoped:
   - `parity_mode=true`: comparator acceptance behavior matches Aider (`parityTokenCount`-based scoring).
   - `parity_mode=false`: `safetyTokenCount <= TokenBudget` in 100% of runs.
3. Determinism assertions are profile-scoped in `files` refresh mode with unchanged inputs:
   - `parity_mode=true`: 10 repeated runs produce identical comparator-normalized output hashes (stage-3 order-insensitive normalization).
   - `parity_mode=false`: 10 repeated runs produce byte-identical raw output hashes.
4. Stage-0+1/2/3 assembly invariants are validated on the fixture:
   - stage 0 (optional) = special-file prelude,
   - stage 1 = ranked definition entries,
   - stage 2 = remaining graph-node filename-only entries,
   - stage 3 = remaining repo filename-only entries,
   - trim order = stage 3 -> stage 2 -> stage 1, with stage 0 preserved by prepend priority.

### 3A.1: Tag Extraction + Graph Construction

**Files to create:**

`internal/repomap/repomap.go`:
```go
type Service struct {
    parser       treesitter.Parser
    db           *db.Queries
    rawDB        *sql.DB
    rootDir      string
    cfg          *config.RepoMapOptions
    lifecycleCtx context.Context // app lifecycle context, passed at construction; cancels on SIGINT
}

func NewService(cfg *config.Config, q *db.Queries, rawDB *sql.DB, rootDir string, lifecycleCtx context.Context) *Service
func (s *Service) Generate(ctx context.Context, opts GenerateOpts) (string, int, error) // request-scoped ctx (5-15s deadline)
func (s *Service) Available() bool
func (s *Service) AllFiles(ctx context.Context) []string                 // cached repo file list from tag extraction walk (blocks until PreIndex completes or ctx cancellation)
func (s *Service) LastGoodMap(sessionID string) string                   // most recently generated map for a session (in-memory, sync.RWMutex-protected)
func (s *Service) LastTokenCount(sessionID string) int                   // token count of the last generated map for a specific session
func (s *Service) ShouldInject(sessionID string, runKey RunInjectionKey) bool // lock-protected atomic check-and-set for per-Run() guard
func (s *Service) RefreshAsync(sessionID string, opts GenerateOpts)      // schedule async regeneration (uses s.lifecycleCtx internally)
func (s *Service) Refresh(ctx context.Context, sessionID string, opts GenerateOpts) (string, int, error) // explicit regenerate path
func (s *Service) Reset(ctx context.Context, sessionID string) error     // explicit cache/state reset path used by project:map-reset
func (s *Service) PreIndex()                                             // walk + parse all files (uses s.lifecycleCtx internally)
func (s *Service) Close() error                                          // release parser pool via treesitter.Parser.Close()

// RunInjectionKey identifies one logical Run() invocation for repo-map injection gating.
// It MUST be derived from data available in the Run()/PrepareStep path (not from fantasy.Message IDs).
type RunInjectionKey struct {
    RootUserMessageID string // DB ID returned by createUserMessage() for the root Run() prompt
    QueueGeneration   int64  // monotonic per-session queue generation for re-queued Run() prompts
}

type GenerateOpts struct {
    SessionID        string
    ChatFiles        []string   // files currently in conversation
    MentionedFnames  []string   // filenames extracted from Aider-equivalent current-message text (`cur_messages` semantics)
    MentionedIdents  []string   // identifiers extracted from Aider-equivalent current-message text (`cur_messages` semantics)
    TokenBudget      int        // max tokens for the map
    MaxContextWindow int        // model's max context window (for budget cap)
    ForceRefresh     bool       // parity path: true only on full attempt #1
}
```

`internal/repomap/tags.go`:
- `extractTags(ctx, rootDir string, parser treesitter.Parser)` — derive file universe and parse files for def/ref tags.
- **Authoritative file universe contract (parity-critical):**
  - `parity_mode=true`: match Aider behavior exactly — when inside a git repo, repo-map ranking input uses tracked + staged files and applies Aider-equivalent ignore filtering (including subtree-only behavior when configured); read-only repo files are included in chat-set computation and excluded from addable universe as in Aider; when no git repo is available, use in-chat files only for the universe fallback.
  - `parity_mode=false`: walker-derived fallback may be used as an enhancement.
  Persist and expose which source was used (e.g. `git_tracked_filtered`, `inchat_fallback`, `walker_fallback`) in parity artifacts/debug logs.
- Normalize all repo-map paths relative to the same root directory used by config/app wiring; do not mix `os.Getwd()` and `cfg.WorkingDir()` semantics in ranking/personalization paths.
- **Universe-source split (mandatory):** in `parity_mode=true`, git-derived tracked+staged universe selection is authoritative whenever a git repo is available. Walker/fsext traversal may be used only for enhancement profile or non-git fallback paths.
- **Ignore/filter implementation note:** parity-mode filtering must be Aider-equivalent. fsext-based traversal/filtering is acceptable as implementation support only when it does not alter parity-universe semantics; any divergence is a hard parity failure.
- **Lock and dependency metadata handling**: `commonIgnorePatterns` (`fsext/ls.go:44-81`) filters `Cargo.lock`, `package-lock.json`, `yarn.lock`, and `pnpm-lock.yaml`. For parity with Aider important-file behavior, stage-0 prelude logic in `special.go` must include the upstream important-file set from `special.py` (not just these 4 lockfiles).
- **Parity-specific stage-0 source invariant (mandatory):** in `parity_mode=true`, special files must come only from computed `otherFnames` (Aider-equivalent behavior). Do not perform root existence re-check inclusion for files absent from `otherFnames`.
- **Enhancement-only allowance:** in `parity_mode=false`, optional root existence re-check inclusion is allowed as a fork enhancement and must be recorded in artifacts as enhancement behavior.
- Mtime-based caching via `repo_map_file_cache` table — skip files with unchanged mtime. On incremental refresh, only files with changed mtime are re-parsed and their tags re-extracted. The graph is rebuilt from the full tag set (cached + freshly extracted) but this is fast since it's purely in-memory set operations.
- Concurrent parsing with `errgroup` + bounded semaphore (capacity = `runtime.NumCPU()`, matching the parser pool size from Phase 1A). The semaphore and pool must have the same bound — if the semaphore allows more goroutines than parsers, goroutines block on the pool; if fewer, pool capacity is wasted.

`internal/repomap/graph.go`:
- `buildGraph(tags []treesitter.Tag) *FileGraph` — directed multigraph, files as nodes
- Edge from file A to file B when A has a ref tag for identifier X and B has a def tag for identifier X
- Per-identifier base multiplier `mul` (per Aider `repomap.py:492-499`):
  - Mentioned identifiers: `mul *= 10`
  - Long names (>= 8 chars, snake_case or camelCase or kebab-case): `mul *= 10`
  - Private names (leading underscore): `mul *= 0.1`
  - Identifiers defined in >5 files: `mul *= 0.1`
- Per-edge multiplier `use_mul` (per Aider `repomap.py:507-514`):
  - If referencer is a chat file: `use_mul *= 50`
- Edge weight = `use_mul * sqrt(num_refs)` — the sqrt provides sublinear scaling so high-frequency references don't dominate (10 refs → weight ×3.16 instead of ×10)
- **Required ordering for fallbacks** (matching Aider's logic — this ordering is critical for correctness):
  1. **Per-file lexical backfill first**: if a file has definitions but no references, backfill refs via lexical `Token.Name` extraction equivalent to Aider behavior.
  2. **No-reference global fallback**: if the references dict is still empty globally, populate references from definitions (`references = defines`) before edge construction.
  3. **Self-edges for orphan definitions**: identify identifiers that have definitions but remain unreferenced, and add self-edge weight 0.1.
  4. **Build cross-file ref→def edges**: for each identifier in the (possibly modified) references dict, connect referencer file to each defining file.
- **Per-file reference extraction fallback**: Aider backfills refs via lexical `Token.Name` extraction when defs exist but refs are absent (`repomap.py:338-363`). To meet parity, implement fallback in two tiers:
  1. **Parity tier (required):** lexical name-token backfill equivalent to Aider behavior.
  2. **Enhanced tier (optional, post-parity):** language-specific AST node fallback can be added, but parity fixtures must run with the parity tier enabled and must not depend on enhanced behavior.

  Any language-specific node table used for enhanced fallback must be limited to supported languages in `languages.json`; remove unsupported/out-of-scope entries from the required plan path.

### 3B: PageRank

`internal/repomap/pagerank.go`:
- Inline personalized PageRank (~60 lines, no gonum dependency)
- `Rank(graph *FileGraph, personalization map[string]float64) []RankedDefinition`
- Damping factor: 0.85
- Convergence: delta < 1e-6 or 100 iterations
- **Dangling node handling**: In parity mode, pass `dangling=personalization` only when personalization is non-empty (matching Aider's conditional `pers_args` behavior). When personalization is empty, run unpersonalized PageRank with no dangling override.
- **Degenerate graph handling**: If the graph has zero nodes or all edge weights are zero, fall back in two stages (matching Aider's `except ZeroDivisionError`): first try the unpersonalized graph, then return an empty `[]RankedDefinition` list. Callers handle the empty case (no repo map is injected). Guard against division by zero in weight normalization.
- **Personalization vector** (three-step, matching Aider `repomap.py:374-445`):
  1. **Chat files** (additive): For each chat file, `personalization[file] += 100.0 / len(allFiles)`.
  2. **Mentioned fnames** (max): For each file in `MentionedFnames`, `personalization[file] = max(personalization[file], 100.0 / len(allFiles))`. Use `max()` (not `+=`) to avoid double-counting files that are both in chat and mentioned.
  3. **Path-component ident matches** (once per file): Build a component set from the file's relative path using `filepath.Dir()` parts + basename with extension + basename without extension. For `src/auth/login.py`, the component set is `{"src", "auth", "login.py", "login"}` (matching Aider's `Path.parts` + `path_obj.name` + `os.path.splitext(name)[0]`). Use **exact set intersection** (not substring matching) between path components and `MentionedIdents`. If the intersection is non-empty, `personalization[file] += 100.0 / len(allFiles)`. This is applied **once per file** regardless of how many identifiers match — use a boolean flag, not an additive counter per match. Matching Aider's `components_to_check.intersection(mentioned_idents)` logic.

  The base unit is `100.0 / len(allFiles)` (all repo files), NOT `100.0 / len(chatFiles)`. This ensures the personalization scale is independent of how many files are in chat.
- **Rank distribution**: After PageRank converges on file-level ranks, distribute each file's rank to its per-definition tags. For each file, iterate its outgoing edges, accumulating `rank * (edge_weight / total_outgoing_weight)` per `(destination_file, identifier)` pair. The output type is `[]RankedDefinition{File string, Ident string, Rank float64}`. This ranked list drives tag selection in Phase 3C budget fitting.
- **Aggregation to RankedFile**: Convert `[]RankedDefinition` to `[]RankedFile` for budget fitting. Sum definition ranks per file:
```go
type RankedDef struct {
    Name string
    Line int     // line number — needed by renderer for scope-aware context
    Rank float64
}

type RankedFile struct {
    Path  string
    Rank  float64
    Defs  []RankedDef // definitions in this file, sorted by rank descending
}
```
  Sort by `Rank` descending. This type is the input to `fitToBudget()` and `Render()`.

`internal/repomap/pagerank_test.go`:
- Star graph: center should rank highest
- Chain graph: first node should rank highest with personalization
- Disconnected components: verify each ranks independently
- Convergence test: verify delta decreases monotonically

### 3C: Token Budget Fitting + Rendering

`internal/repomap/budget.go`:
- `fitToBudget(tags []treesitter.Tag, ranked []RankedFile, budget int, chatFiles []string) []RankedFile`
- **Chat file handling (profile-scoped)**:
  - `parity_mode=true`: preserve Aider-equivalent assembly semantics for ranked tags and stage construction. In particular, do not introduce pre-assembly exclusion behavior that changes candidate composition versus comparator; final render still applies chat-file skipping in `to_tree` as in comparator behavior.
  - `parity_mode=false`: stricter pre-assembly exclusions/optimizations are allowed as enhancements, provided they are not used for parity scoring.
- **Stage-0+1/2/3 assembly first, then budget fit** (matching Aider): Assemble the optional stage 0 special-file prelude plus stages 1/2/3 into a single flat `[]Tag` list before budget fitting. Stage 1 (ranked definition tags from ranked `(destination file, identifier)` pairs) + Stage 2 (remaining graph nodes as bare entries) + Stage 3 (remaining other files as bare entries). The combined list is the input to binary search.
- **Binary search operates on individual tags, not files**. Each tag is the atomic unit — a stage 1 file with multiple definition tags may be partially included (some tags kept, others trimmed). Stages 2 and 3 entries are represented as single pseudo-tags (filename only) and are therefore atomic within the binary search — only stage 1 files can be partially included. After the binary search determines the cutoff index, group surviving tags back into `[]RankedFile` for rendering.
- **Initial guess**: `initialGuess := min(maxTokens/25, len(tags))` (matching Aider). This dramatically reduces iterations by starting near the expected answer instead of at `len(tags)/2`.
- **Binary search** in parity mode follows Aider acceptance semantics with a 15% absolute-error early-accept path; enhancement mode may enforce stricter under-budget-only acceptance.
  1. Track best-so-far candidate by comparator score and safety score.
  2. In `parity_mode=true`, accept candidate when Aider-style error criterion is satisfied (`parityTokenCount` comparator path).
  3. In `parity_mode=false`, early break is allowed only when under budget and within 15% of target utilization, and final acceptance is gated by `safetyTokenCount <= TokenBudget`.
  Return the best valid result found under the active profile.
- **Token estimation and acceptance rule**: implement tokenizer-first counting with explicit parity/safety split so budget fit can both compare to Aider and preserve strict safety.
  - Add `TokenCounter` abstraction in `internal/repomap/tokens.go`:
    ```go
    type TokenCounter interface {
        Count(ctx context.Context, model string, text string) (int, error)
    }
    ```
  - Primary implementation: tokenizer-backed counting via an explicit local tokenizer provider abstraction in `internal/repomap/tokens.go` (Fantasy/Catwalk usage metadata is not a tokenizer API for arbitrary text and is non-authoritative for parity counting).
  - **Tokenizer provider contract (mandatory):**
    - `TokenCounterProvider` maps model family -> tokenizer implementation and exposes `Count(ctx, model, text) (int, error)`.
    - Supported families for parity scoring are declared in a versioned matrix artifact (`internal/repomap/testdata/parity_aider/tokenizer_support.v1.json`) with fields: `model_family`, `tokenizer_id`, `tokenizer_version`, `supported`.
    - Parity run preflight fails when the active comparator model family is unsupported (`supported=false`) or tokenizer initialization fails.
    - Enhancement profile may fall back to heuristic-only acceptance where parity scoring is not requested.
  - Fallback implementation: per-language chars-per-token ratios (below).
  - **Profile rule (mandatory):** in `parity_mode=true`, tokenizer-backed comparator counting is required for scoring and acceptance checks; heuristic fallback must not be used as a parity substitute. Heuristic fallback is allowed for safety accounting/enhancement profile behavior only.
  - Persist two values for observability and gating:
    - `parity_tokens` (Aider comparator estimate path),
    - `safety_tokens` (`max(parity_tokens, ceil(heuristic_tokens*1.15))`).
  - Budget safety pass/fail uses `safety_tokens`; Aider parity comparisons use `parity_tokens`.
  - **Aider comparator pin (mandatory in `parity_mode=true`)**:
    - If rendered map text length `< 200` chars: `parity_tokens = tokenizer_count(full_text)`.
    - Else: sample every `step = max(num_lines // 100, 1)` line from `splitlines(keepends=true)`, compute `sample_tokens = tokenizer_count(sample_text)`, then `parity_tokens = sample_tokens / len(sample_text) * len(full_text)`.
    - Preserve comparator numeric semantics: sampled comparator value is a floating estimate. Do not silently integer-round in acceptance logic.
    - Use this exact comparator path (including line-sampling method and threshold) for parity acceptance and artifact reporting.
    - If tokenizer-backed comparator counting is unavailable, parity adjudication for that run fails (do not substitute heuristic-only pass criteria).
  - **Comparator tuple pin (mandatory for parity adjudication):** parity artifacts MUST include and pin:
    - `aider_commit_sha`
    - `grep_ast_provenance` (resolved dependency fingerprint/commit/tag used for TreeContext comparator behavior)
    - `tokenizer_id` and `tokenizer_version`
    Missing any tuple element invalidates the parity run.
- **Heuristic fallback table**: Per-language chars-per-token ratios. Store a `charsPerToken map[string]float64` table in `internal/repomap/tokens.go`:

  | Language Group | Chars/Token | Rationale |
  |---------------|-------------|-----------|
  | Go, Rust, C/C++ | 3.2 | Verbose keywords, type annotations |
  | Python, Ruby | 3.8 | Shorter keywords, significant whitespace |
  | Java, C#, Kotlin | 3.4 | Verbose OOP keywords |
  | JavaScript, TypeScript | 3.5 | Mixed verbosity |
  | HTML, XML, SVG | 2.8 | Repetitive tag structure |
  | JSON, YAML, TOML | 3.0 | Key-value patterns |
  | Default (unknown) | 3.5 | Conservative middle ground |

  `EstimateTokens(text string, lang string) int` — `int(math.Ceil(float64(len(text)) / ratio))` using ceiling division with the language-specific ratio. For mixed-language map output (the rendered string), use the default ratio of 3.5.

  **Note on Aider parity**: Aider's `RepoMap.token_count()` (`repomap.py:88-100`) relies on a tokenizer-backed estimate with sampling. To meet/exceed parity, Crush must use tokenizer-backed comparator counting in parity runs; ratio heuristics are fallback for non-parity safety accounting only.

  **Note on LCM alignment**: The repo map token count used for budget fit is the same value written into LCM fixed overhead (`BudgetConfig.RepoMapTokens`). LCM must not re-estimate repo map tokens from raw injected text. Coordinator updates this overhead from the repo map service's measured token count each time map content is generated/refreshed.
- When no files in chat, expand budget: `expandedBudget = min(int(float64(budget) * cfg.RepoMap.MapMulNoFiles), maxContextWindow - 4096)`. The `MapMulNoFiles` defaults to 2.0 (matching Aider's effective CLI default from `--map-multiplier-no-files` in `args.py:265`). **Note**: Aider's library-level defaults differ — `repomap.py:56` and `base_coder.py:323` both default to 8 — but the CLI always passes the `args.py` default of 2, which is what end users experience. The `maxContextWindow - 4096` cap prevents consuming the entire context window. Guard against `expandedBudget <= 0`.
- **Comparator entrypoint pin (mandatory):** parity artifacts must record `map_mul_no_files_effective`, `map_mul_no_files_source` (`cli_default` | `config_override`), and comparator entrypoint (`aider_main_cli` vs library call). Parity adjudication is valid only when comparator entrypoint is `aider_main_cli` and effective multiplier is explicit in artifacts.

`internal/repomap/render.go`:
- `Render(files []RankedFile, tags map[string][]treesitter.Tag) string`
- Output format:
```
src/server.go:
│fn NewServer(port int) *Server
│fn (s *Server) Start() error
│fn (s *Server) handleRequest(w http.ResponseWriter, r *http.Request)
src/config.go:
│type Config struct
│fn Load(path string) (*Config, error)
```
- Lines truncated to 100 chars
- **loi_pad = 0**: No extra padding lines around definitions of interest. The definition line itself is shown, plus any parent scope headers added by the TreeContext scope-aware rendering below (e.g., enclosing class/function headers). The repo map is a structural index for the model — token efficiency matters more than readability.
- **Scope-aware rendering** (matching Aider's `TreeContext` usage contract): Mirror the repo-map `TreeContext` configuration Aider explicitly sets in `repomap.py` (flags and call sequence). Implementation internals should follow a pinned `grep-ast` provenance target recorded in parity artifacts; avoid asserting internal behavior that is not directly evidenced by `../aider`. Note: Aider does NOT use `context.scm` query files for scope rendering — scopes are determined by AST structure.
- **Parity adjudication rule:** TreeContext parity is judged by rendered behavior under the pinned comparator tuple (Aider SHA + `grep_ast_provenance` + tokenizer tuple). Internal implementation differences are acceptable if fixture-visible behavior matches comparator expectations.

  `internal/repomap/treecontext.go`:
  ```go
  type TreeContext struct {
      lines                    []string           // source lines
      scopes                   []map[int]struct{} // scopes[i] = set of scope-creating line numbers that line i belongs to
      headers                  [][]headerEntry    // headers[i] = scope headers starting at line i, sorted by size asc (smallest first)
      showLines                map[int]struct{}   // lines to include in output
      linesOfInterest          map[int]struct{}
      headerMax                int                // max lines to show for a header range (default 10, matching Aider)
      showTopOfFileParentScope bool               // whether to include scopes that start at line 0 (default false for repo map)
  }

  type headerEntry struct {
      size      int // endLine - startLine
      startLine int
      endLine   int
  }

  // walkTree populates scopes and headers by visiting every AST node.
  // Uses a tree cursor for traversal (same depth-first result as iterating node.children
  // directly, which is Aider's approach). The caller must call cursor.Close() after
  // walkTree returns to release CGO resources.
  func (tc *TreeContext) walkTree(cursor *tree_sitter.TreeCursor) {
      node := cursor.Node()
      startLine := int(node.StartPosition().Row)
      endLine := int(node.EndPosition().Row)
      size := endLine - startLine

      if size > 0 {
          // Clip header range to headerMax lines.
          clippedEnd := startLine + tc.headerMax
          if clippedEnd > endLine { clippedEnd = endLine }
          tc.headers[startLine] = append(tc.headers[startLine], headerEntry{size, startLine, clippedEnd})
      }
      for i := startLine; i <= endLine; i++ {
          if tc.scopes[i] == nil { tc.scopes[i] = make(map[int]struct{}) }
          tc.scopes[i][startLine] = struct{}{}
      }

      if cursor.GotoFirstChild() {
          tc.walkTree(cursor)
          for cursor.GotoNextSibling() {
              tc.walkTree(cursor)
          }
          cursor.GotoParent()
      }
  }
  ```

  After `walkTree`, sort each `headers[i]` by size **ascending** (smallest scope first) — matching Aider (`grep-ast TreeContext sort behavior`). `addParentScopes` takes `headers[scopeStartLine][0]`, which after ascending sort is the smallest (innermost) enclosing scope. Then for each line of interest:
  1. Add the line itself to `showLines`
  2. `addParentScopes(line)`: for each scope that line belongs to (`scopes[line]`), take the first entry in `headers[scopeStartLine]` (smallest scope). Add its header lines to `showLines` **only if** `head.startLine > 0 || tc.showTopOfFileParentScope`. This guard (initialized to `false` for repo map, matching Aider's `repomap.py:736`) prevents package-level or module-level scopes starting at line 0 from being included, which would inflate output for every file. Recursively add parent scopes of those header lines (memoized via `doneParentScopes` set).
  3. `closeSmallGaps()`: if the gap between two consecutive shown lines is exactly 1 missing line (`sorted_show[i+1] - sorted_show[i] == 2`), include the gap line — matching Aider (`grep-ast TreeContext gap-closing behavior`). Additionally, include blank lines adjacent to already-shown lines (`grep-ast TreeContext blank-line adjacency behavior`) to avoid dangling blank lines in output.

  Render format (matching Aider's `format()` from `grep-ast TreeContext format behavior`):
  - In repo-map mode (`mark_lois=False`), lines in `showLines` are emitted with `│` prefix (no `█` markers).
  - Gaps between shown lines emit a single `⋮` line
  - No line numbers (`line_number=False`), no color (`color=False`), no margin (`margin=0`)
  - `mark_lois=False` (for repo map; `█` LOI marking is not used here)
  - **Intentionally omitted**: Aider's `child_context` and `last_line` features are not implemented. `child_context=False` is Aider's default for repo map (adds child scope context lines — not needed for structural indexing). `last_line=False` is also Aider's default (shows the closing brace of scopes — omitted for token efficiency). Both are disabled in Aider's repo map call, so not implementing them matches the target behavior.

  Example output:
  ```
  src/server.go:
  │type Server struct {
  │⋮
  │func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
  │⋮
  │func (s *Server) Start() error {
  ```

- **Stage-0+1/2/3 output assembly** (matching Aider candidate-universe semantics). All stages are assembled into a single tag list **first**, then `fitToBudget()` runs on the combined list.
  - Reminder: stage ordering governs selection/trim artifacts; final rendered parity is adjudicated separately using Aider-equivalent `to_tree` sorting/render semantics.
  0. **Special-file prelude (optional)**: Important files from `special.py` parity set, prepended as bare `filename` entries (no trailing colon), excluding chat files and files already represented in the ranked output set. In `parity_mode=true`, preserve Aider prelude ordering semantics: ordering is derived from sorted `other_fnames` input filtered by important-file predicate, not by special-file declaration order.
  1. **Ranked definitions**: Definition tags from ranked `(destination_file, identifier)` pairs after PageRank rank distribution, sorted by rank descending, excluding chat files. This is the primary content.
  2. **Remaining graph nodes**: Files that appear in the graph (have references or definitions) but weren't included in stage 1 — appended in PageRank order as bare `filename` entries (no trailing colon) with no definition tags.
  3. **Remaining other_fnames**: Files from the repo that don't appear in the graph at all (no definitions, no references) — appended as bare `filename` entries (no trailing colon).
     - In `parity_mode=true`, preserve Aider ordering semantics, including comparator-faithful non-deterministic set-iteration characteristics where applicable.
     - In `parity_mode=false`, deterministic alphabetical ordering is allowed as an enhancement.

  The binary search in `fitToBudget()` trims from the end of this combined list, so stage 3 entries are trimmed first, then stage 2, then stage 1 definitions, while stage 0 retains highest priority by prepend order.

`internal/repomap/special.go`:
- Special files list (153 unique patterns from Aider — `codecov.yml` is duplicated in Aider's list): README.md, go.mod, package.json, Cargo.toml, pyproject.toml, Dockerfile, .env.example, etc. Note: Makefile is NOT in Aider's list.
- **Root-scoping + upstream exception**: Core important-file patterns are matched against `filepath.Clean(relPath)` and are root-scoped (`README.md` at root matches; `src/README.md` does not), except where the upstream set defines explicit non-root path rules.
- **Dynamic directory matching (upstream parity exception)**: Preserve the upstream special-file rule for `.yml` files under `.github/workflows/`.
- **Integration constraints**:
  1. Special files come only from `otherFnames` (files not in chat) — never from `chatFiles`
  2. Deduplicated against files already present in the ranked tag output set (not stage-1-only)
  3. Prepended (not appended) — they get priority in budget fitting (stage 0 prelude)
  4. Rendered as bare `filename` entries (no trailing colon) with no code context
  5. In `parity_mode=true`, this stage-0 source rule is strict and must match Aider behavior exactly (no out-of-universe additions).

`internal/repomap/render_test.go` — golden file tests

### 3D: Caching Layer

- **Tag cache**: via `repo_map_file_cache` + `repo_map_tags` tables (Phase 0A). Mtime-validated — only re-parse files with changed mtime. On refresh, the tag extraction phase walks the repo and compares each file's mtime against `repo_map_file_cache`. Unchanged files are skipped entirely (no parse, no tag extraction). Changed files have their tags deleted (`DeleteRepoMapTagsByPath`) and re-extracted. New files are parsed and inserted. Deleted files are pruned (see Phase 0A pruning note). This makes incremental refreshes proportional to the number of changed files, not the repo size.
- **Render cache**: in-memory, keyed by hash of dimensions that affect the output. The key composition depends on refresh mode (matching Aider's behavior):
  - **"auto" mode**: hash of (sorted chatFiles + sorted otherFnames + maxMapTokens + sorted mentionedFnames + sorted mentionedIdents). All personalization dimensions are included because they affect PageRank weighting. Uses cache only when map generation took >1s (matching Aider's `map_processing_time > 1.0` check).
  - **"files" mode**: hash of (sorted chatFiles + sorted otherFnames + maxMapTokens). `mentionedFnames` and `mentionedIdents` are excluded — only the file set matters. Always uses cache (if hit). Users who want stable maps for prompt caching should set `refresh_mode: "files"` explicitly.
  - **"manual" mode**: returns `last_map` directly when present. If no `last_map` exists (cold start), generates once; subsequent regenerations happen on explicit `project:map-refresh` or `map_refresh` tool call.
  - **"always" mode**: no caching (regenerated every turn).
- **Last-good-map cache**: `LastGoodMap(sessionID)` returns the most recently generated map string for a session. This is stored in-memory (not persisted to DB) in a `*csync.Map[string, string]` (session ID → map string), matching the codebase's typed concurrent map pattern (see `agent.go:115-116`). When `Generate()` completes successfully, the result is stored. When `Generate()` times out or fails, the last-good map is returned as a fallback. If no files have changed since the last generation AND the personalization dimensions are identical, `Generate()` returns the cached result immediately without re-running PageRank or rendering.
- **Render cache thread safety**: The render cache (keyed by dimension hash → rendered string) is stored in a `*csync.Map[uint64, renderCacheEntry]` where `renderCacheEntry` holds the map string and token count. Both `csync.Map` instances are safe for concurrent reads and writes from multiple sessions without additional locking.
- Full invalidation on `Refresh()` call.

---

## Phase 4: Integration

Wire repo map into the agent prompt pipeline.

### 4A: Prompt Injection

**Injection mechanism: user+assistant message pair in PrepareStep** (matching Aider's `get_repo_messages()` pattern in `base_coder.py:750-761`).

The repo map is injected as a **user message + assistant acknowledgment pair** appended after the stable system prompt but before chat history in `PrepareStep`. This preserves Anthropic prompt caching — the system prompt remains a stable, always-cached prefix. The repo map pair forms a second cacheable block: apply `cache_control: {"type": "ephemeral"}` on the assistant reply message to create a cache breakpoint. Everything from system prompt through the repo map is cacheable; only the tail (chat history onward) recomputes when the map changes.

**LCM budget visibility**: The repo map is treated as **fixed overhead** in the LCM budget computation — the same treatment as `SystemPromptTokens` and `ToolTokens` in `ComputeBudget()` (`lcm/config.go:21-38`). The repo map token count is subtracted from the context window before computing soft/hard thresholds, so LCM never attempts to evict or shrink the map. No changes to `lcm_context_items` or its CHECK constraint are needed.

Implementation details:
- Add `RepoMapTokens int64` to `BudgetConfig` and include it in the `overhead` calculation alongside `SystemPromptTokens` and `ToolTokens`.
- Add an explicit per-session LCM API (in `internal/lcm/manager.go`) to set repo map overhead tokens and recompute cached/persisted budget thresholds.
  - API contract: `SetRepoMapTokens(ctx context.Context, sessionID string, tokens int64) error` updates in-memory budget cache and persisted `lcm_session_config` thresholds atomically for that session.
- The coordinator updates this overhead from the measured `tokenCount` returned by `repoMapSvc.Generate(...)` after successful generation/injection (or `repoMapSvc.LastTokenCount(sessionID)` for explicit fallback paths where generation is skipped).
- To avoid a first-turn timing hole, run `CompactIfOverHardLimit` again after overhead update within the same request path (or defer assistant step until recomputed budget is applied).
- LCM must use this explicit overhead value (not re-estimation from injected text) to avoid chars-per-token divergence issues.

**Message format** (constructed inside the `buildRepoMapHook` closure in `coordinator_opts.go`):
- Retrieve the current repo map via `svc.LastGoodMap(sessionID)` or generate on first step (see 4B)
- Inject a user+assistant message pair into `prepared.Messages` after the system prompt:
```go
// User message with repo map content
repoMapUserMsg := fantasy.NewUserMessage(repoMapContent)
// Assistant acknowledgment with cache breakpoint (fantasy has no NewAssistantMessage — construct manually)
repoMapAsstMsg := fantasy.Message{
    Role:    fantasy.MessageRoleAssistant,
    Content: []fantasy.MessagePart{fantasy.TextPart{Text: "Ok, I won't try and edit those files without asking first."}},
}
// Cache control via package-level function in internal/agent/ (see hooks.go).
// The method body at agent.go:691-706 never references the receiver — only reads
// os.Getenv("CRUSH_DISABLE_ANTHROPIC_CACHE") and returns static provider options.
// Extracted as cacheControlOptions() in hooks.go; sessionAgent.getCacheControlOptions()
// delegates to it. buildRepoMapHook calls cacheControlOptions() directly — no parameter
// passing, no closure capture, no interface concerns.
repoMapAsstMsg.ProviderOptions = cacheControlOptions()
```

The repo map user message content:
```
Below is a map of the repository showing the most relevant files and their key definitions.
Use this to understand the codebase structure. These files are read-only context — use tools to read full contents when needed.

<repo-map>
[rendered map here]
</repo-map>
```

**Service wiring chain** (full path from app init to prompt injection):
1. `internal/app/app.go` `New()` → calls `app.initRepoMap(ctx, conn)` (in new file `internal/app/repomap.go`) → returns `agent.CoordinatorOption`
2. `internal/agent/coordinator.go` `NewCoordinator(…, opts...)` → applies `WithRepoMap(svc)` option → stores service
3. `coordinator.buildAgent()` → constructs PrepareStepHook closure, passes to `result.SetPrepareStepHooks()`
4. `sessionAgent.Run()` → on first PrepareStep, hook calls `repoMapSvc.Generate(ctx, opts)`
5. PrepareStep callback → hook injects user+assistant pair into `prepared.Messages`

**Unconfigured-app behavior**: when `cfg.IsConfigured()` is false and coder-agent initialization is skipped, repo-map coordinator hook wiring is a no-op for that process path.

**Progressive fallback**:
- `parity_mode=true`: retry chain follows Aider parity semantics when map content is empty/falsy (disjoint/global/unhinted fallback sequence).
- `parity_mode=false`: timeout/error-triggered retry/defer behavior is allowed as an enhancement.

Progressive fallback in parity mode must match Aider exactly:
1. Full attempt: `chat_files = in-chat/read-only set`, `other_files = all_abs_files - chat_files`, with mentioned fnames/idents and caller-provided `ForceRefresh` propagated unchanged (default false in normal runs; true in explicit refresh flows).
2. Reduced (disjoint/global): `chat_files = empty`, `other_files = all_abs_files`, keep mentioned fnames/idents.
3. Minimal (unhinted): `chat_files = empty`, `other_files = all_abs_files`, no mentioned fnames/idents.

Each attempt re-runs PageRank with reduced personalization. In attempts #2 and #3 (`chat_files` empty), parity behavior must also honor Aider's no-chat token-target expansion semantics.
If all 3 fail, return an empty map.

**`force_refresh` parity rule (mandatory):** in `parity_mode=true`, attempt #1 must propagate caller-provided `ForceRefresh` exactly (default false in normal Run path; true only for explicit refresh command/tool flows), matching Aider behavior. Fallback attempts #2 and #3 must omit force-refresh behavior (`ForceRefresh=false`/unset). In `parity_mode=false`, broader refresh strategies are allowed as enhancements.
**Prompt-caching mode coercion source of truth (mandatory):** define a single runtime source of truth for “prompt caching enabled” and use it consistently when coercing effective refresh mode `auto -> files`.
**Comparator placement caveat:** in Aider, `auto -> files` coercion is performed in top-level runtime orchestration (`main.py`) before RepoMap invocation, not inside `repomap.py`. Preserve this control-plane/data-plane separation in parity-mode behavior.
**Map multiplier provenance (mandatory):** parity artifacts must capture `map_mul_no_files_effective`, `map_mul_no_files_source`, and comparator entrypoint so CLI-runtime semantics are auditable per run.

**Status display**: On session start, log repo map status at debug level: `"Repo map: {N} files, {M} definitions, {K} tokens ({refresh_mode} refresh)"`. Visible with `--verbose` but doesn't clutter normal sessions.

**Files to modify (minimized — see Implementation Directive):**

All repo map logic lives in new files; existing upstream files get only minimal extension-point additions.

`internal/agent/agent.go` (~5 lines — upstream-tracked):
- Add `prepareStepHooks` field to `sessionAgent` struct (1 line):
  ```go
  prepareStepHooks []PrepareStepHook
  ```
- Add `SetPrepareStepHooks` method (analogous to existing `SetTools`/`SetSystemPrompt`):
  ```go
  func (a *sessionAgent) SetPrepareStepHooks(hooks []PrepareStepHook) {
      a.prepareStepHooks = hooks
  }
  ```
- Add hook invocation loop in the main `Run()` PrepareStep closure (after `promptPrefix` injection at ~line 289, before assistant message creation). **Critical**: only the main `Run()` PrepareStep (the `agent.Stream` call in `Run()`) gets the hook loop — NOT the `Summarize()` PrepareStep (inside the `Summarize` method's `agent.Stream` call) or the title-generation PrepareStep (in `generateTitle()`):
  ```go
  for _, hook := range a.prepareStepHooks {
      callContext, prepared, err = hook(callContext, call.SessionID, prepared)
      if err != nil { break }
  }
  ```
- **No changes to `SessionAgentOptions`** — hooks are set via `SetPrepareStepHooks()` after construction to keep the coordinator diff minimal — no changes to `SessionAgentOptions` or its three construction sites (`coordinator.go:342`, `common_test.go:156`, `agentic_fetch_tool.go:175`).
- **Add `SetPrepareStepHooks` to the `SessionAgent` interface** — `buildAgent()` in `coordinator.go` returns `SessionAgent` (the interface), not `*sessionAgent`. For `result.SetPrepareStepHooks(hooks)` to compile, the method must be on the interface. Add: `SetPrepareStepHooks(hooks []PrepareStepHook)` to the `SessionAgent` interface definition (~1 line in agent.go).

**New file** `internal/agent/hooks.go`:
```go
// PrepareStepHook is called during the main Run() PrepareStep to allow
// injecting additional messages (e.g., repo map) into the prepared message list.
type PrepareStepHook func(ctx context.Context, sessionID string, prepared fantasy.PrepareStepResult) (context.Context, fantasy.PrepareStepResult, error)

// cacheControlOptions returns provider options for Anthropic prompt caching.
// Extracted from sessionAgent.getCacheControlOptions() (agent.go:691-706) — the method
// body never references the receiver, only reads os.Getenv("CRUSH_DISABLE_ANTHROPIC_CACHE")
// and returns static provider options. sessionAgent.getCacheControlOptions() delegates to
// this package-level function. buildRepoMapHook in coordinator_opts.go calls it directly.
func cacheControlOptions() fantasy.ProviderOptions { ... }
```

`internal/agent/coordinator.go` (~6-8 lines — upstream-tracked):
- Add `repoMapSvc` field to `coordinator` struct (1 line). The field type is a narrow interface (defined in coordinator_opts.go) to avoid importing `internal/repomap` directly — only the `WithRepoMap` option constructor imports it.
- Add `...CoordinatorOption` variadic parameter to `NewCoordinator` signature (1 line):
  ```go
  func NewCoordinator(ctx context.Context, cfg *config.Config, ..., lspManager *lsp.Manager, lcm lcm.Manager, extraTools []fantasy.AgentTool, opts ...CoordinatorOption) (Coordinator, error) {
  ```
- Add option-apply loop in `NewCoordinator` body after struct initialization (3 lines):
  ```go
  for _, opt := range opts {
      opt(c)
  }
  ```
- **extra tools gating fix (mandatory):** preserve existing `extraTools` behavior independent of whether the built-in `agent` tool is enabled. Do not couple appending unrelated `extraTools` to the `agent`-tool allow check.
- In `buildAgent()`, construct the PrepareStepHook and pass to the agent (~3-4 lines):
  ```go
  if c.repoMapSvc != nil {
      hooks := buildRepoMapHook(c.repoMapSvc, c.filetracker, c.lcm, c.cfg.WorkingDir())
      result.SetPrepareStepHooks(hooks)
  }
  ```
- **Guard data source (mandatory):** the per-Run injection guard must not derive identity from `prepared.Messages`. Capture guard inputs in `Run()` from durable local data:
  - `RootUserMessageID`: DB ID returned by the initial `createUserMessage()` for that root `Run()`.
  - `QueueGeneration`: monotonic per-session generation incremented when queued prompts are promoted into a new recursive `Run()`.
  The hook consumes a precomputed `RunInjectionKey` from context; no message-ID extraction from fantasy messages is permitted.

**New file** `internal/agent/coordinator_opts.go`:
```go
// CoordinatorOption configures optional coordinator behavior.
type CoordinatorOption func(*coordinator)

// WithRepoMap enables repo map generation for agent sessions.
func WithRepoMap(svc RepoMapService) CoordinatorOption {
    return func(c *coordinator) { c.repoMapSvc = svc }
}

// RepoMapService is the interface required by the coordinator for repo map integration.
// Defined here (consumer-side) to localize the repomap import to this file, keeping
// upstream-tracked agent.go and coordinator.go unchanged. This is a deliberate exception
// to the codebase convention where service interfaces are defined producer-side
// (session.Service, message.Service, etc.) — the tradeoff is justified by minimizing
// upstream diffs. Do not treat this as a precedent for new service interfaces.
//
// This file must import internal/repomap for GenerateOpts and the helper functions
// (ExtractCurrentMessageText, ExtractMentionedFnames, ExtractIdents, IdentFilenameMatches)
// called in buildRepoMapHook. Do not duplicate GenerateOpts — the import is already
// required and duplication adds maintenance burden with zero benefit.
type RepoMapService interface {
    Available() bool
    Generate(ctx context.Context, opts repomap.GenerateOpts) (string, int, error)
    LastGoodMap(sessionID string) string
    LastTokenCount(sessionID string) int
    AllFiles(ctx context.Context) []string
    SessionReadOnlyFiles(ctx context.Context, sessionID string) []string // persisted repo-map/session read-only rel paths
    ShouldInject(sessionID string, runKey repomap.RunInjectionKey) bool // lock-protected atomic check-and-set for per-Run() guard
    RefreshAsync(sessionID string, opts repomap.GenerateOpts)
    Refresh(ctx context.Context, sessionID string, opts repomap.GenerateOpts) (string, int, error)
    Reset(ctx context.Context, sessionID string) error
    Close() error
}

// buildRepoMapHook constructs the PrepareStepHook that injects the repo map.
// All repo map logic (generation, timeout, fallback, mention extraction) lives here,
// not in agent.go.
func buildRepoMapHook(svc RepoMapService, ft filetracker.Service, lcmMgr lcm.Manager, rootDir string) []PrepareStepHook {
    // ... hook implementation with full generation logic, timeout, fallback,
    // and lcmMgr.SetRepoMapTokens updates ...
}
```

`internal/app/app.go` (~8-10 lines — upstream-tracked):
- Add `repoMapOpt agent.CoordinatorOption` field to `App` struct (1 line).
- Add `repoMapSvc *repomap.Service` field to `App` struct (1 line) — needed for explicit `Close()` in `Shutdown()`.
- In `New()`, call `app.initRepoMap(ctx, conn)` and store the result (2 lines):
  ```go
  app.repoMapOpt = app.initRepoMap(ctx, conn)
  ```
  Note: `conn *sql.DB` is a local variable in `New()` — it is NOT stored on the `App` struct. The `initRepoMap` method receives it as a parameter.
- In `InitCoderAgent()`, pass the option to `NewCoordinator` (1-2 lines — append to existing call):
  ```go
  app.AgentCoordinator, err = agent.NewCoordinator(ctx, ..., app.LSPManager, app.lcmManager, lcm.ExtraAgentTools(app.lcmManager), app.repoMapOpt)
  ```
  When `repoMapOpt` is nil (disabled), the variadic `...CoordinatorOption` receives zero options — no special nil handling needed.
- In `Shutdown()`, add explicit `repoMapSvc.Close()` call **before** the existing `cleanupFuncs` loop (~2 lines):
  ```go
  if app.repoMapSvc != nil { app.repoMapSvc.Close() }
  ```

**New file** `internal/app/repomap.go`:
```go
// initRepoMap creates the repo map service and returns a CoordinatorOption to wire it in.
// Returns nil if repo map is disabled in config.
func (app *App) initRepoMap(ctx context.Context, conn *sql.DB) agent.CoordinatorOption {
    if app.config.Options.RepoMap == nil || app.config.Options.RepoMap.Disabled {
        return nil
    }
    q := db.New(conn)
    // Pass ctx as the lifecycle context — inherits from app.globalCtx (signal-aware via fang),
    // so it cancels on SIGINT. In-flight RefreshAsync goroutines are interrupted — partial
    // results are discarded and the next startup re-indexes from scratch.
    svc := repomap.NewService(app.config, q, conn, app.config.WorkingDir(), ctx)
    // Do NOT register svc.Close() in cleanupFuncs — cleanupFuncs run concurrently with
    // conn.Close(), but svc.Close() must complete before conn.Close() (it drains background
    // goroutines that may hold DB connections). Instead, call svc.Close() explicitly in
    // App.Shutdown() before the cleanupFuncs loop. See shutdown coordination below.
    app.repoMapSvc = svc
    go svc.PreIndex()
    return agent.WithRepoMap(svc)
}
```

**Shutdown coordination**: `svc.Close()` must complete before `conn.Close()`. Register it as an explicit pre-cleanup call in `App.Shutdown()` alongside the existing `AgentCoordinator.CancelAll()`, **not** in `cleanupFuncs` (which run concurrently with `conn.Close()`):
```go
func (app *App) Shutdown() {
    if app.AgentCoordinator != nil { app.AgentCoordinator.CancelAll() }
    if app.repoMapSvc != nil { app.repoMapSvc.Close() } // waits for background goroutines before DB close
    // ... existing cleanupFuncs loop ...
}
```
The `Service` tracks async operations with a **service-level WaitGroup** independent from parser-pool internals.
- `RefreshAsync`/`PreIndex` increment service WG on launch and decrement on completion.
- `Close()` sequence is mandatory: set `closing=true` -> stop accepting new async work -> wait service WG -> close parser pool/query/tree caches.
- Parser-pool holder tracking remains internal to parser implementation and must not share wait-state with enqueue paths.
Both `PreIndex` and `RefreshAsync` use `s.lifecycleCtx` internally — no context parameter is needed on their signatures.

**Synchronization**: `PreIndex` signals completion via a `sync.WaitGroup` (or `chan struct{}` closed on completion) stored on the `Service`. `AllFiles(ctx)` blocks on this signal before returning, ensuring callers never see an empty file list due to a race. On cancellation, the signal fires with whatever partial results were collected — `AllFiles(ctx)` returns those partial results (or returns immediately on `ctx.Done()`) rather than blocking indefinitely.

### 4B: Dynamic Refresh

**Refresh modes** (4 supported, matching Aider modes):
- **auto** (default):
  - `parity_mode=true`: follow Aider auto semantics for cache behavior (including `map_processing_time > 1s` cache usage behavior).
  - **RecursionError latch parity (mandatory):** in `parity_mode=true`, if map generation hits a recursion overflow equivalent to Aider's `RecursionError` handling, set an equivalent one-way disable latch for repo-map generation for the process/session scope used by comparator parity (Aider sets `max_map_tokens = 0`). Enhancement profile may implement safer alternative recovery but must not be used for parity scoring.
  - `parity_mode=false`: additional staleness/file-change triggers are allowed as enhancements.
- **files**: regenerate only when the file set changes. Excludes `mentionedFnames` and `mentionedIdents` from the render cache key, producing a more stable map that improves prompt cache hit rates. Always uses render cache.
- **Prompt-cache parity rule**: when prompt caching is enabled and `refresh_mode` is `auto`, coerce effective mode to `files` (matching Aider behavior).
- **manual**: return `last_map` when available; if absent (cold start), generate once. Later regenerations occur on explicit `project:map-refresh` command or `map_refresh` tool call.
- **always**: regenerate every turn unconditionally

Trigger points:
1. **On app startup** — pre-index all files in background (see 4A wiring). Map is likely ready by first prompt.
2. **On session start** — generate initial map (fast if pre-index completed)
3. **After tool calls that modify files** (edit, write, bash) — schedule async refresh (auto/always modes)
4. **On new user message** — extract mentioned identifiers and mentioned filenames, update personalization

**Per-Run() injection guard:** PrepareStep runs on every agent step (5-15+ times per user message during tool calls). Repo map generation must only run once per `Run()` invocation — not on every step. The guard MUST use a **RunInjectionKey** captured from the Run path, not fantasy messages.

`RunInjectionKey` contract:
- `RootUserMessageID`: the DB message ID returned by `createUserMessage()` for that Run.
- `QueueGeneration`: monotonic per-session generation incremented when queued prompts are promoted into a new recursive `Run()`.

**Run() write-point requirement (mandatory):** in `sessionAgent.Run()`, construct `RunInjectionKey` immediately after obtaining `RootUserMessageID` and determining `QueueGeneration`, then store it in context via `repomap.WithRunInjectionKey(ctx, key)` before any PrepareStep hook execution. Hooks must only read this precomputed key via `RunInjectionKeyFromContext`.

Implementation note: `ShouldInject(sessionID, runKey)` must be implemented as a lock-protected atomic check-and-set in the service to avoid race windows under concurrent calls.

This handles recursive `sessionAgent.Run()` paths correctly: queued prompts (`agent.go:561-568`) and post-summarize re-queue (`agent.go:540-554`) both produce a new run key (new root user message and/or queue generation), so the hook fires exactly once per Run. Putting the guard on the coordinator would miss these paths since they never re-enter `coordinator.Run()`.

No extraction from `prepared.Messages` is allowed for guard identity. The hook consumes a precomputed `RunInjectionKey` from context. The `RepoMapService` interface exposes `ShouldInject(sessionID string, runKey repomap.RunInjectionKey) bool` — an atomic check-and-set that returns `true` on the first call for a given run key and `false` on subsequent calls within the same `Run()`.

**Concurrent refresh deduplication:** use two singleflight scopes (from `golang.org/x/sync/singleflight`):
- **Index/build scope** keyed by `repo_key` for expensive repo-wide extraction/index refresh work.
- **Map-generation scope** keyed by `repo_key + session_id + profile + personalization_hash + token_budget` for render generation.
This prevents cross-session personalization bleed while still collapsing redundant concurrent work.

**Async generation with last-good cache:**

`internal/agent/coordinator_opts.go` (inside `buildRepoMapHook`, the PrepareStepHook closure):
```go
// This logic lives entirely in coordinator_opts.go, NOT in agent.go.
// The hook receives (ctx, sessionID, prepared) and returns modified prepared messages.
// buildRepoMapHook must accept lcm.Manager so it can persist repo map token overhead:
// func buildRepoMapHook(svc RepoMapService, ft filetracker.Service, lcmMgr lcm.Manager, rootDir string) []PrepareStepHook
if !svc.Available() {
    return ctx, prepared, nil // no-op
}

// Retrieve the precomputed run key for the per-Run() injection guard.
// ShouldInject atomically checks and sets: returns true on first call for a given
// run key, false on subsequent calls within the same Run().
runKey, ok := repomap.RunInjectionKeyFromContext(ctx)
if !ok {
    return ctx, prepared, fmt.Errorf("missing run injection key")
}
if !svc.ShouldInject(sessionID, runKey) {
    return ctx, prepared, nil // already injected for this Run()
}

// Adaptive timeout: 15s if no cached map exists (cold start),
// 5s if a recent map is available (warm path with fallback).
lastMap := svc.LastGoodMap(sessionID)
timeout := 15 * time.Second
if lastMap != "" {
    timeout = 5 * time.Second
}
genCtx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()

// Get chat files from filetracker (passed to buildRepoMapHook at construction time).
chatFiles, err := ft.ListReadFiles(genCtx, sessionID)
if err != nil {
    // Log and continue without repo map
}
// Normalize absolute paths from filetracker to relative paths for the graph.
// Relpath failure parity rule: if filepath.Rel fails (e.g., cross-volume/cross-mount),
// preserve the cleaned original absolute path instead of dropping the entry.
relChatFiles := make([]string, 0, len(chatFiles))
for _, f := range chatFiles {
    if rel, err := filepath.Rel(rootDir, f); err == nil {
        relChatFiles = append(relChatFiles, rel)
    } else {
        relChatFiles = append(relChatFiles, filepath.Clean(f))
    }
}

currentRunMessages := repomap.ExtractCurrentRunMessages(prepared.Messages)
mentionText := repomap.ExtractCurrentMessageText(currentRunMessages)
allRepoFiles := svc.AllFiles(genCtx) // cached from tag extraction walk; returns partial results or immediate return on ctx cancellation per Service contract
readOnlyRelPaths := svc.SessionReadOnlyFiles(genCtx, sessionID) // source-of-truth: persisted session repo-map state
inChatOrReadOnly := repomap.UnionPaths(relChatFiles, readOnlyRelPaths)
addableRepoFiles := repomap.SubtractFiles(allRepoFiles, inChatOrReadOnly)
mentionedFnames := repomap.ExtractMentionedFnames(mentionText, addableRepoFiles, inChatOrReadOnly)
mentionedIdents := repomap.ExtractIdents(mentionText)
identMatches := repomap.IdentFilenameMatches(mentionedIdents, allRepoFiles)
mentionedFnames = append(mentionedFnames, identMatches...)
slices.Sort(mentionedFnames)
mentionedFnames = slices.Compact(mentionedFnames)

mapStr, tokenCount, err := svc.Generate(genCtx, repomap.GenerateOpts{...})
if errors.Is(err, context.DeadlineExceeded) {
    // Fall back to cached map, schedule async refresh.
    mapStr = lastMap
    svc.RefreshAsync(sessionID, opts) // RefreshAsync schedules goroutine internally (uses s.lifecycleCtx)
    slog.Info("Building repository map in background...")
}
// Persist repo map overhead for LCM budget computation.
// Coordinator updates LCM's per-session fixed overhead from repo map token count.
// LCM must consume this explicit overhead (not text re-estimation).
if tokenCount > 0 && lcmMgr != nil {
    lcmMgr.SetRepoMapTokens(ctx, sessionID, int64(tokenCount))
}

```

- Use `filetracker.Service` (passed to `buildRepoMapHook` at construction time, not via `sessionAgent` which has no such field) for chat files list
- Pass `lcm.Manager` into `buildRepoMapHook` so the hook can persist per-session repo map token overhead via `SetRepoMapTokens`

**Mentioned filename and identifier extraction** (matching Aider's approach in `base_coder.py` — `get_ident_mentions` at line 678, `get_ident_filename_matches` at line 684, and `get_file_mentions` at line 1714; `get_repo_map` at lines 709-748 calls these):

**Parity caveat:** Aider identifier extraction uses set semantics (`set(re.split(...))`), so parity fixtures should normalize/dedupe identifier extraction outputs accordingly and tolerate empty-token artifacts produced by split behavior.

Four functions produce two outputs (`mentionedFnames` and `mentionedIdents`) for `GenerateOpts`. All four live in `internal/repomap/mentions.go`:

0. **`ExtractCurrentMessageText(messages []fantasy.Message) string`** —
   - `parity_mode=true`: mention source text must follow Aider-equivalent **`cur_messages` concatenation semantics** (current-run message bundle, not full prepared-history concatenation).
   - `parity_mode=false`: a stricter last-user-turn window may be used as an enhancement.

   **Supporting helper (mandatory):** `ExtractCurrentRunMessages(prepared []fantasy.Message) []fantasy.Message` must isolate the current Run() message bundle from the prepared stack so parity-mode mention extraction never consumes full-history context.

1. **`ExtractMentionedFnames(text string, addableRepoFiles []string, inChatOrReadOnlyFiles []string) []string`** — Extract filenames from user message text (Aider parity scope):
   - Build candidate file set from **addable files** (repo files excluding in-chat and read-only files).
   - Read-only source-of-truth is explicit: use `repo_map_session_read_only` persisted state (new DB table) loaded through `RepoMapService.SessionReadOnlyFiles(ctx, sessionID)`; do not infer read-only from `filetracker`.
   - Split text on whitespace into words
   - Strip trailing sentence punctuation (`,.!;:?`) from each word
   - Strip surrounding quotes (`"'` `` ` `` `*_`) from each word
   - **Full path matching**: For each addable repo relative path (normalized to `/`), check if it appears in the word set → exact match
   - **Unique basename matching**: Collect basenames that contain at least one of `.`, `_`, `-`, `/`, or `\` (to avoid matching plain words like "run" or "make"). For each such basename, if it uniquely maps to exactly one addable repo file AND appears in the word set, include that file.
  - **Parity rule**: if that basename already exists in in-chat/read-only files for the session, do not auto-add from basename mention (matches Aider `get_file_mentions` behavior).

2. **`ExtractIdents(text string) []string`** — Extract identifier tokens: split on non-word characters (`[^a-zA-Z0-9_]+`), return all tokens. No minimum length (matching Aider — graph weighting de-prioritizes common short identifiers).

3. **`IdentFilenameMatches(idents []string, allRepoFiles []string) []string`** — Bridge identifiers to filenames:
   - Build map: lowercase file stem (≥5 chars) → set of filenames from `allRepoFiles`
   - For each ident ≥5 chars: if `strings.ToLower(ident)` matches a stem key, add those files
   - The ≥5 char threshold prevents short common words from triggering false matches

**Combined flow in `buildRepoMapHook`** (inside `coordinator_opts.go`, which imports `internal/repomap`):
```go
// IMPORTANT: parity_mode=true must derive mention text from current-message payload
// semantics (Aider cur_messages equivalent), not from full prepared stack.
currentRunMessages := repomap.ExtractCurrentRunMessages(prepared.Messages)
mentionText := repomap.ExtractCurrentMessageText(currentRunMessages)
allRepoFiles := svc.AllFiles(genCtx)
mentionedFnames := repomap.ExtractMentionedFnames(mentionText, addableRepoFiles, inChatOrReadOnly)
mentionedIdents := repomap.ExtractIdents(mentionText)
identMatches := repomap.IdentFilenameMatches(mentionedIdents, allRepoFiles)
// Union: mentionedFnames includes both direct mentions and ident-based matches
for _, f := range identMatches {
    mentionedFnames = append(mentionedFnames, f)
}
slices.Sort(mentionedFnames)
mentionedFnames = slices.Compact(mentionedFnames)
```

**Manual refresh — command + tool exposure model**:
- **Project custom command** `project:map-refresh`: user-facing command implemented as `${data_directory}/commands/map-refresh.md` (matching current custom-command loading). It should request map refresh and report result.
- **Project custom command** `project:map-reset`: user-facing command entrypoint in `${data_directory}/commands/map-reset.md` that invokes the internal reset handler; clears repo-map cache state and triggers full rebuild.
  - **Execution contract (mandatory):** because project custom commands are markdown content by default, `project:map-reset` requires explicit internal command routing/plumbing to call reset directly rather than only emitting prompt text.
- **Agent tool** `map_refresh`: callable by the LLM when the map is stale. Wire in `internal/agent/tools/` following existing tool registration pattern. **Enabled in all refresh modes**; in `always` mode, perform immediate regenerate or return deterministic no-op/already-fresh status.
- **`map_reset` execution policy (mandatory, explicit):** `map_reset` is command-only in normal operation. `project:map-reset` invokes the internal reset path directly. If a `map_reset` tool symbol exists for compatibility, it must deterministically deny non-command invocations.
- **Tool exposure integration (required):** `map_refresh` must land in all three layers:
  1. tool implementation in `internal/agent/tools/`,
  2. runtime construction/registration in coordinator tool construction path,
  3. built-in allowlist inclusion (`allToolNames`) plus disabled-tools tests.
  Do not expose unrestricted `map_reset` model invocation.

---

## Phase 5: Polish

### 5A: Performance

- String interning for common identifiers (e.g., `identifier`, `type_identifier`, frequent variable names) to reduce memory pressure from repeated tag storage
- Parser pool sizing tuning based on profiled concurrent usage patterns
- Note: parallel parsing and incremental mtime-based updates are already implemented in Phase 1A (parser pool) and Phase 3A.1 (tag extraction). This phase focuses on optimization beyond the initial implementation.

### 5B: Expanded Data Format Explorers

Data format explorers (JSON, YAML, CSV, XML, HTML, TOML, INI) stay regex/stdlib-parser based — tree-sitter doesn't improve these. Add missing formats from Volt:

- Markdown (879 lines in Volt) — frontmatter, headings, code blocks, reading time
- LaTeX (779 lines in Volt) — sections, environments, bibliographies
- SQLite (384 lines in Volt) — tables, indexes
- Logs (629 lines in Volt) — error/warning counts, log level distribution

**B4 per-format semantic minima (mandatory):**
- `parity_mode=true` (Volt baseline):
  - Markdown: frontmatter parse (when present), heading hierarchy summary, code-block language histogram, link/reference counts.
  - LaTeX: section/subsection counts, environment inventory, bibliography metadata.
  - SQLite: table/index inventory, per-table column summaries, sample-row summaries.
  - Logs: level distribution, timestamp pattern detection, representative error/warning sampling.
  - Quantitative pass criteria (applies to each required format): required-field coverage `100%`, micro F1 `>= 0.90`, macro F1 `>= 0.86`, numeric/count MAPE `<= 0.10` when applicable.
- `parity_mode=false` (exceed track):
  - LaTeX: add label/reference extraction and bibliography/citation counts.
  - SQLite: add view/trigger inventory and constraint extraction.
  - Logs: add repeated error-signature aggregation.
  - Quantitative pass criteria (applies to each required format): required-field coverage `100%`, micro F1 `>= 0.94`, macro F1 `>= 0.90`, numeric/count MAPE `<= 0.05` when applicable.

### 5C: tsaudit Tool

Implement `internal/cmd/tsaudit/` as described in the Dynamic Language Support Tracking section above. This is a dev tool, not shipped in the release binary. It tracks three sources — Aider's primary `tree-sitter-language-pack/` directory, Aider's fallback `tree-sitter-languages/` directory, and grammar-bundled `queries/tags.scm` files — against the `languages.json` manifest. Complements the bash script `scripts/gen-treesitter-deps.sh` (used for initial setup and major version bumps). Add `task tsaudit` targets to `Taskfile.yaml` (described in the language support commands section).

### 5D: Schema + Documentation

- Run `task schema` to regenerate JSON schema with `RepoMapOptions`
- Update `internal/config/AGENTS.md` with repo map config documentation

### 5E: Target-Parity Validation Suite (Blocking)

Add explicit parity checks against both reference projects. This phase is required before declaring completion.

- **Aider parity fixtures** (`internal/repomap/testdata/parity_aider/`):
  - same input repo snapshots evaluated by Crush and Aider comparator via pinned CLI/runtime entrypoint (`aider_main_cli`)
  - direct `../aider/aider/repomap.py` invocation is diagnostic-only and cannot be used for parity adjudication
  - enforce Gate A thresholds (A1–A6) from the quantitative section above
  - fixture composition is frozen per scoring cycle with explicit minima for corpus size/diversity and fallback-case inclusion; update requires protocol/version bump and full rerun
  - capture and store per-fixture metrics as JSON artifacts for offline project verification review
  - persist comparator provenance in each artifact:
    - `aider_commit_sha`
    - parity fixture corpus hash (`fixtures_sha256`)
    - comparator path used (`../aider`)
    - comparator tuple members (`grep_ast_provenance`, `tokenizer_id`, `tokenizer_version`)
  - include explicit file-universe fixtures and assert source selection by profile:
    - `parity_mode=true`: `git_tracked_filtered` (tracked+staged with Aider-equivalent ignore/subtree filtering) / `inchat_fallback`
    - `parity_mode=false`: may additionally exercise `walker_fallback`
  - include mention-extraction fixtures that verify basename suppression when basename is already in chat/read-only (Aider parity behavior).
  - include explicit read-only parity fixtures asserting Aider-compatible chat-set construction: repo read-only files are unioned into `chat_files`, excluded from `other_files`/addable universe, and remain non-addable via basename auto-mention paths.
  - read-only fixture setup/verification MUST use `repo_map_session_read_only` as the authoritative source (no inference from filetracker or prompt text).
  - include mention-source fixtures that verify parity-mode extraction against Aider-equivalent current-message concatenation semantics.
- include explicit parity fixtures for capture-loop quirk policy, RecursionError disable-latch behavior, and relpath-failure fallback behavior.
- **Volt parity fixtures** (`internal/lcm/explorer/testdata/parity_volt/`):
  - representative files across supported code/data formats
  - enforce Gate B thresholds (B1–B5) from the quantitative section above
  - fixture composition is frozen per scoring cycle with explicit minima by language/format family and required negative-fixture classes; update requires protocol/version bump and full rerun
  - include explicit negative fixtures (malformed files, unsupported language) to verify graceful fallback behavior
  - persist comparator provenance in each artifact:
    - `volt_commit_sha`
    - parity fixture corpus hash (`fixtures_sha256`)
    - comparator path used (`../volt`)
  - include runtime-path coverage matrix results for each in-scope large-content ingestion/retrieval path declared in this plan.
- include the concrete runtime-path inventory artifact (`runtime_ingestion_paths.v1.json`, ingestion+retrieval scoped) and fail the suite if runtime-discovered in-scope paths differ from inventory (missing or unexpected entries).
  - Freeze `runtime_ingestion_paths.v1.json` for the scoring cycle; mid-cycle changes require protocol/version bump and full rerun.
- include explicit retrieval-path assertions (`lcm_describe`, `lcm_expand`) for both persisted and non-persisted exploration outcomes and for session-lineage access control outcomes (self/ancestor allowed, unrelated denied).
- for parity adjudication, evaluate retrieval authorization using Volt-strict semantics: session-lineage scope plus sub-agent-only requirement for `lcm_expand`. Any enhancement-profile variants must be scored separately and recorded as non-parity behavior.
- include comparator-caveat fixtures where scoped retrieval queries may omit exploration fields; parity harness must distinguish true non-persistence from column-selection omissions.
- **Explorer-family parity/exclusion matrix (mandatory, scored):** maintain `internal/lcm/explorer/testdata/parity_volt/explorer_family_matrix.v1.json` with rows:
  - Freeze `explorer_family_matrix.v1.json` for the scoring cycle; mid-cycle changes require protocol/version bump and full rerun.
  - `family`, `parity_required` (bool), `allowed_exclusion` (bool), `exclusion_rationale`, `required_evidence`, `score_weight`, `threshold`.
  - Must include explicit decisions for code, JSON/YAML/TOML/INI/XML/HTML, Markdown, LaTeX, SQLite, Logs, binary, shell, PDF, image, executable, and archive families.
  - Any family marked `parity_required=true` without passing evidence is a hard failure.
- Add gating targets in `Taskfile.yaml`:
  - `task test:parity` runs both parity suites in `parity_mode=true` and fails on any threshold breach.
  - `task test:exceed` runs enhancement-profile exceed checks in `parity_mode=false`.
  - parity target includes a fixed Volt-coverage matrix check (explorer family parity/exclusion table) so breadth claims are scored, not inferred.
- Completion requirement (parity baseline): feature is considered parity-complete only when `task test:parity` passes and parity artifacts are recorded locally for fork sign-off.
- Exceed certification requirement: any claim of meet/exceed behavior requires `task test:exceed` pass artifacts from the same protocol cycle in addition to parity sign-off artifacts.
- **Fork-scope execution rule:** all parity/exceed gates in this plan are local task runs (`task ...`), with no CI/CD pipeline dependency required for sign-off.
- **Local-only sign-off authority (normative):** local artifact bundles are authoritative for this fork; CI/CD results are non-authoritative and cannot substitute for local parity sign-off evidence.
- **Atomic sign-off bundle (mandatory):** local fork sign-off must produce a single bundle manifest containing comparator SHAs/paths, fixture hashes, protocol version, inventory/matrix versions and hashes, deterministic flags/toggles, and A1–A6/B1–B5 outcomes from the same scoring run.
- Flake policy: parity runs use deterministic seed and fixed fixture snapshots; retries are not used to pass parity gates.
- **B1 scoring protocol (required):** parity harness defines and version-controls:
  - gold annotation schema for symbols/import categories/visibility,
  - matching rules (exact/normalized name matching, scope handling, unsupported-language handling),
  - aggregation method (micro and macro averages, both reported),
  - per-language minimum floors and fixture weighting,
  - minimum sample-size requirements,
  - per-language capability mapping for visibility scoring (`full`, `export-only`, `none`) and explicit denominator rules for N/A capability classes.
  Any change to scoring protocol must bump protocol version in parity artifacts.
  - Protocol must also define concrete numeric minima for fixture composition and language/format-family coverage before first scoring run of a cycle.
- **B1 protocol freeze point:** protocol version is frozen before first gate-scoring run for a release cycle; changing protocol during a scoring cycle invalidates earlier scores and requires full rerun.
- **Deterministic scoring mode requirement:** parity scoring runs must disable LLM/agent enhancement tiers and execute deterministic static parsing/formatting only; non-deterministic runs are invalid for gate scoring.
- **Deterministic enforcement requirement:** parity artifacts must include explicit flags proving deterministic mode (`parity_mode=true`, enhancement tiers disabled, fixed seed/config), and scoring tests must fail if any `+llm`/`+agent` explorer tier appears in recorded outputs.
- **Parity preflight requirement (mandatory, local):** before `task test:parity`, run a local preflight that validates comparator tuple and corpus readiness (`aider_commit_sha`, `grep_ast_provenance`, `tokenizer_id`, `tokenizer_version`, fixture hashes, profile object fields). Missing preflight inputs is a hard fail and parity scoring must not proceed.

---

## Dependency Graph

```
Phase 0A (config + DB) ──┬──────────────────────────────────┐
Phase 0B (stub types)  ──┼──────────────────────────────────────────────> Phase 5C (tsaudit)
Phase 0C (blocker gate) ─┴──> Phase 1A, 2A, 3A.0
                               v                                  v
Phase 1A (parser: Go + Python) ──> Phase 1B (remaining 37 languages)
                     │    │
                     │    ├──> Phase 2A (TS explorer) ──> Phase 2B (heuristics) ──> Phase 2C (formatting) ──> Phase 2D (runtime wiring)
                     │    │                                                                                               │
                     │    │                                                                                               v
                     │    │                                                                                           Phase 5E
                     │    v
                     │    Phase 3A.0 (vertical slice) ──> Phase 3A.1 (tags + graph) ──> Phase 3B (PageRank) ──> Phase 3C (budget + render) ──> Phase 3D (cache)
                               │                                                                                                       │
                               │ (also depends on 0A directly for DB tables)                                                            v
                                                                                                                        Phase 4A (prompt injection) ──> Phase 4B (refresh)
                                                                                                                                                         │
                                                                                                                                                         v
                                                                                                                                               Phase 5 (polish)
```

**Parallelizable:**
- Phase 3 can proceed concurrently with Phase 1B+2 once Phase 1A is done.
- Phase 2A requires Phase 1A (not 1B — unsupported languages can fall through to regex explorers during incremental rollout).
- Phase 1B language additions are embarrassingly parallel (remaining 37 languages; 35 unique modules in Phase 1B scope).
- Phase 5C (tsaudit) can start any time after Phase 0B.

**Hard dependencies:**
- Phase 0C must pass before 1A/2A/3A.0.
- Phase 3A.0 must pass before 3A.1+.
- Phase 2D must pass before Phase 5E sign-off.
- Phase 3A.1 depends on both Phase 1A (parser) and Phase 0A (DB tables) directly.
- B1 scoring protocol definition (Phase 5E) must be finalized before running/parsing Volt parity gate results.
- Tool allowlist integration for `map_refresh` and command-path integration for `project:map-reset` must land before manual-refresh feature sign-off.
- Path-root normalization contract (`cfg.WorkingDir` vs `os.Getwd`) must be finalized before 4A/4B integration tests.


---

## Critical Files Reference

**Upstream-tracked files (minimize changes — see Implementation Directive):**

| File | Role | ~Lines modified |
|------|------|-----------------|
| `internal/config/config.go` | Options struct + merge delegation | ~5 |
| `internal/config/load.go` | setDefaults() for RepoMapOptions | ~4 |
| `internal/fsext/ls.go` | Optional parity-support filtering hook in `shouldIgnore` path (authoritative ignore semantics) | ~0-5 |
| `internal/fsext/fileutil.go` | Optional delegation/wrapper touch only if needed after `ls.go` hook | ~0-2 |
| `internal/agent/agent.go` | prepareStepHooks field + setter + loop + SessionAgent interface method | ~6 |
| `internal/agent/coordinator_test.go` | Update `dummyAgent` to satisfy extended `SessionAgent` interface | ~2-4 |
| `internal/agent/coordinator.go` | repoMapSvc field + variadic opts + hook wiring | ~6-8 |
| `internal/app/app.go` | repoMapOpt + repoMapSvc fields + initRepoMap call + option pass-through + Shutdown() | ~8-10 |
| `internal/lcm/config.go` | Add `RepoMapTokens` to `BudgetConfig`, include in overhead math | ~3-5 |
| `internal/lcm/manager.go` | Add per-session repo map overhead setter + budget recompute wiring | ~20-35 |
| `internal/lcm/message_decorator.go` | Wire runtime explorer persistence for large stored content + LCM option gating | ~30-55 |
| `internal/filetracker/service.go` | Normalize paths against configured working dir (not implicit cwd) | ~10-18 |
| `Taskfile.yaml` | CGO_ENABLED + tsaudit tasks + parity task | ~18 |
| `go.mod` / `go.sum` | Tree-sitter grammar modules | ~38 / ~80+ |

**Non-upstream files (bulk of implementation):**

| File | Role |
|------|------|
| `internal/agent/hooks.go` | PrepareStepHook type definition + cacheControlOptions() package-level function |
| `internal/agent/coordinator_opts.go` | CoordinatorOption, WithRepoMap, RepoMapService interface, buildRepoMapHook |
| `internal/app/repomap.go` | initRepoMap() — service creation, cleanup, background PreIndex |
| `internal/config/repomap.go` | RepoMapOptions type, merge(), DefaultRepoMapOptions() |
| `internal/lcm/explorer/explorer.go` | Explorer interface, registry, dispatch — modify for tree-sitter (not on upstream) |
| `internal/lcm/explorer/code.go` | Existing regex explorers — replaced by tree-sitter (not on upstream) |
| `internal/lcm/explorer/extensions.go` | Extension-to-language mapping (not on upstream) |
| `internal/config/lcm.go` | LCMOptions pattern to follow (not on upstream) |
| `internal/db/migrations/` | Migration pattern to follow |
| `internal/db/sql/` | sqlc query pattern to follow |
| `internal/filetracker/` | Tracks files read per session — feeds chat files to repo map |
| `scripts/gen-treesitter-deps.sh` | Generates `go get` commands from Aider + tree-sitter-language-pack |

## New Packages

| Package | Purpose |
|---------|---------|
| `internal/treesitter/` | Parser, cache, query runner, embedded `*-tags.scm` files, languages.json |
| `internal/repomap/` | Tags, graph, PageRank, budget, render, cache, `mentions.go` (filename/ident extraction) |
| `internal/lcm/explorer/stdlib/` | Per-language stdlib module lists for import categorization |
| `internal/lcm/explorer/runtime.go` | Runtime adapter for decorator-driven exploration persistence |
| `internal/cmd/tsaudit/` | Dev tool for tracking language support drift |
| `internal/repomap/parity_test.go` | Aider parity threshold harness (A1–A6) |
| `internal/lcm/explorer/parity_test.go` | Volt parity threshold harness (B1–B5) |

## Verification

After each phase:

1. **Phase 0**: `go test ./internal/config/... -run TestConfigMerging && task sqlc && go test ./internal/db/... && go test ./internal/treesitter/...` (includes types package tests: `TestTagString`, `TestSymbolInfoFields`, `TestImportCategoryValues`)
2. **Phase 1**: `go test ./internal/treesitter/...` — verify tag extraction per language against Aider's capture convention (`@name.definition.*` → def, `@name.reference.*` → ref), benchmark parse times, verify extension-to-language mapping golden file, verify custom predicates (`#strip!`, `#set-adjacent!`) are gracefully ignored, and verify parity-mode output does not include legacy paired-node-derived emissions
3. **Phase 2**: `go test ./internal/lcm/explorer/...` — verify tree-sitter explorer produces >= regex quality output, golden file comparison (including Go files — tree-sitter must match or exceed former `GoExplorer` quality)
4. **Phase 3**: `go test ./internal/repomap/...` — PageRank convergence, budget fitting, render golden files, mention extraction tests (`mentions_test.go` covering `ExtractCurrentMessageText`, `ExtractMentionedFnames`, `ExtractIdents`, `IdentFilenameMatches`), plus concurrency tests for lock-protected `ShouldInject` check-and-set
5. **Phase 4**: **End-to-end integration test** — small Go + TypeScript project fixture (5-10 files each) → walk → parse → tag → graph → rank → render → golden file comparison. Also: start Crush with repo map enabled, verify map appears as injected user+assistant pair in `prepared.Messages` (prompt path), verify refresh behavior by profile, verify `AllFiles(ctx)` is bounded by timeout context, verify LCM token budget accounts for repo map tokens.
6. **Phase 5**: `go test ./internal/treesitter/... -bench=.` — parallel parsing benchmarks. `go run ./internal/cmd/tsaudit/ verify` — language support drift detection against Aider + grammar-bundled tags.scm sources. Data format explorer golden file tests for new formats (Markdown, LaTeX, SQLite, Logs).
7. **All phases**: `task lint && task test`
8. **Parity completion criteria**: `task test:parity` must pass before feature is considered complete (covers Aider + Volt parity suites).
9. **Repo-map token safety gate (profile-scoped)**:
   - `parity_mode=true`: assert comparator acceptance behavior using `parityTokenCount`.
   - `parity_mode=false`: assert `safetyTokenCount <= TokenBudget` under tokenizer-backed counting and fallback counting with the configured safety margin.
10. **Threshold conformance report**: persist verification metrics snapshots for A1–A6 and B1–B5; any metric below threshold is a hard failure.
11. **Comparator provenance requirement**: every parity artifact must include comparator commit SHA (`aider_commit_sha` or `volt_commit_sha`), comparator path, fixture corpus hash (`fixtures_sha256`), and parity comparator tuple members where applicable (`grep_ast_provenance`, `tokenizer_id`, `tokenizer_version`). Missing provenance is a hard failure.
12. **Blocker-gate requirement**: feature cannot be marked complete unless Phase 0C and Phase 3A.0 gates are recorded as passed in test artifacts.
13. **Runtime-path coverage requirement (B3)**: parity artifacts must explicitly list a versioned ingestion-path inventory and include pass/fail results for each listed path (success and explicit failure-path assertions).
14. **B1 protocol requirement**: parity artifacts must include B1 scoring protocol version, per-language floors, fixture weighting metadata, and minimum sample-size metadata.
15. **Tool exposure requirement**: parity/integration tests must verify `map_refresh` implementation, coordinator construction visibility, allowlist visibility, and disabled-tools behavior; and verify `project:map-reset` command-path behavior (including deterministic denial for non-command `map_reset` compatibility entrypoints, if present).
16. **Deterministic scoring requirement**: parity scoring runs must explicitly disable LLM/agent enhancement tiers and prove comparator-normalized identical outputs across repeated unchanged-input runs (`parity_mode=true`); enhancement-profile determinism checks may additionally require byte-identical raw outputs where declared.
17. **Retrieval scope requirement**: parity/integration tests must verify `lcm_describe` and `lcm_expand` enforce session-lineage access boundaries (self/ancestor allowed, unrelated denied). In `parity_mode=true`, scoring for `lcm_expand` must use Volt-strict authorization (lineage scope + sub-agent-only caller requirement). Any non-Volt agent-type policy is enhancement-only and must be recorded separately.
18. **Atomic bundle requirement**: parity sign-off must include a single local manifest capturing comparator provenance, fixture hashes, protocol/inventory versions, toggles, and gate outcomes from one scoring run.
19. **Retrieval-state requirement (B3/2D/5E)**: parity/integration tests must verify `lcm_describe` behavior for both persisted exploration outcomes (non-null `exploration_summary` and `explorer_used`) and non-persisted outcomes (fields remain null with explicit no-summary response), per inventoried runtime ingestion path.
