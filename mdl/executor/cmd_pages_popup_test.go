// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func newPopupPageBuilder() *pageBuilder {
	return &pageBuilder{
		moduleID:         model.ID("mod1"),
		moduleName:       "M",
		widgetScope:      map[string]model.ID{},
		paramScope:       map[string]model.ID{},
		paramEntityNames: map[string]string{},
	}
}

// Issue #661 — buildPageV3 carries pop-up dimensions from the header onto the
// page struct (which both writers serialize).
func TestBuildPageV3_PopupDimensions(t *testing.T) {
	w, h, r := 800, 480, true
	s := &ast.CreatePageStmtV3{
		Name:           ast.QualifiedName{Module: "M", Name: "P"},
		PopupWidth:     &w,
		PopupHeight:    &h,
		PopupResizable: &r,
	}
	page, err := newPopupPageBuilder().buildPageV3(s)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	if page.PopupWidth != 800 || page.PopupHeight != 480 || !page.PopupResizable {
		t.Errorf("got %d/%d/%v, want 800/480/true", page.PopupWidth, page.PopupHeight, page.PopupResizable)
	}
}

// When the header omits pop-up properties the Studio Pro default applies: 0/0
// (auto-size), matching what Studio Pro itself stores (issue #713).
func TestBuildPageV3_PopupDefaults(t *testing.T) {
	s := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "M", Name: "P"}}
	page, err := newPopupPageBuilder().buildPageV3(s)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	if page.PopupWidth != 0 || page.PopupHeight != 0 || page.PopupResizable {
		t.Errorf("got %d/%d/%v, want 0/0/false", page.PopupWidth, page.PopupHeight, page.PopupResizable)
	}
}

// Issue #650 — DYNAMICTEXT (Attribute: X) must bind the template parameter, not
// leave an orphaned {1}. buildDynamicTextV3 should produce exactly one bound
// ClientTemplateParameter.
func TestBuildDynamicTextV3_AttributeBinds(t *testing.T) {
	pb := newPopupPageBuilder()
	w := &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: map[string]any{"Attribute": "Title"}}
	dt, err := pb.buildDynamicTextV3(w)
	if err != nil {
		t.Fatalf("buildDynamicTextV3: %v", err)
	}
	if dt.Content == nil {
		t.Fatal("Content template is nil")
	}
	if got := dt.Content.Template.GetTranslation("en_US"); got != "{1}" {
		t.Errorf("template = %q, want {1}", got)
	}
	if len(dt.Content.Parameters) != 1 {
		t.Fatalf("expected 1 bound parameter, got %d (orphaned placeholder)", len(dt.Content.Parameters))
	}
	p := dt.Content.Parameters[0]
	if p.AttributeRef == "" && p.Expression == "" && p.SourceVariable == "" {
		t.Error("parameter has no binding (AttributeRef/Expression/SourceVariable all empty)")
	}
}

// A content-less dynamictext with no binding is unchanged (no panic, no params).
func TestBuildDynamicTextV3_StaticContent(t *testing.T) {
	pb := newPopupPageBuilder()
	w := &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: map[string]any{"Content": "Hello"}}
	dt, err := pb.buildDynamicTextV3(w)
	if err != nil {
		t.Fatalf("buildDynamicTextV3: %v", err)
	}
	if got := dt.Content.Template.GetTranslation("en_US"); got != "Hello" {
		t.Errorf("template = %q, want Hello", got)
	}
	if len(dt.Content.Parameters) != 0 {
		t.Errorf("expected 0 params for static text, got %d", len(dt.Content.Parameters))
	}
}
