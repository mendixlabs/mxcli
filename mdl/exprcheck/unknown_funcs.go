// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "strings"

// FuncRef identifies a function call found in an expression whose name is not a
// known Mendix built-in — a hallucinated or misspelled function (e.g. randomInt).
type FuncRef struct {
	Name       string
	Line       int
	Column     int
	Suggestion string // nearest known built-in, or "" if none is close
	Token      string // the [%Token%] the call stands in for, or "" (see tokenFuncs)
}

// tokenFuncs maps a call that reads like a built-in but is a Mendix token
// (lower-cased name) to that token. A did-you-mean on spelling cannot reach it,
// since the right answer is not a function. currentDateTime() was in funcTable
// and failed the build with CE0117 in a microflow and a nanoflow on 11.14.0,
// where [%CurrentDateTime%] builds at 0 errors. Add a row only when measured.
var tokenFuncs = map[string]string{
	"currentdatetime": "[%CurrentDateTime%]",
}

// UnknownFunctionCalls parses a Mendix expression and returns every call to a
// name that is not in the built-in function table (funcTable, which lists every
// Mendix expression function). A bare `name(...)` in a Mendix expression is
// always a built-in call — enumeration/entity references are qualified names, not
// calls — so an unknown name is a real error: it parses and passes a naive check
// but fails the build with CE0117 "Error(s) in expression". Findings #1.
func UnknownFunctionCalls(src string) []FuncRef {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	root, _ := (&parserImpl{}).Parse(src, Context{})
	if root == nil {
		return nil
	}
	var out []FuncRef
	walkCalls(root, func(c *CallExpr) {
		if c.Name == "" || c.Qualified {
			// A qualified call is never a built-in, so "did you mean toString()?"
			// would be nonsense; MDL066 reports it with the right fix.
			return
		}
		if _, ok := funcTable[c.Name]; ok {
			return
		}
		out = append(out, FuncRef{
			Name:       c.Name,
			Line:       c.Pos().Line,
			Column:     c.Pos().Column,
			Suggestion: nearestFunc(c.Name),
			Token:      tokenFuncs[strings.ToLower(c.Name)],
		})
	})
	return out
}

// walkCalls invokes fn for every CallExpr in the tree (depth-first).
func walkCalls(e RobustExpr, fn func(*CallExpr)) {
	switch n := e.(type) {
	case *CallExpr:
		fn(n)
		for _, a := range n.Args {
			walkCalls(a, fn)
		}
	case *BinExpr:
		walkCalls(n.L, fn)
		walkCalls(n.R, fn)
	case *UnaryExpr:
		walkCalls(n.Operand, fn)
	case *ParenExpr:
		walkCalls(n.Inner, fn)
	case *IfThenElseExpr:
		walkCalls(n.Cond, fn)
		walkCalls(n.Then, fn)
		walkCalls(n.Else, fn)
	}
}

// nearestFunc returns the built-in whose name is closest to name — a prefix
// relationship (randomInt→random) or a small edit distance — for a "did you
// mean" hint. Returns "" when nothing is close enough to be worth suggesting.
func nearestFunc(name string) string {
	lower := strings.ToLower(name)
	best, bestDist := "", 1<<30
	for cand := range funcTable {
		cl := strings.ToLower(cand)
		// A prefix relationship is a strong signal (randomInt / random).
		if strings.HasPrefix(lower, cl) || strings.HasPrefix(cl, lower) {
			return cand
		}
		d := levenshtein(lower, cl)
		if d < bestDist {
			best, bestDist = cand, d
		}
	}
	// Suggest only a genuinely close match: within a third of the name length
	// (min 2), so unrelated names get no misleading suggestion.
	threshold := len(name) / 3
	if threshold < 2 {
		threshold = 2
	}
	if bestDist <= threshold {
		return best
	}
	return ""
}

// levenshtein is the standard edit distance between two strings.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// SourceRejectedForIntegerTarget reports whether assigning src to an Integer/Long
// variable is rejected by Mendix (CE0117) because src is Decimal-typed. It covers
// two cases the build rejects but a naive check misses (findings #2):
//   - a bare arithmetic Decimal result (e.g. `$a * 100 div $b`); and
//   - a call to a Decimal-returning built-in (e.g. random(), secondsBetween(),
//     sqrt(), two-argument round()).
//
// Both answers come from inferKind, so a rounding function is Integer-valued
// wherever it appears rather than only as the whole right-hand side. Special-
// casing it at the root was the sudoku #49 defect: `round($a div $b)` was
// accepted and `round($a div $b) * 3` — which mxbuild builds at 0 errors — was
// refused, because the inner div's Decimal taint propagated back out through a
// round() the table still declared Decimal. The same shortcut let the genuinely
// Decimal `round($a div $b, 2)` through.
func SourceRejectedForIntegerTarget(src string, vars map[string]TypeKind) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	ctx := Context{}
	if vars != nil {
		ctx.Scope = mapScope(vars)
	}
	root, _ := (&parserImpl{}).Parse(src, ctx)
	root = unwrapParens(root)
	switch n := root.(type) {
	case *BinExpr:
		switch n.Op {
		case "+", "-", "*", "div", ":", "mod":
			return inferKind(n, ctx) == KindDecimal
		}
	case *CallExpr:
		if _, ok := funcTable[n.Name]; ok {
			return inferKind(n, ctx) == KindDecimal
		}
	}
	return false
}

// ToStringOfDecimal reports whether src is `toString(x)` with x provably a
// Decimal: a Decimal-returning built-in such as minutesBetween(), Decimal
// arithmetic, or a Decimal variable from vars. Mendix renders such a value with
// its full fraction — 43 characters were seen at runtime for a minutesBetween —
// so writing it to a short String attribute overflows (mendixlabs/mxcli#1515).
// Unknown inference answers false.
func ToStringOfDecimal(src string, vars map[string]TypeKind) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	ctx := Context{}
	if vars != nil {
		ctx.Scope = mapScope(vars)
	}
	root, _ := (&parserImpl{}).Parse(src, ctx)
	call, ok := unwrapParens(root).(*CallExpr)
	if !ok || call.Name != "toString" || call.Qualified || len(call.Args) != 1 {
		return false
	}
	return inferKind(call.Args[0], ctx) == KindDecimal
}
