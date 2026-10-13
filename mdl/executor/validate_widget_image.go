// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL-WIDGET22: an IMAGE widget with nothing to show.
//
// The Image widget's `datasource` property selects where the image comes from,
// and it defaults to `image` — an entry from an image collection, named on the
// widget's `imageObject` property. A widget with that source and no entry is a
// model mxbuild refuses:
//
//	[error] "No image selected." at Image 'i'
//
// This was invisible until the CE0463 fix landed. Every mxcli-authored Image
// failed the build with CE0463 "the definition of this widget has changed" —
// which fired first, and which `mx update-widgets` cleared, leaving the real
// error behind it (mxcli-formula1 FINDINGS §69/§142).
//
// When this rule was first written MDL could not name an image collection entry
// at all, so its only advice was to switch source. `Image:` is now authorable
// (see the `image` operation in widget_engine.go), so the rule reports a
// genuinely incomplete widget rather than a missing capability — and the remedy
// it offers is the one that keeps the author's intent.
//
// Reported at check time because that costs a second rather than a build. It is
// an ERROR: the build does not pass, so accepting it would be mxcli waving
// through a script it knows produces a broken app.
const imageSourceRule = "MDL-WIDGET22"

// imageSourceNeedsImage lists the `datasource` values that require an image
// collection entry. Anything else — including a value this build does not
// recognise — is left alone rather than guessed at.
var imageSourceNeedsImage = map[string]bool{
	"":      true, // absent: the widget's own default is "image"
	"image": true,
}

// validateImageSource reports an IMAGE widget whose configured source has no
// value to render, or whose image-object binding cannot be written.
//
// The image may also be the object in context, `Image: $currentObject`
// (mendixlabs/mxcli#1476). That form names no collection image and is complete
// on its own; `DefaultImage:` is its fallback, and only its — Studio Pro hides
// the default image unless the image is bound to an object.
func validateImageSource(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "image") {
		return nil
	}
	source := strings.TrimSpace(w.GetStringProp("ImageType"))
	binding, bound := imageObjectProp(w)
	hasDefault := strings.TrimSpace(w.GetStringProp("DefaultImage")) != ""

	if bound && !isImageContextRef(binding) {
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) binds its image to %s — only `$currentObject`, the object of "+
					"the enclosing data view or list, can be bound",
				locationPrefix, w.Name, formatDataSourceV3(binding)),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: fmt.Sprintf(
				"Place the widget in a data view over that object and bind it there: "+
					"`dataview v (DataSource: $Photo) { image %s (Image: $currentObject) }`.", w.Name),
		}}
	}

	switch {
	case imageSourceNeedsImage[strings.ToLower(source)]:
		if bound {
			return nil // the image object in context — the widget is complete
		}
		if hasDefault {
			return []linter.Violation{{
				RuleID:   imageSourceRule,
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` (image) has a `DefaultImage` but shows a collection image — the "+
						"default image is the fallback for an image object, and Studio Pro hides it otherwise",
					locationPrefix, w.Name),
				Location: linter.Location{DocumentType: "page"},
				Suggestion: fmt.Sprintf(
					"Bind the image to the object in context, `image %s (Image: $currentObject, "+
						"DefaultImage: 'Module.Collection.ImageName')`, or drop `DefaultImage`.", w.Name),
			}}
		}
		if strings.TrimSpace(w.GetStringProp("Image")) != "" {
			return nil // an entry is named — the widget is complete
		}
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) shows an image from an image collection "+
					"(the default `ImageType: image`) but names no image — mxbuild rejects the "+
					"build with \"No image selected.\"",
				locationPrefix, w.Name),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: fmt.Sprintf(
				"Name the entry: `image %s (Image: 'Module.Collection.ImageName')` — "+
					"`show image collections` lists them, and `describe image collection "+
					"Module.Collection` lists the images inside one. Or show the object in context, "+
					"inside a data view over an entity that specialises System.Image: "+
					"`Image: $currentObject`. Or switch source: "+
					"`ImageType: imageUrl, ImageUrl: 'https://…'`, or `ImageType: icon`.",
				w.Name),
		}}
	case bound || hasDefault:
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) has `ImageType: %s` and an image object or `DefaultImage` — "+
					"both belong to `ImageType: image`, and Studio Pro hides them for any other source",
				locationPrefix, w.Name, source),
			Location:   linter.Location{DocumentType: "page"},
			Suggestion: "Drop `ImageType` (it defaults to `image`), or drop `Image`/`DefaultImage`.",
		}}
	case strings.EqualFold(source, "imageUrl") && strings.TrimSpace(w.GetStringProp("ImageUrl")) == "":
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) has `ImageType: imageUrl` and no `ImageUrl` — "+
					"mxbuild rejects the build with \"No image selected.\"",
				locationPrefix, w.Name),
			Location:   linter.Location{DocumentType: "page"},
			Suggestion: "Give it a URL: `ImageUrl: 'https://…'`.",
		}}
	}
	return nil
}

// imageObjectProp returns the widget's `Image:` value when it is an object
// rather than a collection image's name.
func imageObjectProp(w *ast.WidgetV3) (*ast.DataSourceV3, bool) {
	v, ok := lookupProperty(w.Properties, "Image")
	if !ok {
		return nil, false
	}
	ds, isDS := v.(*ast.DataSourceV3)
	return ds, isDS
}
