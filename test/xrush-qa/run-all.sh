#!/usr/bin/env bash
set -euo pipefail

QA_DIR=$(cd "$(dirname "$0")" && pwd)
source "$QA_DIR/lib/common.sh"
source "$QA_DIR/lib/db-schema.sh"

# Cleanup: kill any tmux sessions created by the QA suite on exit.
cleanup() {
  local sessions
  sessions=$(tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^qa-' || true)
  for s in $sessions; do
    tmux kill-session -t "$s" 2>/dev/null || true
  done
}
trap cleanup EXIT

WAVE=""
CLEAN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --wave)
      WAVE="$2"
      shift 2
      ;;
    --clean)
      CLEAN=true
      shift
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if $CLEAN; then
  rm -rf .crush
fi

mkdir -p "$QA_DIR/reports/bugs"
: > "$QA_DIR/reports/results.txt"

WAVES=(wave1-session wave2-repomap wave3-lcm wave4-orchestration wave5-tui)
FAILURES=0

run_wave() {
  local wave="$1"
  local wave_dir="$QA_DIR/$wave"
  local run_script="$wave_dir/run.sh"
  if [[ -f "$run_script" ]]; then
    echo "=== Running $wave ==="
    if bash "$run_script"; then
      echo "=== $wave: PASS ==="
      echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $wave PASS" >> "$QA_DIR/reports/results.txt"
    else
      status=$?
      if [[ "$status" -eq 2 ]]; then
        echo "=== $wave: PROVIDER UNAVAILABLE (exit $status) ==="
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $wave PROVIDER_UNAVAILABLE" >> "$QA_DIR/reports/results.txt"
      else
        echo "=== $wave: FAIL (exit $status) ==="
        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $wave FAIL" >> "$QA_DIR/reports/results.txt"
      fi
      FAILURES=$((FAILURES + 1))
    fi
  else
    echo "=== Skipping $wave (no run.sh) ==="
  fi
}

if [[ -n "$WAVE" ]]; then
  wave_name="wave${WAVE}-*"
  matching=()
  for w in "${WAVES[@]}"; do
    if [[ "$w" == wave${WAVE}-* ]]; then
      matching+=("$w")
    fi
  done
  if [[ ${#matching[@]} -eq 0 ]]; then
    echo "ERROR: No wave matches 'wave${WAVE}-*'" >&2
    exit 1
  fi
  for w in "${matching[@]}"; do
    run_wave "$w"
  done
else
  for w in "${WAVES[@]}"; do
    run_wave "$w"
  done
fi

echo ""
echo "=== Results Summary ==="
if [[ -f "$QA_DIR/reports/results.txt" ]]; then
  cat "$QA_DIR/reports/results.txt"
  pass=$(grep -c 'PASS$' "$QA_DIR/reports/results.txt" ) || pass=0
  fail=$(grep -c 'FAIL$' "$QA_DIR/reports/results.txt" ) || fail=0
  echo ""
    echo "Passed: $pass  Failed: $fail"
else
  echo "No results recorded."
fi

exit "$FAILURES"
