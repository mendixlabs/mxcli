// SPDX-License-Identifier: Apache-2.0

package microflows

import "testing"

// ActionErrorHandlingType is the reflection lookup that replaced the per-type switch
// (mendixlabs/mxcli#1078). Pinning it directly is what makes it durable: a NEW action
// type carrying ErrorHandlingType is handled the moment it exists.
func TestActionErrorHandlingType(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action MicroflowAction
		want   ErrorHandlingType
	}{
		{"reads the field", &CreateVariableAction{ErrorHandlingType: ErrorHandlingTypeCustom}, ErrorHandlingTypeCustom},
		{"empty when unset", &CreateVariableAction{}, ""},
		{"action without the field", &ListOperationAction{}, ""},
		{"nil action", nil, ""},
	} {
		if got := ActionErrorHandlingType(tc.action); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}

	// A typed-nil pointer must not panic — activity.Action can hold one.
	var typedNil *CreateVariableAction
	if got := ActionErrorHandlingType(typedNil); got != "" {
		t.Errorf("typed nil: got %q, want empty", got)
	}
}

// The readers put the handling on the action and leave the activity field empty, so the
// action must win; the activity field only answers for an action type that has none.
func TestActionActivityErrorHandling(t *testing.T) {
	for _, tc := range []struct {
		name     string
		activity *ActionActivity
		want     ErrorHandlingType
	}{
		{"on the action, as the reader stores it",
			&ActionActivity{Action: &JavaActionCallAction{ErrorHandlingType: ErrorHandlingTypeCustomWithoutRollback}},
			ErrorHandlingTypeCustomWithoutRollback},
		{"the action wins over the activity",
			&ActionActivity{BaseActivity: BaseActivity{ErrorHandlingType: ErrorHandlingTypeAbort},
				Action: &RestCallAction{ErrorHandlingType: ErrorHandlingTypeContinue}},
			ErrorHandlingTypeContinue},
		{"activity field for an action without one",
			&ActionActivity{BaseActivity: BaseActivity{ErrorHandlingType: ErrorHandlingTypeCustom},
				Action: &ListOperationAction{}},
			ErrorHandlingTypeCustom},
		{"no action", &ActionActivity{BaseActivity: BaseActivity{ErrorHandlingType: ErrorHandlingTypeRollback}},
			ErrorHandlingTypeRollback},
		{"nil activity", nil, ""},
	} {
		if got := tc.activity.ErrorHandling(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
