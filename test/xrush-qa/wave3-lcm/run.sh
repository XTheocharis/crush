#!/usr/bin/env bash
# Run all wave3-lcm tests sequentially.
set -euo pipefail

DIR=$(cd "$(dirname "$0")" && pwd)
PASS=0
FAIL=0

for test in "$DIR"/test-*.sh; do
  echo "--- Running $(basename "$test") ---"
  if bash "$test"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "Wave 3 Summary: $PASS passed, $FAIL failed"
exit "$FAIL"
