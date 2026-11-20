// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func f(stage, key, sev string) Finding {
	return Finding{Stage: stage, Code: "X", Severity: sev, Key: key, MxStage: -1}
}

func mx(key string, mxStage int, unit string) Finding {
	return Finding{Stage: StageMxCheck, Code: "CE", Severity: "error", Key: key, MxStage: mxStage, Unit: unit}
}

// readOnly is the stage set of a run that does not apply scripts.
var readOnly = []string{StageCheck, StageLint, StageMxCheck}

func ok(o Outcome) Stage { return func() (Outcome, error) { return o, nil } }

func statuses(r Result) map[string]string {
	m := map[string]string{}
	for _, s := range r.Stages {
		m[s.Name] = s.Status
	}
	return m
}

func TestRunPassesWhenNothingIsNew(t *testing.T) {
	r := Run(Options{Stages: readOnly}, map[string]Stage{
		StageCheck: ok(Outcome{}), StageLint: ok(Outcome{PreExisting: 400}), StageMxCheck: ok(Outcome{PreExisting: 3}),
	})
	if r.Exit != ExitOK {
		t.Errorf("exit = %d, want 0", r.Exit)
	}
	for _, st := range r.Stages {
		if st.Name == StageLint && st.PreExisting != 400 {
			t.Errorf("lint pre-existing = %d", st.PreExisting)
		}
	}
}

// The control for the pass above: the same run with one new error must fail.
func TestRunFailsOnANewError(t *testing.T) {
	r := Run(Options{Stages: readOnly}, map[string]Stage{
		StageCheck: ok(Outcome{}), StageLint: ok(Outcome{New: []Finding{f(StageLint, "k", "error")}}), StageMxCheck: ok(Outcome{}),
	})
	if r.Exit != ExitFailedBase|stageBit[StageLint] {
		t.Errorf("exit = %d, want %d", r.Exit, ExitFailedBase|stageBit[StageLint])
	}
	if statuses(r)[StageLint] != StatusFailed {
		t.Errorf("lint status = %s", statuses(r)[StageLint])
	}
}

// By default every selected stage runs, so one result carries every stage's
// findings and the exit code is the bitmask of all that failed.
func TestRunAllStagesByDefault(t *testing.T) {
	bad := ok(Outcome{New: []Finding{f("", "k", "error")}})
	r := Run(Options{Stages: readOnly}, map[string]Stage{StageCheck: bad, StageLint: bad, StageMxCheck: bad})
	want := ExitFailedBase | 1 | 2 | 4
	if r.Exit != want {
		t.Errorf("exit = %d, want %d", r.Exit, want)
	}
}

func TestRunFailFastSkipsLaterStages(t *testing.T) {
	ran := map[string]bool{}
	mk := func(name string, o Outcome) Stage {
		return func() (Outcome, error) { ran[name] = true; return o, nil }
	}
	r := Run(Options{Stages: readOnly, FailFast: true}, map[string]Stage{
		StageCheck: mk(StageCheck, Outcome{New: []Finding{f("", "k", "error")}}),
		StageLint:  mk(StageLint, Outcome{}), StageMxCheck: mk(StageMxCheck, Outcome{}),
	})
	if ran[StageLint] || ran[StageMxCheck] {
		t.Errorf("stages ran after a failure with --fail-fast: %v", ran)
	}
	if statuses(r)[StageLint] != StatusSkipped {
		t.Errorf("lint status = %s, want skipped", statuses(r)[StageLint])
	}
	if r.Exit != ExitFailedBase|1 {
		t.Errorf("exit = %d", r.Exit)
	}
}

