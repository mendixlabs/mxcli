// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
)

// ako/mxcli#2004: `search` on a project without a source index answered from
// string literals alone, printed "No matches found." for a name used only in a
// flow body, and exited 0. A plain search now builds the index (once, saying
// so); `--no-build` never does, and exits 3 when that left the source
// unsearched, so the answer cannot be read as "nothing matched".
func TestSearchNoBuild_ExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns mxcli against a project")
	}
	mpr := jsonPurityFixture(t)
	dir := filepath.Dir(mpr)

	// No source index yet, --no-build: incomplete, exit 3, still a pure JSON
	// array on stdout, the reason on stderr.
	args := []string{"-p", mpr, "--json", "search", "zzqqxxnomatch", "--no-build"}
	r := runMainStreams(t, dir, args...)
	if r.exitCode != 3 {
		t.Fatalf("--no-build without a source index: exit %d, want 3\nstdout:\n%s\nstderr:\n%s", r.exitCode, r.stdout, r.stderr)
	}
	assertPureJSON(t, args, r)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &rows); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, r.stdout)
	}
	if !strings.Contains(r.stderr, "source index") || strings.Contains(r.stderr, "Building the source index") {
		t.Errorf("want the not-built warning and no build on stderr:\n%s", r.stderr)
	}

	// A plain search builds it, says so, and exits 0 with nothing found.
	args = []string{"-p", mpr, "--json", "search", "zzqqxxnomatch"}
	r = runMainStreams(t, dir, args...)
	if r.exitCode != 0 || !strings.Contains(r.stderr, "Building the source index") {
		t.Fatalf("plain search: exit %d (want 0) and the build announced\nstderr:\n%s", r.exitCode, r.stderr)
	}
	assertPureJSON(t, args, r)

	// Control: with the index in place, --no-build is a complete answer.
	r = runMainStreams(t, dir, "-p", mpr, "--json", "search", "zzqqxxnomatch", "--no-build")
	if r.exitCode != 0 || strings.Contains(r.stderr, "is not built") {
		t.Fatalf("--no-build with a source index: exit %d, want 0 and no warning\nstderr:\n%s", r.exitCode, r.stderr)
	}
}

func TestSearchExitCode(t *testing.T) {
	for _, tc := range []struct {
		res         executor.SearchOutcome
		failOnMatch bool
		want        int
	}{
		{executor.SearchOutcome{}, false, 0},
		{executor.SearchOutcome{Hits: 2}, false, 0},
		{executor.SearchOutcome{Hits: 2}, true, 1},
		{executor.SearchOutcome{}, true, 0},
		{executor.SearchOutcome{Incomplete: true}, false, 3},
		{executor.SearchOutcome{Incomplete: true}, true, 3},
		{executor.SearchOutcome{Hits: 1, Incomplete: true}, true, 1}, // a hit is a definite answer
		{executor.SearchOutcome{Hits: 1, Incomplete: true}, false, 3},
	} {
		if got := searchExitCode(tc.res, tc.failOnMatch); got != tc.want {
			t.Errorf("searchExitCode(%+v, failOnMatch=%v) = %d, want %d", tc.res, tc.failOnMatch, got, tc.want)
		}
	}
}
