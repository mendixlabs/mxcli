// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Rule restrictions, from Mendix's own reference (docs.mendix.com/refguide/rules):
// a rule "is a special kind of microflow" whose output must be a Boolean or an
// enumeration, may only be used from a decision, and
//
//   - cannot create, delete, modify or roll back database objects;
//   - cannot show a page, close a window, show a message, send validation
//     feedback, or start a file download;
//   - cannot call a web service, generate a document, or import/export XML.
//
// The denylist mirrors checkDisallowedNanoflowAction, and carries the same
// maintenance property: an action type not listed here is implicitly allowed, so
// a new AST statement that a rule may not contain needs a case adding.
//
// mxbuild enforces all of this, so these refusals are measured rather than
// derived from the documentation. Verified on mxbuild 11.13.0 by converting a
// microflow unit into a Microflows$Rule to get past this validator:
//
//	create object in a rule → CE0009 "This action is not supported in rules."
//	void or String return   → CE0103 "The return type should be one of the
//	                          following types: Boolean or Enumeration." and
//	                          CE0139 "The return type of a rule must be either
//	                          Boolean or an enumeration type."
//
// They are refused at check time rather than left to the build because the
// grammar deliberately shares microflowBody: restricting the body in the grammar
// would turn every violation into a parse error naming a token instead of the
// activity.
func checkDisallowedRuleAction(stmt ast.MicroflowStatement) string {
	switch stmt.(type) {
	// Data changes — the restriction that makes a rule cheaper than a microflow.
	case *ast.CreateObjectStmt:
		return "a rule cannot create objects"
	case *ast.ChangeObjectStmt:
		return "a rule cannot change objects"
	case *ast.DeleteObjectStmt:
		return "a rule cannot delete objects"
	case *ast.SendExternalObjectStmt:
		return "a rule cannot send external objects"
	case *ast.DeleteExternalObjectStmt:
		return "a rule cannot delete external objects"
	case *ast.MfCommitStmt:
		return "a rule cannot commit objects"
	case *ast.RollbackStmt:
		return "a rule cannot roll back objects"

	// Client interaction — a rule has no client to talk to.
	case *ast.ShowPageStmt:
		return "a rule cannot show a page"
	case *ast.ClosePageStmt:
		return "a rule cannot close a page"
	case *ast.ShowMessageStmt:
		return "a rule cannot show a message"
	case *ast.ValidationFeedbackStmt:
		return "a rule cannot send validation feedback"
	case *ast.ShowHomePageStmt:
		return "a rule cannot show the home page"
	case *ast.DownloadFileStmt:
		return "a rule cannot start a file download"

	// Integration. (Mendix also bans document generation from a rule; mxcli has
	// no GENERATE DOCUMENT statement, so there is nothing to refuse yet.)
	case *ast.CallWebServiceStmt:
		return "a rule cannot call a web service"
	case *ast.ImportFromMappingStmt:
		return "a rule cannot import XML/JSON via a mapping"
	case *ast.ExportToMappingStmt:
		return "a rule cannot export XML/JSON via a mapping"
	}
	return ""
}

// validateRuleBody walks the body — including branch, loop and error-handler
// bodies — collecting every disallowed activity.
func validateRuleBody(body []ast.MicroflowStatement) []string {
	var errors []string
	validateRuleStatements(body, &errors)
	return errors
}

func validateRuleStatements(stmts []ast.MicroflowStatement, errors *[]string) {
	for _, stmt := range stmts {
		if reason := checkDisallowedRuleAction(stmt); reason != "" {
			*errors = append(*errors, reason)
			continue
		}
		switch s := stmt.(type) {
		case *ast.IfStmt:
			validateRuleStatements(s.ThenBody, errors)
			validateRuleStatements(s.ElseBody, errors)
		case *ast.LoopStmt:
			validateRuleStatements(s.Body, errors)
		case *ast.WhileStmt:
			validateRuleStatements(s.Body, errors)
		}
		if eh := getErrorHandling(stmt); eh != nil && eh.Body != nil {
			validateRuleStatements(eh.Body, errors)
		}
	}
}

// validateRuleReturnType enforces the one restriction that is about the
// signature rather than the body: a rule returns a Boolean or an enumeration.
//
// A missing return type is an error too. For a microflow, void is a legitimate
// choice; for a rule it means the decision that calls it has nothing to branch
// on, so silently writing a void rule produces a document no decision can use.
func validateRuleReturnType(retType *ast.MicroflowReturnType) string {
	if retType == nil {
		return "a rule must return a Boolean or an enumeration — add `returns Boolean` or `returns enum Module.Enum`"
	}
	switch retType.Type.Kind {
	case ast.TypeBoolean, ast.TypeEnumeration:
		return ""
	case ast.TypeVoid:
		return "a rule must return a Boolean or an enumeration, not void"
	default:
		return fmt.Sprintf("a rule must return a Boolean or an enumeration, not %s", retType.Type.Kind.String())
	}
}

// validateRule returns a formatted error message, or "" when the rule is valid.
// Called by both `mxcli check` and exec, so the two cannot disagree.
func validateRule(name string, body []ast.MicroflowStatement, retType *ast.MicroflowReturnType) string {
	var allErrors []string

	if msg := validateRuleReturnType(retType); msg != "" {
		allErrors = append(allErrors, msg)
	}
	allErrors = append(allErrors, validateRuleBody(body)...)
	if msg := raiseErrorOutsideHandlerRuleError(body); msg != "" {
		allErrors = append(allErrors, msg)
	}

	if len(allErrors) == 0 {
		return ""
	}

	var errMsg strings.Builder
	errMsg.WriteString(fmt.Sprintf("rule '%s' has validation errors:\n", name))
	for _, e := range allErrors {
		errMsg.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	return errMsg.String()
}

// validateRuleSetTargets is exec's MDL-SET01 gate for a rule: a `set` on a
// rule parameter is CE7247 "Parameter 'N' cannot be changed." (measured on
// 11.14.0). check reports it through ValidateProgram, so it is not part of
// validateRule, which check --references also runs — that would print it twice.
func validateRuleSetTargets(name string, params []ast.MicroflowParam, body []ast.MicroflowStatement) error {
	var msgs []string
	for _, v := range setTargetViolations("rule", name, params, body) {
		msgs = append(msgs, fmt.Sprintf("[%s] %s", v.RuleID, v.Message))
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("rule '%s' has validation errors:\n  - %s", name, strings.Join(msgs, "\n  - "))
}
