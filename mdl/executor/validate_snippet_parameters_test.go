// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL087 refuses, without a project, what no Mendix version stores on a
// snippet parameter: `optional`. It used to refuse a primitive type as well
// (mendixlabs/mxcli#1028, CE0046); that is valid from 11.17 and is now the
// pages/snippet_parameter_primitive version gate, which `check -p` and exec
// apply (TestStatementVersionGates_PrimitiveSnippetParameter). This pass cannot
// know the version, and refusing here blocked the valid form at exec's
// pre-flight.
func checkSnippetSource(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var msgs []string
	for _, stmt := range prog.Statements {
		for _, v := range validateSnippetParameters(stmt) {
			if v.RuleID != "MDL087" {
				t.Errorf("unexpected rule %s", v.RuleID)
			}
			msgs = append(msgs, v.Message)
		}
	}
	return msgs
}

func TestValidateSnippetParameters(t *testing.T) {
	cases := []struct {
		name    string
		params  string
		want    int
		phrases []string
	}{{
		// The #1028 repro: a primitive is now a version question, not a
		// project-free refusal.
		name:   "a primitive parameter is left to the version gate",
		params: "$Label: string",
		want:   0,
	}, {
		name: "every primitive in one clause",
		params: "$Label: String, $Count: Long, $Rank: Integer, " +
			"$Amount: Decimal, $Active: Boolean, $Due: DateTime",
		want: 0,
	}, {
		// `optional` is stored by no version: still refused, per parameter.
		name:    "an optional parameter",
		params:  "$Order: Sales.Order optional",
		want:    1,
		phrases: []string{"$Order", "optional"},
	}, {
		// CONTROL: the form Mendix accepts on every version.
		name:   "an entity parameter is clean",
		params: "$Order: Sales.Order",
		want:   0,
	}, {
		// CONTROL: a quoted entity name is still an entity.
		name:   "a quoted entity parameter is clean",
		params: `$Order: Sales."Order"`,
		want:   0,
	}, {
		name:   "no parameters at all",
		params: "",
		want:   0,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := ""
			if tc.params != "" {
				header = "( Params: { " + tc.params + " } )"
			}
			msgs := checkSnippetSource(t, "CREATE SNIPPET M.S "+header+
				" { DYNAMICTEXT dt (Content: 'x') }")
			if len(msgs) != tc.want {
				t.Fatalf("got %d violations, want %d: %v", len(msgs), tc.want, msgs)
			}
			for _, phrase := range tc.phrases {
				if !strings.Contains(msgs[0], phrase) {
					t.Errorf("message does not contain %q: %s", phrase, msgs[0])
				}
			}
		})
	}
}

// `alter snippet … add parameters` declares the same element (mendixlabs/mxcli#1234):
// `optional` refused, a primitive left to the gate.
func TestValidateSnippetParameters_AlterAdd(t *testing.T) {
	if msgs := checkSnippetSource(t, `alter snippet M.S { add parameters $Label: String; add parameters $C: M.Customer; };`); len(msgs) != 0 {
		t.Fatalf("a primitive is the version gate's, got %v", msgs)
	}
	msgs := checkSnippetSource(t, `alter snippet M.S { add parameters $C: M.Customer optional; };`)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "$C") {
		t.Fatalf("want one MDL087 for optional $C, got %v", msgs)
	}
}

// MDL-WIDGET34 at check time: `Attribute: $Label` binds a primitive parameter
// of the document as it binds a page variable — exec writes it in the
// SnippetParameter or PageParameter slot (mx 11.17.0: 0 errors). A name that is
// neither still fails, and an entity parameter is not a value an input edits.
func TestValidatePageVariableBindings_PrimitiveParameter(t *testing.T) {
	input := func(name string) []*ast.WidgetV3 {
		return []*ast.WidgetV3{{Type: "textbox", Name: "tb", Properties: map[string]any{
			"Attribute": &ast.DataSourceV3{Reference: "$" + name},
		}}}
	}
	if got := validatePageVariableBindings(input("Label"), nil, []string{"Label"}, "snippet M.S"); len(got) != 0 {
		t.Errorf("$Label (a primitive parameter): %+v, want none", got)
	}
	// Control: the same binding with nothing declared is the refusal it was.
	if got := validatePageVariableBindings(input("Label"), nil, nil, "snippet M.S"); len(got) != 1 || got[0].RuleID != "MDL-WIDGET34" {
		t.Errorf("$Label (undeclared): %+v, want one MDL-WIDGET34", got)
	}
	stmt := &ast.CreateSnippetStmtV3{Parameters: []ast.PageParameter{
		{Name: "Label", Type: ast.DataType{Kind: ast.TypeString}},
		{Name: "C", EntityType: ast.QualifiedName{Module: "M", Name: "Customer"}},
	}}
	if got := documentPrimitiveParameters(stmt); len(got) != 1 || got[0] != "Label" {
		t.Errorf("documentPrimitiveParameters = %v, want [Label]", got)
	}
}