// A stage that cannot run is never "passed", and its exit code is not a failure's.
func TestRunStageThatCannotRunIsAnError(t *testing.T) {
	r := Run(Options{Stages: readOnly}, map[string]Stage{
		StageCheck: ok(Outcome{}), StageLint: ok(Outcome{}),
		StageMxCheck: func() (Outcome, error) { return Outcome{}, errors.New("mx not found") },
	})
	if statuses(r)[StageMxCheck] != StatusError || r.Exit != ExitCouldNotRun {
		t.Errorf("status = %s, exit = %d", statuses(r)[StageMxCheck], r.Exit)
	}
	if r.Exit == ExitOK {
		t.Error("a gate that could not run must not exit 0")
	}
}

func TestRunSkipsUnselectedAndEmptyStages(t *testing.T) {
	r := Run(Options{Stages: []string{StageLint}, Skip: map[string]string{StageCheck: "no scripts given"}},
		map[string]Stage{StageLint: ok(Outcome{})})
	st := statuses(r)
	if st[StageLint] != StatusPassed || st[StageCheck] != StatusSkipped || st[StageMxCheck] != StatusSkipped {
		t.Errorf("statuses = %v", st)
	}
}

func TestFailOnWarning(t *testing.T) {
	w := ok(Outcome{New: []Finding{f(StageLint, "k", "warning")}})
	stages := map[string]Stage{StageCheck: ok(Outcome{}), StageLint: w, StageMxCheck: ok(Outcome{})}
	if r := Run(Options{Stages: readOnly}, stages); r.Exit != ExitOK {
		t.Errorf("a new warning failed the gate by default (exit %d)", r.Exit)
	}
	if r := Run(Options{Stages: readOnly, FailOn: "warning"}, stages); r.Exit == ExitOK {
		t.Error("--fail-on warning did not fail on a new warning")
	}
}

func TestStrictFailsOnUnmasked(t *testing.T) {
	u := ok(Outcome{Unmasked: []Finding{mx("k", 3, "u1")}})
	stages := map[string]Stage{StageCheck: ok(Outcome{}), StageLint: ok(Outcome{}), StageMxCheck: u}
	if r := Run(Options{Stages: readOnly}, stages); r.Exit != ExitOK {
		t.Errorf("unmasked failed the gate without --strict (exit %d)", r.Exit)
	}
	if r := Run(Options{Stages: readOnly, Strict: true}, stages); r.Exit != ExitFailedBase|4 {
		t.Errorf("--strict exit = %d", r.Exit)
	}
}

