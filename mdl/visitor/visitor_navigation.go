// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"github.com/antlr4-go/antlr/v4"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/types"
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
			IsPage:     ctx.PAGE() != nil,
			IsNanoflow: ctx.NANOFLOW() != nil,
			Target:     buildQualifiedName(names[0]),
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
	} else if ch, ok := ctx.NavMenuChildren().(*parser.NavMenuChildrenContext); ok && ch != nil {
		// { navMenuItemDef* } — the profile's menu items as its children (R2)
		stmt.HasMenuBlock = true
		for _, itemCtx := range ch.AllNavMenuItemDef() {
			item := b.buildNavMenuItemDef(itemCtx)
			stmt.MenuItems = append(stmt.MenuItems, item)
		}
	} else if ctx.MENU_KW() != nil {
		// MENU (navMenuItemDef*) — the old spelling (MDL-DEPR121)
		stmt.HasMenuBlock = true
		for _, itemCtx := range ctx.AllNavMenuItemDef() {
			item := b.buildNavMenuItemDef(itemCtx)
			stmt.MenuItems = append(stmt.MenuItems, item)
		}
	} else if ctx.PROGRESSIVE() != nil {
		// PROGRESSIVE WEB APP [( Precaching: b, InstallPrompt: b ) | OFF]
		stmt.ProgressiveWebApp = b.buildNavPWASpec(ctx)
	} else if ctx.ON() != nil && ctx.SYNC() != nil && ctx.ERROR() != nil {
		// ON SYNC ERROR THROW|CONTINUE. Checked before the bare SYNC block
		// because both alternatives carry a SYNC token.
		throw := ctx.THROW() != nil
		stmt.ThrowSyncError = &throw
	} else if ctx.SYNC() != nil {
		// SYNC (navSyncDef*)
		stmt.HasSyncBlock = true
		for _, defCtx := range ctx.AllNavSyncDef() {
			stmt.SyncEntries = append(stmt.SyncEntries, buildNavSyncDef(defCtx))
		}
	}
}

// syncModeFor maps an MDL mode word onto the stored Navigation$SyncMode member.
//
// The words are deliberately NOT Studio Pro's captions: "All Objects" and "By
// XPath" are captions of All and Constrained and are not members of the
// enumeration at all. Writing a caption where a key belongs is what made every
// mxcli-authored gallery fail with CE0463 (mendixlabs/mxcli#1035), so every
// value on the right of this table is asserted to be a declared member by
// TestNavSyncModesAreDeclaredEnumMembers.
func buildNavSyncDef(ctx parser.INavSyncDefContext) ast.NavSyncDef {
	c := ctx.(*parser.NavSyncDefContext)

	def := ast.NavSyncDef{}
	if qn := c.QualifiedName(); qn != nil {
		def.Entity = buildQualifiedName(qn)
	}

	m := c.NavSyncMode()
	if m == nil {
		return def
	}
	mc := m.(*parser.NavSyncModeContext)
	switch {
	case mc.ONLINE() != nil:
		def.Mode = "Online"
	case mc.ALL() != nil:
		def.Mode = "All"
	case mc.NEVER() != nil:
		def.Mode = "Never"
	case mc.NONE() != nil && mc.PRESERVE() != nil:
		def.Mode = "NoneAndPreserveData"
	case mc.NONE() != nil:
		def.Mode = "None"
	case mc.WHERE() != nil:
		// WHERE implies Constrained: the mode and the constraint come from one
		// alternative so they cannot disagree.
		def.Mode = "Constrained"
		if xc := mc.XpathConstraint(); xc != nil {
			// First-class form. The source text is stored bracketed, exactly as
			// a RETRIEVE's multi-predicate WHERE does: a string in it stores its
			// value in Mendix's spelling (storedExpressionSource, #825).
			xcCtx := xc.(*parser.XpathConstraintContext)
			if xe := xcCtx.XpathExpr(); xe != nil {
				if prc, ok := xe.(antlr.ParserRuleContext); ok {
					if src := strings.TrimSpace(extractExpressionText(prc)); src != "" {
						src = storedExpressionSource(src, lexedWithStrictEscapes(prc))
						def.Constraint = normalizeXPathTokens("[" + src + "]")
					}
				}
			}
		} else if lit := mc.STRING_LITERAL(); lit != nil {
			// Legacy quoted form: the '' pairs are MDL escaping and come off here.
			def.Constraint = unquoteStringLit(lit)
		}
	}
	return def
}

