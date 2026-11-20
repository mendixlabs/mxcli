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
	r := Run(Options{Project: "App.mpr", Stages: readOnly, Skip: map[string]string{StageCheck: "no scripts given"}}, map[string]Stage{
		StageLint:    ok(Outcome{New: manyInfos(2)}),
		StageMxCheck: ok(Outcome{PreExisting: 1}),
	})
	out := textOf(r, "error")
	if !strings.Contains(out, "--stages lint --update-baseline") || strings.Contains(out, "mxcheck --update-baseline") || strings.Contains(out, "lint,mxcheck") {
		t.Errorf("the hint must scope --update-baseline to lint:\n%s", out)
	}
}

// A stage nobody asked for is not a line in the text output (the JSON lists it),
// but a stage skipped for a reason the reader needs is.
func TestWriteTextOmitsUnselectedStagesOnly(t *testing.T) {
	r := Run(Options{Project: "App.mpr", Stages: []string{StageCheck, StageLint}, Skip: map[string]string{StageCheck: "no scripts given"}},
		map[string]Stage{StageLint: ok(Outcome{})})
	out := textOf(r, "error")
	if strings.Contains(out, "mxcheck") || strings.Contains(out, "exec") {
		t.Errorf("unselected stages are listed:\n%s", out)
	}
	if !strings.Contains(out, "no scripts given") {
		t.Errorf("a stage skipped for a reason must still be listed:\n%s", out)
	}
	var b bytes.Buffer
	if err := WriteJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"name": "mxcheck"`) {
		t.Error("the JSON must keep every stage")
	}
}

// ako/mxcli#2468: with --apply the exec stage writes the project before mx check
// runs, so a later stage that fails leaves the failing change in the project. The
// text said "Result: FAILED" like any failed gate; it now says the scripts were
// applied.
func TestWriteTextSaysTheScriptsWereAppliedWhenALaterStageFails(t *testing.T) {
	fail := Finding{Stage: StageMxCheck, Code: "CE0117", Severity: "error", Document: "Shop.SNIP", Message: "Error(s) in expression.", Key: "k", MxStage: 0}
	r := Run(Options{Project: "App.mpr", Apply: true, ScriptsGiven: true, Stages: []string{StageExec, StageMxCheck}}, map[string]Stage{
		StageExec:    ok(Outcome{Applied: true}),
		StageMxCheck: ok(Outcome{New: []Finding{fail}}),
	})
	out := textOf(r, "error")
	for _, want := range []string{"FAILED", "The scripts were applied", "Undo it"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}

	// Controls: a pass, a failure before anything was written, and a run that
	// never applied say nothing about applied scripts.
	pass := Run(Options{Project: "App.mpr", Apply: true, ScriptsGiven: true, Stages: []string{StageExec, StageMxCheck}}, map[string]Stage{
		StageExec: ok(Outcome{Applied: true}), StageMxCheck: ok(Outcome{}),
	})
	notApplied := Run(Options{Project: "App.mpr", Stages: []string{StageMxCheck}}, map[string]Stage{
		StageMxCheck: ok(Outcome{New: []Finding{fail}}),
	})
	checkFailed := Run(Options{Project: "App.mpr", Apply: true, ScriptsGiven: true, Stages: []string{StageCheck, StageExec, StageMxCheck}}, map[string]Stage{
		StageCheck: ok(Outcome{New: []Finding{{Stage: StageCheck, Code: "MDL108", Severity: "error", Document: "Shop.E", Message: "m", Key: "c", MxStage: -1}}}),
		StageExec:  ok(Outcome{Applied: true}), StageMxCheck: ok(Outcome{}),
	})
	for name, r := range map[string]Result{"passed": pass, "not applied": notApplied, "check failed, nothing written": checkFailed} {
		if strings.Contains(textOf(r, "error"), "The scripts were applied") {
			t.Errorf("%s: must not claim the scripts were applied:\n%s", name, textOf(r, "error"))
		}
	}
}

// "63 pre-existing" reads the same whether they are notes or errors the baseline
// accepted. The summary says what they are, worst first.
func TestSummaryNamesWhatThePreExistingFindingsAre(t *testing.T) {
	r := Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{
		StageLint: ok(Outcome{PreExisting: 64, PreExistingBy: map[string]int{"error": 2, "warning": 1, "info": 60, "hint": 1}}),
	})
	out := textOf(r, "error")
	if !strings.Contains(out, "0 new, 64 pre-existing (2 errors, 1 warning, 60 info, 1 hint)") {
		t.Errorf("the breakdown is missing:\n%s", out)
	}
	var b bytes.Buffer
	if err := WriteJSON(&b, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"preExistingBySeverity"`) || !strings.Contains(b.String(), `"error": 2`) {
		t.Errorf("the JSON must carry the breakdown:\n%s", b.String())
	}
	// A stage that cannot say keeps the plain count, and zero findings say nothing.
	r = Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{PreExisting: 5})})
	if out := textOf(r, "error"); !strings.Contains(out, "0 new, 5 pre-existing\n") {
		t.Errorf("a stage without a breakdown changed:\n%s", out)
	}
	r = Run(Options{Project: "App.mpr", Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{})})
	if out := textOf(r, "error"); !strings.Contains(out, "0 new, 0 pre-existing\n") {
		t.Errorf("an empty stage changed:\n%s", out)
	}
}

// Classify counts the pre-existing findings by severity, so mxcheck says how many
// errors the baseline accepts on every run.
func TestClassifyCountsPreExistingBySeverity(t *testing.T) {
	e := mx("k1", 3, "u")
	w := Finding{Stage: StageMxCheck, Code: "CW", Severity: "warning", Key: "k2", MxStage: -1}
	o := Classify([]Finding{e, w}, []Finding{e, w, mx("new", 3, "u")}, true)
	if o.PreExisting != 2 || o.PreExistingBy["error"] != 1 || o.PreExistingBy["warning"] != 1 || len(o.New) != 1 {
		t.Errorf("pre=%d by=%v new=%d", o.PreExisting, o.PreExistingBy, len(o.New))
	}
}
