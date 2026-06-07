#!/usr/bin/env bash
set -euo pipefail

# Fuzzy LLM assertion helpers. LLM output is nondeterministic — sentinels
# may appear with slight variations. These wrappers add retry logic and
# partial matching to reduce flaky failures.

# Retry a TUI assertion up to N times with a delay between attempts.
# Usage: assert_tui_contains_retry <expected> [max_attempts] [delay_seconds]
# Returns 0 if found within attempts, 1 otherwise.
assert_tui_contains_retry() {
  local expected="$1"
  local max_attempts="${2:-3}"
  local delay="${3:-10}"
  local attempt=1

  while [[ $attempt -le $max_attempts ]]; do
    if assert_tui_contains "$expected"; then
      return 0
    fi
    if [[ $attempt -lt $max_attempts ]]; then
      sleep "$delay"
    fi
    attempt=$((attempt + 1))
  done
  return 1
}

# Case-insensitive TUI contains check.
# Usage: assert_tui_contains_ci <expected>
# Returns 0 if found (case-insensitive), 1 otherwise.
assert_tui_contains_ci() {
  local expected="$1"
  local output
  output=$(capture_tui | strip_ansi)
  if ! printf '%s' "$output" | grep -qi "$expected"; then
    echo "FAIL: assert_tui_contains_ci: expected '$expected' (case-insensitive) not found in TUI output" >&2
    return 1
  fi
}

# Partial-match TUI assertion: checks if any word-like fragment of the
# expected string appears in the TUI output. Useful when the LLM wraps
# the sentinel in additional formatting.
# Usage: assert_tui_contains_partial <expected> [min_fragment_chars]
# Returns 0 if any fragment matches, 1 otherwise.
assert_tui_contains_partial() {
  local expected="$1"
  local min_chars="${2:-8}"
  local output
  output=$(capture_tui | strip_ansi)

  # Try exact match first.
  if printf '%s' "$output" | grep -qF "$expected"; then
    return 0
  fi

  # Extract sentinel-like fragments (alphanumeric with underscores).
  local fragments
  fragments=$(printf '%s' "$expected" | grep -oE '[A-Z0-9_]+')
  while IFS= read -r frag; do
    [[ ${#frag} -lt $min_chars ]] && continue
    if printf '%s' "$output" | grep -qF "$frag"; then
      return 0
    fi
  done <<< "$fragments"

  echo "FAIL: assert_tui_contains_partial: no fragment of '$expected' (>= $min_chars chars) found in TUI output" >&2
  return 1
}