// buildNavMenuItemDef recursively builds a NavMenuItemDef from the parse context.
//
// An item is read the same whichever spelling wrote it: the canonical
// `menu item 'X' ( OnClick: show page M.P, Icon: … )` with sub-items in { },
// or the old clauses `menu item 'X' page M.P icon …;` with sub-items in ( )
// (R2, ako/mxcli#754). The two build the same item.
func (b *Builder) buildNavMenuItemDef(ctx parser.INavMenuItemDefContext) ast.NavMenuItemDef {
	c := ctx.(*parser.NavMenuItemDefContext)

	caption := ""
	if sl := c.STRING_LITERAL(); sl != nil {
		caption = unquoteStringLit(sl)
	}

	item := ast.NavMenuItemDef{Caption: caption}

	// Old spelling: the PAGE/MICROFLOW target is the item's only qualifiedName
	// now that the icon is its own sub-rule.
	if qn := c.QualifiedName(); qn != nil {
		switch {
		case c.PAGE() != nil:
			built := buildQualifiedName(qn)
			item.Page = &built
			item.Action = &ast.ActionV3{Type: "showPage", Target: built.String()}
		case c.MICROFLOW() != nil:
			built := buildQualifiedName(qn)
			item.Microflow = &built
			item.Action = &ast.ActionV3{Type: "microflow", Target: built.String()}
		}
	}
	// SIGN_OUT names no target, which is why it is read separately rather than
	// as a third switch arm.
	if c.SIGN_OUT() != nil {
		item.SignOut = true
		item.Action = &ast.ActionV3{Type: "signOut"}
	}
	if ic, ok := c.NavMenuIcon().(*parser.NavMenuIconContext); ok && ic != nil {
		applyNavMenuIcon(&item, ic.NavMenuIconValue())
	}

	// Canonical spelling: OnClick and Icon in the item's property list.
	// The list is new syntax, so it is strict: a key written twice is an error
	// rather than a silent first- or last-wins, and a sub-menu — which opens
	// its items rather than acting — takes no OnClick (the old spelling had no
	// way to write one, and describe never prints one).
	if pc, ok := c.NavMenuItemProps().(*parser.NavMenuItemPropsContext); ok && pc != nil {
		seen := map[string]bool{}
		for _, p := range pc.AllNavMenuItemProp() {
			prop := p.(*parser.NavMenuItemPropContext)
			key := prop.GetStart().GetText()
			line := prop.GetStart().GetLine()
			if seen[strings.ToLower(key)] {
				b.addError(fmt.Errorf("line %d: menu item '%s': %s is written twice", line, caption, key))
				continue
			}
			seen[strings.ToLower(key)] = true
			switch {
			case prop.ONCLICK() != nil:
				if c.NavMenuChildren() != nil {
					b.addError(fmt.Errorf("line %d: menu '%s': a sub-menu has no OnClick — it opens its items; "+
						"give the action to one of them", line, caption))
					continue
				}
				b.applyNavMenuAction(&item, prop.NavMenuAction(), line)
			case prop.ICON() != nil:
				applyNavMenuIcon(&item, prop.NavMenuIconValue())
			}
		}
	}

	// Sub-items: in { } (canonical) or directly in ( ) (old spelling).
	subs := c.AllNavMenuItemDef()
	if ch, ok := c.NavMenuChildren().(*parser.NavMenuChildrenContext); ok && ch != nil {
		subs = ch.AllNavMenuItemDef()
	}
	for _, subCtx := range subs {
		item.Items = append(item.Items, b.buildNavMenuItemDef(subCtx))
	}

	return item
}

// menuItemActionKinds are the actions Studio Pro offers on a menu item: show
// page, call microflow, call nanoflow, open link, create object, sign out, and
// nothing. Save, cancel, delete, close page and complete task act on a page's
// object, which a menu item has none of.
var menuItemActionKinds = map[string]bool{
	"showPage": true, "microflow": true, "nanoflow": true, "openLink": true,
	"create": true, "signOut": true, "none": true,
}

