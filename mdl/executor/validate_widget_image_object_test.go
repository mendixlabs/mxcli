// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mendixlabs/mxcli#1476, the check-time half: `image i (Image: $currentObject)`
// is a complete widget — it shows the object in context — so neither
// MDL-WIDGET22 ("No image selected.") nor MDL-WIDGET42 (an expression in a
// non-Expression property) may fire on it.

func boundImageWidget() *ast.WidgetV3 {
	return &ast.WidgetV3{
		Name:        "img",
		Type:        "image",
		Properties:  map[string]any{"Image": &ast.DataSourceV3{Type: "parameter", Reference: "$currentObject"}},
		ValueSource: map[string]string{"Image": "$currentObject"},
	}
}

func widget42Only(vs []linter.Violation, rule string) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestImageSource_CurrentObjectIsComplete(t *testing.T) {
	if got := imageViolations(validateImageSource(boundImageWidget(), "page X")); len(got) != 0 {
		t.Errorf("MDL-WIDGET22 fired on a bound image: %+v", got)
	}
}

func TestImageExpressionValue_CurrentObjectIsNotAnExpression(t *testing.T) {
	got := widget42Only(validateWidgetExpressionValues(boundImageWidget(), realImageDef(t), nil, false, "page X"), "MDL-WIDGET42")
	if len(got) != 0 {
		t.Errorf("MDL-WIDGET42 fired on `Image: $currentObject`: %+v", got)
	}
}

// CONTROL: an arbitrary expression in the same property is still refused.
func TestImageExpressionValue_OtherExpressionIsStillRefused(t *testing.T) {
	w := &ast.WidgetV3{Name: "img", Type: "image",
		Properties: map[string]any{"Image": &ast.WidgetExpressionV3{Text: "$a + 'x'"}}}
	if got := widget42Only(validateWidgetExpressionValues(w, realImageDef(t), nil, false, "page X"), "MDL-WIDGET42"); len(got) != 1 {
		t.Errorf("got %d MDL-WIDGET42, want 1: %+v", len(got), got)
	}
}

// Variant B of the report: `ImageType: dynamicImage` checked and executed
// clean, and mx check passed it too — the widget's `datasource` is image,
// imageUrl or icon, and nothing else is a source the widget can render.
func TestImageSource_UnknownImageTypeIsReported(t *testing.T) {
	got := imageViolations(validateImageSource(imageWidget(map[string]any{"ImageType": "dynamicImage"}), "page X"))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	for _, want := range []string{"dynamicImage", "imageUrl", "icon"} {
		if !strings.Contains(got[0].Message+got[0].Suggestion, want) {
			t.Errorf("report does not mention %q: %+v", want, got[0])
		}
	}
	if !strings.Contains(got[0].Suggestion, "$currentObject") {
		t.Errorf("the suggestion should name the spelling that binds an uploaded image: %s", got[0].Suggestion)
	}
}

// CONTROL: each real value, in any case, stays silent when complete.
func TestImageSource_KnownImageTypesAreAccepted(t *testing.T) {
	for _, props := range []map[string]any{
		{"ImageType": "icon", "Icon": "Atlas_Core.Atlas_Filled.user"},
		{"ImageType": "ICON", "Icon": &ast.WidgetIcon{Kind: types.MenuIconImage, Name: "M.C.I"}},
		{"ImageType": "imageUrl", "ImageUrl": "https://x"},
		{"ImageType": "Image", "Image": "M.C.I"},
	} {
		if got := imageViolations(validateImageSource(imageWidget(props), "page X")); len(got) != 0 {
			t.Errorf("%v: unexpected %+v", props, got)
		}
	}
}

// `DefaultImage:` on the pluggable image names a collection entry like
// `Image:` does, so --references must resolve it — a new spelling nobody
// resolves is how #1149 shipped.
func TestWidgetRefs_ImageDefaultImageIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)
	errs := validateWidgetReferences(ctx,
		imageRefWidget("image", "DefaultImage", "MyFirstModule.Images.NoSuchImage"),
		newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "NoSuchImage") {
		t.Fatalf("an image fallback naming a missing image was accepted; got %v", errs)
	}
}

// `DefaultImage:` is the fallback for a BOUND image; Studio Pro hides
// `defaultImageDynamic` otherwise, and a value in a hidden property is CE0463 —
// measured on mxbuild 11.12.1 with `Image: 'Atlas_Web_Content.Content.user',
// DefaultImage: 'Atlas_Web_Content.Content.user'`.
func TestImageSource_DefaultImageWithoutBindingIsReported(t *testing.T) {
	got := imageViolations(validateImageSource(imageWidget(map[string]any{
		"Image": "M.C.I", "DefaultImage": "M.C.Fallback",
	}), "page X"))
	if len(got) != 1 || got[0].Severity != linter.SeverityError {
		t.Fatalf("got %+v, want one error", got)
	}
	if !strings.Contains(got[0].Message, "CE0463") || !strings.Contains(got[0].Suggestion, "$currentObject") {
		t.Errorf("report should name CE0463 and the binding it needs: %+v", got[0])
	}
}

// CONTROL: with the binding it is the intended shape (mx check: 0 errors).
func TestImageSource_DefaultImageWithBindingIsAccepted(t *testing.T) {
	w := boundImageWidget()
	w.Properties["DefaultImage"] = "M.C.Fallback"
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("unexpected %+v", got)
	}
}
