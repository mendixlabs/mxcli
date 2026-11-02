// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateNanoflow runs the MDL0xx rules that hold for a nanoflow body: MDL044,
// an expression calling a name that is not a Mendix function, and MDL063, a
// variable name created twice.
//
// MDL044 was wired to CREATE MICROFLOW alone (#828), so `currentDeviceType()`
// in a nanoflow passed check and exec and failed the build with CE0117
// (mendixlabs/mxcli#1033). The microflow rule set is deliberately NOT run
// wholesale: several rules are microflow-specific and would be false positives
// here — MDL057 refuses `synchronize`, which only a nanoflow may contain.
func ValidateNanoflow(stmt *ast.CreateNanoflowStmt) []linter.Violation {
	return validateNanoflowWith(stmt, nil)
}

// validateNanoflowWith is ValidateNanoflow with a resolver for the return types
// of the Java/JavaScript actions the body calls (see validateMicroflowWith).
func validateNanoflowWith(stmt *ast.CreateNanoflowStmt, voids *voidCodeActions) []linter.Violation {
	v := &microflowValidator{
		mfName:     stmt.Name.String(),
		docType:    "nanoflow",
		returnType: stmt.ReturnType,
		varKinds:   map[string]exprcheck.TypeKind{},
		params:     stmt.Parameters,
		excluded:   stmt.Excluded,
		voids:      voids,
	}
	v.walkExprFunctions(stmt.Body)
	v.walkAnnotations(stmt.Body)
	// MDL063 holds for a nanoflow too: its variable names are as flat as a
	// microflow's. Measured on mxbuild 11.13.0, each is CE0111 in a nanoflow —
	// a non-void call in each of two if/else branches, a parameter and a
	// declare, a declare inside a loop body and another after it. The
	// exec-side validator treated branches as scopes and passed the first
	// (ako/mxcli#953). The kinds MDL063 reads to spare the String
	// contains/find rewrite come from the parameters and the declares.
	v.seedPrimitiveKinds(stmt.Parameters, stmt.Body)
	v.checkDuplicateVariableNames(v.params, stmt.Body)
	// A nanoflow's show message has the same ShowMessageType (mendixlabs/mxcli#1280).
	v.checkShowMessageType(stmt.Body)
	// MDL-TEMPLATE01: a nanoflow's log, show message and validation feedback
	// store the same templates (ako/mxcli#1683).
	v.checkMessageTemplates(stmt.Body)
	// MDL-TEMPLATE02: and the same template grammar (ako/mxcli#1397).
	v.checkTemplateGrammar(stmt.Body)
	v.checkVoidCallOutputUse(v.params, stmt.Body)
	// MDL-SET01: a nanoflow's Change variable is as primitive-only (CE7247).
	v.checkSetOnObjectVariable(v.params, stmt.Body)
	// MDL-CHANGE01: System.owner / System.changedBy are CE1050 in a nanoflow's
	// create and change too, measured on 11.12.5 (mendixlabs/mxcli#1612).
	v.checkSystemMemberChange(stmt.Body)
	// MDL-FLOW05: a nanoflow's flow is built by the same builder, so a statement
	// after a return is dead there too (mendixlabs/mxcli#1491).
	v.checkUnreachableStatements(stmt.Body)
	// MDL049 / MDL094: a bare association path returned, or a loop over a path.
	// Measured on 11.12.1, a nanoflow's return is CE0117 as a microflow's is,
	// and its loop is built from the same AST (mendixlabs/mxcli#199).
	v.checkAssociationPathValues(stmt.Body)
	// The nanoflow restrictions exec's build refuses (validateNanoflow): an
	// action a nanoflow cannot hold, an error-handling clause its activity
	// rejects (CE6035), a Binary return. They ran only inside exec, so `check`
	// passed what exec then refused (mendixlabs/mxcli#591).
	for _, msg := range validateNanoflowBody(stmt.Body) {
		v.addViolation(nanoflowActivityRule, linter.SeverityError, msg, "")
	}
	if msg := validateNanoflowReturnType(stmt.ReturnType); msg != "" {
		v.addViolation(nanoflowActivityRule, linter.SeverityError, msg, "")
	}
	return v.violations
}

