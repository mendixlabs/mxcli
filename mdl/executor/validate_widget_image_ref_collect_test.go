// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1149. The reporter wants to put the images a Selection
// helper's custom states show into MDL — #1057 made `staticimage` authorable and
// gave it `Image: 'Module.Collection.Image'`, and this is the half that came
// with it: nothing resolves that name.
//
// imageRefErrors was wired for the pluggable `image` widget and keyed on the
// widget TYPE, so the widget #1057 had just given the same reference was not
// collected — nor was a dynamic image's `DefaultImage` fallback. Measured on a
// blank Mendix 11.14.0 project, the reporter's own shape:
//
//	selectionhelper sh1 (renderStyle: 'custom') {
//	  customallselected slot1 {
//	    staticimage imgAll (Image: 'Atlas_UI_Resources.Atlas_Icons.checkbox_checked')
//	  }
//	  …
//	}
//
//	mxcli check sh.mdl -p SH1149.mpr --references -> Check passed!
//	mxcli exec  sh.mdl -p SH1149.mpr             -> Created page …
//	mx check                                      -> [error] [CE1613] "The selected
//	    image 'Atlas_UI_Resources.Atlas_Icons.checkbox_checked' no longer exists."
//	    at Static image 'imgAll'   (x3, one per state)
//
// There is no such collection in a blank app (`show image collections` lists
// seven, none of them Atlas_UI_Resources) — the name is what a reader guesses
// from the icon they want, and mxcli had nothing to say about it until the
// build. Same defect class as #1057 itself: check clean, exec clean, build red.

// imageCollectionCtx is a project holding exactly one image collection with one
// image in it, so a reference can be right or wrong in a measurable way.
func imageCollectionCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyFirstModule")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Images",
		Images:      []types.Image{{Name: "gallery"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// imageRefWidget is one widget of the given type carrying one image reference
// under the given property key.
func imageRefWidget(widgetType, propKey, ref string) []*ast.WidgetV3 {
	return []*ast.WidgetV3{{
		Name:       "img1",
		Type:       widgetType,
		Properties: map[string]any{propKey: ref},
	}}
}

// The reported symptom, at the layer it lives in.
func TestWidgetRefs_StaticImageReferenceIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)

	errs := validateWidgetReferences(ctx,
		imageRefWidget("staticimage", "Image", "Atlas_UI_Resources.Atlas_Icons.checkbox_checked"),
		newScriptContext())
	if len(errs) != 1 {
		t.Fatalf("a staticimage naming an image collection the project does not have was accepted — "+
			"that is CE1613 at build time; got %d errors: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "Atlas_UI_Resources.Atlas_Icons") {
		t.Errorf("the message should name the missing collection: %s", errs[0])
	}
}

// The same reference on a dynamic image's fallback, which #1057's sibling commit
// made authorable on the same day and left unresolved for the same reason.
func TestWidgetRefs_DynamicImageDefaultImageIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)

	errs := validateWidgetReferences(ctx,
		imageRefWidget("dynamicimage", "DefaultImage", "MyFirstModule.Images.NoSuchImage"),
		newScriptContext())
	if len(errs) != 1 {
		t.Fatalf("a dynamicimage fallback naming an image the collection does not hold was "+
			"accepted; got %d errors: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "NoSuchImage") {
		t.Errorf("the message should name the missing image: %s", errs[0])
	}
}

// The reporter's actual shape: the image sits inside a pluggable widget's child
// slot, not at the top of the page. Collection recurses through Children, so
// this passes for free once the type is recognised — and it is the case the
// issue is about, so it is asserted rather than assumed.
func TestWidgetRefs_StaticImageInsideACustomStateIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)

	widgets := []*ast.WidgetV3{{
		Name:       "sh1",
		Type:       "selectionhelper",
		Properties: map[string]any{"renderStyle": "custom"},
		Children: []*ast.WidgetV3{{
			Name:       "customallselected1",
			Type:       "customallselected",
			Properties: map[string]any{},
			Children:   imageRefWidget("staticimage", "Image", "MyFirstModule.Images.NoSuchIcon"),
		}},
	}}

	errs := validateWidgetReferences(ctx, widgets, newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "NoSuchIcon") {
		t.Fatalf("an unresolvable image inside a Selection helper custom state was accepted; got %v", errs)
	}
}

// CONTROL. An image that DOES resolve must stay silent under every spelling —
// otherwise the fix is indistinguishable from forbidding the feature, which is
// what #1057 made possible in the first place.
func TestWidgetRefs_ResolvableImagesStaySilent(t *testing.T) {
	cases := []struct{ widgetType, propKey string }{
		{"staticimage", "Image"},
		{"dynamicimage", "DefaultImage"},
		{"image", "Image"},
	}
	for _, tc := range cases {
		t.Run(tc.widgetType+"."+tc.propKey, func(t *testing.T) {
			ctx := imageCollectionCtx(t)
			errs := validateWidgetReferences(ctx,
				imageRefWidget(tc.widgetType, tc.propKey, "MyFirstModule.Images.gallery"),
				newScriptContext())
			if len(errs) != 0 {
				t.Errorf("an image that exists was reported: %v", errs)
			}
		})
	}
}

// CONTROL. `Image` is only an image-collection reference on the widgets that
// spell it that way. A widget carrying an unrelated `Image` property must not be
// dragged into the check — reporting it would break scripts that are correct.
func TestWidgetRefs_ImagePropOnAnotherWidgetIsNotAnImageReference(t *testing.T) {
	ctx := imageCollectionCtx(t)

	errs := validateWidgetReferences(ctx,
		imageRefWidget("container", "Image", "not.a.reference"),
		newScriptContext())
	if len(errs) != 0 {
		t.Errorf("a non-image widget's Image property was resolved as an image collection entry: %v", errs)
	}
}

// mendixlabs/mxcli#1476 gave the pluggable `image` widget the same fallback,
// `DefaultImage:` beside `Image: $currentObject`; it is resolved like the
// dynamic image's, or a mistyped name is CE1613 at build time.
func TestWidgetRefs_ImageWidgetDefaultImageIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)

	errs := validateWidgetReferences(ctx,
		imageRefWidget("image", "DefaultImage", "MyFirstModule.Images.NoSuchImage"),
		newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "NoSuchImage") {
		t.Fatalf("an image widget's fallback naming an image the collection does not hold was "+
			"accepted; got %v", errs)
	}
}
