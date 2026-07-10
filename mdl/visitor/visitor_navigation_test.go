// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateNavigation(t *testing.T) {
	input := `CREATE NAVIGATION Responsive
		HOME PAGE MyModule.HomePage
		LOGIN PAGE MyModule.LoginPage
		MENU (
			MENU ITEM 'Dashboard' PAGE MyModule.Dashboard;
		);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterNavigationStmt)
	if !ok {
		t.Fatalf("Expected AlterNavigationStmt, got %T", prog.Statements[0])
	}
	if stmt.ProfileName != "Responsive" {
		t.Errorf("Got ProfileName %q", stmt.ProfileName)
	}
	if len(stmt.HomePages) != 1 {
		t.Fatalf("Expected 1 home page, got %d", len(stmt.HomePages))
	}
	if !stmt.HomePages[0].IsPage {
		t.Error("Expected IsPage true")
	}
	if stmt.HomePages[0].Target.Name != "HomePage" {
		t.Errorf("Got target %s", stmt.HomePages[0].Target.Name)
	}
	if stmt.LoginPage == nil || stmt.LoginPage.Name != "LoginPage" {
		t.Error("LoginPage mismatch")
	}
	if !stmt.HasMenuBlock {
		t.Error("Expected HasMenuBlock true")
	}
	if len(stmt.MenuItems) != 1 {
		t.Fatalf("Expected 1 menu item, got %d", len(stmt.MenuItems))
	}
	if stmt.MenuItems[0].Caption != "Dashboard" {
		t.Errorf("Got caption %q", stmt.MenuItems[0].Caption)
	}
}
