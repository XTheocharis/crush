#!/usr/bin/env bash
# Shared helper functions for xrush-qa test suite.
set -euo pipefail

# Source global config helper for provider resolution.
# shellcheck source=global-config.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/global-config.sh"

# Backup existing .crush/ directory and create a fresh one.
setup_clean_crush() {
  if [[ -d .crush ]]; then
    local backup=".crush.bak.$(date +%s)"
    mv .crush "$backup"
  fi
  mkdir -p .crush/logs
}

# Restore original .crush/ from the most recent backup.
restore_crush() {
  local backup
  backup=$(find . -maxdepth 1 -name '.crush.bak.*' -type d | sort -t. -k4 -n | tail -1)
  if [[ -n "$backup" ]]; then
    rm -rf .crush
    mv "$backup" .crush
  fi
}

# Get the most recent session ID from the database.
get_session_id() {
  sqlite3 .crush/crush.db "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1"
}

# Run a SQL query against the crush database and return JSON output.
query_db() {
  sqlite3 -json .crush/crush.db "$@"
}

# Assert that a table has an expected row count (optionally filtered).
# Usage: assert_db_count table expected_count [where_clause]
assert_db_count() {
  local table="$1"
  local expected="$2"
  local where="${3:-}"
  local sql="SELECT COUNT(*) FROM $table"
  [[ -n "$where" ]] && sql="$sql WHERE $where"
  local actual
  actual=$(sqlite3 .crush/crush.db "$sql")
  if [[ "$actual" -ne "$expected" ]]; then
    echo "FAIL: assert_db_count: expected $expected rows in $table${where:+ WHERE $where}, got $actual" >&2
    return 1
  fi
}

# Assert that the crush log contains a pattern at least min_count times.
# Usage: assert_log_contains pattern [min_count]
assert_log_contains() {
  local pattern="$1"
  local min_count="${2:-1}"
  local count
  count=$(grep -c "$pattern" .crush/logs/crush.log 2>/dev/null || echo 0)
  if [[ "$count" -lt "$min_count" ]]; then
    echo "FAIL: assert_log_contains: expected at least $min_count matches for '$pattern', got $count" >&2
    return 1
  fi
}

# Poll for a file to exist, with timeout.
# Usage: wait_for_file filepath [timeout_seconds]
wait_for_file() {
  local filepath="$1"
  local timeout="${2:-30}"
  local elapsed=0
  while [[ ! -f "$filepath" ]]; do
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "FAIL: wait_for_file: '$filepath' not found within ${timeout}s" >&2
      return 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
}

# TMUX_SESSION and TMUX_PANE are set by start_crush, used by other helpers.
# Default tmux session name for QA.
: "${TMUX_SESSION:=qa-crush}"

# DEPRECATED: Use start_crush_tui() instead. This function is preserved for
# backward compatibility but will be removed in a future release.
# Start Crush in a tmux session with the specified wave config.
# Usage: start_crush <wave_number> [--session ID|--continue] [--cwd PATH]
# Copies test/xrush-qa/waveN.json to crush.json in project root, then launches crush --yolo.
start_crush() {
  local wave="${1:-1}"
  shift || true

  # Parse optional flags
  local session_flag=""
  local continue_flag=""
  local cwd_flag=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session) session_flag="--session $2"; shift 2 ;;
      --continue) continue_flag="--continue"; shift ;;
      --cwd) cwd_flag="--cwd $2"; shift 2 ;;
      *) shift ;;
    esac
  done

  QA_DIR="${QA_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR/../.." && pwd)}"
  TMUX_SESSION="qa-w${wave}-$(date +%s)"

  # Resolve global provider config and generate wave-specific project config.
  resolve_global_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
  fi
  generate_project_config "$wave" "$PROJECT_DIR/crush.json"

  # Create tmux session and launch crush
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50
  local launch_cmd="crush --yolo"
  [[ -n "$session_flag" ]] && launch_cmd="$launch_cmd $session_flag"
  [[ -n "$continue_flag" ]] && launch_cmd="$launch_cmd $continue_flag"
  [[ -n "$cwd_flag" ]] && launch_cmd="$launch_cmd $cwd_flag"

  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && $launch_cmd" Enter
  # Wait for TUI to initialize
  sleep 5
}

# Send a text prompt to the running Crush TUI instance.
# Usage: send_prompt <text>
send_prompt() {
  local text="$1"
  tmux send-keys -t "$TMUX_SESSION" "$text" Enter
}

