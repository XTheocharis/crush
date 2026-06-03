#!/usr/bin/env bash
# verify_hashes.sh — Download each LSP server binary, extract, compute SHA256,
# and compare against the hashes embedded in catalog.json.
#
# Usage:
#   ./verify_hashes.sh              # verify all servers
#   ./verify_hashes.sh tinymist     # verify a single server
#
# Exit 0 if all match, exit 1 on mismatch.

set -euo pipefail

CATALOG_DIR="$(cd "$(dirname "$0")" && pwd)"
CATALOG="$CATALOG_DIR/catalog.json"
WORKDIR="${TMPDIR:-/tmp}/lsp-hash-verify-$$"

cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

mkdir -p "$WORKDIR"

# Extract server names from catalog.json (skip _prefixed keys and _skipped_servers).
server_names() {
  python3 -c "
import json, sys
data = json.load(open(sys.argv[1]))
skip = {'_comment', '_skipped_servers'}
for key in data:
  if not key.startswith('_') and key not in skip:
    print(key)
" "$CATALOG"
}

# Extract platforms for a given server: prints "os/arch url sha256 download_type" per line.
platform_entries() {
  local server="$1"
  python3 -c "
import json, sys
data = json.load(open(sys.argv[1]))
server = sys.argv[2]
entry = data.get(server, {})
for plat, info in entry.get('platforms', {}).items():
    url = info['url']
    sha = info['sha256']
    dtype = info.get('download_type', 'binary')
    print(f'{plat} {url} {sha} {dtype}')
" "$CATALOG" "$server"
}

download_and_hash() {
  local url="$1"
  local dtype="$2"
  local tmpfile="$WORKDIR/download"

  curl -sL -o "$tmpfile" "$url"

  case "$dtype" in
    binary)
      # Already a bare binary.
      sha256sum "$tmpfile" | awk '{print $1}'
      ;;
    gzip)
      gunzip -c "$tmpfile" | sha256sum | awk '{print $1}'
      ;;
    zip)
      # Extract the first file from the zip and hash it.
      local extract_dir="$WORKDIR/zip_extract"
      rm -rf "$extract_dir"
      mkdir -p "$extract_dir"
      unzip -o -d "$extract_dir" "$tmpfile" >/dev/null 2>&1
      # Find the binary (first non-directory file).
      local bin
      bin="$(find "$extract_dir" -maxdepth 1 -type f ! -name '*.pdb' | head -1)"
      if [ -z "$bin" ]; then
        bin="$(find "$extract_dir" -type f ! -name '*.pdb' | head -1)"
      fi
      sha256sum "$bin" | awk '{print $1}'
      ;;
    tar.gz)
      local extract_dir="$WORKDIR/tgz_extract"
      rm -rf "$extract_dir"
      mkdir -p "$extract_dir"
      tar xzf "$tmpfile" -C "$extract_dir" 2>/dev/null
      local bin
      bin="$(find "$extract_dir" -type f | head -1)"
      sha256sum "$bin" | awk '{print $1}'
      ;;
    *)
      echo "ERROR: unknown download_type: $dtype" >&2
      return 1
      ;;
  esac
}

verify_server() {
  local server="$1"
  local failed=0

  echo "=== $server ==="

  while IFS=' ' read -r plat url expected_sha dtype; do
    printf "  %-18s " "$plat"
    actual_sha="$(download_and_hash "$url" "$dtype")"
    if [ "$actual_sha" = "$expected_sha" ]; then
      echo "OK ($actual_sha)"
    else
      echo "MISMATCH"
      echo "    expected: $expected_sha"
      echo "    actual:   $actual_sha"
      failed=1
    fi
  done < <(platform_entries "$server")

  return $failed
}

# Main
if [ $# -gt 0 ]; then
  servers=("$@")
else
  mapfile -t servers < <(server_names)
fi

overall_failed=0
for s in "${servers[@]}"; do
  if ! verify_server "$s"; then
    overall_failed=1
  fi
done

if [ $overall_failed -eq 0 ]; then
  echo ""
  echo "All hashes verified OK."
  exit 0
else
  echo ""
  echo "Some hashes did not match!" >&2
  exit 1
fi
