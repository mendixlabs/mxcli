// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// CE0023 "Selected variable 'X' must be of type Object or List." Measured on
// MxBuild 11.16.1, microflows and nanoflows, one flow per case: the operand that
// is not the list — the second operand of UNION, INTERSECT, SUBTRACT, EQUALS, and
// the object of CONTAINS — is CE0023 for every primitive type (String, Integer,
// Long, Decimal, Boolean, DateTime, enumeration). An Object or a List there is
// clean. An enumeration PARAMETER is not judged: `LO.Color` reads as an entity
// type until the project says otherwise, so it is never reported. `check` and `exec` accepted every rejected case.
var listOperandOps = []struct{ name, tpl string }{
	{"union", "$R = UNION $L WITH $X;"},
	{"intersect", "$R = INTERSECT $L WITH $X;"},
	{"subtract", "$R = SUBTRACT $X FROM $L;"},
	{"equals", "$R = EQUALS $L AND $X;"},
	{"contains", "$R = CONTAINS $X IN $L;"},
}

var listOperandPrimitives = []struct{ name, typ string }{
	{"string", "String"}, {"integer", "Integer"}, {"long", "Long"}, {"decimal", "Decimal"},
	{"boolean", "Boolean"}, {"datetime", "DateTime"},
}

func listOperandFlow(t *testing.T, kind, xType, body string) []linter.Violation {
	t.Helper()
	src := fmt.Sprintf("mdl 1;\ncreate %s LO.F ($L: list of LO.Item, $X: %s) begin\n  %s\nend;\n", kind, xType, body)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		return ValidateMicroflow(s)
	case *ast.CreateNanoflowStmt:
		return ValidateNanoflow(s)
	}
	t.Fatalf("unexpected %T", prog.Statements[0])
	return nil
}

func listOperandHits(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == listOperandKindRule {
			out = append(out, v)
		}
	}
	return out
}

func TestListOperandKinds_PrimitiveWhereObjectOrListIsRequired(t *testing.T) {
	for _, kind := range []string{"microflow", "nanoflow"} {
		for _, op := range listOperandOps {
			for _, p := range listOperandPrimitives {
				t.Run(kind+"/"+op.name+"/"+p.name, func(t *testing.T) {
					hits := listOperandHits(listOperandFlow(t, kind, p.typ, op.tpl))
					if len(hits) != 1 {
						t.Fatalf("want one MDL101 violation, got %v", hits)
					}
					if !strings.Contains(hits[0].Message, "CE0023") || !strings.Contains(hits[0].Message, "$X") {
						t.Errorf("message should name CE0023 and the variable: %s", hits[0].Message)
					}
				})
			}
		}
	}
}

// Controls: an Object or a List in the same position is clean on MxBuild, and
// so must not be reported.
func TestListOperandKinds_ObjectAndListOperandsAreFine(t *testing.T) {
	for _, op := range listOperandOps {
		for _, x := range []string{"LO.Item", "list of LO.Item"} {
			t.Run(op.name+"/"+x, func(t *testing.T) {
				if hits := listOperandHits(listOperandFlow(t, "microflow", x, op.tpl)); len(hits) != 0 {
					t.Errorf("an object or list operand was reported: %v", hits)
				}
			})
		}
	}
}

// `$M = contains($Hay, $Needle)` over two Strings parses as a list operation and
// builds as the String function (ledger #53, addListOperationAction): the
// needle being a String is exactly right there.
func TestListOperandKinds_StringContainsIsNotAListOperation(t *testing.T) {
	src := "mdl 1;\ncreate microflow LO.F ($Hay: String, $Needle: String) begin\n  declare $M Boolean = false;\n  set $M = contains($Hay, $Needle);\nend;\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if hits := listOperandHits(ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt))); len(hits) != 0 {
		t.Errorf("string contains was reported as a list operation: %v", hits)
	}
}

// A variable whose kind is not known here (not a parameter or a declare) is never
// reported, and neither is a wrong FIRST operand: that is CE0097, a different rule.
func TestListOperandKinds_OnlyTheSecondOperandAndOnlyWhenKnown(t *testing.T) {
	unknown := "mdl 1;\ncreate microflow LO.F ($L: list of LO.Item) begin\n  $R = UNION $L WITH $Mystery;\nend;\n"
	prog, errs := visitor.Build(unknown)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if hits := listOperandHits(ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt))); len(hits) != 0 {
		t.Errorf("a variable of unknown kind was reported: %v", hits)
	}
	if hits := listOperandHits(listOperandFlow(t, "microflow", "String", "$R = UNION $X WITH $L;")); len(hits) != 0 {
		t.Errorf("a primitive FIRST operand is CE0097, not this rule: %v", hits)
	}
}

// An enumeration is CE0023 too (measured), but only a variable the script says is
// an enumeration can be told from an object here.
func TestListOperandKinds_EnumerationVariable(t *testing.T) {
	declared := "mdl 1;\ncreate microflow LO.F ($L: list of LO.Item) begin\n  declare $X enum LO.Color = LO.Color.Red;\n  $R = UNION $L WITH $X;\nend;\n"
	prog, errs := visitor.Build(declared)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if hits := listOperandHits(ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt))); len(hits) != 1 {
		t.Errorf("a declared enumeration variable should be reported once, got %v", hits)
	}
	// The same name as a parameter is indistinguishable from an object: silent.
	if hits := listOperandHits(listOperandFlow(t, "microflow", "LO.Color", "$R = UNION $L WITH $X;")); len(hits) != 0 {
		t.Errorf("an enumeration parameter cannot be told from an object and must not be reported: %v", hits)
	}
}
