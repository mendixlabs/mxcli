// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: semantic validation
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ValidateMicroflowBody validates the microflow body for semantic errors without building objects.
// This is used by the check command to validate scripts without executing them.
func ValidateMicroflowBody(s *ast.CreateMicroflowStmt) []string {
	return validateFlowBody(s.Parameters, s.ReturnType, s.Body, true, nil)
}

// ValidateNanoflowBody validates the nanoflow body for semantic errors without building objects.
// This is used by the check command to validate scripts without executing them.
func ValidateNanoflowBody(s *ast.CreateNanoflowStmt) []string {
	return validateFlowBody(s.Parameters, s.ReturnType, s.Body, true, nil)
}

// validateFlowBody validates parameters and body statements for semantic errors.
// duplicatesOwnedElsewhere leaves duplicate variable names to MDL063 — see
// flowBuilder.duplicateNamesOwnedElsewhere. assocs, when check --references has
// a project, types an association retrieve as the object or list it yields;
// nil leaves every association retrieve a list, as without a project.
func validateFlowBody(params []ast.MicroflowParam, returns *ast.MicroflowReturnType, body []ast.MicroflowStatement, duplicatesOwnedElsewhere bool, assocs map[string]assocShape) []string {
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)

	var paramErrors []string
	for _, p := range params {
		if p.Type.EntityRef != nil {
			if p.Type.EntityRef.Module == "" {
				paramErrors = append(paramErrors, fmt.Sprintf(
					"parameter '$%s': entity type '%s' is missing module prefix (use 'Module.%s')",
					p.Name, p.Type.EntityRef.Name, p.Type.EntityRef.Name))
				continue
			}
			entityQN := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				varTypes[p.Name] = "List of " + entityQN
			} else {
				varTypes[p.Name] = entityQN
			}
		} else {
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}
	if len(paramErrors) > 0 {
		return paramErrors
	}

	fb := &flowBuilder{
		varTypes:                     varTypes,
		declaredVars:                 declaredVars,
		returnType:                   returns, // only for messages (addUndeclaredSetError)
		errors:                       []string{},
		duplicateNamesOwnedElsewhere: duplicatesOwnedElsewhere,
		checkAssocShapes:             assocs,
		assocObjectVars:              map[string]bool{},
		joinScopes:                   map[string]flowBuilderVariableState{},
	}

	fb.validateStatements(body)

	return fb.errors
}

// validateStatements recursively validates statements for semantic errors.
func (fb *flowBuilder) validateStatements(stmts []ast.MicroflowStatement) {
	for _, stmt := range stmts {
		fb.validateStatement(stmt)
	}
}

func (fb *flowBuilder) validateScopedStatements(stmts []ast.MicroflowStatement) {
	scoped := *fb
	scoped.varTypes = cloneStringMap(fb.varTypes)
	scoped.declaredVars = cloneStringMap(fb.declaredVars)
	scoped.validateStatements(stmts)
	fb.errors = scoped.errors
}

