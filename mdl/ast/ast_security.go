// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Security Statements
// ============================================================================

// CreateModuleRoleStmt represents: CREATE MODULE ROLE Module.RoleName [DESCRIPTION '...']
type CreateModuleRoleStmt struct {
	CreateGuard // `create … if not exists` (ako/mxcli#731)
	Name        QualifiedName
	// Description is nil when the statement has no DESCRIPTION clause: a new
	// role then has none, and `create or modify` keeps the stored one
	// (ako/mxcli#1218), as CreateUserRoleStmt's properties do. `description ''`
	// is the stated empty description, which clears it.
	Description *string
	// CreateOrModify makes the statement idempotent: an existing role has its
	// description updated instead of the statement failing, so a security script
	// can be re-run.
	CreateOrModify bool
}

// DescriptionText is the description the statement states, or "" when it
// states none.
func (s *CreateModuleRoleStmt) DescriptionText() string {
	if s.Description == nil {
		return ""
	}
	return *s.Description
}

func (s *CreateModuleRoleStmt) isStatement() {}

// DropModuleRoleStmt represents: DROP MODULE ROLE [IF EXISTS] Module.RoleName
type DropModuleRoleStmt struct {
	// IfExists downgrades "not found" to a no-op; see DropUserRoleStmt.
	IfExists bool
	Name     QualifiedName
}

func (s *DropModuleRoleStmt) isStatement() {}

// CreateUserRoleStmt represents
// CREATE [OR MODIFY] USER ROLE Name [( ModuleRoles: (…), Description: '…', … )],
// and the old positional form CREATE USER ROLE Name (ModuleRole, …) [MANAGE ALL ROLES].
//
// The pointer properties are nil when the statement does not state them: a new
// role then gets Mendix's default, and `create or modify` leaves the stored
// value alone (ako/mxcli#707).
type CreateUserRoleStmt struct {
	CreateGuard    // `create … if not exists` (ako/mxcli#731)
	Name           string
	ModuleRoles    []QualifiedName
	ManageAllRoles bool
	CreateOrModify bool // If true, adds module roles to existing role instead of failing

	Description             *string
	CheckSecurity           *bool
	ManageUsersWithoutRoles *bool
	// ManageableRoles names the user roles this role may manage; nil when the
	// statement does not say. Only meaningful when ManageAllRoles is false.
	ManageableRoles []string
	ManageableSet   bool
	// ManageAllRolesSet is true when the statement states ManageAllRoles, so
	// `create or modify` can set it to false as well as to true.
	ManageAllRolesSet bool
}

func (s *CreateUserRoleStmt) isStatement() {}

// AlterUserRoleStmt represents: ALTER USER ROLE Name ADD/REMOVE MODULE ROLES (...)
type AlterUserRoleStmt struct {
	Name        string
	Add         bool // true = ADD, false = REMOVE
	ModuleRoles []QualifiedName
}

func (s *AlterUserRoleStmt) isStatement() {}

// DropUserRoleStmt represents: DROP USER ROLE [IF EXISTS] Name
type DropUserRoleStmt struct {
	// IfExists downgrades "not found" to a no-op, so a one-time cleanup can sit
	// in a slice script that is re-run.
	IfExists bool
	Name     string
}

func (s *DropUserRoleStmt) isStatement() {}

// EntityAccessRight represents a single access right in a GRANT statement.
type EntityAccessRight struct {
	Type    EntityAccessRightType
	Members []string // For READ/WRITE with specific members
}

// EntityAccessRightType represents the type of entity access right.
type EntityAccessRightType int

const (
	EntityAccessCreate EntityAccessRightType = iota
	EntityAccessDelete
	EntityAccessReadAll      // READ *
	EntityAccessReadMembers  // READ (member1, member2)
	EntityAccessWriteAll     // WRITE *
	EntityAccessWriteMembers // WRITE (member1, member2)
)

// GrantEntityAccessStmt represents: GRANT role1, role2 ON Module.Entity (CREATE, DELETE, READ *, WRITE *) [WHERE '...']
type GrantEntityAccessStmt struct {
	Roles           []QualifiedName
	Entity          QualifiedName
	Rights          []EntityAccessRight
	XPathConstraint string // Optional WHERE clause
}

func (s *GrantEntityAccessStmt) isStatement() {}

// RevokeEntityAccessStmt represents: REVOKE role1, role2 ON Module.Entity [(rights...)]
// When Rights is nil, the entire access rule is removed. When non-nil, only the
// specified rights are revoked (partial revoke).
type RevokeEntityAccessStmt struct {
	Roles  []QualifiedName
	Entity QualifiedName
	Rights []EntityAccessRight // nil = full revoke, non-nil = partial
}

func (s *RevokeEntityAccessStmt) isStatement() {}

// GrantMicroflowAccessStmt represents: GRANT EXECUTE ON MICROFLOW Module.MF TO role1, role2
type GrantMicroflowAccessStmt struct {
	Microflow QualifiedName
	Roles     []QualifiedName
}

func (s *GrantMicroflowAccessStmt) isStatement() {}

// RevokeMicroflowAccessStmt represents: REVOKE EXECUTE ON MICROFLOW Module.MF FROM role1, role2
type RevokeMicroflowAccessStmt struct {
	Microflow QualifiedName
	Roles     []QualifiedName
}

func (s *RevokeMicroflowAccessStmt) isStatement() {}

