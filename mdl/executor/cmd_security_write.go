// SPDX-License-Identifier: Apache-2.0

// Package executor - Security write commands (CREATE/DROP/ALTER/GRANT/REVOKE)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// execCreateModuleRole handles CREATE MODULE ROLE Module.RoleName [DESCRIPTION '...'].
func execCreateModuleRole(ctx *ExecContext, s *ast.CreateModuleRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", s.Name.Module), err)
	}

	// Check if role already exists
	for _, mr := range ms.ModuleRoles {
		if mr.Name == s.Name.Name {
			return mdlerrors.NewAlreadyExists("module role", s.Name.Module+"."+s.Name.Name)
		}
	}

	if err := ctx.Backend.AddModuleRole(ms.ID, s.Name.Name, s.Description); err != nil {
		return mdlerrors.NewBackend("create module role", err)
	}

	fmt.Fprintf(ctx.Output, "Created module role: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// execDropModuleRole handles DROP MODULE ROLE Module.RoleName.
// Cascade-removes the role from all entity access rules, microflow/nanoflow/page
// allowed roles, and OData service allowed roles before deleting the role itself.
func execDropModuleRole(ctx *ExecContext, s *ast.DropModuleRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", s.Name.Module), err)
	}

	// Check role exists
	found := false
	for _, mr := range ms.ModuleRoles {
		if mr.Name == s.Name.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("module role", s.Name.Module+"."+s.Name.Name)
	}

	qualifiedRole := s.Name.Module + "." + s.Name.Name

	// Cascade: remove role from entity access rules
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err == nil {
		if n, err := ctx.Backend.RemoveRoleFromAllEntities(dm.ID, qualifiedRole); err != nil {
			return mdlerrors.NewBackend("cascade-remove entity access rules", err)
		} else if n > 0 {
			fmt.Fprintf(ctx.Output, "Removed %s from %d entity access rule(s)\n", qualifiedRole, n)
		}
	}

	// Cascade: remove role from microflow/nanoflow/page allowed roles
	h, err := getHierarchy(ctx)
	if err == nil {
		// Microflows
		if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
			for _, mf := range mfs {
				modID := h.FindModuleID(mf.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(mf.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from microflow %s allowed roles\n", qualifiedRole, mf.Name)
				}
			}
		}

		// Nanoflows
		if nfs, err := ctx.Backend.ListNanoflows(); err == nil {
			for _, nf := range nfs {
				modID := h.FindModuleID(nf.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(nf.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from nanoflow %s allowed roles\n", qualifiedRole, nf.Name)
				}
			}
		}

		// Pages
		if pgs, err := ctx.Backend.ListPages(); err == nil {
			for _, pg := range pgs {
				modID := h.FindModuleID(pg.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(pg.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from page %s allowed roles\n", qualifiedRole, pg.Name)
				}
			}
		}

		// OData services
		if svcs, err := ctx.Backend.ListPublishedODataServices(); err == nil {
			for _, svc := range svcs {
				modID := h.FindModuleID(svc.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(svc.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from OData service %s allowed roles\n", qualifiedRole, svc.Name)
				}
			}
		}
	}

	// Cascade: remove role from user roles in ProjectSecurity
	if ps, err := ctx.Backend.GetProjectSecurity(); err == nil {
		if n, err := ctx.Backend.RemoveModuleRoleFromAllUserRoles(ps.ID, qualifiedRole); err == nil && n > 0 {
			fmt.Fprintf(ctx.Output, "Removed %s from %d user role(s)\n", qualifiedRole, n)
		}
	}

	// Finally, remove the role itself
	if err := ctx.Backend.RemoveModuleRole(ms.ID, s.Name.Name); err != nil {
		return mdlerrors.NewBackend("drop module role", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped module role: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// execCreateUserRole handles CREATE [OR MODIFY] USER ROLE Name (ModuleRoles) [MANAGE ALL ROLES].
func execCreateUserRole(ctx *ExecContext, s *ast.CreateUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Build qualified module role names
	var moduleRoleNames []string
	for _, mr := range s.ModuleRoles {
		qn := mr.Module + "." + mr.Name
		moduleRoleNames = append(moduleRoleNames, qn)
	}

	// Check if role already exists
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExists("user role", s.Name)
			}
			// Additive: ensure specified module roles are present
			if err := ctx.Backend.AlterUserRoleModuleRoles(ps.ID, s.Name, true, moduleRoleNames); err != nil {
				return mdlerrors.NewBackend("update user role", err)
			}
			fmt.Fprintf(ctx.Output, "Modified user role: %s\n", s.Name)
			return nil
		}
	}

	if err := ctx.Backend.AddUserRole(ps.ID, s.Name, moduleRoleNames, s.ManageAllRoles); err != nil {
		return mdlerrors.NewBackend("create user role", err)
	}

	fmt.Fprintf(ctx.Output, "Created user role: %s\n", s.Name)
	return nil
}

// execAlterUserRole handles ALTER USER ROLE Name ADD/REMOVE MODULE ROLES (...).
func execAlterUserRole(ctx *ExecContext, s *ast.AlterUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check user role exists
	found := false
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("user role", s.Name)
	}

	// Build qualified module role names
	var moduleRoleNames []string
	for _, mr := range s.ModuleRoles {
		moduleRoleNames = append(moduleRoleNames, mr.Module+"."+mr.Name)
	}

	if err := ctx.Backend.AlterUserRoleModuleRoles(ps.ID, s.Name, s.Add, moduleRoleNames); err != nil {
		return mdlerrors.NewBackend("alter user role", err)
	}

	action := "Added"
	prep := "to"
	if !s.Add {
		action = "Removed"
		prep = "from"
	}
	fmt.Fprintf(ctx.Output, "%s module roles %s %s user role %s\n", action, strings.Join(moduleRoleNames, ", "), prep, s.Name)
	return nil
}

// execDropUserRole handles DROP USER ROLE Name.
func execDropUserRole(ctx *ExecContext, s *ast.DropUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check user role exists
	found := false
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("user role", s.Name)
	}

	if err := ctx.Backend.RemoveUserRole(ps.ID, s.Name); err != nil {
		return mdlerrors.NewBackend("drop user role", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped user role: %s\n", s.Name)
	return nil
}

// execGrantEntityAccess handles GRANT roles ON Module.Entity (rights) [WHERE '...'].
func execGrantEntityAccess(ctx *ExecContext, s *ast.GrantEntityAccessStmt) error {
	e := ctx.executor
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Entity.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Verify entity exists
	entity := dm.FindEntityByName(s.Entity.Name)
	if entity == nil {
		return mdlerrors.NewNotFound("entity", s.Entity.Module+"."+s.Entity.Name)
	}

	// Build role name list
	var roleNames []string
	for _, role := range s.Roles {
		roleNames = append(roleNames, role.Module+"."+role.Name)
	}

	// Parse access rights from the statement.
	// Note: Mendix has no AllowRead/AllowWrite properties on AccessRule.
	// Read/write access is determined by DefaultMemberAccessRights and MemberAccesses.
	allowCreate, allowDelete := false, false
	defaultMemberAccess := "None"
	var readMembers, writeMembers []string // nil = all (wildcard)
	for _, right := range s.Rights {
		switch right.Type {
		case ast.EntityAccessCreate:
			allowCreate = true
		case ast.EntityAccessDelete:
			allowDelete = true
		case ast.EntityAccessReadAll:
			if defaultMemberAccess == "None" {
				defaultMemberAccess = "ReadOnly"
			}
		case ast.EntityAccessReadMembers:
			readMembers = right.Members
		case ast.EntityAccessWriteAll:
			defaultMemberAccess = "ReadWrite"
		case ast.EntityAccessWriteMembers:
			writeMembers = right.Members
		}
	}

	// Build MemberAccess entries for all entity attributes and associations.
	// Mendix requires explicit MemberAccess entries for every member — an empty
	// MemberAccesses array triggers CE0066 "Entity access is out of date".
	var memberAccesses []types.EntityMemberAccess

	// Build sets for specific member overrides (when READ (Name, Email) syntax is used)
	writeMemberSet := make(map[string]bool)
	for _, m := range writeMembers {
		writeMemberSet[m] = true
	}
	readMemberSet := make(map[string]bool)
	for _, m := range readMembers {
		readMemberSet[m] = true
	}

	// Create entries for all entity attributes
	for _, attr := range entity.Attributes {
		rights := defaultMemberAccess
		if writeMemberSet[attr.Name] {
			rights = "ReadWrite"
		} else if readMemberSet[attr.Name] {
			rights = "ReadOnly"
		}
		// Calculated attributes cannot have write rights (CE6592)
		isCalculated := attr.Value != nil && attr.Value.Type == "CalculatedValue"
		if isCalculated && (rights == "ReadWrite" || rights == "WriteOnly") {
			rights = "ReadOnly"
		}
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AttributeRef: module.Name + "." + s.Entity.Name + "." + attr.Name,
			AccessRights: rights,
		})
	}

	// Create entries for associations where this entity is the FROM entity.
	// In Mendix, ParentID = FROM entity (FK owner). MemberAccess for associations
	// is only required on the FROM side; adding it to the TO side triggers CE0066.
	for _, assoc := range dm.Associations {
		if assoc.ParentID == entity.ID {
			rights := defaultMemberAccess
			if writeMemberSet[assoc.Name] {
				rights = "ReadWrite"
			} else if readMemberSet[assoc.Name] {
				rights = "ReadOnly"
			}
			memberAccesses = append(memberAccesses, types.EntityMemberAccess{
				AssociationRef: module.Name + "." + assoc.Name,
				AccessRights:   rights,
			})
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.ParentID == entity.ID {
			rights := defaultMemberAccess
			if writeMemberSet[ca.Name] {
				rights = "ReadWrite"
			} else if readMemberSet[ca.Name] {
				rights = "ReadOnly"
			}
			memberAccesses = append(memberAccesses, types.EntityMemberAccess{
				AssociationRef: module.Name + "." + ca.Name,
				AccessRights:   rights,
			})
		}
	}

	// Add MemberAccess entries for system associations (owner, changedBy).
	// When an entity has HasOwner/HasChangedBy, Mendix implicitly adds
	// System.owner/System.changedBy associations that require MemberAccess.
	if entity.HasOwner {
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AssociationRef: "System.owner",
			AccessRights:   defaultMemberAccess,
		})
	}
	if entity.HasChangedBy {
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AssociationRef: "System.changedBy",
			AccessRights:   defaultMemberAccess,
		})
	}

	if err := ctx.Backend.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID:              dm.ID,
		EntityName:          s.Entity.Name,
		RoleNames:           roleNames,
		AllowCreate:         allowCreate,
		AllowDelete:         allowDelete,
		DefaultMemberAccess: defaultMemberAccess,
		XPathConstraint:     s.XPathConstraint,
		MemberAccesses:      memberAccesses,
	}); err != nil {
		return mdlerrors.NewBackend("grant entity access", err)
	}

	// Reconcile MemberAccesses on pre-existing rules for this entity's domain model
	if count, err := ctx.Backend.ReconcileMemberAccesses(dm.ID, module.Name); err != nil {
		return mdlerrors.NewBackend("reconcile member accesses", err)
	} else if count > 0 && !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) in module %s\n", count, module.Name)
	}

	e.trackModifiedDomainModel(module.ID, module.Name)
	fmt.Fprintf(ctx.Output, "Granted access on %s.%s to %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
	if !ctx.Quiet {
		fmt.Fprint(ctx.Output, formatAccessRuleResult(ctx, s.Entity.Module, s.Entity.Name, roleNames))
	}
	return nil
}

