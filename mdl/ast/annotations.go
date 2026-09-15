// SPDX-License-Identifier: Apache-2.0

package ast

import "reflect"

// StatementAnnotations returns the @-annotations attached to a microflow
// statement, or nil for a statement type that carries none.
//
// Read by reflection on purpose. Fifteen statement types have an `Annotations
// *ActivityAnnotations` field today, and a hand-written type switch over them
// would silently skip the sixteenth — which is exactly the failure this exists to
// catch, since the caller's job is to report annotations that were quietly
// dropped. TestEveryAnnotatedStatementIsReachable pins that the reflective read
// reaches every such type. (upstream #884)
func StatementAnnotations(s MicroflowStatement) *ActivityAnnotations {
	if s == nil {
		return nil
	}
	v := reflect.ValueOf(s)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("Annotations")
	if !f.IsValid() || f.Kind() != reflect.Ptr || f.IsNil() {
		return nil
	}
	ann, _ := f.Interface().(*ActivityAnnotations)
	return ann
}

// SetStatementAnnotations attaches ann to a microflow statement and reports
// whether the statement type has anywhere to hold it.
//
// The write side of StatementAnnotations, reflective for the same reason. The
// visitor and the flow builder each used to carry a hand-written type switch
// here, and neither had a case for the eleven workflow statements, the three
// mapping statements or (on one side each) CASE and SEND REST REQUEST — so their
// @position was parsed and dropped, and rewriting a microflow from its own
// DESCRIBE output moved every one of those activities.
func SetStatementAnnotations(s MicroflowStatement, ann *ActivityAnnotations) bool {
	if s == nil {
		return false
	}
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return false
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("Annotations")
	if !f.IsValid() || !f.CanSet() || f.Type() != reflect.TypeOf(ann) {
		return false
	}
	f.Set(reflect.ValueOf(ann))
	return true
}

// StatementBodies returns every nested statement list a microflow statement
// contains — an IF's two branches, a CASE's arms, a loop body, an ON ERROR
// handler's body — so a check that has to span the whole flow can recurse
// without a type switch that goes stale.
//
// Reflective for the same reason as StatementAnnotations: a hand-written switch
// silently skips the statement type added after it was written, and the callers
// here are looking for something that would otherwise be missed entirely.
// TestStatementBodiesReachesEveryNestedBody pins the coverage.
func StatementBodies(s MicroflowStatement) [][]MicroflowStatement {
	if s == nil {
		return nil
	}
	v := reflect.ValueOf(s)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	var out [][]MicroflowStatement
	stmtSlice := reflect.TypeOf([]MicroflowStatement(nil))
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch {
		case f.Type() == stmtSlice:
			if f.Len() > 0 {
				out = append(out, f.Interface().([]MicroflowStatement))
			}
		case f.Kind() == reflect.Ptr && f.Type() == reflect.TypeOf((*ErrorHandlingClause)(nil)):
			if !f.IsNil() {
				if body := f.Interface().(*ErrorHandlingClause).Body; len(body) > 0 {
					out = append(out, body)
				}
			}
		case f.Kind() == reflect.Slice:
			// Case arms: []EnumSplitCase, []InheritanceSplitCase — each element
			// is a struct with its own Body.
			for j := 0; j < f.Len(); j++ {
				el := f.Index(j)
				if el.Kind() != reflect.Struct {
					break
				}
				body := el.FieldByName("Body")
				if body.IsValid() && body.Type() == stmtSlice && body.Len() > 0 {
					out = append(out, body.Interface().([]MicroflowStatement))
				}
			}
		}
	}
	return out
}
