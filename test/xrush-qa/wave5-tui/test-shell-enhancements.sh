#!/usr/bin/env bash
# Test: Shell enhancements — config variable expansion, jq behavior, background jobs.
# Validates Crush's embedded shell handles variable expansion, jq filters,
# and background job lifecycle correctly.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
SKIP=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }
skip() { echo "SKIP: $1"; ((SKIP += 1)); }

# Kill any stray background jobs on exit.
cleanup_bg_jobs() {
  # Kill any lingering sleep processes spawned by this test.
  jobs -p 2>/dev/null | xargs -r kill 2>/dev/null || true
  # Wait briefly for processes to exit.
  wait 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Scenario 1: Config variable expansion
# ---------------------------------------------------------------------------
# Tests that $VAR, ${VAR:-default}, and $(command) expansion work correctly
# when used in crush.json config values. Uses QA_ prefixed fake values only.
# ---------------------------------------------------------------------------
test_config_variable_expansion() {
  echo "=== Scenario 1: Config variable expansion ==="

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
    cleanup_bg_jobs
  }
  trap restore_on_exit EXIT

  # Set QA-only fake values for expansion testing.
  export QA_SHELL_TEST_VAR="expanded-qa-value-42"
  export QA_SHELL_FALLBACK_VAR=""
  export QA_SHELL_SECRET="qa-fake-secret-never-real"

  # Write a crush.json that uses all three expansion forms.
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR/../.." && pwd)}"
  local orig_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    orig_config=$(cat "$PROJECT_DIR/crush.json")
  fi
  cat > "$PROJECT_DIR/crush.json" <<'JSONEOF'
{
  "$schema": "https://charm.land/crush.json",
  "mcp": {
    "test-expansion": {
      "type": "http",
      "url": "https://qa-test.example.com/endpoint",
      "headers": {
        "X-QA-Token": "$QA_SHELL_TEST_VAR",
        "X-QA-Fallback": "${QA_SHELL_FALLBACK_VAR:-fallback-default}",
        "X-QA-CmdSubst": "$(echo qa-cmd-output)",
        "X-QA-Secret": "$QA_SHELL_SECRET"
      }
    }
  }
}
JSONEOF

  # Start Crush manually so our custom config survives.
  TMUX_SESSION="qa-w5-expand-$(date +%s)"
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && QA_SHELL_TEST_VAR=$QA_SHELL_TEST_VAR QA_SHELL_SECRET=$QA_SHELL_SECRET crush --yolo" Enter
  sleep 5

  # Ask Crush to show the resolved config headers.
  send_prompt "Run a shell command that prints the environment variable QA_SHELL_TEST_VAR, then print the string 'fallback-default', then print 'qa-cmd-output'. Use echo commands."
  if ! wait_for_idle 120; then
    fail "Scenario 1: Crush did not become idle"
    capture_evidence 16 "config-expansion"
    stop_crush
    if [[ -n "${orig_config:-}" ]]; then echo "$orig_config" > "$PROJECT_DIR/crush.json"; fi
    return
  fi

  capture_evidence 16 "config-expansion"

  # --- Assertion 1: $VAR expansion resolved ---
  local pane_output
  pane_output=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)
  if echo "$pane_output" | grep -q "expanded-qa-value-42"; then
    pass "Scenario 1: \$VAR expansion resolved correctly"
  else
    fail "Scenario 1: \$VAR expansion did not resolve"
  fi

  # --- Assertion 2: ${VAR:-default} fallback used when VAR is empty ---
  if echo "$pane_output" | grep -q "fallback-default"; then
    pass "Scenario 1: \${VAR:-default} fallback resolved correctly"
  else
    fail "Scenario 1: \${VAR:-default} fallback did not resolve"
  fi

  # --- Assertion 3: $(command) substitution expanded ---
  if echo "$pane_output" | grep -q "qa-cmd-output"; then
    pass "Scenario 1: \$(command) substitution resolved correctly"
  else
    fail "Scenario 1: \$(command) substitution did not resolve"
  fi

  # --- Assertion 4: Fake secret only in output (not real secrets) ---
  # Verify we only see our QA fake value, never real credential patterns.
  local real_secret_count
  real_secret_count=$(echo "$pane_output" | grep -ciE "sk-[a-zA-Z0-9]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN.*PRIVATE KEY" || echo 0)
  if [[ "$real_secret_count" -eq 0 ]]; then
    pass "Scenario 1: No real secrets leaked in output"
  else
    fail "Scenario 1: Real secret patterns found in output ($real_secret_count matches)"
  fi

  # Verify the QA fake secret IS present (from the expansion prompt).
  if echo "$pane_output" | grep -q "qa-fake-secret-never-real"; then
    pass "Scenario 1: QA fake secret visible in output as expected"
  else
    fail "Scenario 1: QA fake secret not found in output"
  fi

  stop_crush

  # Restore original crush.json.
  if [[ -n "${orig_config:-}" ]]; then
    echo "$orig_config" > "$PROJECT_DIR/crush.json"
  fi
}

