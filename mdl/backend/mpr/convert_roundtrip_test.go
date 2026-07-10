// SPDX-License-Identifier: Apache-2.0

// convert_roundtrip_test.go — internal tests that exercise the actual
// convert/unconvert functions with fully-populated structs to ensure
// round-trip correctness and full field preservation.
package mprbackend

import (
	"errors"
	"reflect"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
	"go.mongodb.org/mongo-driver/bson"
)

var errTest = errors.New("test error")

// ---------------------------------------------------------------------------
// Forward conversions: sdk/mpr -> mdl/types
// ---------------------------------------------------------------------------

func TestConvertFolderInfoSlice(t *testing.T) {
	in := []*mpr.FolderInfo{
		{ID: model.ID("f1"), ContainerID: model.ID("c1"), Name: "Folder1"},
		{ID: model.ID("f2"), ContainerID: model.ID("c2"), Name: "Folder2"},
	}
	out, err := convertFolderInfoSlice(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].ID != "f1" || out[0].Name != "Folder1" {
		t.Errorf("field mismatch on [0]: %+v", out[0])
	}
	if out[1].ID != "f2" || out[1].ContainerID != "c2" {
		t.Errorf("field mismatch on [1]: %+v", out[1])
	}
}

func TestConvertFolderInfoSlice_ErrorPassthrough(t *testing.T) {
	_, err := convertFolderInfoSlice(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertFolderInfoSlice_Nil(t *testing.T) {
	out, err := convertFolderInfoSlice(nil, nil)
	if err != nil || out != nil {
		t.Errorf("expected (nil, nil), got (%v, %v)", out, err)
	}
}

func TestConvertUnitInfoSlice(t *testing.T) {
	in := []*mpr.UnitInfo{
		{ID: model.ID("u1"), ContainerID: model.ID("c1"), ContainmentName: "units", Type: "Pages$Page"},
	}
	out, err := convertUnitInfoSlice(in, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out[0].ContainmentName != "units" || out[0].Type != "Pages$Page" {
		t.Errorf("field mismatch: %+v", out[0])
	}
}

func TestConvertRenameHitSlice(t *testing.T) {
	in := []mpr.RenameHit{
		{UnitID: "u1", UnitType: "Page", Name: "MyPage", Count: 3},
	}
	out, err := convertRenameHitSlice(in, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out[0].Count != 3 || out[0].Name != "MyPage" {
		t.Errorf("field mismatch: %+v", out[0])
	}
}

func TestConvertRawUnitSlice(t *testing.T) {
	in := []*mpr.RawUnit{
		{ID: model.ID("r1"), ContainerID: model.ID("c1"), Type: "Page", Contents: []byte{0x01}},
	}
	out, err := convertRawUnitSlice(in, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if len(out[0].Contents) != 1 || out[0].Contents[0] != 0x01 {
		t.Errorf("contents mismatch: %+v", out[0])
	}
}

func TestConvertRawUnitInfoSlice(t *testing.T) {
	in := []*mpr.RawUnitInfo{
		{ID: "r1", QualifiedName: "Mod.Page", Type: "Page", ModuleName: "Mod", Contents: []byte{0x02}},
	}
	out, err := convertRawUnitInfoSlice(in, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out[0].QualifiedName != "Mod.Page" || out[0].ModuleName != "Mod" {
		t.Errorf("field mismatch: %+v", out[0])
	}
}

func TestConvertRawUnitInfoPtr(t *testing.T) {
	in := &mpr.RawUnitInfo{ID: "r1", QualifiedName: "Q", Type: "T", ModuleName: "M", Contents: []byte{0x03}}
	out, err := convertRawUnitInfoPtr(in, nil)
	if err != nil || out == nil {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out.ID != "r1" {
		t.Errorf("field mismatch: %+v", out)
	}
}

func TestConvertRawCustomWidgetTypePtr(t *testing.T) {
	in := &mpr.RawCustomWidgetType{
		WidgetID: "w1", RawType: bson.D{{Key: "k", Value: "v"}}, RawObject: bson.D{{Key: "k2", Value: "v2"}},
		UnitID: "u1", UnitName: "Unit", WidgetName: "Widget",
	}
	out, err := convertRawCustomWidgetTypePtr(in, nil)
	if err != nil || out == nil {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out.WidgetID != "w1" || out.WidgetName != "Widget" {
		t.Errorf("field mismatch: %+v", out)
	}
}

func TestConvertRawCustomWidgetTypeSlice(t *testing.T) {
	in := []*mpr.RawCustomWidgetType{
		{WidgetID: "w1", UnitName: "U1"},
		{WidgetID: "w2", UnitName: "U2"},
	}
	out, err := convertRawCustomWidgetTypeSlice(in, nil)
	if err != nil || len(out) != 2 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertJavaActionSlice(t *testing.T) {
	in := []*mpr.JavaAction{
		{BaseElement: model.BaseElement{ID: model.ID("j1")}, ContainerID: model.ID("c1"), Name: "MyJA", Documentation: "doc"},
	}
	out, err := convertJavaActionSlice(in, nil)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected: out=%v err=%v", out, err)
	}
	if out[0].Name != "MyJA" || out[0].Documentation != "doc" {
		t.Errorf("field mismatch: %+v", out[0])
	}
}

func TestConvertJavaScriptAction_AllFields(t *testing.T) {
	in := &mpr.JavaScriptAction{
		BaseElement:             model.BaseElement{ID: model.ID("jsa1")},
		ContainerID:             model.ID("c1"),
		Name:                    "MyJSA",
		Documentation:           "doc",
		Platform:                "web",
		Excluded:                true,
		ExportLevel:             "Hidden",
		ActionDefaultReturnName: "result",
	}
	out := convertJavaScriptAction(in)
	if out.Name != "MyJSA" || out.Platform != "web" || !out.Excluded ||
		out.ExportLevel != "Hidden" || out.ActionDefaultReturnName != "result" {
		t.Errorf("field mismatch: %+v", out)
	}
}

func TestConvertJavaScriptActionSlice(t *testing.T) {
	in := []*mpr.JavaScriptAction{
		{BaseElement: model.BaseElement{ID: model.ID("jsa1")}, Name: "JSA1"},
	}
	out, err := convertJavaScriptActionSlice(in, nil)
	if err != nil || len(out) != 1 || out[0].Name != "JSA1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertJavaScriptActionSlice_ErrorPassthrough(t *testing.T) {
	_, err := convertJavaScriptActionSlice(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertJavaScriptActionPtr(t *testing.T) {
	in := &mpr.JavaScriptAction{BaseElement: model.BaseElement{ID: model.ID("jsa1")}, Name: "JSA1"}
	out, err := convertJavaScriptActionPtr(in, nil)
	if err != nil || out == nil || out.Name != "JSA1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertJavaScriptActionPtr_ErrorPassthrough(t *testing.T) {
	_, err := convertJavaScriptActionPtr(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertNavDoc_FullyPopulated(t *testing.T) {
	in := &mpr.NavigationDocument{
		BaseElement: model.BaseElement{ID: model.ID("nd1")},
		ContainerID: model.ID("c1"),
		Name:        "Navigation",
		Profiles: []*mpr.NavigationProfile{
			{
				Name: "Responsive", Kind: "Responsive", IsNative: false,
				LoginPage: "Login", NotFoundPage: "NotFound",
				HomePage: &mpr.NavHomePage{Page: "Home.Page", Microflow: "Home.MF"},
				RoleBasedHomePages: []*mpr.NavRoleBasedHome{
					{UserRole: "Admin", Page: "Admin.Home", Microflow: "Admin.MF"},
				},
				MenuItems: []*mpr.NavMenuItem{
					{Caption: "Top", Page: "P1", ActionType: "OpenPage", Items: []*mpr.NavMenuItem{
						{Caption: "Sub", Microflow: "MF1"},
					}},
				},
				OfflineEntities: []*mpr.NavOfflineEntity{
					{Entity: "Mod.Entity", SyncMode: "FullSync", Constraint: "[Amount > 0]"},
				},
			},
		},
	}

	out := convertNavDoc(in)

	if out.ID != "nd1" || out.Name != "Navigation" || out.ContainerID != "c1" {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(out.Profiles))
	}

	p := out.Profiles[0]
	if p.Name != "Responsive" || p.Kind != "Responsive" || p.IsNative {
		t.Errorf("profile basic mismatch: %+v", p)
	}
	if p.LoginPage != "Login" || p.NotFoundPage != "NotFound" {
		t.Errorf("profile page mismatch: login=%q notfound=%q", p.LoginPage, p.NotFoundPage)
	}
	if p.HomePage == nil || p.HomePage.Page != "Home.Page" || p.HomePage.Microflow != "Home.MF" {
		t.Errorf("homepage mismatch: %+v", p.HomePage)
	}
	if len(p.RoleBasedHomePages) != 1 || p.RoleBasedHomePages[0].UserRole != "Admin" {
		t.Errorf("role-based mismatch: %+v", p.RoleBasedHomePages)
	}
	if len(p.MenuItems) != 1 || p.MenuItems[0].Caption != "Top" {
		t.Errorf("menu mismatch: %+v", p.MenuItems)
	}
	if len(p.MenuItems[0].Items) != 1 || p.MenuItems[0].Items[0].Caption != "Sub" {
		t.Errorf("sub-menu mismatch: %+v", p.MenuItems[0].Items)
	}
	if len(p.OfflineEntities) != 1 || p.OfflineEntities[0].Constraint != "[Amount > 0]" {
		t.Errorf("offline entities mismatch: %+v", p.OfflineEntities)
	}
}

func TestConvertNavDocSlice(t *testing.T) {
	in := []*mpr.NavigationDocument{
		{BaseElement: model.BaseElement{ID: model.ID("nd1")}, Name: "Nav1"},
	}
	out, err := convertNavDocSlice(in, nil)
	if err != nil || len(out) != 1 || out[0].Name != "Nav1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertNavDocSlice_ErrorPassthrough(t *testing.T) {
	_, err := convertNavDocSlice(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertNavDocPtr(t *testing.T) {
	in := &mpr.NavigationDocument{BaseElement: model.BaseElement{ID: model.ID("nd1")}, Name: "Nav1"}
	out, err := convertNavDocPtr(in, nil)
	if err != nil || out == nil || out.Name != "Nav1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertNavDocPtr_ErrorPassthrough(t *testing.T) {
	_, err := convertNavDocPtr(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertJsonStructure_Recursive(t *testing.T) {
	in := &mpr.JsonStructure{
		BaseElement:   model.BaseElement{ID: model.ID("js1")},
		ContainerID:   model.ID("c1"),
		Name:          "MyJSON",
		Documentation: "doc",
		JsonSnippet:   `{"a":1}`,
		Excluded:      true,
		ExportLevel:   "Hidden",
		Elements: []*mpr.JsonElement{
			{
				ExposedName: "Root", Path: "(Object)", ElementType: "Object",
				PrimitiveType: "Unknown", MinOccurs: 1, MaxOccurs: 1,
				Children: []*mpr.JsonElement{
					{
						ExposedName: "A", Path: "(Object)|a", ElementType: "Value",
						PrimitiveType: "Integer", OriginalValue: "1",
						MaxLength: 10, FractionDigits: 2, TotalDigits: 5,
						Nillable: true, IsDefaultType: true,
					},
				},
			},
		},
	}

	out := convertJsonStructure(in)

	if out.Name != "MyJSON" || out.Documentation != "doc" || out.JsonSnippet != `{"a":1}` ||
		!out.Excluded || out.ExportLevel != "Hidden" {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(out.Elements))
	}
	root := out.Elements[0]
	if root.ExposedName != "Root" || root.MinOccurs != 1 {
		t.Errorf("root mismatch: %+v", root)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.PrimitiveType != "Integer" || child.OriginalValue != "1" ||
		child.MaxLength != 10 || child.FractionDigits != 2 || child.TotalDigits != 5 ||
		!child.Nillable || !child.IsDefaultType {
		t.Errorf("child mismatch: %+v", child)
	}
}

func TestConvertJsonStructureSlice(t *testing.T) {
	in := []*mpr.JsonStructure{
		{BaseElement: model.BaseElement{ID: model.ID("js1")}, Name: "JS1"},
	}
	out, err := convertJsonStructureSlice(in, nil)
	if err != nil || len(out) != 1 || out[0].Name != "JS1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertJsonStructureSlice_ErrorPassthrough(t *testing.T) {
	_, err := convertJsonStructureSlice(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertJsonStructurePtr(t *testing.T) {
	in := &mpr.JsonStructure{BaseElement: model.BaseElement{ID: model.ID("js1")}, Name: "JS1"}
	out, err := convertJsonStructurePtr(in, nil)
	if err != nil || out == nil || out.Name != "JS1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertJsonStructurePtr_ErrorPassthrough(t *testing.T) {
	_, err := convertJsonStructurePtr(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

func TestConvertImageCollection(t *testing.T) {
	in := &mpr.ImageCollection{
		BaseElement:   model.BaseElement{ID: model.ID("ic1")},
		ContainerID:   model.ID("c1"),
		Name:          "Images",
		ExportLevel:   "Public",
		Documentation: "img docs",
		Images:        []mpr.Image{{ID: model.ID("i1"), Name: "logo.png", Data: []byte{0x89, 0x50}, Format: "png"}},
	}
	out := convertImageCollection(in)
	if out.Name != "Images" || out.ExportLevel != "Public" || out.Documentation != "img docs" {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.Images) != 1 || out.Images[0].Name != "logo.png" || out.Images[0].Format != "png" {
		t.Errorf("image mismatch: %+v", out.Images)
	}
}

func TestConvertImageCollectionSlice(t *testing.T) {
	in := []*mpr.ImageCollection{
		{BaseElement: model.BaseElement{ID: model.ID("ic1")}, Name: "IC1"},
	}
	out, err := convertImageCollectionSlice(in, nil)
	if err != nil || len(out) != 1 || out[0].Name != "IC1" {
		t.Errorf("unexpected: out=%v err=%v", out, err)
	}
}

func TestConvertImageCollectionSlice_ErrorPassthrough(t *testing.T) {
	_, err := convertImageCollectionSlice(nil, errTest)
	if err != errTest {
		t.Errorf("expected errTest, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unconvert (write-path): mdl/types -> sdk/mpr
// ---------------------------------------------------------------------------

func TestUnconvertNavProfileSpec_FullyPopulated(t *testing.T) {
	in := types.NavigationProfileSpec{
		LoginPage:    "Login.Page",
		NotFoundPage: "NotFound.Page",
		HasMenu:      true,
		HomePages: []types.NavHomePageSpec{
			{IsPage: true, Target: "Home.Page", ForRole: ""},
			{IsPage: false, Target: "Home.MF", ForRole: "Admin"},
		},
		MenuItems: []types.NavMenuItemSpec{
			{Caption: "Top", Page: "P1", Items: []types.NavMenuItemSpec{
				{Caption: "Sub", Microflow: "MF1"},
			}},
		},
	}

	out := unconvertNavProfileSpec(in)

	if out.LoginPage != "Login.Page" || out.NotFoundPage != "NotFound.Page" || !out.HasMenu {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.HomePages) != 2 {
		t.Fatalf("expected 2 home pages, got %d", len(out.HomePages))
	}
	if !out.HomePages[0].IsPage || out.HomePages[0].Target != "Home.Page" {
		t.Errorf("homepage[0] mismatch: %+v", out.HomePages[0])
	}
	if out.HomePages[1].ForRole != "Admin" {
		t.Errorf("homepage[1] mismatch: %+v", out.HomePages[1])
	}
	if len(out.MenuItems) != 1 || out.MenuItems[0].Caption != "Top" {
		t.Errorf("menu mismatch: %+v", out.MenuItems)
	}
	if len(out.MenuItems[0].Items) != 1 || out.MenuItems[0].Items[0].Microflow != "MF1" {
		t.Errorf("sub-menu mismatch: %+v", out.MenuItems[0].Items)
	}
}

func TestUnconvertNavProfileSpec_NilSlices(t *testing.T) {
	in := types.NavigationProfileSpec{LoginPage: "L"}
	out := unconvertNavProfileSpec(in)
	if out.HomePages != nil || out.MenuItems != nil {
		t.Errorf("expected nil slices for nil input: HomePages=%v MenuItems=%v", out.HomePages, out.MenuItems)
	}
}

func TestUnconvertNavMenuItemSpec_Isolated(t *testing.T) {
	in := types.NavMenuItemSpec{
		Caption:   "Parent",
		Page:      "Page1",
		Microflow: "MF1",
		Items: []types.NavMenuItemSpec{
			{Caption: "Child", Microflow: "MF2"},
		},
	}
	// Since mpr.NavMenuItemSpec is aliased to types.NavMenuItemSpec,
	// unconvert is now a pass-through. Verify the alias holds.
	var out mpr.NavMenuItemSpec = in
	if out.Caption != "Parent" || out.Page != "Page1" || out.Microflow != "MF1" {
		t.Errorf("field mismatch: %+v", out)
	}
	if len(out.Items) != 1 || out.Items[0].Caption != "Child" || out.Items[0].Microflow != "MF2" {
		t.Errorf("child mismatch: %+v", out.Items)
	}
}

func TestUnconvertNavMenuItemSpec_NilItems(t *testing.T) {
	in := types.NavMenuItemSpec{Caption: "Leaf"}
	var out mpr.NavMenuItemSpec = in
	if out.Items != nil {
		t.Errorf("expected nil Items for leaf: %+v", out.Items)
	}
}

func TestUnconvertEntityMemberAccessSlice(t *testing.T) {
	in := []types.EntityMemberAccess{
		{AttributeRef: "attr1", AssociationRef: "assoc1", AccessRights: "ReadWrite"},
	}
	out := unconvertEntityMemberAccessSlice(in)
	if len(out) != 1 || out[0].AttributeRef != "attr1" || out[0].AccessRights != "ReadWrite" {
		t.Errorf("mismatch: %+v", out)
	}
}

func TestUnconvertEntityMemberAccessSlice_Nil(t *testing.T) {
	if unconvertEntityMemberAccessSlice(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestUnconvertEntityAccessRevocation(t *testing.T) {
	in := types.EntityAccessRevocation{
		RevokeCreate:       true,
		RevokeDelete:       true,
		RevokeReadMembers:  []string{"attr1"},
		RevokeWriteMembers: []string{"attr2"},
		RevokeReadAll:      true,
		RevokeWriteAll:     false,
	}
	out := unconvertEntityAccessRevocation(in)
	if !out.RevokeCreate || !out.RevokeDelete || !out.RevokeReadAll || out.RevokeWriteAll {
		t.Errorf("bool mismatch: %+v", out)
	}
	if len(out.RevokeReadMembers) != 1 || out.RevokeReadMembers[0] != "attr1" {
		t.Errorf("read members mismatch: %+v", out.RevokeReadMembers)
	}
	if len(out.RevokeWriteMembers) != 1 || out.RevokeWriteMembers[0] != "attr2" {
		t.Errorf("write members mismatch: %+v", out.RevokeWriteMembers)
	}
}

func TestUnconvertJsonStructure_Recursive(t *testing.T) {
	in := &types.JsonStructure{
		BaseElement:   model.BaseElement{ID: model.ID("js1")},
		ContainerID:   model.ID("c1"),
		Name:          "MyJSON",
		Documentation: "doc",
		JsonSnippet:   `{"a":1}`,
		Excluded:      true,
		ExportLevel:   "Hidden",
		Elements: []*types.JsonElement{
			{
				ExposedName: "Root", Path: "(Object)", ElementType: "Object",
				Children: []*types.JsonElement{
					{ExposedName: "A", PrimitiveType: "Integer", MaxLength: 10},
				},
			},
		},
	}

	out := unconvertJsonStructure(in)

	if out.Name != "MyJSON" || out.Documentation != "doc" || !out.Excluded {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.Elements) != 1 || out.Elements[0].ExposedName != "Root" {
		t.Fatalf("element mismatch: %+v", out.Elements)
	}
	if len(out.Elements[0].Children) != 1 || out.Elements[0].Children[0].MaxLength != 10 {
		t.Errorf("child mismatch: %+v", out.Elements[0].Children)
	}
}

func TestUnconvertImageCollection(t *testing.T) {
	in := &types.ImageCollection{
		BaseElement:   model.BaseElement{ID: model.ID("ic1")},
		ContainerID:   model.ID("c1"),
		Name:          "Images",
		ExportLevel:   "Public",
		Documentation: "docs",
		Images:        []types.Image{{ID: model.ID("i1"), Name: "logo.png", Data: []byte{0x89}, Format: "png"}},
	}

	out := unconvertImageCollection(in)

	if out.Name != "Images" || out.ExportLevel != "Public" {
		t.Errorf("top-level mismatch: %+v", out)
	}
	if len(out.Images) != 1 || out.Images[0].Name != "logo.png" || out.Images[0].Data[0] != 0x89 {
		t.Errorf("image mismatch: %+v", out.Images)
	}
}

// ============================================================================
// Field-count drift assertions
// ============================================================================
//
// These tests catch silent field drift: if a struct gains a new field but
// the convert/unconvert function is not updated, the test fails.

func assertFieldCount(t *testing.T, name string, v any, expected int) {
	t.Helper()
	actual := reflect.TypeOf(v).NumField()
	if actual != expected {
		t.Errorf("%s field count changed: expected %d, got %d — update convert.go and this test", name, expected, actual)
	}
}

func TestFieldCountDrift(t *testing.T) {
	// mpr → types pairs (manually copied in convert.go).
	// If a struct gains a field, update the convert function AND this count.
	assertFieldCount(t, "mpr.FolderInfo", mpr.FolderInfo{}, 3)
	assertFieldCount(t, "types.FolderInfo", types.FolderInfo{}, 3)
	assertFieldCount(t, "mpr.UnitInfo", mpr.UnitInfo{}, 4)
	assertFieldCount(t, "types.UnitInfo", types.UnitInfo{}, 4)
	assertFieldCount(t, "mpr.RenameHit", mpr.RenameHit{}, 4)
	assertFieldCount(t, "types.RenameHit", types.RenameHit{}, 4)
	assertFieldCount(t, "mpr.RawUnit", mpr.RawUnit{}, 4)
	assertFieldCount(t, "types.RawUnit", types.RawUnit{}, 4)
	assertFieldCount(t, "mpr.RawUnitInfo", mpr.RawUnitInfo{}, 5)
	assertFieldCount(t, "types.RawUnitInfo", types.RawUnitInfo{}, 5)
	assertFieldCount(t, "mpr.RawCustomWidgetType", mpr.RawCustomWidgetType{}, 6)
	assertFieldCount(t, "types.RawCustomWidgetType", types.RawCustomWidgetType{}, 6)
	assertFieldCount(t, "mpr.JavaAction", mpr.JavaAction{}, 4)
	assertFieldCount(t, "types.JavaAction", types.JavaAction{}, 4)
	assertFieldCount(t, "mpr.JavaScriptAction", mpr.JavaScriptAction{}, 12)
	assertFieldCount(t, "types.JavaScriptAction", types.JavaScriptAction{}, 12)
	assertFieldCount(t, "mpr.NavigationDocument", mpr.NavigationDocument{}, 4)
	assertFieldCount(t, "types.NavigationDocument", types.NavigationDocument{}, 4)
	assertFieldCount(t, "mpr.JsonStructure", mpr.JsonStructure{}, 8)
	assertFieldCount(t, "types.JsonStructure", types.JsonStructure{}, 8)
	assertFieldCount(t, "mpr.JsonElement", mpr.JsonElement{}, 14)
	assertFieldCount(t, "types.JsonElement", types.JsonElement{}, 14)
	assertFieldCount(t, "mpr.ImageCollection", mpr.ImageCollection{}, 6)
	assertFieldCount(t, "types.ImageCollection", types.ImageCollection{}, 6)
	assertFieldCount(t, "mpr.EntityMemberAccess", mpr.EntityMemberAccess{}, 3)
	assertFieldCount(t, "types.EntityMemberAccess", types.EntityMemberAccess{}, 3)
	assertFieldCount(t, "mpr.EntityAccessRevocation", mpr.EntityAccessRevocation{}, 6)
	assertFieldCount(t, "types.EntityAccessRevocation", types.EntityAccessRevocation{}, 6)
}
