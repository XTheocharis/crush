# UMCA for Crush: architecture, implementation contract and acceptance specification

**Target:** charmbracelet/crush at `0b0c79e32ada5c1e5ba26180023be740f0530f7b`. **Historical comparison:** XTheocharis/crush at `dc96baac1c4c25bd3bcd164cea030d90fc3977e9`. **Status:** proposed design and acceptance criteria. This document is the consolidated implementation specification. References to the original brief, proposal and audit explain design history; their contents are not required to implement the contract. The reference archive contains source snapshots, not necessarily their authors’ current releases.

**Full delivery scope:** §§2–7 specify the shared foundation. §11 specifies all advanced functionality, including implementation dependencies, enabled paths, failure behavior and acceptance tests. §§8–10 provide historical evidence.

## 1. Outcome and design authority

A long-running tool-using session must survive a context reduction without executing a completed action twice, losing an outstanding tool result, dropping an active instruction, or exposing another session’s data. Within an authorized scope, a user must be able to inspect what was captured, recover retained material within its retention policy, and delete it along with its attributable derivatives. A summary may lose nuance even when every retained original is recoverable. Offer exact expansion and distinguish observed, inferred, planned and completed work in both the view and inspector. The original UMCA document defines those goals. The implementation proposal maps them onto Crush. Where they disagree on machinery or rollout, follow the correctness audit’s smaller first milestone. Start with one rolling checkpoint, a protected live tail, bounded tool artifacts and lexical recall. Persistent learning is a later, independently enabled capability.

History records what the application actually observed. The context projection orders the material eligible for one request. Memory contains attributed claims that may be stale, contested or revoked. A terminal event is evidence that the host observed a result, not proof that every external effect succeeded. Keep these distinctions visible to users and in the data model.

At the verification pass, upstream main was `04594830aefaaf8af8a7f0699ecd1d4edbdfabb4`. Its only change from the implementation baseline was to `.github/cla-signatures.json`. The file paths and behavior below refer to the pinned baseline.

## 2. Integrate with the pinned upstream

| Existing entry point | Proposed change and contract |
|---|---|
| `internal/agent/agent.go`, main `PrepareStep` | Preserve queue draining, accepted-run semantics, channel/MCP tool filtering and folded-prompt placement. Give the run a projection snapshot after inputs for this step are known. Preserve stable ordering for equal inputs. Compare against the final model call after provider-specific preparation. |
| Same file, `StopWhen`, `Summarize`, `getSessionMessages` | Give automatic and manual compaction one checkpoint service and one publisher. Retain the logical run ID, cancellation owner, reports and terminal notification. Preserve the public manual Summarize busy check and ErrSessionBusy response. The internal automatic path can checkpoint eligible older groups at a safe step boundary while the run still owns its session. |
| `internal/agent/coordinator.go`, model construction and switching | Attach budgeting to the effective model on each dispatch, including subagents, summarization, auth retry and model overrides. Invalidate estimates on provider, model, schema or options changes. Trace auxiliary calls such as titling separately so they cannot mutate the primary checkpoint. |
| `internal/shell/background.go`, `internal/shell/truncate.go`, Bash and job-output tools | Instrument the stream before the earliest dropping buffer. Follow both synchronous and background collection to their producers. Upstream’s existing ANSI-stripped spill is a compatibility path. State explicitly whether an artifact contains native bytes, normalized text or only a preview. |
| `internal/message`, `internal/session`, `internal/db/sql` and migrations | Keep existing messages authoritative. Extend finalization and deletion transactionally. Preserve old sessions and read paths across enable/disable. Allocate migrations after the target checkout’s latest migration; regenerate sqlc outputs by the project workflow. |
| `internal/workspace`, session CLI, server and TUI | Expose the same inspect, expand, compact, forget and delete operations through the workspace service. CLI-only administration must run the same invalidation logic as an interactive session. UI displays bounded previews and capture status. |

Use a small internal context package for pure planning and checkpoint/storage contracts. Keep Fantasy types and provider translations at the agent boundary. The authoritative transactional store owns publication and revision checks. SQLite serves the local deployment; §11H specifies the second backend and worker topology. Keep request execution inside Crush in either deployment and avoid a duplicate transcript. The agent already owns step lifecycle and user-visible completion.

## 3. Persistence and transaction contracts

Proposed logical records below describe responsibilities; combine tables where existing upstream rows already express them. Generate opaque globally unique IDs. A handle is a locator; authorization is separately derived from the calling workspace/session and explicit sharing policy.

| Record | Minimum fields and rules |
|---|---|
| Session projection | Session ID, relevant content revision, active checkpoint ID, publication generation. Increment on source/projection changes, including deletion. Exclude unrelated title, cost and UI writes. |
| Checkpoint | ID, session, predecessor, captured revision, stable covered frontier, summary text, policy/format version, state. Keep coverage of earlier checkpoints traversable after repeated compaction. |
| Coverage edges | Checkpoint ID and ordered finalized source references. Include message and stable part identity, content digest/version, optional byte range with defined encoding, and dependency on preceding derivations. Specify ranges as half-open byte offsets into the identified representation. Reject mismatches. |
| Artifact | Scoped ID, capture origin, representation, byte counts observed/retained, digest of retained bytes, completeness, retention state, storage locator. Distinguish complete, capped, failed, expired and revoked. A successful preview never implies a complete artifact. |
| Later memory | Scope/owner, claim, source edges, source authority, creation/update time, review state, supersession/conflict relation and deletion state. Separate transient task state from accepted cross-session preferences. |

**Publication protocol.** Read a consistent snapshot of finalized groups and their revisions. Release the database connection before summarizing. On completion, begin a short transaction, validate the expected revision, source digests, scope and predecessor, write checkpoint and edges, then publish its pointer using a conditional update. On zero rows affected, roll back the attempted publication. Reuse the candidate only after the append-only validation described under Publish checkpoint in §7; otherwise replan from fresh inputs. Use the transaction handle for every query inside the transaction, including reads and cleanup. Do not retry a stale summary unchanged. Concurrent deletes and writers in separate application processes must obey the same storage predicate.

**Restart and recovery.** Unpublished candidates are invisible and collectible. A committed pointer must refer to complete rows. If validation fails after reopening, rebuild a legal view from retained history or report the missing range. Never regenerate a persistent ID from a process-local counter. Interrupted external tool work remains outcome-unknown unless an authoritative result or idempotency key permits reconciliation. Stale workers must fail publication after cancellation, scope changes or shutdown. Stop admitting jobs, cancel workers, await their termination, then release the DB.

**Legacy sessions.** Recognize an existing summary pointer as a legacy replay boundary with unknown coverage. Resolve a legacy pointer by target ID **and** owning session, summary kind, completion state and current eligibility before accepting it. Missing, foreign, revoked or malformed targets trigger reconstruction from the authorized retained history. At this baseline, ListFromSummary falls back on zero rows. Its SQL obtains a cutoff timestamp by message ID alone: a foreign-session pointer can select a nonempty suffix of the requested session, so the zero-row fallback never runs. Test missing, foreign, non-summary and same-second targets, and assert the actual request contents. Do not fabricate causal order from timestamps or infer source lineage from similar text. During rollout, keep a readable legacy history path and test downgrade on a copy of a migrated database. Turning off new capture does not erase existing data; deletion remains available. Change replay mode only at an idle boundary. For UMCA → native, invalidate obsolete legacy pointers and rebuild from the retained, authorized history if it fits. If a new legacy summary is required, obtain permission for the model call under the active policy. Publish a valid compatibility boundary and the transition together. If conversion cannot fit or retained evidence is insufficient, leave the session readable in its prior mode and give the user an explicit recovery choice. Do not dual-write competing active summaries. An older binary bypasses new policy and tombstones; before supporting downgrade, verify on a disposable copy that it cannot replay revoked native transcript or derived rows. Otherwise declare that database version unsupported for downgrade and provide a filtered export.

## 4. Request algorithm and overflow handling

1. Snapshot the effective provider/model, context window, output cap, tool definitions, protected instructions, finalized group frontier and current in-flight state. Treat signed/opaque continuation data and provider-required prefix segments as indivisible. Identity of concurrent tool calls comes from their call IDs, never adjacency alone.
2. Start from protected instructions, required provider state and complete recent groups, including outstanding calls and their results. Fold a published checkpoint into its covered prefix once. Keep an execution-order record where persisted creation order differs from what the model saw. A summary is labeled derived context and cannot promote quoted tool data to instructions.
3. Estimate all serialized components, including schemas, tool descriptions, attachments, framing, reasoning-related fields and the checkpoint. Reserve output and actually apply the negotiated output cap on the outgoing request. For unknown windows expose an unknown capacity and use existing provider behavior or an explicit configured bound. A character count can inform an estimate; it cannot certify provider acceptance.
4. Reduce optional recall, previews and older completed groups first. Where necessary prepare a checkpoint from a contiguous legal prefix. Budget the summarizer as a separate model call with its own prompt, schema and output limit. For one oversized group use bounded representation/segmented input where legally possible, else defer with a reason. Minimum group counts must never override available capacity.
5. Check final adapter-visible structure and size where observable. Where exact tokenization is unavailable record the estimate and its uncertainty. If the provider rejects context size, allow a bounded replan only before new irreversible work or output requires reconciliation. Keep authentication, rate limiting, transport errors and context-size errors distinct. If required groups alone exceed capacity, surface that cause with recovery choices; never silently delete one half of a tool exchange.
6. Persist content-free diagnostics: run/step and checkpoint IDs, revisions, policy version, capture status, estimated components, selected model, provider usage when available, and the reason for reductions. Raw payload capture requires separate consent, scope and retention. Stable prefix bytes can be verified in adapter tests; remote cache hits are measured separately.

