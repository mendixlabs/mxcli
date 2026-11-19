// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func manyInfos(n int) []Finding {
	var fs []Finding
	for i := 0; i < n; i++ {
		fs = append(fs, Finding{Stage: StageLint, Code: "QUAL002", Severity: "info", Document: fmt.Sprintf("M.P%d", i), Message: "no documentation", Key: fmt.Sprintf("k%d", i), MxStage: -1})
	}
	return fs
}

func textOf(r Result, failOn string) string {
	var b bytes.Buffer
	WriteText(&b, r, failOn)
	return b.String()
}

// Findings below the failure threshold are counted, not listed: 55 info lines
// from a stale baseline bury the one error that fails the gate.
func TestWriteTextSummarisesFindingsBelowTheThreshold(t *testing.T) {
	r := Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{
		StageLint: ok(Outcome{New: append(manyInfos(55), Finding{Stage: StageLint, Code: "MDL098", Severity: "error", Document: "M.A", Message: "undefined", Key: "e", MxStage: -1})}),
	})
	out := textOf(r, "error")
	if n := strings.Count(out, "QUAL002"); n != 0 {
		t.Errorf("%d info findings were listed:\n%s", n, out)
	}
	for _, want := range []string{"MDL098", "56 new (55 below the failure threshold: 55 info)", "FAILED", "below --fail-on=error", "--stages lint --update-baseline"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if len(strings.Split(strings.TrimSpace(out), "\n")) > 8 {
		t.Errorf("output is %d lines for one failing finding:\n%s", len(strings.Split(out, "\n")), out)
	}
}

// The threshold follows --fail-on: a warning is listed when warnings fail the
// gate, and counted when they do not.
func TestWriteTextListsWarningsWhenTheyFail(t *testing.T) {
	w := Finding{Stage: StageLint, Code: "SEC003", Severity: "warning", Document: "ProjectSecurity", Message: "demo users", Key: "w", MxStage: -1}
	r := Run(Options{Project: "App.mpr", Stages: []string{StageLint}, FailOn: "warning"}, map[string]Stage{
		StageLint: ok(Outcome{New: append(manyInfos(3), w)}),
	})
	out := textOf(r, "warning")
	if !strings.Contains(out, "SEC003") || strings.Contains(out, "QUAL002") {
		t.Errorf("with --fail-on warning the warning is listed and the infos are not:\n%s", out)
	}
	// At the default threshold the same warning is below it: counted, not listed.
	r = Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{New: []Finding{w}})})
	out = textOf(r, "error")
	if strings.Contains(out, "SEC003") || !strings.Contains(out, "1 warning") || !strings.Contains(out, "passed") {
		t.Errorf("a warning under --fail-on error must be counted, not listed:\n%s", out)
	}
}

// A run with nothing below the threshold prints no hint, and the JSON keeps
// every finding whatever the text shows.
func TestWriteTextNoHintWhenNothingIsQuiet(t *testing.T) {
	r := Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{PreExisting: 4})})
	if out := textOf(r, "error"); strings.Contains(out, "--update-baseline") || !strings.Contains(out, "0 new, 4 pre-existing") {
		t.Errorf("unexpected output:\n%s", out)
	}
	r = Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{New: manyInfos(3)})})
	var b bytes.Buffer
	if err := WriteJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(b.String(), "QUAL002"); n != 3 {
		t.Errorf("JSON carries %d of the 3 findings", n)
	}
}

// Accepting the quiet findings of one stage must not also record the others:
// --update-baseline records every selected stage, so the hint names only the stage.
func TestWriteTextHintNamesOnlyTheStagesWithQuietFindings(t *testing.T) {
	r := Run(Options{Project: "App.mpr", Stages: AllStages, Skip: map[string]string{StageCheck: "no scripts given"}}, map[string]Stage{
		StageLint:    ok(Outcome{New: manyInfos(2)}),
		StageMxCheck: ok(Outcome{PreExisting: 1}),
	})
	out := textOf(r, "error")
	if !strings.Contains(out, "--stages lint --update-baseline") || strings.Contains(out, "mxcheck --update-baseline") || strings.Contains(out, "lint,mxcheck") {
		t.Errorf("the hint must scope --update-baseline to lint:\n%s", out)
	}
}
