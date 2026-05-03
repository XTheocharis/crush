#!/bin/bash
#
# gen-treesitter-deps.sh
#
# Generates `go get` commands for tree-sitter grammar Go bindings that match
# the exact revisions used by Aider's tree-sitter-language-pack. Also outputs
# git commands to download Aider's tags.scm query files.
#
# Aider ships two query directories:
#   1. tree-sitter-language-pack/ — primary, curated by Aider (30 languages)
#   2. tree-sitter-languages/    — fallback, sourced from grammar repos' own
#                                  queries/tags.scm files (26 languages)
#
# The language-pack directory takes priority when both have a tags.scm for
# the same language. Languages unique to the fallback directory (e.g.
# typescript, kotlin, php, haskell) are included with their source tracked.
#
# Grammar revisions for all languages come from language_definitions.json
# in tree-sitter-language-pack, regardless of which query directory the
# tags.scm was sourced from.
#
# Requirements: git, jq
# Usage: ./scripts/gen-treesitter-deps.sh

set -euo pipefail

# Temp directory with cleanup.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Colors for stderr output.
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

log() { echo -e "${BOLD}==> $1${RESET}" >&2; }
warn() { echo -e "${YELLOW}WARN: $1${RESET}" >&2; }
err() { echo -e "${RED}ERROR: $1${RESET}" >&2; exit 1; }

# Check dependencies.
for cmd in git jq; do
    command -v "$cmd" >/dev/null 2>&1 || err "'$cmd' is required but not found"
done

# --------------------------------------------------------------------------
# Step 1: Clone Aider (shallow) and extract language-pack version + languages
# --------------------------------------------------------------------------
log "Cloning Aider repository (shallow)..."
git clone --depth 1 --quiet https://github.com/Aider-AI/aider.git "$TMPDIR/aider" 2>/dev/null

# Extract tree-sitter-language-pack version from requirements.
PACK_VERSION=$(grep -oP 'tree-sitter-language-pack==\K[0-9.]+' "$TMPDIR/aider/requirements.txt") \
    || err "Could not find tree-sitter-language-pack version in requirements.txt"
log "Found tree-sitter-language-pack==$PACK_VERSION"

# List Aider's tags.scm files from both query directories.
# Priority: tree-sitter-language-pack > tree-sitter-languages (matching Aider's
# get_scm_fname() fallback in repomap.py).
PACK_QUERIES_DIR="$TMPDIR/aider/aider/queries/tree-sitter-language-pack"
LANGS_QUERIES_DIR="$TMPDIR/aider/aider/queries/tree-sitter-languages"

if [ ! -d "$PACK_QUERIES_DIR" ]; then
    err "Queries directory not found: $PACK_QUERIES_DIR"
fi