// execRevokeEntityAccess handles REVOKE roles ON Module.Entity [(rights...)].
func execRevokeEntityAccess(ctx *ExecContext, s *ast.RevokeEntityAccessStmt) error {
	e := ctx.executor
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Entity.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Verify entity exists
	entity := dm.FindEntityByName(s.Entity.Name)
	if entity == nil {
		return mdlerrors.NewNotFound("entity", s.Entity.Module+"."+s.Entity.Name)
	}

	// Build role name list
	var roleNames []string
	for _, role := range s.Roles {
		roleNames = append(roleNames, role.Module+"."+role.Name)
	}

	if len(s.Rights) > 0 {
		// Partial revoke — downgrade specific rights
		revocation := types.EntityAccessRevocation{}
		for _, right := range s.Rights {
			switch right.Type {
			case ast.EntityAccessCreate:
				revocation.RevokeCreate = true
			case ast.EntityAccessDelete:
				revocation.RevokeDelete = true
			case ast.EntityAccessReadAll:
				revocation.RevokeReadAll = true
			case ast.EntityAccessWriteAll:
				revocation.RevokeWriteAll = true
			case ast.EntityAccessReadMembers:
				for _, m := range right.Members {
					revocation.RevokeReadMembers = append(revocation.RevokeReadMembers,
						module.Name+"."+s.Entity.Name+"."+m)
				}
			case ast.EntityAccessWriteMembers:
				for _, m := range right.Members {
					revocation.RevokeWriteMembers = append(revocation.RevokeWriteMembers,
						module.Name+"."+s.Entity.Name+"."+m)
				}
			}
		}

		modified, err := ctx.Backend.RevokeEntityMemberAccess(dm.ID, s.Entity.Name, roleNames, revocation)
		if err != nil {
			return mdlerrors.NewBackend("revoke entity access", err)
		}

		if modified == 0 {
			fmt.Fprintf(ctx.Output, "No access rules found matching %s on %s.%s\n", strings.Join(roleNames, ", "), s.Entity.Module, s.Entity.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked partial access on %s.%s from %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
			if !ctx.Quiet {
				fmt.Fprint(ctx.Output, formatAccessRuleResult(ctx, s.Entity.Module, s.Entity.Name, roleNames))
			}
		}
	} else {
		// Full revoke — remove entire access rule
		modified, err := ctx.Backend.RemoveEntityAccessRule(dm.ID, s.Entity.Name, roleNames)
		if err != nil {
			return mdlerrors.NewBackend("revoke entity access", err)
		}

		if modified == 0 {
			fmt.Fprintf(ctx.Output, "No access rules found matching %s on %s.%s\n", strings.Join(roleNames, ", "), s.Entity.Module, s.Entity.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on %s.%s from %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
			if !ctx.Quiet {
				fmt.Fprint(ctx.Output, "  Result: (no access)\n")
			}
		}
	}
	e.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// execGrantMicroflowAccess handles GRANT EXECUTE ON MICROFLOW Module.MF TO roles.
func execGrantMicroflowAccess(ctx *ExecContext, s *ast.GrantMicroflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Microflow.Module || mf.Name != s.Microflow.Name {
			continue
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range mf.AllowedModuleRoles {
			existing[string(r)] = true
			merged = append(merged, string(r))
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(mf.ID, merged); err != nil {
			return mdlerrors.NewBackend("update microflow access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have execute access on %s.%s\n", modName, mf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted execute access on %s.%s to %s\n", modName, mf.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("microflow", s.Microflow.Module+"."+s.Microflow.Name)
}

// execRevokeMicroflowAccess handles REVOKE EXECUTE ON MICROFLOW Module.MF FROM roles.
func execRevokeMicroflowAccess(ctx *ExecContext, s *ast.RevokeMicroflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Microflow.Module || mf.Name != s.Microflow.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range mf.AllowedModuleRoles {
			if toRemove[string(r)] {
				removed = append(removed, string(r))
			} else {
				remaining = append(remaining, string(r))
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(mf.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update microflow access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had execute access on %s.%s\n", modName, mf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked execute access on %s.%s from %s\n", modName, mf.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("microflow", s.Microflow.Module+"."+s.Microflow.Name)
}

// execGrantPageAccess handles GRANT VIEW ON PAGE Module.Page TO roles.
func execGrantPageAccess(ctx *ExecContext, s *ast.GrantPageAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the page
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	for _, pg := range pages {
		modID := h.FindModuleID(pg.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Page.Module || pg.Name != s.Page.Name {
			continue
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range pg.AllowedRoles {
			existing[string(r)] = true
			merged = append(merged, string(r))
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(pg.ID, merged); err != nil {
			return mdlerrors.NewBackend("update page access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have view access on %s.%s\n", modName, pg.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted view access on %s.%s to %s\n", modName, pg.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("page", s.Page.Module+"."+s.Page.Name)
}

// execRevokePageAccess handles REVOKE VIEW ON PAGE Module.Page FROM roles.
func execRevokePageAccess(ctx *ExecContext, s *ast.RevokePageAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the page
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	for _, pg := range pages {
		modID := h.FindModuleID(pg.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Page.Module || pg.Name != s.Page.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range pg.AllowedRoles {
			if toRemove[string(r)] {
				removed = append(removed, string(r))
			} else {
				remaining = append(remaining, string(r))
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(pg.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update page access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had view access on %s.%s\n", modName, pg.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked view access on %s.%s from %s\n", modName, pg.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("page", s.Page.Module+"."+s.Page.Name)
}

// execGrantWorkflowAccess handles GRANT EXECUTE ON WORKFLOW Module.WF TO roles.
// Mendix workflows do not have a document-level AllowedModuleRoles field (unlike
// microflows and pages), so this operation is not supported.
func execGrantWorkflowAccess(ctx *ExecContext, s *ast.GrantWorkflowAccessStmt) error {
	return mdlerrors.NewUnsupported("GRANT EXECUTE ON WORKFLOW is not supported: Mendix workflows do not have document-level AllowedModuleRoles (unlike microflows and pages). Workflow access is controlled through the microflow that triggers the workflow and UserTask targeting")
}

// execRevokeWorkflowAccess handles REVOKE EXECUTE ON WORKFLOW Module.WF FROM roles.
// Mendix workflows do not have a document-level AllowedModuleRoles field (unlike
// microflows and pages), so this operation is not supported.
func execRevokeWorkflowAccess(ctx *ExecContext, s *ast.RevokeWorkflowAccessStmt) error {
	return mdlerrors.NewUnsupported("REVOKE EXECUTE ON WORKFLOW is not supported: Mendix workflows do not have document-level AllowedModuleRoles (unlike microflows and pages). Workflow access is controlled through the microflow that triggers the workflow and UserTask targeting")
}

// validateModuleRole checks that a module role exists in the project.
func validateModuleRole(ctx *ExecContext, role ast.QualifiedName) error {
	module, err := findModule(ctx, role.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("module not found for role %s.%s", role.Module, role.Name), err)
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", role.Module), err)
	}

	for _, mr := range ms.ModuleRoles {
		if mr.Name == role.Name {
			return nil
		}
	}

	return mdlerrors.NewNotFound("module role", role.Module+"."+role.Name)
}

// execAlterProjectSecurity handles ALTER PROJECT SECURITY LEVEL/DEMO USERS.
func execAlterProjectSecurity(ctx *ExecContext, s *ast.AlterProjectSecurityStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	if s.SecurityLevel != "" {
		// Map from display name to BSON value
		var bsonLevel string
		switch s.SecurityLevel {
		case "Production":
			bsonLevel = security.SecurityLevelProduction
		case "Prototype":
			bsonLevel = security.SecurityLevelPrototype
		case "Off":
			bsonLevel = security.SecurityLevelOff
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown security level: %s", s.SecurityLevel))
		}

		if err := ctx.Backend.SetProjectSecurityLevel(ps.ID, bsonLevel); err != nil {
			return mdlerrors.NewBackend("set security level", err)
		}
		fmt.Fprintf(ctx.Output, "Set project security level to %s\n", s.SecurityLevel)
	}

	if s.DemoUsersEnabled != nil {
		if err := ctx.Backend.SetProjectDemoUsersEnabled(ps.ID, *s.DemoUsersEnabled); err != nil {
			return mdlerrors.NewBackend("set demo users", err)
		}
		state := "disabled"
		if *s.DemoUsersEnabled {
			state = "enabled"
		}
		fmt.Fprintf(ctx.Output, "Demo users %s\n", state)
	}

	return nil
}

// execCreateDemoUser handles CREATE [OR MODIFY] DEMO USER 'name' PASSWORD 'pw' [ENTITY Module.Entity] (Roles).
func execCreateDemoUser(ctx *ExecContext, s *ast.CreateDemoUserStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Validate password against project password policy
	if err := ps.PasswordPolicy.ValidatePassword(s.Password); err != nil {
		return mdlerrors.NewValidationf("password policy violation for demo user '%s': %v\nhint: check your project's password policy with SHOW PROJECT SECURITY", s.UserName, err)
	}

	// Check if user already exists
	for _, du := range ps.DemoUsers {
		if du.UserName == s.UserName {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExists("demo user", s.UserName)
			}
			// Additive: merge roles, update password. Drop and re-create with merged roles.
			mergedRoles := du.UserRoles
			existingSet := make(map[string]bool)
			for _, r := range mergedRoles {
				existingSet[r] = true
			}
			for _, r := range s.UserRoles {
				if !existingSet[r] {
					mergedRoles = append(mergedRoles, r)
				}
			}
			entity := du.Entity
			if s.Entity != "" {
				entity = s.Entity
			}
			if err := ctx.Backend.RemoveDemoUser(ps.ID, s.UserName); err != nil {
				return mdlerrors.NewBackend("update demo user", err)
			}
			if err := ctx.Backend.AddDemoUser(ps.ID, s.UserName, s.Password, entity, mergedRoles); err != nil {
				return mdlerrors.NewBackend("update demo user", err)
			}
			fmt.Fprintf(ctx.Output, "Modified demo user: %s\n", s.UserName)
			return nil
		}
	}

	// Resolve entity: use explicit value or auto-detect from domain models
	entity := s.Entity
	if entity == "" {
		detected, err := detectUserEntity(ctx)
		if err != nil {
			return err
		}
		entity = detected
	}

	if err := ctx.Backend.AddDemoUser(ps.ID, s.UserName, s.Password, entity, s.UserRoles); err != nil {
		return mdlerrors.NewBackend("create demo user", err)
	}

	fmt.Fprintf(ctx.Output, "Created demo user: %s (entity: %s)\n", s.UserName, entity)
	return nil
}

// detectUserEntity finds the entity that generalizes System.User.
func detectUserEntity(ctx *ExecContext) (string, error) {
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return "", mdlerrors.NewBackend("list modules", err)
	}
	moduleNameByID := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNameByID[m.ID] = m.Name
	}

	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return "", mdlerrors.NewBackend("list domain models", err)
	}

	var candidates []string
	for _, dm := range dms {
		moduleName := moduleNameByID[dm.ContainerID]
		for _, ent := range dm.Entities {
			if ent.GeneralizationRef == "System.User" {
				candidates = append(candidates, moduleName+"."+ent.Name)
			}
		}
	}

	switch len(candidates) {
	case 0:
		return "", mdlerrors.NewValidation("no entity found that generalizes System.User; use ENTITY clause to specify one")
	case 1:
		return candidates[0], nil
	default:
		return "", mdlerrors.NewValidationf("multiple entities generalize System.User: %s; use ENTITY clause to specify one", joinCandidates(candidates))
	}
}

func joinCandidates(candidates []string) string {
	result := candidates[0]
	for i := 1; i < len(candidates); i++ {
		result += ", " + candidates[i]
	}
	return result
}

// execDropDemoUser handles DROP DEMO USER 'name'.
func execDropDemoUser(ctx *ExecContext, s *ast.DropDemoUserStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check if user exists
	found := false
	for _, du := range ps.DemoUsers {
		if du.UserName == s.UserName {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("demo user", s.UserName)
	}

	if err := ctx.Backend.RemoveDemoUser(ps.ID, s.UserName); err != nil {
		return mdlerrors.NewBackend("drop demo user", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped demo user: %s\n", s.UserName)
	return nil
}

// ============================================================================
// GRANT/REVOKE ACCESS ON ODATA SERVICE
// ============================================================================

// execGrantODataServiceAccess handles GRANT ACCESS ON ODATA SERVICE Module.Svc TO roles.
func execGrantODataServiceAccess(ctx *ExecContext, s *ast.GrantODataServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the published OData service
	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range svc.AllowedModuleRoles {
			existing[r] = true
			merged = append(merged, r)
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(svc.ID, merged); err != nil {
			return mdlerrors.NewBackend("update OData service access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have access on OData service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted access on OData service %s.%s to %s\n", modName, svc.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published OData service", s.Service.Module+"."+s.Service.Name)
}

// execRevokeODataServiceAccess handles REVOKE ACCESS ON ODATA SERVICE Module.Svc FROM roles.
func execRevokeODataServiceAccess(ctx *ExecContext, s *ast.RevokeODataServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the published OData service
	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range svc.AllowedModuleRoles {
			if toRemove[r] {
				removed = append(removed, r)
			} else {
				remaining = append(remaining, r)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(svc.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update OData service access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had access on OData service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on OData service %s.%s from %s\n", modName, svc.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published OData service", s.Service.Module+"."+s.Service.Name)
}

// ============================================================================
// GRANT/REVOKE ACCESS ON PUBLISHED REST SERVICE
// ============================================================================

// execGrantPublishedRestServiceAccess handles GRANT ACCESS ON PUBLISHED REST SERVICE Module.Svc TO roles.
func execGrantPublishedRestServiceAccess(ctx *ExecContext, s *ast.GrantPublishedRestServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkFeature(ctx, "integration", "published_rest_grant_revoke",
		"GRANT ACCESS ON PUBLISHED REST SERVICE",
		"upgrade your project to 10.0+"); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published REST services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range svc.AllowedRoles {
			existing[r] = true
			merged = append(merged, r)
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdatePublishedRestServiceRoles(svc.ID, merged); err != nil {
			return mdlerrors.NewBackend("update published REST service access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have access on published REST service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted access on published REST service %s.%s to %s\n", modName, svc.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published REST service", s.Service.Module+"."+s.Service.Name)
}

// execRevokePublishedRestServiceAccess handles REVOKE ACCESS ON PUBLISHED REST SERVICE Module.Svc FROM roles.
func execRevokePublishedRestServiceAccess(ctx *ExecContext, s *ast.RevokePublishedRestServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published REST services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range svc.AllowedRoles {
			if toRemove[r] {
				removed = append(removed, r)
			} else {
				remaining = append(remaining, r)
			}
		}

		if err := ctx.Backend.UpdatePublishedRestServiceRoles(svc.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update published REST service access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had access on published REST service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on published REST service %s.%s from %s\n", modName, svc.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published REST service", s.Service.Module+"."+s.Service.Name)
}

// execUpdateSecurity handles UPDATE SECURITY [IN Module].
func execUpdateSecurity(ctx *ExecContext, s *ast.UpdateSecurityStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return err
	}

	totalModified := 0
	for _, mod := range modules {
		if s.Module != "" && mod.Name != s.Module {
			continue
		}

		dm, err := ctx.Backend.GetDomainModel(mod.ID)
		if err != nil {
			continue // module may not have a domain model
		}

		count, err := ctx.Backend.ReconcileMemberAccesses(dm.ID, mod.Name)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("reconcile security for module %s", mod.Name), err)
		}
		if count > 0 {
			fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) in module %s\n", count, mod.Name)
			totalModified += count
		}
	}

	if totalModified == 0 {
		fmt.Fprintf(ctx.Output, "All entity access rules are up to date\n")
	}

	return nil
}

// Executor method wrappers — delegate to free functions for callers that
// still use the Executor receiver (e.g. executor_query.go).
