# Extism WebAssembly Plugin Framework - Research Notes

## Overview

Extism is a lightweight, universal framework for building plugin systems with WebAssembly, by Dylibso. Reached v1 on January 8, 2024. Open source (BSD-3-Clause).

**Key properties:**
- Runs Wasm code **in-process** (not over network boundary like a sidecar) — near-native function call speed
- Full sandbox isolation — plugins can't access host memory, filesystem, or network by default
- High-level abstraction over lower-level Wasm engines (Wasmtime, Wazero, V8, etc.)
- Universal interface — same concepts across all host SDKs and PDKs
- The Go SDK uses **wazero** (pure-Go Wasm runtime, no CGO required)

**Adds on top of raw Wasm runtimes:**
1. Managed input/output protocol (bytes-in, bytes-out)
2. Persistent variables (module-scope state across calls)
3. Built-in HTTP without WASI
4. Simple host function linking with memory helpers
5. Plugin lifecycle management (compile once, instantiate many)
6. Observability hooks (tracing adapters via Dylibso Observe SDK)
7. Runtime limiters and timeouts
8. Hash verification (SHA256) on Wasm sources

**Two-component architecture:**
- **Host SDKs** — embed the runtime in your app (Go, Rust, Python, Ruby, Node/Browser, C/C++, PHP, .NET, OCaml, Haskell, Elixir, Java, Perl, Zig, D, etc.)
- **PDKs** (Plugin Development Kits) — for writing plugins (Go, Rust, JS/TS, C, Zig, Haskell, .NET, AssemblyScript, MoonBit; experimental: Python, C++)

