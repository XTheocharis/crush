# Unified Agent Extension Spec (UAES) Implementation via Extism

> **STATUS: ARCHITECTURAL PROPOSAL / DESIGN SPECIFICATION**
> This document describes the *target* architecture for the Crush extension system. The current `xrush` fork implements LCM and RepoMap using native Go hooks (`PrepareStepHook`). This specification provides the roadmap for migrating those hard-coded features into a universal Wasm-based plugin system.

## 1. Architectural Strategy: The Extism Middleware Pipeline

To implement the exhaustive **Unified Agent Extension Spec (UAES)** within Crush using Extism plugins, the architecture acts as a **middleware pipeline** and an **event observer** spanning the entire application lifecycle.

The Go binary essentially becomes a concurrent hypervisor orchestrating the execution of WebAssembly lifecycle interceptors, while also managing pure declarative packages (like Markdown agents and TOML policies) natively.

### Declarative vs. Imperative Loading
Extism is only used for the *imperative* components of the UAES (hooks, programmable tools, dynamic MCP servers). The Go host natively parses the `uaes.json|yaml` manifests to register declarative components (Agent Definitions, static commands, UI themes, Workflow Graphs). Wasm modules are only loaded and compiled when a package manifest explicitly registers a `handler` pointing to Wasm.

### Concurrency and Instance Pooling
Extism plugin instances are **stateful and not thread-safe**. To support concurrent events without memory corruption, Crush follows this pattern:
1. Crush compiles the Wasm module once via `extism.NewCompiledPlugin`.
2. For every hook invocation, Crush borrows a lightweight instance from a concurrent-safe pool (e.g., a mutex-protected `sync.Pool` wrapping `compiled.Instance()`).
3. **WASI Isolation:** The host MUST set `EnableWasi: true` to satisfy PDK initialization requirements (clocks, entropy). However, Extism's Go SDK defaults to a **deny-by-default** filesystem policy. Crush MUST NOT provide `AllowedPaths` unless explicitly requested by the user, ensuring a "Zero-Trust" host sandbox for untrusted plugins.
4. **State Isolation & Linear Memory:** Instances in a pool have entirely isolated linear memory. While the Extism kernel **auto-reclaims** memory allocated via `p.Alloc()` at the start of the next call, `p.Var` persistence is instance-specific.
5. **State Hydration for Re-entrancy:** Because re-entrant calls borrow a *different* instance from the pool, local guest variables will not persist across the re-entrant boundary. Wasm plugins requiring shared state across `around` hooks **MUST** use the `crush:host/session:patch_metadata` Host ABI to serialize state to the Go Host before the blocking call and retrieve it upon resumption.

### Native Wasm Execution Limits and Host Concurrency
Wasm execution within Extism is strictly synchronous call/return without native stack-switching. However, Crush's architecture (using Bubble Tea) gracefully accommodates blocking operations.
*   **UI Elicitation & Deadlock Mitigation:** A Wasm plugin **can safely pause**. When a plugin needs UI input, it calls a blocking Host Function (`crush:host/ui:elicit`). This blocks only the background agent goroutine running that specific Wasm instance. 
    *   *TIMEOUT MITIGATION:* The Go host MUST pause the plugin's execution `Timeout` during blocking Host Functions to prevent the plugin from being terminated while waiting for human interaction.
    *   *DEADLOCK PREVENTION:* Host Functions must monitor `ctx.Done()` to ensure goroutines and instances are released if the session is aborted.
*   **`around` Hooks & Re-entrancy:** Because Wasm cannot yield execution and resume while preserving its local call stack, `around` hooks are implemented via a synchronous Host Function. The plugin performs its `before` logic, calls `crush:host/core:invoke_downstream`, blocks while the host executes the target action, and upon return, executes its `after` logic.

### Memory Management in Host Functions
While the Extism SDK automatically abstracts memory for top-level `Plugin.Call()` outputs, **Host Functions** require explicit, stack-based pointer management (`uint64` memory offsets).
1. **Auto-Reclamation Safety:** The Host can safely allocate memory for Guest results; the Extism kernel will auto-free these allocations at the beginning of the plugin's next turn.
2. **Intra-Call Safety:** For long-running loops *within* a single Wasm call, the Guest MUST manually call `mem.Free()` to prevent heap exhaustion.
3. **Directional Ownership:**
    *   **Guest → Host (Inputs):** The Guest retains ownership of the `request_ptr` and must `Free` it after the Host Function returns.
    *   **Host → Guest (Outputs):** The Host allocates the `response_ptr`. The Guest MUST call `mem.Free()` on that offset after reading the JSON payload.
