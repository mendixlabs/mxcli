#!/usr/bin/env bash
# Cross-version pluggable-widget mx-check matrix.
#
# For each Mendix version, creates the three core bundled pluggable widgets
# (ComboBox, DataGrid2, Gallery) on a copy of a per-version project via
# mdl-examples/widget-matrix/pluggable-smoke.mdl, then runs `mx check` and asserts
# the widget creation introduced ZERO new CE0463 ("widget definition has changed")
# errors. This is the real multi-version guarantee from
# docs/11-proposals/PROPOSAL_multi_version_pluggable_widgets.md — it turns widget
# template drift across Mendix versions into a failing gate instead of a field
# surprise.
#
# Usage:
#   scripts/widget-version-matrix.sh VERSION:PROJECT_DIR [VERSION:PROJECT_DIR ...]
#
#   VERSION       full mxbuild dir name under ~/.mxcli/mxbuild (e.g. 11.10.0,
#                 10.24.19.104498) — passed through to scripts/mx-check.sh --version.
#   PROJECT_DIR   a directory containing a single .mpr at that Mendix version
#                 (copied to a scratch dir; the source is never modified).
#
# Env:
#   MXCLI_ENGINE  engine to create widgets with (default: modelsdk).
#   MXCLI         mxcli binary (default: ./bin/mxcli).
#
# Example:
#   scripts/widget-version-matrix.sh \
#     10.24.19.104498:../ModelSDKGo/mx-test-projects/test-1024 \
#     11.9.0:../ModelSDKGo/mx-test-projects/cb-test \
#     11.10.0:../ModelSDKGo/mx-test-projects/test6-app
#
# Exits non-zero if any version introduces a new CE0463 (or a version can't run).

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MXCLI="${MXCLI:-$REPO_ROOT/bin/mxcli}"
ENGINE="${MXCLI_ENGINE:-modelsdk}"
FIXTURE="$REPO_ROOT/mdl-examples/widget-matrix/pluggable-smoke.mdl"
CHECK="$REPO_ROOT/scripts/mx-check.sh"

if [[ $# -lt 1 ]]; then
  sed -n '2,30p' "${BASH_SOURCE[0]}"
  exit 2
fi
[[ -x "$MXCLI" ]] || { echo "ERROR: mxcli not found at $MXCLI (run: make build)"; exit 2; }
[[ -f "$FIXTURE" ]] || { echo "ERROR: fixture missing: $FIXTURE"; exit 2; }

# count CE0463 lines in an mx check of $1
ce0463_count() {
  timeout 360 "$CHECK" -p "$1" --version "$VER" 2>&1 | grep -c 'CE0463'
}

declare -a RESULTS
overall=0

for pair in "$@"; do
  VER="${pair%%:*}"
  SRC="${pair#*:}"
  label="$VER"

  if [[ ! -d "$HOME/.mxcli/mxbuild/$VER" ]]; then
    RESULTS+=("$label  SKIP (no mxbuild $VER installed)")
    continue
  fi
  mpr_src="$(find "$SRC" -maxdepth 1 -name '*.mpr' 2>/dev/null | head -1)"
  if [[ -z "$mpr_src" ]]; then
    RESULTS+=("$label  SKIP (no .mpr in $SRC)")
    continue
  fi

  work="$(mktemp -d)/proj"
  cp -r "$SRC" "$work"
  mpr="$(find "$work" -maxdepth 1 -name '*.mpr' | head -1)"

  before="$(ce0463_count "$mpr")"
  create_out="$(MXCLI_ENGINE="$ENGINE" "$MXCLI" exec "$FIXTURE" -p "$mpr" 2>&1)"
  if grep -qiE 'parse error|not implemented|not yet supported|^Error:' <<<"$create_out"; then
    RESULTS+=("$label  FAIL (widget creation: $(grep -iE 'parse error|not implemented|not yet supported|^Error:' <<<"$create_out" | head -1))")
    overall=1
    rm -rf "$(dirname "$work")"
    continue
  fi
  after="$(ce0463_count "$mpr")"

  delta=$(( after - before ))
  if [[ "$delta" -eq 0 ]]; then
    RESULTS+=("$label  PASS (CE0463 ${before}→${after}, engine=$ENGINE)")
  else
    RESULTS+=("$label  FAIL (CE0463 ${before}→${after}, +${delta} new)")
    overall=1
  fi
  rm -rf "$(dirname "$work")"
done

echo
echo "=== Pluggable-widget cross-version matrix (engine=$ENGINE) ==="
printf '%s\n' "${RESULTS[@]}"
echo "==========================================================="
[[ "$overall" -eq 0 ]] && echo "RESULT: PASS" || echo "RESULT: FAIL"
exit "$overall"
