#!/bin/bash

# mx-check.sh — run `mx check` with the libSkiaSharp/FreeType LD_PRELOAD fix.
#
# Some bundled mxbuild releases (observed on 11.10.0) abort in this devcontainer
# with:
#   symbol lookup error: .../libSkiaSharp.so: undefined symbol: FT_Get_BDF_Property
# Root cause: mx runs under the Temurin JVM, whose bundled libfreetype is
# stripped and lacks FT_Get_BDF_Property, so Skia loads the JVM's FreeType
# instead of the system one (which has the symbol). Preloading the system
# libfreetype makes it load first — fixing mx check/build/run while keeping Skia
# working (unlike the old move-aside hack, which disabled Skia).
#
# Note: `mxcli docker check`/`build`/`new` now apply this fix automatically; this
# wrapper is for invoking a bundled `mx` binary directly.
#
# Usage:
#   scripts/mx-check.sh -p <project.mpr> [--version X.Y.Z] [--mx /path/to/modeler/mx] [-- <extra mx check args>]
#
# Resolution order for the mx binary:
#   1. --mx <path>            explicit binary
#   2. --version X.Y.Z        ~/.mxcli/mxbuild/X.Y.Z/modeler/mx
#   3. auto                   the single ~/.mxcli/mxbuild/*/modeler/mx (errors if 0 or >1)

set -euo pipefail

PROJECT=""
VERSION=""
MX_BIN=""
EXTRA=()

usage() {
	echo "usage: scripts/mx-check.sh -p <project.mpr> [--version X.Y.Z] [--mx <mx-binary>] [-- <extra mx check args>]" >&2
	exit 2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		-p|--project) PROJECT="${2:?-p requires a path}"; shift 2 ;;
		--version)    VERSION="${2:?--version requires X.Y.Z}"; shift 2 ;;
		--mx)         MX_BIN="${2:?--mx requires a path}"; shift 2 ;;
		--)           shift; EXTRA+=("$@"); break ;;
		-h|--help)    usage ;;
		*)            echo "error: unknown argument: $1" >&2; usage ;;
	esac
done

[[ -n "$PROJECT" ]] || { echo "error: -p <project.mpr> is required" >&2; usage; }
[[ -f "$PROJECT" ]] || { echo "error: project not found: $PROJECT" >&2; exit 1; }

# Resolve the mx binary.
if [[ -z "$MX_BIN" ]]; then
	if [[ -n "$VERSION" ]]; then
		MX_BIN="$HOME/.mxcli/mxbuild/$VERSION/modeler/mx"
	else
		mapfile -t candidates < <(ls -d "$HOME"/.mxcli/mxbuild/*/modeler/mx 2>/dev/null || true)
		if [[ ${#candidates[@]} -eq 1 ]]; then
			MX_BIN="${candidates[0]}"
		elif [[ ${#candidates[@]} -eq 0 ]]; then
			echo "error: no mxbuild found under ~/.mxcli/mxbuild; pass --version or --mx" >&2
			exit 1
		else
			echo "error: multiple mxbuild versions installed; pass --version X.Y.Z:" >&2
			printf '  %s\n' "${candidates[@]}" >&2
			exit 1
		fi
	fi
fi

[[ -x "$MX_BIN" ]] || { echo "error: mx binary not found or not executable: $MX_BIN" >&2; exit 1; }

# Find the system libfreetype across common multiarch locations (works on
# amd64, aarch64, …) and prepend it to LD_PRELOAD for the mx invocation only.
FREETYPE=""
for pat in /usr/lib/*/libfreetype.so.6 /usr/lib/libfreetype.so.6 /lib/*/libfreetype.so.6 /usr/local/lib/libfreetype.so.6; do
	for cand in $pat; do
		if [[ -f "$cand" ]]; then FREETYPE="$cand"; break 2; fi
	done
done

PRELOAD="${LD_PRELOAD:-}"
if [[ -n "$FREETYPE" && "$PRELOAD" != *libfreetype.so* ]]; then
	if [[ -n "$PRELOAD" ]]; then
		PRELOAD="$FREETYPE:$PRELOAD"
	else
		PRELOAD="$FREETYPE"
	fi
fi

# Run the check, preserving mx's exit code for callers / CI.
set +e
LD_PRELOAD="$PRELOAD" "$MX_BIN" check "$PROJECT" "${EXTRA[@]}"
rc=$?
set -e
exit "$rc"
