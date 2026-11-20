// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitAlterSettingsClause handles ALTER SETTINGS ... clauses.
func (b *Builder) ExitAlterSettingsClause(ctx *parser.AlterSettingsClauseContext) {
	stmt := &ast.AlterSettingsStmt{
		Properties: make(map[string]any),
	}

	if ctx.DROP() != nil && ctx.CONSTANT() != nil {
		// ALTER SETTINGS DROP CONSTANT @name [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		stmt.DropConstant = true
		stmt.ConstantId = settingsConstantRefText(ctx.SettingsConstantRef())
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && ctx.STRING_LITERAL() != nil {
			stmt.ConfigName = unquoteStringLit(ctx.STRING_LITERAL())
		}
	} else if ctx.CONSTANT() != nil {
		// ALTER SETTINGS CONSTANT @name (VALUE 'value' | DROP) [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		stmt.ConstantId = settingsConstantRefText(ctx.SettingsConstantRef())
		if ctx.DROP() != nil {
			stmt.DropConstant = true
		} else if ctx.SettingsValue() != nil {
			stmt.Value = settingsValueText(ctx.SettingsValue().(*parser.SettingsValueContext))
		}
		// Check for IN CONFIGURATION 'name'
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && ctx.STRING_LITERAL() != nil {
			stmt.ConfigName = unquoteStringLit(ctx.STRING_LITERAL())
		}
	} else if ctx.CONFIGURATION() != nil {
		// ALTER SETTINGS CONFIGURATION 'name' Key = Value, ...
		stmt.Section = "configuration"
		if lit := ctx.STRING_LITERAL(); lit != nil {
			stmt.ConfigName = unquoteStringLit(lit)
		}
		eachSettingsProperty(ctx.SettingsItemOptions(), ctx.AllSettingsAssignment(), func(key string, sv *parser.SettingsValueContext) {
			stmt.Properties[key] = settingsPropertyValue(sv)
		})
	} else if ctx.SettingsSection() != nil && ctx.GROUP() != nil {
		// ALTER SETTINGS WORKFLOWS ADD [OR MODIFY] GROUP 'Approvers' [( Description: '…' )]
		// ALTER SETTINGS WORKFLOWS MODIFY          GROUP 'Approvers'  ( Description: '…' )
		// ALTER SETTINGS WORKFLOWS DROP            GROUP 'Approvers' (REMOVE: MDL-DEPR092)
		stmt.Section = settingsSectionName(ctx.SettingsSection())
		stmt.UpsertGroup = ctx.ADD() != nil && ctx.OR() != nil && ctx.MODIFY() != nil
		stmt.AddGroup = ctx.ADD() != nil && !stmt.UpsertGroup
		stmt.ModifyGroup = ctx.MODIFY() != nil && !stmt.UpsertGroup
		stmt.RemoveGroup = ctx.DROP() != nil || ctx.REMOVE() != nil
		if lit := ctx.STRING_LITERAL(); lit != nil {
			stmt.GroupName = unquoteStringLit(lit)
		}
		collectSettingsItemOptions(ctx.SettingsItemOptions(), stmt.Properties)
	} else if ctx.SettingsSection() != nil && (ctx.ADD() != nil || ctx.MODIFY() != nil || ctx.DROP() != nil || ctx.REMOVE() != nil) {
		// ALTER SETTINGS LANGUAGE ADD    'ar_SD' [( key: value, … )]
		// ALTER SETTINGS LANGUAGE MODIFY 'ar_SD'  ( key: value, … )
		// ALTER SETTINGS LANGUAGE DROP   'ar_SD' (REMOVE: MDL-DEPR092)
		stmt.Section = settingsSectionName(ctx.SettingsSection())
		stmt.UpsertLanguage = ctx.ADD() != nil && ctx.OR() != nil && ctx.MODIFY() != nil
		stmt.AddLanguage = ctx.ADD() != nil && !stmt.UpsertLanguage
		stmt.ModifyLanguage = ctx.MODIFY() != nil && !stmt.UpsertLanguage
		stmt.RemoveLanguage = ctx.DROP() != nil || ctx.REMOVE() != nil
		if lit := ctx.STRING_LITERAL(); lit != nil {
			stmt.LanguageCode = unquoteStringLit(lit)
		}
		collectSettingsItemOptions(ctx.SettingsItemOptions(), stmt.Properties)
	} else if ctx.SettingsSection() != nil {
		// ALTER SETTINGS RUNTIME|LANGUAGE|WORKFLOWS ( Key: Value, ... )
		stmt.Section = settingsSectionName(ctx.SettingsSection())
		eachSettingsProperty(ctx.SettingsItemOptions(), ctx.AllSettingsAssignment(), func(key string, sv *parser.SettingsValueContext) {
			stmt.Properties[key] = settingsValueToInterface(sv)
		})
	}
	b.recordSettingsAssignments(ctx.AllSettingsAssignment())

	b.statements = append(b.statements, stmt)
}

