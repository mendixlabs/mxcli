// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestQuotedIdentifiersInPageWidgets(t *testing.T) {
	input := `CREATE PAGE MaisonElegance."Collection_Overview" (
		Layout: Atlas_Core."Atlas_Default"
	) {
		DATAVIEW dv (DataSource: DATABASE FROM MaisonElegance."Collection") {
			TEXTBOX txtName (Attribute: Name, Label: 'Name')
			ACTIONBUTTON btnEdit (
				Caption: 'Edit',
				Action: SHOW_PAGE MaisonElegance."Collection_NewEdit"
			)
			ACTIONBUTTON btnRun (
				Caption: 'Run',
				Action: MICROFLOW MaisonElegance."ACT_Collection_Run"
			)
		}
		SNIPPETCALL sc (Snippet: MaisonElegance."Footer_Snippet")
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}

	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("Expected CreatePageStmtV3, got %T", prog.Statements[0])
	}

	// Layout should be unquoted
	if stmt.Layout != "Atlas_Core.Atlas_Default" {
		t.Errorf("Layout: expected 'Atlas_Core.Atlas_Default', got %q", stmt.Layout)
	}

	// Page name should be unquoted
	if stmt.Name.Module != "MaisonElegance" || stmt.Name.Name != "Collection_Overview" {
		t.Errorf("Page name: expected MaisonElegance.Collection_Overview, got %s.%s", stmt.Name.Module, stmt.Name.Name)
	}

	// Find the DataView and check DataSource entity reference is unquoted
	if len(stmt.Widgets) < 1 {
		t.Fatal("Expected at least 1 child widget")
	}
	dv := stmt.Widgets[0]
	if dv.GetDataSource() == nil {
		t.Fatal("DataView DataSource is nil")
	}
	if dv.GetDataSource().Reference != "MaisonElegance.Collection" {
		t.Errorf("DataSource.Reference: expected 'MaisonElegance.Collection', got %q", dv.GetDataSource().Reference)
	}

	// Find SHOW_PAGE action button and check target is unquoted
	btnEdit := findChildByName(dv, "btnEdit")
	if btnEdit == nil {
		t.Fatal("btnEdit widget not found")
	}
	action := btnEdit.GetAction()
	if action == nil {
		t.Fatal("btnEdit Action is nil")
	}
	if action.Target != "MaisonElegance.Collection_NewEdit" {
		t.Errorf("SHOW_PAGE target: expected 'MaisonElegance.Collection_NewEdit', got %q", action.Target)
	}

	// Find MICROFLOW action button and check target is unquoted
	btnRun := findChildByName(dv, "btnRun")
	if btnRun == nil {
		t.Fatal("btnRun widget not found")
	}
	runAction := btnRun.GetAction()
	if runAction == nil {
		t.Fatal("btnRun Action is nil")
	}
	if runAction.Target != "MaisonElegance.ACT_Collection_Run" {
		t.Errorf("MICROFLOW target: expected 'MaisonElegance.ACT_Collection_Run', got %q", runAction.Target)
	}

	// Find SNIPPETCALL and check snippet reference is unquoted
	sc := findChildByName2(stmt.Widgets, "sc")
	if sc == nil {
		t.Fatal("sc (SnippetCall) widget not found")
	}
	snippetRef, ok := sc.Properties["Snippet"].(string)
	if !ok {
		t.Fatal("Snippet property not a string")
	}
	if snippetRef != "MaisonElegance.Footer_Snippet" {
		t.Errorf("Snippet ref: expected 'MaisonElegance.Footer_Snippet', got %q", snippetRef)
	}
}

// TestQuotedReservedWidgetNames covers issue #619: a widget whose name collides
// with a reserved keyword (e.g. "List", "Column") must be expressible by quoting
// it. DESCRIBE emits the quoted form (see executor.mdlIdent), and this asserts the
// parser+visitor accept it and unquote back to the bare name.
func TestQuotedReservedWidgetNames(t *testing.T) {
	input := `CREATE PAGE MyModule.Home (
		Layout: Atlas_Core.Atlas_Default
	) {
		CONTAINER "List" {
			DYNAMICTEXT "Template" (Content: 'x')
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		t.FailNow()
	}

	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("Expected CreatePageStmtV3, got %T", prog.Statements[0])
	}

	// The reserved-word container name must be unquoted back to "List".
	container := findChildByName2(stmt.Widgets, "List")
	if container == nil {
		t.Fatal(`container named "List" not found (quoted reserved name lost)`)
	}
	// And its reserved-word child "Template" likewise.
	if findChildByName(container, "Template") == nil {
		t.Error(`child dynamictext named "Template" not found`)
	}
}

