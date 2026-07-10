// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph: semantic validation
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

// ValidateMicroflowBody validates the microflow body for semantic errors without building objects.
// This is used by the check command to validate scripts without executing them.
func ValidateMicroflowBody(s *ast.CreateMicroflowStmt) []string {
	return validateFlowBody(s.Parameters, s.Body)
}

// ValidateNanoflowBody validates the nanoflow body for semantic errors without building objects.
// This is used by the check command to validate scripts without executing them.
func ValidateNanoflowBody(s *ast.CreateNanoflowStmt) []string {
	return validateFlowBody(s.Parameters, s.Body)
}

// validateFlowBody validates parameters and body statements for semantic errors.
func validateFlowBody(params []ast.MicroflowParam, body []ast.MicroflowStatement) []string {
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
		varTypes:     varTypes,
		declaredVars: declaredVars,
		errors:       []string{},
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
		if fb.isVariableDeclared(s.Variable) {
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
			fb.addErrorWithExample(
				fmt.Sprintf("variable '%s' is not declared", s.Target),
				errorExampleDeclareVariable(s.Target))
		}

	case *ast.IfStmt:
		fb.validateScopedStatements(s.ThenBody)
		if len(s.ElseBody) > 0 {
			fb.validateScopedStatements(s.ElseBody)
		}

	case *ast.EnumSplitStmt:
		if count := enumSplitBranchCount(s); count > maxEnumSplitBranches {
			fb.addError("enum split has %d branches; at most %d branches are supported", count, maxEnumSplitBranches)
		}
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

	case *ast.RetrieveStmt:
		fb.validateOutputVariable(s.Variable, "retrieve")
		// Register retrieved variable
		if s.Variable != "" && s.Source.Module != "" {
			if s.StartVariable != "" {
				// Association retrieve always returns a list
				fb.varTypes[s.Variable] = "List of " + s.Source.Module + "." + s.Source.Name
			} else if s.Limit == "1" {
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

		// Other statement types don't declare variables.
	}
}

func (fb *flowBuilder) validateOutputVariable(varName, statement string) {
	if varName == "" {
		return
	}
	if fb.isVariableDeclared(varName) {
		fb.addError("duplicate variable name '$%s' — %s output variable is already declared in this scope (CE0111)", varName, statement)
	}
}
