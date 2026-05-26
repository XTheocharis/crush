Replace the body of a brace-delimited symbol (function, method, struct, etc.) at a given line.

<usage>
- Provide file_path pointing to the source file.
- Provide line (1-based) where the symbol declaration starts.
- Provide new_body with the replacement content (inside the braces).
- The signature/opening brace line and closing brace are preserved.
</usage>

<features>
- Replaces only the body while preserving the symbol signature.
- Works with any brace-delimited construct (functions, structs, interfaces, etc.).
- No LSP required — operates purely on text.
</features>

<limitations>
- Requires the symbol to use brace delimiters.
- The opening brace must be on or after the specified line.
- Does not validate syntax of the new body.
</limitations>

<tips>
- Use lsp_definition to find the exact line of a symbol before replacing.
- Preview the symbol first with the view tool.
- For simple line edits, the edit tool may be more appropriate.
</tips>
