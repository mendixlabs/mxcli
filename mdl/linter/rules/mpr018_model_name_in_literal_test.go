// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestModelNameInLiteralRule_Metadata(t *testing.T) {
	r := NewModelNameInLiteralRule()
	if r.ID() != "MPR018" {
		t.Errorf("ID = %q, want MPR018", r.ID())
	}
	if r.Name() != "ModelNameInLiteral" {
		t.Errorf("Name = %q, want ModelNameInLiteral", r.Name())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
}

// Literals are extracted, not whole expressions.
//
// This is the load-bearing decision. An expression's own references —
// $Var/Module.Assoc/Attr, an enumeration value Module.Enum.Value — are spelled
// with dots and would match a qualified-name shape, but they are REAL
// references that Studio Pro sees and renames. Only a single-quoted literal is
// opaque to it. Scanning whole expressions would report every association path
// in the project.
func TestExpressionStringLiterals(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want []string
	}{
		{`'Simulation.SIM_Tick'`, []string{"Simulation.SIM_Tick"}},
		{`$Settings/Shop.Settings_Order/Name`, nil},         // association path, not a literal
		{`Shop.Status.Open`, nil},                           // enumeration reference
		{`$a + 'Shop.Order' + $b`, []string{"Shop.Order"}},  // literal among operands
		{`'a' + 'Shop.Order'`, []string{"a", "Shop.Order"}}, // several literals
		{`'it''s here'`, []string{"it's here"}},             // doubled quote is an escape
		{`'Shop.A' + 'it''s' + 'Shop.B'`, []string{"Shop.A", "it's", "Shop.B"}},
		{`''`, []string{""}},
		{`'unterminated`, nil}, // not a literal; never panics
		{``, nil},
	} {
		got := expressionStringLiterals(tc.expr)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("expressionStringLiterals(%q) = %#v, want %#v", tc.expr, got, tc.want)
		}
	}
}

// A literal is only a candidate when it is EXACTLY a qualified name. A literal
// that merely mentions one in prose is a stale message at worst, not a rename
// trap, and matching inside prose is where the false positives live.
func TestQualifiedNameLiteral(t *testing.T) {
	for _, tc := range []struct {
		lit        string
		wantModule string
		wantOK     bool
	}{
		{"Simulation.SIM_Tick", "Simulation", true},
		{"Shop.Order.Status", "Shop", true}, // Module.Entity.Attribute
		{"Shop.Order", "Shop", true},
		{"calling Shop.Order now", "", false},  // prose mentioning one
		{"Shop.Order ", "", false},             // trailing space: not exactly a name
		{"shop/order", "", false},              // a path
		{"https://x.example/y", "", false},     // a URL
		{"Shop", "", false},                    // one segment
		{"Shop.", "", false},                   // trailing dot
		{".Order", "", false},                  // leading dot
		{"Shop..Order", "", false},             // empty segment
		{"Shop.Order.Status.Extra", "", false}, // too many segments to be a member
		{"com.mendix.core.Core", "", false},    // a Java class: four segments
		{"1Shop.Order", "", false},             // segment must start like an identifier
		{"", "", false},
	} {
		mod, _, ok := qualifiedNameLiteral(tc.lit)
		if ok != tc.wantOK || mod != tc.wantModule {
			t.Errorf("qualifiedNameLiteral(%q) = (%q, _, %v), want (%q, _, %v)",
				tc.lit, mod, ok, tc.wantModule, tc.wantOK)
		}
	}
}

// Only the fields that hold a VALUE are scanned, never the fields that hold a
// reference.
//
// JavaActionCallAction.JavaAction, MicroflowCall.Microflow and
// JavaActionParameterMapping.Parameter all carry qualified names, and all are
// references Studio Pro resolves and renames. Reporting them would flag every
// call in the project. The separation is by field NAME — Expression, Argument,
// Value — which is why reference fields, named after what they reference, fall
// outside it.
func TestValueStringsOfAction_ScansValuesNotReferences(t *testing.T) {
	act := &microflows.JavaActionCallAction{
		JavaAction: "Simulation.JA_StartPeriodicMicroflow", // a REAL reference
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "Simulation.JA_StartPeriodicMicroflow.MicroflowName", // a reference
				// The literal: the defect. Reached through the
				// CodeActionParameterValue interface, which the walk descends.
				Value: &microflows.BasicCodeActionParameterValue{Argument: `'Simulation.SIM_Tick'`},
			},
		},
	}
	got := valueStrings(act)

	if len(got) != 1 || got[0] != `'Simulation.SIM_Tick'` {
		t.Fatalf("valueStrings = %#v, want only the Argument", got)
	}
}

// A Change activity's member value is a value too, and reachable inside a loop.
func TestValueStringsOfAction_FindsMemberChangeValuesInsideLoops(t *testing.T) {
	inner := &microflows.ActionActivity{}
	inner.Action = &microflows.ChangeObjectAction{
		Changes: []*microflows.MemberChange{{Value: `'Shop.Order'`}},
	}
	loop := &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{inner},
		},
	}
	var got []string
	collectValueStrings([]microflows.MicroflowObject{loop}, func(s string) { got = append(got, s) })
	if len(got) != 1 || got[0] != `'Shop.Order'` {
		t.Fatalf("collectValueStrings = %#v, want the member value from inside the loop", got)
	}
}

// The two verdicts, and the exclusion that makes them usable.
func TestClassifyLiteral(t *testing.T) {
	// Modules the project owns, and the documents that resolve.
	modules := map[string]bool{"simulation": true, "shop": true}
	resolves := map[string]bool{"simulation.sim_tick": true, "shop.order": true}

	for _, tc := range []struct {
		name string
		lit  string
		want literalVerdict
	}{
		{"resolves: a rename trap", "Simulation.SIM_Tick", verdictResolves},
		{"module exists, name does not: likely stale", "Shop.NoSuchFlow", verdictDangling},
		{
			// The exclusion that kills prose, filenames, CSS and Java packages:
			// the prefix has to be a module this project actually has.
			"unknown module: not ours to judge",
			"System.Text.Json", verdictNone,
		},
		{"not a qualified name at all", "just some text", verdictNone},
		{"case-insensitive, as Mendix resolves names", "SIMULATION.sim_tick", verdictResolves},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyLiteral(tc.lit, modules, resolves); got != tc.want {
				t.Errorf("classifyLiteral(%q) = %v, want %v", tc.lit, got, tc.want)
			}
		})
	}
}

// No reader means no findings, never "nothing is wrong".
func TestModelNameInLiteralRule_NoReaderReportsNothing(t *testing.T) {
	if got := NewModelNameInLiteralRule().Check(linter.NewLintContextFromDB(nil)); len(got) != 0 {
		t.Fatalf("got %d findings with no reader, want 0", len(got))
	}
}
