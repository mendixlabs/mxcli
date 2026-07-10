// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateModuleRole(t *testing.T) {
	input := `CREATE MODULE ROLE MyModule.Editor DESCRIPTION 'Can edit content';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateModuleRoleStmt)
	if !ok {
		t.Fatalf("Expected CreateModuleRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "Editor" {
		t.Errorf("Expected MyModule.Editor, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
	if stmt.Description != "Can edit content" {
		t.Errorf("Expected description 'Can edit content', got %q", stmt.Description)
	}
}

func TestCreateModuleRole_NoDescription(t *testing.T) {
	input := `CREATE MODULE ROLE MyModule.Viewer;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateModuleRoleStmt)
	if !ok {
		t.Fatalf("Expected CreateModuleRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Description != "" {
		t.Errorf("Expected empty description, got %q", stmt.Description)
	}
}

func TestDropModuleRole(t *testing.T) {
	input := `DROP MODULE ROLE MyModule.Editor;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.DropModuleRoleStmt)
	if !ok {
		t.Fatalf("Expected DropModuleRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "MyModule" || stmt.Name.Name != "Editor" {
		t.Errorf("Expected MyModule.Editor, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}
}

func TestCreateUserRole(t *testing.T) {
	input := `CREATE USER ROLE Administrator (MyModule.Admin, OtherModule.FullAccess) MANAGE ALL ROLES;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateUserRoleStmt)
	if !ok {
		t.Fatalf("Expected CreateUserRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "Administrator" {
		t.Errorf("Expected Administrator, got %s", stmt.Name)
	}
	if len(stmt.ModuleRoles) != 2 {
		t.Fatalf("Expected 2 module roles, got %d", len(stmt.ModuleRoles))
	}
	if stmt.ModuleRoles[0].Module != "MyModule" || stmt.ModuleRoles[0].Name != "Admin" {
		t.Errorf("Expected MyModule.Admin, got %s.%s", stmt.ModuleRoles[0].Module, stmt.ModuleRoles[0].Name)
	}
	if !stmt.ManageAllRoles {
		t.Error("Expected ManageAllRoles true")
	}
	if stmt.CreateOrModify {
		t.Error("Expected CreateOrModify false")
	}
}

func TestCreateUserRole_OrModify(t *testing.T) {
	input := `CREATE OR MODIFY USER ROLE Editor (MyModule.Editor);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateUserRoleStmt)
	if !ok {
		t.Fatalf("Expected CreateUserRoleStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify true")
	}
	if stmt.ManageAllRoles {
		t.Error("Expected ManageAllRoles false")
	}
}

func TestAlterUserRole_Add(t *testing.T) {
	input := `ALTER USER ROLE Administrator ADD MODULE ROLES (NewModule.Admin);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterUserRoleStmt)
	if !ok {
		t.Fatalf("Expected AlterUserRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "Administrator" {
		t.Errorf("Expected Administrator, got %s", stmt.Name)
	}
	if !stmt.Add {
		t.Error("Expected Add true")
	}
	if len(stmt.ModuleRoles) != 1 {
		t.Fatalf("Expected 1 module role, got %d", len(stmt.ModuleRoles))
	}
}

func TestAlterUserRole_Remove(t *testing.T) {
	input := `ALTER USER ROLE Administrator REMOVE MODULE ROLES (OldModule.Legacy);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterUserRoleStmt)
	if !ok {
		t.Fatalf("Expected AlterUserRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Add {
		t.Error("Expected Add false")
	}
}

func TestDropUserRole(t *testing.T) {
	input := `DROP USER ROLE Administrator;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.DropUserRoleStmt)
	if !ok {
		t.Fatalf("Expected DropUserRoleStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "Administrator" {
		t.Errorf("Expected Administrator, got %s", stmt.Name)
	}
}

// Table-driven tests for grant/revoke statements — these follow repetitive patterns.

