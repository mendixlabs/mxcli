// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

// CallArgumentSlot is the slot path of one call microflow / call nanoflow
// argument, qualified with the parameter it is passed to:
// "CallArgument.Value:Mod.Flow(Param)". It is the TargetParameter resolution the
// expectations table declares, in the same shape the adapter already uses for
// AttributeOf:Parent ("ChangeItem.Value:Mod.Entity.Attr").
func CallArgumentSlot(flowQN, param string) string {
	param = strings.TrimPrefix(param, "$")
	if flowQN == "" || param == "" {
		return "CallArgument.Value"
	}
	return "CallArgument.Value:" + flowQN + "(" + param + ")"
}

// splitCallArgumentSlot reverses CallArgumentSlot.
func splitCallArgumentSlot(slotPath string) (flowQN, param string, ok bool) {
	base, qual := splitSlotQual(slotPath)
	if base != "CallArgument.Value" || !strings.HasSuffix(qual, ")") {
		return "", "", false
	}
	flowQN, param, ok = strings.Cut(strings.TrimSuffix(qual, ")"), "(")
	if !ok || flowQN == "" || param == "" {
		return "", "", false
	}
	return flowQN, param, true
}

// checkCallArgumentKind compares a call microflow / call nanoflow argument with
// the type of the parameter it is passed to (ako/mxcli#2096,
// mendixlabs/mxcli#1797).
//
// The expectations table had declared `CallArgument.Value` as resolved by
// "TargetParameter" all along, but nothing resolved it and checkSlotKind skips
// every slot with a ResolveBy — so `call microflow M.F (Qty = '5')` against an
// Integer parameter passed check and exec, and failed the build with CE0117
// "Error(s) in expression" at the call activity.
//
// The predicate is the matrix measured on mxbuild 11.14.0 (9 parameter types x
// 19 argument shapes): Integer and Long pass into each other and widen into
// Decimal, every other primitive mismatch is CE0117, an enumeration must be the
// SAME enumeration, and an object or list parameter takes no primitive. Three
// answers, as everywhere in this checker: anything it cannot establish —
// a flow or parameter the catalog does not know, an argument of unknown kind —
// is silence, not an error, because exec refuses what check reports.
//
// Two measured CE0117s are deliberately NOT reported:
//   - `empty` for a Boolean parameter fails the build, but `empty` is accepted
//     for every other type and is Mendix's null; refusing it needs its own
//     measurement across versions rather than a corner of this rule.
//   - an object of another entity, and a list where an object is wanted: the
//     entity scope cannot tell an object variable from a list variable, and
//     specialisation makes entity identity a hierarchy question. Silence.
func checkCallArgumentKind(expr RobustExpr, ctx Context) []Hint {
	if ctx.Catalog == nil {
		return nil
	}
	flowQN, param, ok := splitCallArgumentSlot(ctx.SlotPath)
	if !ok {
		return nil
	}
	want, ok := ctx.Catalog.MicroflowParam(flowQN, param)
	if !ok || want == KindUnknown || want == KindAny {
		return nil
	}
	wantRef := ""
	if rr, ok := ctx.Catalog.(FlowParamRefReader); ok {
		wantRef, _ = rr.MicroflowParamRef(flowQN, param)
	}
	got := argKind(expr, ctx)
	gotEnum := exprEnumQN(expr, ctx)
	if gotEnum != "" {
		// Settles an enumeration variable the scope could only call Object.
		got = KindEnumeration
	}
	if callArgAccepted(want, wantRef, got, gotEnum) {
		return nil
	}

	wantName := callArgTypeName(want, wantRef)
	gotName := callArgTypeName(got, gotEnum)
	where := hintsLocation(ctx, expr.Pos())
	where.Context = "argument $" + param + " of the call to " + flowQN
	return []Hint{{
		Code:     "E009",
		Slug:     "slot-type-mismatch",
		Severity: hints.SeverityError,
		Where:    where,
		YouWrote: "$" + param + " = <" + gotName + ">",
		Problem: "Parameter $" + param + " of " + flowQN + " is " + withArticle(wantName) +
			", but the argument passed to it is " + withArticle(gotName) + ". Mendix converts a call " +
			"argument only from Integer or Long to Decimal (and between Integer and Long); anything else " +
			"is CE0117 \"Error(s) in expression\" at the call activity.",
		Fix: callArgFix(want, wantRef, got),
	}}
}

