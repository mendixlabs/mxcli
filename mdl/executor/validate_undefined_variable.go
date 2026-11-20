// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// undefinedVariableRule is MDL098: an expression reads `$name` and nothing in the
// flow defines it.
const undefinedVariableRule = "MDL098"

// implicitVariables are the names MxBuild accepts with no declaration, lower-cased.
//
// Measured on 11.16.1, one `mx check -j` over 55 flows (11 names x plain flow,
// loop body, error handler, filter predicate, after a loop): $currentUser,
// $currentSession, $latestError, $latestHttpResponse and $latestSoapFault are
// accepted in every position, $currentIndex and the loop iterator inside the
// loop, and $currentObject inside a filter predicate. $latestJavaException and a
// made-up $IteratorItem are undefined like any other name.
//
// The rule treats $currentIndex and $currentObject as defined everywhere. MxBuild
// is stricter (CE0109 outside the loop or predicate), but a missed report costs
// less than a false one: exec refuses what check reports.
var implicitVariables = map[string]bool{
	"currentuser": true, "currentsession": true,
	"latesterror": true, "latesthttpresponse": true, "latestsoapfault": true,
	"currentindex": true, "currentobject": true,
}

// initDefinedVariables records every name the flow defines: the parameters and
// every variable a statement produces, at any depth, loop iterators included.
// Where a name is defined is not asked — a name defined only in a branch, or used
// before it is declared, is CE0108 / CE0109 for other reasons that other checks
// (MDL-SCOPE, MDL052) own. This one is only for a name defined NOWHERE.
//
// Names compare case-insensitively: whether Mendix distinguishes `$count` from
// `$Count` is unmeasured, and treating them as one can only silence a report.
func (v *microflowValidator) initDefinedVariables(body []ast.MicroflowStatement) {
	v.definedVars = map[string]bool{}
	for _, p := range v.params {
		v.definedVars[strings.ToLower(p.Name)] = true
	}
	for _, n := range FlowDefinedVariables(body) {
		v.definedVars[strings.ToLower(n)] = true
	}
	if v.returnType != nil && v.returnType.Variable != "" {
		// The `as $R` name is the end event's own variable (MDL097 owns it).
		v.definedVars[strings.ToLower(v.returnType.Variable)] = true
	}
	v.undefinedReported = map[string]bool{}
}

// checkUndefinedVariables reports each `$name` an expression reads that the flow
// does not define, once per name per flow. label says where the expression is.
func (v *microflowValidator) checkUndefinedVariables(label, src string) {
	if v.definedVars == nil || v.skipCEGapRules() {
		return
	}
	for _, r := range exprcheck.VariableReferences(src) {
		key := strings.ToLower(r.Name)
		if v.definedVars[key] || implicitVariables[key] || v.undefinedReported[key] {
			continue
		}
		v.undefinedReported[key] = true
		v.addViolation(undefinedVariableRule, linter.SeverityError,
			fmt.Sprintf("%s reads $%s, which nothing in this %s defines — the build fails CE0109 \"Undefined variable '%s'\"",
				label, r.Name, v.docType, r.Name),
			fmt.Sprintf("Declare $%s (`declare $%s <type> = …;`), take it as a parameter, or check the spelling.", r.Name, r.Name))
	}
}
