# Go context reading map — full-source assignments

Exact source: [GO_AGENT_CONTEXT.md](GO_AGENT_CONTEXT.md), 6630 logical lines, SHA-256 `ab11c958f30ca11198b0fc5d030668eb76e48111008bc735475b5e26121a4424`. All 248 unified rule blocks indexed. Ranges below are inclusive, 1-based, for this exact digest. The complete source remains authoritative. This index is a locator, never a replacement for reading the full text.

## Mandatory reading protocol

ORCH and AUD read lines 1–6630 in full before U00/first audit (complete chunks, record hash and coverage; re-read affected blocks on resume). For each other role: read all common sections below, all additional sections and all rule blocks selected in that role’s exact full reading ranges. The map includes the entire version/tool/stdlib catalogs and conflict registry; focus API selection on the actual packages, never assume they are dependencies. Read the full assigned workflow/playbooks and embedded reference code files, including tests/module declarations. If scope expands, add task tags and read the complete corresponding rules/workflows/sections. Read the full relevant source even when a tool truncates; split reads into manageable chunks, record actual completed ranges, do not count truncated output. `GO_CONTEXT_INDEX.json` provides precise machine-readable ranges and rule IDs. Use source line numbers rather than fictitious paths for embedded source sections. A reading manifest is required evidence, not proof of comprehension; independent review checks actual implementation.

```sh
sha256sum "$PLAN_DIR/GO_AGENT_CONTEXT.md"
sed -n '153,180p' "$PLAN_DIR/GO_AGENT_CONTEXT.md" # Example: read complete workflow chapter.
```

## Common full sections (all agents)

These apply in addition to task-specific assignments. Common task tags: analysis, api-design, architecture, build-constraints, concurrency, context, dependencies, errors, generation, language, modules, security, testing, types.

- `Non-negotiable operating contract` — 12–24.
- `Exact API and tool introspection protocol` — 25–56.
- `Suggested repository fact sheet` — 57–77.
- `Bundle baseline and version axes` — 78–103.
- `Task focus map` — 104–150.
- `implementation/00-agent-workflow.md` — 153–180.
- `implementation/01-language.md` — 181–221.
- `implementation/02-values-types-memory.md` — 222–251.
- `implementation/03-projects-toolchains.md` — 252–284.
- `implementation/04-architecture-and-apis.md` — 285–321.
- `implementation/05-errors-context-lifetime.md` — 322–357.
- `implementation/06-concurrency.md` — 358–397.
- `implementation/12-testing-and-analysis.md` — 534–563.
- `implementation/13-debugging-and-troubleshooting.md` — 564–598.
- `implementation/15-security-and-supply-chain.md` — 634–661.
- `implementation/16-build-release-operation.md` — 662–689.
- `implementation/17-platforms-and-external-domains.md` — 690–727.
- `implementation/decisions.md` — 801–882.
- `policy/authority-and-conflicts.md` — 885–902.
- `policy/version-resolution.md` — 903–929.
- `policy/project-inspection.md` — 930–944.
- `policy/language-foundations.md` — 945–966.
- `policy/package-and-api-design.md` — 967–976.
- `policy/interfaces-generics-reflection.md` — 977–994.
- `policy/errors-and-context.md` — 995–1014.
- `policy/concurrency.md` — 1015–1036.
- `policy/architecture-and-module-layout.md` — 1049–1067.
- `policy/dependencies-and-supply-chain.md` — 1068–1079.
- `policy/testing-fuzzing-coverage.md` — 1080–1091.
- `policy/static-analysis.md` — 1092–1103.
- `policy/security.md` — 1132–1152.
- `policy/logging-and-observability.md` — 1153–1164.
- `policy/documentation.md` — 1165–1174.
- `policy/generated-code.md` — 1175–1184.
- `policy/public-api-and-releases.md` — 1185–1196.
- `policy/build-platforms-cgo-unsafe.md` — 1197–1208.
- `policy/domain-overlays.md` — 1209–1220.
- `policy/source-selection.md` — 1221–1236.
- `policy/novel-problem-resolution.md` — 1237–1252.
- `policy/validation.md` — 1253–1275.
- `implementation/workflows/existing-change.md` — 3359–3373.
- `implementation/workflows/validation.md` — 3485–3511.
- `playbooks/existing-project.md` — 3893–3938.
- `Release-sensitive behavior catalog` — 4762–4903.
- `Modern idiom availability` — 4904–4924.
- `Go command/tool interface index` — 4925–5030.
- `Tool catalog` — 5031–5047.
- `Standard-library package landscape` — 5048–5239.
- `Resolved guidance conflicts and stale-source traps` — 5240–5509.
- `End-of-task completion gate` — 6628–6630.

