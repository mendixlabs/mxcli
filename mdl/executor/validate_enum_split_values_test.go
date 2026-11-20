// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// An enumeration split (`case $S/C when R … end case`) needs exactly one branch per
// enum value plus (empty), and each value must exist. Measured on MxBuild 11.16.1,
// one flow per project (a shared project is wrong: CE1613 is an unresolved
// reference and hides every other error), `check` and `exec` accepting every
// rejected case:
//
//	every value + (empty), or several values in one branch   clean
//	a value left out (attribute subject, parameter subject)  CE0079
//	the same on a nanoflow                                   CE0079
//	a value that is not in the enumeration (X)               CE1613
//	the right value in another case (r for R)                CE1613   (values are case-sensitive)
//	a value in two branches, also inside a multi-value one   CE0771   (one per branch)
//
// CE0773 does not appear for any of these on 11.16.1; it is the build's follow-on
// error for a branch with no configured value, which an `else` produces and the
// grammar already refuses.

func enumSplitFlow(kind, subject, params, branches string) string {
	return "mdl 1;\ncreate " + kind + " Es.F (" + params + ") returns Integer as $N\nbegin\n  declare $N Integer = 0;\n  case " +
		subject + "\n" + branches + "\n  end case;\n  return $N;\nend;\n"
}

func enumSplitDuplicateHits(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	var vs []linter.Violation
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		vs = ValidateMicroflow(s)
	case *ast.CreateNanoflowStmt:
		vs = ValidateNanoflow(s)
	}
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == enumSplitDuplicateRule {
			out = append(out, v)
		}
	}
	return out
}

func TestEnumSplitDuplicateValues(t *testing.T) {
	cases := []struct {
		name, branches string
		want           int
	}{
		{"every value once (control)", "    when R then set $N = 1;\n    when G then set $N = 2;\n    when (empty) then set $N = 4;", 0},
		{"the same value in two branches", "    when R then set $N = 1;\n    when R then set $N = 2;\n    when (empty) then set $N = 4;", 1},
		{"a value repeated inside a multi-value branch's sibling", "    when R, G then set $N = 1;\n    when G then set $N = 2;\n    when (empty) then set $N = 4;", 1},
		{"multi-value branches that do not overlap (control)", "    when R, G then set $N = 1;\n    when B then set $N = 2;\n    when (empty) then set $N = 4;", 0},
		{"case differs, so not a duplicate (control)", "    when r then set $N = 1;\n    when R then set $N = 2;\n    when (empty) then set $N = 4;", 0},
	}
	for _, kind := range []string{"microflow", "nanoflow"} {
		for _, c := range cases {
			t.Run(kind+": "+c.name, func(t *testing.T) {
				got := enumSplitDuplicateHits(t, enumSplitFlow(kind, "$S/C", "$S: Es.Shape", c.branches))
				if len(got) != c.want {
					t.Fatalf("violations=%v, want %d", got, c.want)
				}
				if c.want > 0 && (!strings.Contains(got[0].Message, "CE0771") || !strings.Contains(got[0].Message, "G") && !strings.Contains(got[0].Message, "R")) {
					t.Errorf("message should name CE0771 and the value: %s", got[0].Message)
				}
			})
		}
	}
}

const enumSplitScript = `mdl 1;
create enumeration MyFirstModule.Col (R 'R', G 'G', B 'B');
create persistent entity MyFirstModule.Shape (C: Enumeration(MyFirstModule.Col));
`

func enumValueHits(t *testing.T, exec *Executor, src string) []linter.Violation {
	t.Helper()
	var out []linter.Violation
	for _, v := range typeCheck(t, exec, src) {
		if v.RuleID == enumSplitValueRule {
			out = append(out, v)
		}
	}
	return out
}

func esFlow(kind, subject, params, branches string) string {
	return strings.Replace(enumSplitFlow(kind, subject, params, branches), "mdl 1;\n", "", 1)
}

