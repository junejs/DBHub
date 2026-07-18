#!/usr/bin/env bash
# upload_agents.sh — sync agent_team/*.md into the current multica workspace.
#
# What it does: for every row in agents.tsv, read the agent's markdown file and
# either CREATE a new multica agent or UPDATE the existing one (matched by name)
# so the upload is idempotent and re-runnable.
#
# Why this is a script instead of inline `multica agent create` calls in chat:
# the agent markdown files are full of shell metacharacters — backtick code
# fences, $VAR-like tokens, etc. Passing them as `--instructions "$(cat file)"`
# inline happens to be safe (command-substitution results are not re-scanned by
# the shell), but routing the content through a variable (`--instructions
# "$instructions"`) makes that guarantee explicit and obvious. The script also
# centralises the file→name→description mapping and the create-or-update branch
# so every agent is uploaded identically.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(dirname "$SCRIPT_DIR")"
MANIFEST="${SKILL_DIR}/agents.tsv"

# ---- config (overridable by flags / env) -------------------------------------
SOURCE_DIR="./agent_team"
RUNTIME_ID="${MULTICA_RUNTIME_ID:-}"
VISIBILITY="workspace"   # so teammates / other agents can invoke them
DRY_RUN=0
ONLY_NAME=""

# ---- arg parsing -------------------------------------------------------------
usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Sync agent_team/*.md into the current multica workspace (idempotent).

Options:
  -s, --source-dir DIR   Directory with the *_AGENT.md files (default: ./agent_team)
  -r, --runtime-id ID    Target runtime id (default: first online local runtime,
                         or \$MULTICA_RUNTIME_ID if set)
  -v, --visibility MODE  private | workspace (default: workspace)
      --only NAME        Only upload the agent whose multica name is NAME
  -n, --dry-run          Print the multica commands instead of running them
  -h, --help             Show this help

Run this from the repository root so ./agent_team resolves correctly.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -s|--source-dir)  SOURCE_DIR="$2"; shift 2;;
    -r|--runtime-id)  RUNTIME_ID="$2"; shift 2;;
    -v|--visibility)  VISIBILITY="$2"; shift 2;;
    --only)           ONLY_NAME="$2"; shift 2;;
    -n|--dry-run)     DRY_RUN=1; shift;;
    -h|--help)        usage; exit 0;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 2;;
  esac
done

# ---- preflight ---------------------------------------------------------------
command -v jq      >/dev/null || { echo "jq is required (brew install jq)" >&2; exit 1; }
command -v multica >/dev/null || { echo "multica CLI not found on PATH" >&2; exit 1; }
[[ -f "$MANIFEST"  ]] || { echo "manifest not found: $MANIFEST" >&2; exit 1; }
[[ -d "$SOURCE_DIR" ]] || { echo "source dir not found: $SOURCE_DIR (run from repo root, or pass -s)" >&2; exit 1; }

# ---- resolve target runtime --------------------------------------------------
# Only passed on CREATE, never on UPDATE — an existing agent may have been
# intentionally moved to a different runtime; we don't want to clobber that.
if [[ -z "$RUNTIME_ID" ]]; then
  RUNTIME_ID="$(multica runtime list --output json \
    | jq -r '[.[] | select(.status=="online" and .runtime_mode=="local")] | .[0].id // empty')"
  if [[ -z "$RUNTIME_ID" ]]; then
    echo "no online local runtime found; pass --runtime-id or set MULTICA_RUNTIME_ID" >&2
    exit 1
  fi
fi

echo "› runtime:    $RUNTIME_ID"
echo "› source:     $SOURCE_DIR"
echo "› visibility: $VISIBILITY"
echo

# ---- existing agents: name -> id ---------------------------------------------
# Stored as a TAB-separated "name<TAB>id" list and looked up with awk, so the
# script stays portable to bash 3.2 (no associative arrays on stock macOS).
EXISTING="$(multica agent list --output json | jq -r '.[] | [.name, .id] | @tsv')"
lookup_id() {  # lookup_id <name> -> echoes the agent id, or empty if absent
  awk -F'\t' -v n="$1" '$1==n {print $2; exit}' <<<"$EXISTING"
}

# ---- runner: print in dry-run, otherwise execute with stdout suppressed ------
run() {
  if (( DRY_RUN )); then
    printf '  $ %s\n' "$*"
  else
    "$@" >/dev/null
  fi
}

# ---- upload loop -------------------------------------------------------------
created=0; updated=0; failed=0

while IFS=$'\t' read -r file name desc; do
  [[ -z "$file" || "$file" == \#* ]] && continue
  [[ -n "$ONLY_NAME" && "$name" != "$ONLY_NAME" ]] && continue

  src="$SOURCE_DIR/$file"
  if [[ ! -f "$src" ]]; then
    echo "✗ $name — missing file: $src"; failed=$((failed+1)); continue
  fi

  # Safe to expand even though the file contains backticks/$-tokens: the value
  # stored in a variable is substituted once and is never re-evaluated.
  instructions="$(< "$src")"

  id="$(lookup_id "$name")"
  if [[ -n "$id" ]]; then
    printf '↻ update  %s (%s)\n' "$name" "$id"
    if run multica agent update "$id" \
        --name "$name" \
        --instructions "$instructions" \
        --description "$desc" \
        --visibility "$VISIBILITY" \
        --output json; then
      updated=$((updated+1))
    else
      echo "  ✗ update failed"; failed=$((failed+1))
    fi
  else
    printf '+ create  %s\n' "$name"
    if run multica agent create \
        --name "$name" \
        --description "$desc" \
        --instructions "$instructions" \
        --runtime-id "$RUNTIME_ID" \
        --visibility "$VISIBILITY" \
        --output json; then
      created=$((created+1))
    else
      echo "  ✗ create failed"; failed=$((failed+1))
    fi
  fi
done < "$MANIFEST"

echo
echo "done: $created created, $updated updated, $failed failed"
[[ "$failed" -eq 0 ]]
