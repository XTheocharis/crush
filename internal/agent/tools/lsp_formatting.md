Format a file using LSP formatting; returns text edits for consistent code style.

<usage>
- Provide file_path to the file you want to format.
- Tool sends a textDocument/formatting request to the configured LSP server.
- Returns the list of text edits needed to format the file.
</usage>

<features>
- Uses the language server's built-in formatter for accurate results.
- Respects project-specific formatting settings (indent style, tab width).
- Supports all languages with a configured LSP server that provides formatting.
</features>

<limitations>
- Requires a running LSP server for the file type that supports formatting.
- Formatting options (tab size, insert spaces) use sensible defaults.
</limitations>

<tips>
- Use after editing files to maintain consistent formatting.
- Combine with other LSP tools for a complete code intelligence workflow.
</tips>