func TestEnumSplitValues_ScriptEnumeration(t *testing.T) {
	exec := typeCheckFixture(t)
	const S = "$S: MyFirstModule.Shape"
	const P = "$P: MyFirstModule.Col"
	all := "    when R then set $N = 1;\n    when G then set $N = 2;\n    when B then set $N = 3;\n    when (empty) then set $N = 4;"
	cases := []struct {
		name, kind, subject, params, branches string
		want                                  int
		code                                  string
	}{
		{"every value and (empty) (control)", "microflow", "$S/C", S, all, 0, ""},
		{"several values in one branch (control)", "microflow", "$S/C", S,
			"    when R, G then set $N = 1;\n    when B then set $N = 3;\n    when (empty) then set $N = 4;", 0, ""},
		{"a value left out", "microflow", "$S/C", S,
			"    when R then set $N = 1;\n    when G then set $N = 2;\n    when (empty) then set $N = 4;", 1, "CE0079"},
		{"two values left out: one violation naming both", "microflow", "$S/C", S,
			"    when R then set $N = 1;\n    when (empty) then set $N = 4;", 1, "CE0079"},
		{"a value left out on a nanoflow", "nanoflow", "$S/C", S,
			"    when R then set $N = 1;\n    when G then set $N = 2;\n    when (empty) then set $N = 4;", 1, "CE0079"},
		{"a value that is not in the enumeration", "microflow", "$S/C", S,
			all + "\n    when X then set $N = 5;", 1, "CE1613"},
		{"the right value in another case", "microflow", "$S/C", S,
			"    when r then set $N = 1;\n    when G then set $N = 2;\n    when B then set $N = 3;\n    when (empty) then set $N = 4;", 1, "CE1613"},
		{"an unknown value is reported alone, as the build does", "microflow", "$S/C", S,
			"    when R then set $N = 1;\n    when X then set $N = 5;\n    when (empty) then set $N = 4;", 1, "CE1613"},
		{"a parameter typed by the enumeration, all values (control)", "microflow", "$P", P, all, 0, ""},
		{"a parameter typed by the enumeration, a value left out", "microflow", "$P", P,
			"    when R then set $N = 1;\n    when G then set $N = 2;\n    when (empty) then set $N = 4;", 1, "CE0079"},
		{"a subject nothing resolves is not judged (control)", "microflow", "$Z/C", S,
			"    when R then set $N = 1;\n    when (empty) then set $N = 4;", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := enumSplitScript + esFlow(c.kind, c.subject, c.params, c.branches)
			got := enumValueHits(t, exec, src)
			if len(got) != c.want {
				t.Fatalf("violations=%v, want %d", got, c.want)
			}
			if c.want > 0 {
				v := got[0]
				if !strings.Contains(v.Message, c.code) || v.Severity != linter.SeverityError {
					t.Errorf("want an error naming %s: %+v", c.code, v)
				}
			}
		})
	}
	// Both left-out values are named, in one message.
	got := enumValueHits(t, exec, enumSplitScript+esFlow("microflow", "$S/C", S,
		"    when R then set $N = 1;\n    when (empty) then set $N = 4;"))
	if len(got) != 1 || !strings.Contains(got[0].Message, "G") || !strings.Contains(got[0].Message, "B") {
		t.Errorf("both missing values should be named once: %v", got)
	}
}

// The members of an enumeration the script ALTERS or DROPS are not the ones the
// reader holds, so a split on it is not judged.
func TestEnumSplitValues_AlteredEnumerationIsNotJudged(t *testing.T) {
	exec := typeCheckFixture(t)
	src := enumSplitScript + "alter enumeration MyFirstModule.Col add value Y caption 'Y';\n" +
		esFlow("microflow", "$S/C", "$S: MyFirstModule.Shape",
			"    when R then set $N = 1;\n    when (empty) then set $N = 4;")
	if got := enumValueHits(t, exec, src); len(got) != 0 {
		t.Errorf("an altered enumeration must not be judged: %v", got)
	}
}

// The enumeration and entity live in the PROJECT (the fixture's Ticket.Status is
// OrderStatus(Open, Closed)).
func TestEnumSplitValues_ProjectEnumeration(t *testing.T) {
	exec := typeCheckFixture(t)
	src := func(branches string) string {
		return esFlow("microflow", "$T/Status", "$T: MyFirstModule.Ticket", branches)
	}
	if got := enumValueHits(t, exec, src("    when Open then set $N = 1;\n    when Closed then set $N = 2;\n    when (empty) then set $N = 4;")); len(got) != 0 {
		t.Errorf("control: every project value covered was reported: %v", got)
	}
	got := enumValueHits(t, exec, src("    when Open then set $N = 1;\n    when (empty) then set $N = 4;"))
	if len(got) != 1 || !strings.Contains(got[0].Message, "Closed") || !strings.Contains(got[0].Message, "CE0079") {
		t.Errorf("a project enumeration value left out: want one CE0079 naming Closed, got %v", got)
	}
}
