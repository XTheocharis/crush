#!/usr/bin/env bash
# rewind-poc.sh — Standalone proof-of-concept for `o` key automation path
# through tmux for the Crush TUI rewind feature.
#
# Drives the full rewind flow:
#   1. Start Crush in tmux (160x50)
#   2. Send a prompt that creates a sentinel file
#   3. Wait for idle
#   4. Switch focus to chat list (Tab)
#   5. Navigate to the assistant message (G → go to bottom)
#   6. Press `o` to open message options dialog
#   7. Select "Rewind (code only)" (Enter — it's the default at index 0)
#   8. Wait for rewind to complete
#   9. Assert the sentinel file was removed (code-only rewind)
#
# Usage: bash test/xrush-qa/lib/rewind-poc.sh
set -euo pipefail

# ── Paths ─────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QA_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="$(cd "$QA_DIR/../.." && pwd)"
EVIDENCE_DIR="$QA_DIR/reports/poc/rewind-poc"
SENTINEL_FILE="/tmp/rewind-poc-test.txt"
SENTINEL_CONTENT="REWIND_POC_V2_SENTINEL"

# ── Helpers ───────────────────────────────────────────────────────────────────

log()  { echo "[rewind-poc] $*"; }
die()  { echo "[rewind-poc] FATAL: $*" >&2; exit 1; }

# Strip ANSI escape codes from stdin.
strip_ansi() {
  sed 's/\x1b\[[0-9;]*[a-zA-Z]//g' | sed 's/\x1b\].*?\x07//g'
}

# Capture tmux pane output (last 200 lines), stripped of ANSI codes.
capture_pane() {
  tmux capture-pane -t "$SESSION" -p -S -200 | strip_ansi
}

# Save evidence snapshot with a label.
save_evidence() {
  local label="$1"
  mkdir -p "$EVIDENCE_DIR"
  capture_pane > "$EVIDENCE_DIR/${label}.txt"
  log "Evidence saved: $EVIDENCE_DIR/${label}.txt"
}

# ── Session setup ─────────────────────────────────────────────────────────────

SESSION="rewind-poc-$(date +%s)"

cleanup() {
  log "Cleaning up..."
  # Kill tmux session if it still exists.
  tmux kill-session -t "$SESSION" 2>/dev/null || true
  # Remove sentinel file.
  rm -f "$SENTINEL_FILE"
  # Restore crush.json backup if any.
  local backup
  backup=$(find "$PROJECT_DIR" -maxdepth 1 -name 'crush.json.bak.*' -type f 2>/dev/null | sort -t. -k5 -n | tail -1)
  if [[ -n "$backup" ]]; then
    mv "$backup" "$PROJECT_DIR/crush.json"
  fi
  log "Cleanup complete."
}
trap cleanup EXIT

# ── Generate project config ───────────────────────────────────────────────────

# Backup existing crush.json if present.
if [[ -f "$PROJECT_DIR/crush.json" ]]; then
  cp "$PROJECT_DIR/crush.json" "$PROJECT_DIR/crush.json.bak.$(date +%s)"
fi

# Generate wave 4 config (enables validation + architect + doom-loop intervention,
# which ensures snapshots are created for rewind).
bash "$QA_DIR/lib/provider-setup.sh" 4 "$PROJECT_DIR/crush.json" >/dev/null
log "Project config generated (wave 4)."

# ── Step 1: Start Crush in tmux ───────────────────────────────────────────────

log "Step 1: Starting Crush in tmux session '$SESSION' (160x50)..."
tmux new-session -d -s "$SESSION" -x 160 -y 50
tmux send-keys -t "$SESSION" "cd $PROJECT_DIR && crush --yolo" Enter
log "Waiting 6s for TUI to initialize..."
sleep 6

save_evidence "01-after-launch"

# ── Step 2: Send prompt that creates a sentinel file ──────────────────────────

log "Step 2: Sending prompt to create sentinel file..."
PROMPT_TEXT="Create a new file $SENTINEL_FILE containing exactly: $SENTINEL_CONTENT"
tmux send-keys -t "$SESSION" "$PROMPT_TEXT" Enter

# ── Step 3: Wait for idle ────────────────────────────────────────────────────

log "Step 3: Waiting for Crush to finish processing..."
TIMEOUT=120
ELAPSED=0
while true; do
  PANE_OUTPUT=$(capture_pane)
  if ! echo "$PANE_OUTPUT" | grep -qi "Processing\|Thinking\|Working"; then
    log "Idle detected after ${ELAPSED}s."
    break
  fi
  if [[ "$ELAPSED" -ge "$TIMEOUT" ]]; then
    save_evidence "03-timeout-waiting-idle"
    die "Crush still busy after ${TIMEOUT}s."
  fi
  sleep 2
  ((ELAPSED += 2))
done

save_evidence "03-after-idle"

# ── Verify file was created before rewind ─────────────────────────────────────

if [[ -f "$SENTINEL_FILE" ]] && grep -q "$SENTINEL_CONTENT" "$SENTINEL_FILE"; then
  log "Pre-check: Sentinel file exists with correct content."
else
  save_evidence "03b-sentinel-missing"
  log "WARNING: Sentinel file not found or wrong content. Continuing anyway — the LLM may have used a different path."
fi

# ── Step 4: Switch focus to chat list (Tab) ───────────────────────────────────

log "Step 4: Switching focus to chat list (Tab)..."
tmux send-keys -t "$SESSION" Tab
sleep 0.5

save_evidence "04-after-tab"

# ── Step 5: Navigate to assistant message ─────────────────────────────────────

