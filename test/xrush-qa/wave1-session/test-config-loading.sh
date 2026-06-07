#!/usr/bin/env bash
# Test: Config loading from global provider config (TUI-first approach).
# Verifies that Crush loads provider credentials from the user-scoped config
# path, exposes a non-secret provider/model identifier in TUI output, and
# never leaks API keys or secret patterns to the terminal.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
source "$QA_DIR/lib/common.sh"

PASS=0
FAIL=0
pass() { echo "PASS: $1"; ((PASS += 1)); }
fail() { echo "FAIL: $1" >&2; ((FAIL += 1)); }

# ---------------------------------------------------------------------------
# Scenario 1: TUI reports provider/model without leaking secrets
# ---------------------------------------------------------------------------
test_config_provider_visible() {
  echo "=== Scenario 1: TUI shows provider/model identifier, no secrets ==="
  WAVE=1
  SCENARIO="config-loading"

  setup_clean_crush
  cleanup_test() {
    cleanup_tui
    restore_crush
  }
  trap cleanup_test EXIT

  start_crush_tui 1
  focus_editor

  # Deterministic sentinel: ask model to report ONLY its provider name.
  send_tui_prompt "What provider and model are you using? Reply with exactly CONFIG_SENTINEL followed by the provider name in uppercase, like CONFIG_SENTINEL_ANTHROPIC. Do not include any API key or secret."

  if ! wait_for_tui_idle 180; then
    fail "Scenario 1: Crush did not become idle"
    capture_tui_evidence "idle-timeout"
    tmux send-keys -t "$TMUX_SESSION" C-c
    return
  fi

  # --- Primary gate: TUI must contain the CONFIG_SENTINEL prefix ---
  if assert_tui_contains "CONFIG_SENTINEL"; then
    pass "Scenario 1: TUI shows CONFIG_SENTINEL identifier"
  else
    fail "Scenario 1: TUI does not show CONFIG_SENTINEL identifier"
    capture_tui_evidence "sentinel-missing"
    return
  fi

  capture_tui_evidence "tui-provider-response"

  # --- Secret safety: TUI must NOT contain API key patterns ---
  local secret_patterns=(
    "sk-"
    "api_key"
    "apikey"
    "bearer "
    "Authorization:"
  )

  local leaked=0
  for pattern in "${secret_patterns[@]}"; do
    if assert_tui_not_contains "$pattern"; then
      :
    else
      fail "Scenario 1: TUI output contains forbidden pattern: '$pattern'"
      leaked=1
    fi
  done

  if [[ "$leaked" -eq 0 ]]; then
    pass "Scenario 1: No secret patterns found in TUI output"
  fi

  # --- Evidence masking: check captured evidence file for secrets ---
  local evidence_file
  QA_DIR_RESOLVED="${QA_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
  evidence_file="$QA_DIR_RESOLVED/reports/$WAVE/$SCENARIO/tui-provider-response.txt"

  if [[ -f "$evidence_file" ]]; then
    local hex_leak
    # Check for long hex strings (32+ contiguous hex chars, typical of API keys).
    hex_leak=$(grep -oP '[0-9a-fA-F]{32,}' "$evidence_file" 2>/dev/null || true)
    if [[ -z "$hex_leak" ]]; then
      pass "Scenario 1: Evidence file contains no long hex strings"
    else
      fail "Scenario 1: Evidence file contains possible secret hex string"
      leaked=1
    fi
  else
    fail "Scenario 1: Evidence file not found for masking check"
  fi

  # --- Secondary: log/config checks proving provider loaded from global config ---
  local log_path=".crush/logs/crush.log"

  if [[ -f "$log_path" ]]; then
    # Check log mentions provider loading or model resolution.
    local provider_loaded
    provider_loaded=$(grep -ci 'provider\|model\|config' "$log_path" 2>/dev/null ) || provider_loaded=0
    if [[ "$provider_loaded" -gt 0 ]]; then
      pass "Scenario 1: Log file shows provider/model/config references ($provider_loaded)"
    else
      fail "Scenario 1: Log file has no provider/model/config references"
    fi

    # Verify no API key in log file either.
    local log_key_leak
    log_key_leak=$(grep -cE 'sk-|api_key|apikey' "$log_path" 2>/dev/null) || log_key_leak=0
    if [[ "$log_key_leak" -eq 0 ]]; then
      pass "Scenario 1: Log file contains no API key patterns"
    else
      fail "Scenario 1: Log file contains API key pattern ($log_key_leak matches)"
    fi
  else
    fail "Scenario 1: Log file not found at $log_path"
  fi

  # Verify CRUSH_GLOBAL_CONFIG was set (if using non-default location).
  if [[ -n "${CRUSH_GLOBAL_CONFIG:-}" ]]; then
    pass "Scenario 1: CRUSH_GLOBAL_CONFIG is set to '$CRUSH_GLOBAL_CONFIG'"
  else
    # Using default location — verify the default config exists.
    if [[ -f "$HOME/.config/crush/crush.json" ]]; then
      pass "Scenario 1: Using default global config at ~/.config/crush/crush.json"
    else
      fail "Scenario 1: CRUSH_GLOBAL_CONFIG not set and default config not found"
    fi
  fi

  # Verify CRUSH_QA_GLOBAL_CONFIG_FILE points to an existing file.
  if [[ -f "${CRUSH_QA_GLOBAL_CONFIG_FILE:-}" ]]; then
    pass "Scenario 1: Global config file exists at $CRUSH_QA_GLOBAL_CONFIG_FILE"
  else
    fail "Scenario 1: Global config file not found"
  fi

  # Verify project config has no providers block.
  local project_config
  project_config="${PROJECT_DIR:-$(cd "$QA_DIR/../.." && pwd)}/crush.json"
  if [[ -f "$project_config" ]]; then
    local has_providers
    has_providers=$(python3 -c "
import json, sys
with open('$project_config') as f:
    cfg = json.load(f)
print('yes' if 'providers' in cfg else 'no')
" 2>/dev/null || echo "error")
    if [[ "$has_providers" == "no" ]]; then
      pass "Scenario 1: Project config has no providers block (uses global)"
    else
      fail "Scenario 1: Project config unexpectedly has providers block"
    fi
  else
    fail "Scenario 1: Project config file not found at $project_config"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
test_config_provider_visible

echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
