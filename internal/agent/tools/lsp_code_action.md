Request code actions (quick fixes, refactorings) for a range via LSP.

<usage>
- Provide file_path and a range (start_line, start_char, end_line, end_char, all 1-based).
- Optionally filter by kind (e.g., quickfix, refactor, source.organizeImports).
- Returns available code actions with descriptions and associated edits.
</usage>

<features>
- Semantic code actions via LSP (quick fixes, refactorings, source actions).
- Supports multiple programming languages via configured LSP servers.
- Returns action titles, kinds, and preview of associated edits.
- Can filter by action kind for targeted results.
</features>

<limitations>
- Requires a running LSP server for the file type.
- Results depend on LSP server capabilities and available actions.
- Returns available actions; use the edit tool to apply individual changes.
</limitations>

<tips>
- Use without a kind filter to see all available actions.
- Use kind=quickfix for quick fixes on diagnostic errors.
- Use kind=refactor for refactoring suggestions.
- Use kind=source.organizeImports to organize imports.
</tips>
