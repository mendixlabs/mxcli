// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL098: a `$name` used in an expression that nothing in the flow defines.
//
// Measured on MxBuild 11.16.1, one `mx check -j` over 55 flows: an undefined
// name is CE0109 "Undefined variable" at the activity or end event. MxBuild
// accepts a fixed set of names with no declaration — $currentUser,
// $currentSession, $latestError, $latestHttpResponse and $latestSoapFault in
// every position, $currentIndex and the loop iterator inside the loop, and
// $currentObject inside a filter predicate. $latestJavaException and a made-up
// $IteratorItem are undefined like any other name.
func undefinedVarViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, st := range prog.Statements {
		var vs []linter.Violation
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			vs = ValidateMicroflow(s)
		case *ast.CreateNanoflowStmt:
			vs = ValidateNanoflow(s)
		}
		for _, v := range vs {
			if v.RuleID == "MDL098" {
				out = append(out, v)
			}
		}
	}
	return out
}

func TestUndefinedVariable_MDL098(t *testing.T) {
	const hdr = "create microflow Mr.F ($B: Boolean, $O: Mr.Obj, $L: list of Mr.Obj) "
	cases := []struct {
		name string
		src  string
		want string // "" = no MDL098; else the undefined name expected in the message
	}{
		{"return", hdr + "returns Integer begin return $Nope + 1; end;", "Nope"},
		{"if condition", hdr + "begin if $Nope then declare $X Integer = 1; end if; end;", "Nope"},
		{"declare value", hdr + "begin declare $X Integer = $Nope; end;", "Nope"},
		{"set value", hdr + "begin declare $X Integer = 0; set $X = $Nope; end;", "Nope"},
		{"attribute path", hdr + "returns String begin return $Nope/Name; end;", "Nope"},
		{"create member value", hdr + "begin $N = create Mr.Obj (Name = $Nope); end;", "Nope"},
		{"change member value", hdr + "begin change $O (Name = $Nope); end;", "Nope"},
		{"log message", hdr + "begin log info node 'X' $Nope; end;", "Nope"},
		{"nanoflow", "create nanoflow Mr.N ($B: Boolean) returns Integer begin return $Nope; end;", "Nope"},
		{"latestJavaException is not implicit", hdr + "begin declare $X Boolean = ($latestJavaException = empty); end;", "latestJavaException"},
		{"made-up iterator name", hdr + "begin loop $Item in $L begin declare $T Boolean = ($IteratorItem = empty); end loop; end;", "IteratorItem"},

		{"parameter", hdr + "returns Boolean begin return $B; end;", ""},
		{"parameter attribute", hdr + "returns String begin return $O/Name; end;", ""},
		{"declared variable", hdr + "returns Integer begin declare $X Integer = 1; return $X + 1; end;", ""},
		{"defined only in a branch is not this rule's", hdr + "returns Integer begin if $B then declare $X Integer = 1; end if; return $X; end;", ""},
		{"created object", hdr + "returns String begin $N = create Mr.Obj (Name = 'a'); return $N/Name; end;", ""},
		{"retrieved list", hdr + "returns Integer begin retrieve $R from Mr.Obj; return length($R); end;", ""},
		{"loop iterator in the loop", hdr + "begin loop $Item in $L begin declare $T String = $Item/Name; end loop; end;", ""},
		{"currentUser", hdr + "begin declare $T Boolean = ($currentUser = empty); end;", ""},
		{"currentSession", hdr + "begin declare $T Boolean = ($currentSession = empty); end;", ""},
		{"latestError", hdr + "begin declare $T Boolean = ($latestError = empty); end;", ""},
		{"latestHttpResponse", hdr + "begin declare $T Boolean = ($latestHttpResponse = empty); end;", ""},
		{"latestSoapFault", hdr + "begin declare $T Boolean = ($latestSoapFault = empty); end;", ""},
		{"currentIndex", hdr + "begin loop $Item in $L begin declare $T Integer = $currentIndex; end loop; end;", ""},
		{"currentObject", hdr + "begin $F = filter($L, $currentObject/Name = 'a'); end;", ""},
		{"name case differs, stays quiet", hdr + "returns Integer begin declare $Count Integer = 1; return $count; end;", ""},
		{"no variable at all", hdr + "returns Integer begin return 1 + 2; end;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := undefinedVarViolations(t, c.src)
			switch {
			case c.want == "" && len(got) > 0:
				t.Fatalf("unexpected MDL098: %s", got[0].Message)
			case c.want != "" && len(got) == 0:
				t.Fatalf("expected MDL098 for $%s, got none", c.want)
			case c.want != "" && !strings.Contains(got[0].Message, "$"+c.want):
				t.Fatalf("MDL098 message %q does not name $%s", got[0].Message, c.want)
			}
		})
	}
}

// An @excluded document is not built, so a name left undefined in one is not
// reported; the same flow without the annotation is (the control).
func TestUndefinedVariable_MDL098_Excluded(t *testing.T) {
	src := "create microflow Mr.X () returns Integer begin return $Nope; end;"
	if got := undefinedVarViolations(t, "@excluded\n"+src); len(got) != 0 {
		t.Errorf("an @excluded microflow must not be reported: %s", got[0].Message)
	}
	if got := undefinedVarViolations(t, src); len(got) == 0 {
		t.Errorf("control: the same microflow without @excluded must be reported")
	}
}
