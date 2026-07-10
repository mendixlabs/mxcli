// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// buildPermissions extracts security permissions from all documents.
// This is only run in full mode as it requires parsing all documents.
func (b *Builder) buildPermissions() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, MemberName, AccessType, XPathConstraint, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID := b.catalog.projectID
	snapshotID := b.snapshot.ID
	permCount := 0

	permCount += b.buildEntityPermissions(stmt, projectID, snapshotID)
	permCount += b.buildMicroflowPermissions(stmt, projectID, snapshotID)
	permCount += b.buildPagePermissions(stmt, projectID, snapshotID)
	permCount += b.buildODataServicePermissions(stmt, projectID, snapshotID)

	b.report("Permissions", permCount)
	return nil
}

// buildEntityPermissions extracts entity-level and member-level access permissions.
func (b *Builder) buildEntityPermissions(stmt *sql.Stmt, projectID, snapshotID string) int {
	count := 0

	dms, err := b.reader.ListDomainModels()
	if err != nil {
		return 0
	}

	for _, dm := range dms {
		moduleID := b.hierarchy.findModuleID(dm.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		for _, ent := range dm.Entities {
			entityQN := moduleName + "." + ent.Name

			for _, rule := range ent.AccessRules {
				// Each access rule can apply to multiple module roles
				roleNames := rule.ModuleRoleNames
				if len(roleNames) == 0 {
					continue
				}

				xpath := rule.XPathConstraint

				// Derive entity-level READ/WRITE from member access configuration.
				// In Mendix, read/write is controlled via DefaultMemberAccessRights
				// and MemberAccesses, not by AllowRead/AllowWrite flags.
				hasRead, hasWrite := entityAccessFromMemberRights(rule)

				for _, roleName := range roleNames {
					// Entity-level permissions
					if rule.AllowCreate {
						stmt.Exec(roleName, "ENTITY", entityQN, nil, "CREATE", xpath, moduleName, projectID, snapshotID)
						count++
					}
					if hasRead {
						stmt.Exec(roleName, "ENTITY", entityQN, nil, "READ", xpath, moduleName, projectID, snapshotID)
						count++
					}
					if hasWrite {
						stmt.Exec(roleName, "ENTITY", entityQN, nil, "WRITE", xpath, moduleName, projectID, snapshotID)
						count++
					}
					if rule.AllowDelete {
						stmt.Exec(roleName, "ENTITY", entityQN, nil, "DELETE", xpath, moduleName, projectID, snapshotID)
						count++
					}

					// Member-level permissions
					count += b.emitMemberPermissions(stmt, rule, ent, roleName, entityQN, xpath, moduleName, projectID, snapshotID)
				}
			}
		}
	}

	return count
}

// entityAccessFromMemberRights derives entity-level READ/WRITE from an access rule's
// member access configuration. In Mendix, entity-level read/write is not controlled
// by separate boolean flags but is implied by the member access rights:
//   - If any member has read access, the entity is readable
//   - If any member has write access, the entity is writable
func entityAccessFromMemberRights(rule *domainmodel.AccessRule) (hasRead, hasWrite bool) {
	if len(rule.MemberAccesses) > 0 {
		for _, ma := range rule.MemberAccesses {
			if ma.AccessRights == domainmodel.MemberAccessRightsReadOnly || ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				hasRead = true
			}
			if ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				hasWrite = true
			}
		}
	} else {
		dmr := rule.DefaultMemberAccessRights
		if dmr == domainmodel.MemberAccessRightsReadOnly || dmr == domainmodel.MemberAccessRightsReadWrite {
			hasRead = true
		}
		if dmr == domainmodel.MemberAccessRightsReadWrite {
			hasWrite = true
		}
	}
	return
}

