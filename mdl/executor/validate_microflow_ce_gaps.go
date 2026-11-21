// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Three constructs from upstream #893 that parsed, passed `mxcli check`, were
// written by `exec`, and were then rejected by the build. Each was reproduced on
// mxbuild 11.6.6 against a blank app with a 1-error baseline, and each fix
// suggested below was measured to take that app back to the baseline:
//
//	MDL061  declare with no value            CE0038  fix: `= ''`
//	MDL062  return inside a loop             CE0068  fix: `break`
//	MDL063  duplicate variable name          CE0111  fix: drop the declare
//
// Error severity is enough to close the gap the issue was filed about: exec runs
// a pre-flight check over the whole script and refuses with nothing written, so
// "green check, green exec, red Errors pane" no longer happens. Verified end to
// end on the issue's own reproduction — 3 errors reported, 0 microflows written.
//
// They are deliberately NOT added to execEnforcedMicroflowRules, which would
// make `--no-check` refuse them too. That list's bar is a claim verified against
// a real mxbuild, which these meet; the reason to stay off it is different.
// Unlike the XPath rules there, these three predict what the BUILDER emits, and
// building this change turned up four shapes where the AST says "broken" and the
// build says otherwise — two of them in mxcli's own shipped examples. Each is
// exempted and tested below, but a rule with that failure mode should leave the
// author an escape hatch rather than become an unconditional write barrier.

// skipCEGapRules reports whether the three rules stand down for this microflow.
//
// An @excluded document is not part of the build, and mxbuild does not check it:
// measured on 11.6.6, the very microflow that is CE0111 when included produces
// no error at all when excluded. #312 made `check --references` skip excluded
// microflows for that reason — agentic workflows deliberately stash broken
// intermediate state in excluded scaffolding — and error-severity rules block
// `exec`, so flagging them here would undo that fix by another route.
func (v *microflowValidator) skipCEGapRules() bool { return v.excluded }

// checkDeclareHasValue flags a `declare` with no initial value — MDL061.
//
// A declare maps to a Create Variable activity, whose Value property Mendix
// requires: CE0038 "The 'Value' property is required." There is no
// uninitialized variable in a microflow, so this is not a defaultable omission;
// mxcli cannot pick the value without inventing semantics (0? empty? for a
// DateTime, what?), and the author writing the value is a one-token fix.
//
// A list or object declare is left to MDL040/MDL043, which reject the TYPE. On
// those, "supply a value" points at the wrong fix — the activity cannot hold
// the type at all, so adding `= empty` does not help.
func (v *microflowValidator) checkDeclareHasValue(stmt *ast.DeclareStmt) {
	if v.skipCEGapRules() || stmt.InitialValue != nil {
		return
	}
	if stmt.Type.Kind == ast.TypeListOf {
		return // MDL040
	}
	if stmt.Type.Kind == ast.TypeEntity ||
		(stmt.Type.Kind == ast.TypeEnumeration && stmt.Type.EnumRef != nil && !stmt.Type.ExplicitEnum) {
		return // MDL043
	}

	v.addViolation("MDL061", linter.SeverityError,
		fmt.Sprintf("declare '$%s' has no initial value, but the Create Variable activity it builds "+
			"requires one — mxbuild rejects it with CE0038 \"The 'Value' property is required.\". "+
			"Mendix has no uninitialized variable: every variable is created with a value.",
			stmt.Variable),
		fmt.Sprintf("Give it a starting value, e.g. declare $%s %s = %s",
			stmt.Variable, declaredTypeLabel(stmt.Type), zeroValueHint(stmt.Type)))
}

// declaredTypeLabel renders a declare's type the way the statement spells it,
// for use in the suggested replacement line.
func declaredTypeLabel(t ast.DataType) string {
	if t.Kind == ast.TypeEnumeration && t.EnumRef != nil {
		return "Enumeration(" + t.EnumRef.String() + ")"
	}
	return t.Kind.String()
}