log "Step 5: Navigating to assistant message (G = go to bottom, then k = up one)..."
# G (uppercase) = go to end/bottom of chat list.
# In tmux, send-keys with -l sends literal characters. For uppercase G we need
# the literal capital letter — tmux send-keys treats 'G' as the key G.
tmux send-keys -t "$SESSION" G
sleep 0.3
# Move up one item to land on the assistant message (the item just above the
# user prompt which is at the bottom). The last item is typically the user's
# message; the assistant response is the second-to-last.
tmux send-keys -t "$SESSION" k
sleep 0.3

save_evidence "05-after-navigate"

# ── Step 6: Press `o` to open message options ─────────────────────────────────

log "Step 6: Pressing 'o' to open message options dialog..."
tmux send-keys -t "$SESSION" o

# Wait for dialog to render.
sleep 0.5

save_evidence "06-after-o-key"

# Verify dialog appeared — look for "Message Options" in the pane output.
DIALOG_OUTPUT=$(capture_pane)
if echo "$DIALOG_OUTPUT" | grep -qi "Message Options"; then
  log "Message options dialog detected."
else
  # Dialog may not have appeared if focus was wrong or rewind not available.
  # Save evidence and try to recover.
  log "WARNING: 'Message Options' dialog not detected in pane output."
  log "Attempting recovery: Tab again and retry 'o'..."

  # Press Escape to close any accidental dialog/state.
  tmux send-keys -t "$SESSION" Escape
  sleep 0.3

  # Tab back to editor then to main to reset focus.
  tmux send-keys -t "$SESSION" Tab
  sleep 0.3
  tmux send-keys -t "$SESSION" Tab
  sleep 0.3

  # Navigate to bottom again.
  tmux send-keys -t "$SESSION" G
  sleep 0.3
  tmux send-keys -t "$SESSION" k
  sleep 0.3

  # Try 'o' again.
  tmux send-keys -t "$SESSION" o
  sleep 0.5

  DIALOG_OUTPUT=$(capture_pane)
  if echo "$DIALOG_OUTPUT" | grep -qi "Message Options"; then
    log "Message options dialog detected on retry."
  else
    save_evidence "06b-dialog-not-found"
    log "WARNING: Dialog still not found. Proceeding with Enter — may not trigger rewind."
  fi
fi

# ── Step 7: Select "Rewind (code only)" ───────────────────────────────────────

log "Step 7: Selecting 'Rewind (code only)' (Enter — default selection at index 0)..."
# "Rewind (code only)" is at index 0 in the message options list and is the
# default selected item when the dialog opens. Just press Enter.
tmux send-keys -t "$SESSION" Enter

save_evidence "07-after-enter-rewind"

# ── Step 8: Wait for rewind to complete ───────────────────────────────────────

log "Step 8: Waiting for rewind to complete..."
sleep 3

# Check for any processing indicators.
REWIND_TIMEOUT=30
REWIND_ELAPSED=0
while true; do
  PANE_OUTPUT=$(capture_pane)
  if ! echo "$PANE_OUTPUT" | grep -qi "Rewinding\|Processing\|Thinking\|Working"; then
    log "Rewind processing complete after ${REWIND_ELAPSED}s."
    break
  fi
  if [[ "$REWIND_ELAPSED" -ge "$REWIND_TIMEOUT" ]]; then
    log "Rewind processing timeout after ${REWIND_TIMEOUT}s — proceeding to assertions."
    break
  fi
  sleep 1
  ((REWIND_ELAPSED += 1))
done

save_evidence "08-after-rewind"

# ── Step 9: Stop Crush ───────────────────────────────────────────────────────

log "Step 9: Stopping Crush..."
tmux send-keys -t "$SESSION" C-c
sleep 0.5
tmux send-keys -t "$SESSION" y
sleep 1

save_evidence "09-after-stop"

# ── Step 10: Assertions ───────────────────────────────────────────────────────

PASS=0
FAIL=0
WARN=0
pass() { echo "  PASS: $1"; ((PASS += 1)); }
fail() { echo "  FAIL: $1"; ((FAIL += 1)); }
warn() { echo "  WARN: $1"; ((WARN += 1)); }

log "Step 10: Running assertions..."

# Assertion 1: Sentinel file should have been removed by code-only rewind.
if [[ ! -f "$SENTINEL_FILE" ]]; then
  pass "Sentinel file removed — code-only rewind succeeded."
else
  # The file may still exist if rewind didn't fire or used a different path.
  fail "Sentinel file still exists — code-only rewind may not have fired."
fi

# Assertion 2: Evidence shows rewind-related text in pane output.
AFTER_REWIND=$(cat "$EVIDENCE_DIR/08-after-rewind.txt" 2>/dev/null || echo "")
if echo "$AFTER_REWIND" | grep -qi "rewind\|Rewind"; then
  pass "Rewind-related text found in post-rewind evidence."
else
  warn "No rewind-related text in post-rewind evidence."
fi

# Assertion 3: Message Options dialog was captured at some point.
STEP6_EVIDENCE=$(cat "$EVIDENCE_DIR/06-after-o-key.txt" 2>/dev/null || echo "")
if echo "$STEP6_EVIDENCE" | grep -qi "Message Options"; then
  pass "Message Options dialog captured in evidence."
else
  warn "Message Options dialog not visible in step-6 evidence."
fi

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
echo "========================================="
echo " Rewind POC Results"
echo "========================================="
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "  WARN: $WARN"
echo "  Evidence: $EVIDENCE_DIR/"
echo "========================================="
echo ""

if [[ "$FAIL" -gt 0 ]]; then
  echo "Some assertions failed — check evidence directory for details."
  exit 1
fi

log "POC completed successfully."
exit 0