// emitMemberPermissions emits MEMBER_READ/MEMBER_WRITE rows for entity attributes and associations.
// When MemberAccesses is non-empty, use explicit per-member rights.
// When MemberAccesses is empty, expand DefaultMemberAccessRights to all attributes.
func (b *Builder) emitMemberPermissions(stmt *sql.Stmt, rule *domainmodel.AccessRule, ent *domainmodel.Entity,
	roleName, entityQN, xpath, moduleName, projectID, snapshotID string) int {
	count := 0

	if len(rule.MemberAccesses) > 0 {
		// Explicit member-level access
		for _, ma := range rule.MemberAccesses {
			memberName := ma.AttributeName
			if memberName == "" {
				memberName = ma.AssociationName
			}
			if memberName == "" {
				continue
			}

			if ma.AccessRights == domainmodel.MemberAccessRightsReadOnly || ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				stmt.Exec(roleName, "ENTITY", entityQN, memberName, "MEMBER_READ", xpath, moduleName, projectID, snapshotID)
				count++
			}
			if ma.AccessRights == domainmodel.MemberAccessRightsReadWrite {
				stmt.Exec(roleName, "ENTITY", entityQN, memberName, "MEMBER_WRITE", xpath, moduleName, projectID, snapshotID)
				count++
			}
		}
	} else if rule.DefaultMemberAccessRights != "" && rule.DefaultMemberAccessRights != domainmodel.MemberAccessRightsNone {
		// Expand default to all attributes
		for _, attr := range ent.Attributes {
			if rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadOnly || rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadWrite {
				stmt.Exec(roleName, "ENTITY", entityQN, attr.Name, "MEMBER_READ", xpath, moduleName, projectID, snapshotID)
				count++
			}
			if rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadWrite {
				stmt.Exec(roleName, "ENTITY", entityQN, attr.Name, "MEMBER_WRITE", xpath, moduleName, projectID, snapshotID)
				count++
			}
		}
	}

	return count
}

// buildMicroflowPermissions extracts microflow execution permissions.
func (b *Builder) buildMicroflowPermissions(stmt *sql.Stmt, projectID, snapshotID string) int {
	count := 0

	mfs, err := b.reader.ListMicroflows()
	if err != nil {
		return 0
	}

	for _, mf := range mfs {
		if len(mf.AllowedModuleRoles) == 0 {
			continue
		}

		moduleID := b.hierarchy.findModuleID(mf.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		mfQN := moduleName + "." + mf.Name

		for _, roleID := range mf.AllowedModuleRoles {
			// AllowedModuleRoles are BY_NAME strings stored as model.ID
			roleName := string(roleID)
			stmt.Exec(roleName, "MICROFLOW", mfQN, nil, "EXECUTE", nil, moduleName, projectID, snapshotID)
			count++
		}
	}

	return count
}

// buildPagePermissions extracts page view permissions.
func (b *Builder) buildPagePermissions(stmt *sql.Stmt, projectID, snapshotID string) int {
	count := 0

	pages, err := b.reader.ListPages()
	if err != nil {
		return 0
	}

	for _, pg := range pages {
		if len(pg.AllowedRoles) == 0 {
			continue
		}

		moduleID := b.hierarchy.findModuleID(pg.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		pgQN := moduleName + "." + pg.Name

		for _, roleID := range pg.AllowedRoles {
			// AllowedRoles are BY_NAME strings stored as model.ID
			roleName := string(roleID)
			stmt.Exec(roleName, "PAGE", pgQN, nil, "VIEW", nil, moduleName, projectID, snapshotID)
			count++
		}
	}

	return count
}

// buildODataServicePermissions extracts published OData service access permissions.
func (b *Builder) buildODataServicePermissions(stmt *sql.Stmt, projectID, snapshotID string) int {
	count := 0

	services, err := b.reader.ListPublishedODataServices()
	if err != nil {
		return 0
	}

	for _, svc := range services {
		if len(svc.AllowedModuleRoles) == 0 {
			continue
		}

		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		svcQN := moduleName + "." + svc.Name

		for _, roleName := range svc.AllowedModuleRoles {
			stmt.Exec(roleName, "ODATA_SERVICE", svcQN, nil, "ACCESS", nil, moduleName, projectID, snapshotID)
			count++
		}
	}

	return count
}
