Get all document symbols for a file via LSP; returns the hierarchical symbol tree (functions, types, variables, etc.).

<usage>
- Provide file_path to get all symbols defined in the file.
- Returns a tree of symbols with their types and line numbers.
- Supports nested symbols (e.g., methods inside a class).
</usage>

<features>
- Returns the full symbol hierarchy for a single file.
- Shows symbol kinds: Function, Class, Method, Variable, Interface, Struct, Enum, etc.
- Provides line numbers for quick navigation.
- Supports multiple programming languages via configured LSP servers.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Results depend on LSP server capabilities and project indexing state.
</limitations>

<tips>
- Use to quickly understand the structure of a file before editing.
- Use with lsp_hover or lsp_definition to get more details on specific symbols.
- Symbols are shown as a tree reflecting nesting (e.g., methods under their class).
</tips>
