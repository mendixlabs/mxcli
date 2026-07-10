#!/bin/bash

# check-widget-versions.sh — cross-version widget-envelope drift gate (v0.12.0 Stream A / A3).
#
# Runs one widget fixture through `exec` + `mx check` against several Mendix
# versions and compares the CE0463 ("widget definition changed") sets. The
# pluggable-widget BSON envelope is the version-fragile layer; if it drifts
# between minors, a widget that passes on version X fails on version Y. This
# script catches that: it PASSES when every version produces the SAME CE0463
# set (no version-specific drift) and FAILS when any version has errors the
# others don't, naming the offending widgets.
#
# It does NOT require zero CE0463 — version-independent widget bugs (tracked
# separately, e.g. #605 tf1, dgDyn) appear on every version and are not drift.
# The gate is about *differences across versions*, not absolute cleanliness.
#
# Each version needs: its mxbuild installed (~/.mxcli/mxbuild/<ver>/) and a
# reference project that already has the fixture's widgets (.mpk) installed.
#
# Usage:
#   scripts/check-widget-versions.sh <fixture.mdl> <ver>:<project.mpr> [<ver>:<project.mpr> ...]
#
# Example:
#   scripts/check-widget-versions.sh \
#     mdl-examples/doctype-tests/31-pluggable-datagrid-gallery-v010-examples.mdl \
#     11.9.0:../ModelSDKGo/mx-test-projects/test5-app/test5.mpr \
#     11.10.0:../ModelSDKGo/mx-test-projects/test6-app/test6.mpr

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
MXCLI_BIN="${MXCLI_BIN:-$ROOT_DIR/bin/mxcli}"
MX_CHECK="$ROOT_DIR/scripts/mx-check.sh"

FIXTURE="${1:-}"
shift || true
[[ -n "$FIXTURE" && -f "$FIXTURE" ]] || { echo "error: first arg must be a fixture .mdl (got '$FIXTURE')" >&2; exit 2; }
[[ $# -ge 1 ]] || { echo "error: provide at least one <version>:<project.mpr> pair" >&2; exit 2; }
[[ -x "$MXCLI_BIN" ]] || { echo "error: mxcli binary not executable: $MXCLI_BIN (run 'make build')" >&2; exit 2; }
[[ -x "$MX_CHECK" ]] || { echo "error: $MX_CHECK not found/executable" >&2; exit 2; }

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

# ce0463_set <version> <project.mpr> — prints the sorted, de-duplicated set of
# widget names CE0463 fires on (one per line) for the fixture on that version.
# `mx check` exits non-zero when errors exist (expected) and grep exits 1 on no
# match (also expected), so both are tolerated rather than aborting set -e.
ce0463_set() {
	local ver="$1" mpr="$2"
	local src_dir name sandbox check_out
	src_dir="$(cd "$(dirname "$mpr")" && pwd)"
	name="$(basename "$mpr")"
	sandbox="$TMP_ROOT/$ver"
	rm -rf "$sandbox"
	mkdir -p "$sandbox"
	cp -R "$src_dir"/. "$sandbox"/

	if ! "$MXCLI_BIN" widget init -p "$sandbox/$name" >"$sandbox/.widgetinit.log" 2>&1; then
		echo "error: widget init failed for $ver — see $sandbox/.widgetinit.log" >&2
		return 1
	fi

	# Drop any modules the fixture creates before running it, so the comparison
	# is over a clean slate regardless of leftover/divergent state in the
	# reference project (e.g. a stale PgTest from an earlier exec). Without this
	# the gate compares two projects that aren't identical baselines and reports
	# false drift. Errors (module absent) are ignored.
	local mod
	for mod in $(grep -oiE 'create module [A-Za-z_][A-Za-z0-9_]*' "$FIXTURE" | awk '{print $3}' | sort -u); do
		"$MXCLI_BIN" -p "$sandbox/$name" -c "drop module $mod;" >/dev/null 2>&1 || true
	done

	if ! "$MXCLI_BIN" exec "$FIXTURE" -p "$sandbox/$name" >"$sandbox/.exec.log" 2>&1; then
		echo "error: exec failed for $ver — see $sandbox/.exec.log" >&2
		return 1
	fi

	check_out="$("$MX_CHECK" -p "$sandbox/$name" --version "$ver" 2>"$sandbox/.check.err" || true)"
	printf '%s\n' "$check_out" | grep -F "CE0463" | sed -E "s/.*' at //; s/.*at //" | sort -u || true
}

declare -a VERSIONS=()
declare -A SETS=()

for pair in "$@"; do
	ver="${pair%%:*}"
	mpr="${pair#*:}"
	[[ -n "$ver" && -n "$mpr" && "$ver" != "$mpr" ]] || { echo "error: bad pair '$pair' (expected <version>:<project.mpr>)" >&2; exit 2; }
	[[ -f "$mpr" ]] || { echo "error: project not found: $mpr" >&2; exit 2; }
	[[ -d "$HOME/.mxcli/mxbuild/$ver" ]] || { echo "error: mxbuild $ver not installed (~/.mxcli/mxbuild/$ver)" >&2; exit 2; }

	echo "== $ver  ($mpr) =="
	set_out="$(ce0463_set "$ver" "$mpr")"
	VERSIONS+=("$ver")
	SETS[$ver]="$set_out"
	if [[ -z "$set_out" ]]; then
		echo "   CE0463: none"
	else
		echo "$set_out" | sed 's/^/   CE0463: /'
	fi
done

# Compare every version's set against the first.
base="${VERSIONS[0]}"
drift=0
for ver in "${VERSIONS[@]:1}"; do
	only_in_ver="$(comm -13 <(printf '%s\n' "${SETS[$base]}") <(printf '%s\n' "${SETS[$ver]}") | sed '/^$/d')"
	only_in_base="$(comm -23 <(printf '%s\n' "${SETS[$base]}") <(printf '%s\n' "${SETS[$ver]}") | sed '/^$/d')"
	if [[ -n "$only_in_ver" || -n "$only_in_base" ]]; then
		drift=1
		echo
		echo "VERSION DRIFT: $base vs $ver"
		[[ -n "$only_in_ver" ]]  && echo "$only_in_ver"  | sed "s/^/   only on $ver: /"
		[[ -n "$only_in_base" ]] && echo "$only_in_base" | sed "s/^/   only on $base: /"
	fi
done

echo
if [[ $drift -eq 0 ]]; then
	echo "PASS: no cross-version envelope drift (identical CE0463 sets across ${VERSIONS[*]})"
	exit 0
fi
echo "FAIL: widget BSON envelope drifts across versions — see WIDGET_BSON_VERSION_COMPATIBILITY.md"
exit 1
