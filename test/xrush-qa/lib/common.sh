#!/usr/bin/env bash
# Shared helper functions for xrush-qa test suite.
set -euo pipefail

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
    ((elapsed++))
  done
}

# TMUX_SESSION and TMUX_PANE are set by start_crush, used by other helpers.
# Default tmux session name for QA.
: "${TMUX_SESSION:=qa-crush}"

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

  # Generate wave config to project crush.json (backup existing)
  if [[ -f "$PROJECT_DIR/crush.json" ]]; then
    cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
  fi
  bash "$QA_DIR/lib/provider-setup.sh" "$wave" "$PROJECT_DIR/crush.json" >/dev/null

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
# Polls tmux capture-pane for Processing/Thinking spinner to disappear.
# Usage: wait_for_idle [timeout_seconds]
wait_for_idle() {
  local timeout="${1:-120}"
  local elapsed=0
  while tmux capture-pane -t "$TMUX_SESSION" -p -S -100 | grep -qi "Processing\|Thinking\|Working"; do
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

# Capture current tmux pane output as evidence.
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

# Record test result to the reports file.
# Usage: finish_test test_name status
finish_test() {
  local test_name="$1"
  local status="$2"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  echo "$ts $test_name $status" >> "$QA_DIR/reports/results.txt"
}
