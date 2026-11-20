// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// voidCallOutputRule reports a read of the output name of a void action call.
const voidCallOutputRule = "MDL093"

// checkVoidCallOutputUse flags a variable that only a call to a VOID Java or
// JavaScript action names, and that the flow then reads — MDL093.
//
// Such a call keeps its output name in the model but declares nothing
// (ako/mxcli#953), so reading the name afterwards reads a variable that does not
// exist. Measured on mxbuild 11.13.0 (ako/mxcli#962), PedApp copy:
//
//	void Java call `$V1 = …`, then `log … + $V1`           CE0109 "Undefined variable 'V1'"
//	void JS call `$V3 = …`, then `$V3` as a call argument   CE0109 "Undefined variable 'V3'"
//	non-void Java call `$V2 = …`, then `log … + $V2`       0 errors (control)
//	void Java call `$V4 = …`, `declare $V4 …`, then `$V4`  0 errors
//
// So a name some other statement defines — a declare, a parameter, any
// non-void producer, anywhere in the flow — is not reported: variable names are
// flow-wide, and the definition is what the read resolves to. Only a call the
// resolver KNOWS is void counts; an unresolvable action is never reported
// here, whatever the duplicate-name policy does with it.
func (v *microflowValidator) checkVoidCallOutputUse(params []ast.MicroflowParam, body []ast.MicroflowStatement) {
	if v.skipCEGapRules() || v.voids == nil {
		return
	}
	voidNames := map[string]string{} // output name -> the action that "names" it
	defined := map[string]bool{}
	for _, p := range params {
		defined[p.Name] = true
	}
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		if v.voids.callIsKnownVoid(s) {
			if name := outputVariableField(s); name != "" {
				if _, seen := voidNames[name]; !seen {
					voidNames[name] = stmtActionName(s)
				}
			}
			return
		}
		if !v.buildsAsAProducer(s) {
			return
		}
		for _, p := range statementProducedVars(s) {
			defined[p.name] = true
		}
	})
	for name := range defined {
		delete(voidNames, name)
	}
	if len(voidNames) == 0 {
		return
	}

	reported := map[string]bool{}
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		for _, ref := range loopRefVars(s) {
			action, ok := voidNames[ref]
			if !ok || reported[ref] {
				continue
			}
			reported[ref] = true
			v.addViolation(voidCallOutputRule, linter.SeverityError,
				fmt.Sprintf("'$%s' is read, but the only statement naming it is a call to %s, which "+
					"returns nothing — a void action's output name declares no variable, so mxbuild "+
					"rejects this with CE0109 \"Undefined variable '%s'\"", ref, action, ref),
				fmt.Sprintf("Drop the read of '$%s', or call an action that returns a value — the "+
					"output name on a void call is inert", ref))
		}
	})
}

// stmtActionName names the action a Java/JavaScript call statement targets.
func stmtActionName(s ast.MicroflowStatement) string {
	switch st := s.(type) {
	case *ast.CallJavaActionStmt:
		return "java action " + st.ActionName.String()
	case *ast.CallJavaScriptActionStmt:
		return "javascript action " + st.ActionName.String()
	}
	return statementProducerLabel(s)
}

// walkFlowStatements calls fn for every statement of a flow body: nested
// branches, loop bodies and custom error-handler bodies included, since they
// all share the flow's one variable namespace.
func walkFlowStatements(body []ast.MicroflowStatement, fn func(ast.MicroflowStatement)) {
	for _, s := range body {
		fn(s)
		switch st := s.(type) {
		case *ast.LoopStmt:
			walkFlowStatements(st.Body, fn)
		case *ast.WhileStmt:
			walkFlowStatements(st.Body, fn)
		case *ast.IfStmt:
			walkFlowStatements(st.ThenBody, fn)
			walkFlowStatements(st.ElseBody, fn)
		case *ast.EnumSplitStmt:
			for _, c := range st.Cases {
				walkFlowStatements(c.Body, fn)
			}
			walkFlowStatements(st.ElseBody, fn)
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				walkFlowStatements(c.Body, fn)
			}
			walkFlowStatements(st.ElseBody, fn)
		}
		if eh := stmtErrorHandling(s); eh != nil {
			walkFlowStatements(eh.Body, fn)
		}
	}
}

// voidFlowCallResultRule reports a result variable on a call to a microflow or
// nanoflow known to return nothing.
const voidFlowCallResultRule = "MDL096"

// checkVoidFlowCallResult flags `$X = call microflow F(…)` (or nanoflow) when F
// is known to return nothing — MDL096, a warning.
//
// mxbuild accepts it (the name is inert, see voidCodeActions) and so does
// Studio Pro, which keeps such a name when a flow stops returning a value. But
// the statement reads as declaring `$X`, which it does not: exec stores no
// result variable for it, and describe prints the call without one
// (mendixlabs/mxcli#1820, ako/mxcli#2138). Reading `$X` afterwards is MDL093.
// Java and JavaScript action calls are left alone: Studio Pro names a void
// JavaScript action's output after the action, so that shape is ordinary.
func (v *microflowValidator) checkVoidFlowCallResult(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() || v.voids == nil {
		return
	}
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		var kind, target, out string
		switch st := s.(type) {
		case *ast.CallMicroflowStmt:
			kind, target, out = "microflow", st.MicroflowName.String(), st.OutputVariable
		case *ast.CallNanoflowStmt:
			kind, target, out = "nanoflow", st.NanoflowName.String(), st.OutputVariable
		default:
			return
		}
		if out == "" || !v.voids.callIsKnownVoid(s) {
			return
		}
		v.addViolation(voidFlowCallResultRule, linter.SeverityWarning,
			fmt.Sprintf("'$%s = call %s %s(…)': %s returns nothing, so the call declares no variable — "+
				"exec stores it without a result variable", out, kind, target, target),
			fmt.Sprintf("Write `call %s %s(…);` without `$%s =`, or give %s a return type", kind, target, out, target))
	})
}