// zeroValueHint is the value the suggestion offers as a starting point. It is
// advice printed in a message, never something mxcli writes on the author's
// behalf — picking the value is the author's call, which is the whole reason
// MDL061 is a refusal rather than a silent default.
func zeroValueHint(t ast.DataType) string {
	switch t.Kind {
	case ast.TypeString:
		return "''"
	case ast.TypeInteger, ast.TypeLong:
		return "0"
	case ast.TypeDecimal:
		return "0.0"
	case ast.TypeBoolean:
		return "false"
	default:
		return "empty"
	}
}

// checkReturnInLoop flags a `return` anywhere inside a loop — MDL062.
//
// A return builds an End event, and Mendix forbids one inside a LoopedActivity:
// CE0068 "End events cannot be placed inside a loop." Nesting does not help — a
// return inside a branch inside the loop is still inside the loop — so the walk
// tracks loop depth rather than looking only at the loop's immediate body.
//
// `break` is the construct that replaces it: it leaves the loop, and execution
// continues after it. Returning a value from inside a loop needs the value
// stashed in a variable and a single return after the loop.
//
// The rule predicts what the BUILDER emits, not what the MDL looks like, so
// `while true` is exempt: it is built as an ExclusiveMerge back-edge, not a
// LoopedActivity (#350). With no loop object there is no "inside a loop", and
// the return is an ordinary End event. A plain `while <cond>` IS a
// LoopedActivity and is not exempt — measured separately.
//
// `returns T as $Var` used to be exempt too, on the stated grounds that
// buildFlowGraph synthesizes the End event from the variable so none lands in
// the loop, and that the shape builds CE0109 ("Undefined variable") rather than
// CE0068. The first half was never true and the second was an artefact of the
// measurement: mxbuild reports ONE error per microflow, and in the shape that
// was measured — `returns Boolean as $Done` with the loop's `return true` and
// nothing assigning $Done — CE0109 simply won the race and hid CE0068
// underneath. Re-measured on 11.13.0, with `declare $Done Boolean = false`
// added and NOTHING else changed:
//
//	as-clause, $Done unassigned  ->  CE0109 "Undefined variable 'Done'."
//	as-clause, $Done assigned    ->  CE0068 "End events cannot be placed inside a loop."
//
// `describe microflow` shows the in-loop `return true` written in both, so the
// End event was always there. The exemption is gone; a masked error is not an
// absent one, and CapTrackV2 FINDINGS §19 is the report it cost.
func (v *microflowValidator) checkReturnInLoop(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() {
		return
	}

	var walk func(stmts []ast.MicroflowStatement, depth int)
	walk = func(stmts []ast.MicroflowStatement, depth int) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.ReturnStmt:
				if depth > 0 {
					v.addViolation("MDL062", linter.SeverityError,
						"return inside a loop builds an End event inside the loop, which Mendix does "+
							"not allow — mxbuild rejects it with CE0068 \"End events cannot be placed "+
							"inside a loop.\"",
						"Use break to leave the loop; to return a value, assign it to a variable "+
							"inside the loop and put a single return after the loop")
				}
			case *ast.LoopStmt:
				walk(st.Body, depth+1)
			case *ast.WhileStmt:
				inner := depth + 1
				if isUnconditionalTrueWhile(st) {
					inner = depth // an ExclusiveMerge back-edge, not a loop object
				}
				walk(st.Body, inner)
			case *ast.IfStmt:
				walk(st.ThenBody, depth)
				walk(st.ElseBody, depth)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, depth)
				}
				walk(st.ElseBody, depth)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, depth)
				}
				walk(st.ElseBody, depth)
			}
		}
	}
	walk(body, 0)
}

// buildsAsAProducer reports whether a statement that LOOKS like a producer in
// the AST actually builds as one.
//
// `set $Match = contains($Hay, $Needle)` on two Strings parses as a
// ListOperationStmt — the visitor cannot tell a string function from a list
// operation at parse time — but addListOperationAction rewrites it into a
// Change Variable when the input is a declared String, precisely because a list
// operation would create its output variable and collide with the declare
// (ledger findings #53/#63, whose examples exist to pin that rewrite).
//
// MDL063 must not report the CE0111 that rewrite exists to prevent: both
// examples build at 0 errors above baseline on mxbuild 11.6.6, and an earlier
// draft of this rule flagged them. The operation test is shared with the
// builder (stringOverloadedListOp) so the two cannot drift apart silently.
func (v *microflowValidator) buildsAsAProducer(s ast.MicroflowStatement) bool {
	lo, ok := s.(*ast.ListOperationStmt)
	if !ok {
		return true
	}
	return !(stringOverloadedListOp(lo.Operation) && v.varKinds[lo.InputVariable] == exprcheck.KindString)
}