Opening provenance/purpose: 1–11. All full assigned ranges below already include common rules/sections. Embedded examples are not imported into Crush. Exact selected dependency APIs still must be checked.

## HST

Projection and actual provider attempts: immutable snapshots/copy semantics; explicit source order; context/cancel/join; preserve signed wire segments, per-attempt admission and existing Fantasy APIs.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/race.md` — 4491–4535.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/latency.md` — 4212–4256.
- `playbooks/memory-growth.md` — 4257–4301.
- `playbooks/compile-or-type-failure.md` — 3621–3666.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 237 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, debugging, dependencies, errors, generation, http-networking, language, logging-observability, modules, performance-profiling-pgo, profiling, runtime-gc, security, serialization, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–475, 502–727, 801–882, 885–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2603, 2620–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3026–3057, 3060–3123, 3132–3155, 3158–3223, 3244–3269, 3280–3287, 3314–3321, 3359–3387, 3442–3454, 3485–3527, 3621–3666, 3757–3802, 3893–3938, 4076–4120, 4212–4301, 4491–4535, 4717–5509, 5610–5965, 6051–6302, 6463–6630.

## STO

Existing SQLite/sqlc and authoritative Tx ownership; parameter values, Rows.Err/Close, checked commit, flush outside Tx, real driver isolation/ambiguous commit, exact generated artifacts.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/race.md` — 4491–4535.
- `playbooks/generator-drift.md` — 4031–4075.
- `playbooks/module-resolution.md` — 4302–4349.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/failing-test.md` — 3939–3984.
- `playbooks/contention.md` — 3667–3711.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.

Rule selection: 238 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, concurrency, context, debugging, dependencies, errors, generation, io-filesystems, language, logging-observability, modules, operations, profiling, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 476–727, 801–882, 885–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3026–3057, 3084–3123, 3140–3155, 3158–3223, 3226–3241, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3485–3527, 3667–3711, 3757–3802, 3893–3984, 4031–4075, 4302–4349, 4491–4579, 4762–5509, 6051–6302, 6463–6630.

## CAP

Earliest byte capture; n>0 before err, short writes/flush/close, quotas and rooted paths, subprocess descendants/platform cancellation; preserve native exit and representation.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/08-commands-processes.md` — 426–447.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/race.md` — 4491–4535.
- `playbooks/cross-platform-build-failure.md` — 3712–3756.
- `playbooks/memory-growth.md` — 4257–4301.
- `playbooks/allocation-pressure.md` — 3530–3574.
- `reference/portable/cmd/wordcount/main.go` — 5966–6015.
- `reference/portable/cmd/wordcount/main_test.go` — 6016–6050.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.

Rule selection: 239 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, cli-processes, compression, concurrency, context, debugging, dependencies, errors, generation, io-filesystems, language, logging-observability, modules, platforms, profiling, security, testing, text-unicode, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–447, 534–727, 801–882, 885–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2507, 2516–2531, 2540–2611, 2620–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3026–3057, 3060–3107, 3116–3123, 3132–3147, 3158–3223, 3226–3241, 3244–3269, 3272–3287, 3304–3329, 3359–3387, 3442–3454, 3485–3527, 3530–3574, 3712–3756, 3893–3938, 4076–4120, 4257–4301, 4491–4579, 4762–5509, 5966–6058, 6303–6630.

## REC

Exact query/encoding and range contracts; bound FTS fallback/cursors and literal syntax; scope before ranking and expansion; real DB and malformed cursor/parser fuzz evidence.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `playbooks/failing-test.md` — 3939–3984.
- `playbooks/generator-drift.md` — 4031–4075.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 230 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, coverage, crypto-tls, dependencies, errors, fuzzing, generation, io-filesystems, language, modules, security, serialization, sql, testing, text-unicode, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 476–727, 801–882, 885–1103, 1118–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2507, 2516–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3060–3083, 3100–3107, 3132–3147, 3158–3223, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3442–3454, 3485–3511, 3893–3984, 4031–4075, 4580–4624, 4717–5509, 5610–5816, 6051–6462, 6628–6630.

## MEM

Attributed immutable evidence, typed state and time; background ownership/stop, explicit egress, real transaction/invalidation and stale publication tests.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `playbooks/race.md` — 4491–4535.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `playbooks/failing-test.md` — 3939–3984.
- `playbooks/security-finding.md` — 4580–4624.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.

