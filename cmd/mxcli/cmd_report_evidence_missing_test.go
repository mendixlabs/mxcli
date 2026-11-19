// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A test results file that is not there — a `--run` that failed before
// writing it, under `report all` — leaves the report standing without test
// results and says why, instead of failing the whole report: under
// `report all` that dropped tests and traceability with "open junit.xml: no
// such file or directory" though their journeys were given (ako/mxcli#2452).
func TestMissingJUnitIsAWarningNotAFailure(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	evidenceFlags(cmd, true)
	dir := t.TempDir()
	missing := filepath.Join(dir, "junit.xml")
	_ = cmd.Flags().Set("junit", missing)
	ev, err := collectEvidence(cmd, filepath.Join(dir, "App.mpr"), true)
	if err != nil {
		t.Fatalf("a missing results file fails the report: %v", err)
	}
	if len(ev.results) != 0 {
		t.Errorf("results = %v", ev.results)
	}
	found := false
	for _, w := range ev.warnings {
		if strings.Contains(w, missing) && strings.Contains(w, "no test results") {
			found = true
		}
	}
	if !found {
		t.Errorf("the page says the results file is missing: %v", ev.warnings)
	}
}