// fragmentCannotJudge reports, for an alter fragment, a statement whose
// output MDL063's exemptions decide from types the fragment does not have: a
// code-action or flow call (void or not is the called action's, unread here)
// and a String contains/find (a Change Variable when its input is a String
// declared outside the fragment). Left out rather than guessed, so a fragment
// is never refused for a duplicate it does not have (mendixlabs/mxcli#1914).
func (v *microflowValidator) fragmentCannotJudge(s ast.MicroflowStatement) bool {
	if !v.fragment {
		return false
	}
	switch st := s.(type) {
	case *ast.CallJavaActionStmt, *ast.CallJavaScriptActionStmt, *ast.CallMicroflowStmt, *ast.CallNanoflowStmt:
		return v.voids == nil
	case *ast.ListOperationStmt:
		return stringOverloadedListOp(st.Operation)
	}
	return false
}

// stringOverloadedListOp reports whether a list operation shares its name with a
// String function, so the same MDL spells both. Read by the builder, which
// rewrites these into a Change Variable, and by MDL063, which must stay silent
// on exactly the statements that rewrite covers.
func stringOverloadedListOp(op ast.ListOperationType) bool {
	return op == ast.ListOpContains || op == ast.ListOpFind
}

// producedVar is one definition of a variable name within a microflow.
type producedVar struct {
	name  string
	label string // how the definition is described to the author
	loop  bool   // the definition is a loop iterator (MDL052's territory)
}

// checkDuplicateVariableNames flags a name defined more than once — MDL063.
//
// A microflow's variable namespace is FLAT. Measured on mxbuild 11.6.6, each of
// these is CE0111 "Duplicate variable name", isolated one microflow at a time:
// two declares in one body; a declare outside a loop and another inside it; a
// declare in each of two SIBLING if/else branches; two retrieves; a parameter
// and a declare; a loop iterator and a declare. Neither branches nor loop bodies
// open a scope, so the walk deliberately does not track one.
//
// The distinction that matters is create versus assign: `$X = 'b'` after
// `declare $X` is a Change Variable, which defines nothing and measured clean.
// A rule that keyed on "the name appears on the left of `=`" would flag the
// normal way to update a variable.
//
// Iterator-versus-iterator is left to MDL052, whose message explains loop
// scoping; two rules on one line is noise.
func (v *microflowValidator) checkDuplicateVariableNames(params []ast.MicroflowParam, body []ast.MicroflowStatement) {
	if v.skipCEGapRules() {
		return
	}
	first := map[string]producedVar{}

	report := func(p producedVar) {
		prev, seen := first[p.name]
		if !seen {
			first[p.name] = p
			return
		}
		if prev.loop && p.loop {
			return // MDL052
		}
		suggestion := fmt.Sprintf("Rename one of them, or — if you meant to reuse the first — drop the "+
			"redundant definition: %s already creates '$%s', and an activity always creates "+
			"its own output variable rather than writing into an existing one",
			prev.label, p.name)
		if p.label == "create" {
			// The conditional-create idiom: `declare $X M.E = empty` then a create
			// per branch, or the same create name in both branches. Both are CE0111
			// (measured 11.15.0, mendixlabs/mxcli#1516); these two forms build.
			suggestion += fmt.Sprintf(". For an object created conditionally, create it once with "+
				"conditional values (`$%s = create M.E (Attr = if $c then 1 else 2);`), or let a "+
				"sub-microflow create and return it on each branch — see `mxcli syntax microflow.variables`",
				p.name)
		}
		v.addViolation("MDL063", linter.SeverityError,
			fmt.Sprintf("'$%s' is created twice in this %s — first by %s, then by %s. "+
				"A %s's variable names are unique flow-wide (branches and loop bodies do "+
				"not open a scope), so mxbuild rejects this with CE0111 \"Duplicate variable name\".",
				p.name, v.docType, prev.label, p.label, v.docType),
			suggestion)
	}

	for _, p := range params {
		if p.Name != "" {
			report(producedVar{name: p.Name, label: "the " + v.docType + " parameter"})
		}
	}

	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			// A call to a void Java/JavaScript action keeps an output name in
			// the model and declares nothing: two of them build clean, and so
			// does a later declare of the same name (#953, mxbuild 11.13.0).
			if !v.fragmentCannotJudge(s) && v.buildsAsAProducer(s) && !v.voids.callIsVoid(s) {
				for _, p := range statementProducedVars(s) {
					report(p)
				}
			}
			switch st := s.(type) {
			case *ast.LoopStmt:
				walk(st.Body)
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
			// A custom error handler's activities share the flow's namespace.
			if eh := stmtErrorHandling(s); eh != nil {
				walk(eh.Body)
			}
		}
	}
	walk(body)
}

