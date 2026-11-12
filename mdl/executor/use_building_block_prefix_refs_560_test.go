// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// masterDetailRawWidgets is the shape of Atlas_Web_Content.Master_Detail that
// matters for ako/mxcli#560: a list widget and a data view listening to its
// selection by NAME (Forms$ListenTargetSource.ListenTarget).
func masterDetailRawWidgets() map[string]any {
	return map[string]any{
		"Widgets": []any{
			map[string]any{
				"$Type": "Forms$DivContainer",
				"Name":  "container1",
				"Widgets": []any{
					map[string]any{
						"$Type": "Forms$ListView",
						"Name":  "listView2",
					},
					map[string]any{
						"$Type": "Forms$DataView",
						"Name":  "dataView7",
						"DataSource": map[string]any{
							"$Type":        "Forms$ListenTargetSource",
							"ListenTarget": "listView2",
						},
					},
				},
			},
		},
	}
}

func findWidget(ws []*ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, w := range ws {
		if w.Name == name {
			return w
		}
		if f := findWidget(w.Children, name); f != nil {
			return f
		}
	}
	return nil
}

// TestBuildingBlockPrefixRewritesSelectionReference is ako/mxcli#560: copied
// with `as md_`, the block's widgets became md_listView2 / md_dataView7 but the
// data view's `selection listView2` kept the old name, and exec failed with
// "failed to build datasource: widget not found: listView2".
func TestBuildingBlockPrefixRewritesSelectionReference(t *testing.T) {
	mod := mkModule("Atlas_Web_Content")
	bb := mkBuildingBlock(mod.ID, "Master_Detail")
	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return masterDetailRawWidgets(), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	pb := &pageBuilder{ctx: ctx, backend: mb}

	got, err := pb.expandIfFragment(&ast.WidgetV3{
		Type:       "USE_BUILDING_BLOCK",
		Name:       "Atlas_Web_Content.Master_Detail",
		Properties: map[string]any{"Prefix": "md_"},
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if findWidget(got, "md_listView2") == nil {
		t.Fatalf("list widget not prefixed: %+v", got)
	}
	dv := findWidget(got, "md_dataView7")
	if dv == nil {
		t.Fatalf("data view not prefixed")
	}
	ds := dv.GetDataSource()
	if ds == nil || ds.Type != "selection" {
		t.Fatalf("data view lost its selection datasource: %+v", ds)
	}
	if ds.Reference != "md_listView2" {
		t.Errorf("selection reference = %q, want md_listView2 (widget not found: %s at build time)", ds.Reference, ds.Reference)
	}
}

// TestPrefixRenamesEveryIntraBlockWidgetReference covers the other places a
// widget name is written inside a block, and that a name the block does not
// declare — a widget elsewhere on the page — is left alone.
func TestPrefixRenamesEveryIntraBlockWidgetReference(t *testing.T) {
	src := `define fragment F as {
		gallery gal1 (Selection: Single)
		dataview dv1 (DataSource: selection gal1) {
			dynamictext t1 (Content: '{1} / {2}', ContentParams: [{1} = $dv1.Name, {2} = $outside.Name])
			actionbutton b1 (Caption: 'Del', Action: delete $gal1)
			actionbutton b2 (Caption: 'Go', Action: call microflow M.Go(Item = $gal1, Other = $outsideGrid))
			actionbutton b3 (Caption: 'Del other', Action: delete $outsideGrid)
			textbox tb1 (Attribute: Name, Visible: $dv1/Name != empty)
		}
		dataview dv2 (DataSource: selection outsideGrid)
	};`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	widgets := prog.Statements[0].(*ast.DefineFragmentStmt).Widgets

	prefixWidgetNames(widgets, "p_")

	if ds := findWidget(widgets, "p_dv1").GetDataSource(); ds == nil || ds.Reference != "p_gal1" {
		t.Errorf("selection reference not renamed: %+v", ds)
	}
	if ds := findWidget(widgets, "p_dv2").GetDataSource(); ds == nil || ds.Reference != "outsideGrid" {
		t.Errorf("selection of a widget outside the block was renamed: %+v", ds)
	}
	if a := findWidget(widgets, "p_b1").GetAction(); a == nil || a.SourceVariable != "p_gal1" {
		t.Errorf("delete $widget not renamed: %+v", a)
	}
	if a := findWidget(widgets, "p_b3").GetAction(); a == nil || a.SourceVariable != "outsideGrid" {
		t.Errorf("delete of an outside widget was renamed: %+v", a)
	}
	b2 := findWidget(widgets, "p_b2").GetAction()
	if b2 == nil || len(b2.Args) != 2 {
		t.Fatalf("call args lost: %+v", b2)
	}
	if v, _ := b2.Args[0].Value.(string); v != "$p_gal1" {
		t.Errorf("flow argument $widget not renamed: %q", v)
	}
	if v, _ := b2.Args[1].Value.(string); v != "$outsideGrid" {
		t.Errorf("flow argument naming an outside widget renamed: %q", v)
	}
	params, _ := findWidget(widgets, "p_t1").Properties["ContentParams"].([]ast.ParamAssignmentV3)
	if len(params) != 2 {
		t.Fatalf("content params: %#v", findWidget(widgets, "p_t1").Properties["ContentParams"])
	}
	if v, _ := params[0].Value.(string); v != "$p_dv1.Name" {
		t.Errorf("template parameter $widget.Attr not renamed: %q", v)
	}
	if v, _ := params[1].Value.(string); v != "$outside.Name" {
		t.Errorf("template parameter of an outside widget renamed: %q", v)
	}
	if vis := findWidget(widgets, "p_tb1").Properties["VisibleIf"]; !valueCarriesText(vis, "$p_dv1/Name") {
		t.Errorf("visibility expression $widget/Attr not renamed: %#v", vis)
	}
}

// TestPrefixLeavesTheFragmentDefinitionAlone: `use fragment X as p_` clones a
// definition that later uses reuse. Renaming references must not write
// through the shallow clone into the definition's shared property values.
func TestPrefixLeavesTheFragmentDefinitionAlone(t *testing.T) {
	src := `define fragment F as {
		listview lv1
		dataview dv1 (DataSource: selection lv1)
	};`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	def := prog.Statements[0].(*ast.DefineFragmentStmt)
	pb := &pageBuilder{fragments: map[string]*ast.DefineFragmentStmt{"F": def}}

	first, err := pb.expandIfFragment(&ast.WidgetV3{Type: "USE_FRAGMENT", Name: "F", Properties: map[string]any{"Prefix": "a_"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := pb.expandIfFragment(&ast.WidgetV3{Type: "USE_FRAGMENT", Name: "F", Properties: map[string]any{"Prefix": "b_"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := findWidget(first, "a_dv1").GetDataSource().Reference; got != "a_lv1" {
		t.Errorf("first copy: selection = %q, want a_lv1", got)
	}
	if got := findWidget(second, "b_dv1").GetDataSource().Reference; got != "b_lv1" {
		t.Errorf("second copy: selection = %q, want b_lv1", got)
	}
	if got := findWidget(def.Widgets, "dv1").GetDataSource().Reference; got != "lv1" {
		t.Errorf("definition mutated: selection = %q, want lv1", got)
	}
}

// valueCarriesText reports whether a property value, whatever shape the visitor
// stores it in, carries text.
func valueCarriesText(v any, text string) bool {
	return strings.Contains(fmt.Sprintf("%+v", v), text)
}
