// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreateImportMapping_RoundTrip creates an import mapping with a root object
// element and a value child, then confirms it round-trips through
// ListImportMappings — including the element-tree fields (Entity, Kind, child
// Attribute) that the microflow builder reads to shape an "import from mapping"
// result. The read converter must populate these, not just ID/TypeName.
func TestCreateImportMapping_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	im := &model.ImportMapping{
		ContainerID: mod.ID,
		Name:        "ZzImport",
		Elements: []*model.ImportMappingElement{{
			Kind:      "Object",
			Entity:    "MyFirstModule.PetResponse",
			MaxOccurs: 1,
			Children: []*model.ImportMappingElement{{
				Kind:      "Value",
				Attribute: "MyFirstModule.PetResponse.Name",
				DataType:  "String",
			}},
		}},
	}
	if err := b.CreateImportMapping(im); err != nil {
		t.Fatalf("CreateImportMapping: %v", err)
	}

	// GetByQualifiedName must resolve documents in any module (the moduleNameFor
	// fix: pass the document's own unit ID, not its container/module ID).
	got, err := b.GetImportMappingByQualifiedName("MyFirstModule", "ZzImport")
	if err != nil {
		t.Fatalf("GetImportMappingByQualifiedName: %v", err)
	}
	if len(got.Elements) != 1 {
		t.Fatalf("root elements = %d, want 1", len(got.Elements))
	}
	root := got.Elements[0]
	if root.Entity != "MyFirstModule.PetResponse" {
		t.Errorf("root Entity = %q, want MyFirstModule.PetResponse", root.Entity)
	}
	if root.Kind != "Object" {
		t.Errorf("root Kind = %q, want Object", root.Kind)
	}
	if len(root.Children) != 1 || root.Children[0].Attribute != "MyFirstModule.PetResponse.Name" {
		t.Errorf("child attribute not round-tripped: %+v", root.Children)
	}
}

// TestCreateExportMapping_RoundTrip mirrors the import case for the export side.
func TestCreateExportMapping_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	em := &model.ExportMapping{
		ContainerID: mod.ID,
		Name:        "ZzExport",
		Elements: []*model.ExportMappingElement{{
			Kind:   "Object",
			Entity: "MyFirstModule.PetResponse",
			Children: []*model.ExportMappingElement{{
				Kind:      "Value",
				Attribute: "MyFirstModule.PetResponse.Name",
				DataType:  "String",
			}},
		}},
	}
	if err := b.CreateExportMapping(em); err != nil {
		t.Fatalf("CreateExportMapping: %v", err)
	}

	got, err := b.GetExportMappingByQualifiedName("MyFirstModule", "ZzExport")
	if err != nil {
		t.Fatalf("GetExportMappingByQualifiedName: %v", err)
	}
	if len(got.Elements) != 1 || got.Elements[0].Entity != "MyFirstModule.PetResponse" {
		t.Fatalf("root element not round-tripped: %+v", got.Elements)
	}
}