Rule selection: 233 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, concurrency, context, debugging, dependencies, errors, generation, http-networking, language, logging-observability, modules, operations, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–598, 634–727, 801–882, 885–1117, 1132–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2441, 2450–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3026–3057, 3108–3115, 3140–3155, 3158–3223, 3226–3241, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3485–3527, 3893–3984, 4076–4120, 4491–4535, 4580–4624, 4717–5509, 6051–6302, 6463–6630.

## HIE

Concrete ordered hierarchy and hysteresis; runtime timer semantics, budgeted worker admission, durable rollover fences and source authorization, representative measurement.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/race.md` — 4491–4535.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/latency.md` — 4212–4256.
- `playbooks/memory-growth.md` — 4257–4301.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.

Rule selection: 234 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, dependencies, errors, generation, http-networking, language, logging-observability, modules, profiling, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–727, 801–882, 885–1103, 1118–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2441, 2450–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3026–3033, 3050–3057, 3060–3107, 3116–3123, 3132–3147, 3158–3223, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3442–3454, 3485–3527, 3757–3802, 3893–3938, 4076–4120, 4212–4301, 4491–4535, 4717–5509, 6051–6302, 6463–6630.

## VEC

Bound vectors/top-k, deterministic ties, numeric/NaN/encoding and generation policy; selected PG/pgvector/embedding contracts; real DB, revocation, performance evidence.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/allocation-pressure.md` — 3530–3574.
- `playbooks/latency.md` — 4212–4256.
- `playbooks/performance-regression.md` — 4444–4490.
- `playbooks/memory-growth.md` — 4257–4301.
- `playbooks/race.md` — 4491–4535.
- `playbooks/dependency-addition.md` — 3803–3847.
- `playbooks/security-finding.md` — 4580–4624.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 237 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, dependencies, errors, fuzzing, generation, http-networking, language, logging-observability, modules, profiling, runtime-gc, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–727, 801–882, 885–1103, 1118–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3026–3033, 3050–3057, 3060–3123, 3132–3155, 3158–3223, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3442–3454, 3485–3527, 3530–3574, 3803–3847, 3893–3938, 4212–4301, 4444–4535, 4580–4624, 4762–5509, 5610–5965, 6051–6058, 6303–6630.

## HEL

Restricted helper/MCP capabilities; bounded HTTP/stream/JSON, read-only authority, cancellation and joins, selected MCP protocol/transport, preserve native tool permission.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/08-commands-processes.md` — 426–447.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/11-serialization.md` — 502–533.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/new-service.md` — 3424–3441.
- `playbooks/http-network-issue.md` — 4167–4211.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/race.md` — 4491–4535.
- `playbooks/security-finding.md` — 4580–4624.
- `reference/current/cmd/countd/main.go` — 5514–5564.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 227 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, cli-processes, concurrency, context, crypto-tls, dependencies, errors, fuzzing, generation, http-networking, language, logging-observability, modules, operations, security, serialization, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–475, 502–598, 634–727, 801–882, 885–1117, 1132–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2441, 2450–2603, 2620–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3108–3115, 3140–3155, 3158–3223, 3226–3241, 3244–3269, 3280–3287, 3314–3321, 3359–3387, 3424–3441, 3485–3527, 3757–3802, 3893–3938, 4076–4120, 4167–4211, 4491–4624, 4762–5509, 5514–5564, 5610–5965, 6051–6058, 6463–6630.

## PRV

Provider SDK/model/cache wire and retries: exact selected adapter, replayability and cancellation, actual attempt/output escape, preserve v1 wire and measure usage not estimated hits.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/http-network-issue.md` — 4167–4211.
- `playbooks/latency.md` — 4212–4256.
- `playbooks/performance-regression.md` — 4444–4490.
- `playbooks/dependency-addition.md` — 3803–3847.
- `playbooks/module-resolution.md` — 4302–4349.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 237 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, crypto-tls, dependencies, errors, fuzzing, generation, http-networking, language, logging-observability, modules, operations, profiling, security, serialization, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–475, 502–727, 801–882, 885–1103, 1118–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2441, 2450–2603, 2620–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3026–3033, 3050–3057, 3060–3123, 3132–3155, 3158–3223, 3226–3241, 3244–3269, 3280–3287, 3314–3321, 3359–3387, 3442–3454, 3485–3527, 3803–3847, 3893–3938, 4167–4256, 4302–4349, 4444–4490, 4580–4624, 4717–5509, 5610–5965, 6051–6302, 6463–6630.

## PER

