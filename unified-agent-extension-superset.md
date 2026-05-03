
# Unified Agent Extension Spec (UAES) — A Feature-Complete Superset

## 0. Intent

This specification is a canonical superset of the extension surfaces described across Claude Code, Gemini CLI, OpenCode, Codex CLI, the Claude Agent SDK, the OpenCode SDK, Google ADK, the OpenAI Responses API usage pattern in Codex, and the MCP Go SDK. It is intentionally **not source-compatible** with any one system. Instead, it defines a single runtime model that can *compile from* multiple authoring syntaxes and *project to* multiple runtimes.

The core design choice is:

1. **Canonical runtime model**: one internal graph of config, commands, skills, hooks, tools, agents, policies, events, UI, and MCP integrations.
2. **Multiple authoring frontends**: JSON, JSONC, TOML, YAML, Markdown frontmatter, and executable modules all compile into the canonical model.
3. **Multiple execution backends**: subprocess, HTTP, SSE, WebSocket, in-process module hooks, SDK clients, and MCP client/server roles all bind to the same model.

---

## 1. Design Principles

### 1.1 Hard requirements

The superset must satisfy all of the following simultaneously:

- Layered config with user, project, local, system, managed, env, and CLI overrides
- Special context/instructions files with eager and lazy loading, imports, and path-scoped rules
- Custom slash commands with arguments, file injection, shell injection, and tool pre-approval
- Skills with both manual and automatic activation
- Hooks that can observe, block, transform, or synthesize behavior
- Built-in and custom tools, including MCP-backed tools and file/module-based tools
- Multi-agent orchestration with local and remote agents
- MCP integration beyond tools, including resources, prompts, sampling, roots, elicitation, progress, and subscriptions
- Extension packaging, distribution, trust, and managed lockdown
- Tiered permissions, approvals, safety checkers, and sandboxing
- External SDK surfaces and streaming APIs
- Observable event bus
- UI/theme extensibility and headless output

### 1.2 Guiding rules

- **Typed first, scriptable second**: everything has a declarative schema, but every major surface also permits imperative handlers.
- **One event bus**: hooks, observers, UI notifications, SDK streams, and headless mode are all projections of the same event model.
- **Policy before execution**: tool/agent/connector calls pass through policy, sandbox, and hook pipelines before side effects occur.
- **Agent/tool symmetry**: any agent may be exposed as a tool; any tool may internally delegate to an agent.
- **Full content model**: text, structured JSON, images, audio, files, resources, artifacts, tool calls/results, and reasoning summaries are first-class.
- **MCP as a native substrate**: the runtime must support the full 2025-11-25 MCP surface as a client and optionally as a server.

---

## 2. Canonical Identity and Packaging Model

## 2.1 Canonical IDs

Every contributed object has a stable canonical ID:

```text
<kind>:<namespace>/<name>@<version>
```

Examples:

- `command:core/release@1.0.0`
- `skill:acme/security-audit@2.1.0`
- `agent:org/code-reviewer@2026.03`
- `tool:builtin/shell@1`
- `extension:marketplace/acme-devtools@3.4.2`
- `mcp-server:workspace/github@1`

Rules:

- `kind` is required and controls validation
- `namespace` is required and prevents collision
- `name` is human-readable and stable
- `version` is optional at authoring time, required after packaging

Human-facing aliases may still exist:

- slash command alias: `/release`
- namespaced command alias: `/acme:release`
- agent alias: `@code-reviewer`
- MCP tool alias: `github.list_repos`

The runtime resolves aliases to canonical IDs.

## 2.2 Canonical package types

The system supports five package kinds:

1. **Extension package** — a directory/archive that can contribute any declarative assets
2. **Plugin module** — executable code module contributing hooks/tools/checkers/providers
3. **Skill bundle** — distributable skill pack
4. **Agent pack** — distributable agent collection
5. **Policy pack** — distributable rules/checkers/managed controls

## 2.3 Package manifest

```ts
interface ExtensionManifest {
  id: string
  version: string
  displayName?: string
  description?: string
  authors?: Array<{ name: string; email?: string; url?: string }>
  homepage?: string
  repository?: string
  license?: string
  keywords?: string[]

  source?: {
    type: 'marketplace' | 'github' | 'git' | 'npm' | 'url' | 'file' | 'directory' | 'symlink'
    ref?: string
    releaseTag?: string
  }

  compatibility?: {
    runtimeVersion?: string
    platforms?: string[]
    featureFlags?: string[]
  }

  components?: {
    config?: string[]
    context?: string[]
    commands?: string[]
    skills?: string[]
    agents?: string[]
    hooks?: string[]
    policies?: string[]
    tools?: string[]
    mcp?: string[]
    themes?: string[]
    sdk?: string[]
  }

  install?: {
    scopes?: Array<'system' | 'user' | 'workspace' | 'session'>
    autoUpdate?: boolean
    allowPrerelease?: boolean
    linkMode?: boolean
    settings?: ExtensionSetting[]
  }

  trust?: {
    requiresApproval?: boolean
    fingerprints?: string[]
    signatures?: string[]
    sensitivePermissions?: string[]
    managedOnly?: boolean
    allowMarketplaceOnly?: boolean
  }

  migration?: {
    migratedTo?: string
    replaces?: string[]
  }
}

interface ExtensionSetting {
  name: string
  description: string
  key: string
  envVar?: string
  sensitive?: boolean
  required?: boolean
  default?: string | number | boolean | null
}
```

## 2.4 Standard package layout

```text
extension-root/
├── uaes.json|jsonc|toml|yaml              # top-level config / manifest entrypoint
├── context/
│   ├── AGENT.md
│   ├── CONTEXT.md
│   └── rules/
├── commands/
│   ├── release.md
│   └── git/commit.md
├── skills/
│   └── security-audit/SKILL.md
├── agents/
│   └── code-reviewer.md
├── hooks/
│   ├── hooks.yaml
│   └── *.ts|*.js|*.py
├── tools/
│   └── *.ts|*.js|*.py
├── policies/
│   └── *.toml|*.yaml
├── mcp/
│   └── servers.yaml
├── themes/
│   └── *.json
├── sdk/
│   └── generated/
└── scripts/
```

