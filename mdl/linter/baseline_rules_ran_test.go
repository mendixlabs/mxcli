// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2454 and #2455: a baseline holds findings, but not which rules produced
// them. A run in which a rule did not exist (a checkout without .claude/lint-rules/)
// or could not run reported none of its findings, and the comparison counted every
// one as fixed, so the gate passed on a project it had stopped checking.

func recordWith(t *testing.T, findings []Violation, meta BaselineMeta) *Baseline {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lint-baseline.json")
	if err := WriteBaselineWith(path, findings, meta); err != nil {
		t.Fatalf("WriteBaselineWith: %v", err)
	}
	b, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	return b
}

// The baseline of two rules; the run has only one of them.
func twoRuleBaseline(t *testing.T) (*Baseline, []Violation) {
	found := []Violation{
		v("MPR008", "Sales", "ACT_A", "1", "overlap"),
		v("SEC007", "Sales", "ACT_B", "2", "guest can write"),
		v("SEC007", "Sales", "ACT_C", "3", "guest can write"),
	}
	return recordWith(t, found, BaselineMeta{Rules: []string{"SEC007", "MPR008"}, Config: "cfg1"}), found
}

// The dangerous case from #2455: the custom rule is not there, its findings are
// not reported, and they are NOT fixed.
func TestCompareRunMissingRuleIsNotCheckedNotFixed(t *testing.T) {
	b, found := twoRuleBaseline(t)
	run := RunInfo{Known: true, Ran: []string{"MPR008"}, Config: "cfg1"}
	cmp := b.CompareRun(found[:1], run) // only MPR008 reports
	if cmp.Fixed != 0 || cmp.NotChecked != 2 || cmp.Unchanged != 1 || len(cmp.New) != 0 {
		t.Errorf("fixed=%d notChecked=%d unchanged=%d new=%d, want 0/2/1/0", cmp.Fixed, cmp.NotChecked, cmp.Unchanged, len(cmp.New))
	}
	if strings.Join(cmp.RulesNotRun, ",") != "SEC007" || !cmp.Incomplete() {
		t.Errorf("rulesNotRun=%v incomplete=%v", cmp.RulesNotRun, cmp.Incomplete())
	}
	// The control: with the rule present and the findings gone they ARE fixed.
	run.Ran = []string{"MPR008", "SEC007"}
	cmp = b.CompareRun(found[:1], run)
	if cmp.Fixed != 2 || cmp.NotChecked != 0 || cmp.Incomplete() {
		t.Errorf("fixed=%d notChecked=%d incomplete=%v, want 2/0/false", cmp.Fixed, cmp.NotChecked, cmp.Incomplete())
	}
}

// A rule that ran into an error is not new, and its findings are not fixed (#2454).
func TestCompareRunRuleFailureIsNotAFindingAndNotAFix(t *testing.T) {
	b, found := twoRuleBaseline(t)
	failure := Violation{RuleID: "SEC007", Severity: SeverityWarning, RuleFailure: true, Message: "SEC007 could not run"}
	run := RunInfo{Known: true, Ran: []string{"MPR008"}, Failed: []string{"SEC007"}, Config: "cfg1"}
	cmp := b.CompareRun([]Violation{found[0], failure}, run)
	if cmp.Fixed != 0 || cmp.NotChecked != 2 {
		t.Errorf("fixed=%d notChecked=%d, want 0/2", cmp.Fixed, cmp.NotChecked)
	}
	if strings.Join(cmp.RuleFailures, ",") != "SEC007" || !cmp.Incomplete() {
		t.Errorf("failures=%v incomplete=%v", cmp.RuleFailures, cmp.Incomplete())
	}
}

