// SPDX-License-Identifier: Apache-2.0

// Package executor - nanoflow-specific validation rules
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateNanoflowBody checks that a nanoflow body does not contain disallowed
// actions or flow objects. Returns a list of human-readable error messages.
func validateNanoflowBody(body []ast.MicroflowStatement) []string {
	var errors []string
	validateNanoflowStatements(body, &errors)
	return errors
}

func validateNanoflowStatements(stmts []ast.MicroflowStatement, errors *[]string) {
	for _, stmt := range stmts {
		if reason := checkDisallowedNanoflowAction(stmt); reason != "" {
			*errors = append(*errors, reason)
			continue
		}
		if reason := checkNanoflowErrorHandling(stmt); reason != "" {
			*errors = append(*errors, reason)
		}
		// Recurse into compound statements
		switch s := stmt.(type) {
		case *ast.IfStmt:
			validateNanoflowStatements(s.ThenBody, errors)
			validateNanoflowStatements(s.ElseBody, errors)
		case *ast.LoopStmt:
			validateNanoflowStatements(s.Body, errors)
		case *ast.WhileStmt:
			validateNanoflowStatements(s.Body, errors)
		}
		// Also recurse into error handling bodies
		if eh := getErrorHandling(stmt); eh != nil && eh.Body != nil {
			validateNanoflowStatements(eh.Body, errors)
		}
	}
}

// checkDisallowedNanoflowAction returns a human-readable error message if the
// statement is not allowed in nanoflows, or empty string if allowed.
//
// MAINTENANCE: This uses a denylist approach (12 case branches, 22 action types) —
// any action type NOT listed here is implicitly allowed. When adding new action
// AST types, check whether they are available in nanoflows (see Mendix docs
// "Nanoflows" > "Activities") and add a case here if they are server-side only.
// The manual QA test plan (docs/15-testing/nanoflow-test-cases.md §4.1) lists
// all disallowed actions and should be updated in parallel.
func checkDisallowedNanoflowAction(stmt ast.MicroflowStatement) string {
	switch stmt.(type) {
	case *ast.RaiseErrorStmt:
		return "ErrorEvent is not allowed in nanoflows"
	case *ast.CallJavaActionStmt:
		return "Java actions cannot be called from nanoflows"
	case *ast.ExecuteDatabaseQueryStmt:
		return "database queries are not allowed in nanoflows"
	case *ast.CallExternalActionStmt:
		return "external action calls are not allowed in nanoflows"
	case *ast.ShowHomePageStmt:
		return "SHOW HOME PAGE is not allowed in nanoflows"
	case *ast.RestCallStmt:
		return "REST calls are not allowed in nanoflows"
	case *ast.SendRestRequestStmt:
		return "REST requests are not allowed in nanoflows"
	case *ast.SendEmailStmt:
		return "SEND EMAIL is not allowed in nanoflows — mail is sent over SMTP from the server"
	case *ast.ImportFromMappingStmt:
		return "import mapping is not allowed in nanoflows"
	case *ast.ExportToMappingStmt:
		return "export mapping is not allowed in nanoflows"
	case *ast.TransformJsonStmt:
		return "JSON transformation is not allowed in nanoflows"
	// Workflow actions — all server-side only
	case *ast.CallWorkflowStmt,
		*ast.GetWorkflowDataStmt,
		*ast.GetWorkflowsStmt,
		*ast.GetWorkflowActivityRecordsStmt,
		*ast.WorkflowOperationStmt,
		*ast.SetTaskOutcomeStmt,
		*ast.OpenUserTaskStmt,
		*ast.NotifyWorkflowStmt,
		*ast.OpenWorkflowStmt,
		*ast.LockWorkflowStmt,
		*ast.UnlockWorkflowStmt:
		return "workflow actions are not allowed in nanoflows"
	case *ast.DownloadFileStmt:
		return "file downloads are not allowed in nanoflows"
	}
	return ""
}

// getErrorHandling extracts the ErrorHandlingClause from a statement that has
// one. It is stmtErrorHandling: a second hand-kept list of the statements that
// take a clause drifted from the first, which is how a handler body went
// unwalked here.
func getErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	return stmtErrorHandling(stmt)
}

