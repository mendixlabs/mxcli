// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A call argument's type was never compared with the called flow's parameter,
// so `call microflow M.F (Qty = '5')` for an Integer Qty checked clean and
// failed the build with CE0117 "Error(s) in expression" (measured on 11.14.0).

func callArgViolations(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "E009" && strings.Contains(v.Message, "of the call to") {
			out = append(out, v)
		}
	}
	return out
}

// TestTypeCheckProgramChecksCallArgumentsAgainstAStoredFlow: the called flow is
// in the project, so its parameters come from the catalog.
func TestTypeCheckProgramChecksCallArgumentsAgainstAStoredFlow(t *testing.T) {
	exec := typeCheckFixture(t)
	run(t, exec, `CREATE MICROFLOW MyFirstModule.ACT_Target (
		$Qty: Integer,
		$Status: Enumeration(MyFirstModule.OrderStatus),
		$Ticket: MyFirstModule.Ticket
	)
	BEGIN
	END;`)

	wrong := callArgViolations(typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Caller ()
BEGIN
  $T = CREATE MyFirstModule.Ticket ();
  CALL MICROFLOW MyFirstModule.ACT_Target (Qty = '5', Status = 'Open', Ticket = 'x');
END;
`))
	if len(wrong) != 3 {
		t.Errorf("want three mismatched arguments reported, got %+v", wrong)
	}
	for _, v := range wrong {
		if v.Location.DocumentName != "MyFirstModule.ACT_Caller" {
			t.Errorf("reported at %q, want the calling microflow", v.Location.DocumentName)
		}
	}

	// CONTROL: the same call with arguments of the parameters' types, and the
	// coercion Mendix makes (Long for Integer), is clean.
	right := callArgViolations(typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Caller ()
BEGIN
  $T = CREATE MyFirstModule.Ticket ();
  DECLARE $N Long = 5;
  CALL MICROFLOW MyFirstModule.ACT_Target (Qty = $N, Status = MyFirstModule.OrderStatus.Open, Ticket = $T);
END;
`))
	if len(right) != 0 {
		t.Errorf("correct arguments were reported: %+v", right)
	}
}

// TestTypeCheckProgramChecksCallsToAFlowTheScriptCreates: the catalog knows
// only the stored project, so a flow created in the same script would leave
// its calls unchecked, and a replaced one would be checked against its old
// signature.
func TestTypeCheckProgramChecksCallsToAFlowTheScriptCreates(t *testing.T) {
	exec := typeCheckFixture(t)

	created := callArgViolations(typeCheck(t, exec, `
CREATE MICROFLOW MyFirstModule.ACT_New ($Qty: Integer)
BEGIN
END;
CREATE MICROFLOW MyFirstModule.ACT_NewCaller ()
BEGIN
  CALL MICROFLOW MyFirstModule.ACT_New (Qty = 'five');
END;
`))
	if len(created) != 1 {
		t.Errorf("a call to a flow created in the script went unchecked: %+v", created)
	}

	run(t, exec, `CREATE MICROFLOW MyFirstModule.ACT_Old ($Qty: Integer) BEGIN END;`)
	replaced := callArgViolations(typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.ACT_Old ($Qty: String)
BEGIN
END;
CREATE MICROFLOW MyFirstModule.ACT_OldCaller ()
BEGIN
  CALL MICROFLOW MyFirstModule.ACT_Old (Qty = 'five');
END;
`))
	if len(replaced) != 0 {
		t.Errorf("checked against the stored signature the script replaces: %+v", replaced)
	}
}
