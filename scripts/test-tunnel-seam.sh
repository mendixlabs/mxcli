#!/usr/bin/env bash
# Run the tunnel seam's !linux stub tests on a non-Linux runner (ADR-0009).
# Shared by push-test.yml (Windows, on every PR and main push) and
# nightly.yml (macOS), so the two cannot drift.
#
# Scoped with -run to the seam's own tests. The full test binaries are still
# COMPILED for this platform, so a Windows/macOS build break is still caught;
# only the !linux stub behaviour is executed.
#
# Running the whole packages here fails on Windows for reasons that predate
# this check and are unrelated to the tunnel: several tests assert POSIX file
# modes (0600) that Windows does not implement — os.Chmod only toggles the
# read-only bit, so Stat reports 666 — plus one path-separator assumption.
# Tracked separately in #897.
#
# -run can pass vacuously if the tests are renamed or deleted, so assert that
# the expected number actually ran.
#
# `tee` rather than a command substitution: under `bash -e -o pipefail`,
# capturing into `out=$(go test ...)` aborts AT THE ASSIGNMENT when go test
# fails, and the output is never printed. The log then holds nothing but
# "Process completed with exit code 1" — no test name, no failure message, no
# way to tell a real break from a flake. Write the output as it is produced,
# then judge it.
set -uo pipefail

out=go-test-output.txt
go test -v -count=1 -run 'Unsupported' ./cmd/mxcli/docker/... ./cmd/mxcli/tunnelhub/... 2>&1 | tee "$out"
status=${PIPESTATUS[0]}

n=$(grep -c '^--- PASS: Test.*Unsupported' "$out" || true)
echo "seam tests executed: $n"
if [ "$status" -ne 0 ]; then
  echo "FAIL: go test exited $status. Its output is above."
  exit "$status"
fi
if [ "$n" -lt 4 ]; then
  echo "FAIL: expected at least 4 tunnel-seam tests to run, -run matched $n."
  echo "      The !linux stubs in cmd/mxcli/docker and cmd/mxcli/tunnelhub"
  echo "      must each keep a test whose name contains 'Unsupported'."
  exit 1
fi
