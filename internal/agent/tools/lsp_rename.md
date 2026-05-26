Rename a symbol across the workspace via LSP; safely refactors all references.

<usage>
- Provide file_path, line, character (1-based) pointing to the symbol to rename.
- Provide new_name for the symbol.
- Tool uses the LSP to compute all locations that need updating.
- Returns a summary of all changes without applying them.
</usage>

<features>
- Workspace-wide symbol renaming via LSP (safe and accurate).
- Renames all references, declarations, and uses consistently.
- Supports multiple programming languages via configured LSP servers.
- Returns a preview of all changes before application.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Returns a change summary; use the edit tool to apply individual changes.
- Some LSP servers may not support rename for certain symbol types.
</limitations>

<tips>
- Use lsp_references first to understand the scope of the rename.
- Verify the change summary before applying edits.
- For simple single-file renames, the edit tool with replace_all may be faster.
</tips>