func TestGrantEntityAccess(t *testing.T) {
	input := `GRANT MyModule.Admin ON MyModule.Customer (CREATE, DELETE, READ *, WRITE (Name, Price)) WHERE '[Active = true]';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.GrantEntityAccessStmt)
	if !ok {
		t.Fatalf("Expected GrantEntityAccessStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Roles) != 1 {
		t.Fatalf("Expected 1 role, got %d", len(stmt.Roles))
	}
	if stmt.Roles[0].Module != "MyModule" || stmt.Roles[0].Name != "Admin" {
		t.Errorf("Expected MyModule.Admin role, got %s.%s", stmt.Roles[0].Module, stmt.Roles[0].Name)
	}
	if stmt.Entity.Module != "MyModule" || stmt.Entity.Name != "Customer" {
		t.Errorf("Expected MyModule.Customer entity, got %s.%s", stmt.Entity.Module, stmt.Entity.Name)
	}
	if len(stmt.Rights) != 4 {
		t.Fatalf("Expected 4 rights, got %d", len(stmt.Rights))
	}
	if stmt.Rights[0].Type != ast.EntityAccessCreate {
		t.Errorf("Expected CREATE, got %v", stmt.Rights[0].Type)
	}
	if stmt.Rights[1].Type != ast.EntityAccessDelete {
		t.Errorf("Expected DELETE, got %v", stmt.Rights[1].Type)
	}
	if stmt.Rights[2].Type != ast.EntityAccessReadAll {
		t.Errorf("Expected READ *, got %v", stmt.Rights[2].Type)
	}
	if stmt.Rights[3].Type != ast.EntityAccessWriteMembers {
		t.Errorf("Expected WRITE members, got %v", stmt.Rights[3].Type)
	}
	if len(stmt.Rights[3].Members) != 2 {
		t.Fatalf("Expected 2 write members, got %d", len(stmt.Rights[3].Members))
	}
	if stmt.Rights[3].Members[0] != "Name" || stmt.Rights[3].Members[1] != "Price" {
		t.Errorf("Expected [Name, Price], got %v", stmt.Rights[3].Members)
	}
	if stmt.XPathConstraint != "[Active = true]" {
		t.Errorf("Expected '[Active = true]', got %q", stmt.XPathConstraint)
	}
}

func TestGrantEntityAccess_QuotedMembers(t *testing.T) {
	// Members whose name is a reserved word can be escaped with quotes.
	input := `GRANT MyModule.Admin ON MyModule.Order (READ ("Order", "Status"), WRITE ("Order"));`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt := prog.Statements[0].(*ast.GrantEntityAccessStmt)
	if len(stmt.Rights) != 2 {
		t.Fatalf("expected 2 rights, got %d", len(stmt.Rights))
	}
	if stmt.Rights[0].Type != ast.EntityAccessReadMembers ||
		len(stmt.Rights[0].Members) != 2 ||
		stmt.Rights[0].Members[0] != "Order" || stmt.Rights[0].Members[1] != "Status" {
		t.Errorf("expected READ members [Order Status] (unquoted), got %v", stmt.Rights[0].Members)
	}
	if stmt.Rights[1].Type != ast.EntityAccessWriteMembers ||
		len(stmt.Rights[1].Members) != 1 || stmt.Rights[1].Members[0] != "Order" {
		t.Errorf("expected WRITE members [Order] (unquoted), got %v", stmt.Rights[1].Members)
	}
}

func TestGrantEntityAccess_MultipleRoles(t *testing.T) {
	input := `GRANT MyModule.Admin, MyModule.Editor ON MyModule.Customer (READ *);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.GrantEntityAccessStmt)
	if !ok {
		t.Fatalf("Expected GrantEntityAccessStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Roles) != 2 {
		t.Fatalf("Expected 2 roles, got %d", len(stmt.Roles))
	}
	if stmt.XPathConstraint != "" {
		t.Errorf("Expected empty XPath, got %q", stmt.XPathConstraint)
	}
}

func TestRevokeEntityAccess_Full(t *testing.T) {
	input := `REVOKE MyModule.Admin ON MyModule.Customer;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.RevokeEntityAccessStmt)
	if !ok {
		t.Fatalf("Expected RevokeEntityAccessStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Rights) != 0 {
		t.Errorf("Expected 0 rights for full revoke, got %d", len(stmt.Rights))
	}
}

func TestRevokeEntityAccess_Partial(t *testing.T) {
	input := `REVOKE MyModule.Admin ON MyModule.Customer (WRITE *);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.RevokeEntityAccessStmt)
	if !ok {
		t.Fatalf("Expected RevokeEntityAccessStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Rights) != 1 {
		t.Fatalf("Expected 1 right, got %d", len(stmt.Rights))
	}
	if stmt.Rights[0].Type != ast.EntityAccessWriteAll {
		t.Errorf("Expected WRITE *, got %v", stmt.Rights[0].Type)
	}
}

