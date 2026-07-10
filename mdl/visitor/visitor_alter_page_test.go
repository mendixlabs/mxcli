// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestAlterPageSetPropertyOnWidget(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET Caption = 'Save' ON btnSave
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

	stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok {
		t.Fatalf("Expected AlterPageStmt, got %T", prog.Statements[0])
	}

	if stmt.PageName.Module != "Module" || stmt.PageName.Name != "Page" {
		t.Errorf("Expected Module.Page, got %s", stmt.PageName.String())
	}

	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}

	setOp, ok := stmt.Operations[0].(*ast.SetPropertyOp)
	if !ok {
		t.Fatalf("Expected SetPropertyOp, got %T", stmt.Operations[0])
	}

	if setOp.Target.Widget != "btnSave" {
		t.Errorf("Expected widget name 'btnSave', got %q", setOp.Target.Widget)
	}

	if v, ok := setOp.Properties["Caption"]; !ok || v != "Save" {
		t.Errorf("Expected Caption='Save', got %v", setOp.Properties["Caption"])
	}
}

func TestAlterPageSetMultiplePropertiesOnWidget(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET (Caption = 'Save', ButtonStyle = Success) ON btnSave
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	setOp := stmt.Operations[0].(*ast.SetPropertyOp)

	if setOp.Target.Widget != "btnSave" {
		t.Errorf("Expected widget name 'btnSave', got %q", setOp.Target.Widget)
	}

	if len(setOp.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(setOp.Properties))
	}

	if v, ok := setOp.Properties["Caption"]; !ok || v != "Save" {
		t.Errorf("Expected Caption='Save', got %v", setOp.Properties["Caption"])
	}

	if v, ok := setOp.Properties["ButtonStyle"]; !ok || v != "Success" {
		t.Errorf("Expected ButtonStyle='Success', got %v", setOp.Properties["ButtonStyle"])
	}
}

func TestAlterPageSetPageLevel(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET Title = 'New Title'
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	setOp := stmt.Operations[0].(*ast.SetPropertyOp)

	if setOp.Target.Widget != "" {
		t.Errorf("Expected empty widget name for page-level SET, got %q", setOp.Target.Widget)
	}

	if v, ok := setOp.Properties["Title"]; !ok || v != "New Title" {
		t.Errorf("Expected Title='New Title', got %v", setOp.Properties["Title"])
	}
}

func TestAlterPageSetQuotedProperty(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET 'showLabel' = false ON cbStatus
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	setOp := stmt.Operations[0].(*ast.SetPropertyOp)

	if setOp.Target.Widget != "cbStatus" {
		t.Errorf("Expected widget name 'cbStatus', got %q", setOp.Target.Widget)
	}

	if v, ok := setOp.Properties["showLabel"]; !ok || v != false {
		t.Errorf("Expected showLabel=false, got %v", setOp.Properties["showLabel"])
	}
}

