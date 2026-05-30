Get symbols from LSP — either document symbols for a file or workspace symbols matching a query.

<usage>
- Provide file_path to get all symbols defined in that file (functions, types, variables, etc.).
- Provide query to search for symbols across the entire workspace matching the given name.
- If both file_path and query are provided, file_path takes priority.
- If neither is provided, returns an error.
</usage>

<features>
- Scope-aware: returns document symbols for file-level or workspace symbols for project-wide search.
- Hierarchical output for document symbols showing parent-child relationships.
- Supports multiple programming languages via configured LSP servers.
</features>

<limitations>
- Requires a running LSP server for the file type (document symbols) or any configured LSP server (workspace symbols).
- Results depend on LSP server capabilities and project indexing state.
</limitations>

<tips>
- Use with file_path to understand the structure of a file — classes, functions, methods, variables.
- Use with query to find where a symbol is defined across the project.
- Combine with lsp_definition and lsp_references for full symbol exploration.
</tips>