// validateStatement validates a single statement for semantic errors.
func (fb *flowBuilder) validateStatement(stmt ast.MicroflowStatement) {
	switch s := stmt.(type) {
	case *ast.DeclareStmt:
		// Check for duplicate variable declaration
		if !fb.duplicateNamesOwnedElsewhere && fb.isVariableDeclared(s.Variable) {
			fb.addError("duplicate variable name '$%s' — variable is already declared (CE0111)", s.Variable)
		}
		// Register the variable as declared
		if s.Type.EntityRef != nil {
			// Entity type declaration
			fb.varTypes[s.Variable] = s.Type.EntityRef.Module + "." + s.Type.EntityRef.Name
		} else {
			// Primitive type declaration
			fb.declaredVars[s.Variable] = s.Type.Kind.String()
		}

	case *ast.MfSetStmt:
		// Validate that the variable has been declared
		if !fb.isVariableDeclared(s.Target) {
			fb.addUndeclaredSetError(s.Target)
		} else if fb.assocObjectVars[strings.TrimPrefix(s.Target, "$")] {
			// Only the association-retrieved object: every other object
			// producer is visible without a project, and MDL-SET01 reports
			// those once (checkSetOnObjectVariable).
			fb.refuseSetOnObject(s.Target)
		}

	case *ast.IfStmt:
		fb.validateScopedStatements(s.ThenBody)
		if len(s.ElseBody) > 0 {
			fb.validateScopedStatements(s.ElseBody)
		}

	case *ast.EnumSplitStmt:
		for _, c := range s.Cases {
			fb.validateScopedStatements(c.Body)
		}
		if len(s.ElseBody) > 0 {
			fb.validateScopedStatements(s.ElseBody)
		}

	case *ast.InheritanceSplitStmt:
		for _, c := range s.Cases {
			fb.validateScopedStatements(c.Body)
		}
		if len(s.ElseBody) > 0 {
			fb.validateScopedStatements(s.ElseBody)
		}

	case *ast.LoopStmt:
		// Register loop variable (derived from list type)
		if s.ListVariable != "" {
			// Try to get the list type from varTypes
			if listType, ok := fb.varTypes[s.ListVariable]; ok {
				// "List of Module.Entity" -> "Module.Entity"
				if len(listType) > 8 && listType[:8] == "List of " {
					fb.varTypes[s.LoopVariable] = listType[8:]
				}
			}
		}
		// Validate loop body
		fb.validateStatements(s.Body)

	case *ast.CreateObjectStmt:
		fb.validateOutputVariable(s.Variable, "create")
		// Register created variable as entity type
		if s.Variable != "" && s.EntityType.Module != "" {
			fb.varTypes[s.Variable] = s.EntityType.Module + "." + s.EntityType.Name
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallMicroflowStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call microflow")
			mfQN := s.MicroflowName.Module + "." + s.MicroflowName.Name
			if returnType := fb.lookupMicroflowReturnType(mfQN); returnType != nil {
				fb.registerResultVariableType(s.OutputVariable, returnType)
			} else {
				// We don't know the return type, so just mark it as declared
				fb.declaredVars[s.OutputVariable] = "Unknown"
			}
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallNanoflowStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call nanoflow")
			nfQN := s.NanoflowName.Module + "." + s.NanoflowName.Name
			if returnType := fb.lookupNanoflowReturnType(nfQN); returnType != nil {
				fb.registerResultVariableType(s.OutputVariable, returnType)
			} else {
				fb.declaredVars[s.OutputVariable] = "Unknown"
			}
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallJavaActionStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call java action")
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.DownloadFileStmt:
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.SynchronizeStmt:
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.SendEmailStmt:
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallJavaScriptActionStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call javascript action")
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallWebServiceStmt:
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call web service")
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.ExecuteDatabaseQueryStmt:
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "execute database query")
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CallExternalActionStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "call external action")
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.RestCallStmt:
		// Register result variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "rest call")
			// Type depends on result handling
			switch s.Result.Type {
			case ast.RestResultString:
				fb.declaredVars[s.OutputVariable] = "String"
			case ast.RestResultResponse:
				fb.declaredVars[s.OutputVariable] = "System.HttpResponse"
			case ast.RestResultMapping:
				if s.Result.ResultEntity.Module != "" {
					fb.varTypes[s.OutputVariable] = s.Result.ResultEntity.Module + "." + s.Result.ResultEntity.Name
				} else {
					fb.declaredVars[s.OutputVariable] = "Unknown"
				}
			default:
				fb.declaredVars[s.OutputVariable] = "String"
			}
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.SendRestRequestStmt:
		// Register output variable if assigned
		if s.OutputVariable != "" {
			fb.validateOutputVariable(s.OutputVariable, "send rest request")
			fb.declaredVars[s.OutputVariable] = "Unknown" // Type depends on operation response mapping
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.MfCommitStmt:
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.DeleteObjectStmt:
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.RollbackStmt:
		// No error handling to validate

	case *ast.ExternalObjectStmt:
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.RetrieveStmt:
		fb.validateOutputVariable(s.Variable, "retrieve")
		// Register retrieved variable
		if s.Variable != "" && s.Source.Module != "" {
			if s.StartVariable != "" {
				if to, ok := fb.forwardReferenceTarget(s); ok {
					fb.varTypes[s.Variable] = to
					fb.assocObjectVars[s.Variable] = true
				} else {
					fb.varTypes[s.Variable] = "List of " + s.Source.Module + "." + s.Source.Name
				}
			} else if s.First {
				fb.varTypes[s.Variable] = s.Source.Module + "." + s.Source.Name
			} else {
				fb.varTypes[s.Variable] = "List of " + s.Source.Module + "." + s.Source.Name
			}
		}
		// Validate error handler body if present
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.CreateListStmt:
		fb.validateOutputVariable(s.Variable, "create list")
		if s.Variable != "" && s.EntityType.Module != "" {
			fb.varTypes[s.Variable] = "List of " + s.EntityType.Module + "." + s.EntityType.Name
		}

	case *ast.ListOperationStmt:
		// String contains(haystack, needle) / find(haystack, needle) are value
		// expressions assigned to a pre-declared variable, not list operations that
		// create a new variable (ledger #53/#63). Validate like a SET: the target
		// must already be declared, and must not be flagged as a duplicate.
		if (s.Operation == ast.ListOpContains || s.Operation == ast.ListOpFind) &&
			fb.declaredVars[s.InputVariable] == "String" {
			if !fb.isVariableDeclared(s.OutputVariable) {
				fb.addErrorWithExample(
					fmt.Sprintf("variable '%s' is not declared", s.OutputVariable),
					errorExampleDeclareVariable(s.OutputVariable))
			}
			break
		}
		fb.validateOutputVariable(s.OutputVariable, "list operation")
		if s.OutputVariable != "" {
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}

	case *ast.AggregateListStmt:
		fb.validateOutputVariable(s.OutputVariable, "aggregate list")
		if s.OutputVariable != "" {
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}

	case *ast.ImportFromMappingStmt:
		fb.validateOutputVariable(s.OutputVariable, "import mapping")
		if s.OutputVariable != "" {
			fb.declaredVars[s.OutputVariable] = "Unknown"
		}
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.ExportToMappingStmt:
		fb.validateOutputVariable(s.OutputVariable, "export mapping")
		if s.OutputVariable != "" {
			fb.declaredVars[s.OutputVariable] = "String"
		}
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.TransformJsonStmt:
		fb.validateOutputVariable(s.OutputVariable, "transform json")
		if s.OutputVariable != "" {
			fb.declaredVars[s.OutputVariable] = "String"
		}
		if s.ErrorHandling != nil && len(s.ErrorHandling.Body) > 0 {
			fb.validateStatements(s.ErrorHandling.Body)
		}

	case *ast.JoinStmt:
		fb.recordJoinScope(s.Label)

	case *ast.MergeStmt:
		fb.enterMergeScope(s.Label)

		// Other statement types don't declare variables.
	}
}

