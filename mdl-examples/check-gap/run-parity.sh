#!/bin/bash
# Parity harness: mxcli check -p vs mx check (vs mendix check) over broken fixtures.
set -uo pipefail
PROJECT=""; MX=""; MENDIX=""; MXCLI="${MXCLI:-./bin/mxcli}"
while [ $# -gt 0 ]; do case "$1" in
  --project) PROJECT="$2"; shift 2;; --mx) MX="$2"; shift 2;;
  --mendix) MENDIX="$2"; shift 2;; --mxcli) MXCLI="$2"; shift 2;;
  *) echo "unknown arg $1" >&2; exit 2;; esac; done
[ -f "$PROJECT" ] && [ -x "$MX" ] || { echo "usage: $0 --project clean.mpr --mx .../modeler/mx [--mendix path]" >&2; exit 2; }
HERE="$(cd "$(dirname "$0")" && pwd)"; PDIR="$(dirname "$PROJECT")"; PNAME="$(basename "$PROJECT")"
LIB=$(ls /usr/lib/$(uname -m)-linux-gnu/libfreetype.so.6 2>/dev/null || true)
[ -n "$LIB" ] && export LD_PRELOAD="$LIB"
codes() { grep -o '\[C[EW][0-9]*\]' | sort -u | tr -d '[]' | tr '\n' ' '; }
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
printf '%-22s | %-14s | %-16s | %-16s | %s\n' fixture "check -p" "mx check" "mendix check" "secs mx/mendix"
for f in "$HERE"/fixtures/f*.mdl; do
  n=$(basename "$f" .mdl); W="$TMP/$n"; cp -r "$PDIR" "$W"
  { cat "$HERE/fixtures/00_base.mdl"; grep -v '^mdl 1;$' "$f"; } > "$W/script.mdl"   # one header only
  pre=$("$MXCLI" check "$W/script.mdl" -p "$PROJECT" 2>&1); case "$pre" in *"Check passed"*) pre=passed;; *) pre=REJECTED;; esac
  "$MXCLI" exec "$W/script.mdl" -p "$W/$PNAME" --no-check >/dev/null 2>&1 || { printf '%-22s | exec refused\n' "$n"; continue; }
  t0=$(date +%s.%N); b=$(cd "$W" && "$MX" check "$PNAME" 2>&1); t1=$(date +%s.%N)
  a="-"; t2=$t1; if [ -n "$MENDIX" ]; then a=$(cd "$W" && "$MENDIX" check "$PNAME" 2>&1 | codes); t2=$(date +%s.%N); fi
  printf '%-22s | %-14s | %-16s | %-16s | %.1f/%.1f\n' "$n" "$pre" "$(echo "$b"|codes)" "$a" "$(echo "$t1-$t0"|bc)" "$(echo "$t2-$t1"|bc)"
done
