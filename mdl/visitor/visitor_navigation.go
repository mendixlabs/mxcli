// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateNavigationStatement handles CREATE [OR REPLACE] NAVIGATION <profile> <clauses>.
func (b *Builder) ExitCreateNavigationStatement(ctx *parser.CreateNavigationStatementContext) {
	// Extract profile name from qualifiedName or IDENTIFIER
	profileName := ""
	if qn := ctx.QualifiedName(); qn != nil {
		profileName = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		profileName = id.GetText()
	}
	if profileName == "" {
		return
	}

	stmt := &ast.AlterNavigationStmt{
		ProfileName: profileName,
	}

	// Process each navigation clause
	for _, clauseCtx := range ctx.AllNavigationClause() {
		clause := clauseCtx.(*parser.NavigationClauseContext)
		b.processNavigationClause(stmt, clause)
	}

	// Check for CREATE OR REPLACE/MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}

// processNavigationClause processes a single navigation clause.
func (b *Builder) processNavigationClause(stmt *ast.AlterNavigationStmt, ctx *parser.NavigationClauseContext) {
	if ctx.HOME() != nil {
		// HOME PAGE/MICROFLOW qualifiedName [FOR qualifiedName]
		names := ctx.AllQualifiedName()
		if len(names) == 0 {
			return
		}
		hp := ast.NavHomePageDef{
			IsPage: ctx.PAGE() != nil,
			Target: buildQualifiedName(names[0]),
		}
		if ctx.FOR() != nil && len(names) >= 2 {
			forRole := buildQualifiedName(names[1])
			hp.ForRole = &forRole
		}
		stmt.HomePages = append(stmt.HomePages, hp)
	} else if ctx.LOGIN() != nil {
		// LOGIN PAGE qualifiedName
		names := ctx.AllQualifiedName()
		if len(names) > 0 {
			qn := buildQualifiedName(names[0])
			stmt.LoginPage = &qn
		}
	} else if ctx.NOT() != nil && ctx.FOUND() != nil {
		// NOT FOUND PAGE qualifiedName
		names := ctx.AllQualifiedName()
		if len(names) > 0 {
			qn := buildQualifiedName(names[0])
			stmt.NotFoundPage = &qn
		}
	} else if ctx.MENU_KW() != nil {
		// MENU (navMenuItemDef*)
		stmt.HasMenuBlock = true
		for _, itemCtx := range ctx.AllNavMenuItemDef() {
			item := buildNavMenuItemDef(itemCtx)
			stmt.MenuItems = append(stmt.MenuItems, item)
		}
	}
}

// buildNavMenuItemDef recursively builds a NavMenuItemDef from the parse context.
func buildNavMenuItemDef(ctx parser.INavMenuItemDefContext) ast.NavMenuItemDef {
	c := ctx.(*parser.NavMenuItemDefContext)

	caption := ""
	if sl := c.STRING_LITERAL(); sl != nil {
		caption = unquoteString(sl.GetText())
	}

	item := ast.NavMenuItemDef{
		Caption: caption,
	}

	// The grammar uses a single qualifiedName for either PAGE or MICROFLOW target
	qn := c.QualifiedName()

	if c.PAGE() != nil && qn != nil {
		built := buildQualifiedName(qn)
		item.Page = &built
	} else if c.MICROFLOW() != nil && qn != nil {
		built := buildQualifiedName(qn)
		item.Microflow = &built
	}

	// Recurse into sub-items (for MENU 'caption' (...))
	for _, subCtx := range c.AllNavMenuItemDef() {
		subItem := buildNavMenuItemDef(subCtx)
		item.Items = append(item.Items, subItem)
	}

	return item
}
