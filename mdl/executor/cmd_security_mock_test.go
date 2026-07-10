// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

func TestShowProjectSecurity_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				SecurityLevel:   "CheckEverything",
				EnableDemoUsers: true,
				AdminUserName:   "MxAdmin",
				UserRoles:       []*security.UserRole{{Name: "Admin"}, {Name: "User"}},
				DemoUsers:       []*security.DemoUser{{UserName: "demo_admin"}},
				PasswordPolicy:  &security.PasswordPolicy{MinimumLength: 8},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listProjectSecurity(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Security Level:")
	assertContainsStr(t, out, "MxAdmin")
	assertContainsStr(t, out, "Demo Users Enabled:")
}

func TestShowModuleRoles_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{{
				ContainerID: mod.ID,
				ModuleRoles: []*security.ModuleRole{
					{Name: "Admin"},
					{Name: "User"},
				},
			}}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listModuleRoles(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "Qualified Name")
	assertContainsStr(t, out, "Role")
	assertContainsStr(t, out, "Admin")
	assertContainsStr(t, out, "User")
}

func TestShowUserRoles_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				UserRoles: []*security.UserRole{
					{Name: "Administrator", ModuleRoles: []string{"MyModule.Admin"}},
					{Name: "NormalUser", ModuleRoles: []string{"MyModule.User"}},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listUserRoles(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Name")
	assertContainsStr(t, out, "Module Roles")
	assertContainsStr(t, out, "Administrator")
	assertContainsStr(t, out, "NormalUser")
}

func TestShowDemoUsers_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				EnableDemoUsers: true,
				DemoUsers: []*security.DemoUser{
					{UserName: "demo_admin", UserRoles: []string{"Administrator"}},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listDemoUsers(ctx))

	out := buf.String()
	assertContainsStr(t, out, "User Name")
	assertContainsStr(t, out, "demo_admin")
}

func TestShowDemoUsers_Disabled_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				EnableDemoUsers: false,
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listDemoUsers(ctx))
	assertContainsStr(t, buf.String(), "Demo users are disabled.")
}

func TestDescribeModuleRole_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{{
				ContainerID: mod.ID,
				ModuleRoles: []*security.ModuleRole{{Name: "Admin", Description: "Full access"}},
			}}, nil
		},
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				UserRoles: []*security.UserRole{
					{Name: "Administrator", ModuleRoles: []string{"MyModule.Admin"}},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeModuleRole(ctx, ast.QualifiedName{Module: "MyModule", Name: "Admin"}))
	assertContainsStr(t, buf.String(), "create module role")
}

func TestDescribeUserRole_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				UserRoles: []*security.UserRole{
					{Name: "Administrator", ModuleRoles: []string{"MyModule.Admin"}},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeUserRole(ctx, ast.QualifiedName{Name: "Administrator"}))
	assertContainsStr(t, buf.String(), "create user role")
}

func TestDescribeDemoUser_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				EnableDemoUsers: true,
				DemoUsers: []*security.DemoUser{
					{UserName: "demo_admin", UserRoles: []string{"Administrator"}},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeDemoUser(ctx, "demo_admin"))
	assertContainsStr(t, buf.String(), "create demo user")
}

func TestShowModuleRoles_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Sales")
	mod2 := mkModule("HR")
	h := mkHierarchy(mod1, mod2)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{
				{ContainerID: mod1.ID, ModuleRoles: []*security.ModuleRole{{Name: "Manager"}}},
				{ContainerID: mod2.ID, ModuleRoles: []*security.ModuleRole{{Name: "Employee"}}},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listModuleRoles(ctx, "HR"))

	out := buf.String()
	assertNotContainsStr(t, out, "Sales")
	assertContainsStr(t, out, "HR")
	assertContainsStr(t, out, "Employee")
}

func TestDescribeModuleRole_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{{
				ContainerID: mod.ID,
				ModuleRoles: []*security.ModuleRole{{Name: "Admin"}},
			}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, describeModuleRole(ctx, ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestDescribeUserRole_Mock_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				UserRoles: []*security.UserRole{{Name: "Admin"}},
			}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeUserRole(ctx, ast.QualifiedName{Name: "NonExistent"}))
}

func TestDescribeDemoUser_Mock_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				EnableDemoUsers: true,
				DemoUsers:       []*security.DemoUser{{UserName: "demo_admin"}},
			}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeDemoUser(ctx, "nonexistent"))
}

func TestShowAccessOnEntity_Mock_NilName(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listAccessOnEntity(ctx, nil))
}

func TestShowAccessOnMicroflow_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, listAccessOnMicroflow(ctx, &ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestShowAccessOnPage_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertError(t, listAccessOnPage(ctx, &ast.QualifiedName{Module: "MyModule", Name: "NonExistent"}))
}

func TestShowAccessOnWorkflow_Mock_Unsupported(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listAccessOnWorkflow(ctx, &ast.QualifiedName{Module: "MyModule", Name: "SomeWorkflow"}))
}

