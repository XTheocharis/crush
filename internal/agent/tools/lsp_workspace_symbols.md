Search for symbols across the workspace via LSP; returns matching functions, types, variables, etc. from all files.

<usage>
- Provide query to search for symbols across the entire workspace.
- Returns symbol name, kind, containing symbol, file, and line number.
- Searches all configured LSP servers.
</usage>

<features>
- Project-wide symbol search across all files.
- Shows symbol kind (Function, Class, Interface, etc.) and container (parent symbol).
- Aggregates results from all configured LSP servers.
- Useful for finding where a type or function is defined.
</features>

<limitations>
- Requires at least one configured LSP server.
- Results depend on LSP server capabilities and project indexing state.
- Search quality varies by LSP server implementation.
</limitations>

<tips>
- Use partial names for fuzzy matching (e.g., "Handler" to find all handler types).
- Combine with lsp_definition to navigate to the symbol's source.
- Use lsp_document_symbols for a complete symbol tree of a specific file.
</tips>
