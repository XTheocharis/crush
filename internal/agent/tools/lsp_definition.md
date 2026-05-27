Find the definition of a symbol at a specific position via LSP; navigates to the source definition.

<usage>
- Provide file_path, line (1-based), and character (1-based) position.
- Tool uses the LSP to find where the symbol at that position is defined.
- Returns definition locations grouped by file with line and column numbers.
</usage>

<features>
- Precise go-to-definition via LSP (more accurate than text search).
- Supports multiple programming languages via configured LSP servers.
- Returns exact source locations for type definitions, function implementations, etc.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Results depend on LSP server capabilities and project indexing state.
</limitations>

<tips>
- Use this to navigate to the source of a function, type, or variable.
- Combine with lsp_references for full symbol exploration.
- Line and character are 1-based (as shown in editors).
</tips>
