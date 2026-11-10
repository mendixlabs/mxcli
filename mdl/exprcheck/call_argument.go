// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"slices"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

// ParamEnumResolver is the optional half of a CatalogReader that names the
// enumeration an enumeration parameter takes. CatalogReader does not require
// it, so a reader without it leaves enumeration arguments compared by kind only.
type ParamEnumResolver interface {
	MicroflowParamEnumQN(qn, paramName string) (string, bool)
}

// callArgumentSlot is the base of the slot a CALL MICROFLOW / CALL NANOFLOW
// argument is checked in. The adapter appends the target as
// ":<Module.Flow>.<Param>", the shape CreateItem.Value uses for an attribute.
const callArgumentSlot = "CallArgument.Value"

// checkCallArgument compares a call argument's kind with the called flow's
// parameter.
//
// The slot table declares CallArgument.Value as ResolveBy "TargetParameter",
// and checkSlotKind skips every such slot, so nothing ever compared the two: a
// String passed to an Integer parameter checked clean and arrived as CE0117
// "Error(s) in expression" on the call activity.
//
// What is accepted follows what mxbuild does, measured on 11.14.0 for both a
// microflow and a nanoflow call: Integer and Long are interchangeable, both
// widen to Decimal, a Decimal for an Integer or Long is CE0117, and String,
// Boolean and DateTime take only themselves. An enumeration parameter takes
// only a value of that enumeration — a String, an Integer or a value of
// another enumeration is CE0117. An object or list parameter refuses every
// primitive. `empty` is accepted everywhere.
//
// What is not checked: which entity an object or list argument holds. A
// specialization is accepted where its generalization is expected, and the
// flow scope cannot tell an object variable from a list variable, so an object
// or list argument for an object or list parameter is never reported.
func checkCallArgument(expr RobustExpr, ctx Context) []Hint {
	base, qual := splitSlotQual(ctx.SlotPath)
	if base != callArgumentSlot || qual == "" || ctx.Catalog == nil {
		return nil
	}
	flow, param := splitEntityAttr(qual)
	if flow == "" || param == "" {
		return nil
	}
	want, ok := ctx.Catalog.MicroflowParam(flow, param)
	if !ok {
		return nil
	}
	got, gotEnum := callArgumentKind(expr, ctx)
	wantEnum := ""
	if er, ok := ctx.Catalog.(ParamEnumResolver); ok && want == KindEnumeration {
		wantEnum, _ = er.MicroflowParamEnumQN(flow, param)
	}
	switch {
	case !callArgumentAccepted(want, got):
	case want == KindEnumeration && gotEnum != "" && wantEnum != "" && gotEnum != wantEnum:
	default:
		return nil
	}
	return []Hint{{
		Code:     "E009",
		Slug:     "slot-type-mismatch",
		Severity: hints.SeverityError,
		Where:    hintsLocation(ctx, expr.Pos()),
		YouWrote: "<" + paramKindName(got, gotEnum) + ">",
		Problem: "Argument " + param + " of the call to " + flow + ": the parameter is " +
			paramKindName(want, wantEnum) + ", but the argument is " + paramKindName(got, gotEnum) +
			". Mendix does not convert it — MxBuild rejects the call with CE0117 \"Error(s) in expression\".",
		Fix: "Pass an expression of the parameter's type, or convert it (toString, parseInteger, …).",
	}}
}

// paramKindName names a kind the way mxbuild names a parameter type: Integer
// and Long are one type there ("Integer/Long"), and an enumeration is named
// with its enumeration when known.
func paramKindName(k TypeKind, enumQN string) string {
	switch {
	case k == KindInteger || k == KindLong:
		return "Integer/Long"
	case k == KindEnumeration && enumQN != "":
		return "Enumeration " + enumQN
	}
	return typeKindName(k)
}

// callArgumentKind types an argument, naming its enumeration when it is a
// value the catalog can place: `Mod.Enum.Value` or an enumeration attribute
// path. A qualified name that is not a known value of a known enumeration is
// left Unknown.
//
// A [%Token%] anywhere but the top makes the argument Unknown: inferKind
// calls every token a String, which `if $b then [%CurrentDateTime%] else empty`
// would turn into a DateTime argument reported as text. argKind types the
// date tokens when the token is the whole argument.
func callArgumentKind(expr RobustExpr, ctx Context) (TypeKind, string) {
	e := unwrapParens(expr)
	switch n := e.(type) {
	case *TokenExpr:
		return argKind(n, ctx), ""
	case *QNameExpr:
		if n.Name == "" || n.Sub == "" {
			return KindUnknown, ""
		}
		enumQN := n.Module + "." + n.Name
		if cases, ok := ctx.Catalog.EnumCases(enumQN); ok && slices.Contains(cases, n.Sub) {
			return KindEnumeration, enumQN
		}
		return KindUnknown, ""
	case *VariableExpr:
		// A parameter or variable typed with a bare `Mod.Name` is recorded as
		// holding entity Mod.Name whether that is an entity or an enumeration,
		// and inferKind calls it an Object. Ask the catalog which it is: as an
		// Object it would be reported against an enumeration parameter it fits.
		if ctx.Entities != nil {
			if qn, ok := ctx.Entities.VariableEntity(n.Name); ok {
				if _, isEnum := ctx.Catalog.EnumCases(qn); isEnum {
					return KindEnumeration, qn
				}
			}
		}
	case *AttributePathExpr:
		k := attributePathKind(n, ctx)
		if k == KindEnumeration {
			enumQN, _ := attributePathEnumQN(n, ctx)
			return k, enumQN
		}
		return k, ""
	}
	if containsToken(e) {
		return KindUnknown, ""
	}
	return inferKind(e, ctx), ""
}

// callArgumentAccepted reports whether an argument of kind got may be passed
// for a parameter of kind want. Only what was measured to fail is refused; an
// argument or a parameter of a kind not listed is accepted.
func callArgumentAccepted(want, got TypeKind) bool {
	switch got {
	case KindBoolean, KindString, KindInteger, KindLong, KindDecimal, KindDateTime,
		KindEnumeration, KindObject, KindList:
	default:
		// Unknown, Empty, Any, Binary: nothing to prove.
		return true
	}
	switch want {
	case KindInteger, KindLong:
		return got == KindInteger || got == KindLong
	case KindDecimal:
		return got == KindInteger || got == KindLong || got == KindDecimal
	case KindString, KindBoolean, KindDateTime, KindEnumeration:
		return got == want
	case KindObject, KindList:
		// The flow scope records list variables as entity variables, so an
		// Object here may be a list: only a primitive is provably wrong.
		return got == KindObject || got == KindList
	}
	return true
}

// containsToken reports whether a [%Token%] occurs anywhere in e.
func containsToken(e RobustExpr) bool {
	switch n := e.(type) {
	case *TokenExpr:
		return true
	case *CallExpr:
		return slices.ContainsFunc(n.Args, containsToken)
	case *BinExpr:
		return containsToken(n.L) || containsToken(n.R)
	case *UnaryExpr:
		return containsToken(n.Operand)
	case *ParenExpr:
		return containsToken(n.Inner)
	case *IfThenElseExpr:
		return containsToken(n.Cond) || containsToken(n.Then) || containsToken(n.Else)
	}
	return false
}
