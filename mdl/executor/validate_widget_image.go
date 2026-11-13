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

// imageSourceNeedsImage lists the `datasource` values that require an image —
// a collection entry or, since #1476, the object in context.
var imageSourceNeedsImage = map[string]bool{
	"":      true, // absent: the widget's own default is "image"
	"image": true,
}

// imageSourceValues are the Image widget's `datasource` enumeration keys, as
// its template declares them. Any other value checked, executed AND passed mx
// check — `ImageType: dynamicImage` in #1476, which reads like the way to show
// an uploaded image and is no source the widget has.
var imageSourceValues = []string{"image", "imageUrl", "icon"}

func isImageSourceValue(v string) bool {
	if v == "" {
		return true
	}
	for _, known := range imageSourceValues {
		if strings.EqualFold(v, known) {
			return true
		}
	}
	return false
}

// imageNamesItsImage reports whether w says which image it shows: a quoted
// collection entry, or `$currentObject` for the object in context.
func imageNamesItsImage(w *ast.WidgetV3) bool {
	return strings.TrimSpace(w.GetStringProp("Image")) != "" || imageBoundToObject(w)
}

// imageBoundToObject reports whether w's `Image:` is `$currentObject`.
func imageBoundToObject(w *ast.WidgetV3) bool {
	raw, ok := lookupProperty(w.Properties, "Image")
	if !ok {
		return false
	}
	ref, isRef := imageObjectReference(w, "Image", raw)
	return isRef && isImageObjectVariable(ref)
}

// validateImageSource reports an IMAGE widget whose configured source has no
// value to render.
func validateImageSource(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "image") {
		return nil
	}
	source := strings.TrimSpace(w.GetStringProp("ImageType"))
	out := validateImageDefault(w, locationPrefix)
	return append(out, validateImageSourceValue(w, source, locationPrefix)...)
}

// validateImageDefault reports a `DefaultImage:` on an image that is not bound
// to the object in context. The fallback is `defaultImageDynamic`, which Studio
// Pro hides unless `imageObject` is bound, and a value in a hidden property is
// CE0463 — measured on mxbuild 11.12.1 (#1476).
func validateImageDefault(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if strings.TrimSpace(w.GetStringProp("DefaultImage")) == "" || imageBoundToObject(w) {
		return nil
	}
	return []linter.Violation{{
		RuleID:   imageSourceRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: widget `%s` (image) has a `DefaultImage` but is not bound to an object — the "+
				"fallback is shown only for an uploaded image, and Studio Pro hides it otherwise; "+
				"mxbuild rejects the stored value with CE0463",
			locationPrefix, w.Name),
		Location: linter.Location{DocumentType: "page"},
		Suggestion: fmt.Sprintf(
			"Bind the data view's object, `image %s (Image: $currentObject, DefaultImage: …)`, "+
				"or drop DefaultImage.", w.Name),
	}}
}

func validateImageSourceValue(w *ast.WidgetV3, source, locationPrefix string) []linter.Violation {
	switch {
	case !isImageSourceValue(source):
		// A WARNING, not an error: a later widget version may add a source this
		// build does not know, and refusing it would block a valid script. What
		// it must not be is silent — this one passes mx check.
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) has `ImageType: %s`, which is not an image source — "+
					"the Image widget's sources are %s, and mx check does not catch an unknown one",
				locationPrefix, w.Name, source, strings.Join(imageSourceValues, ", ")),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: fmt.Sprintf(
				"To show an uploaded image (an entity that extends System.Image), drop ImageType and "+
					"bind the object in context: `image %s (Image: $currentObject)` inside a data view "+
					"over it. Otherwise use `ImageType: image` with a collection entry, "+
					"`ImageType: imageUrl`, or `ImageType: icon` with an `Icon:`.",
				w.Name),
		}}
	case imageSourceNeedsImage[strings.ToLower(source)]:
		if imageNamesItsImage(w) {
			return nil // an entry or the context object is named — the widget is complete
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
					"Module.Collection` lists the images inside one. To show an uploaded image, "+
					"bind the data view's object: `image %s (Image: $currentObject)`. Or switch source: "+
					"`ImageType: imageUrl, ImageUrl: 'https://…'`, or `ImageType: icon, Icon: '…'`.",
				w.Name, w.Name),
		}}
	case strings.EqualFold(source, "icon") && widgetIconSpec(w) == nil:
		// Measured on 11.16: an icon image with no icon is "No icon selected.",
		// once per image — what re-creating one from describe output wrote
		// before describe printed the icon (ako/mxcli#2174).
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) has `ImageType: icon` and no `Icon` — "+
					"mxbuild rejects the build with \"No icon selected.\"",
				locationPrefix, w.Name),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: "Name the icon as a button does: `Icon: 'Atlas_Core.Atlas_Filled.user'` " +
				"(an icon collection), `Icon: image 'Module.Collection.Image'`, or `Icon: glyph 57377`.",
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