// GrantNanoflowAccessStmt represents: GRANT EXECUTE ON NANOFLOW Module.NF TO role1, role2
type GrantNanoflowAccessStmt struct {
	Nanoflow QualifiedName
	Roles    []QualifiedName
}

func (s *GrantNanoflowAccessStmt) isStatement() {}

// RevokeNanoflowAccessStmt represents: REVOKE EXECUTE ON NANOFLOW Module.NF FROM role1, role2
type RevokeNanoflowAccessStmt struct {
	Nanoflow QualifiedName
	Roles    []QualifiedName
}

func (s *RevokeNanoflowAccessStmt) isStatement() {}

// GrantPageAccessStmt represents: GRANT VIEW ON PAGE Module.Page TO role1, role2
type GrantPageAccessStmt struct {
	Page  QualifiedName
	Roles []QualifiedName
}

func (s *GrantPageAccessStmt) isStatement() {}

// RevokePageAccessStmt represents: REVOKE VIEW ON PAGE Module.Page FROM role1, role2
type RevokePageAccessStmt struct {
	Page  QualifiedName
	Roles []QualifiedName
}

func (s *RevokePageAccessStmt) isStatement() {}

// GrantODataServiceAccessStmt represents: GRANT ACCESS ON ODATA SERVICE Module.Svc TO role1, role2
type GrantODataServiceAccessStmt struct {
	Service QualifiedName
	Roles   []QualifiedName
}

func (s *GrantODataServiceAccessStmt) isStatement() {}

// RevokeODataServiceAccessStmt represents: REVOKE ACCESS ON ODATA SERVICE Module.Svc FROM role1, role2
type RevokeODataServiceAccessStmt struct {
	Service QualifiedName
	Roles   []QualifiedName
}

func (s *RevokeODataServiceAccessStmt) isStatement() {}

// GrantPublishedRestServiceAccessStmt represents: GRANT ACCESS ON PUBLISHED REST SERVICE Module.Svc TO role1, role2
type GrantPublishedRestServiceAccessStmt struct {
	Service QualifiedName
	Roles   []QualifiedName
}

func (s *GrantPublishedRestServiceAccessStmt) isStatement() {}

// RevokePublishedRestServiceAccessStmt represents: REVOKE ACCESS ON PUBLISHED REST SERVICE Module.Svc FROM role1, role2
type RevokePublishedRestServiceAccessStmt struct {
	Service QualifiedName
	Roles   []QualifiedName
}

func (s *RevokePublishedRestServiceAccessStmt) isStatement() {}

// AlterProjectSecurityStmt represents ALTER PROJECT SECURITY commands.
type AlterProjectSecurityStmt struct {
	// SecurityLevel is set for ALTER PROJECT SECURITY LEVEL (PRODUCTION|PROTOTYPE|OFF)
	SecurityLevel string
	// DemoUsersEnabled is set for ALTER PROJECT SECURITY DEMO USERS ON/OFF
	DemoUsersEnabled *bool
	// GuestAccessEnabled is set for ALTER PROJECT SECURITY GUEST ACCESS ON/OFF
	GuestAccessEnabled *bool
	// GuestUserRole is the user role anonymous visitors get, from the optional
	// ROLE clause on GUEST ACCESS ON. Empty means "keep whatever is stored" —
	// never "clear it"; the executor refuses ON when nothing is stored either.
	GuestUserRole string
	// StrictModeEnabled is set for ALTER PROJECT SECURITY STRICT MODE ON/OFF.
	// A pointer, so "the statement said nothing about it" is distinguishable
	// from "the statement asked for off".
	StrictModeEnabled *bool
	// AdminUserName renames the built-in administrator account (MxAdmin by
	// default). Empty means "keep what is stored".
	AdminUserName string
	// AdminUserRole is the user role of that account
	// (Security$ProjectSecurity.AdminUserRole, App Security > Administrator >
	// User role). Empty means "keep what is stored" (ako/mxcli#2015).
	AdminUserRole string
	// AdminPassword sets that account's password, which Mendix stores in the
	// model; empty means "keep what is stored". An empty stored password is
	// CE0129 at prototype and production level (mendixlabs/mxcli#624), so the
	// visitor refuses '' rather than letting a script write the error.
	AdminPassword string
}

func (s *AlterProjectSecurityStmt) isStatement() {}

// CreateDemoUserStmt represents: CREATE [OR MODIFY] DEMO USER 'name' ( Password: 'pw', Entity: Module.Entity, UserRoles: (Role1, Role2) )
type CreateDemoUserStmt struct {
	CreateGuard    // `create … if not exists` (ako/mxcli#731)
	UserName       string
	Password       string
	Entity         string // qualified name of user entity, e.g. "Administration.Account"
	UserRoles      []string
	CreateOrModify bool // If true, updates existing user's roles additively
}

func (s *CreateDemoUserStmt) isStatement() {}

// DropDemoUserStmt represents: DROP DEMO USER [IF EXISTS] 'name'
type DropDemoUserStmt struct {
	// IfExists downgrades "not found" to a no-op; see DropUserRoleStmt.
	IfExists bool
	UserName string
}

func (s *DropDemoUserStmt) isStatement() {}

// UpdateSecurityStmt represents: UPDATE SECURITY [IN Module]
type UpdateSecurityStmt struct {
	Module string // optional, empty = all modules
}

func (s *UpdateSecurityStmt) isStatement() {}
