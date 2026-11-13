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

// CONTROL 2: an icon source needs no image — it needs an icon instead.
func TestImageWidget_IconFormIsClean(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "icon", "Icon": "Atlas_Core.Atlas_Filled.user"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("an icon source with an icon builds clean: %+v", got)
	}
}

// ako/mxcli#2174: an icon source with no `Icon:` is the image mxbuild refuses
// with "No icon selected." — measured on 11.16, once per such image.
func TestImageWidget_IconFormWithNoIconIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "icon"})
	got := imageViolations(validateImageSource(w, "page X"))
	if len(got) != 1 || got[0].Severity != linter.SeverityError ||
		!strings.Contains(got[0].Message, "No icon selected.") {
		t.Fatalf("want one MDL-WIDGET22 error naming \"No icon selected.\", got %+v", got)
	}
	if !strings.Contains(got[0].Suggestion, "Icon:") {
		t.Errorf("the suggestion should name the `Icon:` spelling: %s", got[0].Suggestion)
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

// CONTROL 4: an ImageType nobody recognises is never an ERROR — a widget
// version this build does not know may add a source. It is a warning, because
// silence let `ImageType: dynamicImage` through check, exec and mx check alike
// (mendixlabs/mxcli#1476).
func TestImageWidget_UnknownImageTypeIsOnlyAWarning(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "somethingNew"})
	got := imageViolations(validateImageSource(w, "page X"))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1 warning: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityWarning {
		t.Errorf("severity = %v, want a warning — an unknown source may be a newer widget's", got[0].Severity)
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
