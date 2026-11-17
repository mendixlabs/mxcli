// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// retrieveSingleRule is the rule ID for "an object-range retrieve used as a list".
const retrieveSingleRule = "MDL-RETRIEVE01"

// checkRetrieveLimitOneAsList flags a variable that a retrieve bound to a single
// OBJECT and a later statement uses as a LIST.
//
// The object range is Mendix's "First object" range, written `retrieve … first`
// in every language version, and `retrieve … limit 1` in a script without the
// `mdl 1;` header, where `limit 1` keeps its alpha meaning (ako/mxcli#734). The
// visitor resolves both to RetrieveStmt.First, which is what the writer stores
// too, so the rule keys on it and never on limit text: a check that disagrees
// with the writer it describes is worse than no check.
//
// Before #734 this rule was the only sign of the object: describe re-emitted
// `limit 1` for it, so an object retrieve and a list retrieve were identical text,
// and the first thing the author saw was CE0097 from mxbuild — inside a
// .test.mdl, not even that (mendixlabs/mxcli#1103). describe now prints `first`
// and an mdl 0 `limit 1` warns MDL-V1-LIMIT1, so the spelling says it; the rule
// stays for the mistake that is left, treating an object as a list, which
// check --references does not otherwise catch before the build.
func (v *microflowValidator) checkRetrieveLimitOneAsList(body []ast.MicroflowStatement) {
	// single holds the variables currently bound to one object by an
	// object-range retrieve. Maintained in statement order so a rebinding
	// clears it: a name reused for a real list further down is not this rule's
	// business.
	single := map[string]bool{}

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if name, op := listUseOf(s); name != "" && single[name] {
			// Measured on 11.13: a loop reports CE0100, a list activity CE0097.
			rejection := fmt.Sprintf("CE0097 \"The selected '%s' variable must be of type List\"", name)
			if op == "a loop" {
				rejection = fmt.Sprintf("CE0100 \"'%s' is of type …, but should be of type List\"", name)
			}
			v.addViolation(retrieveSingleRule, linter.SeverityError,
				fmt.Sprintf("$%s was retrieved as a single object (`first`, which is also what `limit 1` "+
					"means in a script without the `mdl 1;` header), not a list, so %s cannot take it — "+
					"mxbuild rejects this with %s.", name, op, rejection),
				fmt.Sprintf("Retrieve a list instead (no range, or under `mdl 1;` `limit 1` for a list of "+
					"one) and keep %s, or keep the object range and use $%s as the object it already is.",
					op, name))
		}

		// Rebinding first, so a statement that both consumes and produces the
		// name is judged on what it consumed.
		for _, p := range statementProducedVars(s) {
			delete(single, p.name)
		}
		if r, ok := s.(*ast.RetrieveStmt); ok && r.First && r.StartVariable == "" && r.Variable != "" {
			single[r.Variable] = true
		}
	})
}

// retrieveListAsObjectRule is the rule ID for "a list retrieve used as an object".
const retrieveListAsObjectRule = "MDL-RETRIEVE03"

// checkRetrieveListAsObject is MDL-RETRIEVE01 the other way round: a variable a
// database retrieve bound to a LIST — no range, `limit n [offset n]`, and under
// `mdl 1;` a bare `limit 1`, a list of one (ako/mxcli#734) — that a later
// `change` takes as the object. mxcli used to write the member qualified
// through the list type, and mxbuild answered CE1613 "The selected attribute
// 'List of Module.Entity.Attr' no longer exists" (mendixlabs/mxcli#1422).
//
// Only a database retrieve is judged here: a retrieve over an association binds
// an object or a list depending on the association's type and owner and on
// which end the start variable is, which needs the project. check --references
// judges those with the same walk (validateAssociationRetrieveListUse,
// mendixlabs/mxcli#1516). A member path `$E/Attr` in any expression of a later
// statement is the same mistake (CE0117 "Error(s) in expression.", measured on
// 11.6.6). `set $E = empty` over the list is a Change list Replace and builds
// (measured), so it is not flagged.
func (v *microflowValidator) checkRetrieveListAsObject(body []ast.MicroflowStatement) {
	list := map[string]bool{}
	walkListObjectUses(body, func(name string) bool { return list[name] },
		func(_ ast.MicroflowStatement, name string, use listObjectUse) {
			switch use {
			case listUseChange:
				v.addViolation(retrieveListAsObjectRule, linter.SeverityError,
					fmt.Sprintf("$%s was retrieved as a list (under `mdl 1;` `limit 1` is a list of one), "+
						"not a single object, so `change $%s` cannot take it — mxbuild rejects the stored member "+
						"with CE1613 \"The selected attribute 'List of …' no longer exists\".", name, name),
					fmt.Sprintf("Retrieve the object with `retrieve $%s from … first;`, or keep the list and "+
						"change its items inside a `loop`.", name))
			case listUseMemberPath:
				v.addViolation(retrieveListAsObjectRule, linter.SeverityError,
					fmt.Sprintf("$%s was retrieved as a list (under `mdl 1;` `limit 1` is a list of one), "+
						"not a single object, so a member path `$%s/…` has nothing to navigate — mxbuild "+
						"rejects the expression with CE0117 \"Error(s) in expression.\".", name, name),
					fmt.Sprintf("Retrieve the object with `retrieve $%s from … first;`, or take one item "+
						"of the list first (`$item = head($%s);`).", name, name))
			}
		},
		func(s ast.MicroflowStatement) {
			// `return $L` from a flow whose return type is not a list
			// (ako/mxcli#2364): mxbuild reports CE0117 "Error(s) in
			// expression." at the End event.
			ret, ok := s.(*ast.ReturnStmt)
			if !ok || v.returnType == nil || v.returnType.Type.Kind == ast.TypeListOf || v.returnType.Type.Kind == ast.TypeVoid {
				return
			}
			ve, ok := ret.Value.(*ast.VariableExpr)
			if !ok || !list[ve.Name] {
				return
			}
			v.addViolation(retrieveListAsObjectRule, linter.SeverityError,
				fmt.Sprintf("$%s was retrieved as a list (under `mdl 1;` `limit 1` is a list of one), "+
					"but the %s returns %s, a single value — mxbuild rejects the return with CE0117 "+
					"\"Error(s) in expression.\" at the End event.", ve.Name, v.docType, returnTypeName(v.returnType.Type)),
				fmt.Sprintf("Retrieve the object with `retrieve $%s from … first;`, return `head($%s)`, "+
					"or declare the return type as `list of …`.", ve.Name, ve.Name))
		},
		func(s ast.MicroflowStatement) {
			for _, p := range statementProducedVars(s) {
				delete(list, p.name)
			}
			if r, ok := s.(*ast.RetrieveStmt); ok && !r.First && r.StartVariable == "" && r.Variable != "" {
				list[r.Variable] = true
			}
		})
}