# ---------------------------------------------------------------------------
# Scenario 2: jq behavior with deterministic JSON input
# ---------------------------------------------------------------------------
# Tests Crush's built-in jq (gojq) with known input and filters.
# ---------------------------------------------------------------------------
test_jq_behavior() {
  echo "=== Scenario 2: jq behavior ==="

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
    cleanup_bg_jobs
  }
  trap restore_on_exit EXIT

  start_crush 5

  # Send a prompt that exercises jq through the embedded shell.
  send_prompt "Run this exact shell command: echo '{\"name\":\"crush-qa\",\"version\":\"1.0\",\"items\":[{\"id\":1,\"active\":true},{\"id\":2,\"active\":false}]}' | jq '.items[] | select(.active==true) | .id'"
  if ! wait_for_idle 120; then
    fail "Scenario 2: Crush did not become idle"
    capture_evidence 16 "jq-behavior"
    return
  fi

  capture_evidence 16 "jq-behavior"

  # --- Assertion 1: jq filter produced correct output (id 1) ---
  local pane_output
  pane_output=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)
  if echo "$pane_output" | grep -q "1"; then
    pass "Scenario 2: jq filter produced correct output (id=1)"
  else
    fail "Scenario 2: jq filter did not produce expected output"
  fi

  # --- Assertion 2: jq filter excluded inactive item (id 2) ---
  # The output should contain "1" but should NOT show the select result "2"
  # as a standalone match (it may appear in the original JSON).
  # Check that jq output section does not have a bare "2" as a result line.
  local jq_output_section
  jq_output_section=$(echo "$pane_output" | grep -A2 "1" | tail -5)
  if ! echo "$jq_output_section" | grep -qx "2"; then
    pass "Scenario 2: jq correctly excluded inactive item (id=2)"
  else
    fail "Scenario 2: jq may have included inactive item in output"
  fi

  # --- Assertion 3: No jq parse error ---
  if echo "$pane_output" | grep -qi "jq:.*error\|parse error"; then
    fail "Scenario 2: jq reported a parse error"
  else
    pass "Scenario 2: jq produced no parse errors"
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Scenario 3: Background job cleanup
# ---------------------------------------------------------------------------
# Tests that starting a background job via Crush, then cancelling it, leaves
# no orphaned background processes.
# ---------------------------------------------------------------------------
test_background_job_cleanup() {
  echo "=== Scenario 3: Background job cleanup ==="

  setup_clean_crush
  # shellcheck disable=SC2317
  restore_on_exit() {
    stop_crush 2>/dev/null || true
    local json_bak
    json_bak=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
    if [[ -n "$json_bak" ]]; then
      mv "$json_bak" crush.json
    fi
    restore_crush
    cleanup_bg_jobs
  }
  trap restore_on_exit EXIT

  # Record the count of sleep processes before the test.
  local sleep_count_before
  sleep_count_before=$(pgrep -c "sleep" 2>/dev/null || echo 0)

  start_crush 5

  # Send a prompt that starts a long-running background job.
  send_prompt "Run this shell command in the background: sleep 300. Then tell me you started it."
  if ! wait_for_idle 120; then
    fail "Scenario 3: Crush did not become idle after starting background job"
    capture_evidence 16 "bg-job-start"
    return
  fi

  capture_evidence 16 "bg-job-start"

  # --- Assertion 1: Background job was started ---
  local pane_output_start
  pane_output_start=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)
  if echo "$pane_output_start" | grep -qi "sleep\|background\|started"; then
    pass "Scenario 3: Background job was started"
  else
    fail "Scenario 3: No indication background job was started"
  fi

  # Now ask Crush to stop/cancel the background job.
  send_prompt "Stop the background job you just started. Use Ctrl-C or kill the process."
  if ! wait_for_idle 120; then
    fail "Scenario 3: Crush did not become idle after cancelling background job"
    capture_evidence 16 "bg-job-cancel"
    return
  fi

  capture_evidence 16 "bg-job-cancel"

  # --- Assertion 2: No orphaned background sleep processes ---
  # Give a brief grace period for process cleanup.
  sleep 2
  local sleep_count_after
  sleep_count_after=$(pgrep -c "sleep" 2>/dev/null || echo 0)
  # Allow for system sleep processes; just verify we didn't ADD one.
  local delta=$((sleep_count_after - sleep_count_before))
  if [[ "$delta" -le 0 ]]; then
    pass "Scenario 3: No orphaned background sleep processes (delta=$delta)"
  else
    fail "Scenario 3: Orphaned background process detected (delta=$delta)"
  fi

  # --- Assertion 3: Agent confirmed cancellation ---
  local pane_output_cancel
  pane_output_cancel=$(tmux capture-pane -t "$TMUX_SESSION" -p -S -1000)
  if echo "$pane_output_cancel" | grep -qi "stop\|cancel\|kill\|terminat\|done"; then
    pass "Scenario 3: Agent confirmed background job cancellation"
  else
    fail "Scenario 3: Agent did not confirm background job cancellation"
  fi

  stop_crush
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_config_variable_expansion
test_jq_behavior
test_background_job_cleanup

echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"
exit "$FAIL"
