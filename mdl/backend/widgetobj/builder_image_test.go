// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// mxcli-formula1 FINDINGS §142, and the gap MDL-WIDGET22 was named for: MDL
// could not say WHICH image an image widget shows.
//
// The Image widget's `datasource` defaults to `image` — an entry from an image
// collection — and the property holding that entry, `imageObject`, is of type
// `image`. The widget engine had no operation for that type, so the property was
// unreachable from MDL and every default-form image widget wrote a model mxbuild
// refuses with "No image selected." It is also why a describe → exec copy of an
// Atlas layout loses its brand image.
//
// The stored shape is a plain string on the WidgetValue, measured on a Studio
// Pro-authored widget in a stock project:
//
//	/Object/Properties[2]/Value/Image = "MyFirstModule.Images._1"
//
// Three parts: Module.Collection.Image. Nothing else about the value node
// changes, which is why this is the same shape as SetSelection rather than
// anything structural.

// imageValue is one WidgetValue as stored: the Image key sits beside the others
// and is empty until something sets it.
func imageValue() bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "PrimitiveValue", Value: ""},
		{Key: "Image", Value: ""},
		{Key: "TextTemplate", Value: nil},
	}
}

func get(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func TestSetImageValue_SetsTheQualifiedName(t *testing.T) {
	got := setImageValue(imageValue(), "MyFirstModule.Images._1")

	if v := get(got, "Image"); v != "MyFirstModule.Images._1" {
		t.Errorf("Image = %v, want the qualified name", v)
	}
}

// Every other key must come through untouched and in order. A WidgetValue that
// gained or lost a key is the CE0463 shape this whole area keeps producing.
func TestSetImageValue_LeavesEverythingElseAlone(t *testing.T) {
	in := imageValue()
	got := setImageValue(in, "Mod.Coll.Img")

	if len(got) != len(in) {
		t.Fatalf("key count changed: %d -> %d", len(in), len(got))
	}
	for i := range in {
		if got[i].Key != in[i].Key {
			t.Errorf("key %d: %q -> %q — order changed", i, in[i].Key, got[i].Key)
		}
	}
	if v := get(got, "PrimitiveValue"); v != "" {
		t.Errorf("PrimitiveValue was disturbed: %v", v)
	}
	if v := get(got, "TextTemplate"); v != nil {
		t.Errorf("TextTemplate was disturbed: %v", v)
	}
}

// A value node with no Image key at all is left as it is rather than having one
// invented. Adding a key the widget's definition does not declare is precisely
// what mxbuild answers with CE0463.
func TestSetImageValue_DoesNotInventTheKey(t *testing.T) {
	in := bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetValue"}, {Key: "PrimitiveValue", Value: ""}}
	got := setImageValue(in, "Mod.Coll.Img")

	if len(got) != len(in) {
		t.Fatalf("a key was added to a value node that does not declare one: %v", got)
	}
}

// An empty name is a no-op, like every other Set* on this builder: "the script
// did not say" must not overwrite what the template holds. Without this, an
// image widget authored without `Image:` would have its template default
// blanked rather than left alone.
func TestSetImageValue_EmptyNameLeavesTheValueAlone(t *testing.T) {
	in := bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "Image", Value: "Mod.Coll.Existing"},
	}
	if got := get(setImageValue(in, ""), "Image"); got != "Mod.Coll.Existing" {
		t.Errorf("Image = %v, want the existing value untouched", got)
	}
}

// mendixlabs/mxcli#1476: the image object in context. Studio Pro 11.12.1
// stores it on the WidgetValue as a DirectEntityRef to the data view's entity,
// with an empty Image:
//
//	EntityRef: {$Type: 'DomainModels$DirectEntityRef', Entity: 'MyFirstModule.Photo'}
//	Image:     ''
func imageObjectValue() bson.D {
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
		{Key: "EntityRef", Value: nil},
		{Key: "Image", Value: "Atlas_Core.Content.Mendix"},
		{Key: "PrimitiveValue", Value: ""},
	}
}

func TestSetImageObjectValue_WritesADirectEntityRefAndClearsTheImage(t *testing.T) {
	got := setImageObjectValue(imageObjectValue(), "MyFirstModule.Photo")

	ref, ok := get(got, "EntityRef").(bson.D)
	if !ok {
		t.Fatalf("EntityRef = %v, want a document", get(got, "EntityRef"))
	}
	if v := get(ref, "$Type"); v != "DomainModels$DirectEntityRef" {
		t.Errorf("EntityRef $Type = %v, want DomainModels$DirectEntityRef", v)
	}
	if v := get(ref, "Entity"); v != "MyFirstModule.Photo" {
		t.Errorf("EntityRef Entity = %v, want MyFirstModule.Photo", v)
	}
	if get(ref, "$ID") == nil {
		t.Error("EntityRef has no $ID")
	}
	if v := get(got, "Image"); v != "" {
		t.Errorf("Image = %v, want empty — Studio Pro clears the collection image", v)
	}
}

// Same key set, same order: only the two values change.
func TestSetImageObjectValue_LeavesEverythingElseAlone(t *testing.T) {
	in := imageObjectValue()
	got := setImageObjectValue(in, "MyFirstModule.Photo")
	if len(got) != len(in) {
		t.Fatalf("key count changed: %d -> %d", len(in), len(got))
	}
	for i := range in {
		if got[i].Key != in[i].Key {
			t.Errorf("key %d: %q -> %q — order changed", i, in[i].Key, got[i].Key)
		}
	}
}

// An empty entity name is "not bound": the value stays as it was.
func TestSetImageObjectValue_EmptyEntityLeavesTheValueAlone(t *testing.T) {
	got := setImageObjectValue(imageObjectValue(), "")
	if get(got, "EntityRef") != nil || get(got, "Image") != "Atlas_Core.Content.Mendix" {
		t.Errorf("value changed for an empty entity: %v", got)
	}
}
