// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2183 (mendixlabs/mxcli#1836): two refusals meant for hand-written
// MDL also fired on widgets `use building block` copies in, so one widget the
// author never wrote blocked the whole block.
//
//   - PlatformDesignSystem.Icon_After_Text carries a dynamic text styled
//     `--icon-size: auto;`, and the inline-style refusal on a dynamictext
//     (MDL-WIDGET03, #673) stopped the block.
//   - PlatformDesignSystem.Avatar_Group_SM carries a list view with no data
//     source configured, which the writer refused ("ListView source <nil>").
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// rawStyledDynamicText is the Icon_After_Text block's text as Studio Pro
// stores it: an inline Style on the dynamic text's own Appearance.
func rawStyledDynamicText(name, style string) map[string]any {
	return map[string]any{
		"$Type":      "Forms$DynamicText",
		"Name":       name,
		"Appearance": map[string]any{"Class": "", "Style": style},
		"Content": map[string]any{
			"Template": map[string]any{
				"Items": []any{map[string]any{"Text": "Label", "LanguageCode": "en_US"}},
			},
		},
	}
}

// blockCtx serves one PlatformDesignSystem block whose widgets are raw, and
// returns the builder and the output the builder warns on.
func blockCtx(t *testing.T, blockName string, widgets ...any) (*pageBuilder, *bytes.Buffer) {
	t.Helper()
	mod := mkModule("PlatformDesignSystem")
	bb := mkBuildingBlock(mod.ID, blockName)
	h := mkHierarchy(mod)
	withContainer(h, bb.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBuildingBlocksFunc: func() ([]*pages.BuildingBlock, error) {
			return []*pages.BuildingBlock{bb}, nil
		},
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return map[string]any{"Widgets": widgets}, nil
		},
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{mkDomainModel(mod.ID, mkEntity(mod.ID, "E"))}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return &pageBuilder{
		ctx:              ctx,
		backend:          mb,
		widgetScope:      map[string]model.ID{},
		paramEntityNames: map[string]string{},
	}, buf
}

func useBlock(name string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:       "USE_BUILDING_BLOCK",
		Name:       "PlatformDesignSystem." + name,
		Properties: map[string]any{"Prefix": "b_"},
	}
}

// expandAndBuild expands the block and builds each top-level widget, as the
// page builder does.
func expandAndBuild(t *testing.T, pb *pageBuilder, block string) ([]*ast.WidgetV3, []pages.Widget, error) {
	t.Helper()
	ws, err := pb.expandFragments([]*ast.WidgetV3{useBlock(block)})
	if err != nil {
		t.Fatalf("expand %s: %v", block, err)
	}
	var built []pages.Widget
	for _, w := range ws {
		b, err := pb.buildWidgetV3(w)
		if err != nil {
			return ws, built, err
		}
		built = append(built, b)
	}
	return ws, built, nil
}

// TestBuildingBlockCarriesDynamicTextStyle: the block's own style is placed as
// Studio Pro stores it, not refused.
func TestBuildingBlockCarriesDynamicTextStyle(t *testing.T) {
	pb, _ := blockCtx(t, "Icon_After_Text", rawStyledDynamicText("text13", "--icon-size: auto;"))
	_, built, err := expandAndBuild(t, pb, "Icon_After_Text")
	if err != nil {
		t.Fatalf("use building block Icon_After_Text: %v", err)
	}
	dt, ok := built[0].(*pages.DynamicText)
	if !ok {
		t.Fatalf("built %T, want *pages.DynamicText", built[0])
	}
	if dt.Name != "b_text13" {
		t.Errorf("name = %q, want b_text13", dt.Name)
	}
	if dt.Style != "--icon-size: auto;" {
		t.Errorf("Style = %q, want the block's stored --icon-size: auto;", dt.Style)
	}
}

// Control: a hand-written dynamictext with an inline style is still refused
// (MDL-WIDGET03); only the block's stored value is carried.
func TestHandWrittenDynamicTextStyleStillRefused(t *testing.T) {
	pb, _ := blockCtx(t, "Unused")
	w := &ast.WidgetV3{
		Type:       "DYNAMICTEXT",
		Name:       "dt",
		Properties: map[string]any{"Content": "x", "Style": "color: red;"},
	}
	_, err := pb.buildWidgetV3(w)
	if err == nil || !strings.Contains(err.Error(), "an inline `style` crashes MxBuild") {
		t.Errorf("want the inline-style refusal, got %v", err)
	}
}
