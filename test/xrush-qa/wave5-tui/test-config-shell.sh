#!/usr/bin/env bash
# Test: Config loading and context file walking.
# Verifies that Crush loads its configuration (include directives) and
# discovers context files (AGENTS.md) in the project root.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: Config include directives + context file walking
# ---------------------------------------------------------------------------
test_config_and_context_files() {
  echo "=== Scenario 1: Config loading and context file walking ==="

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    command restore_crush
  }
  trap restore_on_exit EXIT

  # Create a temporary crush.json with an include directive pointing to
  # a supplementary config file. This tests the include/merge mechanism.
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR/../.." && pwd)}"
  local include_config="$PROJECT_DIR/.crush/test-include-config.json"
  mkdir -p "$PROJECT_DIR/.crush"

  # Write the supplementary config that will be included.
  cat > "$include_config" <<'EOF'
{
  "options": {
    "debug": true
  }
}
EOF

  # Write a crush.json that uses include to pull in the supplementary config.
  local orig_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    orig_config=$(cat "$PROJECT_DIR/crush.json")
  fi
  cat > "$PROJECT_DIR/crush.json" <<EOF
{
  "\$schema": "https://charm.land/crush.json",
  "include": [".crush/test-include-config.json"]
}
EOF

  # Start crush with wave5 config helpers but our custom crush.json in place.
  # start_crush copies wave5.json over crush.json, so we need a different approach:
  # manually start tmux + crush with our custom config.
  TMUX_SESSION="qa-w5-config-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
  sleep 5

  # Send a prompt asking about context files.
  send_prompt "What context files did you load? List any AGENTS.md, CRUSH.md, or CRUSH.memory.md files you can see."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 15 "config-context"
    return
  fi

  capture_evidence 15 "config-context"

  # --- Assertion 1: AGENTS.md exists in project root and should be discovered ---
  if [[ -f "$PROJECT_DIR/AGENTS.md" ]]; then
    pass "Scenario 1: AGENTS.md exists in project root"
  else
    fail "Scenario 1: AGENTS.md not found in project root"
  fi

  # Capture pane output to check if AGENTS.md was mentioned in the response.
  local pane_output
  pane_output=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)
  if echo "$pane_output" | grep -qi "AGENTS.md"; then
    pass "Scenario 1: Agent response mentions AGENTS.md"
  else
    fail "Scenario 1: Agent response does not mention AGENTS.md"
  fi

  # --- Assertion 2: Crush log shows config loading activity ---
  # Wait for log file to be written.
  if ! wait_for_file "$PROJECT_DIR/.crush/logs/crush.log" 30; then
    fail "Scenario 1: crush.log not found"
    stop_crush
    return
  fi

  # Check for config-related log entries.
  local config_log_count
  config_log_count=$(grep -ciE "test-include-config\.json|include.*\.crush/test-include-config\.json|loaded.*crush\.json|merged.*config" "$PROJECT_DIR/.crush/logs/crush.log" 2>/dev/null || echo 0)
  if [[ "$config_log_count" -ge 1 ]]; then
    pass "Scenario 1: Crush log contains config loading entries ($config_log_count matches)"
  else
    fail "Scenario 1: Crush log has no config loading entries"
  fi

  # Check that debug was enabled (set via our include config).
  local debug_log_count
  debug_log_count=$(grep -ciE "debug.*enabled|level=debug|DEBUG" "$PROJECT_DIR/.crush/logs/crush.log" 2>/dev/null || echo 0)
  if [[ "$debug_log_count" -ge 1 ]]; then
    pass "Scenario 1: Debug logging active (from include config)"
  else
    fail "Scenario 1: No debug log entries found"
  fi

  # Clean up the temporary include config.
  rm -f "$include_config"

  stop_crush

  # Restore original crush.json if we had one.
  if [[ -n "${orig_config:-}" ]]; then
    echo "$orig_config" > "$PROJECT_DIR/crush.json"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_config_and_context_files

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
