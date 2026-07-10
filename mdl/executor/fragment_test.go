// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

func TestExecDefineFragment(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	stmt := &ast.DefineFragmentStmt{
		Name: "Footer",
		Widgets: []*ast.WidgetV3{
			{
				Type:       "footer",
				Name:       "f1",
				Properties: map[string]interface{}{},
				Children: []*ast.WidgetV3{
					{
						Type:       "actionbutton",
						Name:       "btnSave",
						Properties: map[string]interface{}{"Caption": "Save"},
						Children:   []*ast.WidgetV3{},
					},
				},
			},
		},
	}

	err := exec.Execute(stmt)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if exec.fragments == nil || exec.fragments["Footer"] == nil {
		t.Fatal("Fragment 'Footer' not registered")
	}

	output := buf.String()
	if !strings.Contains(output, "Defined fragment Footer") {
		t.Errorf("Expected confirmation message, got: %s", output)
	}
}

func TestExecDefineFragmentDuplicate(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	stmt := &ast.DefineFragmentStmt{
		Name:    "Footer",
		Widgets: []*ast.WidgetV3{},
	}

	// First define should succeed
	if err := exec.Execute(stmt); err != nil {
		t.Fatalf("First define failed: %v", err)
	}

	// Second define should fail
	err := exec.Execute(stmt)
	if err == nil {
		t.Fatal("Expected error for duplicate fragment, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestExecShowFragmentsEmpty(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	err := exec.Execute(&ast.ShowStmt{ObjectType: ast.ShowFragments})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No fragments defined") {
		t.Errorf("Expected 'No fragments defined', got: %s", output)
	}
}

func TestExecShowFragmentsWithDefined(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	// Define two fragments
	exec.Execute(&ast.DefineFragmentStmt{
		Name: "Alpha",
		Widgets: []*ast.WidgetV3{
			{Type: "textbox", Name: "t1", Properties: map[string]interface{}{}, Children: []*ast.WidgetV3{}},
		},
	})
	exec.Execute(&ast.DefineFragmentStmt{
		Name: "Beta",
		Widgets: []*ast.WidgetV3{
			{Type: "checkbox", Name: "c1", Properties: map[string]interface{}{}, Children: []*ast.WidgetV3{}},
			{Type: "checkbox", Name: "c2", Properties: map[string]interface{}{}, Children: []*ast.WidgetV3{}},
		},
	})

	buf.Reset()
	err := exec.Execute(&ast.ShowStmt{ObjectType: ast.ShowFragments})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Alpha") {
		t.Errorf("Expected 'Alpha' in output, got: %s", output)
	}
	if !strings.Contains(output, "Beta") {
		t.Errorf("Expected 'Beta' in output, got: %s", output)
	}
}