func TestGrantRevokeMicroflow(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		input := `GRANT EXECUTE ON MICROFLOW MyModule.ProcessOrder TO MyModule.Admin;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.GrantMicroflowAccessStmt)
		if !ok {
			t.Fatalf("Expected GrantMicroflowAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Microflow.Module != "MyModule" || stmt.Microflow.Name != "ProcessOrder" {
			t.Errorf("Expected MyModule.ProcessOrder, got %s.%s", stmt.Microflow.Module, stmt.Microflow.Name)
		}
		if len(stmt.Roles) != 1 || stmt.Roles[0].Name != "Admin" {
			t.Errorf("Expected 1 role (Admin), got %v", stmt.Roles)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		input := `REVOKE EXECUTE ON MICROFLOW MyModule.ProcessOrder FROM MyModule.Admin;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.RevokeMicroflowAccessStmt)
		if !ok {
			t.Fatalf("Expected RevokeMicroflowAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Microflow.Name != "ProcessOrder" {
			t.Errorf("Expected ProcessOrder, got %s", stmt.Microflow.Name)
		}
	})
}

func TestGrantRevokePage(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		input := `GRANT VIEW ON PAGE MyModule.HomePage TO MyModule.User;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.GrantPageAccessStmt)
		if !ok {
			t.Fatalf("Expected GrantPageAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Page.Name != "HomePage" {
			t.Errorf("Expected HomePage, got %s", stmt.Page.Name)
		}
		if len(stmt.Roles) != 1 || stmt.Roles[0].Name != "User" {
			t.Errorf("Expected 1 role (User), got %v", stmt.Roles)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		input := `REVOKE VIEW ON PAGE MyModule.AdminPage FROM MyModule.User;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.RevokePageAccessStmt)
		if !ok {
			t.Fatalf("Expected RevokePageAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Page.Name != "AdminPage" {
			t.Errorf("Expected AdminPage, got %s", stmt.Page.Name)
		}
	})
}

func TestGrantRevokeWorkflow(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		input := `GRANT EXECUTE ON WORKFLOW MyModule.ApprovalWF TO MyModule.Manager;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.GrantWorkflowAccessStmt)
		if !ok {
			t.Fatalf("Expected GrantWorkflowAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Workflow.Name != "ApprovalWF" {
			t.Errorf("Expected ApprovalWF, got %s", stmt.Workflow.Name)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		input := `REVOKE EXECUTE ON WORKFLOW MyModule.ApprovalWF FROM MyModule.Manager;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.RevokeWorkflowAccessStmt)
		if !ok {
			t.Fatalf("Expected RevokeWorkflowAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Workflow.Name != "ApprovalWF" {
			t.Errorf("Expected ApprovalWF, got %s", stmt.Workflow.Name)
		}
	})
}

func TestGrantRevokeODataService(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		input := `GRANT ACCESS ON ODATA SERVICE MyModule.OrderAPI TO MyModule.APIUser;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.GrantODataServiceAccessStmt)
		if !ok {
			t.Fatalf("Expected GrantODataServiceAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Service.Name != "OrderAPI" {
			t.Errorf("Expected OrderAPI, got %s", stmt.Service.Name)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		input := `REVOKE ACCESS ON ODATA SERVICE MyModule.OrderAPI FROM MyModule.APIUser;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.RevokeODataServiceAccessStmt)
		if !ok {
			t.Fatalf("Expected RevokeODataServiceAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Service.Name != "OrderAPI" {
			t.Errorf("Expected OrderAPI, got %s", stmt.Service.Name)
		}
	})
}

