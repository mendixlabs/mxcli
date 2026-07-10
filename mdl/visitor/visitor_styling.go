// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// exitAlterStylingStatement handles ALTER STYLING ON PAGE/SNIPPET Module.Name WIDGET name SET/CLEAR ...
func (b *Builder) exitAlterStylingStatement(ctx *parser.AlterStatementContext) {
	stmt := &ast.AlterStylingStmt{}

	// Container type: PAGE or SNIPPET
	if ctx.PAGE() != nil {
		stmt.ContainerType = "PAGE"
	} else if ctx.SNIPPET() != nil {
		stmt.ContainerType = "SNIPPET"
	}

	// Container name
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.ContainerName = buildQualifiedName(qn)
	}

	// Widget name
	if id := ctx.IDENTIFIER(); id != nil {
		stmt.WidgetName = id.GetText()
	}

	// Parse actions (SET assignments and CLEAR DESIGN PROPERTIES)
	for _, actionCtx := range ctx.AllAlterStylingAction() {
		action := actionCtx.(*parser.AlterStylingActionContext)

		if action.CLEAR() != nil {
			// CLEAR DESIGN PROPERTIES
			stmt.ClearDesignProps = true
			continue
		}

		// SET assignments
		for _, assignCtx := range action.AllAlterStylingAssignment() {
			assign := assignCtx.(*parser.AlterStylingAssignmentContext)
			assignment := parseStylingAssignment(assign)
			stmt.Assignments = append(stmt.Assignments, assignment)
		}
	}

	b.statements = append(b.statements, stmt)
}

// parseStylingAssignment parses a single ALTER STYLING assignment.
func parseStylingAssignment(ctx *parser.AlterStylingAssignmentContext) ast.StylingAssignment {
	assignment := ast.StylingAssignment{}

	if ctx.CLASS() != nil {
		// CLASS = 'value' (CSS appearance, not a design property)
		assignment.Property = "Class"
		assignment.IsCSS = true
		if sl := ctx.STRING_LITERAL(0); sl != nil {
			assignment.Value = unquoteString(sl.GetText())
		}
	} else if ctx.STYLE() != nil {
		// STYLE = 'value' (CSS appearance, not a design property)
		assignment.Property = "Style"
		assignment.IsCSS = true
		if sl := ctx.STRING_LITERAL(0); sl != nil {
			assignment.Value = unquoteString(sl.GetText())
		}
	} else {
		// STRING_LITERAL = STRING_LITERAL | ON | OFF (design property; the key
		// may itself be 'Class' or 'Style' — that is a design property, not CSS)
		literals := ctx.AllSTRING_LITERAL()
		if len(literals) > 0 {
			assignment.Property = unquoteString(literals[0].GetText())
		}
		if ctx.ON() != nil {
			assignment.IsToggle = true
			assignment.ToggleOn = true
		} else if ctx.OFF() != nil {
			assignment.IsToggle = true
			assignment.ToggleOn = false
		} else if len(literals) > 1 {
			assignment.Value = unquoteString(literals[1].GetText())
		}
	}

	return assignment
}
