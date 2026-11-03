// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// continueUnsupportedRule flags `ON ERROR CONTINUE` on an activity Mendix does
// not offer it for.
const continueUnsupportedRule = "MDL076"

// continueUnsupportedOn names the statements whose stored activity rejects
// ErrorHandlingType "Continue", by the caption mxbuild uses in the error.
//
// MEASURED on Mendix 11.14.0, not inferred: each value was written into a stored
// document of an otherwise-clean project and the project built. The split is not
// guessable from the metamodel — one enum covers every action type, and mxbuild
// enforces a per-type subset:
//
//	action           Rollback   Continue   Abort
//	Retrieve            ok         ok        ok
//	Delete              ok         ok        ok
//	MicroflowCall       ok         ok      CE6035
//	CreateVariable      ok         ok         -
//	ChangeVariable      ok         ok         -
//	Log                 ok       CE6035    CE6035
//	Create              ok       CE6035    CE6035
//	Change              ok       CE6035    CE6035
//	Commit              ok       CE6035    CE6035
//	Aggregate           ok       CE6035    CE6035
//	ShowPage            ok       CE6035      -
//	ClosePage           ok       CE6035      -
//	ShowMessage         ok       CE6035      -
//	ValidationFeedback  ok       CE6035      -
//
// A DENY-list rather than an allow-list, deliberately. The statements not named
// here were never measured, and refusing them would reject scripts that may well
// build — the rule reports what is known to fail and stays silent about the rest.
// That is also why the table is per-statement and not "activities that write to
// the database": Delete writes and is fine, Aggregate does not and is not.
//
// The bottom nine rows became REACHABLE with mendixlabs/mxcli#1078, which gave
// eight more statements an onErrorClause so a Studio Pro error handler could
// survive DESCRIBE. Before that they were unreachable from a script and this list
// held only create and commit. Note the split it exposes, which no rule of thumb
// predicts: create-VARIABLE and change-VARIABLE accept Continue while
// change-OBJECT does not — measured on 11.14.0, one microflow per row.
var continueUnsupportedOn = map[string]string{
	"create":              "Create object activity",
	"commit":              "Commit object(s) activity",
	"change":              "Change object activity",
	"log":                 "Log message activity",
	"show page":           "Show page activity",
	"close page":          "Close page activity",
	"show message":        "Show message activity",
	"validation feedback": "Validation feedback activity",
	// mendixlabs/mxcli#175, measured on 11.14.0 in ako/TestApp: continue is
	// CE6035 on a Call workflow activity; no clause, rollback and both custom
	// handlers build.
	"call workflow": "Call workflow activity",
	// mendixlabs/mxcli#1252, measured on 11.12.3, one microflow per cell: no
	// clause, rollback and both custom handlers build on each of these, and
	// continue is CE6035. `rollback` is the exception and is absent — it builds
	// with continue too.
	"list operation": "List operation activity",
	"aggregate":      "Aggregate list activity",
	"create list":    "Create list activity",
	"add":            "Change list activity",
	"remove":         "Change list activity",
	"cast":           "Cast object activity",
	"show home page": "Show home page action activity",
	// mendixlabs/mxcli#1735, measured on mxbuild 11.12.6 against an updatable
	// external entity, one microflow per cell: rollback, custom and custom
	// without rollback build on both; continue is CE6035 on both.
	"send external object":   "Send external object activity",
	"delete external object": "Delete external object activity",
}

// checkErrorHandlingContinueSupported reports `ON ERROR CONTINUE` on a statement
// whose activity Mendix rejects it for.
//
// This one is not a check GAP being closed — mxcli already wrote `Continue` into
// these activities, so the model it produced was one mxbuild refuses:
//
//	$T = CREATE Eh.Thing ( Code = 'a' ) COMMIT ON ERROR CONTINUE;
//	mx check -> [CE6035] "Error handling type is not supported"
//	              at Create object activity 'Create Thing (Code)'
//
// Refused rather than silently downgraded to Rollback. The two mean different
// things at runtime — Continue swallows the error and carries on, Rollback aborts
// the flow — and an author who wrote the clause meant the first; quietly writing
// the second gives a microflow that behaves differently from what its source
// says, which is the failure mode this whole area already had once
// (ako/CapTrackV3 FINDINGS §11).
func (v *microflowValidator) checkErrorHandlingContinueSupported(stmt ast.MicroflowStatement) {
	eh := stmtErrorHandling(stmt)
	if eh == nil || eh.Type != ast.ErrorHandlingContinue {
		return
	}
	keyword, activity := continueUnsupportedStatement(stmt)
	if activity == "" {
		return
	}
	v.addViolation(continueUnsupportedRule, linter.SeverityError,
		fmt.Sprintf("`%s ... on error continue` is not supported — Mendix rejects Continue error handling "+
			"on a %s with CE6035 \"Error handling type is not supported\"", keyword, activity),
		fmt.Sprintf("Drop the clause to keep Mendix's default (rollback and abort), or wrap the statement in a "+
			"custom handler, which %s does accept: `%s ... on error begin ... end error`. Continue IS supported on "+
			"`retrieve`, `delete` and `call microflow` — measured on 11.14.0.", activity, keyword))
}

// continueUnsupportedStatement returns the MDL keyword and the mxbuild activity
// caption for a statement whose activity rejects Continue, or "" for one that
// accepts it or was never measured.
func continueUnsupportedStatement(stmt ast.MicroflowStatement) (keyword, activity string) {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		keyword = "create"
	case *ast.SendExternalObjectStmt:
		keyword = "send external object"
	case *ast.DeleteExternalObjectStmt:
		keyword = "delete external object"
	case *ast.MfCommitStmt:
		keyword = "commit"
	case *ast.ChangeObjectStmt:
		keyword = "change"
	case *ast.LogStmt:
		keyword = "log"
	case *ast.ShowPageStmt:
		keyword = "show page"
	case *ast.ClosePageStmt:
		keyword = "close page"
	case *ast.ShowMessageStmt:
		keyword = "show message"
	case *ast.ValidationFeedbackStmt:
		keyword = "validation feedback"
	case *ast.CallWorkflowStmt:
		keyword = "call workflow"
	case *ast.ListOperationStmt:
		keyword = "list operation"
	case *ast.AggregateListStmt:
		keyword = "aggregate"
	case *ast.CreateListStmt:
		keyword = "create list"
	case *ast.AddToListStmt:
		// `add $X to $Obj/Assoc` is a Change object activity (#1288).
		keyword = "add"
		if s.Association != "" {
			keyword = "change"
		}
	case *ast.RemoveFromListStmt:
		keyword = "remove"
		if s.Association != "" {
			keyword = "change"
		}
	case *ast.CastObjectStmt:
		keyword = "cast"
	case *ast.ShowHomePageStmt:
		keyword = "show home page"
	default:
		// DeclareStmt and MfSetStmt are deliberately absent: create-variable and
		// change-variable accept Continue on 11.14.0. So is RollbackStmt, on
		// 11.12.3 (#1252).
		return "", ""
	}
	return keyword, continueUnsupportedOn[keyword]
}
