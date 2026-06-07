#!/usr/bin/env bash
# provider-setup.sh — Generate wave-specific crush.json configs for xrush-qa.
#
# Usage: bash provider-setup.sh <wave-number> [output-file]
#   wave-number : 1–5
#   output-file : defaults to test/xrush-qa/wave<N>.json
#
# Delegates to global-config.sh. Generates project-local config with model
# routing and wave-specific options. Does NOT include a providers block —
# providers come from the user-scoped global config at runtime.
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly QA_DIR="$(dirname "$SCRIPT_DIR")"

WAVE="${1:-}"
OUTPUT="${2:-}"

[[ -z "$WAVE" ]] && { echo "Usage: bash provider-setup.sh <wave-number> [output-file]" >&2; exit 1; }
[[ "$WAVE" =~ ^[1-5]$ ]] || { echo "ERROR: Wave must be 1-5, got: $WAVE" >&2; exit 1; }

if [[ -z "$OUTPUT" ]]; then
  OUTPUT="${QA_DIR}/wave${WAVE}.json"
fi

# Source the shared helper and validate global config.
source "${SCRIPT_DIR}/global-config.sh"
resolve_global_config

# Generate project-local config (no providers block).
generate_project_config "$WAVE" "$OUTPUT"