---

## 3. Unified Configuration Model

## 3.1 Authoring formats

The canonical config may be authored as:

- JSON
- JSONC
- TOML
- YAML
- Markdown with YAML/TOML frontmatter
- executable module returning config
- inline env var fragments
- CLI `-c path=value` overrides

The runtime compiles all sources into a single canonical config tree.

## 3.2 Scope layers and precedence

The superset supports all existing precedence patterns by normalizing them into this order, lowest to highest:

1. built-in defaults
2. remote organization defaults (`/.well-known/...`, console/org profile, etc.)
3. system defaults
4. user config
5. workspace config discovered from repo root to CWD
6. local/gitignored config
7. extension defaults
8. extension workspace overrides
9. environment files (`.env`, scoped env files)
10. environment variables
11. inline config fragments (`UAES_CONFIG_CONTENT`)
12. CLI overrides (`-c path=value`, flags)
13. session/runtime overrides
14. managed/locked policy overlays

Managed overlays can mark individual keys or subtrees immutable.

## 3.3 Merge semantics

Every field declares a merge mode:

- `replace`
- `deep-merge`
- `append`
- `append-dedupe`
- `union`
- `intersection`
- `priority-merge` (e.g. policies)
- `override-builtins`
- `alias-only` (adds aliases without replacing canonical definitions)

Authors may override default merge behavior for selected maps/arrays when permitted.

## 3.4 Canonical top-level config

```ts
interface UnifiedRuntimeConfig {
  $schema: string

  meta?: {
    name?: string
    description?: string
    owner?: string
  }

  runtime?: RuntimeConfig
  models?: ModelConfig
  services?: ServiceConfig
  context?: ContextConfig
  memory?: MemoryConfig
  commands?: Record<string, CommandDefinition>
  skills?: Record<string, SkillDefinition>
  tools?: ToolRegistryConfig
  agents?: Record<string, AgentDefinition>
  hooks?: HookRegistry
  policies?: PolicyBundle
  mcp?: Record<string, MCPServerDefinition>
  extensions?: ExtensionManagerConfig
  ui?: UIConfig
  history?: HistoryConfig
  telemetry?: TelemetryConfig
  features?: Record<string, boolean | Record<string, any>>
}
```

### 3.4.1 Runtime config

```ts
interface RuntimeConfig {
  cwd?: string
  worktreeRootMarkers?: string[]
  server?: {
    port?: number
    hostname?: string
    mdns?: boolean
    cors?: string[]
  }
  sharing?: 'manual' | 'auto' | 'disabled'
  sessionRetention?: {
    enabled?: boolean
    maxAge?: string
    persistence?: 'none' | 'metadata-only' | 'save-all'
    maxBytes?: number
  }
  snapshot?: boolean
  autoUpdate?: boolean | 'notify'
  language?: string
  outputStyle?: string
  voice?: { enabled?: boolean }
  announcements?: string[]
  attribution?: { commit?: string; pr?: string }
}
```

### 3.4.2 Model config

```ts
interface ModelConfig {
  primary?: ModelRef
  review?: ModelRef
  small?: ModelRef
  planning?: ModelRef
  compaction?: ModelRef
  routing?: {
    byAgent?: Record<string, ModelRef>
    bySkill?: Record<string, ModelRef>
    byCommand?: Record<string, ModelRef>
    byEvent?: Record<string, ModelRef>
  }
  available?: string[]
  overrides?: Record<string, string>
  fallback?: string
  contextWindow?: number
  autoCompactTokenLimit?: number
  verbosity?: 'low' | 'medium' | 'high'
  reasoning?: {
    effort?: 'none' | 'low' | 'medium' | 'high' | 'max'
    summary?: 'none' | 'concise' | 'auto' | 'detailed'
    budgetTokens?: number
    adaptive?: boolean
  }
  serviceTier?: 'fast' | 'default' | 'flex' | 'priority'
}
```

### 3.4.3 Services config

```ts
interface ServiceConfig {
  providers?: Record<string, ProviderConfig>
  formatter?: false | Record<string, FormatterConfig>
  lsp?: false | Record<string, LspConfig>
}
```

---

## 4. Context, Instructions, and Memory

## 4.1 Unified context documents

Instead of hardcoding `CLAUDE.md`, `GEMINI.md`, or `AGENTS.md`, the superset defines a document discovery system.

```ts
interface ContextConfig {
  documents?: {
    names?: string[]                // e.g. ["AGENTS.md", "CLAUDE.md", "GEMINI.md"]
    fallbackNames?: string[]
    globalFiles?: string[]
    localOverrideFiles?: string[]   // e.g. AGENTS.override.md
    managedFiles?: string[]
    excludes?: string[]
    discovery?: {
      upward?: boolean
      downwardLazy?: boolean
      stopMarkers?: string[]        // e.g. .git
      maxDirs?: number
      maxBytes?: number
    }
    load?: {
      eagerScopes?: Array<'managed' | 'global' | 'workspace-ancestors' | 'cwd'>
      lazyForDescendants?: boolean
      concatenateOrder?: 'root-to-leaf' | 'leaf-to-root'
    }
    includes?: {
      enabled?: boolean
      syntaxes?: Array<'@path' | '@{path}' | '!include'>
      maxDepth?: number
      resolveRelativeToIncludingFile?: boolean
      allowAbsolutePaths?: boolean
      allowHomeExpansion?: boolean
      allowResources?: boolean
    }
  }

  pathRules?: Array<{
    id?: string
    paths?: string[]
    body: string
    source?: string
  }>
}
```

This subsumes:

- personal/global context files
- managed enterprise context files
- upward walk from CWD to repo root
- concatenation of all matching docs
- lazy loading for descendant directories
- local override docs
- file import/include syntax
- path-scoped rules triggered when matching files are read

## 4.2 Context document authoring

A context document may be pure Markdown or Markdown with frontmatter:

```md
---
id: context:workspace/root
appliesTo:
  - "src/**/*.ts"
priority: 50
---

# API Rules

- Validate input on all external boundaries
- Prefer structured error types

See @docs/architecture.md
```

Supported include forms:

- `@relative/path.md`
- `@{relative/path.md}`
- `@~/path.md`
- `@resource(mcp://server/resource-uri)`

## 4.3 Memory system

The superset merges auto-memory, explicit memory tools, and per-agent memories.

```ts
interface MemoryConfig {
  enabled?: boolean
  autoGenerate?: boolean
  useDuringPrompting?: boolean
  storage?: {
    root?: string
    layout?: 'topic-files' | 'index-plus-topics' | 'kv' | 'hybrid'
  }
  scopes?: {
    user?: boolean
    workspace?: boolean
    project?: boolean
    local?: boolean
    session?: boolean
    agent?: boolean
  }
  consolidation?: {
    enabled?: boolean
    maxRawEntries?: number
    maxUnusedDays?: number
  }
  loading?: {
    indexPreviewLines?: number
    lazyTopics?: boolean
  }
}
```

Capabilities:

- auto-generated memory summaries
- explicit memory add/show/reload/search APIs
- per-agent memory scopes
- on-demand loading of memory topic files
- memory visibility controls (user/project/local/session/agent/shared)

## 4.4 Memory commands/tools

The runtime exposes both slash commands and tools:

- `/memory show`
- `/memory reload`
- `/memory add <text>`
- `memory.show`
- `memory.add`
- `memory.search`
- `memory.reload`

---

## 5. Commands and Prompt Assets

## 5.1 Unified command model

Commands are first-class prompt assets with optional imperative handlers.

```ts
interface CommandDefinition {
  id: string
  aliases?: string[]
  description?: string
  argumentHint?: string
  arguments?: JSONSchema
  body?: TemplateSource
  handler?: HandlerRef
  visibility?: 'public' | 'hidden'
  source?: 'user' | 'workspace' | 'extension' | 'managed'

  routing?: {
    agent?: string
    model?: string
    subtask?: boolean
  }

  permissions?: {
    allowedTools?: RulePattern[]
    deniedTools?: RulePattern[]
    approvalModeOverride?: string
  }

  injections?: {
    shell?: boolean
    file?: boolean
    resource?: boolean
    tool?: boolean
  }

  overrides?: {
    builtin?: boolean
    namespaceOnConflict?: boolean
  }
}
```

## 5.2 Supported command authoring forms

A command may be authored as:

1. Markdown + frontmatter
2. TOML prompt file
3. YAML/JSON config object
4. imperative code module
5. MCP prompt wrapper

## 5.3 Command templating

The superset supports all known variable styles and adds a canonical one.

### Canonical variables

- `{{ args.raw }}`
- `{{ args[0] }}`
- `{{ args.branch }}`
- `{{ env.NAME }}`
- `{{ session.id }}`
- `{{ skill.dir }}`
- `{{ agent.id }}`

### Canonical injections

- `{{ shell("git status --short") }}`
- `{{ file("README.md") }}`
- `{{ files(["a.ts","b.ts"]) }}`
- `{{ resource("mcp://github/repos/acme/app") }}`
- `{{ glob("src/**/*.ts") }}`
- `{{ grep("TODO", "src") }}`
- `{{ tool("list_dir", { path: "." }) }}`
- `{{ prompt("command:core/init") }}`

This subsumes `$ARGUMENTS`, `$1`, `{{args}}`, shell interpolation, file injection, and resource injection.

## 5.4 Invocation surfaces

Any command may be invoked as:

- slash command (`/release`)
- namespaced slash command (`/acme:release`)
- API prompt asset (`prompt.run(id, args)`)
- MCP prompt proxy
- hook-inserted prompt fragment

---

## 6. Skills

## 6.1 Unified skill model

A skill remains a reusable instruction bundle, but the superset expands it into a typed, policy-aware asset.

```ts
interface SkillDefinition {
  id: string
  name?: string
  description: string
  body: string
  arguments?: JSONSchema

  activation?: {
    manual?: boolean
    automatic?: boolean
    hidden?: boolean
    exposeInMenu?: boolean
    conditions?: ActivationCondition[]
    discoveryWeight?: number
    disableModelInvocation?: boolean
  }

  execution?: {
    contextMode?: 'inline' | 'fork'
    agent?: string
    model?: string
    reasoningEffort?: 'low' | 'medium' | 'high' | 'max'
    maxContextBytes?: number
  }

  permissions?: {
    allowedTools?: RulePattern[]
    deniedTools?: RulePattern[]
    policyOverride?: string
  }

  hooks?: HookRegistry
  metadata?: Record<string, string>
  compatibility?: string[]
  license?: string
}
```

## 6.2 Skill behavior

A skill may:

- be auto-discovered using only metadata
- be manually invoked by slash command
- be hidden from user menus but available to the model
- be user-invocable but not auto-invoked
- fork into a subagent context
- carry tool allowlists/denylists
- select model and reasoning effort
- attach its own hooks
- declare arguments and templated substitutions

## 6.3 Skill sources

Skills may come from:

- workspace directories
- user/global directories
- extension packages
- added external directories
- URLs
- bundled runtime skills
- marketplace installs

---

## 7. Hooks, Middleware, and Lifecycle Interception

## 7.1 Unification model

The superset merges four separate ideas into one system:

1. **Blocking middleware**
2. **Transform hooks**
3. **Observability/event subscribers**
4. **Notification hooks**

Every hook subscribes to an event pattern and declares what level of control it needs:

- `observe`
- `before`
- `after`
- `around`
- `replace`

## 7.2 Canonical event taxonomy

Instead of vendor-specific names like `PreToolUse` or `BeforeTool`, the superset uses normalized dotted event names.

### Core event families

- `session.start`
- `session.resume`
- `session.clear`
- `session.end`
- `session.compact.before`
- `session.compact.after`
- `context.loaded`
- `prompt.submit.before`
- `prompt.submit.after`
- `tool.selection.before`
- `tool.call.before`
- `tool.call.after`
- `tool.call.error`
- `permission.request`
- `permission.decision`
- `agent.start`
- `agent.stop`
- `agent.idle`
- `agent.transfer.before`
- `agent.transfer.after`
- `agent.error`
- `task.completed`
- `task.failed`
- `model.request.before`
- `model.request.after`
- `model.response.before-delivery`
- `model.response.retry`
- `notification.emit`
- `worktree.create.before`
- `worktree.create.after`
- `worktree.remove.before`
- `worktree.remove.after`
- `config.change.before`
- `config.change.after`
- `memory.read.before`
- `memory.write.after`
- `command.invoke.before`
- `command.invoke.after`
- `skill.activate.before`
- `skill.activate.after`
- `mcp.server.connect`
- `mcp.server.disconnect`
- `mcp.tools.changed`
- `mcp.resources.changed`
- `mcp.resource.updated`
- `mcp.prompt.invoked`
- `mcp.elicitation.request`
- `mcp.elicitation.result`
- `ui.notification`
- `ui.toast`
- `ui.theme.changed`
- `extension.install`
- `extension.update`
- `extension.enable`
- `extension.disable`
- `system.error`

This subsumes all observed lifecycle events and adds normalized names for missing asymmetries.

## 7.3 Hook definition

```ts
interface HookDefinition {
  id?: string
  when: HookSelector
  phase?: 'observe' | 'before' | 'after' | 'around' | 'replace'
  kind: 'command' | 'http' | 'websocket' | 'runtime' | 'prompt' | 'agent' | 'module'
  handler: HandlerRef

  execution?: {
    timeoutMs?: number
    async?: boolean
    once?: boolean
    retry?: { attempts: number; backoffMs?: number }
    concurrency?: 'serial' | 'parallel'
    statusMessage?: string
  }

  security?: {
    fingerprint?: string
    allowEnvInterpolation?: string[]
    trustLevel?: 'untrusted' | 'trusted' | 'managed'
  }
}

interface HookSelector {
  events: string[]                     // glob-capable
  tool?: RulePattern[]
  agent?: RulePattern[]
  command?: RulePattern[]
  skill?: RulePattern[]
  mcpServer?: RulePattern[]
  notificationType?: RulePattern[]
  configSource?: RulePattern[]
  path?: string[]
  promptRegex?: string
  errorCode?: string[]
}
```

## 7.4 Hook context

Every hook receives a typed context envelope:

```ts
interface HookContextEnvelope<T = any> {
  event: string
  timestamp: string
  sessionId?: string
  transcriptPath?: string
  cwd?: string
  actor?: {
    kind: 'user' | 'agent' | 'tool' | 'hook' | 'extension' | 'system'
    id?: string
    type?: string
  }
  correlationId?: string
  mode?: string
  source?: string
  data: T
}
```

## 7.5 Hook result

```ts
interface HookResult {
  continue?: boolean
  stopReason?: string
  suppressOutput?: boolean
  systemMessage?: string

  decision?: 'allow' | 'approve' | 'ask' | 'deny' | 'block' | 'retry' | 'replace' | 'skip'
  reason?: string

  updatedInput?: any
  updatedOutput?: any
  additionalContext?: string | string[]
  syntheticResult?: any
  tailCalls?: Array<{ kind: 'tool' | 'command' | 'agent'; name: string; args?: any }>
  toolSelection?: {
    mode?: 'AUTO' | 'ANY' | 'NONE'
    allowed?: string[]
    denied?: string[]
  }

  permissionUpdates?: PermissionMutation[]
  envExports?: Record<string, string | null>
  ui?: UIInstruction[]
  telemetry?: any[]
}
```

This subsumes:

- blocking with exit code or `decision`
- updated tool inputs
- additional model context
- tool result replacement
- synthetic model responses
- tail tool calls
- exported env values on session start
- background notifications

## 7.6 Hook execution backends

A hook may be implemented as:

- external command (stdin/stdout JSON)
- HTTP endpoint
- WebSocket callback
- in-process runtime function
- prompt-to-model transform
- delegated agent
- module function (TS/JS/Python/Rust/Go via adapters)

---

## 8. Tools

## 8.1 Unified tool model

```ts
interface ToolDefinition {
  id: string
  kind:
    | 'builtin'
    | 'function'
    | 'script'
    | 'module'
    | 'mcp'
    | 'openapi'
    | 'graphql'
    | 'connector'
    | 'agent'
    | 'workflow'

  title?: string
  description: string
  inputSchema: JSONSchema
  outputSchema?: JSONSchema
  strict?: boolean
  freeformGrammar?: string

  annotations?: {
    readOnlyHint?: boolean
    destructiveHint?: boolean
    idempotentHint?: boolean
    openWorldHint?: boolean
    title?: string
  }

  execution?: {
    timeoutMs?: number
    background?: boolean
    sandbox?: SandboxRequest
    requiresApproval?: boolean
  }

  handler?: HandlerRef
  discovery?: {
    source?: 'registry' | 'directory' | 'module' | 'mcp' | 'openapi' | 'discovery-command'
    precedence?: number
    canOverrideBuiltin?: boolean
  }
}
```

## 8.2 Tool result model

```ts
interface ToolResult {
  content?: ContentBlock[]
  structuredContent?: any
  metadata?: Record<string, any>
  artifacts?: Artifact[]
  display?: {
    title?: string
    summary?: string
    renderAs?: 'text' | 'diff' | 'table' | 'image' | 'audio' | 'html'
  }
  isError?: boolean
}
```