4. **Serialization Efficiency:** While JSON is the baseline, the host's **LCM (Lossless Context Management)** ensures the message history in the `HookContextEnvelope` is capped at ~128k tokens (~1MB). For future high-frequency hooks, the ABI may transition to Zero-Copy Binary Formats (FlatBuffers) over shared memory offsets to bypass JSON parsing overhead.

---

## 2. Comprehensive Hook Injection Mapping

### Phase 0: Canonical Identity & Extension Loading (UAES Sec 2)
*   **`extension.install` / `extension.update` / `extension.enable` / `extension.disable`**

### Phase 1: Configuration, UI, & Initialization (UAES Sec 3 & 14)
*   **`config.change.before` / `config.change.after`**
*   **`ui.theme.changed`**

### Phase 2: Session, Context, & Memory Lifecycle (UAES Sec 4 & 7.2)
*   **`session.start` / `session.resume` / `session.clear` / `session.end`**
*   **`context.discover.before`**: Intercepts the discovery phase to inject virtual context files or alter path resolution rules (extends the base UAES taxonomy).
*   **`memory.read.before` / `memory.write.after` & `context.loaded`**

### Phase 3: User Input, Commands, & Skills (UAES Sec 5 & 6)
*   **`prompt.submit.before` / `prompt.submit.after`**
*   **`command.invoke.before` / `command.invoke.after`**
*   **`skill.activate.before` / `skill.activate.after`**

### Phase 4: Agent Orchestration (UAES Sec 9)
*   **`agent.start` / `agent.stop` / `agent.idle` / `agent.error`**
*   **`agent.transfer.before` / `agent.transfer.after` / `task.completed` / `task.failed`**

### Phase 5: The LLM Boundary (UAES Sec 7.2)
*   **`model.request.before` / `model.request.after` / `model.response.before-delivery` / `model.response.retry`**
    *   *Note:* The `HookContextEnvelope` passed to these hooks contains message history already processed (truncated/summarized) by the host's **LCM** service. Hooks returning `HookResult` objects can trigger powerful mutations like `tailCalls`, `permissionUpdates`, and `envExports`.

### Phase 6: Tools, Permissions, & Sandbox (UAES Sec 8 & 11)
*   **`tool.selection.before`**
*   **`policy.evaluate.before`**: Intercepts the UAES Policy Engine evaluation phase before permissions are requested, allowing external policy services to inject dynamic rules (extends the base UAES taxonomy).
*   **`safety.check`**: Safety Checkers evaluate components using a `HookSelector` prior to execution. They are generic rule evaluator mechanisms that can apply to tools, commands, agents, or arbitrary event patterns, with specific routing on pass/fail (`ask`, `rewrite`, `sandbox-only`).
*   **`permission.request` / `permission.decision`** (Interfaces with diverse reviewers: human, guardian subagents, policy services).
*   **`tool.call.before` / `tool.call.after` / `tool.call.error`**
    *   *Note on Sandboxing:* Extism's `Manifest.AllowedHosts` and `Manifest.AllowedPaths` are bound at *compilation time* and cannot be dynamically changed per-request. Therefore, dynamic per-request sandboxing (such as per-agent worktree isolation) **cannot** rely solely on Extism's manifest boundaries. To maintain RepoMap ranking and security, Wasm tools SHOULD NOT use native WASI filesystem access. Instead, the Go Host MUST manually validate and track filesystem requests passed over Host ABIs (e.g., `crush:host/fs:write`) to sync with the `filetracker`.
*   **`worktree.create.before` / `worktree.create.after` / `worktree.remove.before` / `worktree.remove.after`**

### Phase 7: MCP Integration (UAES Sec 10)
*   **`mcp.server.connect` / `mcp.server.disconnect` / `mcp.tools.changed`**
*   **`mcp.resources.changed` / `mcp.resource.updated` / `mcp.prompt.invoked`**
*   **`mcp.elicitation.request` / `mcp.elicitation.result`**
*   *(Note: MCP sampling uses standard UI elicitation paths or `mcp.elicitation.*` instead of non-canonical events).*