func TestExecDescribeFragment(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	exec.Execute(&ast.DefineFragmentStmt{
		Name: "Footer",
		Widgets: []*ast.WidgetV3{
			{
				Type:       "footer",
				Name:       "f1",
				Properties: map[string]interface{}{},
				Children: []*ast.WidgetV3{
					{
						Type:       "actionbutton",
						Name:       "btnSave",
						Properties: map[string]interface{}{"Caption": "Save", "ButtonStyle": "Primary"},
						Children:   []*ast.WidgetV3{},
					},
				},
			},
		},
	})

	buf.Reset()
	err := exec.Execute(&ast.DescribeStmt{
		ObjectType: ast.DescribeFragment,
		Name:       ast.QualifiedName{Name: "Footer"},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "define fragment Footer") {
		t.Errorf("Expected 'define fragment Footer' in output, got: %s", output)
	}
	if !strings.Contains(output, "footer f1") {
		t.Errorf("Expected 'footer f1' in output, got: %s", output)
	}
	if !strings.Contains(output, "actionbutton btnSave") {
		t.Errorf("Expected 'actionbutton btnSave' in output, got: %s", output)
	}
}

func TestExecDescribeFragmentNotFound(t *testing.T) {
	var buf bytes.Buffer
	exec := New(&buf)

	err := exec.Execute(&ast.DescribeStmt{
		ObjectType: ast.DescribeFragment,
		Name:       ast.QualifiedName{Name: "Nonexistent"},
	})
	if err == nil {
		t.Fatal("Expected error for nonexistent fragment, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestShowFragmentsNoProjectRequired(t *testing.T) {
	// SHOW FRAGMENTS should work without a project connection
	var buf bytes.Buffer
	exec := New(&buf)

	err := exec.Execute(&ast.ShowStmt{ObjectType: ast.ShowFragments})
	if err != nil {
		t.Fatalf("show fragments should work without project connection, got: %v", err)
	}
}

func TestDescribeFragmentNoProjectRequired(t *testing.T) {
	// DESCRIBE FRAGMENT should work without a project connection
	var buf bytes.Buffer
	exec := New(&buf)

	exec.Execute(&ast.DefineFragmentStmt{
		Name:    "Test",
		Widgets: []*ast.WidgetV3{},
	})

	err := exec.Execute(&ast.DescribeStmt{
		ObjectType: ast.DescribeFragment,
		Name:       ast.QualifiedName{Name: "Test"},
	})
	if err != nil {
		t.Fatalf("describe fragment should work without project connection, got: %v", err)
	}
}

func TestCloneWidgets(t *testing.T) {
	original := []*ast.WidgetV3{
		{
			Type: "footer",
			Name: "f1",
			Properties: map[string]interface{}{
				"Label": "test",
			},
			Children: []*ast.WidgetV3{
				{
					Type:       "actionbutton",
					Name:       "btn1",
					Properties: map[string]interface{}{"Caption": "Save"},
					Children:   []*ast.WidgetV3{},
				},
			},
		},
	}

	cloned := cloneWidgets(original)

	// Verify it's a deep copy
	if len(cloned) != 1 {
		t.Fatalf("Expected 1 cloned widget, got %d", len(cloned))
	}
	if cloned[0] == original[0] {
		t.Error("Cloned widget should be a different pointer")
	}
	if cloned[0].Children[0] == original[0].Children[0] {
		t.Error("Cloned children should be different pointers")
	}

	// Verify values match
	if cloned[0].Name != "f1" {
		t.Errorf("Expected name 'f1', got %q", cloned[0].Name)
	}
	if cloned[0].Children[0].Name != "btn1" {
		t.Errorf("Expected child name 'btn1', got %q", cloned[0].Children[0].Name)
	}

	// Modify clone, verify original unchanged
	cloned[0].Name = "modified"
	if original[0].Name != "f1" {
		t.Error("Modifying clone should not affect original")
	}
}

func TestPrefixWidgetNames(t *testing.T) {
	widgets := []*ast.WidgetV3{
		{
			Type:       "footer",
			Name:       "f1",
			Properties: map[string]interface{}{},
			Children: []*ast.WidgetV3{
				{
					Type:       "actionbutton",
					Name:       "btnSave",
					Properties: map[string]interface{}{},
					Children:   []*ast.WidgetV3{},
				},
				{
					Type:       "actionbutton",
					Name:       "btnCancel",
					Properties: map[string]interface{}{},
					Children:   []*ast.WidgetV3{},
				},
			},
		},
	}

	prefixWidgetNames(widgets, "pfx_")

	if widgets[0].Name != "pfx_f1" {
		t.Errorf("Expected 'pfx_f1', got %q", widgets[0].Name)
	}
	if widgets[0].Children[0].Name != "pfx_btnSave" {
		t.Errorf("Expected 'pfx_btnSave', got %q", widgets[0].Children[0].Name)
	}
	if widgets[0].Children[1].Name != "pfx_btnCancel" {
		t.Errorf("Expected 'pfx_btnCancel', got %q", widgets[0].Children[1].Name)
	}
}

func TestExpandIfFragmentPassthrough(t *testing.T) {
	pb := &pageBuilder{
		fragments: nil,
	}

	w := &ast.WidgetV3{Type: "textbox", Name: "txt1", Properties: map[string]interface{}{}}
	result, err := pb.expandIfFragment(w)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != w {
		t.Error("Non-fragment widget should pass through unchanged")
	}
}

func TestExpandIfFragmentNotFound(t *testing.T) {
	pb := &pageBuilder{
		fragments: map[string]*ast.DefineFragmentStmt{},
	}

	w := &ast.WidgetV3{Type: "USE_FRAGMENT", Name: "Missing", Properties: map[string]interface{}{}}
	_, err := pb.expandIfFragment(w)
	if err == nil {
		t.Fatal("Expected error for missing fragment")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestExpandIfFragmentExpands(t *testing.T) {
	frag := &ast.DefineFragmentStmt{
		Name: "Footer",
		Widgets: []*ast.WidgetV3{
			{Type: "actionbutton", Name: "btnSave", Properties: map[string]interface{}{"Caption": "Save"}, Children: []*ast.WidgetV3{}},
			{Type: "actionbutton", Name: "btnCancel", Properties: map[string]interface{}{"Caption": "Cancel"}, Children: []*ast.WidgetV3{}},
		},
	}

	pb := &pageBuilder{
		fragments: map[string]*ast.DefineFragmentStmt{"Footer": frag},
	}

	w := &ast.WidgetV3{Type: "USE_FRAGMENT", Name: "Footer", Properties: map[string]interface{}{}}
	result, err := pb.expandIfFragment(w)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 expanded widgets, got %d", len(result))
	}
	if result[0].Name != "btnSave" {
		t.Errorf("Expected 'btnSave', got %q", result[0].Name)
	}
	if result[1].Name != "btnCancel" {
		t.Errorf("Expected 'btnCancel', got %q", result[1].Name)
	}

	// Verify cloned (not same pointer)
	if result[0] == frag.Widgets[0] {
		t.Error("Expanded widgets should be clones, not original pointers")
	}
}

func TestExpandIfFragmentWithPrefix(t *testing.T) {
	frag := &ast.DefineFragmentStmt{
		Name: "Footer",
		Widgets: []*ast.WidgetV3{
			{Type: "actionbutton", Name: "btnSave", Properties: map[string]interface{}{}, Children: []*ast.WidgetV3{}},
		},
	}

	pb := &pageBuilder{
		fragments: map[string]*ast.DefineFragmentStmt{"Footer": frag},
	}

	w := &ast.WidgetV3{
		Type:       "USE_FRAGMENT",
		Name:       "Footer",
		Properties: map[string]interface{}{"Prefix": "order_"},
	}
	result, err := pb.expandIfFragment(w)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result[0].Name != "order_btnSave" {
		t.Errorf("Expected 'order_btnSave', got %q", result[0].Name)
	}
}

func TestRoundtripDefineAndDescribe(t *testing.T) {
	input := `define fragment Footer as {
		footer f1 {
			actionbutton btnSave (Caption: 'Save', Action: save_changes, ButtonStyle: Primary)
		}
	};
	describe fragment Footer;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}

	var buf bytes.Buffer
	exec := New(&buf)

	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	}

	output := buf.String()
	if !strings.Contains(output, "define fragment Footer") {
		t.Errorf("Expected define fragment in output, got: %s", output)
	}
	if !strings.Contains(output, "footer f1") {
		t.Errorf("Expected footer f1 in output, got: %s", output)
	}
}

func TestFindRawWidgetByName(t *testing.T) {
	widgets := []rawWidget{
		{
			Type: "Forms$DivContainer",
			Name: "container1",
			Children: []rawWidget{
				{Type: "Forms$TextBox", Name: "txtName"},
				{Type: "Forms$TextBox", Name: "txtEmail"},
			},
		},
		{
			Type: "Forms$Footer",
			Name: "footer1",
			Children: []rawWidget{
				{Type: "Forms$ActionButton", Name: "btnSave"},
			},
		},
	}

	// Find top-level widget
	found := findRawWidgetByName(widgets, "container1")
	if found == nil {
		t.Fatal("Expected to find 'container1'")
	}
	if found.Name != "container1" {
		t.Errorf("Expected name 'container1', got %q", found.Name)
	}

	// Find nested widget
	found = findRawWidgetByName(widgets, "txtEmail")
	if found == nil {
		t.Fatal("Expected to find 'txtEmail'")
	}
	if found.Name != "txtEmail" {
		t.Errorf("Expected name 'txtEmail', got %q", found.Name)
	}

	// Find widget in footer
	found = findRawWidgetByName(widgets, "btnSave")
	if found == nil {
		t.Fatal("Expected to find 'btnSave'")
	}

	// Not found
	found = findRawWidgetByName(widgets, "nonexistent")
	if found != nil {
		t.Errorf("Expected nil for nonexistent widget, got %v", found)
	}
}

func TestFindRawWidgetByNameInRows(t *testing.T) {
	widgets := []rawWidget{
		{
			Type: "Forms$LayoutGrid",
			Name: "lgMain",
			Rows: []rawWidgetRow{
				{
					Columns: []rawWidgetColumn{
						{
							Width: 6,
							Widgets: []rawWidget{
								{Type: "Forms$TextBox", Name: "txtLeft"},
							},
						},
						{
							Width: 6,
							Widgets: []rawWidget{
								{Type: "Forms$TextBox", Name: "txtRight"},
							},
						},
					},
				},
			},
		},
	}

	// Find widget inside a row column
	found := findRawWidgetByName(widgets, "txtRight")
	if found == nil {
		t.Fatal("Expected to find 'txtRight' in layout grid column")
	}
	if found.Name != "txtRight" {
		t.Errorf("Expected name 'txtRight', got %q", found.Name)
	}
}

func TestFindRawWidgetByNameInFilterAndControlBar(t *testing.T) {
	widgets := []rawWidget{
		{
			Type: "Forms$Gallery",
			Name: "gallery1",
			FilterWidgets: []rawWidget{
				{Type: "Forms$TextFilter", Name: "filter1"},
			},
			ControlBar: []rawWidget{
				{Type: "Forms$ActionButton", Name: "cbBtn1"},
			},
		},
	}

	found := findRawWidgetByName(widgets, "filter1")
	if found == nil {
		t.Fatal("Expected to find 'filter1' in FilterWidgets")
	}

	found = findRawWidgetByName(widgets, "cbBtn1")
	if found == nil {
		t.Fatal("Expected to find 'cbBtn1' in ControlBar")
	}
}

func TestRoundtripShowFragments(t *testing.T) {
	input := `define fragment Alpha as { textbox t1 (Label: 'A') };
	define fragment Beta as { checkbox c1 (Label: 'B') };
	show fragments;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}

	var buf bytes.Buffer
	exec := New(&buf)

	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
	}

	output := buf.String()
	if !strings.Contains(output, "Alpha") || !strings.Contains(output, "Beta") {
		t.Errorf("Expected both fragment names in show output, got: %s", output)
	}
}
