#!/usr/bin/env bash
# Which backend.FullBackend methods does anything actually CALL through a
# backend value?
#
# The engine answers unported methods with errUnimplemented. Counting the
# unimplemented ones overstates the problem badly: most are interface surface
# that only an implementation and its own reader ever touch, so nothing can
# reach them and the stub can never fire.
#
# WHAT THIS SCRIPT CANNOT TELL YOU. DEAD has three causes that want opposite
# fixes — a caller that bypasses the abstraction (port it), no caller anywhere
# (delete the method), or callers using a narrower interface with a different
# signature (also delete). All three report DEAD, because all three mean
# "nothing calls it through a backend value". Separate them by grepping for
# callers under ANY type; see the header of
# mdl/backend/modelsdk/unimplemented_reachability_test.go.
#
# Grep cannot tell the difference — `b.reader.GetRawUnitByName(...)` inside the
# MPR backend and `ctx.Backend.GetRawUnitByName(...)` in the executor look the
# same and receiver names vary. The compiler can: remove one method from its
# interface and rebuild. A clean build means nothing calls it through a backend
# value.
#
# Measured on 2026-09-12 over the 19 methods *Backend did not declare: 17 DEAD,
# 2 LIVE (GetRawUnitByName, ParseMicroflowBSON — four call sites, all in
# mdl/executor/cmd_microflows_builder.go). Both are now implemented.
# Re-measured 2026-09-15: six more DEAD entries turned out to be orphans or
# duplicates rather than bypasses and were deleted from the interface.
# TestNoReachableUnimplementedBackendMethods in mdl/backend/modelsdk holds the
# list this script produced.
#
# Slow on purpose: one full `go build ./...` per method, a few minutes for the
# whole set. Run it when the unimplemented list changes, not routinely.
#
# Usage:
#   scripts/backend-reachability.sh                 # every method *Backend lacks
#   scripts/backend-reachability.sh GetWorkflow …   # just these

set -u
cd "$(dirname "$0")/.." || exit 1

methods=("$@")
if [ ${#methods[@]} -eq 0 ]; then
	echo "usage: $0 <Method> [Method...]" >&2
	echo "(the current list is in mdl/backend/modelsdk/unimplemented_reachability_test.go)" >&2
	exit 2
fi

trap 'git checkout -- mdl/backend/ 2>/dev/null' EXIT

for m in "${methods[@]}"; do
	# Where is it declared? Interface methods sit at one tab of indent.
	f=$(grep -rl "^	$m(" mdl/backend/*.go | head -1)
	if [ -z "$f" ]; then
		echo "SKIP:  $m (no interface declaration found)"
		continue
	fi
	cp "$f" /tmp/backend-reachability.bak
	perl -i -pe "s{^(\t$m\()}{\t// PROBE \$1}" "$f"
	out=$(go build ./... 2>&1 | grep -v '^#' | head -4)
	cp /tmp/backend-reachability.bak "$f"
	if [ -z "$out" ]; then
		echo "DEAD:  $m"
	else
		echo "LIVE:  $m"
		echo "$out" | sed 's/^/         /'
	fi
done
