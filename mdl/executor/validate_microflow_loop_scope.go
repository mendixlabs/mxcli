// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkLoopScoping flags a reference, from outside a loop, to a variable that
// only exists inside it — the loop iterator itself, or anything the loop body
// introduces (a retrieve, a create, a call output, …).
//
// Mendix scopes both to the loop body, so the reference builds as
//
//	[CE0108] "Variable 'X' is defined but not in scope at this location."
//
// even though the flow is otherwise well-formed. MDL052 is the sibling rule for
// the other half of Mendix's loop-variable semantics: names must be unique
// across the WHOLE microflow (CE0111), while visibility stops at the loop body.
//
// Only names owned by a loop are considered, and any name that is also
// introduced outside a loop is dropped from the set first — so a re-declared
// name can never be reported.
func (v *microflowValidator) checkLoopScoping(body []ast.MicroflowStatement) {
	owner := map[string]*ast.LoopStmt{}
	ambiguous := map[string]bool{}
	collectLoopScopedVars(body, owner, ambiguous)
	if len(owner) == 0 {
		return
	}

	// A name claimed by two loops has no single owning body to judge references
	// against — and it is already MDL052/CE0111 ("Duplicate variable name").
	for name := range ambiguous {
		delete(owner, name)
	}

	// A name introduced outside any loop is in scope after the loop regardless
	// of what the loop does with it; never report those.
	for name := range collectNonLoopDeclaredVars(body) {
		delete(owner, name)
	}
	if len(owner) == 0 {
		return
	}

	v.walkLoopScope(body, map[*ast.LoopStmt]bool{}, owner, map[string]bool{})
}

// walkLoopScope walks the body with the set of loops whose bodies enclose the
// current position, reporting each out-of-scope name once.
func (v *microflowValidator) walkLoopScope(
	body []ast.MicroflowStatement,
	active map[*ast.LoopStmt]bool,
	owner map[string]*ast.LoopStmt,
	reported map[string]bool,
) {
	for _, s := range body {
		for _, name := range loopRefVars(s) {
			if name == "" || reported[name] {
				continue
			}
			loop, ok := owner[name]
			if !ok || active[loop] {
				continue
			}
			reported[name] = true
			v.addViolation("MDL053", linter.SeverityError,
				fmt.Sprintf("variable '$%s' is used outside the loop over '$%s' that defines it; "+
					"a Mendix loop variable — the iterator and anything the loop body creates — "+
					"is scoped to the loop body, so this builds as CE0108 "+
					"\"Variable '%s' is defined but not in scope at this location\"",
					name, loop.ListVariable, name),
				fmt.Sprintf("Move the statement into the loop body, or carry the value out "+
					"in a variable declared before the loop (e.g. 'declare $Result …' then "+
					"'set $Result = $%s' inside the loop)", name))
		}

		// Recurse into nested bodies, extending the active set for loops.
		switch st := s.(type) {
		case *ast.LoopStmt:
			inner := make(map[*ast.LoopStmt]bool, len(active)+1)
			for l := range active {
				inner[l] = true
			}
			inner[st] = true
			v.walkLoopScope(st.Body, inner, owner, reported)
			continue
		case *ast.WhileStmt:
			v.walkLoopScope(st.Body, active, owner, reported)
		case *ast.IfStmt:
			v.walkLoopScope(st.ThenBody, active, owner, reported)
			v.walkLoopScope(st.ElseBody, active, owner, reported)
		case *ast.EnumSplitStmt:
			for _, c := range st.Cases {
				v.walkLoopScope(c.Body, active, owner, reported)
			}
			v.walkLoopScope(st.ElseBody, active, owner, reported)
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				v.walkLoopScope(c.Body, active, owner, reported)
			}
			v.walkLoopScope(st.ElseBody, active, owner, reported)
		}

		if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
			v.walkLoopScope(eh.Body, active, owner, reported)
		}
	}
}

// collectLoopScopedVars maps every loop-scoped variable name to the loop whose
// body introduces it. Each loop claims only the names in its OWN body — a
// nested loop's names belong to the nested loop — so a name that still ends up
// claimed twice is a genuine duplicate (two sibling loops reusing an iterator);
// those go into ambiguous and are not reported here.
func collectLoopScopedVars(body []ast.MicroflowStatement, owner map[string]*ast.LoopStmt, ambiguous map[string]bool) {
	claim := func(name string, loop *ast.LoopStmt) {
		if name == "" {
			return
		}
		if prev, seen := owner[name]; seen && prev != loop {
			ambiguous[name] = true
		}
		owner[name] = loop
	}

	for _, s := range body {
		switch st := s.(type) {
		case *ast.LoopStmt:
			claim(st.LoopVariable, st)
			for name := range declaredVarsOwnScope(st.Body) {
				claim(name, st)
			}
			collectLoopScopedVars(st.Body, owner, ambiguous)
		case *ast.WhileStmt:
			collectLoopScopedVars(st.Body, owner, ambiguous)
		case *ast.IfStmt:
			collectLoopScopedVars(st.ThenBody, owner, ambiguous)
			collectLoopScopedVars(st.ElseBody, owner, ambiguous)
		case *ast.EnumSplitStmt:
			for _, c := range st.Cases {
				collectLoopScopedVars(c.Body, owner, ambiguous)
			}
			collectLoopScopedVars(st.ElseBody, owner, ambiguous)
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				collectLoopScopedVars(c.Body, owner, ambiguous)
			}
			collectLoopScopedVars(st.ElseBody, owner, ambiguous)
		}
		if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
			collectLoopScopedVars(eh.Body, owner, ambiguous)
		}
	}
}

