// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mxcli-formula1 FINDINGS §142: a describe → rename → exec copy of an Atlas
// layout comes out with no brand image.
//
// `describe` emitted the Image widget as the built-in shorthand with no image
// reference at all —
//
//	image staticImage1 (Responsive: false)
//
// — because MDL had no way to name an image collection entry, so there was
// nothing to emit. Wiring the `image` operation gives the writer that
// capability; without the READ half the round trip still loses it, and the copy
// still renders blank.
//
// The stored form is a plain string on the WidgetValue, measured on a Studio
// Pro-authored widget: `Image: "MyFirstModule.Images._1"`, three parts,
// Module.Collection.Image.

// imagePropWidget is one stored CustomWidget carrying an image-typed property.
// The TypePointer indirection is the whole reason a dedicated extractor exists:
// the property is found by resolving its pointer to a PropertyKey, not by
// position.
func imagePropWidget(propertyKey, stored string) map[string]any {
	return map[string]any{
		"Type": map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{
						"$ID":         "pt-1",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"PropertyKey": propertyKey,
					},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt-1",
					"Value": map[string]any{
						"$Type":          "CustomWidgets$WidgetValue",
						"PrimitiveValue": "",
						"Image":          stored,
					},
				},
			},
		},
	}
}

func TestExtractCustomWidgetPropertyImage_ReadsTheReference(t *testing.T) {
	ctx, _ := newMockCtx(t)
	got := extractCustomWidgetPropertyImage(ctx, imagePropWidget("imageObject", "MyFirstModule.Images._1"), "imageObject")
	if got != "MyFirstModule.Images._1" {
		t.Errorf("got %q, want the stored qualified name", got)
	}
}

// CONTROL 1: an unset image reads as empty, so DESCRIBE emits nothing rather
// than an `Image: ”` that would not re-execute.
func TestExtractCustomWidgetPropertyImage_UnsetIsEmpty(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if got := extractCustomWidgetPropertyImage(ctx, imagePropWidget("imageObject", ""), "imageObject"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// CONTROL 2: it must resolve the property by KEY, not take the first Image it
// finds. The Image widget has two image-typed properties — `imageObject` and
// `defaultImageDynamic` — so a positional read would return the wrong one.
func TestExtractCustomWidgetPropertyImage_ResolvesByPropertyKey(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := imagePropWidget("defaultImageDynamic", "Mod.Coll.Fallback")
	if got := extractCustomWidgetPropertyImage(ctx, w, "imageObject"); got != "" {
		t.Errorf("got %q for imageObject, but only defaultImageDynamic is set", got)
	}
	if got := extractCustomWidgetPropertyImage(ctx, w, "defaultImageDynamic"); got != "Mod.Coll.Fallback" {
		t.Errorf("got %q, want the fallback image", got)
	}
}

// The end §142 is about: what DESCRIBE prints. An image widget with an entry
// must emit `Image:` so describe → exec keeps it.
func TestDescribeImageWidget_EmitsTheImageReference(t *testing.T) {
	w := rawWidget{Name: "imgLogo"}
	w.ImageObject = "MyFirstModule.Images._1"

	out := describeImageWidgetProps(nil, w)
	if !strings.Contains(strings.Join(out, ", "), "Image: 'MyFirstModule.Images._1'") {
		t.Errorf("describe did not emit the image reference: %v", out)
	}
}

// CONTROL: a widget with no entry emits no `Image:` at all. An empty one would
// re-execute into a reference to nothing.
func TestDescribeImageWidget_OmitsAnAbsentReference(t *testing.T) {
	w := rawWidget{Name: "imgLogo"}

	for _, p := range describeImageWidgetProps(nil, w) {
		if strings.HasPrefix(p, "Image:") {
			t.Errorf("emitted %q for a widget with no image", p)
		}
	}
}

// mendixlabs/mxcli#1476: the image object in context as Studio Pro stores it —
// a DirectEntityRef on `imageObject`, no collection image, no source variable.
func imageObjectWidget(value map[string]any) map[string]any {
	w := imagePropWidget("imageObject", "")
	props := w["Object"].(map[string]any)["Properties"].([]any)
	stored := props[0].(map[string]any)["Value"].(map[string]any)
	for k, v := range value {
		stored[k] = v
	}
	return w
}

func TestExtractImageObjectBinding_ReadsTheContextEntity(t *testing.T) {
	w := imageObjectWidget(map[string]any{
		"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "MyFirstModule.Photo"},
	})
	entity, unreadable := extractImageObjectBinding(w, "imageObject")
	if entity != "MyFirstModule.Photo" || unreadable != "" {
		t.Errorf("got (%q, %q), want (MyFirstModule.Photo, \"\")", entity, unreadable)
	}
}

// Shapes MDL cannot spell are reported, never printed as `$currentObject`.
func TestExtractImageObjectBinding_UnmeasuredShapesAreUnreadable(t *testing.T) {
	for name, value := range map[string]map[string]any{
		"association": {"EntityRef": map[string]any{"$Type": "DomainModels$IndirectEntityRef"}},
		"parameter": {
			"EntityRef":      map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "MyFirstModule.Photo"},
			"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Photo"},
		},
	} {
		entity, unreadable := extractImageObjectBinding(imageObjectWidget(value), "imageObject")
		if entity != "" || unreadable == "" {
			t.Errorf("%s: got (%q, %q), want no entity and a reason", name, entity, unreadable)
		}
	}
}

func TestDescribeImageWidget_EmitsTheImageObjectBinding(t *testing.T) {
	w := rawWidget{Name: "Photo_View_Image", ImageObjectEntity: "MyFirstModule.Photo", DefaultImage: "Atlas_Core.Content.Mendix"}
	got := strings.Join(describeImageWidgetProps(nil, w), ", ")
	if !strings.Contains(got, "Image: $currentObject") {
		t.Errorf("describe did not emit the binding: %s", got)
	}
	if !strings.Contains(got, "DefaultImage: 'Atlas_Core.Content.Mendix'") {
		t.Errorf("describe did not emit the default image: %s", got)
	}
}