# Wait for Crush to finish processing (idle detection).
# Polls DB for assistant message with finished_at > 0, falls back to log check.
# Usage: wait_for_idle [timeout_seconds]
wait_for_idle() {
  local timeout="${1:-120}"
  local elapsed=0
  local db_path=".crush/crush.db"
  local log_path=".crush/logs/crush.log"

  while true; do
    local db_idle=false
    local log_idle=false

    if [[ -f "$db_path" ]]; then
      local session_id
      session_id=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || true)
      if [[ -n "$session_id" ]]; then
        local msg_count
        msg_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM messages WHERE role='assistant' AND session_id='$session_id' AND finished_at IS NOT NULL AND finished_at > 0" 2>/dev/null || echo 0)
        if [[ "$msg_count" -gt 0 ]]; then
          db_idle=true
        fi
      fi
    fi

    if [[ -f "$log_path" ]]; then
      if grep -q '"Skill turn summary"\|"Plan step marked as completed"' "$log_path" 2>/dev/null; then
        log_idle=true
      fi
    fi

    if $db_idle || $log_idle; then
      return 0
    fi

    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "FAIL: wait_for_idle: Crush still busy after ${timeout}s" >&2
      return 1
    fi

    sleep 2
    ((elapsed += 2))
  done
}

# Stop the running Crush TUI instance gracefully.
# Usage: stop_crush
stop_crush() {
  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux send-keys -t "$TMUX_SESSION" y
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  # Restore original crush.json if backed up
  local backup
  backup=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f | sort -t. -k5 -n | tail -1)
  if [[ -n "$backup" ]]; then
    mv "$backup" crush.json
  fi
}

# Gracefully tear down the tmux session with existence checks and retries.
# Usage: cleanup_tui
cleanup_tui() {
  if ! tmux has-session -t "$TMUX_SESSION" 2>/dev/null; then
    return 0
  fi

  tmux send-keys -t "$TMUX_SESSION" C-c
  sleep 0.5
  tmux send-keys -t "$TMUX_SESSION" y
  sleep 1
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  local retries=0
  while tmux has-session -t "$TMUX_SESSION" 2>/dev/null; do
    if [[ "$retries" -ge 3 ]]; then
      tmux kill-server 2>/dev/null || true
      break
    fi
    sleep 1
    retries=$((retries + 1))
  done

  local backup
  backup=$(find . -maxdepth 1 -name 'crush.json.bak.*' -type f | sort -t. -k5 -n | tail -1)
  if [[ -n "$backup" ]]; then
    mv "$backup" crush.json
  fi
}

# --- TUI-only tmux launcher contract ---

# Remove ANSI escape codes from stdin or argument string.
# Usage: strip_ansi [string]  OR  echo "$text" | strip_ansi
strip_ansi() {
  if [[ $# -gt 0 ]]; then
    printf '%s' "$1" | sed 's/\x1b\[[0-9;]*[a-zA-Z]//g'
  else
    sed 's/\x1b\[[0-9;]*[a-zA-Z]//g'
  fi
}

# Start Crush TUI in a tmux session at 160x50 dimensions.
# Usage: start_crush_tui <wave_number> [--session ID|--continue] [--cwd PATH]
start_crush_tui() {
  local wave="${1:-1}"
  shift || true

  local session_flag=""
  local continue_flag=""
  local cwd_flag=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --session) session_flag="--session $2"; shift 2 ;;
      --continue) continue_flag="--continue"; shift ;;
      --cwd) cwd_flag="--cwd $2"; shift 2 ;;
      *) shift ;;
    esac
  done

  QA_DIR="${QA_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
  PROJECT_DIR="${PROJECT_DIR:-$(cd "$QA_DIR/../.." && pwd)}"
  TMUX_SESSION="qa-w${wave}-$(date +%s)"

  resolve_global_config
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
  fi
  generate_project_config "$wave" "$PROJECT_DIR/crush.json"

  tmux new-session -d -s "$TMUX_SESSION" -x 160 -y 50
  local launch_cmd="crush --yolo"
  [[ -n "$session_flag" ]] && launch_cmd="$launch_cmd $session_flag"
  [[ -n "$continue_flag" ]] && launch_cmd="$launch_cmd $continue_flag"
  [[ -n "$cwd_flag" ]] && launch_cmd="$launch_cmd $cwd_flag"

  tmux send-keys -t "$TMUX_SESSION" "cd $PROJECT_DIR && $launch_cmd" Enter

  _QA_FOCUS_STATE="editor"

  local waited=0
  while [[ $waited -lt 30 ]]; do
    local pane_content
    pane_content=$(tmux capture-pane -t "$TMUX_SESSION" -p 2>/dev/null || true)
    # Wait for Crush TUI to render its actual UI (not just the typed command echo).
    # "ctrl+c quit" only appears in the TUI footer, never in the typed command.
    # "HEY!" only appears in the TUI splash screen.
    if printf '%s' "$pane_content" | grep -qi 'ctrl+c quit\|HEY!\|Charm.*v[0-9]'; then
      break
    fi
    sleep 1
    waited=$((waited + 1))
  done
}

