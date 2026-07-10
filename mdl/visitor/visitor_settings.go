// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitAlterSettingsClause handles ALTER SETTINGS ... clauses.
func (b *Builder) ExitAlterSettingsClause(ctx *parser.AlterSettingsClauseContext) {
	stmt := &ast.AlterSettingsStmt{
		Properties: make(map[string]any),
	}

	if ctx.DROP() != nil && ctx.CONSTANT() != nil {
		// ALTER SETTINGS DROP CONSTANT 'name' [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		stmt.DropConstant = true
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConstantId = unquoteString(allStrings[0].GetText())
		}
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && len(allStrings) > 1 {
			stmt.ConfigName = unquoteString(allStrings[1].GetText())
		}
	} else if ctx.CONSTANT() != nil {
		// ALTER SETTINGS CONSTANT 'name' (VALUE 'value' | DROP) [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConstantId = unquoteString(allStrings[0].GetText())
		}
		if ctx.DROP() != nil {
			stmt.DropConstant = true
		} else if ctx.SettingsValue() != nil {
			stmt.Value = settingsValueText(ctx.SettingsValue().(*parser.SettingsValueContext))
		}
		// Check for IN CONFIGURATION 'name'
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && len(allStrings) > 1 {
			stmt.ConfigName = unquoteString(allStrings[1].GetText())
		}
	} else if ctx.CONFIGURATION() != nil {
		// ALTER SETTINGS CONFIGURATION 'name' Key = Value, ...
		stmt.Section = "configuration"
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConfigName = unquoteString(allStrings[0].GetText())
		}
		for _, assignCtx := range ctx.AllSettingsAssignment() {
			assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
			if !ok || assign == nil {
				continue
			}
			if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
				continue
			}
			key := assign.IDENTIFIER().GetText()
			svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
			if !ok || svCtx == nil {
				continue
			}
			val := settingsValueText(svCtx)
			stmt.Properties[key] = val
		}
	} else if ctx.SettingsSection() != nil {
		// ALTER SETTINGS MODEL|LANGUAGE|WORKFLOWS Key = Value, ...
		stmt.Section = ctx.SettingsSection().GetText()
		for _, assignCtx := range ctx.AllSettingsAssignment() {
			assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
			if !ok || assign == nil {
				continue
			}
			if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
				continue
			}
			key := assign.IDENTIFIER().GetText()
			svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
			if !ok || svCtx == nil {
				continue
			}
			val := settingsValueToInterface(svCtx)
			stmt.Properties[key] = val
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitCreateConfigurationStatement handles CREATE CONFIGURATION 'name' [Key = Value, ...].
func (b *Builder) ExitCreateConfigurationStatement(ctx *parser.CreateConfigurationStatementContext) {
	stmt := &ast.CreateConfigurationStmt{
		Properties: make(map[string]any),
	}

	if sl := ctx.STRING_LITERAL(); sl != nil {
		stmt.Name = unquoteString(sl.GetText())
	}

	for _, assignCtx := range ctx.AllSettingsAssignment() {
		assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
		if !ok || assign == nil {
			continue
		}
		if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
			continue
		}
		key := assign.IDENTIFIER().GetText()
		svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
		if !ok || svCtx == nil {
			continue
		}
		stmt.Properties[key] = settingsValueText(svCtx)
	}

	b.statements = append(b.statements, stmt)
}

// settingsValueText extracts the string value from a SettingsValue context.
func settingsValueText(ctx *parser.SettingsValueContext) string {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteString(sl.GetText())
	}
	if nl := ctx.NUMBER_LITERAL(); nl != nil {
		return nl.GetText()
	}
	if bl := ctx.BooleanLiteral(); bl != nil {
		return bl.GetText()
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return ctx.GetText()
}

// settingsValueToInterface extracts a typed value from a SettingsValue context.
func settingsValueToInterface(ctx *parser.SettingsValueContext) any {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteString(sl.GetText())
	}
	if nl := ctx.NUMBER_LITERAL(); nl != nil {
		if v, err := strconv.ParseInt(nl.GetText(), 10, 64); err == nil {
			return v
		}
		return nl.GetText()
	}
	if bl := ctx.BooleanLiteral(); bl != nil {
		text := bl.GetText()
		return text == "true" || text == "TRUE" || text == "True"
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return ctx.GetText()
}