### Phase 8: xrush Core Subsystems (Implementation Specific)
*   *Note:* The events below are internal to xrush and extend the base UAES taxonomy. They represent the migration path for current native Go hooks.
*   **`xrush.repomap.refresh.before` / `xrush.repomap.rank.evaluate` / `xrush.repomap.render.after`**: Crucial for intercepting the dynamic ranking index before it's injected via `PrepareStepHook`.
*   **`session.compact.before` / `session.compact.after`** (Standard UAES) and **`xrush.lcm.summarize.before` / `xrush.lcm.summary.generated`** (xrush specific).
*   **`xrush.file.read.after` / `xrush.file.modified.after`**: Hooks ensuring that filesystem activity performed by Wasm tools is synced with the host's filetracker.

### Phase 9: Additional Observability (UAES Sec 12.3)
*   **`agent.*` / `tool.*` / `permission.*` / `prompt.*` / `message.*` / `memory.*`**
*   **`config.*` / `ui.*` / `extension.*` / `worktree.*` / `pty.*` / `filesystem.*` / `project.*` / `telemetry.*` / `server.*` / `error.*`**

### Phase 10: Event Bus & Headless SDK (UAES Sec 12 & 13)
*   **`ui.notification` / `ui.toast` / `notification.emit` / `system.error`**
*   **SDK Control Plane:** The Go host exposes the full `RuntimeClient` via local HTTP/WebSocket/pipes. *IMPLEMENTATION NOTE:* This requires introducing a background daemon/server layer to `internal/app/` to serve external SDK requests.

---

## 3. Extism Wasm Export Contracts (What Crush Calls)

To comply with the strict Extism ABI, **all exported Wasm functions must take exactly zero arguments and return an `int32` status code (0 for success, 1 for failure).** 

All JSON data exchange is performed using `pdk.Input()` to read the host's input payload and `pdk.Output()` to send the response. 

**Capability Verification:** Because plugins may implement only a subset of these interfaces, the Crush host MUST verify capability using `Plugin.FunctionExists(name)` prior to any invocation to avoid runtime errors.

### MCP Server Role Exports
To act fully as an embedded MCP Server (UAES 10.1), the Wasm plugin exports the relevant server protocol surface.
*   **`export fn get_tools() -> int32`**
*   **`export fn execute_tool() -> int32`**
*   **`export fn get_resources() -> int32`**
*   **`export fn read_resource() -> int32`**
*   **`export fn unsubscribe_resource() -> int32`**
*   **`export fn get_resource_templates() -> int32`**
*   **`export fn subscribe_resource() -> int32`**
*   **`export fn get_prompts() -> int32`**
*   **`export fn get_prompt() -> int32`**
*   **`export fn handle_command() -> int32`**: (UAES dispatcher for MCP Task/Sampling primitives).
*   **`export fn complete() -> int32`**

### Lifecycle Hook Exports (The Pipeline)
*   **`export fn hook_before() -> int32`**: Reads `HookContextEnvelope`, writes `HookResult`.
*   **`export fn hook_after() -> int32`**
*   **`export fn hook_around() -> int32`**
*   **`export fn hook_replace() -> int32`**
*   **`export fn hook_observe() -> int32`**: Reads envelope, no output required.
*   **`export fn evaluate_safety() -> int32`**: Dedicated export for Phase 6 Safety Checkers.

---

## 4. Extism Host ABI Requirements (What Plugins Call)

Crush provides a rich set of Host Functions to the Wasm sandbox, utilizing Extism's stack-based ABI (`uint64` PTRs for memory offsets). The plugin and host must diligently apply the directional `Free()` ownership rules described in Section 1.

### Core Orchestration
*   **`crush:host/core:invoke_downstream(request_ptr) -> response_ptr`**
    *   **CRITICAL:** Allows Wasm plugins implementing `around` hooks to block execution while the Go host runs the target lifecycle action, returning the result to the same call stack.
