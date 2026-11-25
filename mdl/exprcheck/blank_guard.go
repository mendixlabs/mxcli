// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

// checkBlankGuards reports the two blank checks around E015 that build and
// mostly behave, so are info (ako/mxcli#2566):
//
//   - E016, `$v != empty and trim($v) != ”`: the `empty` half is dead.
//     trim(empty) is ” (measured, mendixlabs/mxcli#580, and on 11.12.3), so
//     trim($v) != ” is already false for empty and decides the condition
//     alone. Any function in neverEmptyStringFuncs whose first argument is $v
//     does the same.
//   - E017, `$v != empty and $v != ”`: a value of only spaces passes as
//     "has text". The blank check is trim($v) != ”.
//
// The same in an `or` chain with `=`: `$v = empty or trim($v) = ”`. Only
// operands of one chain count, as for E015.
func checkBlankGuards(expr RobustExpr, ctx Context) []Hint {
	switch n := expr.(type) {
	case *BinExpr:
		if n.Op == "AND" || n.Op == "OR" {
			operands := flattenChain(n, n.Op, nil)
			hs := blankGuardHints(operands, n.Op, ctx)
			for _, o := range operands {
				hs = append(hs, checkBlankGuards(o, ctx)...)
			}
			return hs
		}
		return append(checkBlankGuards(n.L, ctx), checkBlankGuards(n.R, ctx)...)
	case *ParenExpr:
		return checkBlankGuards(n.Inner, ctx)
	case *UnaryExpr:
		return checkBlankGuards(n.Operand, ctx)
	case *IfThenElseExpr:
		hs := checkBlankGuards(n.Cond, ctx)
		hs = append(hs, checkBlankGuards(n.Then, ctx)...)
		return append(hs, checkBlankGuards(n.Else, ctx)...)
	case *CallExpr:
		var hs []Hint
		for _, a := range n.Args {
			hs = append(hs, checkBlankGuards(a, ctx)...)
		}
		return hs
	}
	return nil
}

// blankGuardHints judges one chain: each `<x> != empty` (`= empty` in an or
// chain) beside a comparison of the same <x>, or of a never-empty call on it,
// with ”.
func blankGuardHints(chain []RobustExpr, chainOp string, ctx Context) []Hint {
	cmpOp := "!="
	if chainOp == "OR" {
		cmpOp = "="
	}
	var out []Hint
	for _, o := range chain {
		guarded, guard := comparedWith(o, cmpOp, func(e RobustExpr) bool { _, ok := e.(*EmptyExpr); return ok })
		if guarded == nil {
			continue
		}
		key := exprKey(guarded)
		var viaCall *CallExpr
		bare := false
		for _, other := range chain {
			side, _ := comparedWith(other, cmpOp, func(e RobustExpr) bool {
				lit, ok := e.(*StringLit)
				return ok && lit.Value == ""
			})
			if side == nil {
				continue
			}
			if exprKey(side) == key {
				bare = true
			} else if call, ok := side.(*CallExpr); ok && !call.Qualified && neverEmptyStringFuncs[call.Name] &&
				len(call.Args) > 0 && exprKey(unwrapParens(call.Args[0])) == key {
				viaCall = call
			}
		}
		text := renderOperand(guarded)
		switch {
		case viaCall != nil:
			out = append(out, Hint{
				Code: "E016", Slug: "redundant-empty-guard", Severity: hints.SeverityInfo,
				Where:    hintsLocation(ctx, guard.Pos()),
				YouWrote: text + " " + cmpOp + " empty, beside " + viaCall.Name + "(...) " + cmpOp + " ''",
				Problem: viaCall.Name + "(empty) is '', so " + viaCall.Name + "(...) " + cmpOp +
					" '' already decides it; the comparison with empty adds nothing.",
				Fix: "Keep only " + callSpelling(viaCall, text) + " " + cmpOp + " ''",
			})
		case bare:
			out = append(out, Hint{
				Code: "E017", Slug: "blank-check-without-trim", Severity: hints.SeverityInfo,
				Where:    hintsLocation(ctx, guard.Pos()),
				YouWrote: text + " " + cmpOp + " empty, beside " + text + " " + cmpOp + " ''",
				Problem:  "a value of only spaces is neither empty nor '', so it counts as text here.",
				Fix:      "Check the trimmed value instead: trim(" + text + ") " + cmpOp + " ''",
			})
		}
	}
	return out
}

// comparedWith returns the operand o compares, with op, against a value it
// accepts, and the comparison itself; nil when o is not such a comparison.
func comparedWith(o RobustExpr, op string, is func(RobustExpr) bool) (RobustExpr, *BinExpr) {
	b, ok := unwrapParens(o).(*BinExpr)
	if !ok || b.Op != op {
		return nil, nil
	}
	if is(unwrapParens(b.R)) {
		return unwrapParens(b.L), b
	}
	if is(unwrapParens(b.L)) {
		return unwrapParens(b.R), b
	}
	return nil, nil
}

// renderOperand spells a variable or attribute path back as written.
func renderOperand(e RobustExpr) string {
	switch n := e.(type) {
	case *VariableExpr:
		return "$" + n.Name
	case *AttributePathExpr:
		s := "$" + n.Variable
		for _, p := range n.Path {
			s += "/" + p
		}
		return s
	}
	return "the value"
}

// callSpelling writes call over the operand text, its further arguments
// elided: trim($v), substring($v, …).
func callSpelling(call *CallExpr, text string) string {
	if len(call.Args) > 1 {
		return call.Name + "(" + text + ", …)"
	}
	return call.Name + "(" + text + ")"
}