## 8.3 Content blocks

```ts
type ContentBlock =
  | { type: 'text'; text: string }
  | { type: 'reasoning'; summary?: string; detail?: string }
  | { type: 'tool_use'; id: string; name: string; input?: any }
  | { type: 'tool_result'; toolUseId: string; content?: ContentBlock[]; structuredContent?: any; isError?: boolean }
  | { type: 'image'; mimeType: string; data?: string; url?: string }
  | { type: 'audio'; mimeType: string; data?: string; url?: string }
  | { type: 'file'; path: string; mimeType?: string }
  | { type: 'resource_link'; uri: string; name?: string; mimeType?: string }
  | { type: 'embedded_resource'; resource: any }
  | { type: 'artifact'; artifactId: string }
  | { type: 'json'; value: any }
```

This unifies Claude content blocks, MCP content blocks, and the Responses API output model.

## 8.4 Baseline built-in tools

A feature-complete superset runtime should expose at least these built-ins:

### Filesystem
- `fs.read`
- `fs.read_many`
- `fs.write`
- `fs.replace`
- `fs.patch`
- `fs.list`
- `fs.glob`
- `fs.grep`
- `fs.status`

### Execution
- `shell.exec`
- `shell.exec_command`
- `shell.write_stdin`
- `shell.repl.js`
- `shell.repl.reset`

### Web and browser
- `web.search`
- `web.fetch`
- `browser.navigate`
- `browser.extract`
- `browser.interact`

### Planning and tasking
- `plan.enter`
- `plan.exit`
- `plan.update`
- `todo.read`
- `todo.write`

### User interaction
- `user.ask`
- `user.request_input`
- `user.request_permissions`

### Memory and docs
- `memory.show`
- `memory.add`
- `memory.reload`
- `memory.search`
- `docs.get_internal`

### Skills and prompts
- `skill.activate`
- `prompt.run`

### Agents
- `agent.spawn`
- `agent.wait`
- `agent.send_input`
- `agent.close`
- `agent.resume`
- `agent.spawn_batch_csv`
- `agent.report_result`

### Media and artifacts
- `image.view`
- `artifact.create`
- `artifact.update`
- `artifact.read`

### MCP
- `mcp.list_resources`
- `mcp.list_resource_templates`
- `mcp.read_resource`
- `mcp.subscribe_resource`
- `mcp.list_prompts`
- `mcp.get_prompt`

### Connectors
- `connector.call`
- `connector.list_tools`

## 8.5 Custom tool discovery

Tools may be discovered from:

- workspace `tools/`
- user/global `tools/`
- extensions
- plugin modules
- discovery command (`tools.discoveryCommand`)
- MCP servers
- OpenAPI specs
- GraphQL schemas
- runtime SDK registration

Tools may override built-ins if explicitly allowed by policy.

---

## 9. Agents and Workflow Orchestration

## 9.1 Unified agent model

```ts
type AgentKind = 'local' | 'remote' | 'hybrid' | 'workflow'

interface AgentDefinition {
  id: string
  displayName?: string
  description: string
  kind?: AgentKind

  prompt?: TemplateSource
  initialMessages?: Message[]
  model?: string
  modelConfig?: {
    temperature?: number
    topP?: number
    topK?: number
    maxTokens?: number
    reasoningEffort?: 'none' | 'low' | 'medium' | 'high' | 'max'
    reasoningSummary?: 'none' | 'concise' | 'auto' | 'detailed'
    serviceTier?: 'fast' | 'default' | 'flex' | 'priority'
  }

  inputs?: JSONSchema
  outputs?: JSONSchema

  tools?: RulePattern[]
  disallowedTools?: RulePattern[]
  permissions?: PermissionOverride
  skills?: string[]
  hooks?: HookRegistry
  mcpServers?: Record<string, MCPServerRef | MCPServerDefinition>

  limits?: {
    maxTurns?: number
    maxTimeMs?: number
    maxBudgetUsd?: number
    maxChildren?: number
    maxDepth?: number
    steps?: number
  }

  memory?: {
    scope?: 'user' | 'workspace' | 'project' | 'local' | 'session' | 'agent'
    loadPreviewLines?: number
    shareWithParent?: boolean
  }

  isolation?: {
    mode?: 'none' | 'worktree' | 'sandbox' | 'container' | 'vm'
    autoCleanup?: boolean
    symlinkDirectories?: string[]
    sparsePaths?: string[]
  }

  execution?: {
    background?: boolean
    forkContext?: boolean
    includeParentHistory?: boolean
    allowTransferToParent?: boolean
    allowTransferToPeers?: boolean
    exposeAsTool?: boolean
  }

  remote?: {
    agentCardUrl?: string
    endpoint?: string
    auth?: A2AAuthConfig
  }

  workflow?: WorkflowDefinition

  ui?: {
    color?: string
    hidden?: boolean
    disabled?: boolean
    nicknameCandidates?: string[]
    icon?: string
  }
}
```

## 9.2 Workflow agents

The superset adopts ADK-style orchestration as a native agent kind:

```ts
interface WorkflowDefinition {
  type: 'sequential' | 'parallel' | 'loop' | 'router' | 'planner-executor'
  steps?: WorkflowStep[]
  termination?: {
    mode?: 'goal' | 'timeout' | 'max-turns' | 'explicit-complete'
  }
}
```

This subsumes:

- normal subagents
- batch agents
- planner/executor modes
- parallel specialist graphs
- loop agents
- agent-as-tool wrappers
- remote A2A agents

## 9.3 Invocation surfaces

Agents may be invoked via:

- natural-language routing
- explicit mentions (`@code-reviewer`)
- tool calls (`agent.spawn`)
- session-wide agent mode
- workflow definition
- command routing
- skill forking

## 9.4 Agent termination semantics

