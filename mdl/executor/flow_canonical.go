// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/mendixlabs/mxcli/mdl/ast"

// # One control-flow form for both sides of a diff (ako/mxcli#859)
//
// diff-then-patch matches the statements a script declares against the stored
// flow's description. MDL can spell one graph more than one way, and describe
// prints the stored graph in only one of them, so a script that states the
// graph mxcli itself built from it could fail to match its own flow: under
// mdl 0 that fell back to a silent rebuild, under mdl 1 to a refusal of an
// identical re-run. canonicalFlow rewrites both sides into one form before
// they are compared, for each pair of spellings that build the same graph:
//
//   - A guard clause and an if/else whose then-branch ends the path.
//     `if c then …return; end if; rest` and `if c then …return; else rest end if`
//     build the same split: the then-branch ends, so there is no merge, and the
//     false flow leads to rest. describe prints the else form when the false
//     flow goes straight to an end event (a guard directly before the final
//     return) and the guard form otherwise. The canonical form is the guard
//     form: the else statements follow the if in the enclosing list, where
//     each is still located by its @position and stays addressable.
//
//   - An `if` with an empty `else` and the same `if` with none. Both build a
//     split whose false flow goes on to what follows; describe drops the
//     empty else, so the canonical form has none (ako/mxcli#942) — in an
//     activity's error handler as well, where describe prints the empty
//     else (ako/mxcli#2259).
//
//   - A `join L` directly followed by its own `merge L`, where nothing else
//     joins L. Falling through into a merge and joining it are the same flow,
//     and a merge only one path reaches is no join point at all: describe
//     prints the pair for a nested guard whose false flow shares the outer
//     `if`'s merge (the merge is the `if`'s own, stated by its @merge), and no
//     author writes it. Both statements are dropped.
//
// Only where a path ends and how paths meet are rewritten; every statement
// that stands for a stored activity keeps its annotations, so the stored side
// still locates each by its @position. The rewrite copies what it changes: the
// declared statements are the ones the mdl 0 rebuild may still build from.
func canonicalFlow(stmts []ast.MicroflowStatement) []ast.MicroflowStatement {
	joins := map[string]int{}
	countJoins(stmts, joins)
	return canonicalList(stmts, joins)
}

func canonicalList(stmts []ast.MicroflowStatement, joins map[string]int) []ast.MicroflowStatement {
	if len(stmts) == 0 {
		return stmts
	}
	out := make([]ast.MicroflowStatement, 0, len(stmts))
	for i := 0; i < len(stmts); i++ {
		st := stmts[i]
		if j, ok := st.(*ast.JoinStmt); ok && joins[j.Label] == 1 && i+1 < len(stmts) {
			if m, ok := stmts[i+1].(*ast.MergeStmt); ok && m.Label == j.Label {
				i++
				continue
			}
		}
		st = canonicalNested(st, joins)
		if s, ok := st.(*ast.IfStmt); ok && len(s.ElseBody) > 0 && lastStmtIsReturn(s.ThenBody) {
			guard := *s
			guard.ElseBody, guard.HasElse = nil, false
			out = append(out, &guard)
			// The else statements are canonical already (canonicalNested);
			// they are spliced in at this level as they are.
			out = append(out, s.ElseBody...)
			continue
		}
		out = append(out, st)
	}
	return out
}

// canonicalNested returns st with the statement lists it holds made canonical,
// as a copy when any of them is.
func canonicalNested(st ast.MicroflowStatement, joins map[string]int) ast.MicroflowStatement {
	switch s := st.(type) {
	case *ast.IfStmt:
		c := *s
		c.ThenBody, c.ElseBody = canonicalList(s.ThenBody, joins), canonicalList(s.ElseBody, joins)
		if len(c.ElseBody) == 0 {
			c.HasElse = false
		}
		return &c
	case *ast.LoopStmt:
		c := *s
		c.Body = canonicalList(s.Body, joins)
		return &c
	case *ast.WhileStmt:
		c := *s
		c.Body = canonicalList(s.Body, joins)
		return &c
	case *ast.EnumSplitStmt:
		c := *s
		c.Cases = make([]ast.EnumSplitCase, len(s.Cases))
		for i, k := range s.Cases {
			k.Body = canonicalList(k.Body, joins)
			c.Cases[i] = k
		}
		c.ElseBody = canonicalList(s.ElseBody, joins)
		return &c
	case *ast.InheritanceSplitStmt:
		c := *s
		c.Cases = make([]ast.InheritanceSplitCase, len(s.Cases))
		for i, k := range s.Cases {
			k.Body = canonicalList(k.Body, joins)
			c.Cases[i] = k
		}
		c.ElseBody = canonicalList(s.ElseBody, joins)
		return &c
	}
	// An activity's custom error handler is a statement list like a branch:
	// describe prints an `if` in it with an empty else, the script states
	// none, and the two build the same graph (ako/mxcli#2259).
	if eh := errorHandling(st); eh != nil && len(eh.Body) > 0 {
		return withHandlerBody(st, canonicalList(eh.Body, joins))
	}
	return st
}

// countJoins counts the `join` statements per label, at any depth.
func countJoins(stmts []ast.MicroflowStatement, joins map[string]int) {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ast.JoinStmt:
			joins[s.Label]++
		case *ast.IfStmt:
			countJoins(s.ThenBody, joins)
			countJoins(s.ElseBody, joins)
		case *ast.LoopStmt:
			countJoins(s.Body, joins)
		case *ast.WhileStmt:
			countJoins(s.Body, joins)
		case *ast.EnumSplitStmt:
			for _, k := range s.Cases {
				countJoins(k.Body, joins)
			}
			countJoins(s.ElseBody, joins)
		case *ast.InheritanceSplitStmt:
			for _, k := range s.Cases {
				countJoins(k.Body, joins)
			}
			countJoins(s.ElseBody, joins)
		default:
			// A handler's `join` counts as any other: a label joined from a
			// handler as well is a real join point.
			if eh := errorHandling(st); eh != nil {
				countJoins(eh.Body, joins)
			}
		}
	}
}
