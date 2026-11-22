// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for consumed REST client operations.
//
// `Body:`/`Response: MAPPING X` names an entity and maps JSON fields onto it in
// a `{ ... }` body; Mendix stores that inline on the operation. There is no
// response handling that references an import/export mapping *document* — the
// metamodel offers exactly two (Rest$ImplicitMappingResponseHandling and
// Rest$NoResponseHandling). Written without a body the clause therefore carries
// no mapping at all, and used to be persisted as "no response handling" without
// a word of warning — see issue #843.
//
// The check compares only what is already in the statement, so it needs no
// project.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateRestClientMappings reports (MDL-REST01) a REST client operation whose
// body/response mapping clause has no mapping body.
func ValidateRestClientMappings(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		var opDefs []*ast.RestOperationDef
		switch s := stmt.(type) {
		case *ast.CreateRestClientStmt:
			opDefs = s.Operations
		case *ast.AlterRestClientStmt:
			for _, a := range s.Actions {
				switch a := a.(type) {
				case *ast.RestClientAddOperationAction:
					opDefs = append(opDefs, a.Operation)
				case *ast.RestClientModifyOperationAction:
					opDefs = append(opDefs, a.Operation)
				}
			}
		default:
			continue
		}
		for _, opDef := range opDefs {
			if err := checkInlineMappingBody(opDef); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-REST01",
					Severity: linter.SeverityError,
					Message:  "operation \"" + opDef.Name + "\": " + err.Error(),
					Suggestion: "List the JSON fields inline. A consumed REST operation stores its mapping on the " +
						"operation itself, so an existing import/export mapping document cannot be referenced here.",
				})
			}
			// MDL-REST02: a file request body has no representation in Mendix and
			// used to be downgraded to a string body holding the expression text,
			// which returns 200 with the wrong payload.
			if err := checkFileRequestBody(opDef); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-REST02",
					Severity: linter.SeverityError,
					Message:  "operation \"" + opDef.Name + "\": " + err.Error(),
					Suggestion: "Upload binary from a Java action. Mendix's consumed REST operation has no " +
						"binary request body, so there is no syntax here that would send the file.",
				})
			}
		}
	}
	return out
}

// ValidatePublishedRestCommit reports (MDL-REST03) a published REST operation
// whose commit clause names no commit option. exec refuses the same statement
// (publishedRestCommit), so check predicts it.
func ValidatePublishedRestCommit(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	visit := func(res *ast.PublishedRestResourceDef) {
		if res == nil {
			return
		}
		for _, op := range res.Operations {
			if _, err := publishedRestCommit(op.Commit); err != nil {
				out = append(out, linter.Violation{
					RuleID:     "MDL-REST03",
					Severity:   linter.SeverityError,
					Message:    "resource '" + res.Name + "', operation " + op.HTTPMethod + " " + op.Path + ": " + err.Error(),
					Suggestion: "Write commit Yes, commit YesWithoutEvents or commit No, or leave the clause out (Yes).",
				})
			}
		}
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePublishedRestServiceStmt:
			for _, res := range s.Resources {
				visit(res)
			}
		case *ast.AlterPublishedRestServiceStmt:
			for _, a := range s.Actions {
				switch add := a.(type) {
				case *ast.PublishedRestAddResourceAction:
					visit(add.Resource)
				case *ast.PublishedRestAddOperationsAction:
					visit(&ast.PublishedRestResourceDef{Name: add.Resource, Operations: add.Operations})
				}
			}
		}
	}
	return out
}
