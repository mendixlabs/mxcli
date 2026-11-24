// SPDX-License-Identifier: Apache-2.0

package sessionreport

import "testing"

// ako/mxcli#2525: a Bash read that exits 0 but prints documentation quoting an
// error ("ERROR - Client: …" in a skill, "ERROR: `Action: OPEN LINK` …" in
// `mxcli syntax`) was counted as a failure, and the next call as a retry.
// The line heuristic exists for commands whose exit code a pipe hides; a call
// that only reads (orientation) has nothing to hide.
func TestReadPrintingErrorTextIsNotAFailure_2525(t *testing.T) {
	for _, tc := range []struct {
		cmd, out string
		failed   bool
	}{
		{"sed -n 440,537p .ai-context/skills/write-workflows/SKILL.md",
			"ERROR - Client: You can't complete this user task, it is not assigned to you.", false},
		{"./mxcli syntax page action -q",
			"ERROR: `Action: OPEN LINK` without a URL or `Action: SHOW PAGE` without a", false},
		{"cat docs/x.md | grep -n ERROR", "ERROR: something", false},
		// Controls: a validation whose exit a pipe hides still fails on its
		// error line, and so does a mixed chain.
		{"./mxcli check x.mdl -p app.mpr | tail -3", "Error: parse failed", true},
		{"cat notes.md; ./mxcli exec x.mdl -p app.mpr", "Error: refused", true},
	} {
		failed, _ := detectFailure("Bash", tc.cmd, false, tc.out)
		if failed != tc.failed {
			t.Errorf("%s\n  -> %q: failed = %v, want %v", tc.cmd, tc.out, failed, tc.failed)
		}
	}
	// A harness-marked error is a failure whatever the command.
	if failed, _ := detectFailure("Bash", "cat missing.md", true, "cat: missing.md: No such file"); !failed {
		t.Error("an is_error read was not a failure")
	}
}