// listObjectUse is how a statement takes a list variable as one object.
type listObjectUse int

const (
	listUseChange     listObjectUse = iota // `change $L (…)`
	listUseMemberPath                      // `$L/Attr` in one of the statement's expressions
)

// walkListObjectUses is the walk MDL-RETRIEVE03 runs, with and without a
// project. In statement order (nested bodies included) it reports each `change`
// of, and each member path through, a variable isList says holds a list — at
// most one member-path report per variable per statement. other, when set, runs
// next for checks the caller adds, and rebind last, so a statement that both
// consumes and produces a name is judged on what it consumed.
func walkListObjectUses(body []ast.MicroflowStatement, isList func(string) bool,
	use func(ast.MicroflowStatement, string, listObjectUse),
	other func(ast.MicroflowStatement), rebind func(ast.MicroflowStatement)) {

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if c, ok := s.(*ast.ChangeObjectStmt); ok && isList(c.Variable) {
			use(s, c.Variable, listUseChange)
		}
		reported := map[string]bool{}
		for _, e := range statementOwnExpressions(s) {
			for _, name := range predicatePathVariables(e) {
				if !isList(name) || reported[name] {
					continue
				}
				reported[name] = true
				use(s, name, listUseMemberPath)
			}
		}
		if other != nil {
			other(s)
		}
		rebind(s)
	})
}

// statementOwnExpressions returns the expressions a statement holds itself, in
// any field or slice of structs, but not those of the statements nested in its
// bodies (forEachMicroflowStatement visits those on their own).
func statementOwnExpressions(s ast.MicroflowStatement) []ast.Expression {
	exprType := reflect.TypeOf((*ast.Expression)(nil)).Elem()
	stmtType := reflect.TypeOf((*ast.MicroflowStatement)(nil)).Elem()
	var out []ast.Expression
	var walk func(rv reflect.Value, top bool)
	walk = func(rv reflect.Value, top bool) {
		switch rv.Kind() {
		case reflect.Interface:
			if rv.IsNil() {
				return
			}
			if rv.Type() == stmtType && !top {
				return
			}
			if rv.Type() == exprType {
				out = append(out, rv.Interface().(ast.Expression))
				return
			}
			walk(rv.Elem(), top)
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if !top && rv.Type().Implements(stmtType) {
				return
			}
			walk(rv.Elem(), false)
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i), false)
				}
			}
		case reflect.Slice:
			if rv.Type().Elem() == stmtType {
				return
			}
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i), false)
			}
		}
	}
	walk(reflect.ValueOf(s), true)
	return out
}

// listUseOf reports the list variable a statement consumes, and a phrase naming
// what consumes it. ("", "") when the statement takes no list.
func listUseOf(s ast.MicroflowStatement) (string, string) {
	switch st := s.(type) {
	case *ast.ListOperationStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
		if st.SecondVariable != "" {
			return st.SecondVariable, st.Operation.String() + "()"
		}
	case *ast.AggregateListStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
	case *ast.LoopStmt:
		if st.ListVariable != "" {
			return st.ListVariable, "a loop"
		}
	case *ast.AddToListStmt:
		// An association target (`$Obj/Assoc`) takes the object (#1288).
		if st.List != "" && st.Association == "" {
			return st.List, "ADD … TO"
		}
	case *ast.RemoveFromListStmt:
		if st.List != "" && st.Association == "" {
			return st.List, "REMOVE … FROM"
		}
	case *ast.ClearListStmt:
		if st.List != "" {
			return st.List, "CLEAR"
		}
	}
	return "", ""
}

// returnTypeName names a declared return type for a message. A bare
// `Module.Name` is parsed as an enumeration-or-entity reference, which
// formatDataTypeForMDL does not name.
func returnTypeName(dt ast.DataType) string {
	switch {
	case dt.EntityRef != nil:
		return dt.EntityRef.String()
	case dt.EnumRef != nil:
		return dt.EnumRef.String()
	}
	return formatDataTypeForMDL(dt)
}