// recordJoinScope remembers what is declared where a path `join`s a label.
//
// What is in scope after a `merge <label>` is not what precedes it in the text:
// the merge is entered from its joins, wherever they are. A description of an
// irreducible graph puts a shared region in a section after the branches that
// reach it (emitCrossedMergeSections), so read in text order every variable
// those branches declared looked undeclared, and the faithful description of a
// valid microflow failed check (AgentCommons.Tool_Validate, Evora).
func (fb *flowBuilder) recordJoinScope(label string) {
	if fb.joinScopes == nil {
		return
	}
	scope, ok := fb.joinScopes[label]
	if !ok {
		scope = flowBuilderVariableState{varTypes: map[string]string{}, declaredVars: map[string]string{}}
		fb.joinScopes[label] = scope
	}
	for k, v := range fb.varTypes {
		scope.varTypes[k] = v
	}
	for k, v := range fb.declaredVars {
		scope.declaredVars[k] = v
	}
}

// enterMergeScope adds what the label's joins (so far) had declared. A union,
// not an intersection: this validator exists to catch a misspelt or missing
// variable, and a description of a model Studio Pro accepted must not fail it
// on a path-sensitivity question it does not otherwise ask. A backward join
// (a retry loop) comes after the merge and contributes nothing, which is right:
// what it carries round was declared before the merge in the first place.
func (fb *flowBuilder) enterMergeScope(label string) {
	scope, ok := fb.joinScopes[label]
	if !ok {
		return
	}
	for k, v := range scope.varTypes {
		if _, exists := fb.varTypes[k]; !exists {
			fb.varTypes[k] = v
		}
	}
	for k, v := range scope.declaredVars {
		if _, exists := fb.declaredVars[k]; !exists {
			fb.declaredVars[k] = v
		}
	}
}

func (fb *flowBuilder) validateOutputVariable(varName, statement string) {
	if varName == "" || fb.duplicateNamesOwnedElsewhere {
		return
	}
	if fb.isVariableDeclared(varName) {
		fb.addError("duplicate variable name '$%s' — %s output variable is already declared in this scope (CE0111)", varName, statement)
	}
}

// ValidateRuleBody validates a rule body for semantic errors (undeclared
// variables and the like) without building objects — the check-command
// counterpart of ValidateMicroflowBody. What a rule may not *contain* is a
// separate question, answered by validateRule.
func ValidateRuleBody(s *ast.CreateRuleStmt) []string {
	return validateFlowBody(s.Parameters, nil, s.Body, false, nil)
}

// forwardReferenceTarget reports the entity a retrieve over association yields
// when that is one object: a Reference followed from its FROM entity, by the
// rule the builder takes (associationRetrieveEnd). A self-reference is
// followed from its FROM end, so it is that object too: it used to stay a
// list here, and `set` on it passed as a Change list Replace over what Mendix
// types as an object (ako/mxcli#1793). A reverse traversal, a ReferenceSet or
// an association check cannot resolve stays a list, so the rules keyed on it
// never see an object that is not one.
func (fb *flowBuilder) forwardReferenceTarget(s *ast.RetrieveStmt) (string, bool) {
	shape, ok := fb.checkAssocShapes[strings.ToLower(s.Source.Module+"."+s.Source.Name)]
	if !ok || shape.refSet {
		return "", false
	}
	start := fb.varTypes[s.StartVariable]
	end := associationRetrieveEnd(strings.EqualFold(start, shape.from), strings.EqualFold(start, shape.to),
		strings.EqualFold(shape.from, shape.to))
	if end != assocEndFrom {
		return "", false
	}
	return shape.to, true
}
