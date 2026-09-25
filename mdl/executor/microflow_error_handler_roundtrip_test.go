// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1078: a microflow whose activity carried a custom error
// handler came back from DESCRIBE with the handler — and the whole branch behind
// it — gone, so a describe→edit→exec round-trip deleted it from the model.
//
// The gate is getActionErrorHandlingType. emitActivityStatement only walks the
// error branch when hasCustomErrorHandler() agrees, and the hand-maintained
// switch that fed it covered 17 of the 38 action types that store the field. The
// reported activity (create variable) was one of the 21 it missed; so were
// create-object and change-object, which is most of what a real microflow is
// made of.
//
// The output stayed valid MDL throughout, which is why nothing caught it: no
// parse error, no mx check finding, just a smaller microflow.

// errorHandlerCase is one action type that had no case in the old switch.
type errorHandlerCase struct {
	name   string
	action microflows.MicroflowAction
	want   string // a fragment of the statement DESCRIBE should render
}

// The action types the switch missed, one per family. Every one of these stores
// ErrorHandlingType and every one lost its branch before the fix.
func missingErrorHandlerCases() []errorHandlerCase {
	return []errorHandlerCase{
		{"create variable", &microflows.CreateVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			VariableName:      "name", DataType: &microflows.StringType{},
			InitialValue: "'NameValue'",
		}, "declare $name"},
		{"change variable", &microflows.ChangeVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			VariableName:      "name", Value: "'other'",
		}, "set $name"},
		{"create object", &microflows.CreateObjectAction{
			ErrorHandlingType:   microflows.ErrorHandlingTypeCustom,
			EntityQualifiedName: "Mod.Car", OutputVariable: "Car",
		}, "create Mod.Car"},
		{"change object", &microflows.ChangeObjectAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			ChangeVariable:    "Car",
		}, "change $Car"},
		{"log message", &microflows.LogMessageAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
		}, "log "},
		{"close page", &microflows.ClosePageAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom, NumberOfPages: 1,
		}, "close page"},
		{"custom without rollback", &microflows.CreateVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustomWithoutRollback,
			VariableName:      "n", DataType: &microflows.StringType{}, InitialValue: "'v'",
		}, "declare $n"},
	}
}

// describeWithErrorHandler runs the real traversal over a two-activity flow whose
// first activity carries a custom handler, and returns the rendered MDL.
func describeWithErrorHandler(t *testing.T, action microflows.MicroflowAction) string {
	t.Helper()
	e := newTestExecutor()

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("act"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act")},
			Action:       action,
		},
		// The error branch. Its statement is the canary: if the branch is not
		// traversed, this string is absent from the output entirely.
		mkID("err"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("err")},
			Action:       &microflows.RollbackObjectAction{RollbackVariable: "Handled"},
		},
		mkID("end"): &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"): {mkFlow("start", "act")},
		mkID("act"):   {mkFlow("act", "end"), mkErrorFlow("act", "err")},
	}

	var lines []string
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil,
		make(map[model.ID]bool), nil, nil, &lines, 1, nil, 0, nil)
	return strings.Join(lines, "\n")
}

// The regression itself. With the reflection lookup reverted to the old switch,
// every subtest here fails on the first assertion — the branch statement is
// simply not in the output.
func TestDescribe_ErrorHandlerSurvives_ActionsTheSwitchMissed(t *testing.T) {
	for _, tc := range missingErrorHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			out := describeWithErrorHandler(t, tc.action)

			if !strings.Contains(out, "rollback $Handled;") {
				t.Errorf("the error branch was dropped — a describe→exec round-trip "+
					"would delete it from the model (#1078):\n%s", out)
			}
			if !strings.Contains(out, "on error") {
				t.Errorf("output does not mark the handler, so the branch would be "+
					"re-executed unconditionally in the main flow:\n%s", out)
			}
			// Live MDL, not the commented-out fallback: these statements can all
			// carry an onErrorClause, so the round trip must actually round-trip.
			if strings.Contains(out, "-- on error") {
				t.Errorf("handler rendered commented-out, but this statement can carry "+
					"the clause — describe→exec would still lose it:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("statement %q missing from output:\n%s", tc.want, out)
			}
		})
	}
}

// Control for the test above. Without this, a "fix" that reported a custom
// handler for EVERY action would pass every case in the table while inventing
// handlers on activities that have none — and DESCRIBE would grow an `on error`
// block on activities the author never put one on.
func TestDescribe_NoErrorHandler_WhenTheActionHasNone(t *testing.T) {
	// Same flow shape, same error flow present in the model, but the action
	// carries no error-handling type at all.
	out := describeWithErrorHandler(t, &microflows.CreateVariableAction{
		VariableName: "name", DataType: &microflows.StringType{}, InitialValue: "'v'",
	})
	if strings.Contains(out, "on error {") {
		t.Errorf("a live `on error { }` block was rendered for an action with no "+
			"error handling type:\n%s", out)
	}
}

// Rollback is deliberately excluded and must stay excluded: it is what an
// activity with NO authored clause stores, so rendering it would put a clause in
// the user's script that they never wrote (#840). Keeping this beside the #1078
// cases pins both directions of the same decision.
func TestDescribe_RollbackIsStillNotRendered(t *testing.T) {
	out := describeWithErrorHandler(t, &microflows.CreateVariableAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		VariableName:      "name", DataType: &microflows.StringType{}, InitialValue: "'v'",
	})
	if strings.Contains(out, "on error rollback") {
		t.Errorf("#840 regression: `on error rollback` rendered for the default:\n%s", out)
	}
}

// RestOperationCallAction stores the field but Mendix refuses a custom handler on
// it (CE6035), so it is the one action deliberately not reported. A reflection
// lookup would otherwise pick it up — this is the case that stops the generic fix
// from being too generic.
func TestGetActionErrorHandlingType_RestOperationCallIsExcluded(t *testing.T) {
	activity := &microflows.ActionActivity{
		Action: &microflows.RestOperationCallAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
		},
	}
	if got := getActionErrorHandlingType(activity); got != "" {
		t.Errorf("got %q, want empty — Mendix rejects a custom handler here with CE6035", got)
	}
}
