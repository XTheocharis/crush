Insert text after a specified line in a file.

<usage>
- Provide file_path pointing to the source file.
- Provide line (1-based) after which to insert.
- Provide text containing the content to insert.
- Text may contain newlines for multi-line insertion.
</usage>

<features>
- Inserts content after the target line without modifying existing lines.
- No LSP required — operates purely on text.
- Useful for adding new functions, methods, or blocks after existing ones.
</features>

<limitations>
- Does not validate the inserted text's syntax.
- Does not resolve symbol boundaries — only uses line numbers.
</limitations>

<tips>
- Use lsp_definition to find the exact line of a symbol.
- To insert before a symbol's declaration, use lsp_insert_before instead.
- For replacing a symbol's body, use lsp_replace_symbol.
</tips>