## 5. Artifact lifecycle, recall and deletion

Stream bounded chunks to temporary storage with quota reservation before bytes are discarded. Track stdout/stderr separately where the producer provides that distinction. Preserve binary data or explicitly mark an unsupported representation. On process completion flush, verify byte count/digest, close and publish metadata; reconcile orphan files and dangling metadata after crashes. On ENOSPC, permission failure or quota exhaustion, allow a bounded preview with a truthful partial/failed status. Do not change the tool’s actual exit status. Use restrictive permissions, scoped lookup and opaque store-owned paths. Enforce quotas atomically across concurrent producers. Retention cleanup must handle active readers and repeated deletion.

Recall defaults to the authorized current session. Explicit sharing can widen scope only after host authorization. Apply scope inside SQL before ranking and limiting, for direct ID expansion as well as search. Index finalized changes and invalidate on deletion; make schema capability detection explicit for each SQLite driver/build. A bounded scan of eligible sources can serve installations without FTS. Bound total returned bytes and estimated tokens as well as row count; page large entries with a revision-bound cursor. Treat snippets and memory text as untrusted data, and verify retained bytes against the stored digest on expansion.

Deletion starts from source IDs while their lineage still exists. In one transaction discover all dependent checkpoints, original-content copies, task state, observations and index records; revoke access, invalidate projections, increment relevant revisions and record pending physical cleanup. Preserve enough non-content bookkeeping to finish cleanup idempotently. Include derivatives copied into later messages where the host tracks that provenance. Document the limits for pasted text, third-party providers, backup media and user-edited exports; never imply retroactive erasure from a request already sent. Test child sessions explicitly with an intentional retain/delete policy. Reauthorize a prepared request immediately before dispatch when deletion can race it. Any worker completing after revocation must discard its result.

## 6. Core implementation sequence and merge gates

| PR | Deliverable | Evidence required to merge |
|---|---|---|
| 1 · Baseline | Add fixture instrumentation around session replay and effective model calls. Record captured prompt structure, ordering, tool IDs, output limits and run lifecycle events. Keep content local to tests. | Existing replay, cancellation, queued-run, subagent and model-switch fixtures pass. Establish baseline latency, database reads, request size and replay behavior on the same machine and fixtures. |
| 2 · Capture | Producer-side artifact writer plus a scoped, bounded expansion API. Route Bash, background job output and any other producer that truncates through the shared policy. | Verify retained bytes against source for complete, binary and split UTF-8 streams. Force quota failure, disk failure, cancellation and restart before/after publication. Verify partial status and no duplicate execution. |
| 3 · Checkpoint | Finalized source references, storage revision, rolling checkpoint commit, invalidation and compatibility reader. Implement deletion alongside storage. | Two independent connections race compact/delete and compact/compact. For competing candidates with the same predecessor, at most one switches that pointer; the loser must revalidate before another publication. If deletion wins first, neither stale candidate may publish. A committed winner subsequently affected by deletion must become ineligible. One-connection tests detect escaped transaction reads. Kill/reopen around each publication boundary. |
| 4 · Live projection | Integrate legal cuts and final request checks into the accepted-run lifecycle. Manual and automatic compaction share the implementation. Feature-gate per project for rollout. | Inspect the actual next provider request during one logical run. Preserve interleaved tool IDs, folded prompts, model overrides, signed prefixes and completion identity. An oversized mandatory group produces a bounded error. |
| 5 · Inspection and recall | Add scoped read-only search, preview, expand and attribution through workspace APIs. Expose deletion as a separate, user-authorized mutation. | Both SQLite configurations pass insert→search→delete→search without an index rebuild. Session B cannot find or expand session A’s matching data. Returned payload stays within budget even for a huge requested limit. |
| 6 · User-controlled learning | Separate opt-ins for extraction, model usage, cross-session recall and export. Include evidence, review, expiry, correction and supersession. | A tool result posing as a user preference cannot acquire user authority. A stale worker cannot resurrect a forgotten claim. Manual edits to an export are preserved according to the documented ownership policy. |

The first end-to-end milestone is PRs 1–4 with a minimal exact-handle expand/inspect and delete path delivered with the first durable artifact in PR 2 and extended to checkpoints in PR 3. PR 5 adds lexical search and the full inspection interface. Later PRs expand retrieval and learning. Keep the existing upstream default until the comparison fixtures pass. Roll back by disabling new projection and background jobs while preserving administrative access to retained data. Migration removal is a separate operation requiring explicit ownership of the affected data.

**Release fixture:** one logical run starts two tools; one completes with an oversized result containing a unique marker in the omitted middle, while the other is pending. Seed enough older, fully settled groups to permit a cut. While the sibling is pending, request reduction and assert that the cut excludes the entire active group. After the sibling settles, inspect the next actual provider call. Never manufacture a model dispatch between parallel tool results merely to exercise this fixture. Assert valid tool linkage, protected instructions and one terminal event. Reopen the DB, expand the retained marker from the authorized session, deny it from a second session, delete its source, then check expansion, search, caches and checkpoint descendants. Repeat with an injected crash and an ambiguous tool outcome. Run variants with a folded user correction, subagent, provider switch and signed continuation. Record quantitative deltas for prompt size, tool-result fidelity, duplicate calls, DB reads, disk growth and p50/p95 preparation time. Set release thresholds after observing baseline distributions.

## 7. Builder’s contracts

### Delivery boundaries and ownership

Implement the six core increments in §6 and the eight required capability streams in §11 in dependency order. Capture and checkpoint storage can proceed independently once source identity, scope and error contracts are agreed. Live projection depends on both. Memory depends on working deletion and authorized recall. One implementer owns each migration and its generated code. One integration owner owns the agent-loop diff. Run each increment through the ordinary CLI/workspace entry point before declaring it complete. The first vertical slice is a release gate, not the end of UMCA: deliver manual memory, automatic opt-in learning, branch-aware task continuity, bounded retrieval and inspection to complete the functional scope.

Keep existing Crush sessions usable at each increment. Treat one or more implementation PRs per increment as an organizational choice; a single maintainer can implement and verify them sequentially. A proposed type or method below is an implementation contract, not an API that already exists. Use host types directly where they encode the same semantics. Confirm filenames and function signatures again if rebasing.

### Integration spike and ownership of step state

The pinned module uses Go 1.27.4 and Fantasy v0.48.2. In Fantasy’s agent.go, PrepareStepResult exposes Model, Messages, System, ToolChoice, ActiveTools, DisableAllTools and Tools. It has no per-step output-token field. In both generation and streaming, supplied messages become step input. Treat this as a per-call projection; use the next callback’s actual options as the new input. Do not mutate the callback slice in place or assume a one-time truncation rewrote Fantasy’s accumulated history.

1. Start with `internal/agent/context_replay_test.go`. Extend the recording model to capture the effective Fantasy call for each step. Exercise at least three steps in one accepted run. Force a model refresh, queue a correction between steps, and change the available tools. Establish which settings arrive before and after PrepareStep. Use a fake transport when asserting serialized provider content.
2. In `internal/agent/agent.go`, identify the source of each folded prompt, settled tool exchange and still-live part before representation-changing transformations. Preserve ordered identities in run-local state; a hash of text is only an integrity check. Two identical user messages need distinct IDs. A fully settled earlier assistant step can be summarized within the same user turn. Treat one assistant emission and all its required tool results as a unit. If any sibling call from that emission is pending, protect the entire group, including already returned siblings.
3. Apply the projection consistently on every subsequent preparation. Reapply native media conversion, prefix and cache-option logic in the order required by the adapter. Recalculate after dynamic tool filtering. Preserve original native parts for replay, including opaque signatures. If an adapter cannot safely cut any prefix, keep the required prefix, or return a typed incompatibility/limit result. Never synthesize a signature.
4. Bind final admission to the selected language model on every construction path. `internal/agent/coordinator.go` already wraps language models with a request **timeout** guard. Preserve its timeout/error behavior. Implement output-limit enforcement on a surface that actually controls the call sent to the provider. Verify this with a captured request after a model switch. Count system text exactly once even if represented separately from messages.
5. Let the existing run own queue draining, permissions, cancellation, tool effects and RunComplete publication. No synthetic user prompt to resume a reduced run. Maintain the mapping from projected step to original source frontier. If the spike cannot show a smaller second request within the same logical run, keep the feature off and fix the integration before adding memory.

### Source identity, projection and budget examples

Treat a scope as a host-resolved principal plus allowed resource set. Session, workspace/repository, branch or revision, subagent and channel may further restrict it. Never derive the owner from an LLM tool argument or a prompt string. Native source identity is the persisted message/part identity or a host-issued event identity. Track stream order and a finalized version separately from message creation time. When a source changes, invalidate the covered version. Old sessions with insufficient order evidence use a deterministic tie-breaker for display and are labeled legacy.

The following example has an older finalized, contiguous prefix. G1–G7 are ordered execution groups, each anchored to the corresponding host step:

```text
protected instructions | checkpoint covering G1..G4 | G5 | G6 | user correction | active G7
                       ↓ after G5 is finalized and eligible
protected instructions | checkpoint covering G1..G5      | G6 | user correction | active G7
```

G7 may include multiple dispatched calls. Keep the initiating assistant part and all corresponding results required by the provider. Never remove an unresolved group merely because its launch occurs before a chosen position. If a legal contiguous cut cannot advance, retain that segment and report why. Store host-computed coverage, predecessor and protected constraints independently of summary prose. Reject empty, truncated, stale or structurally invalid summaries. A deterministic excerpt may be published only with its actual representation and coverage and only if it satisfies the same structural rules.