// applyNavMenuAction reads a menu item's `OnClick:` action — the widget action
// expression (ako/mxcli#980). The simple targets are also recorded on Page /
// Microflow / SignOut, which the reference checks read.
func (b *Builder) applyNavMenuAction(item *ast.NavMenuItemDef, ctx parser.INavMenuActionContext, line int) {
	a, ok := ctx.(*parser.NavMenuActionContext)
	if !ok || a == nil || a.ActionExprV3() == nil {
		return
	}
	act := buildActionV3(a.ActionExprV3())
	if act == nil {
		return
	}
	if !menuItemActionKinds[act.Type] {
		b.addError(fmt.Errorf("line %d: menu item '%s': a menu item cannot %s — its OnClick is show page, "+
			"call microflow, call nanoflow, open link, create object, sign out or nothing", line, item.Caption, menuActionWords(act)))
		return
	}
	if act.Type == "openLink" && act.LinkVariable != "" {
		b.addError(fmt.Errorf("line %d: menu item '%s': open link %s/%s reads its address from an object, and a menu item has none — "+
			"write the address as a string", line, item.Caption, act.LinkVariable, act.LinkAttribute))
		return
	}
	item.Action = act
	qn := buildQualifiedName(a.ActionExprV3().(*parser.ActionExprV3Context).QualifiedName())
	switch act.Type {
	case "signOut":
		item.SignOut = true
	case "showPage":
		item.Page = &qn
	case "microflow":
		item.Microflow = &qn
	}
}

// menuActionWords names a refused action in the words the script used.
func menuActionWords(a *ast.ActionV3) string {
	switch a.Type {
	case "save":
		return "save changes"
	case "cancel":
		return "cancel changes"
	case "close":
		return "close a page"
	case "delete":
		return "delete"
	case "completeTask":
		return "complete a task"
	case "param":
		return "take a fragment action parameter ($" + a.Target + ")"
	}
	return a.Type
}

// applyNavMenuIcon reads the ICON clause onto the item.
//
// Mendix stores three different icon ELEMENTS, not three spellings of one
// value: a collection icon and an image icon each hold a qualified name (into an
// icon collection and an image collection — different documents), while a glyph
// icon holds a numeric character code and no name at all. The kind is recorded
// so the writer emits the right $Type; collapsing them onto one string is what
// made a rewrite turn a glyph into nothing.
//
// The bare form is the collection icon, which keeps every existing script
// meaning exactly what it did.
func applyNavMenuIcon(item *ast.NavMenuItemDef, ctx parser.INavMenuIconValueContext) {
	c, ok := ctx.(*parser.NavMenuIconValueContext)
	if !ok || c == nil {
		return
	}
	switch {
	case c.GLYPH() != nil:
		item.IconKind = types.MenuIconGlyph
		if n := c.NUMBER_LITERAL(); n != nil {
			// A glyph code is a character code: whole, and small. A fractional or
			// unparseable literal leaves the code at zero rather than guessing,
			// and the writer refuses to emit a glyph without one.
			if v, err := strconv.Atoi(n.GetText()); err == nil {
				item.IconCode = v
			}
		}
	case c.IMAGE() != nil:
		item.IconKind = types.MenuIconImage
		if qn := c.QualifiedName(); qn != nil {
			item.Icon = buildQualifiedName(qn).String()
		}
	default:
		item.IconKind = types.MenuIconCollection
		if qn := c.QualifiedName(); qn != nil {
			item.Icon = buildQualifiedName(qn).String()
		}
	}
}

// navPWAExample is the syntax an error about the clause shows.
const navPWAExample = `create or modify navigation PhoneOffline
  home page Module.Home
  progressive web app ( Precaching: true, InstallPrompt: true );
-- progressive web app off      stores no settings again`

// buildNavPWASpec reads the PROGRESSIVE WEB APP clause. The keys are the stored
// property names of Navigation$ProgressiveWebAppSettings, each a boolean; any
// other key, or a value that is not true or false, is an error (R11).
func (b *Builder) buildNavPWASpec(ctx *parser.NavigationClauseContext) *types.NavPWASpec {
	spec := &types.NavPWASpec{}
	if ctx.OFF() != nil {
		spec.Off = true
		return spec
	}
	eachSettingsProperty(ctx.SettingsItemOptions(), nil, func(key string, sv *parser.SettingsValueContext) {
		bl := sv.BooleanLiteral()
		if bl == nil {
			b.addErrorWithExample(fmt.Sprintf("progressive web app: %s takes true or false, not %s", key, sv.GetText()), navPWAExample)
			return
		}
		v := strings.EqualFold(bl.GetText(), "true")
		switch {
		case strings.EqualFold(key, "Precaching"):
			spec.Precaching = &v
		case strings.EqualFold(key, "InstallPrompt"):
			spec.InstallPrompt = &v
		default:
			b.addErrorWithExample(fmt.Sprintf("progressive web app: unknown property %s -- the settings are Precaching and InstallPrompt", key), navPWAExample)
		}
	})
	return spec
}