// nanoflowActivityRule reports what a nanoflow cannot hold — the messages of
// validateNanoflow, which exec refuses in its build. A check-side ID for them.
const nanoflowActivityRule = "MDL091"

// walkAnnotations applies checkUnknownAnnotations (MDL059, MDL060,
// MDL079) to every statement of a body, nested ones and error-handler bodies
// included. A nanoflow runs it over its whole body; a microflow's walkBody
// covers its own statements and hands it the handler bodies, which walkBody
// does not enter. A nanoflow is where layout annotations matter most — Studio Pro's
// PED API does not write nanoflows, so MDL is their only scripted writer — and
// an annotation that parsed but was dropped there passed check AND exec
// (mendixlabs/mxcli#992).
func (v *microflowValidator) walkAnnotations(body []ast.MicroflowStatement) {
	for _, s := range body {
		v.checkUnknownAnnotations(s)
		switch stmt := s.(type) {
		case *ast.IfStmt:
			v.walkAnnotations(stmt.ThenBody)
			v.walkAnnotations(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			for _, c := range stmt.Cases {
				v.walkAnnotations(c.Body)
			}
			v.walkAnnotations(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				v.walkAnnotations(c.Body)
			}
			v.walkAnnotations(stmt.ElseBody)
		case *ast.LoopStmt:
			v.walkAnnotations(stmt.Body)
		case *ast.WhileStmt:
			v.walkAnnotations(stmt.Body)
		}
		if eh := stmtErrorHandling(s); eh != nil {
			v.walkAnnotations(eh.Body)
		}
	}
}

// walkExprFunctions applies checkStmtExprFunctions to every statement in the
// body, including branches, loops and error-handler bodies.
func (v *microflowValidator) walkExprFunctions(body []ast.MicroflowStatement) {
	for _, s := range body {
		v.checkStmtExprFunctions(s)
		switch stmt := s.(type) {
		case *ast.IfStmt:
			v.walkExprFunctions(stmt.ThenBody)
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			for _, c := range stmt.Cases {
				v.walkExprFunctions(c.Body)
			}
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				v.walkExprFunctions(c.Body)
			}
			v.walkExprFunctions(stmt.ElseBody)
		case *ast.LoopStmt:
			v.walkExprFunctions(stmt.Body)
		case *ast.WhileStmt:
			v.walkExprFunctions(stmt.Body)
		}
		if eh := stmtErrorHandling(s); eh != nil {
			v.walkExprFunctions(eh.Body)
		}
	}
}

// validateNanoflowRules is validateMicroflowRules for a nanoflow: exec refuses
// what ValidateNanoflow reports, restricted to the same verified allowlist, so
// check and exec cannot disagree.
func validateNanoflowRules(stmt *ast.CreateNanoflowStmt) error {
	var msgs []string
	for _, v := range ValidateNanoflow(stmt) {
		if v.Severity != linter.SeverityError || !execEnforcedMicroflowRules[v.RuleID] {
			continue
		}
		msg := fmt.Sprintf("[%s] %s", v.RuleID, v.Message)
		if v.Suggestion != "" {
			msg += "\n    " + v.Suggestion
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("nanoflow '%s' has validation errors:\n  - %s",
		stmt.Name.String(), strings.Join(msgs, "\n  - "))
}

// seedPrimitiveKinds records the primitive kind of every parameter and declare,
// which a microflow's walkBody records as it goes. The nanoflow walk does not
// run walkBody, and MDL063 reads these kinds to tell a String contains/find —
// which the builder rewrites into a Change Variable — from a list operation.
func (v *microflowValidator) seedPrimitiveKinds(params []ast.MicroflowParam, body []ast.MicroflowStatement) {
	for _, p := range params {
		if k, ok := astKindToExprKind(p.Type.Kind); ok {
			v.varKinds[p.Name] = k
		}
	}
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if d, ok := s.(*ast.DeclareStmt); ok {
			if k, ok := astKindToExprKind(d.Type.Kind); ok {
				v.varKinds[d.Variable] = k
			}
		}
	})
}
