// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/suggest"
)

// ExitCreateModuleRoleStatement handles CREATE [OR MODIFY] MODULE ROLE Module.RoleName [DESCRIPTION '...']
func (b *Builder) ExitCreateModuleRoleStatement(ctx *parser.CreateModuleRoleStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	stmt := &ast.CreateModuleRoleStmt{
		Name: buildQualifiedName(qn),
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		stmt.CreateOrModify = createStmt.OR() != nil && (createStmt.MODIFY() != nil || b.replaceMeansModify(createStmt))
	}
	if ctx.DESCRIPTION() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			// A pointer, so `description ''` (clear it) and no clause at all
			// (keep what is stored) stay different statements (ako/mxcli#1218).
			d := unquoteStringLit(sl)
			stmt.Description = &d
		}
	}
	b.statements = append(b.statements, stmt)
}

// ExitDropModuleRoleStatement handles DROP MODULE ROLE [IF EXISTS] Module.RoleName
func (b *Builder) ExitDropModuleRoleStatement(ctx *parser.DropModuleRoleStatementContext) {
	if qn := ctx.QualifiedName(); qn != nil {
		b.statements = append(b.statements, &ast.DropModuleRoleStmt{
			IfExists: ctx.IfExists() != nil,
			Name:     buildQualifiedName(qn),
		})
	}
}