// TestGrantEntityAccess_XPathConstraint_PreservesRights verifies that granting
// entity access with an XPath WHERE clause shows the correct rights immediately
// after the GRANT (issue #431: output showed "(no access)" instead of "read *, write *").
func TestGrantEntityAccess_XPathConstraint_PreservesRights(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	statusAttr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{ID: nextID("attr")},
		Name:        "Status",
	}
	entityBefore := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: mod.ID,
		Name:        "Order",
		Persistable: true,
		Attributes:  []*domainmodel.Attribute{statusAttr},
		AccessRules: nil, // no rules yet
	}
	dmBefore := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{entityBefore},
	}

	// After AddEntityAccessRule, the second GetDomainModel call returns the entity
	// with the rule already applied (simulating what a real MPR backend would do).
	entityAfter := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: entityBefore.ID},
		ContainerID: mod.ID,
		Name:        "Order",
		Persistable: true,
		Attributes:  []*domainmodel.Attribute{statusAttr},
		AccessRules: []*domainmodel.AccessRule{
			{
				ModuleRoleNames:           []string{"MyModule.User"},
				AllowCreate:               false,
				AllowDelete:               false,
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadWrite,
				XPathConstraint:           "[Status = 'Open']",
				MemberAccesses: []*domainmodel.MemberAccess{
					{
						AttributeName: "MyModule.Order.Status",
						AccessRights:  domainmodel.MemberAccessRightsReadWrite,
					},
				},
			},
		},
	}
	dmAfter := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: dmBefore.ID},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{entityAfter},
	}

	callCount := 0
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		GetModuleSecurityFunc: func(moduleID model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{
				ModuleRoles: []*security.ModuleRole{{Name: "User"}},
			}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			callCount++
			if callCount == 1 {
				return dmBefore, nil // first call: before grant
			}
			return dmAfter, nil // second call (formatAccessRuleResult): after grant
		},
		AddEntityAccessRuleFunc: func(params backend.EntityAccessRuleParams) error {
			if params.XPathConstraint != "[Status = 'Open']" {
				t.Errorf("XPathConstraint not passed: got %q, want %q", params.XPathConstraint, "[Status = 'Open']")
			}
			if params.DefaultMemberAccess != "ReadWrite" {
				t.Errorf("DefaultMemberAccess not passed: got %q, want ReadWrite", params.DefaultMemberAccess)
			}
			return nil
		},
		ReconcileMemberAccessesFunc: func(unitID model.ID, moduleName string) (int, error) {
			return 0, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "MyModule", Name: "Order"},
		Roles:  []ast.QualifiedName{{Module: "MyModule", Name: "User"}},
		Rights: []ast.EntityAccessRight{
			{Type: ast.EntityAccessReadAll},
			{Type: ast.EntityAccessWriteAll},
		},
		XPathConstraint: "[Status = 'Open']",
	}
	assertNoError(t, execGrantEntityAccess(ctx, stmt))

	out := buf.String()
	assertContainsStr(t, out, "Granted access")
	assertNotContainsStr(t, out, "(no access)")
	assertContainsStr(t, out, "read *")
}

// TestOutputEntityAccessGrants_XPathConstraint_EscapedQuotes verifies that
// outputEntityAccessGrants escapes single quotes inside the XPath constraint
// so the DESCRIBE ENTITY output is valid re-parseable MDL (issue #431).
func TestOutputEntityAccessGrants_XPathConstraint_EscapedQuotes(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	entity := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: mod.ID,
		Name:        "Order",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: "Status"},
		},
		AccessRules: []*domainmodel.AccessRule{
			{
				ModuleRoleNames:           []string{"MyModule.User"},
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadWrite,
				XPathConstraint:           "[Status = 'Open']",
				MemberAccesses: []*domainmodel.MemberAccess{
					{
						AttributeName: "MyModule.Order.Status",
						AccessRights:  domainmodel.MemberAccessRightsReadWrite,
					},
				},
			},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	outputEntityAccessGrants(ctx, entity, "MyModule", "Order")

	out := buf.String()
	// Single quotes inside the XPath must be doubled for valid MDL
	assertContainsStr(t, out, "''Open''")
	// Should NOT contain unescaped version
	assertNotContainsStr(t, out, "= 'Open'")
	// The outer where clause delimiters must still be single quotes
	assertContainsStr(t, out, "where '")
}

// TestGrantEntityAccess_FakeRole_Issue399 verifies that GRANT ON ENTITY rejects
// a non-existent module role instead of silently creating a phantom access rule.
func TestGrantEntityAccess_FakeRole_Issue399(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	entity := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		Name:        "Order",
		Persistable: true,
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{entity},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		// No roles defined in module security
		GetModuleSecurityFunc: func(moduleID model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: nil}, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execGrantEntityAccess(ctx, &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "MyModule", Name: "Order"},
		Roles:  []ast.QualifiedName{{Module: "MyModule", Name: "FakeRole"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessReadAll}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "module role")
	assertContainsStr(t, err.Error(), "FakeRole")
}

// TestRevokeEntityAccess_FakeRole_Issue399 verifies that REVOKE ON ENTITY also
// rejects non-existent module roles.
func TestRevokeEntityAccess_FakeRole_Issue399(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	entity := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		Name:        "Customer",
		Persistable: true,
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{entity},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleSecurityFunc: func(moduleID model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: nil}, nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execRevokeEntityAccess(ctx, &ast.RevokeEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "MyModule", Name: "Customer"},
		Roles:  []ast.QualifiedName{{Module: "MyModule", Name: "GhostRole"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "module role")
	assertContainsStr(t, err.Error(), "GhostRole")
}
