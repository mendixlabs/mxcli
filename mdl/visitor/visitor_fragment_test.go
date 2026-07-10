// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestDefineFragment(t *testing.T) {
	input := `DEFINE FRAGMENT Footer AS {
		FOOTER f1 {
			ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
			ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.DefineFragmentStmt)
	if !ok {
		t.Fatalf("Expected DefineFragmentStmt, got %T", prog.Statements[0])
	}

	if stmt.Name != "Footer" {
		t.Errorf("Expected name 'Footer', got %q", stmt.Name)
	}

	if len(stmt.Widgets) != 1 {
		t.Fatalf("Expected 1 top-level widget, got %d", len(stmt.Widgets))
	}

	footer := stmt.Widgets[0]
	if footer.Type != "footer" {
		t.Errorf("Expected footer widget, got %s", footer.Type)
	}
	if footer.Name != "f1" {
		t.Errorf("Expected name 'f1', got %q", footer.Name)
	}
	if len(footer.Children) != 2 {
		t.Errorf("Expected 2 children in footer, got %d", len(footer.Children))
	}
}

func TestDefineFragmentMultipleWidgets(t *testing.T) {
	input := `DEFINE FRAGMENT FormFields AS {
		TEXTBOX txtName (Label: 'Name', Attribute: Name)
		TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt := prog.Statements[0].(*ast.DefineFragmentStmt)
	if stmt.Name != "FormFields" {
		t.Errorf("Expected name 'FormFields', got %q", stmt.Name)
	}
	if len(stmt.Widgets) != 2 {
		t.Errorf("Expected 2 widgets, got %d", len(stmt.Widgets))
	}
}

func TestShowFragments(t *testing.T) {
	input := `SHOW FRAGMENTS;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.ShowStmt)
	if !ok {
		t.Fatalf("Expected ShowStmt, got %T", prog.Statements[0])
	}

	if stmt.ObjectType != ast.ShowFragments {
		t.Errorf("Expected ShowFragments, got %v", stmt.ObjectType)
	}
}

func TestDescribeFragment(t *testing.T) {
	input := `DESCRIBE FRAGMENT Footer;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.DescribeStmt)
	if !ok {
		t.Fatalf("Expected DescribeStmt, got %T", prog.Statements[0])
	}

	if stmt.ObjectType != ast.DescribeFragment {
		t.Errorf("Expected DescribeFragment, got %v", stmt.ObjectType)
	}
	if stmt.Name.Name != "Footer" {
		t.Errorf("Expected name 'Footer', got %q", stmt.Name.Name)
	}
}

func TestUseFragmentInPageBody(t *testing.T) {
	input := `CREATE PAGE MyModule.TestPage
	(Title: 'Test', Layout: Atlas_Core.Atlas_Default)
	{
		TEXTBOX txt1 (Label: 'First', Attribute: Name)
		USE FRAGMENT Footer
		TEXTBOX txt2 (Label: 'Last', Attribute: Email)
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("Expected CreatePageStmtV3, got %T", prog.Statements[0])
	}

	if len(stmt.Widgets) != 3 {
		t.Fatalf("Expected 3 widgets (txt1, USE_FRAGMENT, txt2), got %d", len(stmt.Widgets))
	}

	// First widget: normal TEXTBOX
	if stmt.Widgets[0].Type != "textbox" {
		t.Errorf("Widget 0: expected textbox, got %s", stmt.Widgets[0].Type)
	}

	// Second widget: USE_FRAGMENT sentinel
	if stmt.Widgets[1].Type != "USE_FRAGMENT" {
		t.Errorf("Widget 1: expected USE_FRAGMENT, got %s", stmt.Widgets[1].Type)
	}
	if stmt.Widgets[1].Name != "Footer" {
		t.Errorf("Widget 1: expected name 'Footer', got %q", stmt.Widgets[1].Name)
	}

	// Third widget: normal TEXTBOX
	if stmt.Widgets[2].Type != "textbox" {
		t.Errorf("Widget 2: expected textbox, got %s", stmt.Widgets[2].Type)
	}
}

func TestUseFragmentWithPrefix(t *testing.T) {
	input := `CREATE PAGE MyModule.TestPage
	(Title: 'Test', Layout: Atlas_Core.Atlas_Default)
	{
		USE FRAGMENT Footer AS pfx_
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if len(stmt.Widgets) != 1 {
		t.Fatalf("Expected 1 widget, got %d", len(stmt.Widgets))
	}

	w := stmt.Widgets[0]
	if w.Type != "USE_FRAGMENT" {
		t.Errorf("Expected USE_FRAGMENT, got %s", w.Type)
	}
	if w.Name != "Footer" {
		t.Errorf("Expected name 'Footer', got %q", w.Name)
	}
	prefix, ok := w.Properties["Prefix"].(string)
	if !ok || prefix != "pfx_" {
		t.Errorf("Expected Prefix 'pfx_', got %v", w.Properties["Prefix"])
	}
}

func TestUseFragmentInNestedBody(t *testing.T) {
	input := `CREATE PAGE MyModule.TestPage
	(Title: 'Test', Layout: Atlas_Core.Atlas_Default)
	{
		DATAVIEW dv (DataSource: $Customer) {
			TEXTBOX txt1 (Label: 'Name', Attribute: Name)
			USE FRAGMENT Footer
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	if len(stmt.Widgets) != 1 {
		t.Fatalf("Expected 1 top-level widget (DATAVIEW), got %d", len(stmt.Widgets))
	}

	dv := stmt.Widgets[0]
	if dv.Type != "dataview" {
		t.Errorf("Expected dataview, got %s", dv.Type)
	}
	if len(dv.Children) != 2 {
		t.Fatalf("Expected 2 children in DATAVIEW, got %d", len(dv.Children))
	}
	if dv.Children[1].Type != "USE_FRAGMENT" {
		t.Errorf("Expected USE_FRAGMENT as second child, got %s", dv.Children[1].Type)
	}
}

func TestDescribeFragmentFromPage(t *testing.T) {
	input := `DESCRIBE FRAGMENT FROM PAGE MyModule.CustomerEdit WIDGET footer1;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.DescribeFragmentFromStmt)
	if !ok {
		t.Fatalf("Expected DescribeFragmentFromStmt, got %T", prog.Statements[0])
	}

	if stmt.ContainerType != "PAGE" {
		t.Errorf("Expected ContainerType 'PAGE', got %q", stmt.ContainerType)
	}
	if stmt.ContainerName.Module != "MyModule" {
		t.Errorf("Expected module 'MyModule', got %q", stmt.ContainerName.Module)
	}
	if stmt.ContainerName.Name != "CustomerEdit" {
		t.Errorf("Expected name 'CustomerEdit', got %q", stmt.ContainerName.Name)
	}
	if stmt.WidgetName != "footer1" {
		t.Errorf("Expected widget name 'footer1', got %q", stmt.WidgetName)
	}
}

