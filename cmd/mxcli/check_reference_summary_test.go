// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1928 (mendixlabs/mxcli#1726): a run that failed on a reference
// error ended with "0 issues: 0 errors, 0 warnings, 0 info" — the summary
// counted the violations the other tiers report, and the reference tier
// printed its errors itself. The summary is the total, so it counts them.
func TestCheck_SummaryCountsReferenceErrors(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", filepath.Join(dir, "PedApp.mpr"))
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	scripts := t.TempDir()
	bad := writeScript(t, scripts, "bad.mdl", `mdl 1;
create or modify microflow MyFirstModule.CallsNothing ()
begin
  call microflow MyFirstModule.NoSuchFlow1928();
end;
`)
	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{bad}) })
	if code == 0 || !strings.Contains(out, "microflow not found: MyFirstModule.NoSuchFlow1928") {
		t.Fatalf("the reference error was not reported (exit %d):\n%s", code, out)
	}
	if strings.Contains(out, " 0 errors") || !strings.Contains(out, "1 errors") {
		t.Errorf("the summary does not count the reference error:\n%s", out)
	}
}
