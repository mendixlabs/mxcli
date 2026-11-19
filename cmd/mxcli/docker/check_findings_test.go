// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

// The finding lines are the ones mx check printed in the repository's own
// records (docker/check.go's CE0463 comment, the fix-issue findings); the
// closing line is mx's.
const sampleCheck = `Checking project...
[error] [CE0463] "The definition of this widget has changed. ..." at Data grid 2 'dgRequests'
[error] [CE0463] "The definition of this widget has changed. ..." at Data grid 2 'dgRequests'
[error] [CE0117] "Error(s) in expression." at Microflow 'ACT_X'
[error] [CE0066] "Entity access is out of date…" at Domain model of module 'Administration'
[error] [CE0090] "The 'I913.Animal' value should be configured for an outgoing flow." at Object type decision ''
[error] [CE0109] "Variable has no value."
The app contains: 6 errors.
`

func TestParseCheckOutput(t *testing.T) {
	fs := ParseCheckOutput(sampleCheck)
	if len(fs) != 6 {
		t.Fatalf("parsed %d findings, want 6: %+v", len(fs), fs)
	}
	for _, c := range []struct {
		i           int
		code, kind  string
		name        string
		class       LocationClass
		stage       int
		messagePart string
	}{
		{0, "CE0463", "Data grid 2", "dgRequests", Unlocated, 3, "widget has changed"},
		{2, "CE0117", "Microflow", "ACT_X", DocumentOnly, 3, "Error(s) in expression."},
		{3, "CE0066", "Domain model of module", "Administration", DocumentOnly, 3, "Entity access"},
		{4, "CE0090", "Object type decision", "", Unlocated, 3, "'I913.Animal'"},
		{5, "CE0109", "", "", Unlocated, 2, "Variable has no value."},
	} {
		f := fs[c.i]
		if f.Code != c.code || f.Kind != c.kind || f.Name != c.name || f.Class != c.class || f.Stage != c.stage ||
			!strings.Contains(f.Message, c.messagePart) {
			t.Errorf("finding %d = %+v, want code %s kind %q name %q class %v stage %d message ~%q",
				c.i, f, c.code, c.kind, c.name, c.class, c.stage, c.messagePart)
		}
	}
}

// A finding on a widget with a code that does name its element is Located; the
// CE0463 override is what makes the same text Unlocated. This is the control for
// the table: without it every widget finding would pass as Unlocated.
func TestClassifyLocatedNeedsAnElementName(t *testing.T) {
	f := ParseCheckOutput(`[error] [CE0001] "msg" at Data grid 2 'dgRequests'`)[0]
	if f.Class != Located {
		t.Errorf("class = %v, want located", f.Class)
	}
	f = ParseCheckOutput(`[error] [CE0463] "msg" at Data grid 2 'dgRequests'`)[0]
	if f.Class != Unlocated {
		t.Errorf("CE0463 class = %v, want unlocated", f.Class)
	}
}

// The key ignores the message: the same finding reworded or on another line is
// one finding, which is the point of keying (the control: a different location
// is a different key).
func TestKeyIgnoresMessage(t *testing.T) {
	a := ParseCheckOutput(`[error] [CE0117] "one wording" at Microflow 'ACT_X'`)[0]
	b := ParseCheckOutput(`[error] [CE0117] "another wording" at Microflow 'ACT_X'`)[0]
	c := ParseCheckOutput(`[error] [CE0117] "one wording" at Microflow 'ACT_Y'`)[0]
	if a.Key() != b.Key() {
		t.Error("the same finding with a different message must have the same key")
	}
	if a.Key() == c.Key() {
		t.Error("a different location must have a different key")
	}
}

func TestStageOf(t *testing.T) {
	for code, want := range map[string]int{"CE1613": 0, "CE0111": 1, "CE0109": 2, "CE0067": 3, "CE9999": 3} {
		if got := StageOf(code); got != want {
			t.Errorf("StageOf(%s) = %d, want %d", code, got, want)
		}
	}
}

func TestParseCheckReportVerify(t *testing.T) {
	r := ParseCheckReport(sampleCheck)
	if !r.HasDeclared || r.Declared != 6 {
		t.Fatalf("declared = %d (%v), want 6", r.Declared, r.HasDeclared)
	}
	if err := r.Verify(); err != nil {
		t.Errorf("Verify() = %v on a complete report", err)
	}

	// Control: a format change that loses lines must be reported, not absorbed.
	lost := strings.Replace(sampleCheck, "[error] [CE0117]", "[err] [CE0117]", 1)
	if err := ParseCheckReport(lost).Verify(); err == nil {
		t.Error("Verify() = nil although a finding line was not parsed")
	}
	if err := ParseCheckReport("[error] [CE0117] \"x\" at Microflow 'M'\n").Verify(); err == nil {
		t.Error("Verify() = nil without mx's closing count")
	}
}

func TestParseCheckOutputKeepsSeverityAndCRLF(t *testing.T) {
	fs := ParseCheckOutput("[warning] [CW0001] \"w\" at Page 'P'\r\n[deprecation] [CD0001] \"d\"\r\n")
	if len(fs) != 2 || fs[0].Severity != "warning" || fs[1].Severity != "deprecation" {
		t.Fatalf("got %+v", fs)
	}
}
