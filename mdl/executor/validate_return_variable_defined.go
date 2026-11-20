// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// returnVariableDefinedRule is MDL097: `returns T as $R` whose body can run off
// the end while $R holds no value.
const returnVariableDefinedRule = "MDL097"

// checkReturnVariableDefined flags a microflow or nanoflow declared
// `returns T as $R` that has a path off its end and never gives $R a value.
//
// MDL003 exempts the AS clause, because the final End event returns `$R`
// whether or not the body spells a RETURN out. That holds only while `$R` exists
// where the flow ends. Measured on MxBuild 11.16.1, one `mx check` over 15 flows,
// microflow and nanoflow alike:
//
//	$R never defined, a path falls off the end   CE0109 "Undefined variable 'R'" at the End event
//	$R defined only inside a branch or a loop    CE0108 "defined but not in scope" at the End event
//	$R declared at the top level                 0 errors, whether the body falls off the end or not
//	$R never defined, every path returns         0 errors: an explicit RETURN wins
//
// "Defined at the top level" is any statement of the flow's own body, in any
// position: declaring $R after an `if` that returns is clean too. A parameter of
// the same name defines it. A name only a nested body defines — an `if` branch, a
// loop body, an error handler — is out of scope at the End event, which is why
// those two cases report differently.
//
// The rule stays quiet unless the flow really can fall off the end, by both
// predicates the repo already has: bodyReturns (the MDL003 one) and
// lastStmtIsReturn (the builder's, which also counts a named join and a `raise
// error`). It never fires on a flow with no AS clause, which is MDL003's.
func (v *microflowValidator) checkReturnVariableDefined(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() || v.returnType == nil || v.returnType.Type.Kind == ast.TypeVoid || v.returnType.Variable == "" {
		return
	}
	name := v.returnType.Variable
	if bodyReturns(body) || lastStmtIsReturn(body) {
		return
	}
	for _, p := range v.params {
		if p.Name == name {
			return
		}
	}
	for _, s := range body {
		for _, p := range statementProducedVars(s) {
			if p.name == name && !p.loop {
				return
			}
		}
	}

	var problem string
	if containsName(FlowDefinedVariables(body), name) {
		problem = fmt.Sprintf("$%s is defined only inside a branch or loop, so it is out of scope where the %s ends "+
			"and not every path returns — the build fails CE0108", name, v.docType)
	} else {
		problem = fmt.Sprintf("returns %s as $%s, but $%s is never defined and not every path returns — "+
			"the build fails CE0109 \"Undefined variable '%s'\" at the end event", returnTypeString(v.returnType), name, name, name)
	}
	v.addViolation(returnVariableDefinedRule, linter.SeverityError, problem,
		fmt.Sprintf("Declare $%s at the top level of the %s (`declare $%s %s = …;`), or end every path with a return.",
			name, v.docType, name, returnTypeString(v.returnType)))
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