// validateNanoflowReturnType checks that the return type is allowed for nanoflows.
// Binary return type is not supported in nanoflows.
func validateNanoflowReturnType(retType *ast.MicroflowReturnType) string {
	if retType == nil {
		return ""
	}
	switch retType.Type.Kind {
	case ast.TypeBinary:
		return "Binary return type is not allowed in nanoflows"
	}
	return ""
}

// validateNanoflow runs all nanoflow-specific validations and returns a combined
// error message, or empty string if valid.
func validateNanoflow(name string, body []ast.MicroflowStatement, retType *ast.MicroflowReturnType) string {
	var allErrors []string

	if msg := validateNanoflowReturnType(retType); msg != "" {
		allErrors = append(allErrors, msg)
	}

	allErrors = append(allErrors, validateNanoflowBody(body)...)

	if len(allErrors) == 0 {
		return ""
	}

	var errMsg strings.Builder
	errMsg.WriteString(fmt.Sprintf("nanoflow '%s' has validation errors:\n", name))
	for _, e := range allErrors {
		errMsg.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	return errMsg.String()
}

// nanoflowErrorHandlingUnsupported names the activities that accept NO error
// handling at all inside a nanoflow, by the caption mxbuild uses.
//
// MEASURED on Mendix 11.14.0, one nanoflow per cell, not inferred. A dash is a
// cell that was NOT measured, not one that passed — every activity listed in the
// map below has at least one measured CE6035, and none has a measured pass:
//
//	activity (in a NANOFLOW)   continue   rollback   custom { }
//	CreateVariable (declare)      ok          -          ok
//	ChangeVariable (set)           -          -          ok
//	ChangeObject                   -       CE6035      CE6035
//	Log                         CE6035     CE6035      CE6035
//	ShowPage                       -          -        CE6035
//	ClosePage                      -       CE6035      CE6035
//	ShowMessage                    -          -        CE6035
//	ValidationFeedback             -       CE6035      CE6035
//
// The rollback column comes from #1078's own regression: writing "Rollback"
// instead of the nanoflow default Abort was CE6035 on exactly ChangeObject,
// ClosePage and ValidationFeedback in the doctype scripts. Log is the one
// activity measured in all three forms, and rejects all three — which is why the
// rule refuses any clause rather than one form: the accepted value is Abort, and
// no MDL syntax writes it.
//
// The two VARIABLE activities are the permissive pair here, exactly as they are
// for `continue` in a microflow (see continueUnsupportedOn) — the split is by
// activity, not by "client-side vs server-side".
//
// This is a nanoflow-only rule and cannot live in MDL076: that one runs on the
// microflow validator, which has no flow flavour. Reaching this at all is new —
// mendixlabs/mxcli#1078 gave these six statements an onErrorClause so a Studio
// Pro error handler could survive DESCRIBE, and a nanoflow accepts none of them.
var nanoflowErrorHandlingUnsupported = map[string]string{
	"change":              "Change object activity",
	"log":                 "Log message activity",
	"show page":           "Show page activity",
	"close page":          "Close page activity",
	"show message":        "Show message activity",
	"validation feedback": "Validation feedback activity",
}

// checkNanoflowErrorHandling reports an ON ERROR clause on a nanoflow activity
// that cannot carry one.
//
// Refused rather than dropped or downgraded: the alternatives are a nanoflow
// mxbuild rejects (CE6035) or one that silently does something the script does
// not say. `mxcli check` passed the rejected form until this rule existed.
func checkNanoflowErrorHandling(stmt ast.MicroflowStatement) string {
	if getErrorHandling(stmt) == nil {
		return ""
	}
	var keyword string
	switch s := stmt.(type) {
	case *ast.ChangeObjectStmt:
		keyword = "change"
	case *ast.AddToListStmt, *ast.RemoveFromListStmt:
		// A reference-set target is a Change object activity (#1288); a list
		// target is a Change list activity, which takes a handler without
		// rollback (#1252).
		if assoc := addRemoveAssociation(s); assoc != "" {
			keyword = "change"
		} else {
			return checkNanoflowWithoutRollbackOnly(stmt)
		}
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
	case *ast.CreateObjectStmt, *ast.MfCommitStmt, *ast.CallNanoflowStmt, *ast.CallMicroflowStmt,
		*ast.ListOperationStmt, *ast.AggregateListStmt, *ast.CreateListStmt, *ast.CastObjectStmt:
		return checkNanoflowWithoutRollbackOnly(stmt)
	default:
		// declare and set are deliberately absent: both variable activities accept
		// every form in a nanoflow, and so do retrieve and delete (measured,
		// mendixlabs/mxcli#591) and rollback (11.12.3, mendixlabs/mxcli#1252). The remaining statements that carry the clause
		// are unmeasured here, and refusing them would reject nanoflows that
		// build today.
		return ""
	}
	return "`" + keyword + " ... on error` is not supported in a nanoflow — Mendix rejects " +
		"error handling on a " + nanoflowErrorHandlingUnsupported[keyword] +
		" there with CE6035 \"Error handling type is not supported\". Drop the clause " +
		"(a nanoflow activity aborts the flow on error by default)"
}

// nanoflowWithoutRollbackOnly names the nanoflow activities that accept exactly
// one error-handling clause: a custom handler WITHOUT rollback.
//
// MEASURED on Mendix 11.14.0 (an ako/TestApp copy, one nanoflow per cell),
// mendixlabs/mxcli#591:
//
//	activity (in a NANOFLOW)   continue   rollback   custom   without rollback
//	Create object               CE6035     CE6035    CE6035        ok
//	Commit                      CE6035     CE6035    CE6035        ok
//	Call nanoflow               CE6035     CE6035    CE6035        ok
//	Call microflow              CE6035     CE6035    CE6035        ok
//	Retrieve                      ok         ok        ok          ok
//	Delete                        ok         ok        ok          ok
//
// mendixlabs/mxcli#1252 added the list activities and cast, MEASURED on 11.12.3
// the same way and with the same row as Create object: List operation,
// Aggregate list, Create list, Change list (add/remove) and Cast object accept
// only the handler without rollback. Rollback object accepts every form.
//
// With no clause the activity stores the nanoflow default Abort, which builds;
// a nanoflow has no transaction for "rollback" to undo, and Studio Pro's own
// nanoflows in ako/TestApp store only Abort and CustomWithoutRollBack.
var nanoflowWithoutRollbackOnly = map[string]string{
	"create":         "Create object activity",
	"commit":         "Commit object(s) activity",
	"call nanoflow":  "Nanoflow call action activity",
	"call microflow": "Call microflow activity",
	"list operation": "List operation activity",
	"aggregate":      "Aggregate list activity",
	"create list":    "Create list activity",
	"add":            "Change list activity",
	"remove":         "Change list activity",
	"cast":           "Cast object activity",
}

func checkNanoflowWithoutRollbackOnly(stmt ast.MicroflowStatement) string {
	eh := getErrorHandling(stmt)
	if eh == nil || eh.Type == ast.ErrorHandlingCustomWithoutRollback {
		return ""
	}
	var keyword string
	switch stmt.(type) {
	case *ast.CreateObjectStmt:
		keyword = "create"
	case *ast.MfCommitStmt:
		keyword = "commit"
	case *ast.CallNanoflowStmt:
		keyword = "call nanoflow"
	case *ast.CallMicroflowStmt:
		keyword = "call microflow"
	case *ast.ListOperationStmt:
		keyword = "list operation"
	case *ast.AggregateListStmt:
		keyword = "aggregate"
	case *ast.CreateListStmt:
		keyword = "create list"
	case *ast.AddToListStmt:
		keyword = "add"
	case *ast.RemoveFromListStmt:
		keyword = "remove"
	case *ast.CastObjectStmt:
		keyword = "cast"
	default:
		return ""
	}
	form := "on error rollback"
	switch eh.Type {
	case ast.ErrorHandlingContinue:
		form = "on error continue"
	case ast.ErrorHandlingCustom:
		form = "on error begin … end error"
	}
	return "`" + keyword + " ... " + form + "` is not supported in a nanoflow — Mendix rejects it on a " +
		nanoflowWithoutRollbackOnly[keyword] + " there with CE6035 \"Error handling type is not supported\". " +
		"A nanoflow has no transaction to roll back: drop the clause (the activity aborts the flow on error), " +
		"or handle the error with `on error without rollback begin … end error`"
}

// addRemoveAssociation returns the association an add/remove targets, or "" for
// a list target.
func addRemoveAssociation(stmt ast.MicroflowStatement) string {
	switch s := stmt.(type) {
	case *ast.AddToListStmt:
		return s.Association
	case *ast.RemoveFromListStmt:
		return s.Association
	}
	return ""
}