# Send a text prompt to the running Crush TUI via tmux literal mode.
# Usage: send_tui_prompt <text>
send_tui_prompt() {
  local text="$1"
  tmux send-keys -t "$TMUX_SESSION" -l "$text"
  sleep 0.1
  tmux send-keys -t "$TMUX_SESSION" Enter
}

# Wait for Crush TUI to become idle using composite polling.
# Polls DB for assistant message with finished_at > 0.
# Polls log for turn-end event.
# Usage: wait_for_tui_idle [timeout_seconds]
wait_for_tui_idle() {
  local timeout="${1:-120}"
  local elapsed=0
  local db_path=".crush/crush.db"
  local log_path=".crush/logs/crush.log"

  while true; do
    local db_idle=false
    local log_idle=false

    if [[ -f "$db_path" ]]; then
      local session_id
      session_id=$(sqlite3 "$db_path" "SELECT id FROM sessions ORDER BY created_at DESC LIMIT 1" 2>/dev/null || true)
      if [[ -n "$session_id" ]]; then
        local msg_count
        msg_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM messages WHERE role='assistant' AND session_id='$session_id' AND finished_at IS NOT NULL AND finished_at > 0" 2>/dev/null || echo 0)
        if [[ "$msg_count" -gt 0 ]]; then
          db_idle=true
        fi
      fi
    fi

    if [[ -f "$log_path" ]]; then
      if grep -q '"Skill turn summary"\|"Plan step marked as completed"' "$log_path" 2>/dev/null; then
        log_idle=true
      fi
    fi

    if $db_idle || $log_idle; then
      return 0
    fi

    if [[ "$elapsed" -ge "$timeout" ]]; then
      tmux capture-pane -t "$TMUX_SESSION" -p -S -1000 > /tmp/tmux-idle-timeout-"$TMUX_SESSION".txt 2>/dev/null || true
      echo "FAIL: wait_for_tui_idle: Crush still busy after ${timeout}s" >&2
      return 1
    fi

    sleep 2
    ((elapsed += 2))
  done
}

# Capture current tmux pane output (last 1000 lines).
# Usage: capture_tui
capture_tui() {
  tmux capture-pane -t "$TMUX_SESSION" -p -S -1000
}

# Assert captured TUI output contains a string (after stripping ANSI).
# Usage: assert_tui_contains <expected_string>
assert_tui_contains() {
  local expected="$1"
  local output
  output=$(capture_tui | strip_ansi)
  if ! printf '%s' "$output" | grep -qF "$expected"; then
    echo "FAIL: assert_tui_contains: expected '$expected' not found in TUI output" >&2
    return 1
  fi
}

# Assert captured TUI output does NOT contain a string (after stripping ANSI).
# Usage: assert_tui_not_contains <forbidden_string>
assert_tui_not_contains() {
  local forbidden="$1"
  local output
  output=$(capture_tui | strip_ansi)
  if printf '%s' "$output" | grep -qF "$forbidden"; then
    echo "FAIL: assert_tui_not_contains: forbidden '$forbidden' found in TUI output" >&2
    return 1
  fi
}

# Save TUI evidence to test/xrush-qa/reports/<wave>/<scenario>/.
# Uses WAVE and SCENARIO env vars set by the test harness.
# Usage: capture_tui_evidence <label>
capture_tui_evidence() {
  local label="$1"
  local wave="${WAVE:-unknown}"
  local scenario="${SCENARIO:-default}"
  QA_DIR="${QA_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
  local evidence_dir="$QA_DIR/reports/$wave/$scenario"
  mkdir -p "$evidence_dir"
  capture_tui | mask_secret > "$evidence_dir/${label}.txt"
}