**Supported plugin languages (via PDKs):** Go, Rust, JavaScript/TypeScript, Haskell, AssemblyScript, .NET (C#/F#), C, Zig, MoonBit (officially listed). Python and C++ PDKs exist on GitHub but are not on the official PDK docs page.

**Core threading model:** Plugins are single-threaded. Host application handles concurrency by creating pools of plugin instances.

**WASI support:** Superset — WASI is optional at the framework level. Plugins can run without WASI using Extism's own I/O. However, some PDK toolchains (Go, .NET, JS) currently require WASI as a practical build target even when plugins don't use system resources.

---

## Go SDK (`github.com/extism/go-sdk`)

### Core Types

#### Manifest
```go
type Manifest struct {
    Wasm         []Wasm                    // WASM sources (files, URLs, or raw data)
    Memory       *ManifestMemory           // Memory limits
    Config       map[string]string         // Static configuration passed to plugins
    AllowedHosts []string                  // Glob patterns for HTTP access
    AllowedPaths map[string]string         // Host→plugin filesystem mappings
    Timeout      uint64                    // Execution timeout in milliseconds
}

type ManifestMemory struct {
    MaxPages             uint32  // Max WASM memory pages (64KB each)
    MaxHttpResponseBytes int64   // Max bytes for HTTP response (-1 = 50MB default)
    MaxVarBytes          int64   // Max bytes for persistent variables (-1 = 1MB default)
}
```

Wasm sources can be specified three ways:
- `WasmFile{Path, Hash, Name}` — local file path
- `WasmUrl{Url, Headers, Method, Hash, Name}` — remote URL with optional auth headers
- `WasmData{Data, Hash, Name}` — raw bytes in memory

Hash field is optional SHA256 for verification. Manifest can be parsed from JSON via `manifest.UnmarshalJSON()`.

AllowedPaths supports read-only mounts with `"ro:"` prefix: `{"ro:config": "/etc/config"}`.

#### PluginConfig
```go
type PluginConfig struct {
    RuntimeConfig               wazero.RuntimeConfig   // Wazero runtime options (memory limits, caching)
    EnableWasi                  bool                    // Enable WASI (system access)
    ModuleConfig                wazero.ModuleConfig     // Wazero module options
    ObserveAdapter              *observe.AdapterBase    // Observability
    ObserveOptions              *observe.Options        // Observability config
    EnableHttpResponseHeaders   bool                    // Capture HTTP response headers
}
```

#### CompiledPlugin (internal struct fields from source)
- `runtime` wazero.Runtime — the WebAssembly runtime instance
- `main` wazero.CompiledModule — the main user plugin module
- `extism` wazero.CompiledModule — the embedded Extism kernel module
- `env` api.Module — the host environment module
- `modules` map[string]wazero.CompiledModule — additional compiled modules (dependencies)
- `instanceCount` atomic.Uint64 — counter for unique instance naming
- `wasmBytes` []byte — raw WASM bytecode (for observability)
- `hasWasi` bool
- `manifest` Manifest
- `observeAdapter` *observe.AdapterBase
- `observeOptions` *observe.Options
- `maxHttp` int64 — max HTTP response bytes (default 50MB)
- `maxVar` int64 — max variable store size (default 1MB)
- `enableHttpResponseHeaders` bool

#### Plugin (runtime instance, internal struct fields from source)
- `close` []func(ctx context.Context) error — cleanup functions
- `extism` api.Module — Extism kernel module instance
- `mainModule` api.Module — main plugin module instance
- `modules` map[string]api.Module — instantiated helper modules
- `Timeout` time.Duration
- `Config` map[string]string — plugin configuration values
- `Var` map[string][]byte — persistent variables (state across calls)
- `AllowedHosts` []string — glob pattern HTTP allowlist
- `AllowedPaths` map[string]string — host→guest filesystem mappings
- `LastStatusCode` int — last HTTP response status
- `LastResponseHeaders` map[string]string — last HTTP response headers (optional)
- `MaxHttpResponseBytes` int64
- `MaxVarBytes` int64
- `guestRuntime` guestRuntime — auto-detected guest runtime type (Haskell, WASI reactor/command)
- `hasWasi` bool
- `Adapter` *observe.AdapterBase
- `traceCtx` *observe.TraceCtx
- `log` func(LogLevel, string)

#### CurrentPlugin
Wrapper that gives host functions access to plugin memory and state.

Methods:
- `Alloc(n uint64) (uint64, error)` / `AllocWithContext(ctx, n)` — calls kernel's `alloc` export
- `Free(offset uint64) error` / `FreeWithContext(ctx, offset)` — calls kernel's `free` export
- `Length(offs uint64) (uint64, error)` / `LengthWithContext(ctx, offs)` — calls kernel's `length` export
- `WriteString(s string) (uint64, error)` — allocate + write string to guest memory
- `WriteBytes(b []byte) (uint64, error)` — allocate + write bytes to guest memory
- `ReadString(offset uint64) (string, error)` — read string from guest memory
- `ReadBytes(offset uint64) ([]byte, error)` — read bytes from guest memory
- `Memory() api.Memory` — direct access to wazero memory (delegates to `Plugin.Memory()` which calls `p.extism.ExportedMemory("memory")`)
- `Log(level LogLevel, message string)`
- `Logf(level LogLevel, format string, args ...any)`

#### HostFunction
```go
hostFunc := extism.NewHostFunctionWithStack(
    "name",                          // function name
    callback,                        // HostFunctionStackCallback
    []extism.ValueType{...params},   // parameter types
    []extism.ValueType{...returns},  // return types
)
hostFunc.SetNamespace("custom")      // default: "extism:host/user"
```

`HostFunctionStackCallback` signature: `func(ctx context.Context, p *CurrentPlugin, stack []uint64)`

### Complete Plugin API Surface

**Creation:**
- `NewPlugin(ctx, manifest, config, functions) → (*Plugin, error)` — compile + instantiate in one call
- `NewCompiledPlugin(ctx, manifest, config, functions) → (*CompiledPlugin, error)` — compile only
- `CompiledPlugin.Instance(ctx, PluginInstanceConfig{}) → (*Plugin, error)` — create instance
- `CompiledPlugin.Close(ctx) → error`

**Execution:**
- `Plugin.Call(name, data) → (uint32, []byte, error)` — call function
- `Plugin.CallWithContext(ctx, name, data) → (uint32, []byte, error)` — with context for timeout/cancel
- `Plugin.SetInput(data) → (uint64, error)` — set input buffer
- `Plugin.GetOutput() → ([]byte, error)` — get output buffer
- `Plugin.GetError() → string` / `GetErrorWithContext(ctx)` — get error message from plugin

**Introspection:**
- `Plugin.FunctionExists(name) → bool`
- `Plugin.Module() → *Module` — get main module info

**Configuration:**
- `Plugin.SetLogger(func(LogLevel, string))` — custom logger
- `Plugin.Log(level, message)` / `Logf(level, format, ...args)`

**Cleanup:**
- `Plugin.Close(ctx) → error` / `CloseWithContext(ctx)`

**Global:**
- `SetLogLevel(level)` — global minimum log level
- `RuntimeVersion() → string` — Extism kernel version

**Log levels:** `LogLevelTrace`, `LogLevelDebug`, `LogLevelInfo`, `LogLevelWarn`, `LogLevelError`, `LogLevelOff`

### Plugin Lifecycle

1. `NewCompiledPlugin(ctx, manifest, config, hostFuncs)` — compiles Wasm, inits wazero runtime, builds host modules
2. `compiled.Instance(ctx, PluginInstanceConfig{})` — lightweight instance with unique module names
3. `plugin.Call("func", input)` → `(exitCode uint32, output []byte, err error)`
4. `plugin.Close(ctx)` — cleanup; calling after close returns error

`NewPlugin()` is a convenience that does steps 1+2 in one call.

### Module Instantiation Order (from source)

Within `NewCompiledPlugin()` + `Instance()`:
1. WASI module (`wasi_snapshot_preview1`) if `EnableWasi` is true
2. Custom host modules — user-provided host functions grouped by namespace, each namespace becomes a wazero host module via `buildHostModule()`
3. Extism kernel — `extism-runtime.wasm` embedded via `//go:embed`, compiled once, instantiated per-instance (stateful)
4. `extism:host/env` bridge module — via `instantiateEnvModule()`, bridges guest calls to kernel exports + implements host capabilities (config_get, var_get/set, http_request, logging)
5. Non-main guest modules — compiled, wrapped via `createModuleWrapper()` for cross-module calls, instantiated with unique names (`{name}_{instanceNum}`)
6. Main guest module — instantiated last with unique name (`main_{instanceNum}`)

### Embedded Kernel

The `extism-runtime.wasm` is embedded via `//go:embed extism-runtime.wasm` as a byte slice. Version tracked alongside.

**Kernel exported functions** (from `extism/extism` `kernel/src/lib.rs`):
- `alloc(size: u64) → u64` — allocate memory in plugin's linear memory
- `free(ptr: u64)` — free memory
- `load_u8(offset: u64) → u8` — load byte from memory
- `load_u64(offset: u64) → u64` — load 8 bytes from memory
- `store_u64(offset: u64, value: u64)` — store 64-bit value
- `store_u8(offset: u64, value: u8)` — store byte
- `input_set(offset: u64, length: u64)` — set input buffer location
- `input_load_u8(offset: u64) → u8` — load byte from input buffer
- `input_load_u64(offset: u64) → u64` — load 8 bytes from input buffer
- `output_set(offset: u64, length: u64)` — set output buffer location
- `input_length() → u64` / `output_length() → u64` — get buffer lengths
- `input_offset() → u64` / `output_offset() → u64` — get buffer offsets
- `length(offset: u64) → u64` — get allocated block length
- `length_unsafe(offset: u64) → u64` — get length without bounds check
- `memory_bytes() → u64` — total memory size
- `reset()` — reset state
- `error_set(handle: u64)` / `error_get() → u64` — error handling

The kernel uses a bump allocator with free-list reuse, tracking total bytes and current position. The `MemoryRoot` structure is at byte offset 1 of linear memory (offset 0 is reserved as a NULL sentinel, similar to a C NULL pointer). Fields: `initialized` (AtomicBool), `position` (AtomicU64), `length` (AtomicU64), `error` (AtomicU64), `input_offset` (u64), `input_length` (u64), `output_offset` (u64), `output_length` (u64), `blocks` ([MemoryBlock; 0] — marks start of block data).

### extism:host/env Bridge Module

Implemented in `instantiateEnvModule()` in `host.go`. Bridges plugin calls to kernel + provides host capabilities (31 unique functions):

| Function | Signature | Purpose |
|---|---|---|
| `alloc` | `(i64) → i64` | Delegates to kernel alloc |
| `free` | `(i64) → void` | Delegates to kernel free |
| `length` | `(i64) → i64` | Delegates to kernel length |
| `length_unsafe` | `(i64) → i64` | Delegates to kernel length_unsafe |
| `load_u8` | `(i64) → i32` | Load byte from memory |
| `store_u8` | `(i64, i32) → void` | Store byte to memory |
| `load_u64` | `(i64) → i64` | Load 8 bytes from memory (custom Go impl) |
| `store_u64` | `(i64, i64) → void` | Store 8 bytes to memory (custom Go impl) |
| `input_set` | `(i64, i64) → void` | Set input buffer location |
| `input_length` | `() → i64` | Get input buffer length |
| `input_offset` | `() → i64` | Get input buffer offset |
| `input_load_u8` | `(i64) → i32` | Load byte from input |
| `input_load_u64` | `(i64) → i64` | Load 8 bytes from input (custom Go impl) |
| `output_set` | `(i64, i64) → void` | Set output buffer |
| `output_length` | `() → i64` | Get output buffer length |
| `output_offset` | `() → i64` | Get output buffer offset |
| `reset` | `() → void` | Reset kernel state |
| `error_set` | `(i64) → void` | Set error message |
| `error_get` | `() → i64` | Get error message offset |
| `memory_bytes` | `() → i64` | Get total memory size |
| `config_get` | `(i64) → i64` | Get config value by key offset |
| `var_get` | `(i64) → i64` | Get variable by key offset |
| `var_set` | `(i64, i64) → void` | Set variable (key, value offsets) |
| `http_request` | `(i64, i64) → i64` | HTTP request (meta offset, body offset) → response offset |
| `http_status_code` | `() → i32` | Get last HTTP status code |
| `http_headers` | `() → i64` | Get response headers as JSON offset |
| `log_trace` | `(i64) → void` | Log trace message |
| `log_debug` | `(i64) → void` | Log debug message |
| `log_info` | `(i64) → void` | Log info message |
| `log_warn` | `(i64) → void` | Log warning message |
| `log_error` | `(i64) → void` | Log error message |
| `get_log_level` | `() → i32` | Get current log level |

### Concurrency Model

Plugin instances are **NOT thread-safe**. Recommended pattern:

```go
// Compile once (expensive)
compiled, _ := extism.NewCompiledPlugin(ctx, manifest, config, hostFuncs)
defer compiled.Close(ctx)

// Create lightweight instances per goroutine (cheap)
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        instance, _ := compiled.Instance(ctx, extism.PluginInstanceConfig{})
        defer instance.Close(ctx)
        _, out, _ := instance.Call("func", input)
    }(i)
}
wg.Wait()
```

Each instance has **isolated state** — separate `Var` map, `Config`, memory. Instances do NOT share state.

### Host Function Patterns

#### Namespace Mechanics
- Default namespace: `"extism:host/user"`
- Custom: `hostFunc.SetNamespace("myservice")`
- Host functions grouped by namespace: `map[string][]HostFunction`
- Each unique namespace becomes a separate wazero `HostModuleBuilder`
- Plugin context passed via `context.WithValue(ctx, PluginCtxKey("plugin"), p)`
- Plugins import by namespace: `//go:wasmimport myservice func_name`

#### Stack-Based ABI

Parameters and return values passed via `[]uint64` stack:
```
stack[0] = first param / first return (overwritten)
stack[1] = second param / second return
...
```

**Value Types:**
```go
ValueTypeI32 = api.ValueTypeI32    // 32-bit integer
ValueTypeI64 = api.ValueTypeI64    // 64-bit integer
ValueTypeF32 = api.ValueTypeF32    // 32-bit float
ValueTypeF64 = api.ValueTypeF64    // 64-bit float
ValueTypePTR = ValueTypeI64        // Pointer (memory address, alias for I64)
```

**When to use each:**
- `ValueTypePTR` — for all memory references (strings, bytes, structs serialized as JSON)
- `ValueTypeI32` — small integers, booleans
- `ValueTypeI64` — large integers
- `ValueTypeF32/F64` — floating point values

**Encoding helpers:**
```go
EncodeI32(int32) uint64     / DecodeI32(uint64) int32
EncodeU32(uint32) uint64    / DecodeU32(uint64) uint32
EncodeI64(int64) uint64     // No DecodeI64 — int64 and uint64 share bit width, just cast
EncodeF32(float32) uint64   / DecodeF32(uint64) float32
EncodeF64(float64) uint64   / DecodeF64(uint64) float64
```

PTR values need no encoding — they're already uint64 offsets.

#### Error Propagation (3 pathways)

1. **Panic in host function** → caught by wazero → returned from `Call()` as Go error with exit code 1
2. **Non-zero exit code** from `_start` → `sys.ExitError` → specific exit code + error
3. **Explicit error_set** → plugin sets error message → host reads via `GetError()`

Example: HTTP to disallowed host panics with `fmt.Errorf("HTTP request to '%v' is not allowed", url)`.

#### Multi-Module Linking

Multiple `.wasm` files in manifest linked via wrapper functions:
- Each module compiled separately, last (or named "main") is entry point
- `createModuleWrapper()` creates host module proxies for each non-main module's exports
- Wrapper functions forward calls to actual module instances, enabling cross-module calls
- All modules share the same extism kernel memory
- Each instance gets unique module names (`{name}_{instanceNum}`) to prevent collision
- Test: `TestModuleLinking` loads `lib.wasm` + `main.wasm`, main calls lib functions

#### Complex Data Exchange
- All complex data (structs, arrays) serialized as JSON, written to memory, passed as PTR offsets
- Pattern: `json.Marshal(data)` → `WriteBytes(jsonBytes)` → `stack[0] = offset`
- Plugin side: `FindMemory(offset).ReadBytes()` → `json.Unmarshal(bytes, &data)`

#### Host Function Examples from Source

**Simple arithmetic (no memory):**
```go
mult := NewHostFunctionWithStack("mult",
    func(ctx context.Context, plugin *CurrentPlugin, stack []uint64) {
        a := DecodeI32(stack[0])
        b := DecodeI32(stack[1])
        stack[0] = EncodeI32(a * b)
    },
    []ValueType{ValueTypePTR, ValueTypePTR},
    []ValueType{ValueTypePTR},
)
```

**String processing (with memory):**
```go
toUpper := NewHostFunctionWithStack("to_upper",
    func(ctx context.Context, plugin *CurrentPlugin, stack []uint64) {
        offset := stack[0]
        buffer, _ := plugin.ReadBytes(offset)
        result := bytes.ToUpper(buffer)
        plugin.Free(offset)
        offset, _ = plugin.WriteBytes(result)
        stack[0] = offset
    },
    []ValueType{ValueTypePTR},
    []ValueType{ValueTypePTR},
)
toUpper.SetNamespace("host")
```

**KV store (persistent state via host closure):**
```go
kvStore := make(map[string][]byte)
kvRead := extism.NewHostFunctionWithStack("kv_read",
    func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
        key, _ := p.ReadString(stack[0])
        value, ok := kvStore[key]
        if !ok { value = []byte{0, 0, 0, 0} }
        stack[0], _ = p.WriteBytes(value)
    },
    []ValueType{ValueTypePTR}, []ValueType{ValueTypePTR},
)
kvWrite := extism.NewHostFunctionWithStack("kv_write",
    func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
        key, _ := p.ReadString(stack[0])
        value, _ := p.ReadBytes(stack[1])
        kvStore[key] = value
    },
    []ValueType{ValueTypePTR, ValueTypePTR}, []ValueType{},
)
```

**HTTP request (built-in host function, from env module):**
- Deserializes JSON `HttpRequest{Url, Headers, Method}` from plugin memory
- Validates against `AllowedHosts` glob patterns
- Performs actual HTTP call
- Stores `LastStatusCode` and `LastResponseHeaders` on plugin
- Returns response body offset
- Panics on disallowed host

### Memory Management Implementation

Memory accessed through `p.Memory()` which returns `p.extism.ExportedMemory("memory")` — the kernel module's exported memory.

**WriteBytes:**
1. `p.Alloc(len(b))` — calls kernel's `alloc` exported function
2. `p.Memory().Write(uint32(ptr), b)` — writes bytes to allocated region
3. Returns offset

**ReadBytes:**
1. `p.Length(offset)` — calls kernel's `length` exported function
2. `p.Memory().Read(uint32(offset), uint32(length))` — reads copy of bytes
3. Returns copy (not a reference)

### Wazero Compilation Cache

```go
// In-memory cache
cache := wazero.NewCompilationCache()

// Persistent directory cache (survives program restarts)
cache, _ := wazero.NewCompilationCacheWithDir("/tmp/wazero-cache")
defer cache.Close(ctx)

config := PluginConfig{
    RuntimeConfig: wazero.NewRuntimeConfig().WithCompilationCache(cache),
}
```

**Lifetime rules:** Don't close cache while runtimes/compiled plugins using it are still alive. Shared cache can be used across all plugins in application.

**Performance:** Compilation cache significantly reduces time waiting to compile the same module a second time.

**Common cache paths in real projects:** Helm uses `helmpath.CachePath("wazero-build")`, wasilibs libraries (used by Hugo and others) use `filepath.Join(userCacheDir(), "com.github.wasilibs")`.

### Guest Runtime Detection

The SDK auto-detects and initializes guest runtimes via `detectGuestRuntime()` / `detectModuleRuntime()`:
- **Haskell**: detects `hs_init(i32, i32)`, calls `_initialize()` then `hs_init(0, 0)`
- **WASI Reactor**: detects `_initialize()` export
- **WASI Command**: detects `__wasm_call_ctors()` export
- Auto-initialized on first plugin call (unless function is `_start` or `_initialize`)

### Hash Verification

Each Wasm entry has optional `Hash` field (SHA256). On load, `calculateHash()` computes actual hash. If manifest provides hash, must match or initialization fails. Uses Go's `crypto/sha256`.

### Observability (Dylibso Observe SDK)

```go
adapter := stdout.NewStdoutAdapter()
adapter.Start(ctx)
config := PluginConfig{
    ObserveAdapter: adapter.AdapterBase,
    ObserveOptions: &observe.Options{
        SpanFilter: &observe.SpanFilter{MinDuration: 1 * time.Nanosecond},
        ChannelBufferSize: 1024,
    },
}
// After creating instance:
instance.traceCtx.Metadata(map[string]string{
    "http.url": "https://example.com",
    "http.status_code": "200",
})
```

### Filesystem Access

```go
manifest := extism.Manifest{
    AllowedPaths: map[string]string{
        "data": "/mnt",              // Read-write mount
        "ro:testdata": "/mnt/test",  // Read-only mount
    },
    Wasm: []extism.Wasm{extism.WasmFile{Path: "fs_plugin.wasm"}},
}
config := extism.PluginConfig{EnableWasi: true}  // Required for FS access
```

### Timeout and Cancellation

```go
// Via manifest
manifest.Timeout = 5000  // 5 seconds

// Via context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
exitCode, output, err := plugin.CallWithContext(ctx, "func", input)
// Timeout: exit code = sys.ExitCodeDeadlineExceeded, err = "module closed with context deadline exceeded"
// Cancel: exit code = sys.ExitCodeContextCanceled, err = "module closed with context canceled"
```

Requires `wazero.NewRuntimeConfig().WithCloseOnContextDone(true)` (automatically set when timeout configured).

### Logging

```go
// Global log level
extism.SetLogLevel(extism.LogLevelDebug)

// Custom logger per plugin
plugin.SetLogger(func(level extism.LogLevel, message string) {
    // Custom handling
})

// From host functions
plugin.Log(extism.LogLevelDebug, "message")
plugin.Logf(extism.LogLevelInfo, "format: %v", arg)
```

### Variables (Persistent State)

```go
// From host side
plugin.Var["key"] = []byte("value")
data := plugin.Var["key"]

// Size limits enforced
// manifest.Memory.MaxVarBytes (default 1MB, -1 = unlimited, 0 = disabled)
// Exceeding limit panics: "Variable store is full"
```

---

## Go PDK (`github.com/extism/go-pdk`)

### Source Structure
```
extism/go-pdk/
├── extism_pdk.go              # Main public API
├── env.go                      # Host function imports (wasmimport declarations)
├── go.mod                      # Go 1.21.0 requirement
├── internal/
│   ├── memory/
│   │   ├── memory.go           # Load/Store/Free on WASM memory
│   │   ├── allocate.go         # Allocate() implementation
│   │   ├── extism.go           # ExtismPointer type, ExtismAlloc/Free/Load/Store
│   │   └── pointer.go          # ExtismPointer = uint64
│   └── http/
│       └── extism_http.go      # HTTP host function imports
├── http/
│   └── httptransport.go        # net/http.RoundTripper implementation
├── wasi-reactor/
│   └── extism_pdk_reactor.go   # Reactor module initialization
└── example/
    ├── countvowels/            # TinyGo and std Go examples
    ├── http/                   # HTTP request examples
    ├── httptransport/          # Standard Go http.Client with PDK transport
    └── reactor/                # WASI reactor module example
```

### Plugin Export Pattern
```go
package main

import "github.com/extism/go-pdk"

//go:wasmexport greet
func greet() int32 {
    input := pdk.Input()
    pdk.OutputString("Hello, " + string(input) + "!")
    return 0  // 0=success, 1=error
}

func main() {} // required for standard Go; not needed for TinyGo with //go:wasmexport
```

**ABI contract:** Exported functions take no parameters, return `int32` (0=success, 1=failure). All data flows through shared memory. Note: TinyGo builds with `//go:wasmexport` do not require a `main()` function. Standard Go builds and the older `//export` pattern do require it.

### JSON Input/Output
```go
//go:wasmexport add
func add() int32 {
    var params struct{ A, B int `json:"a"` `json:"b"` }
    pdk.InputJSON(&params)
    pdk.OutputJSON(struct{ Sum int `json:"sum"` }{params.A + params.B})
    return 0
}
```

### Host Function Import (from plugin side)
```go
//go:wasmimport extism:host/user kv_read
func kvRead(uint64) uint64

//go:wasmexport use_kv
func useKv() int32 {
    mem := pdk.AllocateString("my-key")
    defer mem.Free()
    ptr := kvRead(mem.Offset())
    rmem := pdk.FindMemory(ptr)
    response := string(rmem.ReadBytes())
    pdk.OutputString(response)
    return 0
}
```

### Compilation

**TinyGo (recommended, ~200KB binaries):**
```bash
tinygo build -target wasip1 -buildmode=c-shared -o plugin.wasm main.go
```
- Auto-exports top-level functions
- Each function individually callable by name
- TinyGo 0.34.0+ supports `//go:wasmexport` for reactor modules
- Limited stdlib support

**Standard Go (~2MB binaries):**
```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm main.go
```
- Creates `_start` entry point (WASI command pattern) unless using `//go:wasmexport`
- Full Go stdlib available
- `//go:wasmexport` available Go 1.24+ (experimental in earlier)
- Requires `main()` function

**`-buildmode=c-shared` is recommended** for proper libc and Go runtime initialization. The go-pdk README shows it for both TinyGo and standard Go, though the repo's own Makefile builds standard Go examples without it (using `-tags std` and the older `//export` pattern instead).

| Aspect | TinyGo | Standard Go |
|---|---|---|
| Binary size | ~200KB | ~2MB |
| Exports | Auto by name | `_start` or `//go:wasmexport` |
| Stdlib | Limited | Full |
| `//go:wasmexport` | TinyGo 0.34.0+ | Go 1.24+ |

### extism:host/env Imports (25 functions)

All pointer/offset parameters use `ExtismPointer` which is `uint64` (i64 at Wasm level). The PDK source uses `//go:wasmimport extism:host/env <name>` declarations spread across `env.go`, `internal/memory/extism.go`, and `internal/http/extism_http.go`.

**Input/Output:**
- `input_length() → u64`
- `input_load_u8(offset: u64) → u32`
- `input_load_u64(offset: u64) → u64`
- `output_set(offset: u64, length: u64) → void`
- `error_set(offset: u64) → void`

**Config:**
- `config_get(key_offset: u64) → u64` (returns value offset)

**Variables:**
- `var_get(key_offset: u64) → u64` (returns value offset)
- `var_set(key_offset: u64, value_offset: u64) → void`

**Logging:**
- `log_info/debug/warn/error/trace(offset: u64) → void`
- `get_log_level() → i32`

**HTTP:**
- `http_request(request_offset: u64, body_offset: u64) → u64` (returns response offset)
- `http_status_code() → i32`
- `http_headers() → u64` (returns JSON offset)

**Memory:**
- `alloc(length: u64) → u64` (returns offset)
- `free(offset: u64) → void`
- `length(offset: u64) → u64`
- `length_unsafe(offset: u64) → u64`
- `load_u8(offset: u64) → u32`
- `store_u8(offset: u64, value: u32) → void`
- `load_u64(offset: u64) → u64`
- `store_u64(offset: u64, value: u64) → void`

### Internal Implementation Details

**pdk.Input() internals:**
1. Calls `extismInputLength()` to get buffer size
2. Loads in 8-byte chunks via `extismInputLoadU64()` (optimized)
3. Loads remaining 0-7 bytes via `extismInputLoadU8()`
4. Returns assembled byte slice

**pdk.Output(data) internals:**
1. `AllocateBytes(data)` — calls `ExtismAlloc(len)` + `Store(offset, data)` (8-byte chunked writes)
2. `extismOutputSet(offset, length)` — tells kernel where output is

**Memory type:**
```go
type ExtismPointer uint64  // Simple alias for memory offsets
type Memory struct {
    offset ExtismPointer
    length uint64
}
```
- `Memory.ReadBytes()` — creates buffer, loads via `ExtismLoadU64` in 8-byte chunks
- `Memory.Store(data)` — writes via `ExtismStoreU64` in 8-byte chunks
- `Memory.Free()` — calls `ExtismFree(offset)`
- `Memory.Offset()` → uint64
- `Memory.Length()` → uint64
- `FindMemory(offset)` — calls `ExtismLength(offset)` to get length, constructs Memory

### HTTP Support (3-layer architecture)

1. **Public API** (`extism_pdk.go`): `HTTPRequest`/`HTTPResponse` types, builder pattern, `Send()` marshals metadata to JSON
2. **http.RoundTripper** (`http/httptransport.go`): `HTTPTransport` implements `http.RoundTripper` for standard `http.Client` usage
3. **Host imports** (`internal/http/extism_http.go`): Raw `ExtismHTTPRequest`, `ExtismHTTPStatusCode`, `ExtismHTTPHeaders`

```go
//go:wasmexport http_get
func httpGet() int32 {
    req := pdk.NewHTTPRequest(pdk.MethodGet, "https://api.example.com/data")
    req.SetHeader("Authorization", "Bearer token")
    res := req.Send()
    pdk.OutputMemory(res.Memory())
    return 0
}
```

HTTP methods (iota order): `MethodGet`(0), `MethodHead`(1), `MethodPost`(2), `MethodPut`(3), `MethodPatch`(4), `MethodDelete`(5), `MethodConnect`(6), `MethodOptions`(7), `MethodTrace`(8)

### Complete PDK API Surface

**Input/Output (7):**
- `Input() []byte`
- `InputString() string`
- `InputJSON(v any) error`
- `Output(data []byte)`
- `OutputString(s string)`
- `OutputMemory(mem Memory)`
- `OutputJSON(v any) error`

**Memory (12):**
- `Allocate(length int) Memory`
- `AllocateBytes(data []byte) Memory`
- `AllocateString(data string) Memory`
- `AllocateJSON(v any) (Memory, error)`
- `NewMemory(offset, length uint64) Memory`
- `FindMemory(offset uint64) Memory`
- `Memory.Load(buf []byte)` — read from memory into buffer
- `Memory.Store(data []byte)` — write buffer into memory
- `Memory.Free()` — return memory to host
- `Memory.ReadBytes() []byte` — read entire memory region
- `Memory.Offset() uint64` / `Memory.Length() uint64`

**Config (1):**
- `GetConfig(key string) (string, bool)`

**Variables (5):**
- `GetVar(key string) []byte`
- `SetVar(key string, value []byte)`
- `GetVarInt(key string) int`
- `SetVarInt(key string, value int)`
- `RemoveVar(key string)`

**HTTP (8):**
- `NewHTTPRequest(method HTTPMethod, url string) *HTTPRequest`
- `HTTPRequest.SetHeader(key, value string) *HTTPRequest`
- `HTTPRequest.SetBody(body []byte) *HTTPRequest`
- `HTTPRequest.Send() HTTPResponse`
- `HTTPResponse.Body() []byte`
- `HTTPResponse.Status() uint16`
- `HTTPResponse.Headers() map[string]string`
- `HTTPResponse.Memory() Memory`

**Logging (2):**
- `Log(level LogLevel, s string)`
- `LogMemory(level LogLevel, m Memory)`
- Log levels: `LogTrace`, `LogDebug`, `LogInfo`, `LogWarn`, `LogError`

**Error Handling (2):**
- `SetError(err error)`
- `SetErrorString(err string)`

**Parameter/Result Helpers (8):**
- `ParamBytes(offset uint64) []byte`
- `ParamString(offset uint64) string`
- `ParamU32(offset uint64) uint32`
- `ParamU64(offset uint64) uint64`
- `ResultBytes(d []byte) uint64`
- `ResultString(s string) uint64`
- `ResultU32(d uint32) uint64`
- `ResultU64(d uint64) uint64`

**JSON Helpers (3):**
- `JSONFrom(offset uint64, v any) error`
- `OutputJSON(v any) error`
- `InputJSON(v any) error`

---

## Integration Patterns (from real-world projects)

### Plugin Discovery and Loading

**Directory scanning pattern (Helm):**
- Glob patterns: `filepath.Join(basedir, "*", PluginFileName)`
- Manifest files: `plugin.yaml` (defined as `const PluginFileName = "plugin.yaml"`)
- Load process: directory scan → manifest read/parse → API version detection → format-specific loading
- Supports multiple manifest versions with `peekAPIVersion()` helper

**Plugin registry pattern (Dragonfly-wasm):**
- Dependency resolution with topological sorting for load order
- Supports optional dependencies and `LoadAfter` directives
- Circular dependency detection
- Duplicate plugin name detection

**Navidrome:** Embedded `.ndp` package format (zip archive containing `plugin.wasm` + `manifest.json`), JSON manifests parsed with `json.Unmarshal`.

### Lifecycle State Machine (Dragonfly-wasm)
```
Unloaded(0) → Loading(1) → Loaded(2) → Enabling(3) → Enabled(4)
                                ↑                         ↓
                            Disabled(6) ← Disabling(5) ←─┘
              Loading/Enabling failures → Error(7)
              Any state → Unloaded (via explicit UnloadPlugin())
```
- Mutex-protected state transitions per plugin (manager-level `sync.RWMutex` + per-plugin `sync.Mutex`)
- `plugin_init` is **required** — must exist and succeed or plugin enters Error state
- Optional `on_enable` and `on_disable` callbacks
- Initialization timeout = 10x execution timeout

### Pooling Patterns

**Pattern 1: CompiledPlugin + per-goroutine instances (recommended)**
```go
compiled, _ := extism.NewCompiledPlugin(ctx, manifest, config, hostFuncs)
// Each goroutine: instance, _ := compiled.Instance(ctx, PluginInstanceConfig{})
```

**Pattern 2: sync.Pool + semaphore (kubiyabot)**
```go
var pluginPool sync.Pool
var sem = semaphore.NewWeighted(10)
pluginPool.New = func() interface{} {
    plugin, _ := extism.NewPlugin(ctx, manifest, config, [])
    return plugin
}
```
Note: Basic sync.Pool not fully thread-safe for Extism plugins; need additional synchronization.

**Pattern 3: UUID-keyed compilation cache (raptor)**
```go
type DefaultModCache struct {
    mu    sync.RWMutex
    cache map[uuid.UUID]wazero.CompilationCache
}
```
Separate compilation cache per module/plugin, thread-safe with RWMutex.

### Error Handling at Scale

**Panic recovery:**
```go
defer func() {
    if r := recover(); r != nil {
        log.Error("Panic while loading plugin", "plugin", plugin.ID, "panic", r)
    }
}()
```

**Timeout pattern (goroutine + channel + select):**
```go
ctx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
resultCh := make(chan result, 1)
go func() {
    _, output, err := instance.Call("func", data)
    resultCh <- result{output, err}
}()
select {
case <-ctx.Done(): return fmt.Errorf("handler timeout")
case r := <-resultCh: // handle result
}
```

**Exit code checking (Helm):**
```go
exitCode, outputData, err := pe.Call(entryFuncName, inputData)
if err != nil { return nil, fmt.Errorf("plugin error: %w", err) }
if exitCode != 0 { return nil, &InvokeExecError{ExitCode: int(exitCode)} }
```

**Error persistence (Dragonfly-wasm):** Plugin errors tracked in-memory via `Metrics` struct (`ErrorCount`, `LastError`, `LastErrorTime`), auto-enter Error state on init failure, metrics tracking via `Metrics.RecordError()`.

### Testing

**XTP test framework:** Write test plugins in Wasm that test other plugins:
```bash
xtp plugin test kvplugin.wasm --with kvtest.wasm --mock-host kvhost.wasm
```
- `xtptest.CallString("func", data)` — call plugin function from test
- `xtptest.AssertEq/AssertNe` — assertion helpers
- `--mock-input-data` / `--mock-input-file` for test input

**Mock host functions (Navidrome pattern):**
- Abstraction layer: import `pdk` not `extism/go-pdk` directly
- For WASM builds: delegates to real `extism/go-pdk`
- For native test builds: uses `testify/mock` implementations
- Allows unit testing plugin logic without WASM toolchain

**Capability detection:**
```go
capabilities := detectCapabilities(instance)  // Check what functions plugin exports
instance.Close(ctx)
if err := ValidateWithCapabilities(manifest, capabilities); err != nil { return err }
```

### Performance Considerations

- **Compile once, instantiate many** — compilation is expensive, instantiation is cheap
- **Compilation caching** — directory-based cache persists across program runs
- **Memory limits tuning** — `MaxPages` (64KB each), `MaxHttpResponseBytes`, `MaxVarBytes`
- **Instance reuse** — for high-concurrency, use CompiledPlugin; for batch, direct NewPlugin is fine

### Real-World Applications Using Extism

**Helm (Kubernetes):**
- Production K8s plugin system with multi-runtime support (subprocess + Extism)
- Per-runtime host function registration
- Configurable max pages + HTTP response size limits
- Global timeout per manifest
- Sandboxed filesystem with allowed paths

**Navidrome (Music Server):**
- 10 host function service categories: Config, SubsonicAPI, Scheduler, WebSocket, Artwork, Cache, Library, KVStore, Users, HTTP
- Plugin manifest declares required capabilities (permission model)
- Shared compilation cache across plugins: `WithCompilationCache(m.cache)`
- testify/mock for unit testing

**Dragonfly-wasm (Minecraft Server):**
- Per-plugin state machine with full lifecycle management
- Host functions manually written as wrappers around a `ServerAPI` interface (not code-generated)
- Dependency resolution with topological sort
- Event dispatcher pattern for plugin communication
- Per-plugin execution metrics tracking

**1Password SDK** (`1Password/onepassword-sdk-go`):
- Security-critical password manager
- 4 namespace-grouped host functions: `"op-extism-core"` (random_fill), `"op-now"` (unix_time_milliseconds), `"zxcvbn"` (unix_time_milliseconds), `"op-time"` (utc_offset_seconds)
- Stack-based value encoding for memory safety

**KubeCon** (`wasmkwokwizardry/kubecon-eu-2025` — "Get WITty: Evolving Kubernetes Scheduling With the WebAssembly Component Model"):
- Benchmarks showing sequential vs parallel performance (imports `sigs.k8s.io/kube-scheduler-wasm-extension` as a dependency)
- Compilation cache at `/tmp/wazero-cache`
- Parallel instances show significant speedup over sequential

---

## Dependency Graph (from extism DEVELOPING.md)

```
runtime → libextism → extism-maturin → python-sdk
                    → nuget-extism
                    → ruby-sdk
runtime → go-sdk → cli
plugins → libextism, python-sdk, ruby-sdk, go-sdk, js-sdk
js-sdk (standalone — uses platform-native Wasm, not libextism)
```

## Key Design Patterns Summary

1. **Two-stage instantiation** — compile once (expensive), instantiate many (cheap)
2. **Context-based plugin access** — plugin context through `context.WithValue()`
3. **Stack-based host function ABI** — raw WASM stack values with encoding helpers
4. **Memory boxing** — all complex data passed as memory offsets
5. **Resource cleanup chaining** — multiple closers collected and called in order
6. **Namespace grouping** — host functions organized into logical modules
7. **Bump allocator kernel** — lightweight memory management in plugin linear memory

---

## Sources

- [Extism GitHub](https://github.com/extism/extism) — runtime, kernel (`kernel/src/lib.rs`), `DEVELOPING.md`
- [Extism Go SDK](https://github.com/extism/go-sdk) — `extism.go`, `plugin.go`, `host.go`, `runtime.go`
- [Extism Go PDK](https://github.com/extism/go-pdk) — `extism_pdk.go`, `env.go`, `internal/memory/`, `internal/http/`
- [Extism Docs](https://extism.org/docs/) — concepts, FAQ, host SDK and PDK listings
- [Dylibso v1 Announcement](https://dylibso.com/blog/announcing-extism-v1/)
- [Wazero](https://github.com/tetratelabs/wazero) — `cache.go`, `config.go`, `sys/error.go`
- [XTP Test Framework](https://github.com/dylibso/xtp-test-go)
- [Helm](https://github.com/helm/helm) — `internal/plugin/`
- [Navidrome](https://github.com/navidrome/navidrome) — `plugins/`
- [Dragonfly-wasm](https://github.com/EinBexiii/dragonfly-wasm) — `internal/manager/`, `pkg/plugin/`, `pkg/host/`
- [1Password SDK Go](https://github.com/1Password/onepassword-sdk-go) — `internal/imported.go`
- [KubeCon Wasm Extension](https://github.com/wasmkwokwizardry/kubecon-eu-2025) — `filter-json/host/benchmark_test.go`