func TestCompareRunNamesAChangedConfigAndNewRules(t *testing.T) {
	b, found := twoRuleBaseline(t)
	run := RunInfo{Known: true, Ran: []string{"MPR008", "SEC007", "QUAL009"}, Config: "cfg2"}
	cmp := b.CompareRun(found, run)
	if !cmp.ConfigChanged || !cmp.Incomplete() {
		t.Errorf("configChanged=%v incomplete=%v", cmp.ConfigChanged, cmp.Incomplete())
	}
	if strings.Join(cmp.NewRules, ",") != "QUAL009" {
		t.Errorf("newRules=%v, want QUAL009", cmp.NewRules)
	}
	// The same config, the same rules: nothing to say.
	run = RunInfo{Known: true, Ran: []string{"MPR008", "SEC007"}, Config: "cfg1"}
	if cmp := b.CompareRun(found, run); cmp.Incomplete() || len(cmp.NewRules) != 0 || cmp.Unchanged != 3 {
		t.Errorf("a matching run was flagged: %+v", cmp)
	}
}

// A baseline written before rules were recorded still works: the rules it holds
// findings for are the ones that must have run, and nothing else is claimed.
func TestCompareRunOnABaselineWithoutRecordedRules(t *testing.T) {
	found := []Violation{v("MPR008", "Sales", "ACT_A", "1", "overlap"), v("SEC007", "Sales", "ACT_B", "2", "guest")}
	b := roundTrip(t, found)
	cmp := b.CompareRun(found[:1], RunInfo{Known: true, Ran: []string{"MPR008"}, Config: "anything"})
	if cmp.NotChecked != 1 || cmp.Fixed != 0 || strings.Join(cmp.RulesNotRun, ",") != "SEC007" {
		t.Errorf("notChecked=%d fixed=%d rulesNotRun=%v", cmp.NotChecked, cmp.Fixed, cmp.RulesNotRun)
	}
	if cmp.ConfigChanged || len(cmp.NewRules) != 0 {
		t.Errorf("claimed more than the baseline knows: %+v", cmp)
	}
}

// A run narrowed to some rules, modules or documents is expected to miss the rest.
func TestCompareRunScopedRunHoldsNothingAgainstTheRun(t *testing.T) {
	b, found := twoRuleBaseline(t)
	cmp := b.CompareRun(found[:1], RunInfo{Known: true, Scoped: true, Ran: []string{"MPR008"}, Config: "cfg2"})
	if cmp.Incomplete() || cmp.NotChecked != 0 {
		t.Errorf("a scoped run was flagged: %+v", cmp)
	}
}

// Without run information the comparison is what it always was.
func TestCompareWithoutRunInfoIsUnchanged(t *testing.T) {
	b, found := twoRuleBaseline(t)
	cmp := b.Compare(found[:1])
	if cmp.Fixed != 2 || cmp.NotChecked != 0 || cmp.Incomplete() {
		t.Errorf("fixed=%d notChecked=%d incomplete=%v", cmp.Fixed, cmp.NotChecked, cmp.Incomplete())
	}
}

