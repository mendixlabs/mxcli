#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT_MPR="${1:?usage: run-mdl-tests.sh <project.mpr> [mxcli-bin] [test-spec] [bootstrap-mdl]}"
MXCLI_BIN="${2:-$ROOT_DIR/bin/mxcli}"
TEST_SPEC="${3:-$ROOT_DIR/mdl-examples/doctype-tests/microflow-spec.test.mdl}"
BOOTSTRAP_MDL="${4:-$ROOT_DIR/mdl-examples/doctype-tests/02-microflow-examples.mdl}"

# Validate the inputs up front — `${VAR:?...}` only fires for *unset* variables,
# so a typo'd path would otherwise get silently copied as an empty sandbox.
[[ -f "$PROJECT_MPR" ]] || { echo "error: project MPR not found: $PROJECT_MPR" >&2; exit 1; }
[[ -x "$MXCLI_BIN" ]] || { echo "error: mxcli binary not executable: $MXCLI_BIN" >&2; exit 1; }
[[ -f "$TEST_SPEC" ]] || { echo "error: test spec not found: $TEST_SPEC" >&2; exit 1; }
[[ -f "$BOOTSTRAP_MDL" ]] || { echo "error: bootstrap MDL not found: $BOOTSTRAP_MDL" >&2; exit 1; }

SOURCE_DIR="$(cd "$(dirname "$PROJECT_MPR")" && pwd)"
PROJECT_NAME="$(basename "$PROJECT_MPR")"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cp -R "$SOURCE_DIR"/. "$TMP_DIR"/

"$MXCLI_BIN" exec "$BOOTSTRAP_MDL" -p "$TMP_DIR/$PROJECT_NAME"
"$MXCLI_BIN" test "$TEST_SPEC" -p "$TMP_DIR/$PROJECT_NAME"
