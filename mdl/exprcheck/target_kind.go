// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "github.com/mendixlabs/mxcli/mdl/exprcheck/hints"

// checkTargetKind compares an expression with the declared type of the variable
// it initialises, the variable it is assigned to, or the flow it is returned from.
//
// `declare $X Integer = 'abc'`, `set $X = 'abc'` and a `return 'abc'` in a flow
// that returns Integer all passed `mxcli check` and `exec`, and failed the build:
// CE0117 "Error(s) in expression" on MxBuild, CE0844 "The expression is of type
// String but should be of type Integer/Long" on Mendix CLI 2. The slot table had
// declared all three (`DeclareType`, `TargetVariable`, `MicroflowReturn`) and
// nothing resolved them, because checkSlotKind skips every slot with a ResolveBy
// — the same gap #2096 closed for a call argument.
//
// The predicate is the one for call arguments, and for the same reason: the three
// positions were measured as a 6 x 12 matrix on MxBuild 11.16.1 (216 microflows,
// one `mx check`) and are identical to each other and to what callArgAccepted
// already encodes — Integer and Long pass into each other and widen into Decimal,
// every other primitive mismatch is CE0117. See TestTargetKindMatrix.
//
// Both sides must be a known primitive. A target of enumeration, object or list
// type, an expression of unknown kind and `empty` are silence, not an error:
// exec refuses what check reports. `empty` for a Boolean is a measured CE0117 and
// stays unreported here exactly as it does for a call argument.
func checkTargetKind(expr RobustExpr, ctx Context) []Hint {
	want := ctx.Target
	if !primitiveKind(want) {
		return nil
	}
	got := argKind(expr, ctx)
	if !primitiveKind(got) || callArgAccepted(want, "", got, "") {
		return nil
	}
	if coveredByDecimalIntoInteger(expr, ctx, want, got) {
		return nil
	}
	wantName, gotName := typeKindName(want), typeKindName(got)
	where := hintsLocation(ctx, expr.Pos())
	where.Context = "value for a " + wantName + " variable or return"
	return []Hint{{
		Code:     "E009",
		Slug:     "slot-type-mismatch",
		Severity: hints.SeverityError,
		Where:    where,
		YouWrote: "<" + gotName + ">",
		Problem: "This position holds " + withArticle(wantName) + ", but the expression is " +
			withArticle(gotName) + ". Mendix converts only from Integer or Long to Decimal (and " +
			"between Integer and Long); anything else is CE0117 \"Error(s) in expression\".",
		Fix: callArgFix(want, "", got),
	}}
}

func primitiveKind(k TypeKind) bool {
	switch k {
	case KindBoolean, KindString, KindInteger, KindLong, KindDecimal, KindDateTime:
		return true
	}
	return false
}

// coveredByDecimalIntoInteger reports whether MDL041 (mdl/executor,
// checkNumericAssignment) already reports this one. It flags a COMPUTED Decimal —
// arithmetic, or a Decimal-returning built-in such as secondsBetween() — assigned
// to an Integer or Long by DECLARE or SET, with a message that names the cause
// and without needing a project. Reporting it again here would print one problem
// twice. A bare Decimal literal or variable, and any RETURN, are not MDL041's, so
// they stay reported here.
func coveredByDecimalIntoInteger(expr RobustExpr, ctx Context, want, got TypeKind) bool {
	if got != KindDecimal || (want != KindInteger && want != KindLong) {
		return false
	}
	if base, _ := splitSlotQual(ctx.SlotPath); base != "DeclareStmt.InitialValue" && base != "MfSetStmt.Value" {
		return false
	}
	switch n := unwrapParens(expr).(type) {
	case *BinExpr:
		switch n.Op {
		case "+", "-", "*", "div", ":", "mod":
			return true
		}
	case *CallExpr:
		_, known := funcTable[n.Name]
		return known
	}
	return false
}