# Collect languages from the primary (language-pack) directory.
declare -a AIDER_LANGS=()
declare -A LANG_SOURCE=()  # lang -> "pack" or "langs"
for f in "$PACK_QUERIES_DIR"/*-tags.scm; do
    lang=$(basename "$f" | sed 's/-tags\.scm$//')
    AIDER_LANGS+=("$lang")
    LANG_SOURCE["$lang"]="pack"
done
log "Found ${#AIDER_LANGS[@]} languages in tree-sitter-language-pack/"

# Collect additional languages from the fallback (tree-sitter-languages)
# directory, skipping any already covered by the primary directory.
FALLBACK_COUNT=0
if [ -d "$LANGS_QUERIES_DIR" ]; then
    for f in "$LANGS_QUERIES_DIR"/*-tags.scm; do
        lang=$(basename "$f" | sed 's/-tags\.scm$//')
        if [ -z "${LANG_SOURCE[$lang]+x}" ]; then
            AIDER_LANGS+=("$lang")
            LANG_SOURCE["$lang"]="langs"
            FALLBACK_COUNT=$((FALLBACK_COUNT + 1))
        fi
    done
    log "Found $FALLBACK_COUNT additional languages in tree-sitter-languages/"
else
    warn "Fallback query directory not found: $LANGS_QUERIES_DIR"
fi
log "Total: ${#AIDER_LANGS[@]} languages across both directories"

# --------------------------------------------------------------------------
# Step 2: Clone tree-sitter-language-pack at matching version
# --------------------------------------------------------------------------
log "Cloning tree-sitter-language-pack at v$PACK_VERSION..."
git clone --depth 1 --quiet --branch "v$PACK_VERSION" \
    https://github.com/Goldziher/tree-sitter-language-pack.git "$TMPDIR/tslp" 2>/dev/null

LANG_DEFS="$TMPDIR/tslp/sources/language_definitions.json"
if [ ! -f "$LANG_DEFS" ]; then
    err "language_definitions.json not found in language-pack"
fi

# Get all available JSON keys for languages.
ALL_JSON_KEYS=$(jq -r 'keys[]' "$LANG_DEFS")

# --------------------------------------------------------------------------
# Step 3: Map Aider language names to JSON keys
# --------------------------------------------------------------------------

# Resolve an Aider language name to the JSON key in language_definitions.json.
# Tries: exact match, underscore variant, hyphen variant.
resolve_json_key() {
    local lang="$1"

    # Exact match.
    if echo "$ALL_JSON_KEYS" | grep -qx "$lang"; then
        echo "$lang"
        return
    fi

    # Try replacing hyphens with underscores.
    local underscore_variant="${lang//-/_}"
    if echo "$ALL_JSON_KEYS" | grep -qx "$underscore_variant"; then
        echo "$underscore_variant"
        return
    fi

    # Try replacing underscores with hyphens.
    local hyphen_variant="${lang//_/-}"
    if echo "$ALL_JSON_KEYS" | grep -qx "$hyphen_variant"; then
        echo "$hyphen_variant"
        return
    fi

    return 1
}

# --------------------------------------------------------------------------
# Step 4: Process each language
# --------------------------------------------------------------------------
log "Processing languages..."

# Track results for summary.
declare -a GO_GET_CMDS=()
declare -a NO_BINDINGS=()
declare -a NOT_FOUND=()
declare -a SUMMARY_LINES=()

# Languages not found in language_definitions.json are deferred to pip fallback.
declare -a DEFERRED_LANGS=()

for lang in "${AIDER_LANGS[@]}"; do
    json_key=$(resolve_json_key "$lang") || {
        DEFERRED_LANGS+=("$lang")
        continue
    }

    source="${LANG_SOURCE[$lang]}"

    # Extract repo URL, revision, and optional directory from language_definitions.json.
    repo_url=$(jq -r --arg k "$json_key" '.[$k].repo' "$LANG_DEFS")
    rev=$(jq -r --arg k "$json_key" '.[$k].rev' "$LANG_DEFS")
    grammar_dir_field=$(jq -r --arg k "$json_key" '.[$k].directory // empty' "$LANG_DEFS")

    if [ "$repo_url" = "null" ] || [ "$rev" = "null" ]; then
        warn "Missing repo/rev for '$lang' (key: $json_key)"
        NOT_FOUND+=("$lang")
        SUMMARY_LINES+=("$lang|SKIPPED|${source}|missing repo or rev in JSON")
        continue
    fi

    # Convert GitHub URL to Go module path.
    # e.g., https://github.com/tree-sitter/tree-sitter-go -> github.com/tree-sitter/tree-sitter-go
    go_module=$(echo "$repo_url" | sed 's|https://||; s|\.git$||')

    # Check for Go bindings by shallow-cloning the grammar repo.
    clone_dir="$TMPDIR/grammar-$lang"
    has_go_bindings=false

    if git clone --depth 1 --quiet "$repo_url" "$clone_dir" 2>/dev/null; then
        # For grammars with a directory field (e.g. typescript -> "typescript",
        # tsx -> "tsx"), Go bindings may be in the subdirectory.
        if [ -n "$grammar_dir_field" ]; then
            bindings_base="$clone_dir/$grammar_dir_field"
        else
            bindings_base="$clone_dir"
        fi
        if [ -f "$bindings_base/bindings/go/binding.go" ] || [ -f "$bindings_base/bindings/go/binding_test.go" ]; then
            has_go_bindings=true
        # Also check the repo root for Go bindings (some multi-grammar repos
        # put bindings at root level).
        elif [ -n "$grammar_dir_field" ]; then
            if [ -f "$clone_dir/bindings/go/binding.go" ] || [ -f "$clone_dir/bindings/go/binding_test.go" ]; then
                has_go_bindings=true
            fi
        fi
        # Clean up immediately to save disk space.
        rm -rf "$clone_dir"
    else
        warn "Failed to clone $repo_url for '$lang'"
        NOT_FOUND+=("$lang")
        SUMMARY_LINES+=("$lang|ERROR|${source}|clone failed: $repo_url")
        continue
    fi

    short_rev="${rev:0:12}"
    dir_note=""
    if [ -n "$grammar_dir_field" ]; then
        dir_note=" (dir: $grammar_dir_field)"
    fi

    if $has_go_bindings; then
        GO_GET_CMDS+=("go get ${go_module}@${rev}")
        SUMMARY_LINES+=("$lang|GO|${source}|${go_module}@${short_rev}${dir_note}")
        echo -e "  ${GREEN}+${RESET} $lang -> ${go_module}@${short_rev}${dir_note} [${source}]" >&2
    else
        NO_BINDINGS+=("$lang")
        SUMMARY_LINES+=("$lang|NO GO|${source}|${go_module}@${short_rev}${dir_note}")
        echo -e "  ${YELLOW}-${RESET} $lang (no Go bindings) [${source}]" >&2
    fi
done

# --------------------------------------------------------------------------
# Step 4b: Pip package fallback for deferred languages
# --------------------------------------------------------------------------
# Some grammars (e.g. C#) aren't in language_definitions.json but are pulled
# in as transitive pip dependencies of tree-sitter-language-pack. We match
# them by normalizing names: strip hyphens/underscores so "csharp" matches
# "c-sharp" from pip package "tree-sitter-c-sharp".

if [ ${#DEFERRED_LANGS[@]} -gt 0 ]; then
    log "Resolving ${#DEFERRED_LANGS[@]} deferred language(s) via pip packages..."

    # Normalize a name by stripping hyphens and underscores, lowercasing.
    normalize() {
        echo "$1" | tr -d '_-' | tr '[:upper:]' '[:lower:]'
    }

    # Extract tree-sitter-* pip packages from requirements.txt.
    # Exclude the base library and language-pack itself.
    declare -A PIP_PKGS=()  # normalized_suffix -> "pkg_name==version"
    while IFS= read -r line; do
        pkg_name=$(echo "$line" | cut -d= -f1)
        pkg_version=$(echo "$line" | grep -oP '==\K[0-9.]+')
        # Strip "tree-sitter-" prefix to get the language suffix.
        suffix="${pkg_name#tree-sitter-}"
        norm=$(normalize "$suffix")
        PIP_PKGS["$norm"]="${pkg_name}==${pkg_version}"
    done < <(grep -oP 'tree-sitter-[a-z0-9][-a-z0-9]*==[0-9.]+' "$TMPDIR/aider/requirements.txt" \
        | grep -v -E '^tree-sitter==' \
        | grep -v 'tree-sitter-language-pack' \
        | grep -v 'tree-sitter-languages')

    for lang in "${DEFERRED_LANGS[@]}"; do
        norm_lang=$(normalize "$lang")

        if [ -z "${PIP_PKGS[$norm_lang]+x}" ]; then
            warn "No JSON key or pip package found for '$lang'"
            NOT_FOUND+=("$lang")
            SUMMARY_LINES+=("$lang|SKIPPED|${LANG_SOURCE[$lang]}|not in language_definitions.json or pip")
            continue
        fi

        pip_entry="${PIP_PKGS[$norm_lang]}"
        pkg_name=$(echo "$pip_entry" | cut -d= -f1)
        pkg_version=$(echo "$pip_entry" | grep -oP '==\K[0-9.]+')
        # Convention: pip package tree-sitter-X -> github.com/tree-sitter/tree-sitter-X
        repo_url="https://github.com/tree-sitter/${pkg_name}"
        go_module="github.com/tree-sitter/${pkg_name}"

        # Clone at the version tag to check for Go bindings.
        grammar_dir="$TMPDIR/grammar-pip-$lang"
        has_go_bindings=false
        tag_rev=""

        if git clone --depth 1 --quiet --branch "v${pkg_version}" "$repo_url" "$grammar_dir" 2>/dev/null; then
            if [ -f "$grammar_dir/bindings/go/binding.go" ] || [ -f "$grammar_dir/bindings/go/binding_test.go" ]; then
                has_go_bindings=true
            fi
            # Get the commit hash for the tag.
            tag_rev=$(git -C "$grammar_dir" rev-parse HEAD)
            rm -rf "$grammar_dir"
        else
            warn "Failed to clone $repo_url at v${pkg_version} for '$lang'"
            NOT_FOUND+=("$lang")
            SUMMARY_LINES+=("$lang|ERROR|${LANG_SOURCE[$lang]}|clone failed: ${repo_url}@v${pkg_version}")
            continue
        fi

        short_rev="${tag_rev:0:12}"

        source="${LANG_SOURCE[$lang]}"

        if $has_go_bindings; then
            GO_GET_CMDS+=("go get ${go_module}@${tag_rev}")
            SUMMARY_LINES+=("$lang|GO(pip)|${source}|${go_module}@${short_rev} (v${pkg_version})")
            echo -e "  ${GREEN}+${RESET} $lang -> ${go_module}@${short_rev} (pip: v${pkg_version}) [${source}]" >&2
        else
            NO_BINDINGS+=("$lang")
            SUMMARY_LINES+=("$lang|NO GO|${source}|${go_module}@${short_rev} (v${pkg_version})")
            echo -e "  ${YELLOW}-${RESET} $lang (no Go bindings, pip: v${pkg_version}) [${source}]" >&2
        fi
    done
fi

# --------------------------------------------------------------------------
# Step 5: Output
# --------------------------------------------------------------------------

echo ""
echo "# ============================================================================"
echo "# Tree-sitter dependencies for Crush repo map"
echo "# Generated from Aider $(git -C "$TMPDIR/aider" log -1 --format='%h %s' 2>/dev/null || echo '(unknown)')"
echo "# tree-sitter-language-pack v$PACK_VERSION"
echo "# Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "# ============================================================================"
echo ""

echo "# --- Download Aider's tags.scm query files ---"
echo "# git clone --depth 1 https://github.com/Aider-AI/aider.git /tmp/aider"
echo "# mkdir -p queries/"
echo "#"
echo "# Primary (tree-sitter-language-pack): curated by Aider"
echo "# cp /tmp/aider/aider/queries/tree-sitter-language-pack/*-tags.scm queries/"
echo "#"
echo "# Fallback (tree-sitter-languages): from grammar repos' own queries/tags.scm"
echo "# Only copy languages not already covered by the primary directory:"
echo "# for f in /tmp/aider/aider/queries/tree-sitter-languages/*-tags.scm; do"
echo "#     [ ! -f \"queries/\$(basename \"\$f\")\" ] && cp \"\$f\" queries/"
echo "# done"
echo "#"
echo "# rm -rf /tmp/aider"
echo ""

# Deduplicate go get commands (e.g. ocaml + ocaml_interface share a repo,
# csharp + c_sharp resolve to the same module).
declare -A SEEN_CMDS=()
declare -a UNIQUE_GO_GET_CMDS=()
for cmd in "${GO_GET_CMDS[@]}"; do
    if [ -z "${SEEN_CMDS[$cmd]+x}" ]; then
        SEEN_CMDS["$cmd"]=1
        UNIQUE_GO_GET_CMDS+=("$cmd")
    fi
done

echo "# --- Go bindings available (${#UNIQUE_GO_GET_CMDS[@]} unique modules, ${#GO_GET_CMDS[@]} grammars) ---"
for cmd in "${UNIQUE_GO_GET_CMDS[@]}"; do
    echo "$cmd"
done

if [ ${#NO_BINDINGS[@]} -gt 0 ]; then
    echo ""
    echo "# --- No Go bindings (${#NO_BINDINGS[@]} grammars) ---"
    for lang in "${NO_BINDINGS[@]}"; do
        echo "# $lang"
    done
fi

if [ ${#NOT_FOUND[@]} -gt 0 ]; then
    echo ""
    echo "# --- Not found in language_definitions.json (${#NOT_FOUND[@]}) ---"
    for lang in "${NOT_FOUND[@]}"; do
        echo "# $lang"
    done
fi

# Summary table to stderr.
echo "" >&2
log "Summary"
printf "  ${BOLD}%-25s %-8s %-6s %s${RESET}\n" "LANGUAGE" "STATUS" "SOURCE" "DETAILS" >&2
printf "  %-25s %-8s %-6s %s\n" "--------" "------" "------" "-------" >&2
for line in "${SUMMARY_LINES[@]}"; do
    IFS='|' read -r l_lang l_status l_source l_detail <<< "$line"
    printf "  %-25s %-8s %-6s %s\n" "$l_lang" "$l_status" "$l_source" "$l_detail" >&2
done

echo "" >&2
log "Done. ${#UNIQUE_GO_GET_CMDS[@]} unique go get commands (${#GO_GET_CMDS[@]} grammars), ${#NO_BINDINGS[@]} without Go bindings, ${#NOT_FOUND[@]} not found."
echo "Pipe stdout to a file to save the commands: ./scripts/gen-treesitter-deps.sh > deps.sh" >&2
