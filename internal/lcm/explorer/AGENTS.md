# LCM Explorer

File-type exploration subsystem for lossless context management. Produces
structured summaries of files using a priority-based explorer registry with
optional LLM and agent enhancement (three-tier dispatch).

## Structure

**Core:**
- `explorer.go` - `Explorer` interface (`CanHandle`/`Explore`), `Registry`,
  `ExploreInput`, `ExploreResult`, helpers (`sampleContent`, `hexDump`,
  `looksLikeText`, `detectShebang`)
- `explorer_llm.go` - `LLMClient` interface, `AgentFunc` type, three-tier
  dispatch (`exploreLLMEnhanced`), `NewRegistryWithLLM`
- `explorer_prompts.go` - System prompts and `languagePrompts` map (10
  languages) for LLM and agent tiers
- `extensions.go` - `TEXT_EXTENSIONS` and `BINARY_EXTENSIONS` maps
- `formatter.go` - `OutputProfile` (`parity`/`enhancement`), section-based
  summary rendering with truncation markers
- `heuristic.go` - `EnrichAnalysis`: import categorization, visibility
  inference, idiom detection, module pattern detection
- `conformance.go` - `ConformanceSnapshot`: Volt parity sign-off inputs

**File-type explorers (registered in priority order):**
- `archive.go` - `ArchiveExplorer`: ZIP, TAR, GZIP, BZIP2, ZSTD, DEB, RPM
- `binary.go` - `BinaryExplorer` (generic binary), `TextExplorer` (text
  with sampling), `FallbackExplorer` (always matches)
- `pdf.go` - `PDFExplorer`, `image.go` - `ImageExplorer`,
  `executable.go` - `ExecutableExplorer` (ELF/Mach-O/PE)
- `data.go` - `JSONExplorer`, `CSVExplorer`, `YAMLExplorer`,
  `TOMLExplorer`, `INIExplorer`, `XMLExplorer`, `HTMLExplorer`
- `markdown.go` - `MarkdownExplorer`, `latex.go` - `LatexExplorer`
- `sqlite.go` - `SQLiteExplorer`, `logs.go` - `LogsExplorer`
- `shell.go` - `ShellExplorer`
- `code_treesitter.go` - `TreeSitterExplorer`: code analysis via tree-sitter
  with enriched heuristic metadata

**Supporting:**
- `file_structure.go` - `SymbolInfo`, `CodeSection`, `FileStructure`
- `runtime.go` - `RuntimeAdapter`: wraps `Registry` for LCM, returns
  summary + explorer name + persistence decision
- `runtime_inventory.go` - `RuntimePersistenceMatrix`, `RuntimePersistencePolicy`,
  `RuntimeIngestionPath`: persistence decisions per explorer type
- `parity_fixtures.go`, `parity_provenance.go` - Parity testing fixtures
  and provenance tracking
- `protocol_artifacts.go` - `TokenizerSupport`, `ExplorerFamilyMatrix`
- `tempfile.go` - `withTempFile` helper
- `stdlib/` - Per-language stdlib membership functions (15 files: c, common,
  cpp, csharp, go, haskell, java, kotlin, node, php, python, ruby, rust,
  scala, swift)

## Three-Tier Dispatch

`Registry.Explore` runs tiers in sequence:

1. **Static (template)** - First matching `Explorer` produces baseline result.
   Always runs.
2. **LLM (O19a)** - `LLMClient` configured: content truncated to 50K chars
   (40K head + 10K tail), single-call summarization. Gets `+llm`.
3. **Agent (O19b)** - `AgentFunc` + `SessionID` set: language-specific
   prompt spawns agent sub-session. Gets `+agent`.

Python exception: skips tier 2, goes tier 1 -> tier 3 directly.

## Registry Priority

First `CanHandle` wins: Archive -> PDF/Image/Executable -> Binary ->
Data formats (JSON/CSV/YAML/TOML/INI/XML/HTML/Markdown/LaTeX/SQLite/Logs) ->
TreeSitter (when parser configured, inserted after Logs) -> Shell ->
Text -> Fallback.

## Enriched Analysis

`EnrichAnalysis` augments tree-sitter output for 13 languages:
- **Import categorization**: stdlib/third-party/local via `stdlib/` functions
  (`IsGoStdlib`, `IsPythonStdlib`, `IsNodeStdlib`, `IsRustStdlib`, etc.)
- **Symbol visibility**: public/private/package (Go: uppercase, Python: `_`,
  Java: modifiers, Rust: `pub`)
- **Idioms**: `dataclass`, `react_component`, `async_generator`
- **Module patterns**: `python_main_guard`, `esm_default_export`,
  `go_main_package`

## Integration

- `NewRegistry` or `NewRegistryWithLLM` creates the explorer chain
- `RuntimeAdapter` wraps registry for LCM decorator: summary + persistence
  decision in one call
- `TreeSitterExplorer` requires `treesitter.Parser` (CGO)
- `RuntimePersistenceMatrix` determines persistence to `lcm_large_files`

## Anti-Patterns

- Never add an explorer matching before `BinaryExplorer` without handling
  large binary content. `BinaryExplorer` is the safety net.
- `FallbackExplorer.CanHandle` always returns true. Insert new explorers
  before it.
- Tree-sitter skipped for files over `MaxFullLoadSize` (50 MB). Don't
  increase without profiling memory.
- `LLMClient` and `AgentFunc` are defined locally to avoid circular imports
  with `internal/lcm`.