func TestParseStages(t *testing.T) {
	if got, err := ParseStages([]string{"lint", " mxcheck", "lint"}); err != nil || len(got) != 2 {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := ParseStages([]string{"smoke"}); err == nil {
		t.Error("an unknown stage must be refused")
	}
}

// --- classification -------------------------------------------------------

func TestClassifyPlain(t *testing.T) {
	base := []Finding{f(StageLint, "a", "error"), f(StageLint, "a", "error"), f(StageLint, "gone", "error")}
	cur := []Finding{f(StageLint, "a", "error"), f(StageLint, "a", "error"), f(StageLint, "a", "error"), f(StageLint, "b", "error")}
	o := Classify(base, cur, false)
	if len(o.New) != 2 || o.PreExisting != 2 || o.Resolved != 1 {
		t.Errorf("new=%d pre=%d resolved=%d, want 2/2/1 (a third 'a' is new, multiplicity counts)", len(o.New), o.PreExisting, o.Resolved)
	}
}

// Fixing a stage 0 error (an unresolved reference) uncovers every later-stage
// error in the project, none of them the change's: unmasked, not new.
func TestClassifyUnmaskedByStage0(t *testing.T) {
	base := []Finding{mx("ref", 0, "flow9")}
	cur := []Finding{mx("type", 3, "flow1"), mx("undef", 2, "flow2")}
	o := Classify(base, cur, true)
	if len(o.Unmasked) != 2 || len(o.New) != 0 || o.Resolved != 1 {
		t.Errorf("unmasked=%d new=%d resolved=%d, want 2/0/1", len(o.Unmasked), len(o.New), o.Resolved)
	}
}

// An earlier-stage error of the same microflow can only vanish if the change
// edited that microflow, so a later error uncovered there is new: the change
// must leave the flow clean. (Seen on real mx output: replacing a duplicate
// variable with a type error passed as "unmasked" before this rule.)
func TestClassifySameUnitUncoveredErrorIsNew(t *testing.T) {
	base := []Finding{mx("dup", 1, "flow1")}
	cur := []Finding{mx("type", 3, "flow1")}
	o := Classify(base, cur, true)
	if len(o.New) != 1 || len(o.Unmasked) != 0 || o.Resolved != 1 {
		t.Errorf("new=%d unmasked=%d resolved=%d, want 1/0/1", len(o.New), len(o.Unmasked), o.Resolved)
	}
}

// The control for unmasked: with no stage 0 error in the baseline, a new
// later-stage error in another flow is new too.
func TestClassifyNewWhenNothingHidIt(t *testing.T) {
	base := []Finding{mx("dup", 1, "flow1")}
	cur := []Finding{mx("dup", 1, "flow1"), mx("type", 3, "flow2")}
	o := Classify(base, cur, true)
	if len(o.New) != 1 || len(o.Unmasked) != 0 {
		t.Errorf("new=%d unmasked=%d, want 1/0: flow2 was not hidden by a flow1 error", len(o.New), len(o.Unmasked))
	}
	// Without staging the stage 0 case is new as well, so the rule is what makes it unmasked.
	if o := Classify([]Finding{mx("ref", 0, "f")}, []Finding{mx("type", 3, "g")}, false); len(o.New) != 1 {
		t.Errorf("without staging a plain diff calls it new, got %d", len(o.New))
	}
}

// A change that introduces a stage 0 error hides everything else: the other
// findings are hidden, not resolved.
func TestClassifyHiddenNotResolved(t *testing.T) {
	base := []Finding{mx("a", 3, "f1"), mx("b", 2, "f2")}
	cur := []Finding{mx("ref", 0, "f3")}
	o := Classify(base, cur, true)
	if o.Hidden != 2 || o.Resolved != 0 || len(o.New) != 1 {
		t.Errorf("hidden=%d resolved=%d new=%d, want 2/0/1", o.Hidden, o.Resolved, len(o.New))
	}
	if o := Classify(base, nil, true); o.Resolved != 2 || o.Hidden != 0 {
		t.Errorf("with nothing hiding them they are resolved: resolved=%d hidden=%d", o.Resolved, o.Hidden)
	}
}

func TestMxBaselineRoundTripAndCompat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "mxcheck.json")
	if b, err := LoadMxBaseline(p); b != nil || err != nil {
		t.Fatalf("a missing baseline must be (nil, nil), got %v, %v", b, err)
	}
	if err := WriteMxBaseline(p, "mx check", "11.16.1", []Finding{mx("b", 3, "u"), mx("a", 2, "u")}); err != nil {
		t.Fatal(err)
	}
	b, err := LoadMxBaseline(p)
	if err != nil || len(b.Findings) != 2 || b.Findings[0].Key != "a" {
		t.Fatalf("round trip = %+v, %v", b, err)
	}
	if err := b.CheckCompatible("mx check", "11.16.1"); err != nil {
		t.Errorf("same checker refused: %v", err)
	}
	if b.CheckCompatible("mendix check", "11.16.1") == nil || b.CheckCompatible("mx check", "11.12.1") == nil {
		t.Error("another checker or mx version must be refused")
	}
}

// --- a stage that ran but cannot vouch for its result ----------------------------