// TestQuotedReservedParamNames covers issue #114: a page or snippet parameter
// named after a reserved keyword (e.g. "List") must be expressible by quoting it
// in the Params block. The visitor unquotes it back to the bare name. (The $-form
// $List already worked; this closes the bare-declaration gap for parity with #619.)
func TestQuotedReservedParamNames(t *testing.T) {
	t.Run("page", func(t *testing.T) {
		input := `CREATE PAGE M.Home (
			Layout: Atlas_Core.Atlas_Default,
			Params: { "List": M.Order, "Template": M.Order }
		) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`

		prog, errs := Build(input)
		if len(errs) > 0 {
			t.Fatalf("Parse errors: %v", errs)
		}
		stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
		got := map[string]bool{}
		for _, p := range stmt.Parameters {
			got[p.Name] = true
		}
		if !got["List"] || !got["Template"] {
			t.Errorf("expected unquoted params List, Template; got %v", got)
		}
	})

	t.Run("snippet", func(t *testing.T) {
		input := `CREATE SNIPPET M.Snip (
			Params: { "List": M.Order }
		) { CONTAINER c { DYNAMICTEXT t (Content: 'x') } };`

		prog, errs := Build(input)
		if len(errs) > 0 {
			t.Fatalf("Parse errors: %v", errs)
		}
		stmt := prog.Statements[0].(*ast.CreateSnippetStmtV3)
		if len(stmt.Parameters) != 1 || stmt.Parameters[0].Name != "List" {
			t.Errorf(`expected one param unquoted to "List", got %+v`, stmt.Parameters)
		}
	})
}

// TestQuotedReservedSelectionAndArgNames covers the #619 grammar-widen slice:
// a SELECTION data-source widget reference (dataSourceExprV3) and a microflow/
// page argument parameter name (microflowArgV3) that collide with a reserved
// keyword. Both positions previously accepted only a bare IDENTIFIER (so even
// hand-quoting failed); they now accept QUOTED_IDENTIFIER and the visitor
// unquotes back to the bare name.
func TestQuotedReservedSelectionAndArgNames(t *testing.T) {
	input := `CREATE PAGE M.Home (
		Layout: Atlas_Core.Atlas_Default
	) {
		GALLERY "List" (DataSource: DATABASE FROM M.Product, Selection: single) {
			DYNAMICTEXT t1 (Content: 'row')
		}
		DATAVIEW detail (DataSource: SELECTION "List") {
			ACTIONBUTTON btnOpen (
				Caption: 'Open',
				Action: SHOW_PAGE M.Home ("Status": $currentObject)
			)
		}
	};`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)

	// SELECTION reference must unquote back to the reserved word "List".
	detail := findChildByName2(stmt.Widgets, "detail")
	if detail == nil || detail.GetDataSource() == nil {
		t.Fatal("detail dataview or its datasource not found")
	}
	if got := detail.GetDataSource().Reference; got != "List" {
		t.Errorf(`SELECTION reference: expected "List", got %q`, got)
	}

	// The reserved-word argument param name "Status" must unquote.
	btnOpen := findChildByName(detail, "btnOpen")
	if btnOpen == nil || btnOpen.GetAction() == nil {
		t.Fatal("btnOpen or its action not found")
	}
	args := btnOpen.GetAction().Args
	if len(args) != 1 || args[0].Name != "Status" {
		t.Errorf(`expected one arg named "Status", got %+v`, args)
	}
}

func findChildByName(parent *ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, c := range parent.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func findChildByName2(widgets []*ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, w := range widgets {
		if w.Name == name {
			return w
		}
		if found := findChildByName(w, name); found != nil {
			return found
		}
	}
	return nil
}