Every agent reports a typed termination mode:

- `GOAL`
- `TIMEOUT`
- `MAX_TURNS`
- `ABORTED`
- `ERROR`
- `NO_COMPLETE_SIGNAL`
- `POLICY_DENIED`

---

## 10. MCP Integration

## 10.1 Full MCP role support

The superset runtime must support:

- **MCP client role** (consume external servers)
- **MCP server role** (expose local runtime capabilities)
- **embedded SDK-backed MCP server instances**

## 10.2 Supported MCP primitives

### Server-side primitives
- Tools
- Resources
- Resource templates
- Prompts
- Completions
- Logging
- Subscriptions

### Client-side primitives
- Sampling
- Sampling with tools
- Roots
- Elicitation

### Cross-cutting
- notifications
- progress
- pagination
- structured output
- output schemas
- resource updates
- stream resumption
- OAuth / bearer auth / DCR / PRM

## 10.3 MCP server definition

```ts
interface MCPServerDefinition {
  type: 'stdio' | 'sse' | 'http' | 'ws' | 'inmemory' | 'sdk'
  command?: string
  args?: string[]
  env?: Record<string, string>
  cwd?: string

  url?: string
  headers?: Record<string, string>
  envHeaders?: Record<string, string>

  timeoutMs?: number
  required?: boolean
  trust?: boolean
  description?: string

  includeTools?: string[]
  excludeTools?: string[]
  enabledTools?: string[]
  disabledTools?: string[]

  oauth?: {
    enabled?: boolean
    clientId?: string
    clientSecret?: string
    authorizationUrl?: string
    issuer?: string
    tokenUrl?: string
    scopes?: string[]
    audiences?: string[]
    redirectUri?: string
    registrationUrl?: string
    protectedResource?: string
    callbackPort?: number
    credentialStore?: 'keyring' | 'file' | 'auto'
  }

  scope?: 'global' | 'workspace' | 'agent'
}
```

## 10.4 MCP transport support

A feature-complete runtime should support:

- stdio
- legacy SSE
- streamable HTTP
- WebSocket
- in-memory/test transports
- command/subprocess transport
- logging transport wrappers

## 10.5 MCP resource and prompt integration

Resources and prompts are promoted to native runtime objects:

- MCP prompts can be exposed as slash commands, prompt assets, or skill dependencies
- MCP resources can be referenced in command/skill/context templates using resource URIs
- MCP resources may be listed/read/subscribed via tools
- MCP tools retain `annotations`, `outputSchema`, and `structuredContent`

## 10.6 Roots and elicitation

The runtime should expose workspace roots and per-agent worktrees to MCP servers, and should fully support server-initiated elicitation:

- `form` elicitation
- `url` elicitation
- user approval / decline / cancel
- hook interception of elicitation
- policy control over which servers may elicit

---

## 11. Permissions, Approvals, Safety, and Sandbox

## 11.1 Unified approval modes

```ts
type ApprovalMode =
  | 'default'
  | 'auto-edit'
  | 'plan'
  | 'yolo'
  | 'untrusted'
  | 'on-request'
  | 'never'
  | 'bypass'
  | 'dont-ask'
  | 'granular'
```

This subsumes the observed modes and adds a canonical vocabulary.

## 11.2 Unified rule model

```ts
interface PolicyRule {
  id?: string
  tier?: 'builtin' | 'extension' | 'workspace' | 'user' | 'admin' | 'managed'
  priority: number

  when?: {
    subject?: Array<'tool' | 'command' | 'skill' | 'agent' | 'mcp-server' | 'resource' | 'connector' | 'path' | 'network'>
    name?: RulePattern | RulePattern[]
    namespace?: RulePattern | RulePattern[]
    toolName?: RulePattern | RulePattern[]
    commandPrefix?: string
    commandRegex?: string
    argsRegex?: string
    argsJsonPath?: Record<string, RulePattern>
    path?: string[]
    uri?: string[]
    mcpName?: string[]
    agent?: string[]
    subagent?: string[]
    mode?: string[]
    interactive?: boolean
    annotations?: {
      readOnlyHint?: boolean
      destructiveHint?: boolean
      idempotentHint?: boolean
      openWorldHint?: boolean
    }
    extension?: string[]
    riskLevel?: Array<'low' | 'medium' | 'high' | 'critical'>
  }

  decision:
    | 'allow'
    | 'approve'
    | 'ask'
    | 'deny'
    | 'hide'
    | 'sandbox-only'
    | 'rewrite'
    | 'audit-only'

  message?: string
  reviewer?: 'user' | 'guardian-agent' | 'policy-service' | 'none'
  mutations?: PermissionMutation[]
  terminal?: boolean
  allowRedirection?: boolean
}
```

## 11.3 Rule evaluation

The superset supports both compatibility styles, but canonical evaluation is:

1. collect applicable rules
2. sort by `(tierBase, priority, specificity, recency)`
3. apply managed-terminal rules first
4. resolve conflicts using:
   - `hide` > `deny` > `ask` > `sandbox-only` > `rewrite` > `allow`
5. apply mutations
6. run safety checkers
7. dispatch to reviewer if still unresolved

This subsumes “deny first”, “last-match wins”, and numeric priority systems.

## 11.4 Safety checkers

```ts
interface SafetyChecker {
  id: string
  appliesTo: HookSelector
  type: 'in-process' | 'external' | 'model' | 'policy-service'
  requiredContext?: string[]
  handler: HandlerRef
  onPass?: 'continue'
  onFail?: 'deny' | 'ask' | 'rewrite' | 'sandbox-only'
}
```

## 11.5 Reviewer model

Approvals may be decided by:

- human user
- guardian subagent
- external policy service
- auto-approval rule
- one-shot session approval
- workspace-persistent approval
- managed/global allowlist

## 11.6 Sandbox model