For a shared 128,000-token window and an enforced 8,000-token output cap, the gross input allowance is 120,000. A separate 100,000 input limit lowers it to 100,000. Subtract margin and required content from that allowance. For independent input/output windows use the declared input window directly and clamp output separately. Never treat cached tokens as free context capacity. Multimodal and reasoning counts are provider-dependent. If exact counting is inaccessible, expose estimated admission; do not assert a precise final wire count. Stop reduction when the measured candidate stops shrinking or no eligible content remains. For provider overflow, permit at most one smaller replan for the same logical step across all transport/auth attempts. Record the allowance on the run/step, not on the individual attempt. A second overflow, unchanged candidate, or evidence of prior emitted output/tool effects ends automatic retry. Preserve the underlying cause.

### Storage and operation semantics

| Operation | Precondition, write and failure behavior |
|---|---|
| Finalize source | Upsert by host identity and expected source version, with digest and representation. Duplicate delivery of the same version/content is idempotent. Reject a digest conflict on the same version. An authorized edit advances the version and invalidates dependents. Synthetic repair records carry outcome-unknown, separate from tool-reported success/failure. |
| Publish checkpoint | Outside a transaction, generate from a frozen ordered range. Inside one transaction validate membership, source versions, applicable settings and predecessor, insert coverage, then conditionally switch pointer. Read through the transaction-bound querier. On conflict roll back all candidate rows. A new unrelated tail entry alone need not invalidate the candidate: re-read and validate that its covered prefix and current pointer are unchanged, and rebase the conditional publication against the current revision. Never blindly use a fresh revision with old evidence. |
| Expand retained source | Authenticate scope, check live source/version, validate range, verify digest against the stored representation, then return bounded content with handle and next range. Return opaque unavailable for unauthorized requests; for authorized owners distinguish expired, partial, missing and corrupt. Empty retained content and failed capture are distinct. |
| Forget a record | Revoke the accepted version, invalidate recall and cached selections, remove or recompute dependent derivatives and fence older queued results. Repeating the operation succeeds without recreating anything. Forgetting a learned claim leaves authoritative transcript history intact unless the user also requests source deletion. Prevent automatic re-learning of a deliberately forgotten claim from the same evidence using minimal suppression metadata within policy. |
| Delete source/session | Compute dependent closure before cascades sever edges. Include recall tool outputs and subsequent summaries derived from them, fork/handoff copies where traced, legacy pointers, FTS, caches and files. Remove sensitive payloads or redact attributable spans where reliable. For mixed-source text of uncertain provenance suppress the whole derived unit. Mark any dependent unknown-origin copy as such, rather than asserting precise erasure. |
| Concurrent dispatch and deletion | Define the dispatch decision’s linearization point. If revocation wins, abort/replan. After irreversible send, cancel where supported, invalidate future reads/publications and expose the already-sent limitation. A final authorization read alone cannot close a gap between that read and send. Use host serialization for local work and a storage-backed generation/claim for competing processes. Treat the successful generation-checked admission as the ordering point for “already in flight.” Work admitted first may still reach the provider after local deletion returns. Cancel it best-effort, reject late publications and inform the user of this boundary. Revoke first and a later admission must fail. Release DB resources before network I/O; a DB transaction cannot atomically commit a remote HTTP send. |
| Physical cleanup | Journal owned file identifiers for deletion in the revoking transaction. Retries verify scope and identity. When live readers still hold a file, logical access is revoked immediately and physical cleanup follows safely. Never delete source project files or user-maintained exports as if they were store-owned artifacts. |

Index source edges in both directions, revisions by owner, the active frontier by session and artifact cleanup work by state. Put scope into every direct lookup and join. SQLite foreign keys enforce referential integrity, not authority or derivative erasure. Exercise both supported SQLite implementations on their supported GOOS/GOARCH combinations. If FTS is unavailable, start with exact lookup and a bounded lexical fallback with an explicit incomplete status. Do not quietly rebuild an index in test setup to make a production write path pass.

### Artifact capture details

Follow `internal/shell/capped_buffer.go`, `stream.go`, `background.go`, `truncate.go` and `persist_message.go` from producer to transcript. Tee at the earliest reliable byte writer, before capped buffers, ANSI removal, formatting or combination. Trace Bash, job polling, direct-shell execution and persisted shell messages separately. Polling the same background job must reuse one producer-owned artifact and offset, not duplicate its storage. Never block forever on the artifact sink or convert a storage error into a successful command exit. Apply both per-artifact and aggregate quotas including temporary files. Store observed length separately from retained ranges; if the producer itself dropped bytes, mark the observed boundary unknown. Head/tail previews disclose omitted ranges. Before publishing a ready DB reference, close the writer and rename the verified temp file on the same filesystem. If claiming power-loss durability, sync file and directory using supported platform primitives and test that guarantee. A process crash between rename and commit leaves an orphan. Sweep only store-owned files after checking active writer/reservation state, and make cleanup idempotent; age alone cannot identify an orphan. Scope files by opaque IDs in an owned directory; reject path traversal and symlink substitution on expansion.

### Memory, search and task-state implementation

Deliver manual memory creation, edit, retrieval and forgetting before enabling automatic learning. Define a record with owner/scope, stable ID, type (preference, observation, decision, procedure or task note), topic, text, evidence, status and versions. For a manually entered claim, create a user-authored source/version and record deliberate scope. A manual edit creates new user evidence and retains provenance of any incorporated material. A direct user statement about a preference can support that preference. A pasted log inside a user message remains log evidence. A source-backed assertion describes what the source says; current repository state requires a revision or fresh inspection. Store conflicting observations with evidence and validity times. User editing or retracting a preference supersedes the prior version atomically. Default recall excludes needs-review, superseded, expired and retracted entries. Permit explicit inspection of retained versions subject to deletion policy.

After opt-in, enqueue extraction only for finalized, eligible groups. Job identity includes purpose, source version set and relevant policy/model configuration. Summarization and extraction are separate operations. Deduplicate pending work, bound concurrency, snapshot inputs, run model calls outside transactions, limit transient retries, then recheck sources and consent before commit. Rebuild missing work from retained source coverage after restart. An extraction error does not interrupt the user’s turn. Record extraction usage separately. Promote inferred candidates according to user policy; never automatically promote tool text into system or developer instructions.

Reuse session.Todos and its existing mutation path as the source of task-list status. Store a source/version reference when summarizing a todo. A checked todo by itself does not prove the corresponding tool succeeded. Invalidate the note after applicable todo edits or independent subagent changes. Keep one current task note per branch/session scope with goal, active constraints, decisions, files/revisions, verified progress, open questions, unresolved tool calls and next action. Update it after a meaningful transition with references to supporting results. A handoff to a new session is explicit, size-bounded, linked to the original and checked against the recipient’s authorization. Mark progress stale when files or branch identity change. Do not equate a Git branch name with an authorization principal.

Search exact handle/path first when applicable, then lexical matches in eligible finalized sources and active memories. Rank by applicability and evidence, use recency only when relevant, and collapse repeated derivatives of a single source. Return type, scoped handle, version, representation, excerpt and completeness. Retrieve by immutable file revision where available. For asynchronous or partial indexing, revalidate live rows and signal missing coverage/timeouts. Avoid caching initially. If measured cost requires caching, key by scope, authorization/policy generation, source/index version, query or model/render settings as applicable; revalidate before disclosure. A provider cache hit requires provider-reported evidence.

### User controls and operations

| Control | Initial semantics and user-visible result |
|---|---|
| Context management | Experimental project opt-in. Reuse existing JSON `disable_auto_summarize` and its inverted shell option `auto-summarize`. Manual compact remains a separate request to the same service. Disabling UMCA requests the mode transition below. Native transcript persistence remains governed by Crush. |
| Capture and retention | Expanded capture off until quotas, eviction and deletion work. Expose retained representation, size, policy and partial reasons. Index only the enabled scope. |
| Learning | Separate manual create/edit/forget from automatic extraction. Automatic learning off prevents new automated publication, including late workers. Explicit manual creation remains independently available. It does not erase existing accepted records. |
| Recall | Same-session retrieval under UMCA, cross-session retrieval opt-in, no implicit cross-project access. A recall-off setting removes previously accepted memories from future projections. Tool registration obeys disabled-tool and per-agent allow lists and channel restrictions. |
| Visibility | Metadata-only context inspector lists required groups, checkpoint source ranges, optional recall, dropped units, estimate vs observed usage, retention state and reason for reductions. User controls explicit raw-payload debugging and export independently. |

Expose workspace operations for inspect, compact, search, expand, create/update/forget memory and delete retained data. Use the same authorization and deletion service from embedded and server-backed clients. Configuration must round-trip through the existing JSON schema and shell configuration paths, including inverted booleans, defaults and pre-UMCA configs. Supply user-facing examples when the implemented option names and UI are stable. Treat names in this document as proposed unless explicitly identified as existing.

### Acceptance ledger and handoff

Each test below needs a reproducible fixture, expected outcome, command and observed result in the implementing PR. Unit tests of a planner cannot substitute for an assertion about an outgoing call. No production provider credentials are required for deterministic transport tests. Mark an adapter requiring an untested native capability experimental until it passes.


**History and provenance**

- [ ] Identical text in two messages retains distinct event identities.
- [ ] Equal-timestamp messages receive deterministic ordering.
- [ ] Interleaved parallel results remain associated with their original assistant step.
- [ ] Interrupted and synthetic results retain their capture and outcome status.

**Context and compaction**

- [ ] Current input and folded user corrections survive compaction.
- [ ] Live projections survive repeated PrepareStep reconstruction.
- [ ] A long single autonomous turn can compact settled groups.
- [ ] The summary request itself stays within its applicable budget.
- [ ] Failed or truncated summaries never become active.
- [ ] Repeated reduction terminates when no safe progress is possible.

**Provider integration**