// callArgAccepted reports whether an argument may be passed to a parameter.
// It refuses only what the 11.14.0 matrix showed to be CE0117.
func callArgAccepted(want TypeKind, wantRef string, got TypeKind, gotRef string) bool {
	switch got {
	case KindUnknown, KindEmpty, KindAny:
		return true
	}
	primitive := func(k TypeKind) bool {
		switch k {
		case KindBoolean, KindString, KindInteger, KindLong, KindDecimal, KindDateTime:
			return true
		}
		return false
	}
	switch {
	case primitive(want):
		switch {
		case primitive(got):
			// The same numeric widening the built-in argument check measured
			// (mendixlabs/mxcli#1515) — one rule, so the two cannot drift.
			return argAccepted([]TypeKind{want}, got)
		case got == KindEnumeration, got == KindObject:
			// Measured CE0117 for every primitive parameter. An Object here may
			// really be a bare-named enumeration variable, which fails too.
			return false
		}
		return true
	case want == KindEnumeration:
		switch {
		case primitive(got):
			return false
		case got == KindEnumeration:
			// Only two KNOWN, different enumerations are a mismatch.
			return wantRef == "" || gotRef == "" || wantRef == gotRef
		}
		// An Object may be a bare-named enumeration variable: cannot tell.
		return true
	case want == KindObject || want == KindList:
		return !primitive(got) && got != KindEnumeration
	}
	return true
}

// exprEnumQN returns the enumeration an argument's value belongs to, when the
// argument names one outright: a value literal (Mod.Enum.Value), an
// enumeration variable, or an attribute path landing on an enumeration.
func exprEnumQN(e RobustExpr, ctx Context) string {
	switch n := unwrapParens(e).(type) {
	case *QNameExpr:
		if n.Sub == "" || ctx.Catalog == nil {
			return ""
		}
		qn := n.Module + "." + n.Name
		if _, ok := ctx.Catalog.EnumCases(qn); ok {
			return qn
		}
	case *VariableExpr:
		// The entity scope records the type name an object OR enumeration
		// variable was declared with — in a microflow signature `enum M.E`
		// does not set ExplicitEnum, so the kind scope does not say which.
		// Document names are unique within a module, so a name the catalog
		// knows as an enumeration is one.
		if ctx.Entities == nil || ctx.Catalog == nil {
			return ""
		}
		if qn, ok := ctx.Entities.VariableEntity(n.Name); ok {
			if _, isEnum := ctx.Catalog.EnumCases(qn); isEnum {
				return qn
			}
		}
	case *AttributePathExpr:
		if qn, ok := attributePathEnumQN(n, ctx); ok {
			return qn
		}
	}
	return ""
}

func callArgTypeName(k TypeKind, ref string) string {
	name := typeKindName(k)
	if ref == "" {
		return name
	}
	switch k {
	case KindEnumeration, KindObject:
		return name + " " + ref
	case KindList:
		return "List of " + ref
	}
	return name
}

func callArgFix(want TypeKind, wantRef string, got TypeKind) string {
	switch {
	case want == KindEnumeration && wantRef != "":
		return "Pass a value of " + wantRef + " (" + wantRef + ".<Value>), a variable or attribute of that enumeration, or empty."
	case want == KindString:
		return "Convert the value, e.g. with toString(...)."
	case (want == KindInteger || want == KindLong) && got == KindDecimal:
		return "Round the value (round/floor/ceil), or change the parameter to Decimal."
	case (want == KindInteger || want == KindLong || want == KindDecimal) && got == KindString:
		return "Pass a number, or convert the text with parseInteger(...) / parseDecimal(...)."
	case want == KindObject || want == KindList:
		return "Pass an object variable (or list) of the parameter's entity, or empty."
	}
	return "Pass an expression of type " + callArgTypeName(want, wantRef) + "."
}
