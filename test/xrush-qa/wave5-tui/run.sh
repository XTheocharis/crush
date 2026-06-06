#!/usr/bin/env bash
# Run all wave5-tui tests sequentially (bash integration + Go unit tests).
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

echo "--- Running Go unit tests ---"
if go test ./test/xrush-qa/wave5-tui/ -count=1 -v; then
  PASS=$((PASS + 1))
else
  FAIL=$((FAIL + 1))
fi

echo ""
echo "Wave 5 Summary: $PASS passed, $FAIL failed"
exit "$FAIL"