*   **`crush:host/tool:call(request_ptr) -> response_ptr`**
*   **`crush:host/llm:prompt(request_ptr) -> response_ptr`**
*   **`crush:host/agent:spawn(agent_def_json_ptr) -> result_json_ptr`**
*   **`crush:host/agent:wait(agent_id_ptr) -> result_json_ptr`**
*   **`crush:host/agent:send_input(agent_id_ptr, input_ptr)`**
*   **`crush:host/agent:close(agent_id_ptr)`**

### Domain-Specific Relational Persistence
*(Note: A flat KV interface creates severe Read-Modify-Write race conditions for concurrent pools and ignores Crush's relational `sqlc` schema. The ABI must provide semantic, transaction-safe queries. IMPLEMENTATION NOTE: This requires new SQLite migrations in internal/db/).*
*   **`crush:host/history:query(query_json_ptr) -> results_ptr`**
*   **`crush:host/session:get_metadata(key_ptr) -> value_ptr`**
*   **`crush:host/session:patch_metadata(patch_json_ptr)`**
    *   Replaces `set_metadata` to perform atomic updates using JSON Merge Patch (RFC 7396) or JSON Patch (RFC 6902).
*   **`crush:host/memory:add_topic(scope_ptr, topic_json_ptr)`**
*   **`crush:host/memory:query(scope_ptr, query_json_ptr) -> results_ptr`**
*   **`crush:host/workflow:get_state(workflow_id_ptr) -> state_json_ptr`**
*   **`crush:host/workflow:transition_state(workflow_id_ptr, update_json_ptr)`**
*   **`crush:host/workflow:terminate(workflow_id_ptr, mode_ptr)`**
*   **`crush:host/policy:dispatch_reviewer(reviewer_id_ptr, request_ptr) -> decision_ptr`**

### xrush Core Subsystems (LCM, RepoMap, Tree-Sitter, Filetracker, LSP, PubSub)
*   **`crush:host/repomap:query(context_ptr) -> map_json_ptr`** / **`crush:host/repomap:trigger_refresh()`**
*   **`crush:host/lcm:fetch_large_output(uuid_ptr) -> text_ptr`**
    *   Retrieves massive tool outputs stripped from the context window by LCM.
*   **`crush:host/lcm:get_summary(context_ptr) -> text_ptr`**
*   **`crush:host/treesitter:parse(file_path_ptr) -> ast_handle_ptr`**
    *   Returns an opaque `uint64` handle to a host-side AST to ensure memory efficiency.
*   **`crush:host/treesitter:query(ast_handle_ptr, query_ptr) -> matches_ptr`**
*   **`crush:host/treesitter:get_node_text(ast_handle_ptr, node_ptr) -> text_ptr`**
*   **`crush:host/treesitter:free(ast_handle_ptr)`**
    *   **CRITICAL:** Must be called by the Guest to release host-side resources and prevent memory leaks.
*   **`crush:host/filetracker:mark_touched(path_ptr)`**
    *   Ensures file activity performed by plugins maintains RepoMap ranking heuristics.
*   **`crush:host/lsp:query(method_ptr, params_ptr) -> result_ptr`**
*   **`crush:host/pubsub:subscribe(topic_ptr)`** / **`crush:host/pubsub:poll() -> event_ptr`**

### Advanced MCP Primitives (UAES 10.2)
*   **`crush:host/mcp:sample(request_ptr) -> response_ptr`**
*   **`crush:host/mcp:read_resource(uri_ptr) -> resource_ptr`**
*   **`crush:host/mcp:subscribe_resource(uri_ptr)`**
*   **`crush:host/mcp:call_prompt(name_ptr, args_ptr) -> prompt_ptr`**
*   **`crush:host/mcp:emit_progress(progress_json_ptr)`**
*   **`crush:host/mcp:emit_log(log_json_ptr)`**
*   **`crush:host/mcp:notify_tools_changed()`**
*   **`crush:host/mcp:notify_resources_changed()`**
*   **`crush:host/mcp:notify_prompts_changed()`**
*   **`crush:host/mcp:get_roots() -> roots_json_ptr`**

### UI & Observability
*   **`crush:host/ui:get_state() -> state_json_ptr`**
*   **`crush:host/ui:render(markdown_ptr)`**
*   **`crush:host/ui:elicit(prompt_ptr) -> response_ptr`**
    *   Synchronously requests user input, blocking the Wasm goroutine while the Bubble Tea TUI handles the interaction.
*   **`crush:host/event:publish(envelope_json_ptr)`**