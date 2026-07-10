// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// TestValidateMicroflow_DeclareListIsRejected guards issue #607: a list-typed
// `declare` parses and passed `mxcli check`, but Studio Pro rejects it with
// CE0053 ("type not allowed") and CE0038 ("value required") because `declare`
// maps to a Create Variable activity, which cannot produce a list. Lists must
// come from a microflow parameter, a `retrieve`, or a `create list`.
func TestValidateMicroflow_DeclareListIsRejected(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "empty initializer (the reported case)",
			input: `create microflow Synthetic.MF_DeclareEmptyList ()
begin
  declare $Items list of Synthetic.Item = empty;
end;`,
		},
		{
			name: "non-empty initializer is still a Create Variable on a list",
			input: `create microflow Synthetic.MF_DeclareList ()
begin
  declare $Items list of Synthetic.Item = $Other;
end;`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.input)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

			violations := ValidateMicroflow(stmt)
			var found *linter.Violation
			for i := range violations {
				if violations[i].RuleID == "MDL040" {
					found = &violations[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected MDL040 for list-typed declare, got %#v", violations)
			}
			if found.Severity != linter.SeverityError {
				t.Errorf("MDL040 severity = %v, want Error (Studio Pro rejects this with CE0053/CE0038)", found.Severity)
			}
			if !strings.Contains(found.Message, "Items") {
				t.Errorf("MDL040 message should name the offending variable, got %q", found.Message)
			}
		})
	}
}

// TestValidateMicroflow_DeclareScalarIsAllowed ensures the new rule is scoped to
// list types only — a plain object/primitive declare must NOT be flagged.
func TestValidateMicroflow_DeclareScalarIsAllowed(t *testing.T) {
	input := `create microflow Synthetic.MF_DeclareScalar ()
begin
  declare $Customer Synthetic.Customer = empty;
  declare $Count Integer = 0;
end;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL040" {
			t.Fatalf("MDL040 must not fire on scalar declares, got: %q", v.Message)
		}
	}
}