- [ ] The request guard remains active after model refresh.
- [ ] Configured output limits agree with planner reserves.
- [ ] Provider prefixes and tool schemas are counted.
- [ ] Unsupported media or continuation formats produce explicit compatibility behavior.
- [ ] Overflow recovery does not repeat previous tool execution.

**Lifecycle**

- [ ] Cancellation works while planning, summarizing and resuming.
- [ ] Queued RunIDs complete exactly according to host semantics.
- [ ] Manual and automatic compaction cannot conflict.
- [ ] Channel responses are not duplicated.
- [ ] Shutdown drains database-dependent work before releasing SQLite.

**Storage and deletion**

- [ ] Deleting a source invalidates its derived summaries and memories.
- [ ] Recall-derived tool results cannot restore deleted sources.
- [ ] Legacy summary pointers are not left active after invalidation.
- [ ] Missing, partial and corrupted artifacts are marked correctly.
- [ ] Retention limits remain enforced during long-running processes.
- [ ] Concurrent access cannot publish conflicting checkpoints.

**Memory and authorization**

- [ ] Tool output cannot become authoritative user instructions.
- [ ] Subagent and channel recall cannot access unauthorized sessions.
- [ ] Disabling learning prevents stale jobs from publishing.
- [ ] Superseded preferences are not recalled as current.
- [ ] User deletion and correction propagate to applicable search results.

Add dedicated negative fixtures for equal-timestamp migrations, same block handle in two sessions, an FTS insert without rebuild, deletion of the sole source of a reversible snapshot, retained output with a missing middle, exhausted disk during capture, an expired cursor, and a one-connection publication with an injected failure at each write. Verify a failed publication leaves the prior checkpoint usable if its sources remain eligible; a deletion racing the failure must instead invalidate that checkpoint. Validate both context-only operation and all learning/recall flag combinations. Count primary, summary and extraction usage, not just the final shrunken prompt. Benchmark disabled vs enabled on identical fixture histories and report database reads, bytes retained, time to first output, preparation latency, recall success and total model usage. Preserve meaningful failures in the report; an implemented test is not evidence it passed.

For local verification use the Go version in go.mod. Run the touched package tests first, then the repository suite (`go test ./...`), build (`go build .`), and formatting/lint checks required by AGENTS.md. For SQL or config changes use the repository tasks `task sqlc` and `task schema`; inspect generated diffs. A race build needs a supported race toolchain/CGO environment and must be a separate check from the pure-Go build. Preserve the native fixtures in context_replay_test.go, queued_runid_test.go, folded_prompt_test.go, interrupted_tool_call_test.go, model_switch_test.go and dispatch_cancel_test.go. Add small fixtures at each failure boundary rather than recording live confidential prompts.

A work handoff includes the target SHA, exact changed entry points, schema compatibility and regeneration commands, fixture IDs, observed checks, unverified providers/platforms, and residual risks. The receiving implementer can proceed once predecessor merge gates pass. Stage numbers assign dependency order, not a requirement to buy or deploy external services: the complete implementation supports local-only operation, including when all remote egress and paid processing are disabled. The full completion gate includes all eight streams in §11. Measure their effects before enabling them by default; a disappointing benchmark calls for a documented fallback or improvement and does not remove the enabled-path implementation and test obligation. Historical queries remain subject to retention, scope and deletion.


### Host compatibility checks before merging

| Source contract | Implementation and test |
|---|---|
| **Final request and retries** · Fantasy v0.48.2, `agent.go`, Generate and Stream | Fantasy builds each step from the initial prompt plus accumulated response messages. After preparation it can replace the first message when System changes and materializes the final tools, including provider-defined tools. On an attempt with ModelProvider it obtains the model from that callback, even if PrepareStep returned a Model. Place admission on every effective model returned by the host. Verify Prompt, Tools, ToolChoice, media and output limit at the Fantasy Call; use a transport fixture to assert provider translation separately. Capture no live secrets or real user prompts in tests. |
| **Turn model identity** · `internal/agent/model_switch_test.go`, `shared_models_test.go` | A model/config switch leaves a running turn and a previously queued pinned call on their accepted model. A later prompt can select the new model. Credential refresh may rebuild the same selection. Assert all three cases, including the refreshed model's wrapper, credentials and effective limits. Do not implement “check effective model” by reading the newest global model on every step. |
| **Limits and omission** · `internal/agent/agent.go`, `request_guard.go` | Crush omits MaxOutputTokens for a nonpositive configured value; sending a literal zero can break compatible providers. Use a positive supported cap when UMCA promises a reservation. When a provider requires omission, honor its verified bound, or report unknown/estimated capacity. Do not overwrite a tighter caller cap or make a precision claim based solely on last-step usage. Keep timeout, context-size, incomplete response and cancellation as separate results. |
| **Hook and session ownership** · `internal/agent/agent.go`, Summarize/summarize | Preserve manual/automatic trigger values. Run PreCompact before generating the candidate and respect a veto without repeatedly calling the hook at the same frontier. Emit PostCompact only after successful publication. Keep summary-generation cancellation subordinate to the owning run; do not simply call the existing public Summarize from inside its busy session or reuse the internal helper that sets a new activeRequests entry. Test blocked, cancelled and committed paths for correct hook, title, cost, TurnEnd, channel reply and RunComplete counts. |
| **Real capture points** · `internal/shell/stream.go`, `capped_buffer.go` | RunAndCaptureStream assigns stdout and stderr to one writer; progress callbacks can run concurrently in a different order from retained output. Tap before the capped buffer. Label this representation merged unless the producer is changed to provide independent streams. Never infer stream identity or canonical ordering from progress events. Measure producer latency with the artifact sink stalled; bound the queue and record dropped ranges when capture cannot keep up. |
| **Stored parts and CLI deletion** · `internal/message/message.go`, `internal/cmd/session.go` | Parts are serialized on the message row. An array index into mutable JSON is not a permanent part ID. Use a finalized message version and part locator plus representation digest, or introduce a persistent part key with migration. The standalone session command can call the session service directly without AppWorkspace. Put invalidation below both deletion entry points and exercise local CLI, connected CLI and workspace API with the same fixtures. |

### User-control and retention decisions

Start the first opt-in slice with one explicitly named adapter and one local user. Name the tested model family, capabilities and request shapes in the implementing PR. Unsupported adapters retain ordinary Crush behavior or produce a documented capability result when strict UMCA behavior was requested. Expand adapter coverage independently after the same source and protocol gates pass. Production telemetry, an extra agent, cross-project discovery and a distributed job service are unnecessary for this slice.

Offer **remove from current context** as an operation on optional material, distinct from forgetting learned memory and deleting retained sources. Bind a hide/pin decision to scope and source version; expose its reason and removal in the inspector. Keep required provider parts intact. Pins cannot override revocation or access control. When pins and required content exceed capacity, report the conflict and allow a user to unpin, revise input or select a compatible model. Historical instructions preserve original authority and applicability; quoting them in a summary cannot grant them new authority over the current user.

Suppress additional UMCA indexing, capture, export and model-based extraction independently when the user opts out. Parent and child sessions have separate source identities. Default to retaining independent child content while invalidating any traced derivatives of a deleted parent source, and offer explicit deletion of a selected subtree. Never grant child, channel or sibling recall solely by matching filesystem paths. Mark a derived record blocked until its surviving sources have been revalidated. Keep negative suppression metadata minimal and content-free where possible. Restoring a pre-deletion backup can resurrect forgotten data: quarantine it for explicit restore/import and apply available revocations before enabling recall. Report any missing revocation history. Local logical deletion does not guarantee forensic erasure from SQLite free pages, WAL, snapshots, or copies outside host control.

For ingestion, a repeated event ID **and version** with a different payload is a conflict, not an authorized update. Accept legitimate edits only through a new source version with an expected predecessor. For partial capture, store ranges with offsets in the original stream where known, digest each available range and mark gaps. Never concatenate head and tail and advertise the result as a contiguous original. Cap both expansion I/O and returned bytes. If full-file verification would require reading an unbounded file per small excerpt, verify authenticated chunks against a trusted manifest or verify once at ingestion and revalidate against replacement/corruption under the store’s access model. Do not claim full-file verification from a checked excerpt.

Define summary acceptance on the actual summarizer response. A transmission error, length stop or missing required source group cannot publish. When a provider omits the finish reason, either establish completion through a supported adapter contract or defer the checkpoint. Preserve Crush’s cost accounting for a failed or rejected summary without activating it. Keep the last valid projection **only while all its inputs remain authorized**. Otherwise reconstruct from surviving sources, or stop with a recoverable status.

### Coverage of the three design documents

Use this crosswalk to distinguish a deliverable from an example or a superseded design. The original brief defines the capabilities; the Crush proposal supplies candidate integration points; the audit revises unsafe assumptions. Where two versions disagree, use the disposition below and the acceptance ledger above.

