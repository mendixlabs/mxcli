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

// getErrorHandling extracts the ErrorHandlingClause from statements that have one.
//
// Only statements reachable in nanoflows (i.e., NOT in the denylist) need coverage
// here. Disallowed actions are rejected by checkDisallowedNanoflowAction before
// this function is called. Statements like ListOperationStmt that have no
// ErrorHandling field are also omitted (they return nil implicitly via default).
func getErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	}
	return nil
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
