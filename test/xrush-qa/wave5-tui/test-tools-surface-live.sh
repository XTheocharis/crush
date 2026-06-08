#!/usr/bin/env bash
# Test: Tools surface — deterministic tool selection via mock LLM server.
# Delegates to Go tests in internal/agent/tool_surface_mock_test.go which use
# MockServer for deterministic responses instead of live LLM calls.
#
# Scenarios covered by Go tests:
#   1. Surface includes edit tool → mock LLM returns edit tool_call
#   2. No LSP → code-intelligence tools hidden from surface
#   3. Planning phase → edit tools filtered from surface
#   4. Surface descriptions include visible tools, exclude hidden ones
#   5. Mock server returns consistent tool_calls across different prompts
set -euo pipefail

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

echo "=== Tools surface mock tests (delegating to Go) ==="
echo ""

# Run the Go test suite.
if go test ./internal/agent/ \
  -run "TestToolSurfaceInfluencesToolSelection|TestSurfaceDescriptionContainsVisibleTools|TestMockServerDeterministicToolSelection" \
  -v -count=1; then
  pass "All 3 Go tool surface mock tests passed"
else
  fail "Go tool surface mock tests failed"
fi

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
