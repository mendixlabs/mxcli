// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// TestValidateMicroflow_DivIntoInteger covers MDL041: integer division ('div')
// yields a Decimal, so assigning it to an Integer/Long variable fails mx check
// with CE0117 even though the MDL is syntactically valid. Rounding-function
// results assigned to Integer are accepted by Mendix and must not be flagged.
func TestValidateMicroflow_DivIntoInteger(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMDL bool
	}{
		{"div into Integer var", "declare $I Integer = 0;\n  set $I = $a * 100 div $b;", true},
		{"div into declared Integer", "declare $I Integer = $a div $b;", true},
		{"div into Long", "declare $L Long = $a div $b;", true},
		{"div into Decimal is fine", "declare $D Decimal = $a div $b;", false},
		{"integer add is fine", "declare $I Integer = 0;\n  set $I = $a + $b;", false},
		{"integer mult is fine", "declare $I Integer = 0;\n  set $I = $a * $b;", false},
		{"round result into Integer is fine", "declare $I Integer = round(sqrt($a));", false},
		{"round of div into Integer is fine", "declare $I Integer = round($a div $b);", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($a: Integer, $b: Integer)\nbegin\n  " + tc.body + "\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			var got bool
			for _, s := range prog.Statements {
				mf, ok := s.(*ast.CreateMicroflowStmt)
				if !ok {
					continue
				}
				for _, vi := range ValidateMicroflow(mf) {
					if vi.RuleID == "MDL041" {
						got = true
					}
				}
			}
			if got != tc.wantMDL {
				t.Errorf("MDL041 fired=%v, want %v (body: %q)", got, tc.wantMDL, tc.body)
			}
		})
	}
}

// TestValidateMicroflow_DivMessage checks the diagnostic names the target and
// the div cause, and suggests the fix.
func TestValidateMicroflow_DivMessage(t *testing.T) {
	src := "create microflow M.F ($a: Integer, $b: Integer)\nbegin\n  declare $Count Integer = $a div $b;\nend;"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var msg, sugg string
	for _, vi := range ValidateMicroflow(mf) {
		if vi.RuleID == "MDL041" {
			msg, sugg = vi.Message, vi.Suggestion
		}
	}
	if msg == "" {
		t.Fatal("expected MDL041 violation")
	}
	if !strings.Contains(msg, "$Count") || !strings.Contains(msg, "CE0117") || !strings.Contains(msg, "div") {
		t.Errorf("message missing detail: %q", msg)
	}
	if !strings.Contains(sugg, "Decimal") {
		t.Errorf("suggestion should mention Decimal: %q", sugg)
	}
}