Live origin grant/revocation, archive/path/expansion limits, exact JSON presence/encoding, credentials and authorization, idempotent export/import and fail-closed offline behavior.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/race.md` — 4491–4535.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 225 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, compression, concurrency, context, crypto-tls, dependencies, errors, fuzzing, generation, http-networking, io-filesystems, language, logging-observability, modules, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–598, 634–727, 801–882, 885–1117, 1132–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3140–3147, 3158–3223, 3244–3269, 3280–3287, 3314–3321, 3332–3345, 3359–3387, 3485–3527, 3893–3938, 4491–4624, 4717–5509, 5610–5816, 6051–6462, 6628–6630.

## DST

Selected PG/RabbitMQ delivery/ack/lease/fence/outbox and commit ambiguity; lifecycle, bounded admission, real restart/partition/restore tests and reproducible service versions.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/release-and-operate.md` — 3471–3484.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `playbooks/dependency-addition.md` — 3803–3847.
- `playbooks/dependency-upgrade.md` — 3848–3892.
- `playbooks/generator-drift.md` — 4031–4075.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/race.md` — 4491–4535.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/contention.md` — 3667–3711.
- `playbooks/http-network-issue.md` — 4167–4211.
- `playbooks/module-resolution.md` — 4302–4349.
- `playbooks/security-finding.md` — 4580–4624.
- `reference/current/cmd/countd/main.go` — 5514–5564.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 245 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, concurrency, context, crypto-tls, debugging, dependencies, errors, generation, http-networking, io-filesystems, language, logging-observability, modules, operations, platforms, profiling, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–425, 448–727, 801–882, 885–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3026–3057, 3060–3123, 3132–3155, 3158–3223, 3226–3241, 3244–3269, 3272–3287, 3304–3329, 3332–3345, 3359–3387, 3442–3454, 3471–3527, 3667–3711, 3757–3938, 4031–4120, 4167–4211, 4302–4349, 4491–4624, 4762–5509, 5514–5564, 5610–5965, 6051–6302, 6463–6630.

## REW