func TestAlterPageInsertAfter(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		INSERT AFTER txtName {
			TEXTBOX txtNew (Label: 'New Field', Attribute: NewAttr)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	insertOp := stmt.Operations[0].(*ast.InsertWidgetOp)

	if insertOp.Position != "AFTER" {
		t.Errorf("Expected position 'AFTER', got %q", insertOp.Position)
	}

	if insertOp.Target.Widget != "txtName" {
		t.Errorf("Expected target 'txtName', got %q", insertOp.Target.Widget)
	}

	if len(insertOp.Widgets) != 1 {
		t.Fatalf("Expected 1 widget, got %d", len(insertOp.Widgets))
	}

	if insertOp.Widgets[0].Type != "textbox" {
		t.Errorf("Expected textbox, got %s", insertOp.Widgets[0].Type)
	}
}

func TestAlterPageInsertBefore(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		INSERT BEFORE txtEmail {
			TEXTBOX txtMiddle (Label: 'Middle Name', Attribute: MiddleName)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	insertOp := stmt.Operations[0].(*ast.InsertWidgetOp)

	if insertOp.Position != "BEFORE" {
		t.Errorf("Expected position 'BEFORE', got %q", insertOp.Position)
	}

	if insertOp.Target.Widget != "txtEmail" {
		t.Errorf("Expected target 'txtEmail', got %q", insertOp.Target.Widget)
	}
}

func TestAlterPageDropWidget(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		DROP WIDGET txtOld
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	dropOp := stmt.Operations[0].(*ast.DropWidgetOp)

	if len(dropOp.Targets) != 1 {
		t.Fatalf("Expected 1 widget name, got %d", len(dropOp.Targets))
	}

	if dropOp.Targets[0].Widget != "txtOld" {
		t.Errorf("Expected 'txtOld', got %q", dropOp.Targets[0].Widget)
	}
}

func TestAlterPageDropMultipleWidgets(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		DROP WIDGET a, b, c
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	dropOp := stmt.Operations[0].(*ast.DropWidgetOp)

	if len(dropOp.Targets) != 3 {
		t.Fatalf("Expected 3 widget names, got %d", len(dropOp.Targets))
	}

	expected := []string{"a", "b", "c"}
	for i, name := range expected {
		if dropOp.Targets[i].Widget != name {
			t.Errorf("Expected %q at index %d, got %q", name, i, dropOp.Targets[i].Widget)
		}
	}
}

func TestAlterPageReplace(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		REPLACE footer1 WITH {
			FOOTER newFooter {
				ACTIONBUTTON btnOK (Caption: 'OK', Action: SAVE_CHANGES, ButtonStyle: Primary)
			}
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	replaceOp := stmt.Operations[0].(*ast.ReplaceWidgetOp)

	if replaceOp.Target.Widget != "footer1" {
		t.Errorf("Expected 'footer1', got %q", replaceOp.Target.Widget)
	}

	if len(replaceOp.NewWidgets) != 1 {
		t.Fatalf("Expected 1 new widget, got %d", len(replaceOp.NewWidgets))
	}

	if replaceOp.NewWidgets[0].Type != "footer" {
		t.Errorf("Expected footer, got %s", replaceOp.NewWidgets[0].Type)
	}
}

func TestAlterPageMultipleOperations(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET Caption = 'Updated' ON btnSave;
		INSERT AFTER txtName {
			TEXTBOX txtMiddle (Label: 'Middle', Attribute: Middle)
		};
		DROP WIDGET txtUnused
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)

	if len(stmt.Operations) != 3 {
		t.Fatalf("Expected 3 operations, got %d", len(stmt.Operations))
	}

	if _, ok := stmt.Operations[0].(*ast.SetPropertyOp); !ok {
		t.Errorf("Operation 0: expected SetPropertyOp, got %T", stmt.Operations[0])
	}
	if _, ok := stmt.Operations[1].(*ast.InsertWidgetOp); !ok {
		t.Errorf("Operation 1: expected InsertWidgetOp, got %T", stmt.Operations[1])
	}
	if _, ok := stmt.Operations[2].(*ast.DropWidgetOp); !ok {
		t.Errorf("Operation 2: expected DropWidgetOp, got %T", stmt.Operations[2])
	}
}

func TestAlterPageNoSemicolonsBetweenOps(t *testing.T) {
	// Semicolons between operations are optional
	input := `ALTER PAGE Module.Page {
		SET Caption = 'Save' ON btnSave
		DROP WIDGET txtOld
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if len(stmt.Operations) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(stmt.Operations))
	}
}

func TestAlterPageInsertMultipleWidgets(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		INSERT AFTER txtName {
			TEXTBOX txtMiddle (Label: 'Middle', Attribute: Middle)
			TEXTBOX txtLast (Label: 'Last', Attribute: Last)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	insertOp := stmt.Operations[0].(*ast.InsertWidgetOp)

	if len(insertOp.Widgets) != 2 {
		t.Fatalf("Expected 2 widgets, got %d", len(insertOp.Widgets))
	}
}

// ============================================================================
// ALTER SNIPPET tests
// ============================================================================

func TestAlterSnippetReplace(t *testing.T) {
	input := `ALTER SNIPPET TaskList.Entity_Menu {
		REPLACE text1 WITH {
			DYNAMICTEXT text1 (Content: 'Your Tasks', RenderMode: H2)
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

	stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok {
		t.Fatalf("Expected AlterPageStmt, got %T", prog.Statements[0])
	}

	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("Expected ContainerType 'SNIPPET', got %q", stmt.ContainerType)
	}
	if stmt.PageName.Module != "TaskList" || stmt.PageName.Name != "Entity_Menu" {
		t.Errorf("Expected TaskList.Entity_Menu, got %s", stmt.PageName.String())
	}

	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}

	replaceOp, ok := stmt.Operations[0].(*ast.ReplaceWidgetOp)
	if !ok {
		t.Fatalf("Expected ReplaceWidgetOp, got %T", stmt.Operations[0])
	}
	if replaceOp.Target.Widget != "text1" {
		t.Errorf("Expected widget name 'text1', got %q", replaceOp.Target.Widget)
	}
	if len(replaceOp.NewWidgets) != 1 {
		t.Fatalf("Expected 1 new widget, got %d", len(replaceOp.NewWidgets))
	}
	if replaceOp.NewWidgets[0].Type != "dynamictext" {
		t.Errorf("Expected dynamictext, got %s", replaceOp.NewWidgets[0].Type)
	}
}

func TestAlterSnippetDrop(t *testing.T) {
	input := `ALTER SNIPPET Module.MySnippet {
		DROP WIDGET txtOld
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("Expected ContainerType 'SNIPPET', got %q", stmt.ContainerType)
	}

	dropOp := stmt.Operations[0].(*ast.DropWidgetOp)
	if len(dropOp.Targets) != 1 || dropOp.Targets[0].Widget != "txtOld" {
		t.Errorf("Expected DROP WIDGET txtOld, got %v", dropOp.Targets)
	}
}

func TestAlterSnippetSet(t *testing.T) {
	input := `ALTER SNIPPET Module.MySnippet {
		SET Caption = 'Updated' ON btnAction
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("Expected ContainerType 'SNIPPET', got %q", stmt.ContainerType)
	}

	setOp := stmt.Operations[0].(*ast.SetPropertyOp)
	if setOp.Target.Widget != "btnAction" {
		t.Errorf("Expected widget name 'btnAction', got %q", setOp.Target.Widget)
	}
}

func TestAlterSnippetMultipleOps(t *testing.T) {
	input := `ALTER SNIPPET Module.MySnippet {
		SET Caption = 'New' ON btn1;
		DROP WIDGET txtUnused;
		INSERT AFTER txt1 {
			TEXTBOX txtNew (Label: 'New', Attribute: NewAttr)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("Expected ContainerType 'SNIPPET', got %q", stmt.ContainerType)
	}
	if len(stmt.Operations) != 3 {
		t.Fatalf("Expected 3 operations, got %d", len(stmt.Operations))
	}
}

func TestAlterPageContainerType(t *testing.T) {
	// Verify ALTER PAGE sets ContainerType to "PAGE"
	input := `ALTER PAGE Module.Page { DROP WIDGET txt1 };`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}

	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if stmt.ContainerType != "PAGE" {
		t.Errorf("Expected ContainerType 'PAGE', got %q", stmt.ContainerType)
	}
}

// Issue #627 — ALTER PAGE SET Visible/Editable = [expr] roots bare attributes in
// $currentObject and routes to VisibleIf/EditableIf (so the mutator builds a
// ConditionalVisibilitySettings node, not a static field).
func TestAlterPageSetConditionalVisibility(t *testing.T) {
	input := `ALTER PAGE Module.Page {
		SET Visible = [Name != ''] ON ctn;
		SET Editable = [Active] ON tb;
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	if len(stmt.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(stmt.Operations))
	}
	vis := stmt.Operations[0].(*ast.SetPropertyOp)
	if vis.Target.Widget != "ctn" {
		t.Errorf("visible target = %q, want ctn", vis.Target.Widget)
	}
	if v, _ := vis.Properties["VisibleIf"].(string); v != "$currentObject/Name != ''" {
		t.Errorf("VisibleIf = %q, want $currentObject/Name != ''", v)
	}
	edit := stmt.Operations[1].(*ast.SetPropertyOp)
	if v, _ := edit.Properties["EditableIf"].(string); v != "$currentObject/Active" {
		t.Errorf("EditableIf = %q, want $currentObject/Active", v)
	}
}
