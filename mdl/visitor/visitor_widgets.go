// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// parseWidgetConditions extracts widget filter conditions from the grammar context.
func parseWidgetConditions(conditions []parser.IWidgetConditionContext) []ast.WidgetFilter {
	filters := make([]ast.WidgetFilter, 0, len(conditions))
	for _, cond := range conditions {
		filter := ast.WidgetFilter{}

		// Get field name
		if cond.WIDGETTYPE() != nil {
			filter.Field = "WidgetType"
		} else if cond.IDENTIFIER() != nil {
			filter.Field = cond.IDENTIFIER().GetText()
		}

		// Get operator
		if cond.LIKE() != nil {
			filter.Operator = "LIKE"
		} else if cond.EQUALS() != nil {
			filter.Operator = "="
		}

		// Get value
		if cond.STRING_LITERAL() != nil {
			filter.Value = unquoteString(cond.STRING_LITERAL().GetText())
		}

		filters = append(filters, filter)
	}
	return filters
}

// ExitUpdateWidgetsStatement handles the UPDATE WIDGETS SET ... WHERE ... statement.
func (b *Builder) ExitUpdateWidgetsStatement(ctx *parser.UpdateWidgetsStatementContext) {
	stmt := &ast.UpdateWidgetsStmt{
		Assignments: make([]ast.WidgetPropertyAssignment, 0),
		Filters:     make([]ast.WidgetFilter, 0),
	}

	// Parse property assignments
	for _, assignCtx := range ctx.AllWidgetPropertyAssignment() {
		assignment := ast.WidgetPropertyAssignment{}

		// Get property path (the first STRING_LITERAL)
		if assignCtx.STRING_LITERAL() != nil {
			assignment.PropertyPath = unquoteString(assignCtx.STRING_LITERAL().GetText())
		}

		// Get value from widgetPropertyValue
		if valCtx := assignCtx.WidgetPropertyValue(); valCtx != nil {
			assignment.Value = parseWidgetPropertyValue(valCtx)
		}

		stmt.Assignments = append(stmt.Assignments, assignment)
	}

	// Parse filter conditions
	stmt.Filters = parseWidgetConditions(ctx.AllWidgetCondition())

	// Parse IN module clause
	if ctx.IN() != nil {
		if qn := ctx.QualifiedName(); qn != nil {
			stmt.InModule = getQualifiedNameText(qn)
		} else if id := ctx.IDENTIFIER(); id != nil {
			stmt.InModule = id.GetText()
		}
	}

	// Check for DRY RUN
	stmt.DryRun = ctx.DRY() != nil && ctx.RUN() != nil

	b.statements = append(b.statements, stmt)
}

// parseWidgetPropertyValue parses a widget property value (string, number, bool, null).
func parseWidgetPropertyValue(ctx parser.IWidgetPropertyValueContext) any {
	if ctx.STRING_LITERAL() != nil {
		return unquoteString(ctx.STRING_LITERAL().GetText())
	}
	if ctx.NUMBER_LITERAL() != nil {
		numStr := ctx.NUMBER_LITERAL().GetText()
		// Try integer first
		if i, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			return i
		}
		// Then try float
		if f, err := strconv.ParseFloat(numStr, 64); err == nil {
			return f
		}
		return numStr // fallback to string
	}
	if ctx.BooleanLiteral() != nil {
		text := strings.ToLower(ctx.BooleanLiteral().GetText())
		return text == "true"
	}
	if ctx.NULL() != nil {
		return nil
	}
	return nil
}
