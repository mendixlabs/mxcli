// SPDX-License-Identifier: Apache-2.0

package microflows

import "reflect"

// ErrorHandling returns how the activity handles an error in its action.
//
// Mendix stores the handling on the action (Microflows$*Action.ErrorHandlingType),
// and that is where the readers put it; the activity-level field is only a
// fallback for an action type that has no such field. Reading the activity
// field alone sees "" for every action the reader built, which made CONV013
// flag handled Java/REST/web service calls and CONV014 miss `on error continue`.
func (a *ActionActivity) ErrorHandling() ErrorHandlingType {
	if a == nil {
		return ""
	}
	if a.Action != nil {
		if errType := ActionErrorHandlingType(a.Action); errType != "" {
			return errType
		}
	}
	return a.ErrorHandlingType
}

// ActionErrorHandlingType reads ErrorHandlingType off any action that declares it.
//
// Reflection rather than a case per action type: a hand-maintained switch had
// drifted to 17 of the 38 action types that carry the field (mendixlabs/mxcli#1078),
// so a new action type must not have to be remembered. Actions embed
// model.BaseElement, which has no such field, so a promoted field cannot be
// picked up by accident.
func ActionErrorHandlingType(action MicroflowAction) ErrorHandlingType {
	v := reflect.ValueOf(action)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("ErrorHandlingType")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return ErrorHandlingType(f.String())
}
