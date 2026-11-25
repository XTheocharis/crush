# Acceptance traceability — original specification to work and evidence

This matrix is a planned verification ledger seed, **all implementation assertions are NOT_RUN**. Original [UMCA specification](reference/umca.md) retained byte-identically. Link to its section/line plus exact owner fixture and applicable gate. IDs refer to tests specified in owner documents, not existing executable tests. All A–I remain mandatory. At execution QA expands each row to actual test names, expected/observed result, SHA/environment/command/log, PASS/FAIL/BLOCKED/NOT_RUN, reviewer and residual risk. See [gate contracts](GATE_CONTRACTS.md) for split tests. A missing later assertion cannot pass an earlier gate by implication.

## All 31 §7 checklist requirements

| Spec line | Exact requirement | Gate / fixture IDs |
|---|---|---|
| L167 | Identical text in two messages retains distinct event identities. | G1 / HST-01, STO-01 |
| L168 | Equal-timestamp messages receive deterministic ordering. | G1 / HST-01, STO-01 |
| L169 | Interleaved parallel results remain associated with their original assistant step. | G4 / HST-02,03 |
| L170 | Interrupted and synthetic results retain their capture and outcome status. | G3–4 / STO-02, HST-03,05 |
| L174 | Current input and folded user corrections survive compaction. | G4 / HST-03 |
| L175 | Live projections survive repeated PrepareStep reconstruction. | G4 / HST-03 |
| L176 | A long single autonomous turn can compact settled groups. | G4 / HST-02,03 |
| L177 | The summary request itself stays within its applicable budget. | G4 / HST-04 |
| L178 | Failed or truncated summaries never become active. | G3–4 / STO-04, HST-04 |
| L179 | Repeated reduction terminates when no safe progress is possible. | G4 / HST-04,05 |
| L183 | The request guard remains active after model refresh. | G4; G7 / HST-05, PRV-05 |
| L184 | Configured output limits agree with planner reserves. | G4; G7 / HST-04, PRV-05 |
| L185 | Provider prefixes and tool schemas are counted. | G4; G7 / HST-04, PRV-02,05 |
| L186 | Unsupported media or continuation formats produce explicit compatibility behavior. | G4; G7 / HST-03,04,07, PRV-05 |
| L187 | Overflow recovery does not repeat previous tool execution. | G4; G7 / HST-05, PRV-06 |
| L191 | Cancellation works while planning, summarizing and resuming. | G4 / HST-06,07 |
| L192 | Queued RunIDs complete exactly according to host semantics. | G1; G4 / HST-01,02,05 |
| L193 | Manual and automatic compaction cannot conflict. | G4 / HST-06, OPS-04 |
| L194 | Channel responses are not duplicated. | G4 / HST-02,06 |
| L195 | Shutdown drains database-dependent work before releasing SQLite. | G3–4; G7 / STO-07, HST-07, DST-08 |
| L199 | Deleting a source invalidates its derived summaries and memories. | G3; G6; G7 / STO-06, MEM-01,03, QA-X02,04 |
| L200 | Recall-derived tool results cannot restore deleted sources. | G5; G7 / REC-04, QA-X02 |
| L201 | Legacy summary pointers are not left active after invalidation. | G3 / STO-05,06 |
| L202 | Missing, partial and corrupted artifacts are marked correctly. | G2 / CAP-01,03,04,05 |
| L203 | Retention limits remain enforced during long-running processes. | G2; G7 / CAP-03,06, DST-08 |
| L204 | Concurrent access cannot publish conflicting checkpoints. | G3 / STO-03,04 |
| L208 | Tool output cannot become authoritative user instructions. | G4; G6 / HST-03, MEM-02 |
| L209 | Subagent and channel recall cannot access unauthorized sessions. | G5; G7 / REC-02, HEL-02, PER-02 |
| L210 | Disabling learning prevents stale jobs from publishing. | G6 / MEM-03, OPS-05 |
| L211 | Superseded preferences are not recalled as current. | G6 / MEM-01,02 |
| L212 | User deletion and correction propagate to applicable search results. | G5–7 / REC-01,04, MEM-01,05, PER-03 |

## Additional contract and negative fixtures