// A lint run in which the baseline's rules did not run finds nothing new, and
// "nothing new" would mean "nothing was checked": it is an error (exit 2), not a
// pass. The control is the same run without the incompleteness, which passes.
func TestIncompleteStageIsAnErrorNotAPass(t *testing.T) {
	stages := func(incomplete string) map[string]Stage {
		return map[string]Stage{
			StageCheck: ok(Outcome{}), StageMxCheck: ok(Outcome{}),
			StageLint: ok(Outcome{PreExisting: 3, Incomplete: incomplete}),
		}
	}
	if r := Run(Options{Stages: readOnly}, stages("")); r.Exit != ExitOK {
		t.Fatalf("control: exit = %d, want 0", r.Exit)
	}
	r := Run(Options{Stages: readOnly}, stages("60 finding(s) of SEC007 were not checked"))
	if r.Exit != ExitCouldNotRun || statuses(r)[StageLint] != StatusError {
		t.Errorf("exit = %d, lint status = %s; want %d and error", r.Exit, statuses(r)[StageLint], ExitCouldNotRun)
	}
	for _, s := range r.Stages {
		if s.Name == StageLint && !strings.Contains(s.Note, "not checked") {
			t.Errorf("the note must say what was not checked: %q", s.Note)
		}
	}
}

// A stage that is incomplete AND has a new error still fails: the failure is the
// actionable result, and the note keeps the incompleteness.
func TestIncompleteStageWithNewErrorsStillFails(t *testing.T) {
	r := Run(Options{Stages: readOnly}, map[string]Stage{
		StageCheck: ok(Outcome{}), StageMxCheck: ok(Outcome{}),
		StageLint: ok(Outcome{New: errFinding, Incomplete: "SEC007 did not run"}),
	})
	if r.Exit != ExitFailedBase|stageBit[StageLint] {
		t.Errorf("exit = %d, want lint's failure bit %d", r.Exit, ExitFailedBase|stageBit[StageLint])
	}
	for _, s := range r.Stages {
		if s.Name == StageLint && !strings.Contains(s.Note, "SEC007 did not run") {
			t.Errorf("note = %q", s.Note)
		}
	}
}

// --- --apply: the dependency rule, and --through -----------------------------

func applyOpts(through ...string) Options {
	return Options{Project: "App.mpr", Stages: through, Apply: true, ScriptsGiven: true}
}

func recording(ran map[string]bool, name string, o Outcome) Stage {
	return func() (Outcome, error) { ran[name] = true; return o, nil }
}

var errFinding = []Finding{{Stage: "x", Code: "E", Severity: "error", Key: "e", MxStage: -1}}

// The happy path: every stage runs in pipeline order and the result says the
// scripts were applied.
func TestApplyRunsCheckExecLintMxcheck(t *testing.T) {
	ran := map[string]bool{}
	r := Run(applyOpts(AllStages...), map[string]Stage{
		StageCheck: recording(ran, StageCheck, Outcome{}), StageExec: recording(ran, StageExec, Outcome{Applied: true}),
		StageLint: recording(ran, StageLint, Outcome{}), StageMxCheck: recording(ran, StageMxCheck, Outcome{}),
	})
	for _, n := range AllStages {
		if !ran[n] {
			t.Errorf("%s did not run", n)
		}
	}
	if !r.Applied || r.Exit != ExitOK || len(r.Notes) != 0 {
		t.Errorf("applied=%v exit=%d notes=%v", r.Applied, r.Exit, r.Notes)
	}
}

// A failed check writes nothing: exec and everything after it are skipped, and the
// reason says so. The control is the previous test: the same stages run when check passes.
func TestApplyFailedCheckSkipsExecAndEverythingAfter(t *testing.T) {
	ran := map[string]bool{}
	r := Run(applyOpts(AllStages...), map[string]Stage{
		StageCheck: recording(ran, StageCheck, Outcome{New: errFinding}), StageExec: recording(ran, StageExec, Outcome{Applied: true}),
		StageLint: recording(ran, StageLint, Outcome{}), StageMxCheck: recording(ran, StageMxCheck, Outcome{}),
	})
	if ran[StageExec] || ran[StageLint] || ran[StageMxCheck] {
		t.Errorf("stages ran after a failed check: %v", ran)
	}
	st := statuses(r)
	if st[StageExec] != StatusSkipped || r.Applied || r.Exit != ExitFailedBase|stageBit[StageCheck] {
		t.Errorf("statuses=%v applied=%v exit=%d", st, r.Applied, r.Exit)
	}
	for _, s := range r.Stages {
		if s.Name == StageExec && !strings.Contains(s.Note, "nothing was written") {
			t.Errorf("exec note = %q", s.Note)
		}
	}
}

