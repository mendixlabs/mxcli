// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

// SecurityBackend composes all security-related sub-interfaces.
//
// TODO: ADR Decision 2 specifies a different split: EntityAccessBackend,
// DocumentAccessBackend (with DocumentType discriminator), and RoleCRUDBackend.
// Current split (ProjectSecurityBackend, ModuleSecurityBackend, EntityAccessBackend)
// is a pragmatic simplification for the infrastructure phase. Refactor to match
// ADR during handler migration when DocumentType enum is introduced.
type SecurityBackend interface {
	ProjectSecurityBackend
	ModuleSecurityBackend
	EntityAccessBackend
}

// ProjectSecurityBackend manages project-level security settings.
type ProjectSecurityBackend interface {
	GetProjectSecurity() (*security.ProjectSecurity, error)
	SetProjectSecurityLevel(unitID model.ID, level string) error
	SetProjectDemoUsersEnabled(unitID model.ID, enabled bool) error
	AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error
	AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error
	RemoveUserRole(unitID model.ID, name string) error
	AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error
	RemoveDemoUser(unitID model.ID, userName string) error
}

// ModuleSecurityBackend manages module-level security.
type ModuleSecurityBackend interface {
	ListModuleSecurity() ([]*security.ModuleSecurity, error)
	GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error)
	AddModuleRole(unitID model.ID, roleName, description string) error
	RemoveModuleRole(unitID model.ID, roleName string) error
	RemoveModuleRoleFromAllUserRoles(unitID model.ID, qualifiedRole string) (int, error)
}

// EntityAccessRuleParams groups parameters for AddEntityAccessRule to reduce
// positional argument errors.
type EntityAccessRuleParams struct {
	UnitID              model.ID
	EntityName          string
	RoleNames           []string
	AllowCreate         bool
	AllowDelete         bool
	DefaultMemberAccess string
	XPathConstraint     string
	MemberAccesses      []types.EntityMemberAccess
}

// EntityAccessBackend manages entity-level access rules and role assignments.
type EntityAccessBackend interface {
	UpdateAllowedRoles(unitID model.ID, roles []string) error
	UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error
	RemoveFromAllowedRoles(unitID model.ID, roleName string) (bool, error)
	AddEntityAccessRule(params EntityAccessRuleParams) error
	RemoveEntityAccessRule(unitID model.ID, entityName string, roleNames []string) (int, error)
	RevokeEntityMemberAccess(unitID model.ID, entityName string, roleNames []string, revocation types.EntityAccessRevocation) (int, error)
	RemoveRoleFromAllEntities(unitID model.ID, roleName string) (int, error)
	ReconcileMemberAccesses(unitID model.ID, moduleName string) (int, error)
}