// collectNonLoopDeclaredVars returns the names introduced anywhere OUTSIDE a
// loop body (branches and error handlers included — those are a different
// scoping question, covered by MDL005).
func collectNonLoopDeclaredVars(body []ast.MicroflowStatement) map[string]bool {
	vars := map[string]bool{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			for name := range collectDeclaredVars([]ast.MicroflowStatement{s}) {
				vars[name] = true
			}
			switch st := s.(type) {
			case *ast.LoopStmt:
				// Deliberately not descended into: those names are loop-scoped.
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
			if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
				walk(eh.Body)
			}
		}
	}
	walk(body)
	return vars
}

// declaredVarsOwnScope returns the variable names a loop body introduces
// itself — descending into branches, whiles, and error handlers, but NOT into a
// nested loop, whose names belong to that loop.
func declaredVarsOwnScope(body []ast.MicroflowStatement) map[string]bool {
	vars := map[string]bool{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			for name := range collectDeclaredVars([]ast.MicroflowStatement{s}) {
				vars[name] = true
			}
			switch st := s.(type) {
			case *ast.LoopStmt:
				// Owned by the nested loop, not by this body.
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
			if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
				walk(eh.Body)
			}
		}
	}
	walk(body)
	return vars
}

// loopRefVars returns the variable names a single statement reads, WITHOUT
// descending into nested bodies (walkLoopScope visits those itself, so that a
// reference is judged against the loops actually enclosing it).
//
// A statement kind missing here only costs a missed report, never a false one.
func loopRefVars(stmt ast.MicroflowStatement) []string {
	var refs []string
	add := func(names ...string) {
		for _, n := range names {
			if n != "" {
				refs = append(refs, extractVarName(n))
			}
		}
	}
	addExpr := func(exprs ...ast.Expression) {
		for _, e := range exprs {
			refs = append(refs, exprVarRefs(e)...)
		}
	}
	addArgs := func(args []ast.CallArgument) {
		for _, a := range args {
			addExpr(a.Value)
		}
	}
	addChanges := func(items []ast.ChangeItem) {
		for _, c := range items {
			addExpr(c.Value)
		}
	}

	switch s := stmt.(type) {
	case *ast.MfSetStmt:
		add(s.Target)
		addExpr(s.Value)
	case *ast.DeclareStmt:
		addExpr(s.InitialValue)
	case *ast.ReturnStmt:
		addExpr(s.Value)
	case *ast.CreateObjectStmt:
		addChanges(s.Changes)
	case *ast.ChangeObjectStmt:
		add(s.Variable)
		addChanges(s.Changes)
	case *ast.MfCommitStmt:
		add(s.Variable)
	case *ast.DeleteObjectStmt:
		add(s.Variable)
	case *ast.SendExternalObjectStmt:
		add(s.Variable)
	case *ast.DeleteExternalObjectStmt:
		add(s.Variable)
	case *ast.RollbackStmt:
		add(s.Variable)
	case *ast.RetrieveStmt:
		add(s.StartVariable)
		addExpr(s.Where)
	case *ast.IfStmt:
		addExpr(s.Condition)
	case *ast.WhileStmt:
		addExpr(s.Condition)
	case *ast.LoopStmt:
		// The iterator is defined here, not referenced; the list is not.
		add(s.ListVariable)
	case *ast.EnumSplitStmt:
		add(s.Variable)
		addExpr(s.Condition)
	case *ast.InheritanceSplitStmt:
		add(s.Variable)
	case *ast.CastObjectStmt:
		add(s.ObjectVariable)
	case *ast.LogStmt:
		addExpr(s.Node, s.Message)
	case *ast.CallMicroflowStmt:
		addArgs(s.Arguments)
	case *ast.CallNanoflowStmt:
		addArgs(s.Arguments)
	case *ast.CallJavaActionStmt:
		addArgs(s.Arguments)
	case *ast.CallJavaScriptActionStmt:
		addArgs(s.Arguments)
	case *ast.ExecuteDatabaseQueryStmt:
		addArgs(s.Arguments)
		addArgs(s.ConnectionArguments)
	case *ast.ListOperationStmt:
		add(s.InputVariable, s.SecondVariable)
		addExpr(s.Condition, s.OffsetExpr, s.LimitExpr)
	case *ast.AggregateListStmt:
		add(s.InputVariable)
		addExpr(s.Expression)
	case *ast.AddToListStmt:
		add(s.List)
		if s.Value != nil {
			addExpr(s.Value)
		} else {
			add(s.Item)
		}
	case *ast.RemoveFromListStmt:
		add(s.Item, s.List)
	case *ast.ClearListStmt:
		add(s.List)
	case *ast.ShowPageStmt:
		add(s.ForObject)
		for _, a := range s.Arguments {
			addExpr(a.Value)
		}
	case *ast.ShowMessageStmt:
		addExpr(s.Message)
		addExpr(s.TemplateArgs...)
	case *ast.DownloadFileStmt:
		add(s.FileDocument)
	case *ast.SynchronizeStmt:
		add(s.Variables...)
	case *ast.SendEmailStmt:
		addExpr(s.Expressions()...)
		if s.Attachment != "" {
			add(s.Attachment)
		}
	case *ast.ValidationFeedbackStmt:
		if s.AttributePath != nil {
			add(s.AttributePath.Variable)
		}
		addExpr(s.Message)
		addExpr(s.TemplateArgs...)
	}

	// Normalise: strip any leftover $ sigils from expression-derived names.
	for i, r := range refs {
		refs[i] = strings.TrimPrefix(r, "$")
	}
	return refs
}
