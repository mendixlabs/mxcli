// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// ExitCreateModuleRoleStatement handles CREATE MODULE ROLE Module.RoleName [DESCRIPTION '...']
func (b *Builder) ExitCreateModuleRoleStatement(ctx *parser.CreateModuleRoleStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}
	stmt := &ast.CreateModuleRoleStmt{
		Name: buildQualifiedName(qn),
	}
	if ctx.DESCRIPTION() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			stmt.Description = unquoteString(sl.GetText())
		}
	}
	b.statements = append(b.statements, stmt)
}

// ExitDropModuleRoleStatement handles DROP MODULE ROLE Module.RoleName
func (b *Builder) ExitDropModuleRoleStatement(ctx *parser.DropModuleRoleStatementContext) {
	if qn := ctx.QualifiedName(); qn != nil {
		b.statements = append(b.statements, &ast.DropModuleRoleStmt{
			Name: buildQualifiedName(qn),
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
		Name:           identifierOrKeywordText(iok),
		ManageAllRoles: ctx.MANAGE() != nil,
	}

	// Check parent createStatement for OR MODIFY
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && createStmt.MODIFY() != nil {
			stmt.CreateOrModify = true
		}
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, qn := range mrl.AllQualifiedName() {
			stmt.ModuleRoles = append(stmt.ModuleRoles, buildQualifiedName(qn))
		}
	}

	b.statements = append(b.statements, stmt)
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
	if iok := ctx.IdentifierOrKeyword(); iok != nil {
		b.statements = append(b.statements, &ast.DropUserRoleStmt{
			Name: identifierOrKeywordText(iok),
		})
	}
}

// ExitGrantEntityAccessStatement handles GRANT role1, role2 ON Module.Entity (rights) [WHERE '...']
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

	// Parse WHERE clause
	if ctx.WHERE() != nil {
		if sl := ctx.STRING_LITERAL(); sl != nil {
			stmt.XPathConstraint = unquoteString(sl.GetText())
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeEntityAccessStatement handles REVOKE role1, role2 ON Module.Entity [(rights...)]
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

	// Parse optional rights list for partial revoke
	if earl := ctx.EntityAccessRightList(); earl != nil {
		for _, ear := range earl.AllEntityAccessRight() {
			right := parseEntityAccessRight(ear)
			stmt.Rights = append(stmt.Rights, right)
		}
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

// ExitGrantWorkflowAccessStatement handles GRANT EXECUTE ON WORKFLOW Module.WF TO role1, role2
func (b *Builder) ExitGrantWorkflowAccessStatement(ctx *parser.GrantWorkflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.GrantWorkflowAccessStmt{
		Workflow: buildQualifiedName(qn),
	}

	if mrl := ctx.ModuleRoleList(); mrl != nil {
		for _, rqn := range mrl.AllQualifiedName() {
			stmt.Roles = append(stmt.Roles, buildQualifiedName(rqn))
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitRevokeWorkflowAccessStatement handles REVOKE EXECUTE ON WORKFLOW Module.WF FROM role1, role2
func (b *Builder) ExitRevokeWorkflowAccessStatement(ctx *parser.RevokeWorkflowAccessStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.RevokeWorkflowAccessStmt{
		Workflow: buildQualifiedName(qn),
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

// ExitAlterProjectSecurityStatement handles ALTER PROJECT SECURITY commands
func (b *Builder) ExitAlterProjectSecurityStatement(ctx *parser.AlterProjectSecurityStatementContext) {
	stmt := &ast.AlterProjectSecurityStmt{}

	if ctx.LEVEL() != nil {
		if ctx.PRODUCTION() != nil {
			stmt.SecurityLevel = "Production"
		} else if ctx.PROTOTYPE() != nil {
			stmt.SecurityLevel = "Prototype"
		} else if ctx.OFF() != nil {
			stmt.SecurityLevel = "Off"
		}
	} else if ctx.DEMO() != nil {
		enabled := ctx.ON() != nil
		stmt.DemoUsersEnabled = &enabled
	}

	b.statements = append(b.statements, stmt)
}

// ExitCreateDemoUserStatement handles CREATE [OR MODIFY] DEMO USER 'name' PASSWORD 'pw' [ENTITY Module.Entity] (Role1, Role2)
func (b *Builder) ExitCreateDemoUserStatement(ctx *parser.CreateDemoUserStatementContext) {
	sls := ctx.AllSTRING_LITERAL()
	if len(sls) < 2 {
		return
	}

	stmt := &ast.CreateDemoUserStmt{
		UserName: unquoteString(sls[0].GetText()),
		Password: unquoteString(sls[1].GetText()),
	}

	// Check parent createStatement for OR MODIFY
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && createStmt.MODIFY() != nil {
			stmt.CreateOrModify = true
		}
	}

	// Parse optional ENTITY clause
	if qn := ctx.QualifiedName(); qn != nil {
		stmt.Entity = buildQualifiedName(qn).String()
	}

	// Parse user role names from identifierOrKeyword list
	for _, iok := range ctx.AllIdentifierOrKeyword() {
		stmt.UserRoles = append(stmt.UserRoles, identifierOrKeywordText(iok))
	}

	b.statements = append(b.statements, stmt)
}

// ExitDropDemoUserStatement handles DROP DEMO USER 'name'
func (b *Builder) ExitDropDemoUserStatement(ctx *parser.DropDemoUserStatementContext) {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		b.statements = append(b.statements, &ast.DropDemoUserStmt{
			UserName: unquoteString(sl.GetText()),
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
