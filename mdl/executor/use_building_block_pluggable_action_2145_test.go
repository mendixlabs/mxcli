// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The id's last segment is what a propertyless pluggable widget describes as,
// so `pdsbutton pDSButton1` — the line the issue quotes.
const pdsButtonWidgetID = "com.mendix.widget.pds.PDSButton"

// pdsButtonRegistry declares a pluggable button the way GenerateDefJSON
// derives one from PDS Button's package: its click action is the property
// `onClickAction`, which actionSourceForKey maps to MDL's `OnClick:` (the
// widget's Action slot). The data container beside it has a click action too,
// as the pluggable Gallery does.
func pdsButtonRegistry() *WidgetRegistry {
	button := &WidgetDefinition{
		WidgetID: pdsButtonWidgetID,
		MDLName:  "PDSBUTTON",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "caption", Source: "Caption", Operation: "texttemplate"},
			{PropertyKey: "onClickAction", Source: actionSourceForKey("onClickAction"), Operation: "action"},
		},
	}
	list := &WidgetDefinition{
		WidgetID: "com.example.PDSList",
		MDLName:  "PDSLIST",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "datasource", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "onClick", Source: "OnClick", Operation: "action"},
		},
	}
	return &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"PDSBUTTON": button, "PDSLIST": list},
		byWidgetID: map[string]*WidgetDefinition{button.WidgetID: button, list.WidgetID: list},
	}
}

func rawCustomWidget(name, widgetID string) map[string]any {
	return map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  name,
		"Type": map[string]any{
			"$Type":      "CustomWidgets$CustomWidgetType",
			"WidgetId":   widgetID,
			"ObjectType": map[string]any{"PropertyTypes": []any{int32(2)}},
		},
		"Object": map[string]any{
			"$Type":      "CustomWidgets$WidgetObject",
			"Properties": []any{int32(2)},
		},
	}
}

func pdsBlockCtx(t *testing.T, raw map[string]any) (*pageBuilder, *ExecContext) {
	t.Helper()
	mod := mkModule("PlatformDesignSystem")
	bb := mkBuildingBlock(mod.ID, "Button")
	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	ctx.widgetRegistry, ctx.widgetRegistryLoaded = pdsButtonRegistry(), true
	return &pageBuilder{ctx: ctx, backend: mb}, ctx
}

func useButtonBlock(act *ast.ActionV3) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "USE_BUILDING_BLOCK",
		Name: "PlatformDesignSystem.Button",
		Properties: map[string]any{
			"Prefix":         "b_",
			"ActionOverride": act,
		},
	}
}

// TestBuildingBlockActionOverrideRebindsPluggableButton is ako/mxcli#2145
// (mendixlabs/mxcli#1821): `use building block PlatformDesignSystem.Button
// (action: call microflow …) as b_` failed with "action override supplied, but
// the block has no button to rebind" — the block is one pluggable `pdsbutton`,
// and the target was matched against the core button keywords only.
func TestBuildingBlockActionOverrideRebindsPluggableButton(t *testing.T) {
	raw := map[string]any{"Widgets": []any{rawCustomWidget("pDSButton1", pdsButtonWidgetID)}}
	pb, _ := pdsBlockCtx(t, raw)
	act := &ast.ActionV3{Type: "microflow", Target: "PDSRepro.ACT_Noop"}

	got, err := pb.expandIfFragment(useButtonBlock(act))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	btn := findWidget(got, "b_pDSButton1")
	if btn == nil {
		t.Fatalf("pluggable button not expanded: %+v", got)
	}
	if !strings.EqualFold(btn.Type, "pdsbutton") {
		t.Fatalf("expanded as %q, want pdsbutton", btn.Type)
	}
	// `OnClick:` is the widget's Action slot; the engine reads it into the
	// mapping whose Source is OnClick — onClickAction here.
	if btn.GetAction() != act {
		t.Errorf("override not bound to the button's click action: %+v", btn.Properties)
	}
}

// TestBuildingBlockActionOverrideSkipsPluggableDataContainer: a data
// container's click action (a Gallery's onClick) is an item action, not the
// block's primary action, so the first button-like widget after it wins.
func TestBuildingBlockActionOverrideSkipsPluggableDataContainer(t *testing.T) {
	raw := map[string]any{"Widgets": []any{
		rawCustomWidget("list1", "com.example.PDSList"),
		rawCustomWidget("pDSButton1", pdsButtonWidgetID),
	}}
	pb, _ := pdsBlockCtx(t, raw)
	act := &ast.ActionV3{Type: "microflow", Target: "PDSRepro.ACT_Noop"}
	got, err := pb.expandIfFragment(useButtonBlock(act))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if l := findWidget(got, "b_list1"); l == nil || l.GetAction() != nil {
		t.Errorf("data container took the override: %+v", l)
	}
	if b := findWidget(got, "b_pDSButton1"); b == nil || b.GetAction() != act {
		t.Errorf("button did not get the override: %+v", b)
	}
}

// TestBuildingBlockActionOverridePrefersCoreButton keeps the existing contract:
// a block with a core button binds it, even when a clickable pluggable widget
// comes first (Atlas's Card_ActionWithImage: an `image`, then the button).
func TestBuildingBlockActionOverridePrefersCoreButton(t *testing.T) {
	raw := map[string]any{"Widgets": []any{
		rawCustomWidget("pDSButton1", pdsButtonWidgetID),
		map[string]any{"$Type": "Forms$ActionButton", "Name": "actionButton1"},
	}}
	pb, _ := pdsBlockCtx(t, raw)
	act := &ast.ActionV3{Type: "microflow", Target: "PDSRepro.ACT_Noop"}
	got, err := pb.expandIfFragment(useButtonBlock(act))
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if b := findWidget(got, "b_actionButton1"); b == nil || b.GetAction() != act {
		t.Errorf("core button did not get the override: %+v", b)
	}
	if b := findWidget(got, "b_pDSButton1"); b == nil || b.GetAction() != nil {
		t.Errorf("pluggable widget took the override from a core button: %+v", b)
	}
}

// TestBuildingBlockActionOverrideStillRefusesWhenNothingClickable: a block
// with neither a button nor a widget with a click action is still refused.
func TestBuildingBlockActionOverrideStillRefusesWhenNothingClickable(t *testing.T) {
	pb, _ := pdsBlockCtx(t, bbRawWidgets())
	_, err := pb.expandIfFragment(useButtonBlock(&ast.ActionV3{Type: "microflow", Target: "M.F"}))
	if err == nil || !strings.Contains(err.Error(), "no button to rebind") {
		t.Errorf("want the no-button refusal, got %v", err)
	}
}
