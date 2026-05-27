Get code completion suggestions at a specific position via LSP; shows available symbols and their types.

<usage>
- Provide file_path, line (1-based), and character (1-based) position.
- Tool sends a textDocument/completion request to the LSP server.
- Returns a list of completion items with labels and type details.
</usage>

<features>
- Provides intelligent code completions based on language semantics.
- Shows type information and documentation for each suggestion.
- Supports all languages with a configured LSP server that provides completions.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Results depend on LSP server capabilities and project indexing state.
</limitations>

<tips>
- Use to discover available methods, fields, and variables at a position.
- Combine with lsp_hover for detailed type information on specific items.
- Line and character are 1-based (as shown in editors).
</tips>
