// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

func TestValidateMicroflowReferencesAddExpressionValue(t *testing.T) {
	input := `create microflow Synthetic.MF_AddExpressionScope ()
returns Boolean
begin
  declare $Items List of Synthetic.Item = empty;
  declare $Fallback Synthetic.Item = empty;
  if true then
    declare $SourceItem Synthetic.Item = empty;
  end if;
  add if true then $SourceItem else $Fallback to $Items;
  return true;
end;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	violations := ValidateMicroflow(stmt)
	for _, violation := range violations {
		if violation.RuleID == "MDL005" && strings.Contains(violation.Message, "$SourceItem") {
			return
		}
	}
	t.Fatalf("Expected MDL005 for add expression source variable, got %#v", violations)
}
