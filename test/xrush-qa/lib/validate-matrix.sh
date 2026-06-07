#!/usr/bin/env bash
# validate-matrix.sh — Validates GAP_MATRIX.md for structural correctness.
#
# Checks:
#   1. Every TUI Scenario Path referenced in the matrix actually exists on disk.
#   2. Every RESOLVED row has a non-empty TUI Scenario Path.
#   3. No row uses `go test` in any column as primary evidence.
#   4. No stale path patterns (test-processor.sh, test-routing.sh, etc.)
#      unless the file actually exists.
#   5. RESOLVED rows must have a TUI Capture Artifact.
#
# Usage:
#   bash test/xrush-qa/lib/validate-matrix.sh [--fix]
#
# Exit 0 if all checks pass, exit 1 otherwise.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MATRIX_FILE="$SCRIPT_DIR/../GAP_MATRIX.md"
QA_DIR="$SCRIPT_DIR/.."

# Stale path patterns that should not appear unless the file actually exists.
STALE_PATTERNS=(
  "test-processor.sh"
  "test-routing.sh"
  "test-tui.sh"
  "test-extensions.sh"
  "test-lsp.sh"
  "test-tools-surface.sh"
  "test-db-migrations.sh"
)

errors=0

# ---------- Helper functions ----------

log_error() {
  echo "FAIL: $*" >&2
  errors=$((errors + 1))
}

log_ok() {
  echo "  OK: $*"
}

# Extract data rows from the markdown table (lines starting with | F).
# Skips the header, separator, and non-table lines.
extract_rows() {
  grep '^| *F[0-9]' "$MATRIX_FILE"
}

# Get a specific column value from a pipe-delimited row.
# Usage: get_column "$row" <column_number>
get_column() {
  local row="$1"
  local col="$2"
  echo "$row" | awk -F'|' -v c="$col" '{gsub(/^ +| +$/, "", $c); print $c}'
}

# ---------- Check 1: Referenced paths exist ----------

check_paths_exist() {
  echo ""
  echo "=== Check 1: Referenced TUI scenario paths exist ==="
  local rows
  rows=$(extract_rows)
  local count=0

  while IFS= read -r row; do
    [ -z "$row" ] && continue
    local fpath
    fpath=$(get_column "$row" 4)

    # Skip empty (OPEN rows have no path)
    if [ -z "$fpath" ] || [ "$fpath" = "—" ]; then
      count=$((count + 1))
      continue
    fi

    # Strip backticks if present
    fpath="${fpath#\`}"
    fpath="${fpath%\`}"

    local full_path="$QA_DIR/$fpath"
    if [ -f "$full_path" ]; then
      log_ok "$fpath exists"
    else
      log_error "Path referenced but does not exist: $fpath"
    fi
    count=$((count + 1))
  done <<< "$rows"

  echo "  Checked $count rows."
}

# ---------- Check 2: RESOLVED rows have TUI path + artifact ----------

check_resolved_rows() {
  echo ""
  echo "=== Check 2: RESOLVED rows have TUI scenario path + capture artifact ==="
  local rows
  rows=$(extract_rows)
  local resolved=0

  while IFS= read -r row; do
    [ -z "$row" ] && continue
    local status
    status=$(get_column "$row" 7)

    if [ "$status" != "RESOLVED" ]; then
      continue
    fi
    resolved=$((resolved + 1))

    local fpath artifact
    fpath=$(get_column "$row" 4)
    artifact=$(get_column "$row" 5)

    # Strip backticks
    fpath="${fpath#\`}"
    fpath="${fpath%\`}"
    artifact="${artifact#\`}"
    artifact="${artifact%\`}"

    if [ -z "$fpath" ] || [ "$fpath" = "—" ]; then
      log_error "RESOLVED row has no TUI scenario path: $(get_column "$row" 2) $(get_column "$row" 3)"
    else
      log_ok "RESOLVED row has TUI path: $fpath"
    fi

    if [ -z "$artifact" ] || [ "$artifact" = "—" ]; then
      log_error "RESOLVED row has no capture artifact: $(get_column "$row" 2)"
    else
      log_ok "RESOLVED row has artifact: $artifact"
    fi
  done <<< "$rows"

  if [ "$resolved" -eq 0 ]; then
    echo "  No RESOLVED rows (expected at this stage)."
  else
    echo "  Checked $resolved RESOLVED rows."
  fi
}