func TestDescribeFragmentFromSnippet(t *testing.T) {
	input := `DESCRIBE FRAGMENT FROM SNIPPET MyModule.CardSnippet WIDGET container1;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.DescribeFragmentFromStmt)
	if !ok {
		t.Fatalf("Expected DescribeFragmentFromStmt, got %T", prog.Statements[0])
	}

	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("Expected ContainerType 'SNIPPET', got %q", stmt.ContainerType)
	}
	if stmt.ContainerName.Module != "MyModule" {
		t.Errorf("Expected module 'MyModule', got %q", stmt.ContainerName.Module)
	}
	if stmt.ContainerName.Name != "CardSnippet" {
		t.Errorf("Expected name 'CardSnippet', got %q", stmt.ContainerName.Name)
	}
	if stmt.WidgetName != "container1" {
		t.Errorf("Expected widget name 'container1', got %q", stmt.WidgetName)
	}
}

func TestDescribeFragmentSimpleStillWorks(t *testing.T) {
	// Verify that the simple DESCRIBE FRAGMENT Name still works after adding FROM variant
	input := `DESCRIBE FRAGMENT MyFooter;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.DescribeStmt)
	if !ok {
		t.Fatalf("Expected DescribeStmt, got %T", prog.Statements[0])
	}

	if stmt.ObjectType != ast.DescribeFragment {
		t.Errorf("Expected DescribeFragment, got %v", stmt.ObjectType)
	}
	if stmt.Name.Name != "MyFooter" {
		t.Errorf("Expected name 'MyFooter', got %q", stmt.Name.Name)
	}
}

func TestDefineAndShowFragments(t *testing.T) {
	input := `DEFINE FRAGMENT A AS { TEXTBOX t1 (Label: 'X') };
	DEFINE FRAGMENT B AS { CHECKBOX c1 (Label: 'Y') };
	SHOW FRAGMENTS;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	if len(prog.Statements) != 3 {
		t.Fatalf("Expected 3 statements, got %d", len(prog.Statements))
	}

	// Verify types
	if _, ok := prog.Statements[0].(*ast.DefineFragmentStmt); !ok {
		t.Errorf("Statement 0: expected DefineFragmentStmt, got %T", prog.Statements[0])
	}
	if _, ok := prog.Statements[1].(*ast.DefineFragmentStmt); !ok {
		t.Errorf("Statement 1: expected DefineFragmentStmt, got %T", prog.Statements[1])
	}
	if _, ok := prog.Statements[2].(*ast.ShowStmt); !ok {
		t.Errorf("Statement 2: expected ShowStmt, got %T", prog.Statements[2])
	}
}