# Reset the tracked focus state to editor (Crush starts with editor focused).
# Usage: reset_focus_state
reset_focus_state() {
  _QA_FOCUS_STATE="editor"
}

# Send Tab key to switch focus to chat list pane.
# Tracks focus state to avoid redundant Tab presses.
# Usage: focus_chat
focus_chat() {
  if [[ "${_QA_FOCUS_STATE:-editor}" == "chat" ]]; then
    return 0
  fi
  tmux send-keys -t "$TMUX_SESSION" Tab
  _QA_FOCUS_STATE="chat"
}

# Send Tab key to switch focus to editor pane.
# Tracks focus state to avoid redundant Tab presses.
# Usage: focus_editor
focus_editor() {
  if [[ "${_QA_FOCUS_STATE:-editor}" == "editor" ]]; then
    return 0
  fi
  tmux send-keys -t "$TMUX_SESSION" Tab
  _QA_FOCUS_STATE="editor"
}

# Select a message by offset from the bottom of the chat list.
# Sends G (go to bottom) then k N times to move up.
# Usage: select_message_by_offset <offset_from_bottom>
select_message_by_offset() {
  local offset="${1:-0}"
  tmux send-keys -t "$TMUX_SESSION" G
  sleep 0.2
  local i
  for ((i = 0; i < offset; i++)); do
    tmux send-keys -t "$TMUX_SESSION" k
    sleep 0.05
  done
}

# --- End TUI-only tmux launcher contract ---

# DEPRECATED: Legacy evidence capture using .sisyphus/evidence path.
# Use capture_tui_evidence() instead.
# Usage: capture_evidence <task_num> <scenario_name>
capture_evidence() {
  local task="$1"
  local scenario="$2"
  local evidence_dir="${EVIDENCE_DIR:-.sisyphus/evidence}"
  mkdir -p "$evidence_dir"
  tmux capture-pane -t "$TMUX_SESSION" -p -S -1000 > "$evidence_dir/task-${task}-${scenario}.txt"
}

# Get the most recent session ID (alias for get_session_id).
get_latest_session() {
  get_session_id
}

# Dump all relevant tables for a session as JSON to stdout.
# Usage: dump_session_state session_id
dump_session_state() {
  local session_id="$1"
  local tables
  tables=$(list_tables)
  echo "{"
  local first=true
  while IFS= read -r table; do
    # Only dump tables that have a session_id column.
    local has_col
    has_col=$(sqlite3 .crush/crush.db "PRAGMA table_info($table)" | grep -c "session_id" || echo 0)
    if [[ "$has_col" -gt 0 ]]; then
      if [[ "$first" == "true" ]]; then
        first=false
      else
        echo ","
      fi
      echo "\"$table\":"
      sqlite3 -json .crush/crush.db "SELECT * FROM $table WHERE session_id = '$session_id'"
    fi
  done <<< "$tables"
  echo "}"
}

# Initialize test environment: backup .crush, set paths.
init_test() {
  setup_clean_crush
  SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
  QA_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
}

# Mask potential secrets in a string before writing to evidence files.
# Replaces API-key-like patterns with REDACTED placeholders.
# Usage: masked_output=$(echo "$output" | mask_secret)
mask_secret() {
  sed \
    -e 's/sk-[a-zA-Z0-9]\{20,\}/REDACTED_SK/g' \
    -e 's/api[_-]\?key[[:space:]]*[:=][[:space:]]*"[a-zA-Z0-9]\{10,\}"/api_key: "REDACTED"/gi' \
    -e 's/api[_-]\?key[[:space:]]*[:=][[:space:]]*[a-zA-Z0-9]\{10,\}/api_key: REDACTED/gi' \
    -e 's/\(api_key\|apiKey\|API_KEY\)[[:space:]]*[:=][[:space:]]*"[^"]\{10,\}"/\1: "REDACTED"/g' \
    -e 's/token[[:space:]]*[:=][[:space:]]*"[a-zA-Z0-9]\{10,\}"/token: "REDACTED"/gi' \
    -e 's/bearer[[:space:]]\+[a-zA-Z0-9._-]\{20,\}/Bearer REDACTED/gi'
}

# Record test result to the reports file.
# Usage: finish_test test_name status
finish_test() {
  local test_name="$1"
  local status="$2"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  echo "$ts $test_name $status" >> "$QA_DIR/reports/results.txt"
}