| Source sections | Implementer disposition |
|---|---|
| Brief §§1–3; proposal §§1–7; audit §§1–3, 6, 14–15 | Implement the native responsibilities in §§1–3 and 7 of this guide. Preserve host event identity. If the host supports a fork, inherit only authorized history through its recorded fork point. Later sibling events remain separate. Branch merging is outside scope. Session hierarchy and Git ancestry are different relationships. |
| Brief §§4–5; proposal §§10, 13; audit §§5–8 | Implement bounded capture, scope, deletion and provenance from the first durable record. Recognize interactive shell commands persisted as user-role messages as shell events. A job-kill result records observed termination, never rollback of side effects. A subagent report is a generated claim; unknown legacy origin is weak evidence. Test each through memory extraction. |
| Brief §§6–8; proposal §§8–9; audit §§3–4, 10 | Use legal contiguous cuts, independent summarizer budgets and one publication owner. Disabling automatic summarization suppresses automatic summary model calls; deterministic previews and required admission checks still apply. Test this with a recording model and an oversized input. |
| Brief §§9–10; proposal §§11–12; audit §§7–9 | Deliver versioned memory, task notes, correction, forgetting and lexical recall. Session, project and deliberately shared personal memories have distinct scopes. Personal scope requires explicit user authorization for cross-project access. Keep cross-project recall disabled in the initial implementation and show it as unavailable until access and deletion tests pass. |
| Brief §§11–12; proposal §§14–15; audit §§11–12 | Provide inspect, compact, search, expand, memory inspection, explicit learning and forgetting through the shared operations in §7. The proposal’s command spellings (e.g. `/context expand`, `/memory forget`) are examples. Preserve existing permission, busy-session and confirmation behavior through every frontend. |
| Brief §§13–14; proposal §§16–20; audit §§16–19 | Deliver the staged gates in §§6–7. Demonstrate a long coding session, a task change, restart and follow-up recall against native Crush. A performance or quality claim needs measurements from equivalent fixtures. The example time estimates and token settings are not release requirements. |
| Brief §15; proposal §§2, 5; audit §13 | The source index below supports independent implementation. Reference tests illustrate edge cases; they are not portable conformance tests for Crush. |

The original documents placed advanced features behind milestones. This specification includes each of them in §11. Maturity, cost and consent determine runtime activation; each feature still requires implementation and acceptance evidence.

For request inspection, record an optional bounded receipt with request/attempt identity, authorized session, selected source/checkpoint/memory versions, estimate, reported usage and outcome. Reuse existing operation IDs. Never expose a source handle after access is revoked. The receipt supports diagnosis and attribution; it does not justify storing a second raw prompt or a transcript of every token.

## 8. Findings from XRush that directly affect this design

| Mechanism | Reuse and regression condition |
|---|---|
| Leaf summaries and ordered source links | Keep host-assigned coverage. XRush’s replacement helper reads through the pool while the caller holds the sole SQLite connection. Inject deadlines and verify the transaction-bound reader. Its reversible snapshot also precedes publication; test both sides of that crash window. See `internal/lcm/compactor.go`, `store.go` and `internal/db/connect.go`. |
| Reversible blocks | The process-local block counter can restart at the same value. The lossless lookup uses the block ID alone. The earlier two-session SQL fixture returned the other session’s original. Resolve a scoped immutable identity and check content digest. See `internal/lcm/reversible.go` and `store.go`. |
| FTS | On the full XRush schema fresh summary inserts returned zero ranked hits. After a rebuild the same query returned both sessions. Exercise ordinary writes and authorization together. See `internal/lcm/store_fts_test.go`, `retrieval_tools.go` and `store.go`. |
| Message deletion | Source deletion removed the link while leaving summary text and reversible original content. Traverse dependents before unlinking. Session-level cascade is a different operation. |
| Compaction triggers and estimates | XRush LCM reacts to step usage; the agent also stops, summarizes and can requeue a rewritten prompt. Micro-compaction can count savings after the replacement record failed. Measure the provider call, keep one publication owner, and bound summarizer input even for the first group. |
| Memory export and coordination | XRush’s run-end hook can write session memories into a common working-directory file. Separate storage, authority to recall, permission to export, and manual edits. Model routing, planning and swarm execution are outside the initial context-management milestone. |

## 9. Decisions from the 20 archived repositories

The source IDs refer to the supplied UMCA reference catalog. Each repository below was extracted locally. The appendix identifies the actual files consulted. These are design comparisons from archive snapshots; no third-party runtime or benchmark was executed.

| Repository | Decision for upstream Crush |
|---|---|
| Volt | Carry forward transactional prefix replacement and integrity diagnostics. Add expected-revision validation to the transaction. Keep the first checkpoint linear even though Volt models lanes and summaries. |
| billion-context | Use a stable content reference and preserve an unchanged projected prefix where possible. Text hashes and formatted expansion do not establish byte fidelity for binary or provider-native content. |
| Fold | Check source generation, topology and protected groups before publishing a prepared fold. Pair counts alone cannot validate interleaved provider tool-call IDs. |
| pi-tuck | Reserve room for output, required instructions and the recent tail. Use estimate-versus-usage receipts for calibration. Clamp the actual outgoing cap and test adapter overhead separately. |
| Mastra | Adopt storage-side generation fencing and keep pending tool groups out of background observation. A process-local commit queue can release stalled work without preventing its late write. Translate the transaction predicate into SQLite. |
| magic-context | Pressure bands and structural checks can guide scheduling. Keep heuristics behind fixed invariants; recency or model verification cannot establish truth or authorize deletion. |
| Remnic | Validate quotes and exact source spans; test offsets with Unicode before porting JS string-index assumptions to Go. Reuse the ideas of provenance and invalidation before considering reranking or a belief ledger. |
| Engram | Version corrections and reject a replayed operation key with a different fingerprint. Exact topic lookup and lexical previews are suitable before embeddings. Verify freshness on the ordinary update path. |
| context-mode | Select chunks under a byte budget, then return them in reading order. Capture streams upstream of text selection and retain stderr even when the process exits successfully. |
| avtc-pi-session-owl | Test parent/attachment and working-view invariants. A partial curation result after cancellation must never publish as the authoritative checkpoint. |
| pi-stack | Reversible tool-description shortening can change agent behavior. Keep tool schemas intact in the first release. Later evaluate evidence ordering with held-out queries. |
| pi-smart-compact | Use its verification scenarios to test coverage, progress and session continuity. A model-generated statement that tests passed is evidence of a claim only. |
| pi-cache-optimizer | Use deterministic ordering and provider-aware cache diagnostics. Preserve provider-required ordering, signed data and user-visible tool semantics; measure actual cache use independently. |
| pi-context-view | Adapt request-baseline capture for deterministic tests and redacted diagnostics. Track attribution at each transformation; missing metadata stays unknown. |
| pi-observational-memory | Use ledger fold/tombstone cases for replay tests. Keep logical hiding distinct from physical deletion and reject an unrecognized snapshot frontier. |
| pi-vcc | Compact sectioned briefs are useful for previews. Keep original commands, files and source links separately recoverable where capture was complete. |
| pi-posthorse | Borrow bounded rollover and scope-aware provenance resolution. An oversized summary defers; publication still requires an atomic source-generation check. |
| pi-hermes-memory | Correction heuristics can propose candidates for review. Approval, provenance and persistence require explicit host operations. |
| pi | Borrow scheduling and cancellation tests for background extraction/prefetch. Recheck scope and revision after asynchronous retrieval; cancellation can leave remote work finishing late. |
| pi-extensions | Borrow scoped session-lookup examples. Recall should be read-only and observable; invoking another assistant to obtain data needs its own authority and accounting. |

## 10. Source index and review boundaries

Paths below are relative to the extracted archive. The line ranges are the UMCA document’s locators. The SHA-256 digests below identify the archived ZIP bytes even where the ZIP contains no Git metadata. Source presence and implementation inspection do not establish production reliability. The Crush LICENSE identifies FSL-1.1-MIT and the Context Mode snapshot identifies Elastic License 2.0. Reimplement mechanisms in idiomatic Go; before any code reuse, review the exact files’ license and notices, including nested packages.

- S01 · `volt-dev/packages/voltcode/src/session/lcm/db.ts:161-236`

- S02 · `volt-dev/packages/voltcode/src/session/lcm/integrity.ts:30-105`

- S03 · `volt-dev/packages/voltcode/src/session/lcm/db.ts:1922-1982`

- S04 · `billion-context-master/kernel/src/content-store.ts:29-118`

- S05 · `billion-context-master/kernel/src/view-latch.ts:71-151`

- S06 · `billion-context-master/kernel/src/decompress.ts:176-235`

- S06 · `billion-context-master/kernel/src/turn-integrity.ts:198-261`

- S07 · `fold-main/extensions/lib/folding.ts:611-683,847-867,912-956`

- S08 · `fold-main/extensions/lib/scheduling.ts:314-335,951-1120`

- S08 · `fold-main/extensions/lib/canonical.ts:41-52`

- S09 · `pi-tuck-main/src/request-capacity.ts:68-122`

- S09 · `pi-tuck-main/src/budget.ts:75-104`

- S10 · `pi-tuck-main/src/tuck/cut.ts:74-122,155-196`

- S10 · `pi-tuck-main/src/budget.ts:117-170`

- S11 · `mastra-main/packages/memory/src/processors/observational-memory/commit-queue.ts:3-39,69-100`

- S11b · `mastra-main/stores/pg/src/storage/domains/memory/index.ts:492-505,3416-3480,3494-3514`

- S11b · `mastra-main/packages/core/src/storage/domains/memory/base.ts:614-644`

- S12 · `mastra-main/packages/memory/src/processors/observational-memory/observation-turn/safe-buffer-prefix.ts:42-75`

- S13 · `mastra-main/packages/memory/src/processors/observational-memory/observation-strategies/async-buffer.ts:152-210`

- S13a · `mastra-main/packages/memory/src/processors/observational-memory/activation-ttl.ts:36-90`

- S14 · `magic-context-master/crates/mc-core/src/decay.rs:49-131`

- S15 · `magic-context-master/crates/mc-module/src/scheduler.rs:465-647,750-850`

- S15 · `magic-context-master/crates/mc-module/src/historian_validate.rs:521-744`

- S15a · `magic-context-master/packages/plugin/src/features/magic-context/dreamer/verify.ts:89-107,260-300,745-780`

- S15a · `magic-context-master/packages/plugin/src/features/magic-context/dreamer/verify-gate.ts:98-140`

- S16 · `remnic-main/packages/remnic-core/src/lcm/schema.ts:60-120`

- S16 · `remnic-main/packages/remnic-core/src/lcm/dag.ts:17-110`

- S17 · `remnic-main/packages/remnic-core/src/extraction-span-provenance.ts:19-41`

