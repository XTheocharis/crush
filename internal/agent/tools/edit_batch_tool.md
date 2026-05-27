Apply multiple edit operations atomically (all-or-nothing rollback on failure). Supports string-replacement and position-independent anchor-hash operations. Use for batch modifications across multiple files where atomicity is critical.

<parameters>
1. ops: Array of edit operations (required, at least 1). Each operation is one of:

  String-replacement (default when op is omitted):
  - file_path: File to modify (required)
  - old_content: Text to find and replace (required)
  - new_content: Replacement text (required)

  Position-independent operations (use anchor hashes instead of line numbers):
  - file_path: File to modify (required)
  - op: "insert_before" | "insert_after" | "replace_range" | "delete_range" (required)
  - anchor_hash: Hash for insert_before/insert_after (required for those ops)
  - start_hash, end_hash: Range boundaries for replace_range/delete_range (required for those ops)
  - content: Content to insert or replace with (required for insert/replace, omit for delete)
</parameters>

<operation>
- All operations are applied atomically: if any operation fails, all changes are rolled back.
- Position-independent ops use anchor hashes from View output (e.g., <hash:a1b2c3d4>).
- Anchor-based ops are drift-tolerant: they work even if lines were added or removed before the anchor.
- Anchor maps are rebuilt after successful batch application.
</operation>

<critical_requirements>
1. All operations must succeed for changes to persist.
2. For anchor ops, use hash values from the most recent View output.
3. Anchor drift tolerance is ±5 lines from the original position.
4. If rollback occurs, check the error message for the failing operation and retry.
</critical_requirements>

<examples>
String-replacement batch:
```json
{
  "ops": [
    {"file_path": "/src/main.go", "old_content": "func old() {}", "new_content": "func new() {}"},
    {"file_path": "/src/util.go", "old_content": "var x = 1", "new_content": "var x = 2"}
  ]
}
```

Anchor-based insert:
```json
{
  "ops": [
    {"file_path": "/src/main.go", "op": "insert_before", "anchor_hash": "a1b2c3d4", "content": "// new comment"},
    {"file_path": "/src/main.go", "op": "insert_after", "anchor_hash": "e5f6a7b8", "content": "extra := true"}
  ]
}
```

Anchor-based range replace and delete:
```json
{
  "ops": [
    {"file_path": "/src/main.go", "op": "replace_range", "start_hash": "a1b2c3d4", "end_hash": "e5f6a7b8", "content": "// replaced block"},
    {"file_path": "/src/util.go", "op": "delete_range", "start_hash": "11223344", "end_hash": "55667788"}
  ]
}
```
</examples>
