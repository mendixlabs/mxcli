// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitAlterModuleJarDepStatement handles:
//
//	ALTER MODULE name ADD JAR DEPENDENCY (...)
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' VERSION '...'
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' INCLUDED true|false
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' ADD EXCLUSION '...'
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' DROP EXCLUSION '...'
//	ALTER MODULE name DROP JAR DEPENDENCY 'coord'
func (b *Builder) ExitAlterModuleJarDepStatement(ctx *parser.AlterModuleJarDepStatementContext) {
	var moduleName string
	if qn := ctx.QualifiedName(); qn != nil {
		moduleName = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		moduleName = id.GetText()
	}
	if moduleName == "" {
		return
	}

	stmt := &ast.AlterModuleJarDepStmt{ModuleName: moduleName}

	for _, rawAction := range ctx.AllAlterModuleJarDepAction() {
		actionCtx := rawAction.(*parser.AlterModuleJarDepActionContext)
		action := parseJarDepAction(actionCtx)
		if action != nil {
			stmt.Actions = append(stmt.Actions, action)
		}
	}

	if len(stmt.Actions) > 0 {
		b.statements = append(b.statements, stmt)
	}
}

func parseJarDepAction(ctx *parser.AlterModuleJarDepActionContext) ast.JarDepAction {
	if ctx.ADD() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil && ctx.LPAREN() != nil {
		// ADD JAR DEPENDENCY (group = '...', artifact = '...', ...)
		action := &ast.AddJarDepAction{Included: true} // default included = true
		for _, rawProp := range ctx.AllJarDepProperty() {
			propCtx := rawProp.(*parser.JarDepPropertyContext)
			key := strings.ToLower(identifierOrKeywordText(propCtx.IdentifierOrKeyword()))
			var val string
			var boolVal bool
			isBool := false
			if sl := propCtx.STRING_LITERAL(); sl != nil {
				val = unquoteString(sl.GetText())
			} else if bl := propCtx.BooleanLiteral(); bl != nil {
				boolVal = strings.EqualFold(bl.GetText(), "true")
				isBool = true
			}
			switch key {
			case "group":
				action.Group = val
			case "artifact":
				action.Artifact = val
			case "version":
				action.Version = val
			case "included":
				if isBool {
					action.Included = boolVal
				}
			}
		}
		return action
	}

	if ctx.SET() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil {
		// The coordinate is STRING_LITERAL(0)
		strs := ctx.AllSTRING_LITERAL()
		if len(strs) == 0 {
			return nil
		}
		coordinate := unquoteString(strs[0].GetText())

		if ctx.VERSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' VERSION 'version'
			return &ast.SetJarDepVersionAction{
				Coordinate: coordinate,
				Version:    unquoteString(strs[1].GetText()),
			}
		}

		if ctx.INCLUDED() != nil && ctx.BooleanLiteral() != nil {
			// SET JAR DEPENDENCY 'coord' INCLUDED true|false
			included := strings.EqualFold(ctx.BooleanLiteral().GetText(), "true")
			return &ast.SetJarDepIncludedAction{
				Coordinate: coordinate,
				Included:   included,
			}
		}

		if ctx.ADD() != nil && ctx.EXCLUSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' ADD EXCLUSION 'excl'
			return &ast.AddJarDepExclusionAction{
				Coordinate: coordinate,
				Exclusion:  unquoteString(strs[1].GetText()),
			}
		}

		if ctx.DROP() != nil && ctx.EXCLUSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' DROP EXCLUSION 'excl'
			return &ast.DropJarDepExclusionAction{
				Coordinate: coordinate,
				Exclusion:  unquoteString(strs[1].GetText()),
			}
		}

		return nil
	}

	if ctx.DROP() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil {
		// DROP JAR DEPENDENCY 'coord'
		strs := ctx.AllSTRING_LITERAL()
		if len(strs) == 0 {
			return nil
		}
		return &ast.DropJarDepAction{Coordinate: unquoteString(strs[0].GetText())}
	}

	return nil
}