# ---------- Check 3: No go test in evidence columns ----------

check_no_go_test() {
  echo ""
  echo "=== Check 3: No 'go test' in evidence columns ==="
  local rows
  rows=$(extract_rows)

  while IFS= read -r row; do
    [ -z "$row" ] && continue
    local id
    id=$(get_column "$row" 2)

    # Check columns 4-6 (TUI Scenario Path, Capture Artifact, Secondary Evidence)
    for col in 4 5 6; do
      local val
      val=$(get_column "$row" "$col")
      if echo "$val" | grep -qi 'go test'; then
        log_error "$id: Column $col contains 'go test': $val"
      fi
    done
  done <<< "$rows"

  log_ok "No 'go test' found in evidence columns."
}

# ---------- Check 4: No stale paths ----------

check_no_stale_paths() {
  echo ""
  echo "=== Check 4: No stale path patterns ==="
  local rows
  rows=$(extract_rows)
  local stale_found=0

  for pattern in "${STALE_PATTERNS[@]}"; do
    # Check if the pattern appears in any TUI Scenario Path column
    while IFS= read -r row; do
      [ -z "$row" ] && continue
      local fpath
      fpath=$(get_column "$row" 4)
      fpath="${fpath#\`}"
      fpath="${fpath%\`}"

      if echo "$fpath" | grep -q "$pattern"; then
        # Check if the file actually exists
        local full_path="$QA_DIR/$fpath"
        if [ -f "$full_path" ]; then
          log_ok "Stale pattern '$pattern' but file exists: $fpath"
        else
          log_error "Stale path referenced but file does not exist: $fpath"
          stale_found=$((stale_found + 1))
        fi
      fi
    done <<< "$rows"

    # Also check secondary evidence
    while IFS= read -r row; do
      [ -z "$row" ] && continue
      local secondary
      secondary=$(get_column "$row" 6)

      if echo "$secondary" | grep -q "$pattern"; then
        log_error "Stale pattern '$pattern' found in secondary evidence: $secondary"
        stale_found=$((stale_found + 1))
      fi
    done <<< "$rows"
  done

  if [ "$stale_found" -eq 0 ]; then
    log_ok "No stale path patterns found."
  fi
}

# ---------- Check 5: Every non-empty path is an actual test file ----------

check_paths_are_test_files() {
  echo ""
  echo "=== Check 5: Paths point to test-*.sh files ==="
  local rows
  rows=$(extract_rows)

  while IFS= read -r row; do
    [ -z "$row" ] && continue
    local fpath
    fpath=$(get_column "$row" 4)
    fpath="${fpath#\`}"
    fpath="${fpath%\`}"

    if [ -z "$fpath" ] || [ "$fpath" = "—" ]; then
      continue
    fi

    local basename
    basename=$(basename "$fpath")
    if [[ "$basename" != test-* ]]; then
      log_error "Path is not a test-*.sh file: $fpath"
    else
      log_ok "$fpath is a test file"
    fi
  done <<< "$rows"
}

# ---------- Run all checks ----------

main() {
  echo "Validating GAP_MATRIX.md..."
  echo "Matrix file: $MATRIX_FILE"
  echo "QA dir: $QA_DIR"

  if [ ! -f "$MATRIX_FILE" ]; then
    echo "FAIL: Matrix file not found: $MATRIX_FILE" >&2
    exit 1
  fi

  check_paths_exist
  check_resolved_rows
  check_no_go_test
  check_no_stale_paths
  check_paths_are_test_files

  echo ""
  echo "========================================"
  if [ "$errors" -eq 0 ]; then
    echo "RESULT: ALL CHECKS PASSED (0 errors)"
    exit 0
  else
    echo "RESULT: $errors ERROR(S) FOUND"
    exit 1
  fi
}

main "$@"
