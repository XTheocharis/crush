Check if a symbol can be safely deleted by inspecting references via LSP.

<usage>
- Provide file_path, line, and character (1-based) pointing to the symbol.
- Tool queries LSP for all references to the symbol.
- Reports whether the symbol has external references that would break.
- Returns a warning listing external references if any are found.
</usage>

<features>
- Workspace-wide reference checking via LSP before deletion.
- Identifies all external references that would break.
- Prevents accidental breaking changes during refactoring.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Only checks references; does not perform the actual deletion.
- Use the edit tool or bash to remove the symbol after confirming safety.
</limitations>

<tips>
- Use lsp_references first for a broader view of symbol usage.
- Combine with lsp_definition to navigate to symbol locations.
- After confirming safety, use the edit tool to remove the symbol.
</tips>