Exact root/path/mode/link/rename/durability semantics per OS; write intents, filesystem/DB receipt, recovery ownership and checked compensation, preserve dirty/staged work.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/08-commands-processes.md` — 426–447.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `playbooks/cross-platform-build-failure.md` — 3712–3756.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/race.md` — 4491–4535.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/unexpected-output-or-state.md` — 4717–4761.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.

Rule selection: 233 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, cli-processes, concurrency, context, crypto-tls, debugging, dependencies, errors, fuzzing, generation, io-filesystems, language, logging-observability, modules, platforms, security, serialization, sql, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–447, 476–598, 634–727, 801–882, 885–1117, 1132–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2507, 2516–2635, 2638–2671, 2674–2827, 2830–2885, 2894–2973, 2976–3023, 3026–3057, 3140–3147, 3158–3223, 3226–3241, 3244–3269, 3272–3287, 3304–3329, 3332–3345, 3359–3387, 3485–3527, 3712–3802, 3893–3938, 4491–4624, 4717–5509, 6051–6630.

## OPS

Preserve Cobra/Bubble Tea/workspace config; typed DTO/JSON old-new compatibility, status/stdout/stderr, scoped endpoints, lifecycle and canonical schema generator.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/08-commands-processes.md` — 426–447.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/11-serialization.md` — 502–533.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `implementation/workflows/new-command.md` — 3388–3405.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/release-and-operate.md` — 3471–3484.
- `playbooks/generator-drift.md` — 4031–4075.
- `playbooks/cross-platform-build-failure.md` — 3712–3756.
- `playbooks/strict-lint-failure.md` — 4669–4716.
- `playbooks/http-network-issue.md` — 4167–4211.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/failing-test.md` — 3939–3984.
- `reference/current/cmd/countd/main.go` — 5514–5564.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/cmd/wordcount/main.go` — 5966–6015.
- `reference/portable/cmd/wordcount/main_test.go` — 6016–6050.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 233 complete blocks selected by tags `analysis, api-design, architecture, build-constraints, cli-processes, concurrency, context, dependencies, errors, fuzzing, generation, http-networking, io-filesystems, language, logging-observability, modules, operations, platforms, publishing, security, serialization, testing, time, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–475, 502–598, 634–727, 801–882, 885–1117, 1132–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2611, 2620–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3108–3115, 3124–3131, 3140–3155, 3158–3223, 3226–3241, 3244–3269, 3272–3287, 3304–3329, 3359–3405, 3471–3511, 3712–3756, 3893–3984, 4031–4075, 4167–4211, 4580–4624, 4669–4716, 4762–5509, 5514–5564, 5610–6058, 6628–6630.

## QA

Independent selected-version evidence, real boundaries not mocks, target matrix and reproducibility, scoped fuzz/race, generated drift and vulnerability finding gate; no fabricated pass.

Additional full sections beyond common:
- `implementation/07-io-files-text-time.md` — 398–425.
- `implementation/08-commands-processes.md` — 426–447.
- `implementation/09-http-and-networking.md` — 448–475.
- `implementation/10-databases.md` — 476–501.
- `implementation/11-serialization.md` — 502–533.
- `implementation/14-performance.md` — 599–633.
- `policy/io-networking-and-data.md` — 1037–1048.
- `policy/diagnostics-and-debugging.md` — 1104–1117.
- `policy/performance-profiling-pgo.md` — 1118–1131.
- `implementation/workflows/database-change.md` — 3332–3345.
- `implementation/workflows/external-integration.md` — 3374–3387.
- `implementation/workflows/workers-and-streams.md` — 3512–3527.
- `implementation/workflows/performance-change.md` — 3442–3454.
- `implementation/workflows/release-and-operate.md` — 3471–3484.
- `implementation/workflows/diagnose-defect.md` — 3346–3358.
- `playbooks/failing-test.md` — 3939–3984.
- `playbooks/race.md` — 4491–4535.
- `playbooks/deadlock.md` — 3757–3802.
- `playbooks/goroutine-leak.md` — 4076–4120.
- `playbooks/resource-leak.md` — 4536–4579.
- `playbooks/generator-drift.md` — 4031–4075.
- `playbooks/cross-platform-build-failure.md` — 3712–3756.
- `playbooks/strict-lint-failure.md` — 4669–4716.
- `playbooks/security-finding.md` — 4580–4624.
- `playbooks/performance-regression.md` — 4444–4490.
- `playbooks/static-analysis-finding.md` — 4625–4668.
- `reference/current/cmd/countd/main.go` — 5514–5564.
- `reference/current/service/handler.go` — 5618–5706.
- `reference/current/service/handler_test.go` — 5707–5816.
- `reference/current/service/serve.go` — 5817–5865.
- `reference/current/service/serve_test.go` — 5866–5965.
- `reference/portable/cmd/wordcount/main.go` — 5966–6015.
- `reference/portable/cmd/wordcount/main_test.go` — 6016–6050.
- `reference/portable/patterns/patterns.go` — 6059–6121.
- `reference/portable/patterns/patterns_test.go` — 6122–6168.
- `reference/portable/patterns/syntax.go` — 6169–6246.
- `reference/portable/patterns/syntax_test.go` — 6247–6302.
- `reference/portable/textstats/example_test.go` — 6303–6316.
- `reference/portable/textstats/stats.go` — 6317–6379.
- `reference/portable/textstats/stats_test.go` — 6380–6462.
- `reference/portable/work/map.go` — 6463–6526.
- `reference/portable/work/map_test.go` — 6527–6627.
- `reference/portable/go.mod` — 6051–6058.
- `reference/current/go.mod` — 5610–5617.

Rule selection: 248 complete blocks selected by tags `analysis, api-design, architecture, benchmarking, build-constraints, cgo, cli-processes, concurrency, context, coverage, crypto-tls, debugging, dependencies, errors, fuzzing, generation, http-networking, io-filesystems, language, logging-observability, modules, operations, pgo, platforms, profiling, publishing, runtime-gc, security, serialization, sql, testing, time, tracing, types`. Exact IDs and their individual bounds are in `GO_CONTEXT_INDEX.json`.

**Read all these full ranges (common + additional + rule blocks):** 1–150, 153–727, 801–882, 885–1275, 1282–1469, 1472–1761, 1764–2041, 2044–2071, 2074–2203, 2206–2431, 2434–2635, 2638–2671, 2674–2827, 2830–2973, 2976–3023, 3026–3057, 3060–3155, 3158–3223, 3226–3241, 3244–3269, 3272–3329, 3332–3387, 3442–3454, 3471–3527, 3712–3802, 3893–3984, 4031–4120, 4444–4716, 4762–5509, 5514–5564, 5610–6630.

## ORCH and AUD

Read full 1–6630 including every handbook, policy, rule, workflow, diagnostic playbook, version/tool/package catalog, stale-source trap and embedded reference package. Audit roles reopen the audited agent’s complete assigned ranges and newly relevant blocks at each review; verify exact selected versions separately.

## Reading evidence

For each unit record document SHA-256, role, assigned ranges, actually read ranges (including full additional blocks), rule IDs checked, task tags, selected API/version/target facts, unresolved questions, and evidence paths. Save this in `go-alignment.md` and machine-readable `report.json`; if reading is interrupted, checkpoint last verified complete line and resume. Do not claim complete source consumption if context/tool truncation prevented it. The supplied projection is complete; the separate canonical corpus/source package is not bundled.