// The new fields are optional: a file without them loads, and one with them
// round-trips sorted.
func TestBaselineRecordsRulesAndConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	if err := WriteBaselineWith(path, nil, BaselineMeta{Rules: []string{"SEC007", "MPR008"}, Config: "abc"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"rules": [`) || strings.Index(string(data), "MPR008") > strings.Index(string(data), "SEC007") ||
		!strings.Contains(string(data), `"config": "abc"`) {
		t.Errorf("baseline file:\n%s", data)
	}
	old := filepath.Join(t.TempDir(), "old.json")
	_ = os.WriteFile(old, []byte(`{"version": 1, "findings": []}`), 0o644)
	if b, err := LoadBaseline(old); err != nil || b.rulesRecorded {
		t.Errorf("an old baseline: %v, rulesRecorded=%v", err, b != nil && b.rulesRecorded)
	}
}

func TestConfigFingerprint(t *testing.T) {
	on := true
	a := &Config{ExcludeModules: []string{"B", "A"}, Rules: map[string]RuleConfigYAML{"X": {Enabled: &on, Severity: "warning", Options: map[string]any{"n": 3}}}}
	same := &Config{ExcludeModules: []string{"A", "B"}, Rules: map[string]RuleConfigYAML{"X": {Enabled: &on, Severity: "warning", Options: map[string]any{"n": 3}}}}
	if a.Fingerprint(nil) != same.Fingerprint(nil) {
		t.Error("the order of excluded modules must not change the fingerprint")
	}
	if a.Fingerprint(nil) != a.Fingerprint([]string{"A"}) {
		t.Error("excluding a module the config already excludes must not change the fingerprint")
	}
	for name, other := range map[string]*Config{
		"an option":   {ExcludeModules: []string{"A", "B"}, Rules: map[string]RuleConfigYAML{"X": {Enabled: &on, Severity: "warning", Options: map[string]any{"n": 4}}}},
		"a severity":  {ExcludeModules: []string{"A", "B"}, Rules: map[string]RuleConfigYAML{"X": {Enabled: &on, Severity: "error", Options: map[string]any{"n": 3}}}},
		"an exclude":  {ExcludeModules: []string{"A"}, Rules: a.Rules},
		"no config":   nil,
		"empty rules": {ExcludeModules: []string{"A", "B"}},
	} {
		if other.Fingerprint(nil) == a.Fingerprint(nil) {
			t.Errorf("a different %s has the same fingerprint", name)
		}
	}
	if a.Fingerprint([]string{"C"}) == a.Fingerprint(nil) {
		t.Error("a module excluded on the command line must change the fingerprint")
	}
}

type ranRule struct {
	id   string
	fail bool
}

func (r ranRule) ID() string                { return r.id }
func (r ranRule) Name() string              { return r.id }
func (r ranRule) Description() string       { return "" }
func (r ranRule) DefaultSeverity() Severity { return SeverityWarning }
func (r ranRule) Category() string          { return "test" }
func (r ranRule) Check(*LintContext) []Violation {
	if r.fail {
		return []Violation{{RuleID: r.id, Severity: SeverityWarning, RuleFailure: true, Message: r.id + " could not run"}}
	}
	return nil
}

func TestLinterReportsWhichRulesRan(t *testing.T) {
	l := New(NewLintContextFromDB(nil))
	for _, r := range []ranRule{{id: "B"}, {id: "A"}, {id: "F", fail: true}, {id: "OFF"}} {
		l.AddRule(r)
	}
	l.ConfigureRule("OFF", RuleConfig{Enabled: false})
	if _, err := l.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ran, failed := l.RulesRun()
	if strings.Join(ran, ",") != "A,B" || strings.Join(failed, ",") != "F" {
		t.Errorf("ran=%v failed=%v, want A,B and F (a disabled rule is in neither)", ran, failed)
	}
}

// Re-recording is how a failing gate becomes a passing one without fixing anything.
// It accepts only the errors the baseline does not already hold, and only on request.
func TestErrorsToAcceptAreTheNewErrors(t *testing.T) {
	errV := func(rule, doc, id string) Violation {
		x := v(rule, "Sales", doc, id, "broken")
		x.Severity = SeverityError
		return x
	}
	held := errV("SEC007", "ACT_Old", "1")
	b := recordWith(t, []Violation{held, v("QUAL002", "Sales", "ACT_W", "2", "no docs")}, BaselineMeta{Rules: []string{"SEC007", "QUAL002"}})
	run := RunInfo{Known: true, Ran: []string{"SEC007", "QUAL002"}}
	fresh := errV("SEC007", "ACT_New", "3")
	warn := v("QUAL002", "Sales", "ACT_W2", "4", "no docs")
	failure := Violation{RuleID: "MPR017", Severity: SeverityError, RuleFailure: true, Message: "could not run"}

	got := b.ErrorsToAccept([]Violation{held, fresh, warn, failure}, run)
	if len(got) != 1 || got[0].Location.DocumentID != "3" {
		t.Errorf("errors to accept = %v, want only the new error", messages(got))
	}
	// The control: a re-record with nothing new (the accepted error is still there)
	// accepts nothing, so it needs no flag.
	if got := b.ErrorsToAccept([]Violation{held, warn}, run); len(got) != 0 {
		t.Errorf("a re-record that adds no error would accept %v", messages(got))
	}
	// A first record (no baseline) accepts every error of the run, and none of the rest.
	var none *Baseline
	if got := none.ErrorsToAccept([]Violation{held, fresh, warn, failure}, run); len(got) != 2 {
		t.Errorf("a first record accepts %v, want the two errors", messages(got))
	}
	if got := none.ErrorsToAccept([]Violation{warn}, run); len(got) != 0 {
		t.Errorf("warnings are not errors to accept: %v", messages(got))
	}
}
