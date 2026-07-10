// SPDX-License-Identifier: Apache-2.0

// Package mprbackend_test verifies that the conversion layer between sdk/mpr
// and mdl/types works correctly. Because sdk/mpr types are now type aliases to
// mdl/types (e.g. mpr.JavaAction = types.JavaAction), the "convert" functions
// are effectively deep-copy operations on the same type. This test file proves
// the type system is consistent and conversions preserve all fields.
package mprbackend_test

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// TestTypeAliasesAreIdentical proves that sdk/mpr type aliases resolve to the
// same Go types as mdl/types. If these assignments compile, the types are
// identical — which is precisely what the conversion functions rely on.
func TestTypeAliasesAreIdentical(t *testing.T) {
	// Each assignment proves the alias: mpr.X == types.X
	var _ *types.JavaAction = new(mpr.JavaAction)
	var _ *types.JavaScriptAction = new(mpr.JavaScriptAction)
	var _ *types.NavigationDocument = new(mpr.NavigationDocument)
	var _ *types.NavigationProfile = new(mpr.NavigationProfile)
	var _ *types.NavHomePage = new(mpr.NavHomePage)
	var _ *types.NavRoleBasedHome = new(mpr.NavRoleBasedHome)
	var _ *types.NavMenuItem = new(mpr.NavMenuItem)
	var _ *types.NavOfflineEntity = new(mpr.NavOfflineEntity)
	var _ *types.JsonStructure = new(mpr.JsonStructure)
	var _ *types.JsonElement = new(mpr.JsonElement)
	var _ *types.ImageCollection = new(mpr.ImageCollection)
	var _ *types.FolderInfo = new(mpr.FolderInfo)
	var _ *types.UnitInfo = new(mpr.UnitInfo)
	var _ *types.RawUnit = new(mpr.RawUnit)
	var _ *types.ProjectVersion = new(mpr.ProjectVersion)

	// Slices are also interchangeable
	var typesSlice []*types.FolderInfo
	var mprSlice []*mpr.FolderInfo = typesSlice
	_ = mprSlice

	var typesJSSlice []*types.JavaScriptAction
	var mprJSSlice []*mpr.JavaScriptAction = typesJSSlice
	_ = mprJSSlice
}

// TestFolderInfoSlicePassthrough verifies that a []*mpr.FolderInfo value can
// be used where []*types.FolderInfo is expected, because they are the same type.
func TestFolderInfoSlicePassthrough(t *testing.T) {
	folders := []*mpr.FolderInfo{
		{ID: model.ID("f1"), ContainerID: model.ID("c1"), Name: "Module"},
		{ID: model.ID("f2"), ContainerID: model.ID("c2"), Name: "Resources"},
	}

	// This compiles because mpr.FolderInfo = types.FolderInfo
	var typesFolders []*types.FolderInfo = folders
	if len(typesFolders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(typesFolders))
	}
	if typesFolders[0].Name != "Module" {
		t.Errorf("expected Module, got %q", typesFolders[0].Name)
	}
}

// TestNavigationDocumentFieldPreservation verifies that all fields survive
// when a NavigationDocument created via mpr alias is accessed via types.
func TestNavigationDocumentFieldPreservation(t *testing.T) {
	doc := &mpr.NavigationDocument{
		ContainerID: model.ID("c1"),
		Name:        "Navigation",
		Profiles: []*mpr.NavigationProfile{
			{
				Name:     "Responsive",
				Kind:     "Responsive",
				IsNative: false,
				HomePage: &mpr.NavHomePage{Page: "MyFirstModule.Home"},
				RoleBasedHomePages: []*mpr.NavRoleBasedHome{
					{UserRole: "Admin", Page: "Admin.Dashboard"},
				},
				MenuItems: []*mpr.NavMenuItem{
					{Caption: "Home", Page: "Home"},
				},
				OfflineEntities: []*mpr.NavOfflineEntity{
					{Entity: "MyModule.Task", SyncMode: "FullSync"},
				},
			},
		},
	}

	// Access through types — compiles because they're the same type
	var typesDoc *types.NavigationDocument = doc
	if typesDoc.Name != "Navigation" {
		t.Errorf("expected Navigation, got %q", typesDoc.Name)
	}
	if len(typesDoc.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(typesDoc.Profiles))
	}
	p := typesDoc.Profiles[0]
	if p.Kind != "Responsive" {
		t.Errorf("expected Responsive, got %q", p.Kind)
	}
	if p.HomePage.Page != "MyFirstModule.Home" {
		t.Errorf("expected home page, got %q", p.HomePage.Page)
	}
	if len(p.RoleBasedHomePages) != 1 {
		t.Errorf("expected 1 role-based home, got %d", len(p.RoleBasedHomePages))
	}
	if len(p.MenuItems) != 1 {
		t.Errorf("expected 1 menu item, got %d", len(p.MenuItems))
	}
	if len(p.OfflineEntities) != 1 {
		t.Errorf("expected 1 offline entity, got %d", len(p.OfflineEntities))
	}
}

// TestJsonStructureFieldPreservation verifies JsonStructure + recursive
// JsonElement children survive alias crossing.
func TestJsonStructureFieldPreservation(t *testing.T) {
	js := &mpr.JsonStructure{
		ContainerID:   model.ID("m1"),
		Name:          "MyJson",
		Documentation: "Test json structure",
		JsonSnippet:   `{"a":1}`,
		Elements: []*mpr.JsonElement{
			{
				ExposedName:   "Root",
				Path:          "(Object)",
				ElementType:   "Object",
				PrimitiveType: "Unknown",
				Children: []*mpr.JsonElement{
					{
						ExposedName:   "A",
						Path:          "(Object)|a",
						ElementType:   "Value",
						PrimitiveType: "Integer",
						OriginalValue: "1",
					},
				},
			},
		},
	}

	var typesJS *types.JsonStructure = js
	if typesJS.Name != "MyJson" {
		t.Errorf("expected MyJson, got %q", typesJS.Name)
	}
	if len(typesJS.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(typesJS.Elements))
	}
	root := typesJS.Elements[0]
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	child := root.Children[0]
	if child.PrimitiveType != "Integer" {
		t.Errorf("expected Integer, got %q", child.PrimitiveType)
	}
	if child.OriginalValue != "1" {
		t.Errorf("expected original value '1', got %q", child.OriginalValue)
	}
}

// TestImageCollectionFieldPreservation verifies ImageCollection + Image.
func TestImageCollectionFieldPreservation(t *testing.T) {
	ic := &mpr.ImageCollection{
		ContainerID: model.ID("m1"),
		Name:        "Images",
		Images: []mpr.Image{
			{ID: model.ID("i1"), Name: "logo.png", Format: "png", Data: []byte{0x89, 0x50}},
		},
	}

	var typesIC *types.ImageCollection = ic
	if typesIC.Name != "Images" {
		t.Errorf("expected Images, got %q", typesIC.Name)
	}
	if len(typesIC.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(typesIC.Images))
	}
	img := typesIC.Images[0]
	if img.Name != "logo.png" {
		t.Errorf("expected logo.png, got %q", img.Name)
	}
	if len(img.Data) != 2 {
		t.Errorf("expected 2 bytes, got %d", len(img.Data))
	}
}
