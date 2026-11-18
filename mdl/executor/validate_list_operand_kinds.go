// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// listOperandKindRule is the rule ID for "a primitive where a list operation
// needs an object or a list".
const listOperandKindRule = "MDL101"

// checkListOperandKinds reports a list operation whose second operand is a
// variable of a primitive type — CE0023 "Selected variable 'X' must be of type
// Object or List."
//
// Measured on MxBuild 11.16.1, microflows and nanoflows, one flow per case: for
// UNION, INTERSECT, SUBTRACT, EQUALS and CONTAINS the operand that is not the
// list must be an Object or a List, and every primitive type — String, Integer,
// Long, Decimal, Boolean, DateTime, enumeration — is CE0023 there. An Object or
// a List is clean (`union $L with $O` builds). `check` and `exec` accepted every
// rejected case and wrote the activity.
//
// Only a parameter or declared variable of a known primitive kind is judged. A
// variable whose kind is not known here is never reported, and a primitive FIRST
// operand is a different error (CE0097), left to its own rule.
//
// `set $M = contains($Hay, $Needle)` over two Strings parses as the same node,
// and builds as the String function (addListOperationAction rewrites it when the
// haystack is a declared String): the needle being a String is right there, so a
// String haystack is skipped, by the same test the builder and MDL063 share.
func (v *microflowValidator) checkListOperandKinds(body []ast.MicroflowStatement) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		lo, ok := s.(*ast.ListOperationStmt)
		if !ok || lo.SecondVariable == "" {
			return
		}
		switch lo.Operation {
		case ast.ListOpUnion, ast.ListOpIntersect, ast.ListOpSubtract, ast.ListOpEquals:
		case ast.ListOpContains:
			if stringOverloadedListOp(lo.Operation) && v.varKinds[lo.InputVariable] == exprcheck.KindString {
				return
			}
		default:
			return
		}
		kind, known := v.varKinds[lo.SecondVariable]
		if !known || !isPrimitiveOperandKind(kind) {
			return
		}
		v.addViolation(listOperandKindRule, linter.SeverityError,
			fmt.Sprintf("$%s is a %s, but %s needs an object or a list there — mxbuild rejects this with "+
				"CE0023 \"Selected variable '%s' must be of type Object or List.\" (measured for every primitive type)",
				lo.SecondVariable, kindName(kind), listOperationWord(lo.Operation), lo.SecondVariable),
			"Pass an object or a list variable; to test a primitive value, compare it in an expression instead")
	})
}

// isPrimitiveOperandKind is the set measured as CE0023: Binary is left out
// because it was not measured, and anything else is not a primitive value.
func isPrimitiveOperandKind(k exprcheck.TypeKind) bool {
	switch k {
	case exprcheck.KindString, exprcheck.KindInteger, exprcheck.KindLong, exprcheck.KindDecimal,
		exprcheck.KindBoolean, exprcheck.KindDateTime, exprcheck.KindEnumeration:
		return true
	}
	return false
}

func kindName(k exprcheck.TypeKind) string {
	switch k {
	case exprcheck.KindString:
		return "String"
	case exprcheck.KindInteger:
		return "Integer"
	case exprcheck.KindLong:
		return "Long"
	case exprcheck.KindDecimal:
		return "Decimal"
	case exprcheck.KindBoolean:
		return "Boolean"
	case exprcheck.KindDateTime:
		return "DateTime"
	case exprcheck.KindEnumeration:
		return "enumeration value"
	}
	return "primitive value"
}

func listOperationWord(op ast.ListOperationType) string {
	switch op {
	case ast.ListOpUnion:
		return "UNION"
	case ast.ListOpIntersect:
		return "INTERSECT"
	case ast.ListOpSubtract:
		return "SUBTRACT"
	case ast.ListOpEquals:
		return "EQUALS"
	case ast.ListOpContains:
		return "CONTAINS"
	}
	return "this list operation"
}
