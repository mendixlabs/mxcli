// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
)

// ako/mxcli#2160: `check -p` runs several tiers that each list the project's
// flows. Each tier held the units it read only for itself, so the next tier
// read them again — on a project of thousands of microflows, most of what a
// check of one statement cost.
//
// A read not served from memory is a file read or a hit on the persisted unit
// store; the stats file counts both. A check that reads no unit twice cannot
// make more of them than the project has units. Before the tiers shared what
// they read, this script made 454 on the fixture's 369 units.
func TestCheck_ReadsEachUnitOnce(t *testing.T) {
	mpr := copyPedApp(t)
	// A fixture that has been used in place carries a store; the copy must
	// start without one for a second read to be told from a first.
	if err := os.RemoveAll(filepath.Join(filepath.Dir(mpr), ".mxcli")); err != nil {
		t.Fatal(err)
	}
	stats := filepath.Join(t.TempDir(), "stats.jsonl")
	t.Setenv(executor.StatsFileEnv, stats)
	if err := checkCmd.Flags().Set("references", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = checkCmd.Flags().Set("references", "false") })

	// A flow change: the script every flow tier has something to do for.
	const script = `mdl 1;
create or modify microflow MyFirstModule.ReadsOnceProbe ($Count: integer) returns integer
begin
  declare $Total integer = $Count;
  return $Total;
end;
`
	code, out := runCheck2014(t, mpr, script)
	if code != 0 {
		t.Fatalf("check failed (exit %d):\n%s", code, out)
	}

	data, err := os.ReadFile(stats)
	if err != nil {
		t.Fatalf("no stats written: %v", err)
	}
	var total executor.ProcessStats
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var s executor.ProcessStats
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("stats line %q: %v", line, err)
		}
		total.UnitFileReads += s.UnitFileReads
		total.UnitStoreHits += s.UnitStoreHits
	}
	units, _ := filepath.Glob(filepath.Join(filepath.Dir(mpr), "mprcontents", "*", "*", "*.mxunit"))
	reads := total.UnitFileReads + total.UnitStoreHits
	// The flow tiers ran: they read the project's flows.
	if reads < 50 || len(units) < 50 {
		t.Fatalf("check read %d of %d units: the tiers this test is about did not run\n%s", reads, len(units), out)
	}
	if reads > int64(len(units)) {
		t.Fatalf("check made %d unit reads in a project of %d units: its tiers do not share what they read", reads, len(units))
	}
}