| Source | Coverage / boundary | Gate and fixture |
|---|---|---|
| §§2–4 host/transaction | Existing messages and native accepted-run identity, no second transcript; group snapshots, exact version/digest/coverage, scope before use; provider output cap and signed state. | G1/G3/G4; HST-01–07, STO-01–05, PRV-05/06 at G7 |
| §5 and §6 PR2 | Earliest producer before first drop, binary/split UTF-8 and partial ranges (missing middle), aggregate quota/ENOSPC/permission/short write/rename/close, restart, native exit and flags-off deletion, scope guessed ID. | G2 CAP-01–06, OPS-01/02, STO minimal closure |
| §6 PR3 | True two connections vs one pooled connection; compact/delete and competing checkpoint; one-connection injected failure each write/read/cleanup, prior checkpoint usable iff still eligible, legacy foreign/missing/same timestamp/non-summary target. | G3 STO-02–07, QA-X01 |
| §6 PR4 | One actual run two parallel tools, pending sibling excluded; smaller next request, one terminal; original protected/folded input and marker exact recovery; signed/provider/subagent variants and crash/unknown tool outcome. | G4 HST-02–07, OPS-03 core/04; repeat G5/G6/G7 |
| §6 PR5/§7 negatives | Same block ID across scopes, ordinary FTS insertion no rebuild and separate preexisting-row backfill, expired/tampered cursor, huge result budget, deletion of sole original and tool-generated descendants; native/connected status. | G5 REC-01–06, STO-06 applicable closure, OPS-03, QA-X02 |
| §6 PR6 | Manual edits/export ownership, current supersession/review/forget, independent flags/permission, stale worker cannot relearn. | G6 MEM-01–04, OPS-05; F1 export/import and remote at G7 |
| §7 compatibility and retention | Idle mode changes, explicit incompatible fallback, prior legacy pointer invalidation; downgrade proven on disposable copy or explicit filtered export; no claim of forensic erasure. | G3/G4 STO-05, OPS-04; G7 QA-X08 |
| §7 delivery coverage | Native fixtures, actual Go build/lint/generator/race, matched disabled/enabled measurements, every PR migration/consent/status/metrics/rollback/fixtures. | All gates; U14 actual target ledger |
| §§8–10 | Historical XRush regressions and all 20 reference repositories' source/fixtures adapted to Crush, exact pins and licenses; not wholesale imports. | Per unit ref receipt and AUD, QA final |

## All advanced acceptance streams and shared boundaries

| Requirement | Work / full enabled-path fixtures |
|---|---|
| §11 A multi-level context, pressure, local/cross-store rollover | A1/A2 HIE-01–06; G7 QA-X04/05, DST-07 and OPS-06 |
| §11 B observations/reflection/evidence/as-of | B1 MEM-05–07; G7 QA-X02/03/04, present revocation |
| §11 C embeddings, hybrid/rerank and external index | C1/C2 VEC-01–06; G7 real pgvector, real enabled embedding adapter, labeled retrieval evaluation, QA-X03 |
| §11 D prefetch/read-only helper and MCP | D1/D2 HEL-01–06; G7 actual client negotiation, native parity, scope and no side effects, QA-X03/09 |
| §11 E dynamic catalog/cache/usage | E1 PRV-01–04; G7 schema changes, signed bytes, real wire, no invented cache hit, total purpose costs |
| §11 F personal grant/export/import | F1 PER-01–06; G7 offline denial, current/as-of, remote invalidation, grant revocation and edited export |
| §11 G provider expansion and conformance | G7P PRV-05–06 and PRV-01–04; actual selected adapter matrix incl all required families, incompatibility behavior not counted as supported |
| §11 H backend/workers/durability | H1/H2 DST-01–08; G7 actual SQLite/Postgres/broker and cross-store consumers, partition/clock/unknown receipt/cutover/backup, QA-X05/06/08 |
| §11 I four rewind modes | I1/I2/I3 REW-01–07; G7 actual native/connected entry, rooted file journal, external conflicts, next provider call, fork/recall/as-of, lost receipt, QA-X04/07 |
| §11 shared: streaming source → checkpoint | G3 STO-02/04, QA-X01; G4 actual projection |
| §11 shared: request → derivative → delete | G5 REC-04, QA-X02; G6 memories, G7 all external/rewind derivatives |
| §11 shared: time, authority, audit | G7 QA-X03; VEC-05/06, HEL-01/02, PER-03/04/06, DST-05/08, PRV-06, OPS-07 |
| §11 shared: fenced rollover and migration | G7 QA-X05/06; HIE-06, DST-02/03/04/06/07, REW-07 |
| §11 shared: release evidence/user benefit | G7 QA-X09, OPS-06/07; U14 exact commit full behavior/metrics/rollback/consent/degraded path |

**Status semantics:** NOT_RUN is not a defect in this planning artifact. A failed or unavailable required fixture is a release blocker, not an exemption. Partial implementation may be delivered only with accurate capability status, safe disabled fallback and admin access; full implementation requires every applicable row passing on actual selected target and relevant enabled/disabled/outage states.