// FlowDefinedVariables returns every variable name the statements define,
// nested bodies included, each once and in the order first defined: the
// definitions MDL063 and MDL052 count. A flow's variable namespace is flat, so
// two of these with one name are CE0111.
//
// It leaves out MDL063's two exemptions, which need resolved types (a void
// call's output name, and a String contains/find that builds as a Change
// Variable): its caller renames a body's variables per copy — the `mxcli test`
// after-startup runner, which joins every test into one flow (ako/mxcli#1775) —
// and renaming one name too many costs nothing, where missing one is CE0111.
func FlowDefinedVariables(body []ast.MicroflowStatement) []string {
	var names []string
	seen := map[string]bool{}
	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			for _, p := range statementProducedVars(s) {
				if !seen[p.name] {
					seen[p.name] = true
					names = append(names, p.name)
				}
			}
			for _, nested := range ast.StatementBodies(s) {
				walk(nested)
			}
		}
	}
	walk(body)
	return names
}

// statementProducedVars returns the variables a statement CREATES.
//
// The `OutputVariable` field is read by reflection rather than from a switch:
// fifteen statement types carry it, the name is unambiguous — a field called
// OutputVariable is always a producer — and a statement type added later is
// covered without anyone remembering to extend a list. The alternative, a
// hand-maintained type switch, is exactly the shape of the blind spot that let
// #892's DROP FOLDER guard miss five document kinds.
//
// Reflection is NOT used for the `Variable` field, which is ambiguous: it names
// the produced variable on declare/create/retrieve/create list, and a CONSUMED
// one on change/commit/delete/rollback and on the split statements. Those five
// producers are therefore listed explicitly.
func statementProducedVars(s ast.MicroflowStatement) []producedVar {
	var out []producedVar

	switch st := s.(type) {
	case *ast.DeclareStmt:
		out = append(out, producedVar{name: st.Variable, label: "declare"})
	case *ast.CreateObjectStmt:
		out = append(out, producedVar{name: st.Variable, label: "create"})
	case *ast.CreateListStmt:
		out = append(out, producedVar{name: st.Variable, label: "create list"})
	case *ast.RetrieveStmt:
		out = append(out, producedVar{name: st.Variable, label: "retrieve"})
	case *ast.LoopStmt:
		out = append(out, producedVar{name: st.LoopVariable, label: "the loop iterator", loop: true})
	}

	if name := outputVariableField(s); name != "" {
		out = append(out, producedVar{name: name, label: statementProducerLabel(s)})
	}

	// A producer with an empty name is an optional output the author left off.
	kept := out[:0]
	for _, p := range out {
		if p.name != "" {
			kept = append(kept, p)
		}
	}
	return kept
}

// outputVariableField reads a statement's OutputVariable field, if it has one.
func outputVariableField(s ast.MicroflowStatement) string {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName("OutputVariable")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// statementProducerLabel describes an activity in the author's vocabulary.
// stmtActivityName covers the common ones; anything it does not know falls back
// to the AST type name rather than the generic "Activity", so a statement type
// added later still names itself in the message.
func statementProducerLabel(s ast.MicroflowStatement) string {
	if name := stmtActivityName(s); name != "Activity" {
		return name
	}
	t := reflect.TypeOf(s)
	if t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return "an activity"
	}
	return "the " + strings.TrimSuffix(t.Name(), "Stmt") + " activity"
}
