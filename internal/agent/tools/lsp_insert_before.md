Insert text before a specified line in a file.

<usage>
- Provide file_path pointing to the source file.
- Provide line (1-based) before which to insert.
- Provide text containing the content to insert.
- Text may contain newlines for multi-line insertion.
</usage>

<features>
- Inserts content before the target line without modifying existing lines.
- No LSP required — operates purely on text.
- Useful for adding comments, imports, or declarations above symbols.
</features>

<limitations>
- Does not validate the inserted text's syntax.
- Does not resolve symbol boundaries — only uses line numbers.
</limitations>

<tips>
- Use lsp_definition to find the exact line of a symbol.
- Combine with lsp_safe_delete for safe refactoring workflows.
- For appending after a symbol's full block, use lsp_insert_after instead.
</tips>
