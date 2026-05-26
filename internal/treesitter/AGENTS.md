# Tree-sitter Integration

## Overview

Parser pool, grammar loading, and query execution for code analysis.
Requires `CGO_ENABLED=1` and a C compiler. A non-CGO build panics at init
(`cgo_check_nocgo.go`).

## Structure

| File | Purpose |
|------|---------|
| `treesitter.go` | Core types: `Tag`, `SymbolInfo`, `FileAnalysis`, `ImportInfo`, `Parser` interface |
| `parser.go` | `ParserPool` (channel-based), `parser` scaffold, `Analyze()` / `ParseTree()` |
| `query.go` | `QueryLoader`: compiles and caches `.scm` queries, runs tag extraction |
| `cache.go` | LRU `Cache` for parsed AST trees (5000 entries, 256 MB) |
| `languages.go` | Extension-to-language mapping, aliases, `MapPath()`, `GetQueryKey()` |
| `imports.go` | Per-language import extraction (Go, Python, TS/JS, Java, Rust, Ruby, etc.) |
| `embed.go` | `//go:embed queries/* languages.json` and manifest loading |
| `cgo_check*.go` | Build-tag guards |

## Parser Pool

Channel-based pool sized to `runtime.NumCPU()`. Acquire blocks until a
parser is free or the context/pool closes. Release returns it. Close drains
all parsers and waits for active holders. `ParserConfig.PoolSize` overrides
the default.

## Grammar Loading

28 tree-sitter grammars compiled via CGO from vendored Go modules (see
`parser.go` imports). `initLanguages()` maps each language to a
`tree_sitter.Language` pointer and registers it with `QueryLoader`. Language
list comes from the embedded `languages.json` manifest.

## Query Execution

`QueryLoader` compiles `.scm` files once per language and caches the compiled
`tree_sitter.Query`. 38 embedded query files cover tags and imports.
`ExtractTagsWithCursor()` walks the AST and returns `Tag` (def/ref) and
`SymbolInfo` slices. Import extraction dispatches per-language from `imports.go`.

## Caching

`Cache` wraps `hashicorp/golang-lru/v2` with byte-budget eviction. Defaults:
5000 entries, 256 MB. Stores master parse trees, returns clones. Cache key
is FNV-1a of `(path, content)`.

## Consumers

- **repomap** (`internal/repomap/`): `Analyze()` for tag extraction and scope rendering.
- **lcm/explorer** (`internal/lcm/`): `ParseTree()` and `Analyze()` for code file analysis.

## Anti-Patterns

- Never call `runtime/debug.SetLimit(0)`. It deadlocks with tree-sitter CGO.
- Never build with `CGO_ENABLED=0`. The `!cgo` build tag panics at init.
- Never edit embedded `.scm` files by hand. They are vendored.
- Always `defer pool.release(lp)`. Never hold parsers across goroutines.
