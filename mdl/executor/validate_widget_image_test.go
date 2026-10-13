// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Found while fixing the Image CE0463 (mxcli-formula1 FINDINGS §69/§142): with
// CE0463 out of the way, the bare form's real problem is visible.
//
//	image i (Responsive: false)
//	  → [error] "No image selected." at Image 'i'
//
// The Image widget's `datasource` defaults to `image`, which shows an image from
// an image collection, named on the widget's `imageObject` property.
//
// When this rule was written MDL could not name one at all, so the only advice
// it could give was to switch source. `Image:` is authorable now, so the rule
// reports a genuinely incomplete widget — and the case that must NOT fire is a
// widget that names its entry.
//
// CE0463 was masking that, which is why it went unnoticed: the definition error
// fired first and `mx update-widgets` "fixed" the page by clearing it, leaving
// the real error behind. The same gap is what makes a describe → exec copy of an
// Atlas layout lose its brand image (§142).
//
// Reporting it at check time is the whole value: mxbuild reports it at the far
// end of a build, and `mxcli check` runs in a second.

func imageWidget(props map[string]any) *ast.WidgetV3 {
	return &ast.WidgetV3{Name: "img", Type: "image", Properties: props}
}

func imageViolations(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL-WIDGET22" {
			out = append(out, v)
		}
	}
	return out
}

// The reported case: the default data source with no image to show.
func TestImageWidget_DefaultSourceWithNoImageIsReported(t *testing.T) {
	got := imageViolations(validateImageSource(imageWidget(map[string]any{"Responsive": "false"}), "page X"))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — mxbuild refuses the build", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "No image selected") {
		t.Errorf("the message should quote what mxbuild says: %s", got[0].Message)
	}
	// The suggestion must lead with the remedy that keeps the author's intent —
	// naming the entry — not only with "use a different source".
	if !strings.Contains(got[0].Suggestion, "Image: 'Module.Collection.ImageName'") {
		t.Errorf("the suggestion should show how to name the entry: %s", got[0].Suggestion)
	}
	if !strings.Contains(got[0].Suggestion, "imageUrl") {
		t.Errorf("the suggestion should still offer the alternative sources: %s", got[0].Suggestion)
	}
}

// …and with an explicit ImageType: image, which is the same model.
func TestImageWidget_ExplicitImageTypeWithNoImageIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "image"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}

// CONTROL 1: the URL form is complete and must pass. This is the shape the
// CE0463 fix was verified on, at 0 errors.
func TestImageWidget_UrlFormIsClean(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "imageUrl", "ImageUrl": "https://example.com/x.png"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("the URL form builds at 0 errors and must not be reported: %+v", got)
	}
}

// CONTROL 2: an icon source needs no image either.
func TestImageWidget_IconFormIsClean(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "icon"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("an icon source needs no image: %+v", got)
	}
}

// CONTROL 3: the rule is about the IMAGE widget only. A different widget with a
// stray ImageType property is none of its business.
func TestImageWidget_OtherWidgetsAreNotTouched(t *testing.T) {
	w := &ast.WidgetV3{Name: "dg", Type: "datagrid", Properties: map[string]any{}}
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("a datagrid was reported: %+v", got)
	}
}

// CONTROL 4: an ImageType nobody recognises is left alone rather than guessed
// at. Reporting on an unknown source would fire on a widget version this build
// does not know about.
func TestImageWidget_UnknownImageTypeIsLeftAlone(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "somethingNew"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("an unrecognised ImageType was reported: %+v", got)
	}
}

// The URL form with an EMPTY url is the same incompleteness by the other route,
// and mxbuild rejects it too.
func TestImageWidget_EmptyUrlIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "imageUrl", "ImageUrl": ""})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}

// The capability this rule used to say did not exist: an image collection entry
// named on the widget. It must not be reported.
func TestImageWidget_NamedCollectionEntryIsClean(t *testing.T) {
	for _, props := range []map[string]any{
		{"Image": "MyFirstModule.Images._1"},                       // default source
		{"ImageType": "image", "Image": "MyFirstModule.Images._1"}, // source spelled out
	} {
		if got := imageViolations(validateImageSource(imageWidget(props), "page X")); len(got) != 0 {
			t.Errorf("a widget naming its image was reported: %+v", got)
		}
	}
}

// CONTROL: an empty Image is not an image. Whitespace is not either — the value
// reaches the writer verbatim, so " " would store a reference to nothing.
func TestImageWidget_EmptyOrBlankImageIsStillReported(t *testing.T) {
	for _, v := range []string{"", "   "} {
		got := imageViolations(validateImageSource(imageWidget(map[string]any{"Image": v}), "page X"))
		if len(got) != 1 {
			t.Errorf("Image=%q: got %d violations, want 1", v, len(got))
		}
	}
}

// mendixlabs/mxcli#1476: `Image: $currentObject` binds the image to the object
// in context. It names no collection image and is complete — MDL-WIDGET22 must
// not ask for one, and MDL-WIDGET42 must not call it an expression.
func currentObjectRef() *ast.DataSourceV3 {
	return &ast.DataSourceV3{Type: "parameter", Reference: "$currentObject"}
}

func TestImageWidget_BoundToTheObjectInContextIsClean(t *testing.T) {
	for _, props := range []map[string]any{
		{"Image": currentObjectRef()},
		{"Image": currentObjectRef(), "ImageType": "image"},
		{"Image": currentObjectRef(), "DefaultImage": "Atlas_Core.Content.Mendix"},
	} {
		if got := imageViolations(validateImageSource(imageWidget(props), "page P")); len(got) != 0 {
			t.Errorf("%v reported: %v", props, got)
		}
	}
}

func TestImageWidget_BindingIsNotAnExpression(t *testing.T) {
	reg := LoadWidgetRegistry("")
	w := imageWidget(map[string]any{"Image": currentObjectRef()})
	w.ValueSource = map[string]string{"Image": "$currentObject"}
	for _, v := range validateWidgetExpressionValues(w, reg.byMDLName["IMAGE"], nil, false, "page P") {
		t.Errorf("reported as an expression: %s", v.Message)
	}
}

// Anything but the context object is refused: its stored shape is unmeasured.
func TestImageWidget_BindingToAnotherObjectIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"Image": &ast.DataSourceV3{Type: "parameter", Reference: "$Photo"}})
	got := imageViolations(validateImageSource(w, "page P"))
	if len(got) != 1 || !strings.Contains(got[0].Message, "$currentObject") {
		t.Errorf("got %v, want one MDL-WIDGET22 naming $currentObject", got)
	}
}

// The default image is the fallback for an image object; Studio Pro hides it
// beside a collection image, and with any other image type.
func TestImageWidget_DefaultImageWithoutABindingIsReported(t *testing.T) {
	for _, props := range []map[string]any{
		{"Image": "Atlas_Core.Content.Mendix", "DefaultImage": "Atlas_Core.Content.Mendix"},
		{"ImageType": "imageUrl", "ImageUrl": "https://example.com/x.png", "DefaultImage": "Atlas_Core.Content.Mendix"},
		{"ImageType": "icon", "Image": currentObjectRef()},
	} {
		if got := imageViolations(validateImageSource(imageWidget(props), "page P")); len(got) != 1 {
			t.Errorf("%v: got %d violations, want 1", props, len(got))
		}
	}
}