- S17a · `remnic-main/packages/remnic-core/src/retrieval.ts:8-104`

- S17b · `remnic-main/packages/remnic-core/src/rerank.ts:1-160`

- S17c · `remnic-main/packages/remnic-core/src/memory-cache.ts:301-341,419-449`

- S18 · `remnic-main/packages/belief-ledger/src/types.ts:3-60`

- S18 · `remnic-main/packages/belief-ledger/src/ledger.ts:66-123`

- S19 · `engram-main/internal/store/store.go:1563-1605,4684-4750`

- S20 · `engram-main/internal/store/store.go:6188-6250,6355-6405,6517-6596`

- S20 · `engram-main/internal/store/search_projection.go:10-52`

- S21 · `context-mode-main/src/core/tools/execute.ts:137-237`

- S21 · `context-mode-main/src/core/store/schema.ts:28-77`

- S22 · `context-mode-main/src/core/index/select.ts:19-68`

- S23 · `avtc-pi-session-owl-main/src/types.ts:94-190`

- S23 · `avtc-pi-session-owl-main/src/graph/invariants.ts:32-194`

- S23 · `avtc-pi-session-owl-main/src/selector/run.ts:108-238`

- S24 · `pi-stack-main/packages/pi-deferred-context-engine/lib/compact.js:3-81,137-187`

- S24 · `pi-stack-main/packages/pi-deferred-context-engine/lib/engine.js:32-90`

- S25 · `pi-stack-main/packages/pi-supernova/src/context/evidence-rank.js:6-154`

- S26 · `pi-smart-compact-main/src/phases/verify.ts:66-86,2062-2113`

- S26 · `pi-smart-compact-main/src/app/run-smart-compact.ts:123-132,195-261,379-408`

- S27 · `pi-cache-optimizer-master/src/request-payload.ts:9-42,131-263`

- S27 · `pi-cache-optimizer-master/src/tool-ordering.ts:103-207`

- S27 · `pi-cache-optimizer-master/src/prompt-rewrite.ts:209-242`

- S28 · `pi-context-view-develop/src/capture/request.ts:121-163`

- S28 · `pi-context-view-develop/src/capture/attribution.ts:1-34`

- S29 · `pi-observational-memory-master/src/session-ledger/fold.ts:25-100`

- S29 · `pi-observational-memory-master/src/session-ledger/types.ts:1-75`

- S30 · `pi-vcc-master/src/core/build-sections.ts:20-42`

- S31 · `pi-posthorse-main/src/rollover.ts:45-110,140-170,369-443`

- S31 · `pi-posthorse-main/src/note-provenance.ts:129-162,228-265`

- S32 · `pi-hermes-memory-main/src/handlers/correction-detector.ts:82-125,135-160`

- S32 · `pi-hermes-memory-main/src/handlers/learn-memory.ts:7-45`

- S33 · `pi-master/packages/pi-memory/src/background-extraction.ts:114-180,235-278`

- S33a · `pi-master/packages/pi-memory-mem0/src/prefetch.ts:45-119`

- S34 · `pi-extensions-main/packages/pi-peek-agent/src/tool.ts:39-76,151-172,222-239,293-320`

- S34 · `pi-extensions-main/packages/pi-peek-agent/src/index.ts:28-54`

### Archive identity

| Snapshot | SHA-256 of inner ZIP |
|---|---|
| avtc-pi-session-owl-main | `3c19a839423bde4b416510a0a5d56822c57ef61f9e5d0d262d060469f4ca10c9` |
| billion-context-master | `3476dad9f28ce48020e85adf5d5850500630971178ba5a142e9ded346a512cc9` |
| context-mode-main | `9bd9ce69c33d29d6647e4d2ce1917492993d398b30d590539fae305f4d8aa232` |
| engram-main | `878f151bbfb1ad044f7c0e75a37cfaf497004b55cf183a9498e7c8e7f499fdce` |
| fold-main | `e70b899ad2c060f402b6600ea04d96e19a39f8b141ecb479eb5b1fd7577619c3` |
| magic-context-master | `05cdd1ce631653753d718821498364cefed65fa4749e713fd5cbecba4268354f` |
| mastra-main | `a25927ddf99488ad85a8feff2bbd806a3af10efbcc2a8656dfb836888a0a4ef4` |
| pi-cache-optimizer-master | `d7756c986a8dd4f82141f4549ecbc572ff001a237cc493170197cc9ef6f24709` |
| pi-context-view-develop | `8eadc05046419242d05a2f7ed3378b23a2846d19eddae10b4cca9d5db3929bb0` |
| pi-extensions-main | `8f2e3fe25594d8a911a43dbac44eac55ab099e0d7a8610fe2ffd03ac45035e9f` |
| pi-hermes-memory-main | `6830e9c453cefc9b3f1b3f0f3680a4b23b9952463d95e988877cd41d8b16eb2b` |
| pi-master | `d68af992da26288bc8a0044e0cedb4f09b373aab9373d705b9d4434d0d9ab6bf` |
| pi-observational-memory-master | `dc784b4bca1e126f61cbead1ea3d0a41dfaeead458a3a70f3fa0c6d1d10c2f50` |
| pi-posthorse-main | `62908bac171bd70d5fe0dfb799a4f341a9c1cbac219bb437660a3bcda5219e8c` |
| pi-smart-compact-main | `279c66f5b195611eea7cd29b637e7df0ea2299f5babce3874fd9db262be7cc77` |
| pi-stack-main | `1d063c0067e260cffcd812b65c25a4f6354677af7d43d124ae3a54efe2986500` |
| pi-tuck-main | `e644c6afa34afcf4abb747c0346cd30cf21559d81cd7caf9224d6c1ac6b3d4b5` |
| pi-vcc-master | `21382d3ba48a7b8eaad11e9aa8ee08224d8ffe3fcfb056b5a6b5adae18a8482b` |
| remnic-main | `4e8ffeecb780e02969520604466bfea40fbbd242fc55903ed4b8866263b0ab4a` |
| volt-dev | `5db38cd9e0989a939c653a6b941ef506b3e01c19cee6241fc834346a5523013f` |

Verification scope: the upstream replay checks and XRush SQLite probes establish only the cases described. Full provider interoperability, kill/restart recovery, multi-process invalidation and end-to-end performance are implementation acceptance gates. The 20 external snapshots were reviewed statically at the cited paths. Their dependency suites, live services, licensing compatibility and latest remote revisions were not verified.

## 11. Full implementation beyond the first vertical slice

Stages 1–6 in §6 establish the shared host, storage and permission contracts. **Ship every capability below as part of the complete implementation.** A gate protects activation, data access, spending or rollout. An unmet gate leaves that capability open in the implementation ledger. Users can keep paid processing, remote storage, personal recall and other modes disabled indefinitely. Implement and test both the enabled and disabled paths.

The following are proposed engineering choices for Crush. Keep SQLite and the existing workspace API as the default deployment. Use one authoritative copy of each record per scope; derived indexes may be rebuilt. Put new features behind the same inspect, forget and deletion interfaces. Every async job and derived record carries a resolvable owner, source versions, effective policy generation, producer version and lifecycle state. Queues and ledgers carry scoped references wherever possible. Immutable job inputs mean a frozen version selection, not an irrevocable copy of sensitive content. Reauthorize before returning data and before using it in a request. Charge model usage to the purpose that incurred it.

### A. Multi-level context, pressure and rollover

**Prerequisite:** checkpoint publication, expansion and tool-group tests in §§3–7. Extend the rolling checkpoint with ordered child edges, level, branch, first/last covered event and coverage digest. Select adjacent, settled siblings on the same branch. Budget the condensation call independently; validate coverage against the union of children, then compare-and-swap the frontier. A parent may refer to several children. Require an acyclic graph, prohibit overlapping primary coverage and retain a bounded expansion traversal with cycle detection. A missing or expired original is reported at its actual position. Descendant deletion invalidates all covering ancestors before they can be selected. Keep one published checkpoint frontier per independently writable conversation lineage. When each branch is a distinct Crush session, the session ID is sufficient. Each request derives its own view; two sibling runs must never share a mutable active view. Git branch names and parent-session metadata cannot establish that two lineages inherited the same conversation.

Add compartments for current task, earlier tasks, repo evidence, preferences and archived details. Assign items by typed provenance and explicit task boundaries. Put each optional item at a chosen detail level (full, excerpt, summary or handle). Score optional items using task relevance, age, explicit importance and current budget pressure; record the contributions in the inspector. Apply stable tie-breaking and hysteresis between pressure bands to prevent churn near a threshold. Reserve mandatory content first. Decay changes presentation only; it cannot change a fact's authority, erase evidence or override a pin. Run proactive summarization on a frozen eligible prefix during idle time. Abort on invalidation, account for its cost and keep an on-demand path if it misses the deadline.

For rollover, atomically record source branch/frontier, a task note, retained tail and pending-operation handles under an idempotency key. Create the new session with explicit user/host authorization and a scoped handoff reference. Render and budget the recipient view before committing activation. If publication crosses stores, use stage H's recoverable handoff protocol. Preserve independent continuation of the original branch. Do not resume an external side effect whose outcome is unknown.

**Acceptance:** expand a three-level checkpoint to the original ordered IDs, invalidate a middle leaf, compare two siblings after a fork, oscillate the budget around a pressure threshold, interrupt proactive summarization, and retry an already-created rollover. Require one handoff, no duplicate coverage, no leaked sibling content and a legal outgoing tool history.

### B. Observations, reflection, evidence graph and historical queries

