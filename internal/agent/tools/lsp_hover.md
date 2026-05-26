Get hover information for a symbol at a specific position via LSP; shows type info and documentation.

<usage>
- Provide file_path, line (1-based), and character (1-based) position.
- Tool sends a textDocument/hover request to the LSP server.
- Returns type information, documentation, and signature for the symbol.
</usage>

<features>
- Retrieves type information and documentation for symbols.
- Shows function signatures, type definitions, and doc comments.
- Supports multiple programming languages via configured LSP servers.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Results depend on LSP server capabilities and project indexing state.
</limitations>

<tips>
- Use to quickly understand the type and documentation of a symbol.
- Combine with lsp_definition to navigate to the source.
- Line and character are 1-based (as shown in editors).
</tips>
