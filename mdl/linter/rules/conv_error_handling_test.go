// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
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

	if len(violations) != 1 {
		t.Errorf("expected 1 violation inside loop, got %d", len(violations))
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
