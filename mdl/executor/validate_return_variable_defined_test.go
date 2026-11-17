// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL097: `returns T as $R` whose body can run off the end without ever giving
// $R a value. MDL003 exempts the AS clause because the End event returns $R,
// which is right only while $R exists on that path.
//
// Measured on MxBuild 11.16.1, one `mx check -j` over 15 flows (microflow and
// nanoflow alike):
//
//	$R never defined, a path falls off the end   CE0109 Undefined variable 'R' (End event)
//	$R defined only inside a branch or loop      CE0108 defined but not in scope (End event)
//	$R declared at the top level                 0 errors, falling off the end or not
//	$R never defined, every path returns         0 errors
//	no AS clause, a path falls off the end       CE0067 (MDL003's, unchanged)
func returnVarViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, st := range prog.Statements {
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			out = append(out, ValidateMicroflow(s)...)
		case *ast.CreateNanoflowStmt:
			out = append(out, ValidateNanoflow(s)...)
		}
	}
	return out
}

func TestReturnVariableDefined_MDL097(t *testing.T) {
	const hdr = "create microflow Mr.F ($B: Boolean) "
	cases := []struct {
		name string
		src  string
		want string // "" = no MDL097; otherwise a substring of the message
	}{
		{"never defined, falls off", hdr + "returns Integer as $R begin declare $X Integer = 1; end;", "never defined"},
		{"never defined, if without else", hdr + "returns Integer as $R begin if $B then return 1; end if; end;", "never defined"},
		{"never defined, object", hdr + "returns Mr.Obj as $R begin declare $X Integer = 1; end;", "never defined"},
		{"defined only inside a branch", hdr + "returns Integer as $R begin if $B then declare $R Integer = 1; end if; end;", "inside a branch"},
		{"declared at the top, falls off", hdr + "returns Integer as $R begin declare $R Integer = 0; if $B then set $R = 1; end if; end;", ""},
		{"declared at the top, if returns", hdr + "returns Integer as $R begin declare $R Integer = 0; if $B then return 1; end if; end;", ""},
		{"declared after the if", hdr + "returns Integer as $R begin if $B then return 1; end if; declare $R Integer = 5; end;", ""},
		{"never defined, every path returns", hdr + "returns Integer as $R begin if $B then return 1; else return 2; end if; end;", ""},
		{"created at the top", hdr + "returns Mr.Obj as $R begin $R = create Mr.Obj (Name = 'x'); end;", ""},
		{"no AS clause is MDL003's, not this rule", hdr + "returns Integer begin declare $X Integer = 1; end;", ""},
		{"returns nothing", hdr + "begin declare $X Integer = 1; end;", ""},
		{"nanoflow never defined", "create nanoflow Mr.N ($B: Boolean) returns Integer as $R begin declare $X Integer = 1; end;", "never defined"},
		{"nanoflow declared at the top", "create nanoflow Mr.N ($B: Boolean) returns Integer as $R begin declare $R Integer = 0; end;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got *linter.Violation
			for _, v := range returnVarViolations(t, c.src) {
				if v.RuleID == "MDL097" {
					v := v
					got = &v
				}
			}
			switch {
			case c.want == "" && got != nil:
				t.Fatalf("unexpected MDL097: %s", got.Message)
			case c.want != "" && got == nil:
				t.Fatalf("expected MDL097 containing %q, got none", c.want)
			case c.want != "" && !strings.Contains(got.Message, c.want):
				t.Fatalf("MDL097 message %q does not contain %q", got.Message, c.want)
			}
		})
	}
}

// A parameter of the same name defines $R, and an @excluded document is not
// built, so neither is reported.
func TestReturnVariableDefined_MDL097_Exemptions(t *testing.T) {
	param := returnVarViolations(t, "create microflow Mr.P ($R: Integer) returns Integer as $R begin declare $X Integer = 1; end;")
	for _, v := range param {
		if v.RuleID == "MDL097" {
			t.Errorf("a parameter named like the return variable defines it: %s", v.Message)
		}
	}
}

// mxbuild does not check an @excluded document, so a flow stashed there with a
// broken return is not reported (the rule stands down like MDL061-063).
func TestReturnVariableDefined_MDL097_Excluded(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow"} {
		src := "@excluded\ncreate " + kind + " Mr.X ($B: Boolean) returns Integer as $R begin declare $X Integer = 1; end;"
		var any097 bool
		for _, v := range returnVarViolations(t, src) {
			if v.RuleID == "MDL097" {
				any097 = true
			}
		}
		if any097 {
			t.Errorf("an @excluded %s must not be reported", kind)
		}
		// Control: the same flow without @excluded is reported.
		ctl := returnVarViolations(t, strings.TrimPrefix(src, "@excluded\n"))
		found := false
		for _, v := range ctl {
			if v.RuleID == "MDL097" {
				found = true
			}
		}
		if !found {
			t.Errorf("control: the same %s without @excluded must be reported", kind)
		}
	}
}
