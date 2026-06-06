#!/usr/bin/env bash
# Test: Tree-sitter tag extraction and import resolution.
# Verifies that Crush extracts Go tags (functions, types, methods) and
# populates the import graph via tree-sitter when run against this repo.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-verify.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Tags extracted for Go files
# ---------------------------------------------------------------------------
test_go_tags_extracted() {
  echo "=== Scenario 1: Tags extracted for Go files ==="

  setup_clean_crush
  # shellcheck disable=SC2317  # restore_crush is called below
  restore_crush() {
    command restore_crush
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
  }
  trap restore_crush EXIT

  start_crush 2
  send_prompt "Show me the main.go file"
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 9 "treesitter-tags"
    stop_crush
    return
  fi

  local SID
  SID=$(get_session_id)
  if [[ -z "$SID" ]]; then
    fail "Scenario 1: No session ID found in DB"
    capture_evidence 9 "treesitter-tags"
    stop_crush
    return
  fi

  # Verify Go tags were extracted (count > 0).
  local tag_count
  tag_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags WHERE language='go'" | jq '.[0].count')
  if [[ "$tag_count" -gt 0 ]]; then
    pass "Scenario 1: $tag_count Go tags extracted"
  else
    fail "Scenario 1: No Go tags found in repo_map_tags"
  fi

  # Verify 'go' appears in distinct languages.
  local languages
  languages=$(query_db "SELECT DISTINCT language FROM repo_map_tags" | jq -r '.[].language')
  if echo "$languages" | grep -qx "go"; then
    pass "Scenario 1: 'go' present in extracted languages"
  else
    fail "Scenario 1: 'go' not found in distinct languages: $languages"
  fi

  local main_tag_count
  main_tag_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags WHERE language='go' AND rel_path='main.go' AND name='main'" | jq '.[0].count')
  if [[ "$main_tag_count" -ge 1 ]]; then
    pass "Scenario 1: main.go main symbol extracted"
  else
    fail "Scenario 1: Expected main.go main symbol in repo_map_tags"
  fi

  local app_tag_count
  app_tag_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_tags WHERE language='go' AND rel_path='internal/app/app.go' AND name='App'" | jq '.[0].count')
  if [[ "$app_tag_count" -ge 1 ]]; then
    pass "Scenario 1: internal/app/app.go App symbol extracted"
  else
    fail "Scenario 1: Expected App symbol from internal/app/app.go"
  fi

  # Log sample tags for evidence.
  local sample_tags
  sample_tags=$(query_db "SELECT name, kind, rel_path FROM repo_map_tags WHERE language='go' LIMIT 10")
  echo "Sample Go tags:"
  echo "$sample_tags" | jq -r '.[] | "  \(.kind) \(.name) in \(.rel_path)"'

  capture_evidence 9 "treesitter-tags"
  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 2: Import resolution populated
# ---------------------------------------------------------------------------
test_imports_populated() {
  echo "=== Scenario 2: Import resolution populated ==="

  # Verify import graph has rows.
  local import_count
  import_count=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports" | jq '.[0].count')
  if [[ "$import_count" -gt 0 ]]; then
    pass "Scenario 2: $import_count imports in repo_map_imports"
  else
    fail "Scenario 2: No imports found in repo_map_imports"
  fi

  local cobra_imports
  cobra_imports=$(query_db "SELECT COUNT(*) as count FROM repo_map_imports WHERE path='main.go' AND import_path='github.com/charmbracelet/crush/internal/cmd'" | jq '.[0].count')
  if [[ "$cobra_imports" -ge 1 ]]; then
    pass "Scenario 2: main.go import edge to internal/cmd recorded"
  else
    fail "Scenario 2: Expected main.go -> internal/cmd import edge"
  fi

  # Log sample imports for evidence.
  local sample_imports
  sample_imports=$(query_db "SELECT path, import_path FROM repo_map_imports LIMIT 5")
  echo "Sample imports:"
  echo "$sample_imports" | jq -r '.[] | "  \(.path) -> \(.import_path)"'

  capture_evidence 9 "imports"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_go_tags_extracted
test_imports_populated

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