**Prerequisite:** versioned memory, review UI and deletion. Add an observation buffer keyed by scope and finalized frontier. Each observation carries a typed claim, source spans, evidence class, extraction run and review status. A reflector reads a frozen batch plus relevant accepted records. Its output proposes merge, supersede, contradict, support, split or retract operations with input IDs and a reason. Validate operation shape and sources; discard unknown IDs, cycles in supersession, stale input and unapproved scope expansion. Publish the changes in one transaction. Store ledger revisions with effective interval and recorded-at sequence. Represent uncertain dates explicitly. Store confidence as an attributed estimate; if showing calibrated confidence, version the calibration and validate it on labeled data.

Provide current and as-of queries: select only revisions known by the requested recording cutoff, applicable at the requested effective time, and visible under **present** access and deletion policy. Show conflicting live claims separately. Evidence edges point to typed, versioned sources or other claims; cap traversal depth and deduplicate source families. A verifier checks deterministic evidence first (exact quotation, source span, file revision, actual recorded test result). If enabled, use an independently budgeted model pass for semantic assessment and record its verdict as an assessment. Unverified work stays unverified. Provide inspection of evidence, correction and manual approval. On deletion remove or rebuild graph edges, snapshots, transitive derivatives and historical views.

**Acceptance:** submit competing time-scoped facts, apply a retroactive correction, query prior and current knowledge, and delete their shared source. Retest as-of queries, blocked evidence and caches. Duplicate or late reflector runs publish once or lose the revision check. A confident generated claim about passing tests cannot appear as a verified test run without corresponding evidence.

### C. Embeddings, vector storage, hybrid retrieval and reranking

