// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2108: a dynamic image whose source is an association from the
// context object — `DataSource: $currentObject/Mod.Stock_ItemImage`, as
// describe prints three FactoryManagement pages — was refused on write ("a
// *pages.AssociationSource data source cannot be stored on a
// Forms$ImageViewerSource"), so describe -> exec failed on pages Studio Pro
// stored. The path goes in the source's EntityRef as an IndirectEntityRef,
// the same slot a data view uses for it.
func TestDynamicImageWritesAnAssociationSource(t *testing.T) {
	img := &pages.DynamicImage{
		DataSource: &pages.AssociationSource{EntityPath: "Mod.Stock_ItemImage/Mod.ItemImage"},
	}
	img.Name = "dynamicImage1"

	doc := encodeWidget(t, img)
	src, ok := docGet(doc, "DataSource").(bson.D)
	if !ok || docGet(src, "$Type") != "Forms$ImageViewerSource" {
		t.Fatalf("DataSource = %#v, want a Forms$ImageViewerSource", docGet(doc, "DataSource"))
	}
	ref, ok := docGet(src, "EntityRef").(bson.D)
	if !ok || docGet(ref, "$Type") != "DomainModels$IndirectEntityRef" {
		t.Fatalf("EntityRef = %#v, want a DomainModels$IndirectEntityRef", docGet(src, "EntityRef"))
	}
	var steps []bson.D
	if arr, ok := docGet(ref, "Steps").(bson.A); ok {
		for _, el := range arr {
			if d, ok := el.(bson.D); ok {
				steps = append(steps, d)
			}
		}
	}
	if len(steps) != 1 || docGet(steps[0], "Association") != "Mod.Stock_ItemImage" ||
		docGet(steps[0], "DestinationEntity") != "Mod.ItemImage" {
		t.Errorf("Steps = %#v, want one step Mod.Stock_ItemImage -> Mod.ItemImage", steps)
	}
	// $currentObject is the context: no source variable.
	if sv := docGet(src, "SourceVariable"); sv != nil {
		t.Errorf("SourceVariable = %#v, want none for $currentObject", sv)
	}
}

// A page parameter as the context rides in SourceVariable, as on a data view.
func TestDynamicImageAssociationSourceFromAParameter(t *testing.T) {
	img := &pages.DynamicImage{
		DataSource: &pages.AssociationSource{EntityPath: "Mod.Stock_ItemImage/Mod.ItemImage", ContextVariable: "Stock"},
	}
	img.Name = "dynamicImage1"
	src, _ := docGet(encodeWidget(t, img), "DataSource").(bson.D)
	if _, ok := docGet(src, "SourceVariable").(bson.D); !ok {
		t.Errorf("SourceVariable = %#v, want a Forms$PageVariable for $Stock", docGet(src, "SourceVariable"))
	}
}