```ts
interface SandboxConfig {
  mode: 'read-only' | 'workspace-write' | 'project-write' | 'custom-write' | 'container' | 'danger-full-access'
  backend?: 'seatbelt' | 'landlock' | 'bwrap' | 'docker' | 'podman' | 'runsc' | 'lxc' | 'windows-native'
  writableRoots?: string[]
  readableRoots?: string[]
  denyRead?: string[]
  denyWrite?: string[]
  networkAccess?: boolean
  allowedDomains?: string[]
  allowUnixSockets?: string[]
  allowAllUnixSockets?: boolean
  allowLocalBinding?: boolean
  httpProxyPort?: number
  socksProxyPort?: number
  excludedCommands?: string[]
  allowUnsandboxedCommands?: boolean
  weakerNestedSandbox?: boolean
  weakerNetworkIsolation?: boolean
}
```

This subsumes filesystem, network, worktree, and platform-native sandbox controls.

---

## 12. Event Bus and Observability

## 12.1 Canonical event envelope

```ts
interface EventEnvelope<T = any> {
  id: string
  type: string
  timestamp: string
  sessionId?: string
  messageId?: string
  agentId?: string
  correlationId?: string
  parentId?: string
  severity?: 'debug' | 'info' | 'notice' | 'warning' | 'error' | 'critical'
  actor?: {
    kind: 'user' | 'assistant' | 'agent' | 'tool' | 'hook' | 'extension' | 'mcp' | 'system'
    id?: string
    name?: string
  }
  data: T
}
```

## 12.2 Delivery mechanisms

The same event stream can be delivered via:

- in-process subscriber API
- SDK callback
- SSE
- WebSocket
- JSONL transcript
- headless `stream-json`
- command notifier
- webhook sink
- telemetry exporter

## 12.3 Event categories

A production implementation should expose at least:

- `session.*`
- `message.*`
- `prompt.*`
- `tool.*`
- `permission.*`
- `agent.*`
- `command.*`
- `skill.*`
- `memory.*`
- `mcp.*`
- `config.*`
- `ui.*`
- `extension.*`
- `server.*`
- `pty.*`
- `filesystem.*`
- `project.*`
- `worktree.*`
- `error.*`
- `telemetry.*`

## 12.4 Headless output stream

Headless streaming should offer a normalized event subset for shell automation:

- `init`
- `message`
- `reasoning`
- `tool_use`
- `tool_result`
- `approval_request`
- `event`
- `warning`
- `error`
- `result`

---

## 13. SDK and External API

## 13.1 Transport options

The superset SDK should support:

- subprocess transport to a local CLI/runtime
- HTTP JSON API
- SSE streaming
- WebSocket streaming
- in-memory testing transport
- embedded runtime mode

## 13.2 Multi-language SDKs

Generated and/or hand-maintained SDKs should exist for:

- TypeScript
- Python
- Go
- Java
- Rust

## 13.3 SDK surface

```ts
interface RuntimeClient {
  connect(): Promise<void>
  createSession(input?: SessionCreateInput): Promise<SessionInfo>
  query(input: QueryInput): AsyncIterable<Message | EventEnvelope>
  interrupt(sessionId: string): Promise<void>
  stopTask(taskId: string): Promise<void>

  setModel(sessionId: string, model?: string): Promise<void>
  setApprovalMode(sessionId: string, mode: string): Promise<void>

  listAgents(): Promise<AgentDefinition[]>
  listTools(): Promise<ToolDefinition[]>
  listCommands(): Promise<CommandDefinition[]>
  listSkills(): Promise<SkillDefinition[]>

  reconnectMcpServer(name: string): Promise<void>
  toggleMcpServer(name: string, enabled: boolean): Promise<void>
  getMcpStatus(): Promise<any>

  rewindFiles(checkpointId: string): Promise<void>
  subscribeEvents(filter?: EventFilter): AsyncIterable<EventEnvelope>
}
```

## 13.4 SDK builder APIs

The superset SDK should include helpers to build:

- function tools
- agent tools
- MCP servers
- workflow graphs
- hook handlers
- safety checkers
- theme packs
- prompt assets

## 13.5 Query model

```ts
interface QueryInput {
  sessionId?: string
  input: string | ContentBlock[]
  instructions?: string
  previousResponseId?: string
  model?: string
  tools?: string[]
  toolChoice?: 'auto' | 'required' | 'none' | { type: 'function'; name: string }
  parallelToolCalls?: boolean
  outputSchema?: JSONSchema
  includePartialMessages?: boolean
}
```

---

## 14. UI, Themes, and Interaction Surface

## 14.1 Unified UI config

```ts
interface UIConfig {
  theme?: string
  customThemes?: Record<string, ThemeDefinition>
  autoThemeSwitching?: boolean

  accessibility?: {
    screenReader?: boolean
    hideAnimations?: boolean
    highContrast?: boolean
  }

  layout?: {
    hideBanner?: boolean
    hideFooter?: boolean
    hideContextSummary?: boolean
    showLineNumbers?: boolean
    alternateScreen?: 'auto' | 'always' | 'never'
    diffStyle?: 'auto' | 'stacked' | 'inline'
  }

  notifications?: {
    enabled?: boolean
    method?: 'auto' | 'osc9' | 'bel' | 'command'
    command?: string[]
    events?: string[]
  }

  statusLine?: {
    components?: string[]
    provider?: HandlerRef
  }

  terminalTitle?: {
    components?: string[]
  }

  keybinds?: Record<string, string | string[] | 'none'>

  inlineThinkingMode?: 'off' | 'summary' | 'detailed'
  vimMode?: boolean
}
```

## 14.2 Theme definition

```ts
interface ThemeDefinition {
  type: 'custom'
  name: string
  text?: {
    primary?: string
    secondary?: string
    link?: string
    accent?: string
    response?: string
  }
  background?: {
    primary?: string
    diff?: { added?: string; removed?: string }
  }
  border?: {
    default?: string
    focused?: string
  }
  status?: {
    success?: string
    warning?: string
    error?: string
  }
  ui?: {
    comment?: string
    symbol?: string
    active?: string
    focus?: string
    gradient?: string[]
  }
}
```