func TestGrantRevokePublishedRestService(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		input := `GRANT ACCESS ON PUBLISHED REST SERVICE MyModule.RestAPI TO MyModule.APIUser;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.GrantPublishedRestServiceAccessStmt)
		if !ok {
			t.Fatalf("Expected GrantPublishedRestServiceAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Service.Name != "RestAPI" {
			t.Errorf("Expected RestAPI, got %s", stmt.Service.Name)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		input := `REVOKE ACCESS ON PUBLISHED REST SERVICE MyModule.RestAPI FROM MyModule.APIUser;`
		prog, errs := Build(input)
		if len(errs) > 0 {
			for _, e := range errs {
				t.Errorf("Parse error: %v", e)
			}
			return
		}
		stmt, ok := prog.Statements[0].(*ast.RevokePublishedRestServiceAccessStmt)
		if !ok {
			t.Fatalf("Expected RevokePublishedRestServiceAccessStmt, got %T", prog.Statements[0])
		}
		if stmt.Service.Name != "RestAPI" {
			t.Errorf("Expected RestAPI, got %s", stmt.Service.Name)
		}
	})
}

func TestAlterProjectSecurity_Level(t *testing.T) {
	input := `ALTER PROJECT SECURITY LEVEL PRODUCTION;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterProjectSecurityStmt)
	if !ok {
		t.Fatalf("Expected AlterProjectSecurityStmt, got %T", prog.Statements[0])
	}
	if stmt.SecurityLevel != "Production" {
		t.Errorf("Expected Production, got %q", stmt.SecurityLevel)
	}
	if stmt.DemoUsersEnabled != nil {
		t.Error("Expected nil DemoUsersEnabled")
	}
}

func TestAlterProjectSecurity_DemoUsers(t *testing.T) {
	input := `ALTER PROJECT SECURITY DEMO USERS ON;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterProjectSecurityStmt)
	if !ok {
		t.Fatalf("Expected AlterProjectSecurityStmt, got %T", prog.Statements[0])
	}
	if stmt.SecurityLevel != "" {
		t.Errorf("Expected empty security level, got %q", stmt.SecurityLevel)
	}
	if stmt.DemoUsersEnabled == nil || !*stmt.DemoUsersEnabled {
		t.Error("Expected DemoUsersEnabled true")
	}
}

func TestCreateDemoUser(t *testing.T) {
	input := `CREATE DEMO USER 'demo_admin' PASSWORD '1' ENTITY MyModule.Account (Administrator);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateDemoUserStmt)
	if !ok {
		t.Fatalf("Expected CreateDemoUserStmt, got %T", prog.Statements[0])
	}
	if stmt.UserName != "demo_admin" {
		t.Errorf("Expected demo_admin, got %q", stmt.UserName)
	}
	if stmt.Password != "1" {
		t.Errorf("Expected '1', got %q", stmt.Password)
	}
	if stmt.Entity != "MyModule.Account" {
		t.Errorf("Expected MyModule.Account, got %q", stmt.Entity)
	}
	if len(stmt.UserRoles) != 1 || stmt.UserRoles[0] != "Administrator" {
		t.Errorf("Expected [Administrator], got %v", stmt.UserRoles)
	}
}

func TestCreateDemoUser_OrModify(t *testing.T) {
	input := `CREATE OR MODIFY DEMO USER 'demo_admin' PASSWORD '1' (Administrator);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateDemoUserStmt)
	if !ok {
		t.Fatalf("Expected CreateDemoUserStmt, got %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify {
		t.Error("Expected CreateOrModify true")
	}
}

func TestDropDemoUser(t *testing.T) {
	input := `DROP DEMO USER 'demo_admin';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.DropDemoUserStmt)
	if !ok {
		t.Fatalf("Expected DropDemoUserStmt, got %T", prog.Statements[0])
	}
	if stmt.UserName != "demo_admin" {
		t.Errorf("Expected demo_admin, got %q", stmt.UserName)
	}
}

func TestUpdateSecurity(t *testing.T) {
	input := `UPDATE SECURITY;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.UpdateSecurityStmt)
	if !ok {
		t.Fatalf("Expected UpdateSecurityStmt, got %T", prog.Statements[0])
	}
	if stmt.Module != "" {
		t.Errorf("Expected empty module, got %q", stmt.Module)
	}
}

func TestUpdateSecurity_InModule(t *testing.T) {
	input := `UPDATE SECURITY IN MyModule;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.UpdateSecurityStmt)
	if !ok {
		t.Fatalf("Expected UpdateSecurityStmt, got %T", prog.Statements[0])
	}
	if stmt.Module != "MyModule" {
		t.Errorf("Expected MyModule, got %q", stmt.Module)
	}
}