// ExitCreateConfigurationStatement handles CREATE CONFIGURATION 'name' [Key = Value, ...].
func (b *Builder) ExitCreateConfigurationStatement(ctx *parser.CreateConfigurationStatementContext) {
	stmt := &ast.CreateConfigurationStmt{
		Properties: make(map[string]any),
	}

	if sl := ctx.STRING_LITERAL(); sl != nil {
		stmt.Name = unquoteStringLit(sl)
	}

	// CREATE OR MODIFY / OR REPLACE, read off the shared create wrapper.
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	eachSettingsProperty(ctx.SettingsItemOptions(), ctx.AllSettingsAssignment(), func(key string, sv *parser.SettingsValueContext) {
		stmt.Properties[key] = settingsValueText(sv)
	})
	b.recordSettingsAssignments(ctx.AllSettingsAssignment())

	b.statements = append(b.statements, stmt)
}

// eachSettingsProperty calls f for every property of a settings list, in
// either spelling: the canonical ( Key: value, … ) list, or the old
// `Key = value, …` assignments (MDL-DEPR060). Both build the same statement.
func eachSettingsProperty(opts parser.ISettingsItemOptionsContext, assigns []parser.ISettingsAssignmentContext,
	f func(key string, sv *parser.SettingsValueContext)) {
	if oc, ok := opts.(*parser.SettingsItemOptionsContext); ok && oc != nil {
		for _, o := range oc.AllSettingsItemOption() {
			so, ok := o.(*parser.SettingsItemOptionContext)
			if !ok || so == nil || so.IdentifierOrKeyword() == nil || so.SettingsValue() == nil {
				continue
			}
			if sv, ok := so.SettingsValue().(*parser.SettingsValueContext); ok && sv != nil {
				f(unquoteIdentifier(so.IdentifierOrKeyword().GetText()), sv)
			}
		}
	}
	for _, a := range assigns {
		assign, ok := a.(*parser.SettingsAssignmentContext)
		if !ok || assign == nil || assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
			continue
		}
		if sv, ok := assign.SettingsValue().(*parser.SettingsValueContext); ok && sv != nil {
			f(assign.IDENTIFIER().GetText(), sv)
		}
	}
}

// collectSettingsItemOptions reads a ( key: value, … ) option list — the shared
// form behind ALTER SETTINGS LANGUAGE's language options and WORKFLOWS' group
// options — into the statement's property map.
func collectSettingsItemOptions(opts parser.ISettingsItemOptionsContext, into map[string]any) {
	if opts == nil {
		return
	}
	oc, ok := opts.(*parser.SettingsItemOptionsContext)
	if !ok || oc == nil {
		return
	}
	for _, o := range oc.AllSettingsItemOption() {
		so, ok := o.(*parser.SettingsItemOptionContext)
		if !ok || so == nil || so.IdentifierOrKeyword() == nil || so.SettingsValue() == nil {
			continue
		}
		sv, ok := so.SettingsValue().(*parser.SettingsValueContext)
		if !ok || sv == nil {
			continue
		}
		into[unquoteIdentifier(so.IdentifierOrKeyword().GetText())] = settingsValueText(sv)
	}
}

// settingsPropertyValue is a settings property's value: a []string for a
// parenthesised list (ThemeModuleOrder, ako/mxcli#1286), else the text.
func settingsPropertyValue(ctx *parser.SettingsValueContext) any {
	if ctx.LPAREN() == nil {
		return settingsValueText(ctx)
	}
	items := []string{}
	for _, v := range ctx.AllSettingsValue() {
		if vc, ok := v.(*parser.SettingsValueContext); ok && vc != nil {
			items = append(items, settingsValueText(vc))
		}
	}
	return items
}

// settingsValueText extracts the string value from a SettingsValue context.
func settingsValueText(ctx *parser.SettingsValueContext) string {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteStringLit(sl)
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
	if ctx.LPAREN() != nil {
		return settingsPropertyValue(ctx)
	}
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteStringLit(sl)
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
