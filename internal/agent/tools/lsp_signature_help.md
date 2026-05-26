Get signature help for a function or method call at a specific position via LSP; shows parameter info and overloads.

<usage>
- Provide file_path, line (1-based), and character (1-based) position inside a function call.
- Tool sends a textDocument/signatureHelp request to the LSP server.
- Returns available signatures, parameter names, and active parameter index.
</usage>

<features>
- Shows all overloads for a function or method call.
- Highlights the currently active parameter.
- Includes parameter labels and documentation.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Best used inside function call parentheses.
- Results depend on LSP server capabilities.
</limitations>

<tips>
- Use to understand required and optional parameters when calling functions.
- Position the cursor inside the parentheses of a function call for best results.
- Combine with lsp_hover for detailed type information.
- Line and character are 1-based (as shown in editors).
</tips>
