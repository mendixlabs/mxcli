// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#122: a control-bar button opening a page with the grid's
// selection had no spelling. `$currentObject` is unbound there (MDL-BUTTON01,
// CE1571), no argument is CE1571, and `$dgOrders` — what MDL-BUTTON01's own hint
// suggests — was refused as MDL-PAGEARG01 "a widget's page argument is always
// the enclosing context object". Studio Pro binds it as an explicit mapping
// whose PageVariable names the grid in its Widget slot; mxbuild 11.12.5 builds
// that at 0 errors.

func controlBarPage(t *testing.T, arg string) []*ast.WidgetV3 {
	t.Helper()
	src := `create page Mod.Overview (title: 'O', layout: Atlas_Core.Atlas_Default) {
  datagrid dgOrders (datasource: database from Mod.Order, selection: single) {
    controlbar cb {
      actionbutton btnView (caption: 'View', action: show page Mod.Detail(OrderParam = ` + arg + `))
    }
    column colNum (attribute: OrderNumber, caption: 'Order')
  }
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s.Widgets
}

func pageArgViolations(t *testing.T, tree []*ast.WidgetV3) []string {
	t.Helper()
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("LoadWidgetRegistry returned nil")
	}
	var out []string
	for _, v := range validateDocumentWidgetTree(tree, registry, "page Mod.Overview", nil) {
		if v.RuleID == "MDL-PAGEARG01" {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestValidateShowPageArguments_ControlBarPassesGridSelection(t *testing.T) {
	if got := pageArgViolations(t, controlBarPage(t, "$dgOrders")); len(got) != 0 {
		t.Errorf("control-bar show page passing its grid's selection refused: %q", got)
	}
	if got := pageArgViolations(t, controlBarPage(t, "$DGORDERS")); len(got) != 0 {
		t.Errorf("widget names are case-insensitive: %q", got)
	}
	// Control: another widget's name is still a variable that binds nothing.
	got := pageArgViolations(t, controlBarPage(t, "$dgOther"))
	if len(got) != 1 {
		t.Fatalf("control-bar show page passing $dgOther: %d MDL-PAGEARG01, want 1", len(got))
	}
	// And the refusal in a control bar names the spelling that works.
	if !strings.Contains(got[0], "$dgOrders") {
		t.Errorf("refusal should offer the grid's selection $dgOrders, got: %s", got[0])
	}
}

// A column button passing its own selectable grid's name passes the grid's
// selection too: Studio Pro stores Variable {Widget: grid1} there
// (FactoryManagement MxModelReflection.MxObjectType_Select, which builds), and
// so does the builder now, at 0 errors on 11.16 (ako/mxcli#2109). This used to
// be refused as an unmeasured shape. A grid without a selection still has no
// `$grid` to pass (TestCheck_GridSelectionFromColumnContent).
func TestValidateShowPageArguments_GridSelectionOnlyInItsControlBar(t *testing.T) {
	src := `create page Mod.Overview (title: 'O', layout: Atlas_Core.Atlas_Default) {
  datagrid dgOrders (datasource: database from Mod.Order, selection: single) {
    column colNum (attribute: OrderNumber, caption: 'Order') {
      actionbutton btnView (caption: 'View', action: show page Mod.Detail(OrderParam = $dgOrders))
    }
  }
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s := prog.Statements[0].(*ast.CreatePageStmtV3)
	if got := pageArgViolations(t, s.Widgets); len(got) != 0 {
		t.Errorf("row-scoped button passing its selectable grid's selection $dgOrders: %d MDL-PAGEARG01, want 0", len(got))
	}
}

// The exec half: the builder writes the selection as a "widget" mapping.
func TestBuildShowPage_ControlBarSelectionIsAWidgetMapping(t *testing.T) {
	build := func(ctx pageArgContext, arg string) (*pages.PageClientAction, error) {
		pb := &pageBuilder{
			execCache: &executorCache{createdPages: map[string]*createdPageInfo{"Mod.Detail": {ID: "detail-id"}}},
			argCtx:    ctx,
		}
		built, err := pb.buildClientActionV3(showPageButton(arg).GetAction())
		if err != nil {
			return nil, err
		}
		return built.(*pages.PageClientAction), nil
	}
	grid := enteringDataWidget(&ast.DataSourceV3{Type: "database", Reference: "Mod.Order"}, "Mod.Order").inDataWidget("dgOrders")
	bar := argContextForChildren(&ast.WidgetV3{Type: "controlbar", Name: "cb"}, grid)

	act, err := build(bar, "$dgOrders")
	if err != nil {
		t.Fatalf("control-bar show page passing $dgOrders refused: %v", err)
	}
	if len(act.ParameterMappings) != 1 {
		t.Fatalf("mappings = %d, want 1", len(act.ParameterMappings))
	}
	if m := act.ParameterMappings[0]; m.Variable != "$dgOrders" || m.VariableKind != "widget" {
		t.Errorf("mapping = %+v, want Car <- $dgOrders as widget", m)
	}

	// Controls: outside the control bar, and another name inside it.
	if _, err := build(grid, "$dgOrders"); err == nil {
		t.Error("$dgOrders from a grid row was accepted; only the control bar has the selection")
	}
	if _, err := build(bar, "$dgOther"); err == nil {
		t.Error("$dgOther in dgOrders' control bar was accepted; it binds nothing")
	}
}

// The read half: DESCRIBE prints the selection mapping back as `$dgOrders`. It
// read only the PageParameter / SnippetParameter / LocalVariable slots, so the
// mapping described as absent.
func TestDescribeShowPage_SelectionMapping(t *testing.T) {
	ctx, _ := newMockCtx(t)
	settings := map[string]any{
		"ParameterMappings": []any{int32(2), map[string]any{
			"$Type":     "Forms$PageParameterMapping",
			"Parameter": "Mod.Detail.OrderParam",
			"Argument":  "",
			"Variable": map[string]any{
				"$Type": "Forms$PageVariable", "Widget": "dgOrders",
				"PageParameter": "", "SnippetParameter": "", "LocalVariable": "",
			},
		}},
	}
	if got := extractPageParameters(ctx, settings); got != "OrderParam = $dgOrders" {
		t.Errorf("extractPageParameters = %q, want %q", got, "OrderParam = $dgOrders")
	}
}

// Through the engine: a Data grid 2's control bar is a widgets slot whose
// children the engine builds directly, never through the `controlbar` node, so
// the selection has to be put in scope there. The builder test above passed
// while exec still refused `$dgOrders` for exactly that reason.
type slotRecorder struct {
	backend.WidgetObjectBuilder
	children map[string][]pages.Widget
}

func (r *slotRecorder) SetChildWidgets(key string, ws []pages.Widget) { r.children[key] = ws }

func TestApplyChildSlots_ControlBarHoldsTheGridSelection(t *testing.T) {
	def := datagridLikeDef()
	build := func(arg string) (*slotRecorder, error) {
		pb := &pageBuilder{
			execCache:   &executorCache{createdPages: map[string]*createdPageInfo{"Mod.Detail": {ID: "detail-id"}}},
			widgetScope: map[string]model.ID{},
			// The grid's own context, as the engine's datasource pass sets it.
			argCtx: enteringDataWidget(&ast.DataSourceV3{Type: "database", Reference: "Mod.Order"}, "Mod.Order").inDataWidget("dgOrders"),
		}
		e := &PluggableWidgetEngine{pageBuilder: pb}
		rec := &slotRecorder{children: map[string][]pages.Widget{}}
		grid := controlBarPage(t, arg)[0]
		var bar []*ast.WidgetV3
		for _, c := range grid.Children {
			if strings.EqualFold(c.Type, "controlbar") {
				bar = append(bar, c)
			}
		}
		grid.Children = bar
		return rec, e.applyChildSlots(rec, def.ChildSlots, grid, nil, nil)
	}
	rec, err := build("$dgOrders")
	if err != nil {
		t.Fatalf("control-bar show page passing $dgOrders refused by exec: %v", err)
	}
	if len(rec.children["filtersPlaceholder"]) != 1 {
		t.Fatalf("control bar slot = %v, want the one button", rec.children)
	}
	if _, err := build("$dgOther"); err == nil || !strings.Contains(err.Error(), "MDL-PAGEARG01") {
		t.Errorf("control-bar show page passing $dgOther: err = %v, want MDL-PAGEARG01", err)
	}
}