// ExitCreateUserRoleStatement handles CREATE [OR MODIFY] USER ROLE Name (ModuleRole, ...) [MANAGE ALL ROLES]
func (b *Builder) ExitCreateUserRoleStatement(ctx *parser.CreateUserRoleStatementContext) {
	iok := ctx.IdentifierOrKeyword()
	if iok == nil {
		return
	}

	stmt := &ast.CreateUserRoleStmt{
		Name: identifierOrKeywordText(iok),
	}
	if ctx.MANAGE() != nil {
		stmt.ManageAllRoles, stmt.ManageAllRolesSet = true, true
	}

	// Check parent createStatement for OR MODIFY
	// `or replace` means `or modify` from mdl 1 on (roleReplaceIsModify).
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || b.replaceMeansModify(createStmt)) {
			stmt.CreateOrModify = true
		}
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		// The positional form (MDL-DEPR710).
		for _, qn := range mrl.AllQualifiedName() {
			stmt.ModuleRoles = append(stmt.ModuleRoles, buildQualifiedName(qn))
		}
		b.recordDeprecation(deprecation.UserRolePositional, ctx.LPAREN().GetSymbol(), "")
		b.fixLastDeprecation(deprecation.UserRolePositional, userRolePositionalFix(ctx), "")
	}
	if pl, ok := ctx.UserRolePropertyList().(*parser.UserRolePropertyListContext); ok && pl != nil {
		for _, p := range pl.AllUserRoleProperty() {
			if pc, ok := p.(*parser.UserRolePropertyContext); ok && pc != nil {
				b.userRoleProperty(stmt, pc)
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// userRoleProperty reads one `Key: value` of a user role's property list. The
// list is new syntax, so an unknown key or a value of the wrong shape is an
// error under every language version: there is no older reading to keep.
func (b *Builder) userRoleProperty(stmt *ast.CreateUserRoleStmt, pc *parser.UserRolePropertyContext) {
	key := identifierOrKeywordText(pc.IdentifierOrKeyword())
	line := pc.GetStart().GetLine()
	isList := pc.LPAREN() != nil
	var list []ast.QualifiedName
	for _, qn := range pc.AllQualifiedName() {
		list = append(list, buildQualifiedName(qn))
	}
	var str *string
	if sl := pc.STRING_LITERAL(); sl != nil {
		s := unquoteStringLit(sl)
		str = &s
	}
	var boolean *bool
	if bl := pc.BooleanLiteral(); bl != nil {
		v := strings.EqualFold(bl.GetText(), "true")
		boolean = &v
	}
	wrong := func(takes string) {
		b.addError(fmt.Errorf("line %d: user role property '%s' takes %s", line, key, takes))
	}
	switch strings.ToLower(key) {
	case "moduleroles":
		if !isList {
			wrong("a list of module roles: ModuleRoles: (Module.Role, …)")
			return
		}
		stmt.ModuleRoles = append(stmt.ModuleRoles, list...)
	case "manageableroles":
		if !isList {
			wrong("a list of user roles: ManageableRoles: (UserRole, …)")
			return
		}
		stmt.ManageableSet = true
		stmt.ManageableRoles = []string{}
		for _, qn := range list {
			stmt.ManageableRoles = append(stmt.ManageableRoles, qn.String())
		}
	case "description":
		if str == nil {
			wrong("a string: Description: '…'")
			return
		}
		stmt.Description = str
	case "manageallroles":
		if boolean == nil {
			wrong("true or false")
			return
		}
		stmt.ManageAllRoles, stmt.ManageAllRolesSet = *boolean, true
	case "manageuserswithoutroles":
		if boolean == nil {
			wrong("true or false")
			return
		}
		stmt.ManageUsersWithoutRoles = boolean
	case "checksecurity":
		if boolean == nil {
			wrong("true or false")
			return
		}
		stmt.CheckSecurity = boolean
	default:
		known := []string{"ModuleRoles", "Description", "ManageAllRoles", "ManageableRoles", "ManageUsersWithoutRoles", "CheckSecurity"}
		detail := fmt.Sprintf("line %d: unknown user role property '%s'", line, key)
		if near := suggest.Closest(key, known); near != "" {
			detail += fmt.Sprintf(" — did you mean '%s'?", near)
		}
		b.addError(fmt.Errorf("%s Known properties: %s", detail, strings.Join(known, ", ")))
	}
}

// userRolePositionalFix rewrites `(M.A, M.B) manage all roles` as
// `( ModuleRoles: (M.A, M.B), ManageAllRoles: true )`. The role list's text is
// kept as written.
func userRolePositionalFix(ctx *parser.CreateUserRoleStatementContext) *ast.Fix {
	lp, rp := ctx.LPAREN().GetSymbol(), ctx.RPAREN().GetSymbol()
	props := "ModuleRoles: (" + nodeText(ctx.ModuleRoleList()) + ")"
	last := rp
	if ctx.MANAGE() != nil {
		props += ", ManageAllRoles: true"
		last = ctx.ROLES().GetSymbol()
	}
	return &ast.Fix{Edits: []ast.TextEdit{replaceSpan(lp, last, "( "+props+" )")}}
}

// ExitAlterUserRoleStatement handles ALTER USER ROLE Name ADD/REMOVE MODULE ROLES (...)
func (b *Builder) ExitAlterUserRoleStatement(ctx *parser.AlterUserRoleStatementContext) {
	iok := ctx.IdentifierOrKeyword()
	if iok == nil {
		return
	}

	stmt := &ast.AlterUserRoleStmt{
		Name: identifierOrKeywordText(iok),
		Add:  ctx.ADD() != nil,
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, qn := range mrl.AllQualifiedName() {
			stmt.ModuleRoles = append(stmt.ModuleRoles, buildQualifiedName(qn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitDropUserRoleStatement handles DROP USER ROLE Name
func (b *Builder) ExitDropUserRoleStatement(ctx *parser.DropUserRoleStatementContext) {
	name := ""
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		name = identifierOrKeywordText(iok)
	} else if sl := ctx.STRING_LITERAL(); sl != nil {
		// Quoted form (FINDINGS #5): DROP USER ROLE 'User' — matches DESCRIBE, which
		// also accepts quotes. Bare and quoted are now consistent across both.
		name = unquoteStringLit(sl)
	}
	if name != "" {
		b.statements = append(b.statements, &ast.DropUserRoleStmt{
			Name:     name,
			IfExists: ctx.IfExists() != nil,
		})
	}
}

// ExitGrantEntityAccessStatement handles GRANT rights ON ENTITY Module.Entity TO role1, role2 [WHERE [xpath]],
// and its deprecated alias GRANT role1, role2 ON Module.Entity (rights) [WHERE '...'].
func (b *Builder) ExitGrantEntityAccessStatement(ctx *parser.GrantEntityAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantEntityAccessStmt{
		Entity: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	// Parse access rights
	if earl := ctx.EntityAccessRightList(); earl != nil {
		for _, ear := range earl.AllEntityAccessRight() {
			right := parseEntityAccessRight(ear)
			stmt.Rights = append(stmt.Rights, right)
		}
	}

	// Parse WHERE clause: bracketed groups, verbatim (canonical), or the
	// deprecated quoted string.
	if ctx.WHERE() != nil {
		if groups := ctx.AllXpathConstraint(); len(groups) > 0 {
			stmt.XPathConstraint = bracketedXPathText(groups)
		} else if sl := ctx.STRING_LITERAL(); sl != nil {
			stmt.XPathConstraint = unquoteStringLit(sl)
			b.refuseQuotedXPathStringValues(sl.GetSymbol())
		}
		// A double-quoted name is a name, as in a retrieve's XPath: stored
		// verbatim it is a string literal there, `"Status" = 'Accepted'` is
		// always false, and the rule silently grants no rows
		// (mendixlabs/mxcli#1243).
		stmt.XPathConstraint = stripExpressionIdentifierQuotes(stmt.XPathConstraint)
	}
	if ctx.ENTITY() == nil {
		b.recordReversedEntityGrant(ctx)
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeEntityAccessStatement handles REVOKE rights|ALL ON ENTITY Module.Entity FROM role1, role2,
// and its deprecated alias REVOKE role1, role2 ON Module.Entity [(rights...)].
func (b *Builder) ExitRevokeEntityAccessStatement(ctx *parser.RevokeEntityAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokeEntityAccessStmt{
		Entity: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	// Parse optional rights list for partial revoke; `all` (or, in the old
	// form, no list) is the full revoke.
	if earl := ctx.EntityAccessRightList(); earl != nil {
		for _, ear := range earl.AllEntityAccessRight() {
			right := parseEntityAccessRight(ear)
			stmt.Rights = append(stmt.Rights, right)
		}
	}
	if ctx.ENTITY() == nil {
		b.recordReversedEntityRevoke(ctx)
	}

	b.statements = append(b.statements, stmt)
}

// ExitGrantMicroflowAccessStatement handles GRANT EXECUTE ON MICROFLOW Module.MF TO role1, role2
func (b *Builder) ExitGrantMicroflowAccessStatement(ctx *parser.GrantMicroflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantMicroflowAccessStmt{
		Microflow: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeMicroflowAccessStatement handles REVOKE EXECUTE ON MICROFLOW Module.MF FROM role1, role2
func (b *Builder) ExitRevokeMicroflowAccessStatement(ctx *parser.RevokeMicroflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokeMicroflowAccessStmt{
		Microflow: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitGrantNanoflowAccessStatement handles GRANT EXECUTE ON NANOFLOW Module.NF TO role1, role2
func (b *Builder) ExitGrantNanoflowAccessStatement(ctx *parser.GrantNanoflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantNanoflowAccessStmt{
		Nanoflow: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeNanoflowAccessStatement handles REVOKE EXECUTE ON NANOFLOW Module.NF FROM role1, role2
func (b *Builder) ExitRevokeNanoflowAccessStatement(ctx *parser.RevokeNanoflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokeNanoflowAccessStmt{
		Nanoflow: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitGrantPageAccessStatement handles GRANT VIEW ON PAGE Module.Page TO role1, role2
func (b *Builder) ExitGrantPageAccessStatement(ctx *parser.GrantPageAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantPageAccessStmt{
		Page: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokePageAccessStatement handles REVOKE VIEW ON PAGE Module.Page FROM role1, role2
func (b *Builder) ExitRevokePageAccessStatement(ctx *parser.RevokePageAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokePageAccessStmt{
		Page: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitGrantODataServiceAccessStatement handles GRANT ACCESS ON ODATA SERVICE Module.Svc TO role1, role2
func (b *Builder) ExitGrantODataServiceAccessStatement(ctx *parser.GrantODataServiceAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantODataServiceAccessStmt{
		Service: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeODataServiceAccessStatement handles REVOKE ACCESS ON ODATA SERVICE Module.Svc FROM role1, role2
func (b *Builder) ExitRevokeODataServiceAccessStatement(ctx *parser.RevokeODataServiceAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokeODataServiceAccessStmt{
		Service: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitGrantPublishedRestServiceAccessStatement handles GRANT ACCESS ON PUBLISHED REST SERVICE Module.Svc TO role1, role2
func (b *Builder) ExitGrantPublishedRestServiceAccessStatement(ctx *parser.GrantPublishedRestServiceAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantPublishedRestServiceAccessStmt{
		Service: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokePublishedRestServiceAccessStatement handles REVOKE ACCESS ON PUBLISHED REST SERVICE Module.Svc FROM role1, role2
func (b *Builder) ExitRevokePublishedRestServiceAccessStatement(ctx *parser.RevokePublishedRestServiceAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokePublishedRestServiceAccessStmt{
		Service: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitAlterProjectSecurityStatement handles ALTER APP SECURITY: the property
// list, or one of the clause forms it replaces (MDL-DEPR133, R10).
func (b *Builder) ExitAlterProjectSecurityStatement(ctx *parser.AlterProjectSecurityStatementContext) {
	stmt := &ast.AlterProjectSecurityStmt{}

	if opts := ctx.SettingsItemOptions(); opts != nil {
		b.appSecurityProperties(stmt, opts.(*parser.SettingsItemOptionsContext))
		b.statements = append(b.statements, stmt)
		return
	}

	first := ctx.GetStart() // ALTER; the clause starts after the name rule
	if kw := ctx.AppSecurityKw(); kw != nil {
		first = nextToken(kw.GetStop(), ctx)
	}
	kw := func(word string) string { return keywordLike(first.GetText(), word) }
	onOff := func() string {
		if ctx.ON() != nil {
			return kw("true")
		}
		return kw("false")
	}
	var list string
	if ctx.LEVEL() != nil {
		if ctx.PRODUCTION() != nil {
			stmt.SecurityLevel = "Production"
		} else if ctx.PROTOTYPE() != nil {
			stmt.SecurityLevel = "Prototype"
		} else if ctx.OFF() != nil {
			stmt.SecurityLevel = "Off"
		}
		list = "SecurityLevel: " + kw(strings.ToLower(stmt.SecurityLevel))
	} else if ctx.DEMO() != nil {
		enabled := ctx.ON() != nil
		stmt.DemoUsersEnabled = &enabled
		list = "EnableDemoUsers: " + onOff()
	} else if ctx.GUEST() != nil {
		enabled := ctx.ON() != nil
		stmt.GuestAccessEnabled = &enabled
		list = "EnableGuestAccess: " + onOff()
		if roleCtx := ctx.IdentifierOrKeyword(); roleCtx != nil {
			stmt.GuestUserRole = unquoteIdentifier(roleCtx.GetText())
			list += ", GuestUserRole: " + roleCtx.GetText()
		}
	} else if ctx.STRICT() != nil {
		enabled := ctx.ON() != nil
		stmt.StrictModeEnabled = &enabled
		list = "StrictMode: " + onOff()
	}
	if first != nil && list != "" {
		b.recordDeprecation(deprecation.AppSecurityClause, first, "")
		edit := replaceSpan(first, ctx.GetStop(), "( "+list+" )")
		b.fixLastDeprecation(deprecation.AppSecurityClause, &ast.Fix{Edits: []ast.TextEdit{edit}}, "")
	}

	b.statements = append(b.statements, stmt)
}

// nextToken is the first token of ctx after the token at stop.
func nextToken(stop antlr.Token, ctx antlr.ParserRuleContext) antlr.Token {
	for i := 0; i < ctx.GetChildCount(); i++ {
		if tn, ok := ctx.GetChild(i).(antlr.TerminalNode); ok && tn.GetSymbol().GetTokenIndex() > stop.GetTokenIndex() {
			return tn.GetSymbol()
		}
	}
	return nil
}

// appSecurityKeys are the properties `alter app security ( … )` takes: the
// Security$ProjectSecurity property names.
var appSecurityKeys = []string{"SecurityLevel", "EnableDemoUsers", "EnableGuestAccess", "GuestUserRole", "StrictMode", "AdminUserName", "AdminUserRole", "AdminPassword"}

// appSecurityProperties reads the property list into stmt. A key or value the
// list does not take is an error, never an ignored property.
func (b *Builder) appSecurityProperties(stmt *ast.AlterProjectSecurityStmt, opts *parser.SettingsItemOptionsContext) {
	seen := map[string]bool{}
	for _, o := range opts.AllSettingsItemOption() {
		so, ok := o.(*parser.SettingsItemOptionContext)
		if !ok || so.IdentifierOrKeyword() == nil || so.SettingsValue() == nil {
			continue
		}
		line := so.GetStart().GetLine()
		key := unquoteIdentifier(so.IdentifierOrKeyword().GetText())
		sv := so.SettingsValue().(*parser.SettingsValueContext)
		canonical := ""
		for _, k := range appSecurityKeys {
			if strings.EqualFold(k, key) {
				canonical = k
			}
		}
		if canonical == "" {
			b.addError(fmt.Errorf("line %d: alter app security has no property %q; it takes %s",
				line, key, strings.Join(appSecurityKeys, ", ")))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: alter app security sets %s twice", line, canonical))
			continue
		}
		seen[canonical] = true
		boolValue := func() *bool {
			bl := sv.BooleanLiteral()
			if bl == nil {
				b.addError(fmt.Errorf("line %d: %s takes true or false, not %s", line, canonical, sv.GetText()))
				return nil
			}
			v := strings.EqualFold(bl.GetText(), "true")
			return &v
		}
		switch canonical {
		case "SecurityLevel":
			level := ""
			if sv.QualifiedName() != nil {
				switch strings.ToLower(sv.GetText()) {
				case "production":
					level = "Production"
				case "prototype":
					level = "Prototype"
				case "off":
					level = "Off"
				}
			}
			if level == "" {
				b.addError(fmt.Errorf("line %d: SecurityLevel takes production, prototype or off, not %s", line, sv.GetText()))
				continue
			}
			stmt.SecurityLevel = level
		case "EnableDemoUsers":
			stmt.DemoUsersEnabled = boolValue()
		case "EnableGuestAccess":
			stmt.GuestAccessEnabled = boolValue()
		case "StrictMode":
			stmt.StrictModeEnabled = boolValue()
		case "GuestUserRole":
			if sv.QualifiedName() == nil && sv.STRING_LITERAL() == nil {
				b.addError(fmt.Errorf("line %d: GuestUserRole takes the name of a user role, not %s", line, sv.GetText()))
				continue
			}
			stmt.GuestUserRole = settingsValueText(sv)
		case "AdminUserName":
			if sv.STRING_LITERAL() == nil {
				b.addError(fmt.Errorf("line %d: AdminUserName takes the administrator's user name as a string, not %s", line, sv.GetText()))
				continue
			}
			name := settingsValueText(sv)
			if strings.TrimSpace(name) == "" {
				b.addError(fmt.Errorf("line %d: AdminUserName cannot be empty — the administrator account needs a name", line))
				continue
			}
			stmt.AdminUserName = name
		case "AdminUserRole":
			if sv.QualifiedName() == nil && sv.STRING_LITERAL() == nil {
				b.addError(fmt.Errorf("line %d: AdminUserRole takes the name of a user role, not %s", line, sv.GetText()))
				continue
			}
			role := settingsValueText(sv)
			if strings.TrimSpace(role) == "" {
				// An empty AdminUserRole is CE0130 "No administrator user role
				// selected" (measured on 11.12.6, mendixlabs/mxcli#1775).
				b.addError(fmt.Errorf("line %d: AdminUserRole cannot be empty — Mendix reports CE0130 \"No administrator user role selected\"", line))
				continue
			}
			stmt.AdminUserRole = role
		case "AdminPassword":
			if sv.STRING_LITERAL() == nil {
				b.addError(fmt.Errorf("line %d: AdminPassword takes the administrator's password as a string, not %s", line, sv.GetText()))
				continue
			}
			pw := settingsValueText(sv)
			if pw == "" {
				// Measured on 11.13.0: an empty AdminPassword at prototype or
				// production level is CE0129 "Administrator password has not been
				// set"; at level off it passes. Either way '' is never the fix.
				b.addError(fmt.Errorf("line %d: AdminPassword cannot be empty — an empty administrator password is what CE0129 reports", line))
				continue
			}
			stmt.AdminPassword = pw
		}
	}
}

// ExitCreateDemoUserStatement handles CREATE [OR MODIFY] DEMO USER 'name' ( Password: 'pw', Entity: Module.Entity, UserRoles: (Role1, Role2) ),
// and the clause form `password 'pw' [entity Module.Entity] (Role1, Role2)` it replaces (MDL-DEPR137).
func (b *Builder) ExitCreateDemoUserStatement(ctx *parser.CreateDemoUserStatementContext) {
	sls := ctx.AllSTRING_LITERAL()
	if len(sls) == 0 {
		return
	}

	stmt := &ast.CreateDemoUserStmt{
		UserName: unquoteStringLit(sls[0]),
	}

	// Check parent createStatement for OR MODIFY
	// `or replace` means `or modify` from mdl 1 on (roleReplaceIsModify).
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || b.replaceMeansModify(createStmt)) {
			stmt.CreateOrModify = true
		}
	}

	if pl, ok := ctx.DemoUserPropertyList().(*parser.DemoUserPropertyListContext); ok && pl != nil {
		b.demoUserProperties(stmt, pl)
		b.statements = append(b.statements, stmt)
		return
	}
	if len(sls) < 2 {
		return
	}

	// The clause form (MDL-DEPR137).
	stmt.Password = unquoteStringLit(sls[1])
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Entity = buildQualifiedName(qn).String()
	}
	for _, iok := range ctx.AllIdentifierOrKeyword() {
		stmt.UserRoles = append(stmt.UserRoles, identifierOrKeywordText(iok))
	}
	b.recordDemoUserClauses(ctx)

	b.statements = append(b.statements, stmt)
}

// ExitDropDemoUserStatement handles DROP DEMO USER 'name'
func (b *Builder) ExitDropDemoUserStatement(ctx *parser.DropDemoUserStatementContext) {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		b.statements = append(b.statements, &ast.DropDemoUserStmt{
			UserName: unquoteStringLit(sl),
			IfExists: ctx.IfExists() != nil,
		})
	}
}

// ExitUpdateSecurityStatement handles UPDATE SECURITY [IN Module]
func (b *Builder) ExitUpdateSecurityStatement(ctx *parser.UpdateSecurityStatementContext) {
	stmt := &ast.UpdateSecurityStmt{}
	if qn := ctx.QualifiedName(); qn != nil {
		parsed := buildQualifiedName(qn)
		// Module name is a single identifier, so it goes in Name
		stmt.Module = parsed.Name
		if parsed.Module != "" {
			stmt.Module = parsed.Module
		}
	}
	b.statements = append(b.statements, stmt)
}

// parseEntityAccessRight converts an EntityAccessRightContext to an AST EntityAccessRight.
func parseEntityAccessRight(ctx parser.IEntityAccessRightContext) ast.EntityAccessRight {
	earCtx, ok := ctx.(*parser.EntityAccessRightContext)
	if !ok {
		return ast.EntityAccessRight{}
	}

	if earCtx.CREATE() != nil {
		return ast.EntityAccessRight{Type: ast.EntityAccessCreate}
	}
	if earCtx.DELETE() != nil {
		return ast.EntityAccessRight{Type: ast.EntityAccessDelete}
	}
	if earCtx.READ() != nil {
		if earCtx.STAR() != nil {
			return ast.EntityAccessRight{Type: ast.EntityAccessReadAll}
		}
		right := ast.EntityAccessRight{Type: ast.EntityAccessReadMembers}
		for _, m := range earCtx.AllEntityMemberName() {
			right.Members = append(right.Members, unquoteIdentifier(m.GetText()))
		}
		return right
	}
	if earCtx.WRITE() != nil {
		if earCtx.STAR() != nil {
			return ast.EntityAccessRight{Type: ast.EntityAccessWriteAll}
		}
		right := ast.EntityAccessRight{Type: ast.EntityAccessWriteMembers}
		for _, m := range earCtx.AllEntityMemberName() {
			right.Members = append(right.Members, unquoteIdentifier(m.GetText()))
		}
		return right
	}

	return ast.EntityAccessRight{}
}
