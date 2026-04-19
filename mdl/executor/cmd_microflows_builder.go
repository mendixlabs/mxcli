// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph builder: core types and helpers
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowBuilder helps construct the flow graph from AST statements.
type flowBuilder struct {
	objects             []microflows.MicroflowObject
	flows               []*microflows.SequenceFlow
	annotationFlows     []*microflows.AnnotationFlow
	posX                int
	posY                int
	baseY               int // Base Y position (for returning after ELSE branches)
	spacing             int
	returnValue         string                       // Return value expression for RETURN statement (used by buildFlowGraph final EndEvent)
	endsWithReturn      bool                         // True if the flow already ends with EndEvent(s) from RETURN statements
	varTypes            map[string]string            // Variable name -> entity qualified name (for CHANGE statements)
	declaredVars        map[string]string            // Declared primitive variables: name -> type (e.g., "$IsValid" -> "Boolean")
	errors              []string                     // Validation errors collected during build
	measurer            *layoutMeasurer              // For measuring statement dimensions
	nextConnectionPoint model.ID                     // For compound statements: the exit point differs from entry point
	nextFlowCase        string                       // If set, next connecting flow uses this case value (for merge-less splits)
	reader              backend.FullBackend              // For looking up page/microflow references
	hierarchy           *ContainerHierarchy          // For resolving container IDs to module names
	pendingAnnotations  *ast.ActivityAnnotations     // Pending annotations to attach to next activity
	restServices        []*model.ConsumedRestService // Cached REST services for parameter classification
}

// addError records a validation error during flow building.
func (fb *flowBuilder) addError(format string, args ...any) {
	fb.errors = append(fb.errors, fmt.Sprintf(format, args...))
}

// addErrorWithExample records a validation error with a code example showing the fix.
func (fb *flowBuilder) addErrorWithExample(message, example string) {
	fb.errors = append(fb.errors, fmt.Sprintf("%s\n\n  Example:\n%s", message, example))
}

// GetErrors returns all validation errors collected during build.
func (fb *flowBuilder) GetErrors() []string {
	return fb.errors
}

// errorExampleDeclareVariable returns an example for declaring a variable.
func errorExampleDeclareVariable(varName string) string {
	// Remove $ prefix if present for cleaner display
	cleanName := varName
	if len(varName) > 0 && varName[0] == '$' {
		cleanName = varName[1:]
	}
	return fmt.Sprintf(`    DECLARE $%s Boolean = true;  -- or String, Integer, Decimal, DateTime
    ...
    SET $%s = false;`, cleanName, cleanName)
}

// isVariableDeclared checks if a variable has been declared (either as primitive or entity).
func (fb *flowBuilder) isVariableDeclared(varName string) bool {
	// Check entity variables (from parameters with entity types)
	if _, ok := fb.varTypes[varName]; ok {
		return true
	}
	// Check primitive variables (from DECLARE statements or primitive parameters)
	if _, ok := fb.declaredVars[varName]; ok {
		return true
	}
	return false
}

// exprToString converts an AST Expression to a Mendix expression string,
// resolving association navigation paths to include the target entity qualifier.
// e.g. $Order/MyModule.Order_Customer/Name → $Order/MyModule.Order_Customer/MyModule.Customer/Name
func (fb *flowBuilder) exprToString(expr ast.Expression) string {
	resolved := fb.resolveAssociationPaths(expr)
	return expressionToString(resolved)
}

// resolveAssociationPaths walks an expression tree and, for any AttributePathExpr
// whose path contains an association (qualified name like Module.AssocName), inserts
// the association's target entity after the association segment.
func (fb *flowBuilder) resolveAssociationPaths(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.AttributePathExpr:
		resolved := fb.resolvePathSegments(e.Path)
		return &ast.AttributePathExpr{
			Variable: e.Variable,
			Path:     resolved,
			Segments: e.Segments,
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			Left:     fb.resolveAssociationPaths(e.Left),
			Operator: e.Operator,
			Right:    fb.resolveAssociationPaths(e.Right),
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{
			Operator: e.Operator,
			Operand:  fb.resolveAssociationPaths(e.Operand),
		}
	case *ast.FunctionCallExpr:
		args := make([]ast.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			args[i] = fb.resolveAssociationPaths(arg)
		}
		return &ast.FunctionCallExpr{
			Name:      e.Name,
			Arguments: args,
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Inner: fb.resolveAssociationPaths(e.Inner)}
	case *ast.IfThenElseExpr:
		return &ast.IfThenElseExpr{
			Condition: fb.resolveAssociationPaths(e.Condition),
			ThenExpr:  fb.resolveAssociationPaths(e.ThenExpr),
			ElseExpr:  fb.resolveAssociationPaths(e.ElseExpr),
		}
	default:
		return expr
	}
}

// resolvePathSegments processes path segments in an attribute path expression.
// For each segment that is a qualified association name (Module.AssocName), it looks up
// the association's target entity and inserts it after the association.
func (fb *flowBuilder) resolvePathSegments(path []string) []string {
	if fb.reader == nil || len(path) == 0 {
		return path
	}

	var resolved []string
	for i, segment := range path {
		resolved = append(resolved, segment)

		// A qualified name (contains ".") that isn't the last segment might be an association
		if !strings.Contains(segment, ".") {
			continue
		}
		// If the next segment is already a qualified name, the target entity is already present
		if i+1 < len(path) && strings.Contains(path[i+1], ".") {
			continue
		}
		// If this is the last segment, nothing to insert after
		if i == len(path)-1 {
			continue
		}

		// Look up association target entity
		parts := strings.SplitN(segment, ".", 2)
		if len(parts) != 2 {
			continue
		}
		result := fb.lookupAssociation(parts[0], parts[1])
		if result != nil && result.childEntityQN != "" {
			resolved = append(resolved, result.childEntityQN)
		}
	}
	return resolved
}
