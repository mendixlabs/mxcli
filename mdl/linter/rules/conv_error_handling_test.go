// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestFindUnhandledCalls_RestCallNoCustom(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
			},
			Action: &microflows.RestCallAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV013" {
		t.Errorf("expected CONV013, got %s", violations[0].RuleID)
	}
}

func TestFindUnhandledCalls_RestCallCustom(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
			},
			Action: &microflows.RestCallAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations with Custom handling, got %d", len(violations))
	}
}

func TestFindUnhandledCalls_CustomWithoutRollback(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeCustomWithoutRollback,
			},
			Action: &microflows.RestCallAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations with CustomWithoutRollback, got %d", len(violations))
	}
}

func TestFindUnhandledCalls_JavaAction(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
			},
			Action: &microflows.JavaActionCallAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for Java action, got %d", len(violations))
	}
}

func TestFindUnhandledCalls_WebServiceCall(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
			},
			Action: &microflows.WebServiceCallAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for WS call, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV013" {
		t.Errorf("expected CONV013, got %s", violations[0].RuleID)
	}
}

func TestFindUnhandledCalls_NonExternalAction(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
			},
			Action: &microflows.CommitObjectsAction{},
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for non-external action, got %d", len(violations))
	}
}

// ako/mxcli#1838 (mendixlabs/mxcli#1689): inside a loop Mendix allows only
// Rollback (CE0644, which check reports as MDL006), so a CONV013 finding there
// could not be fixed in place — one rule or the other always fired. A call in
// a loop is not reported; its handling is the loop's.
func TestFindUnhandledCalls_InsideLoop(t *testing.T) {
	loopBody := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{
					ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
				},
				Action: &microflows.RestCallAction{},
			},
		},
	}
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: loopBody,
		},
	}

	var violations []linter.Violation
	r := NewErrorHandlingOnCallsRule()
	findUnhandledCalls(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("a call inside a loop can only roll back (CE0644); want no finding, got %d", len(violations))
	}
}

// ako/mxcli#1838: a call to an action that always throws, such as
// CommunityCommons.ThrowException, is how a flow rolls itself back — MDL
// refuses `raise error` in the main flow (MDL084/CE0710). Its Rollback is the
// point of the call; a custom handler beside it is what another rule flags.
func TestFindUnhandledCalls_ThrowingJavaActionIsDeliberate(t *testing.T) {
	call := func(javaAction string) []microflows.MicroflowObject {
		return []microflows.MicroflowObject{&microflows.ActionActivity{
			Action: &microflows.JavaActionCallAction{JavaAction: javaAction},
		}}
	}
	r := NewErrorHandlingOnCallsRule()
	for _, ja := range []string{"CommunityCommons.ThrowException", "CommunityCommons.ThrowWebserviceException"} {
		var violations []linter.Violation
		findUnhandledCalls(call(ja), testMicroflow(), r, &violations)
		if len(violations) != 0 {
			t.Errorf("%s: want no finding for a deliberate throw, got %v", ja, violations)
		}
	}
	// Control: any other Java action with Rollback handling is still reported.
	var violations []linter.Violation
	findUnhandledCalls(call("CommunityCommons.Base64Encode"), testMicroflow(), r, &violations)
	if len(violations) != 1 {
		t.Errorf("control: want 1 finding for a Java action that does not throw, got %d", len(violations))
	}
}

// --- CONV014 tests ---

func TestFindContinueErrorHandling_Activity(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				Caption:           "Do something",
				ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
			},
			Action: &microflows.CommitObjectsAction{},
		},
	}

	var violations []linter.Violation
	r := NewNoContinueErrorHandlingRule()
	findContinueErrorHandling(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV014" {
		t.Errorf("expected CONV014, got %s", violations[0].RuleID)
	}
}

func TestFindContinueErrorHandling_Loop(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			Caption:           "Process items",
			ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
		},
	}

	var violations []linter.Violation
	r := NewNoContinueErrorHandlingRule()
	findContinueErrorHandling(objects, testMicroflow(), r, &violations)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for loop, got %d", len(violations))
	}
}

func TestFindContinueErrorHandling_AbortIsOk(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{
				ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
			},
			Action: &microflows.CommitObjectsAction{},
		},
	}

	var violations []linter.Violation
	r := NewNoContinueErrorHandlingRule()
	findContinueErrorHandling(objects, testMicroflow(), r, &violations)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for Abort, got %d", len(violations))
	}
}

func TestErrorHandlingOnCallsRule_Metadata(t *testing.T) {
	r := NewErrorHandlingOnCallsRule()
	if r.ID() != "CONV013" {
		t.Errorf("ID = %q, want CONV013", r.ID())
	}
}

func TestNoContinueErrorHandlingRule_Metadata(t *testing.T) {
	r := NewNoContinueErrorHandlingRule()
	if r.ID() != "CONV014" {
		t.Errorf("ID = %q, want CONV014", r.ID())
	}
}

// --- mendixlabs/mxcli#1202: the reader stores error handling on the ACTION ---
//
// Every test above sets BaseActivity.ErrorHandlingType, which the model reader
// never fills: Mendix stores an action activity's handling on the action
// (Microflows$JavaActionCallAction.ErrorHandlingType). The objects below are
// shaped the way flowObjectFromGen builds them, which is what the rules see on
// a real project. Reading the activity field, CONV013 reported "uses ''" on
// every call, handled or not, and CONV014 never fired on an action.

// readerShapedCall builds an action activity the way the reader does: the
// activity field empty, the handling on the action.
func readerShapedCall(eh microflows.ErrorHandlingType) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{Caption: "Call echo"},
		Action:       &microflows.JavaActionCallAction{ErrorHandlingType: eh},
	}
}

func TestFindUnhandledCalls_ReadsTheActionsHandling(t *testing.T) {
	cases := []struct {
		eh   microflows.ErrorHandlingType
		want int
	}{
		{microflows.ErrorHandlingTypeCustom, 0},
		{microflows.ErrorHandlingTypeCustomWithoutRollback, 0},
		{microflows.ErrorHandlingTypeRollback, 1},
		{microflows.ErrorHandlingTypeContinue, 1},
	}
	for _, tc := range cases {
		t.Run(string(tc.eh), func(t *testing.T) {
			var violations []linter.Violation
			findUnhandledCalls([]microflows.MicroflowObject{readerShapedCall(tc.eh)},
				testMicroflow(), NewErrorHandlingOnCallsRule(), &violations)
			if len(violations) != tc.want {
				t.Fatalf("got %d CONV013 violations for a Java action call with %s handling, want %d",
					len(violations), tc.eh, tc.want)
			}
			if tc.want == 1 && !strings.Contains(violations[0].Message, "'"+string(tc.eh)+"'") {
				t.Errorf("message %q does not name the stored handling %q -- it reads an empty field",
					violations[0].Message, tc.eh)
			}
		})
	}
}

func TestFindContinueErrorHandling_ReadsTheActionsHandling(t *testing.T) {
	var violations []linter.Violation
	findContinueErrorHandling([]microflows.MicroflowObject{
		readerShapedCall(microflows.ErrorHandlingTypeContinue),
		readerShapedCall(microflows.ErrorHandlingTypeRollback),
	}, testMicroflow(), NewNoContinueErrorHandlingRule(), &violations)
	if len(violations) != 1 {
		t.Fatalf("got %d CONV014 violations, want 1 -- `on error continue` on an action is stored on the action", len(violations))
	}
	if !strings.Contains(violations[0].Message, "Call echo") {
		t.Errorf("message %q does not name the activity", violations[0].Message)
	}
}
