#!/usr/bin/env bash
# Static lint checks for xrush-qa test files.
# Catches regressions like `command restore_crush` (should be just `restore_crush`)
# and local `restore_crush()` definitions outside common.sh (should be `cleanup_test()`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QA_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RC=0

# Collect files to check.
if [[ $# -gt 0 ]]; then
  FILES=("$@")
else
  # Default: all .sh files under test/xrush-qa/ (excluding lib/).
  mapfile -t FILES < <(find "$QA_DIR" -name '*.sh' -not -path "$QA_DIR/lib/*" | sort)
fi

# Check 1: No file may contain `command restore_crush`.
for f in "${FILES[@]}"; do
  matches=$(grep -cn 'command restore_crush' "$f" 2>/dev/null || true)
  if [[ "$matches" -gt 0 ]]; then
    echo "ERROR: $f contains 'command restore_crush' ($matches occurrences)" >&2
    grep -n 'command restore_crush' "$f" >&2
    RC=1
  fi
done

# Check 2: No file outside lib/common.sh may define a `restore_crush()` function.
for f in "${FILES[@]}"; do
  # Skip common.sh — the canonical definition lives there.
  if [[ "$(basename "$f")" == "common.sh" ]]; then
    continue
  fi
  matches=$(grep -cE 'restore_crush\(\)' "$f" 2>/dev/null || true)
  if [[ "$matches" -gt 0 ]]; then
    echo "WARNING: $f defines restore_crush() — should be cleanup_test()" >&2
    grep -nE 'restore_crush\(\)' "$f" >&2
    # Warning only, not a hard failure.
  fi
done

if [[ $RC -eq 0 ]]; then
  echo "lint-checks: all ${#FILES[@]} file(s) passed"
fi
exit $RC