// A script exec refused is not applied, so lint and mxcheck would judge the
// project without it: they are skipped and the failure is exec's bit.
func TestApplyFailedExecSkipsLintAndMxcheck(t *testing.T) {
	ran := map[string]bool{}
	r := Run(applyOpts(AllStages...), map[string]Stage{
		StageCheck: recording(ran, StageCheck, Outcome{}), StageExec: recording(ran, StageExec, Outcome{New: errFinding}),
		StageLint: recording(ran, StageLint, Outcome{}), StageMxCheck: recording(ran, StageMxCheck, Outcome{}),
	})
	if ran[StageLint] || ran[StageMxCheck] {
		t.Errorf("lint or mxcheck ran after a failed exec: %v", ran)
	}
	if r.Applied || r.Exit != ExitFailedBase|stageBit[StageExec] {
		t.Errorf("applied=%v exit=%d, want exec's bit %d", r.Applied, r.Exit, ExitFailedBase|stageBit[StageExec])
	}
}

// Without --apply the stages stay independent: a failed check does not stop lint
// and mxcheck, which describe the project as it is, and the result says the
// script was not applied. The control for the two tests above.
func TestWithoutApplyStagesStayIndependentAndSaySo(t *testing.T) {
	ran := map[string]bool{}
	r := Run(Options{Project: "App.mpr", Stages: []string{StageCheck, StageLint, StageMxCheck}, ScriptsGiven: true,
		Skip: map[string]string{StageExec: "scripts are only applied with --apply"}}, map[string]Stage{
		StageCheck: recording(ran, StageCheck, Outcome{New: errFinding}),
		StageLint:  recording(ran, StageLint, Outcome{}), StageMxCheck: recording(ran, StageMxCheck, Outcome{}),
	})
	if !ran[StageLint] || !ran[StageMxCheck] {
		t.Errorf("lint and mxcheck must still run without --apply: %v", ran)
	}
	if r.Applied || len(r.Notes) != 1 || r.Notes[0] != ScriptsNotAppliedNote {
		t.Errorf("applied=%v notes=%v", r.Applied, r.Notes)
	}
	// No scripts, no note.
	if r := Run(Options{Stages: []string{StageLint}}, map[string]Stage{StageLint: ok(Outcome{})}); len(r.Notes) != 0 {
		t.Errorf("notes without scripts: %v", r.Notes)
	}
}

func TestThroughStopsWhereAsked(t *testing.T) {
	for last, want := range map[string]string{
		"check": "check", "exec": "check,exec", "lint": "check,exec,lint", "mxcheck": "check,exec,lint,mxcheck",
	} {
		got, err := Through(last)
		if err != nil || strings.Join(got, ",") != want {
			t.Errorf("Through(%q) = %v, %v; want %s", last, got, err, want)
		}
	}
	if _, err := Through("test"); err == nil {
		t.Error("a stage that does not exist yet must be refused")
	}
}

// An exec that could not run at all (a failed start) also blocks the stages after
// it, and is a "could not run", not a failure.
func TestApplyExecThatCannotRunBlocksTheRest(t *testing.T) {
	ran := map[string]bool{}
	r := Run(applyOpts(AllStages...), map[string]Stage{
		StageCheck: recording(ran, StageCheck, Outcome{}),
		StageExec:  func() (Outcome, error) { return Outcome{}, errors.New("cannot start") },
		StageLint:  recording(ran, StageLint, Outcome{}), StageMxCheck: recording(ran, StageMxCheck, Outcome{}),
	})
	if ran[StageLint] || ran[StageMxCheck] || r.Exit != ExitCouldNotRun {
		t.Errorf("ran=%v exit=%d", ran, r.Exit)
	}
}
