// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestBuildingBlockDataSourceRebindTargetsUnboundWidgets guards the regression
// #941's emitter fix exposed.
//
// `use building block X (datasource: …)` finds its binding point by re-rendering
// the block to MDL and re-parsing it. It used to look for a widget that already
// carried a DataSource property, on the stated assumption that "a block's
// outermost list/grid/dataview always emits one". That was only true because of
// the bug #941 fixed: an unbound datasource — which is what a reusable Atlas
// block has, since it is a template — rendered as the malformed
// `DataSource: database from ,`. Emitting nothing for it, which is correct, left
// the rebind with nothing to match.
//
// So the target is matched by widget TYPE, exactly as the action override
// already does. That case learned this lesson first: its comment notes Atlas
// blocks "ship placeholder buttons with no action", which is the same fact about
// the same blocks.
func TestBuildingBlockDataSourceRebindTargetsUnboundWidgets(t *testing.T) {
	for _, widgetType := range []string{"gallery", "listview", "datagrid", "dataview", "DATAGRID2"} {
		t.Run(widgetType, func(t *testing.T) {
			if !isDataSourceWidget(&ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s is not recognised as a datasource-carrying widget", widgetType)
			}
		})
	}
	for _, widgetType := range []string{"container", "text", "actionbutton", "layoutgrid"} {
		t.Run("not/"+widgetType, func(t *testing.T) {
			if isDataSourceWidget(&ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s must not be treated as a datasource target", widgetType)
			}
		})
	}
}

// TestBuildingBlockRebindPrefersTheOutermost pins that the override still lands
// on the block's outermost datasource widget when the tree nests several — the
// binding-point rule the feature documents.
func TestBuildingBlockRebindPrefersTheOutermost(t *testing.T) {
	inner := &ast.WidgetV3{Type: "listview", Name: "inner"}
	outer := &ast.WidgetV3{Type: "gallery", Name: "outer", Children: []*ast.WidgetV3{inner}}
	var got string
	rebindFirst([]*ast.WidgetV3{outer}, isDataSourceWidget, func(w *ast.WidgetV3) { got = w.Name })
	if got != "outer" {
		t.Errorf("rebound %q, want the outermost datasource widget", got)
	}
}

// pluggableClickRegistry holds definitions shaped like the PlatformDesignSystem
// widgets a PDS block is built from: a button and a link that both source their
// `onClickAction` slot from OnClick, a gallery whose row click does the same, and
// a "button" whose definition maps no click slot at all.
func pluggableClickRegistry() *WidgetRegistry {
	onClick := []PropertyMapping{{PropertyKey: "onClickAction", Source: "OnClick", Operation: "action"}}
	return &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{
		"PDSBUTTON":       {WidgetID: "mendix.pdsbutton.PDSButton", MDLName: "pdsbutton", PropertyMappings: onClick},
		"PDSLINK":         {WidgetID: "mendix.pdslink.PDSLink", MDLName: "pdslink", PropertyMappings: onClick},
		"GALLERY":         {WidgetID: "com.mendix.widget.web.gallery.Gallery", MDLName: "gallery", PropertyMappings: onClick},
		"NOCLICKBUTTON":   {WidgetID: "x.noclickbutton.NoClickButton", MDLName: "noclickbutton"},
		"MODECLICKBUTTON": {WidgetID: "x.modeclickbutton.ModeClickButton", MDLName: "modeclickbutton", Modes: []WidgetMode{{Name: "default", PropertyMappings: onClick}}},
	}}
}

// TestBuildingBlockActionRebindFindsPluggableButtons: `use building block
// PlatformDesignSystem.Button (action: …)` failed with "the block has no button
// to rebind", because the PDS blocks are built from the pluggable `pdsbutton`
// and the action target matched only the built-in button types. Measured on
// mxcli v0.31.0, Mendix 11.12.6, PlatformDesignSystem 44.4.0.
func TestBuildingBlockActionRebindFindsPluggableButtons(t *testing.T) {
	reg := pluggableClickRegistry()
	for _, widgetType := range []string{"pdsbutton", "PDSBUTTON", "modeclickbutton"} {
		t.Run(widgetType, func(t *testing.T) {
			if !isActionTargetWidget(reg, &ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s is not an action target", widgetType)
			}
		})
	}
	// The built-in buttons stay targets, with or without a registry.
	for _, widgetType := range []string{"actionbutton", "linkbutton"} {
		if !isActionTargetWidget(nil, &ast.WidgetV3{Type: widgetType}) {
			t.Errorf("%s is no longer an action target", widgetType)
		}
	}
	// A click slot alone is not a button: a link click or a gallery row click is
	// not the block's primary action. Nor is a button that takes no action.
	for _, widgetType := range []string{"pdslink", "gallery", "noclickbutton", "container", "unknownbutton"} {
		t.Run("not/"+widgetType, func(t *testing.T) {
			if isActionTargetWidget(reg, &ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s must not be an action target", widgetType)
			}
		})
	}
	// No project, no definitions: only the built-in button types match, as before.
	if isActionTargetWidget(nil, &ast.WidgetV3{Type: "pdsbutton"}) {
		t.Error("with no registry, a pdsbutton must not match")
	}
}

// TestBuildingBlockActionRebindSkipsLinksBeforeTheButton pins the binding point
// on a PDS page header: its first clickable widget is a "Back to …" PDS Link, and
// the action belongs on the first button after it.
func TestBuildingBlockActionRebindSkipsLinksBeforeTheButton(t *testing.T) {
	reg := pluggableClickRegistry()
	back := &ast.WidgetV3{Type: "pdslink", Name: "pDSLink1", Properties: map[string]any{}}
	primary := &ast.WidgetV3{Type: "pdsbutton", Name: "pDSButton3", Properties: map[string]any{}}
	secondary := &ast.WidgetV3{Type: "pdsbutton", Name: "pDSButton4", Properties: map[string]any{}}
	header := &ast.WidgetV3{Type: "container", Name: "container2",
		Children: []*ast.WidgetV3{back, {Type: "container", Name: "container6", Children: []*ast.WidgetV3{primary, secondary}}}}

	act := &ast.ActionV3{}
	hit := rebindFirst([]*ast.WidgetV3{header},
		func(w *ast.WidgetV3) bool { return isActionTargetWidget(reg, w) },
		func(w *ast.WidgetV3) { w.Properties["Action"] = act })
	if !hit {
		t.Fatal("no target found")
	}
	if primary.Properties["Action"] != act {
		t.Errorf("the action did not land on the first PDS button")
	}
	if back.Properties["Action"] != nil || secondary.Properties["Action"] != nil {
		t.Errorf("the action landed on a widget other than the first PDS button")
	}
}