Extensions may contribute themes, status line providers, and UI commands.

---

## 15. Minimal Unified Authoring Examples

## 15.1 Command

```md
---
id: command:core/release
description: Create a release commit and pull request
argumentHint: [branch]
arguments:
  type: object
  properties:
    branch:
      type: string
  required: [branch]
permissions:
  allowedTools:
    - tool:builtin/shell(git add *)
    - tool:builtin/shell(git commit *)
    - tool:builtin/shell(gh pr create *)
routing:
  agent: agent:core/build
---

Current status:
{{ shell("git status --short") }}

Current branch:
{{ shell("git branch --show-current") }}

Target branch:
{{ args.branch }}

Create an appropriate commit, push it, and open a PR.
```

## 15.2 Skill

```md
---
id: skill:core/security-audit
description: Audit code for security issues and risky shell/database/network behavior
activation:
  manual: true
  automatic: true
  conditions:
    - promptRegex: '(security|audit|vulnerability|threat)'
execution:
  contextMode: fork
  agent: agent:core/security-reviewer
  reasoningEffort: high
permissions:
  allowedTools:
    - tool:builtin/fs.read
    - tool:builtin/fs.grep
    - mcp://github/*
---

# Security audit procedure

1. Identify trust boundaries
2. Look for injection surfaces
3. Check authn/authz paths
4. Review secrets handling
5. Report concrete findings with reproduction steps
```

## 15.3 Hook

```yaml
hooks:
  - when:
      events: ["tool.call.before"]
      tool: ["tool:builtin/shell"]
    phase: before
    kind: command
    handler:
      command: ./.uaes/hooks/validate-shell.sh
    execution:
      timeoutMs: 30000
      concurrency: serial
      statusMessage: Validating shell command...
```

## 15.4 Tool

```ts
export default defineTool({
  id: "tool:acme/release-notes",
  description: "Generate release notes from commits",
  inputSchema: {
    type: "object",
    properties: {
      since: { type: "string" }
    },
    required: ["since"],
    additionalProperties: false
  },
  outputSchema: {
    type: "object",
    properties: {
      markdown: { type: "string" }
    },
    required: ["markdown"]
  },
  annotations: {
    readOnlyHint: true,
    idempotentHint: true
  },
  async execute(args, ctx) {
    const commits = await ctx.tool("tool:builtin/shell", { command: ["git", "log", `${args.since}..HEAD`, "--oneline"] })
    return {
      structuredContent: { markdown: renderReleaseNotes(commits) }
    }
  }
})
```

## 15.5 Agent

```md
---
id: agent:core/code-reviewer
description: Review code for correctness, maintainability, and security
model: openai/gpt-5.4
tools:
  - tool:builtin/fs.read
  - tool:builtin/fs.grep
  - tool:builtin/fs.glob
  - mcp://github/*
skills:
  - skill:core/security-audit
limits:
  maxTurns: 20
memory:
  scope: project
isolation:
  mode: worktree
ui:
  color: blue
execution:
  exposeAsTool: true
---

You are a code reviewer. Produce prioritized findings with file/line references.
```

## 15.6 Policy

```toml
[[rules]]
tier = "workspace"
priority = 500
decision = "deny"
message = "Destructive recursive deletes are blocked."
terminal = true

[rules.when]
subject = ["tool"]
toolName = "tool:builtin/shell"
commandRegex = "rm -rf"

[[rules]]
tier = "user"
priority = 300
decision = "allow"

[rules.when]
subject = ["tool"]
toolName = "tool:builtin/shell"
commandPrefix = "git status"
```

## 15.7 MCP server

```yaml
mcp:
  github:
    type: http
    url: https://mcp.example.com/github
    oauth:
      enabled: true
      scopes: ["repo", "read:user"]
    includeTools: ["list_repos", "create_pr"]
    excludeTools: ["delete_repo"]
```

---

## 16. Why This Meets or Exceeds the Source Systems

This superset exceeds the documented systems because it combines, rather than chooses among, their strongest features:

- **Config**: supports all observed scopes, formats, inline fragments, env overrides, CLI overrides, and managed locks
- **Context**: supports special docs, upward walk, lazy descendant loading, imports, path-scoped rules, global/local/managed docs, and explicit byte budgets
- **Commands**: supports Markdown, TOML, JSON/YAML, templating, shell/file/resource injection, argument hints, namespaces, and imperative handlers
- **Skills**: supports manual and automatic activation, hidden/manual-only/model-only modes, forked execution, model/tool/hook overrides, and packaged skill bundles
- **Hooks**: supports external command hooks, HTTP hooks, in-process hooks, model hooks, synthetic responses, tail calls, env exports, observability-only hooks, and typed event subscriptions
- **Tools**: supports built-ins, file/module tools, MCP tools, OpenAPI tools, agent tools, freeform grammar tools, output schemas, annotations, PTY, artifacts, and media
- **Agents**: supports local and remote agents, A2A auth, workflow graphs, agent memory, worktree isolation, background execution, concurrency limits, and agent-as-tool exposure
- **MCP**: supports full client/server roles and the complete modern primitive set rather than only tools
- **Packaging**: supports manifest-based extensions, module plugins, trust/fingerprinting, scoped enablement, marketplace controls, and migration metadata
- **Policy**: supports rule tiers, numeric priorities, allow/ask/deny/hide/rewrite/sandbox decisions, safety checkers, reviewers, and managed lockdown
- **SDK/API**: supports subprocess and network transports, streaming messages/events, session control, MCP control, checkpoint rewinds, and builder APIs
- **Events/UI**: supports rich event buses, SSE/WebSocket/headless streams, custom themes, keybindings, notifications, status lines, and accessibility
