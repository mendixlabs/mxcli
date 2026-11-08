// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
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
	// SetProjectStrictMode toggles security strict mode, which mxcli has always
	// READ (it is what lint rule SEC005 reports on) and could not write, so the
	// rule had no remedy short of Studio Pro (ako/mxcli#526).
	SetProjectStrictMode(unitID model.ID, enabled bool) error
	// SetProjectAdminUserName renames the built-in administrator account
	// (Security$ProjectSecurity.AdminUserName), which lint reads as
	// project_security().admin_user_name (mendixlabs/mxcli#1314).
	SetProjectAdminUserName(unitID model.ID, name string) error
	// SetProjectAdminUserRole sets that account's user role
	// (Security$ProjectSecurity.AdminUserRole). The caller resolves the role:
	// mxbuild does not report a name that does not resolve, it crashes on it
	// (ako/mxcli#2015).
	SetProjectAdminUserRole(unitID model.ID, role string) error
	// SetProjectAdminPassword sets that account's password
	// (Security$ProjectSecurity.AdminPassword); an empty one is CE0129 at
	// prototype and production level (mendixlabs/mxcli#624).
	SetProjectAdminPassword(unitID model.ID, password string) error
	// SetProjectGuestAccess toggles anonymous access. An empty guestUserRole
	// leaves the stored role untouched — the caller is responsible for having
	// established that a role exists, because Mendix raises CE0133 on guest
	// access with no role.
	SetProjectGuestAccess(unitID model.ID, enabled bool, guestUserRole string) error
	AddUserRole(unitID model.ID, name string, moduleRoles []string, manageAllRoles bool) error
	AlterUserRoleModuleRoles(unitID model.ID, userRoleName string, add bool, moduleRoles []string) error
	// SetUserRoleProperties sets the properties props states on an existing
	// user role and leaves the others as stored (ako/mxcli#707).
	SetUserRoleProperties(unitID model.ID, userRoleName string, props UserRoleProperties) error
	RemoveUserRole(unitID model.ID, name string) error
	AddDemoUser(unitID model.ID, userName, password, entity string, userRoles []string) error
	RemoveDemoUser(unitID model.ID, userName string) error
}

// UserRoleProperties are a user role's properties other than its name and
// module roles. A nil field is left as stored.
type UserRoleProperties struct {
	Description             *string
	ManageAllRoles          *bool
	ManageUsersWithoutRoles *bool
	CheckSecurity           *bool
	// ManageableRoles is set when non-nil; an empty, non-nil slice clears it.
	ManageableRoles []string
}

// IsZero reports whether p states no property.
func (p UserRoleProperties) IsZero() bool {
	return p.Description == nil && p.ManageAllRoles == nil && p.ManageUsersWithoutRoles == nil &&
		p.CheckSecurity == nil && p.ManageableRoles == nil
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