**Prerequisite:** ordinary lexical writes and deletes pass the isolation tests. Chunk finalized, eligible representations by logical boundaries with source ID, version, UTF-8 range and chunk digest. For each embedding store scope, chunk key, model/version, dimension, normalization and indexing generation. Choose a local embedding backend or an explicitly authorized remote one. Remote embedding transmits source material and has a separate consent and usage control. Begin with scope-partitioned vectors and bounded exact similarity in the local store. Implement the indexed backend using PostgreSQL with pgvector as the reference deployment. Choose the distance metric and compatible index operator class for each model generation. Enforce scope in the database query and at result use. An approximate pgvector scan can apply its filter after scanning neighbors: a WHERE clause alone does not establish tenant-isolated candidate selection or sufficient recall. For isolation use separate security partitions/indexes or exact search over the eligible set. Within an authorized partition, bounded iterative scanning can improve recall where supported. Fall back to scoped exact search or explicitly mark incomplete results on exhaustion. Test with many closer disallowed vectors and a known allowed match; compare against the authorized exact baseline. [pgvector filtering and multitenancy](https://github.com/pgvector/pgvector#filtering) Expose a search/upsert/delete/rebuild interface, using stage H's replication contract when external. Keep incompatible model versions in separate index generations; backfill under budget, compare results, switch generation, and delete superseded vectors.

After authorization, retrieve a bounded lexical and vector candidate set. Use exact matches for handles and file identity. Fuse ranked lists deterministically (for example reciprocal rank fusion), deduplicate by source/version and apply the total result budget. Query expansion may generate bounded synonyms or language variants; bind each to the same scope and deadline. A reranker receives only eligible candidate IDs and bounded excerpts and returns an ordered subset of those IDs. Drop invented/duplicate IDs, append omitted candidates in baseline order, and fall back to lexical or fused order on timeout. Reauthorize joined records against the live database before exposing even a snippet. Report partial index coverage distinctly from zero matches.

**Acceptance:** compare lexical, vector, hybrid and reranked results on held-out exact identifier, paraphrase, multilingual and stale-revision queries. Record recall@k, ranking quality, latency and cost. Test two scopes containing the same phrase, model migration mid-query, deleted/stale vectors, reranker hallucinations, unavailable embedding provider, and concurrent reindex/delete. No unauthorized text may reach either model or caller.

### D. Speculative retrieval, read-only investigation and MCP interoperability

**Prerequisite:** scoped recall, cancellation, budgets and live-record checks. On eligible input, prefetch against a snapshot keyed by run, query, source/index generation and policy revision. Bound concurrency, bytes, tokens and wall time; register cancellation on task change. Never wait for optional prefetch on the critical path beyond its deadline. Check the snapshot and permissions again on consumption. A timed-out search may finish, but its stale result must not enter the request or become a durable cache entry.

Give the recall helper a restricted read-only tool set (search, scoped expand and evidence inspection), a delegated principal no broader than its caller, depth/step and spend limits, and an isolated request. Disable shell, file edits, tool replay, external messaging, recursive helper spawning and state mutation for that role. The helper returns evidence handles and quoted ranges plus explicitly labeled inferences. Validate returned handles and citations at the host. A helper request incurs separately visible usage and requires the applicable model/egress permission.

Expose the same bounded read-only search/expand contracts via an opt-in **MCP server** backed by the workspace service. The existing MCP client/tool integration consumes external tools; implement the server endpoint separately. Negotiate supported protocol and schemas, authenticate the connection, map it to allowed scopes and reject unauthorized scope widening. Use opaque handles, bounded pages, cancellation and per-client rate limits. For stdio, bind the launched process to an explicit workspace/principal and keep logs off protocol stdout. For Streamable HTTP, bind local-only deployments to loopback, validate Origin when present and authenticate requests independently of client-supplied session IDs. Test invalid origin, wrong principal, revoked grant, cached cursor reuse and valid reconnect. Pin the negotiated protocol revision in fixtures. [MCP transport specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) MCP clients have no authority to change learning, data retention or sharing by embedding instructions in a query.

**Acceptance:** cancel prefetch immediately before a source is deleted; simulate a backend that ignores cancellation; call helper and MCP with another session's handle; inject a recalled excerpt that asks the helper to execute a command. Verify zero side effects and no disclosures. Compare MCP and native responses for the same authorized request and verify revoke/reconnect behavior.

### E. Dynamic tool catalog, stable provider caches and usage attribution

**Prerequisite:** adapter-level capture of actual tool schemas, request order and runtime allow lists. Build a versioned manifest from currently enabled native and MCP tools, including name, source, description/schema digest and capability constraints. Start each dispatch from the effective allowed set. Keep mandatory and in-flight tools present. For compatible adapters, expose a bounded discovery interface returning eligible names and summaries; activate selected definitions on the next legal step. Revalidate permissions and fetch the exact canonical executable schema before advertising and before executing. If a definition changes, invalidate the manifest and replan. Where supported, shorten only descriptive prose under a versioned policy. Retain native validation, defaults, types, required fields, permission prompts and host-side executor checks. A provider lacking legal dynamic selection uses the full authorized catalog within budget.

Build an adapter-specific prefix policy from provider capabilities. Preserve stable messages and tool order where protocol allows; place permitted cache markers deterministically; key by provider/account, selected model, schema/policy version and isolated scope. Invalidate on source changes, revocation and capability changes. Never reorder a signed continuation or expose a cache key derived from confidential plaintext to another scope. Separate predicted reusable prefix length, requested cache behavior and actual reported cache reads/writes. Missing provider cache metrics remain unknown.

Record per request/attempt the purpose (primary, summary, extraction, reflection, embeddings, reranking or helper), model, token estimates by component, reported input/output/cache/reasoning fields, actual charge or rate-card version, and outcome. Define each provider's usage semantics to avoid double counting included cache/reasoning tokens. Distinguish an estimate, a known flat-rate plan, a reported zero and an unknown cost. Attribute retries and background calls to their parent once; handle duplicate terminal events idempotently. Debug views explain both gross and marginal cost with redacted metadata.

**Acceptance:** vary MCP tools between steps, revoke a discovered tool before execution and require refusal, replay signed content with cache optimization on/off, and compare canonical schemas and provider wire fixtures. Use a controlled workload to compare selection accuracy and cache/cost effects. Reconcile reported totals with fixtures containing cache hits, missing fields, cancellation, refresh retries and duplicate accounting events.

### F. Cross-project personal memory and controlled export

**Prerequisite:** scoped current/as-of lookup, user review and invalidation. Implement a distinct user-owned personal namespace outside project-local databases. Promotion is an explicit action showing the claim, evidence scope, recipients and whether a model or external service would receive its text. Store a grant with owner, readable projects/principals, operations, revision and revocation state. A personal record can carry a user-authored durable assertion while its raw source remains project-private. A grant to the claim never implicitly grants access to its sources. Read from session, project and authorized personal namespaces separately, then fuse only eligible candidates. Child sessions and helpers inherit at most the caller's grants.

For cross-store derivations, keep origin handles and revocation generations, and use a durable invalidation event. Make the origin authority participate in admission for external disclosures, including submissions to a remote embedder or reranker. A replica watermark measures events processed; by itself it cannot prove that no newer revocation exists. Fail closed on origin outage for origin-dependent content; retain offline access to unrelated, locally owned sources. Apply events with per-origin monotonic revisions, detect sequence gaps and fetch missing state. An older upsert must not overwrite a newer tombstone. Require this check on fallback, cache, as-of and import paths as well. On grant removal, delete or invalidate downstream excerpts, vectors and caches. Offer owner-scoped export/import with explicit destinations, representation version and provenance manifest. Validate imports, detect older source generations, use new scoped IDs where identities collide, and require explicit handling of missing origins and pre-deletion backups. Surface the boundary for user-owned external copies.

**Acceptance:** promote one claim from project A to B while withholding raw evidence, decline a helper request for that evidence, revoke the grant while B is offline, then reconnect and attempt native, vector, MCP and historical recall. No path returns the revoked content. Import an old export into a fresh store without silently granting former permissions.

### G. Provider expansion and conformance

**Prerequisite:** one first-provider vertical slice. For each Crush provider/adapter family supported at the target revision, enumerate actual request entry points and features in a checked-in capability matrix. Include compatible endpoints with limited or unknown capabilities. Record window topology, tokenizer/estimator, omitted versus explicit output cap, media and reasoning accounting, tool schema translation, parallel result rules, signed/opaque continuation, overflow classification, cache metadata and retry semantics. Implement per-capability behavior at the model/adapter boundary with fixture-backed defaults. Never guess support from a model name alone. Run the same logical scenario on each supported adapter and capture its own translated requests. Explicitly distinguish strict protection, conservative estimated behavior and provider-incompatible operations. An unverified adapter row stays open in the release ledger; it cannot be counted as supported UMCA coverage.

**Acceptance:** for every row, test over-budget mandatory input, a resumable tool exchange, model change/credential refresh and cancellation. Include media/signatures/cache fixtures where applicable; prove safe fallback where unavailable. An opt-in live smoke test checks evolving provider behavior with synthetic content and controlled spending. Record adapter version, test method and last verified capability date.

### H. Additional backends, distributed execution and crash recovery

**Prerequisite:** transactional local conformance and traceable deletion. Extract a small store contract for snapshot reads, conditional publication, scoped lookup, tombstones, job claims and outbox replay. Keep existing Crush transcript ownership intact. Implement PostgreSQL as the second transactional UMCA backend with equivalent constraints, ordered source coverage, compare-and-swap and scoped indexes. Add an external vector-index adapter as a derived projection; the transactional backend remains authoritative. For separate host and UMCA databases, persist a source-mutation/outbox record alongside the host mutation, replicate idempotently, and validate origin revision at publication/read. If a backend cannot maintain this invariant, disable affected operations with an explicit capability result. Implement snapshot export, checkpointed migration/backfill, catch-up to a recorded watermark, validation and an atomic authority switch. During cutover keep only one write authority; rollback replays post-cutover changes or stays on the new authority until reconciliation succeeds.

Persist jobs as queued/running/succeeded/failed/cancelled with operation key, purpose, immutable inputs, policy generation, attempt count, owner, lease expiry, fence and last error. Claim transactionally using database time; increment the fence on every acquisition. A heartbeat renews only a still-valid lease for its current owner/fence. The publication transaction requires running state, matching owner/fence, an unexpired lease evaluated using database time at the conditional update, and current source/policy generations. An expired worker loses write authority even if no replacement has claimed its job. Transient retries renew neither source authority nor permission to spend. Test stale publication both before and after reassignment. Expiry permits reassignment; the old worker cannot publish after losing ownership. Commit result, state change and outbound event together. Relay outbox events to the distributed queue with stable IDs; consumers deduplicate and acknowledge only after durable application. Bound retries, support cancellation and dead-letter inspection/replay, and prioritize interactive work without starving maintenance. Implement a local worker and a separately runnable worker using this same protocol; use RabbitMQ as the reference broker transport with durable queues, publisher confirms, bounded consumer prefetch, explicit acknowledgments and dead-letter routing. Publish persistent events to verified durable topology using mandatory routing and correlate returns, negative acknowledgments and publisher confirms to the outbox ID. An unroutable publish can still receive a positive broker confirmation. Mark the outbox row relayed only after confirmation with successful routing. Retain and retry failed/uncertain sends; alert on broken bindings. Persist consumer deduplication and the domain change in the same transaction, then acknowledge. Track relayed and applied separately. Test a missing binding, a lost confirm and a consumer crash after commit but before acknowledgment. [RabbitMQ confirmation semantics](https://www.rabbitmq.com/docs/confirms#when-publishes-are-confirmed) Version event payloads and test mixed-version rolling upgrades. Scope service credentials by tenant/operation. Remote services and additional processing costs require explicit deployment configuration.

Keep an operation ledger for scheduling, model calls, tool effects and recovery. Distinguish accepted, dispatched, observed result, outcome-unknown and terminal states. Use provider idempotency keys only where supported; an expired lease cannot justify repeating an external effect. Reconcile pending artifacts, orphan files, unpublished summaries, stale jobs, index lag and deletion events at startup. Offer preview and repair modes with content-free reports. Verify backups on an isolated restore, reapply revocation high-water marks and deny recall if required deletion history is unavailable. Test restoration and rollback procedures with actual fixtures, recording observed recovery time and loss window.

**Acceptance:** run one conformance suite against SQLite and PostgreSQL and both worker modes. Terminate processes before/after blob rename, checkpoint commit, outbox send, queue acknowledgment and deletion. Partition a worker, expire its lease, let another complete, reconnect the old one and prove only the current fence can publish. Inject duplicate/out-of-order deliveries, broker outage, exhausted quota and backend migration interrupted during live updates. Require no resurrection, duplicate terminal state, or automatic repeat of an unknown external effect. Measure backlog drainage, interactive latency and recovery behavior.

### Integration contracts shared by the advanced stages

**User and agent entry points.** Register read-only `context_search` and `context_expand` through the normal tool resolver with the same allowed-tool filtering as native tools. Proposed search input is query, requested scope, kinds, bounded limit and cursor. Return source handle/version, timestamp, kind, representation, capture status, bounded snippet, retrieval method and continuation/completeness. Expand accepts a scoped handle and either half-open byte offsets or a bounded line interval for a known text representation; resolve line boundaries against that immutable version. Reject mixed units, arbitrary filesystem paths and invalid ranges. Both tools apply server-side limits even if the caller asks for more. For MCP, use the same operations and response semantics. Return an explicit unavailable/incomplete status when a gate closes; do not substitute invented evidence.

**Capture and egress policy.** Derive a searchable projection from explicitly allowed text parts. Exclude opaque reasoning signatures, binary encodings, credential fields and serialized metadata from search, embeddings, helper input and export unless a specific safe representation is approved. Preserve provider-required opaque data only under the original history/continuation policy. Apply capture exclusions before durable storage. Mark a filtered representation redacted or partial, record the policy version, and never promise byte-exact recovery of removed content. Index deletion and policy tightening must invalidate derived vectors, queued model inputs, receipts containing source excerpts, and shared-store copies. Tests seed deterministic synthetic secrets in allowed text, excluded fields and stderr; verify the configured exclusions across native search, external model inputs, MCP, diagnostics and exports. Pattern matching cannot guarantee detection of every secret.

**Stage rollout and evidence.** For each A–H stage, add a default-safe configuration path, a deterministic integration fixture and a rollback exercise. Expose switches through Crush configuration plus workspace-visible state. Separate permissions for capture, background model spending, remote embedding/reranking, remote storage, personal sharing and MCP exposure. A deployment choice cannot grant a previously denied content permission. On schema/index upgrades build a shadow generation, validate coverage and expected results, switch the reader atomically and retain a bounded rollback window. A rollback must enforce all current revocations. An operator must be able to inspect failed items, retry idempotently, rebuild a derivative index from authorized sources and distinguish a service outage from no matching evidence.

### Boundary tests for the complete implementation

| Boundary | Test setup and observable result |
|---|---|
| Streaming source → checkpoint | Pause a debounced message update and an in-flight flush. Finalize only after the required writes are visible. Use the message service flush barrier before opening the checkpoint transaction; never call a pool-backed flush from inside a transaction holding the only connection. Race deletion with the barrier. Assert the covered version and digest match the final native parts, or that publication loses, with no resurrected row. |
| Request → derivative → delete | Store host-owned reads-from edges from recall/expanded artifacts to their tool-result parts and downstream generated groups. A content-free receipt can hold IDs, but it must survive long enough to invalidate retained derivatives; ordinary diagnostics remain optional. If precise influence is unknowable, conservatively associate the response with the set of controlled sources admitted to its request. Invalidate affected outputs before they can be replayed or indexed. Never infer that independently authored user text has the same provenance from matching words alone. |
| Time, authority and audit data | Revoke a shared claim during prefetch, between ANN selection and reranking, and between prompt preparation and a transport retry. Verify that the denied source reaches no new admission, including diagnostic/raw exports. Test logs, broker payloads, dead-letter records, embeddings and receipts under retention and erasure. A cost/usage ledger can retain non-content aggregates under policy without storing source text or a reversible content fingerprint. |
| Fenced rollover and migration | Commit a pending handoff at the source with an idempotency key and validated frontier. At the destination create an inactive recipient by unique key. Recheck source authority and bounds before publishing an activation decision; redeliver that decision idempotently. Expose only activated recipients, revalidate at use, and revoke or collect abandoned pending records. Crash at each boundary. During store migration, use a durable authority epoch, fence the old writer at the cutover barrier and catch the target up through that barrier before enabling the new writer. A disabled old writer cannot publish stale jobs after reconnection. |
| Release evidence and user benefit | For each A–H stream, test both switch states and the outage fallback from the ordinary workspace entry point. Keep consent separate from capability detection. Use matched workloads and record sample counts, retrieval labels, result correctness and total usage including rejected summaries, background calls and retries. Report incomplete measurements, unknown prices and cases where simpler operation wins. Mark a capability implemented only when its behavioral fixtures pass; enable paid or cross-scope behavior only on the user's effective policy. |

### Completion and operation

A–H are required implementation streams. A and B build on checkpoint and memory storage; C and F build on scoped recall and invalidation; D consumes the bounded recall API; E and G share adapter fixtures; H supplies cross-store and multi-worker durability. An implementer can use a local transactional fake for cross-store events during development, but F and external projections cannot pass their deployment gates until H's partition and recovery tests pass. Each implementation PR names its storage migrations, configuration/consent switches, user-visible entry point, metrics, rollback procedure and acceptance fixtures. Expose per-capability status in inspection: implemented/version, enabled for this caller, unavailable reason, last successful validation, cost/retention policy. Disabling a capability stops new jobs and publications, preserves authorized inspection/deletion, and falls back to the tested simpler path. The complete release requires §§6–7 plus A–H demonstrated through their real host entry points. A disabled-by-default implementation needs a passing enabled-path test.
