// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// Resolving the `Image:` reference an image widget now carries.
//
// Wiring the `image` operation gave MDL a new qualified name to get wrong, and
// the first thing a typo did was slip past the reference check and fail the
// build instead:
//
//	image imgTypo (Image: 'MyFirstModule.Images.NoSuchImage')
//
//	mxcli check --references -> Check passed!
//	mx check                 -> [CE1613] "The selected image
//	                            'MyFirstModule.Images.NoSuchImage' no longer exists."
//
// Every other name a widget can carry — microflow, nanoflow, page, snippet,
// entity — is resolved by validateWidgetReferences. This is the same check for
// the one reference that has THREE parts: Module.Collection.Image. Which half is
// wrong decides where the author has to look, so the two mistakes get two
// messages.

// imageRefErrors reports the image references in refs that do not resolve.
//
// known holds every image as a three-part qualified name and collections every
// image collection as a two-part one, both spelled as stored: Mendix resolves
// an image reference case-sensitively in every part (measured on 11.13.0 — a
// case difference anywhere is CE1613), so the match is exact, and a miss that
// differs only in case names the stored spelling. A nil map means the
// project's collections could not be listed — then nothing is reported,
// because calling every image missing on a failed read is the wrong direction
// to be wrong in. An empty non-nil map is a real answer: the project has no
// image collections, so any reference to one is wrong.
func imageRefErrors(refs []string, known map[string]bool, collections map[string]bool) []string {
	var errors []string
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		parts := strings.Split(ref, ".")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			errors = append(errors, fmt.Sprintf(
				"invalid image reference: %s — an image is named "+
					"Module.Collection.ImageName (three parts); `show image collections` "+
					"lists the collections",
				ref))
			continue
		}
		if collections == nil || known == nil {
			continue // nothing to resolve against
		}

		collection := parts[0] + "." + parts[1]
		if !collections[collection] {
			if stored := sameNameOtherCase(collections, collection); stored != "" {
				errors = append(errors, fmt.Sprintf(
					"image collection not found: %s (referenced as %s) — image names are "+
						"case-sensitive; the collection is %s",
					collection, ref, stored))
				continue
			}
			if isSystemCollection(collection) {
				errors = append(errors, fmt.Sprintf(
					"image collection not found: %s (referenced as %s) — the System module's "+
						"image collections are System.Images and System.WorkflowImages",
					collection, ref))
				continue
			}
			errors = append(errors, fmt.Sprintf(
				"image collection not found: %s (referenced as %s) — "+
					"`show image collections` lists them",
				collection, ref))
			continue
		}
		if !known[ref] && isSystemCollection(collection) {
			// An image of a System collection the measured list lacks: other
			// Mendix versions hold other images, so refusing it would block a
			// valid reference. mxbuild resolves it (CE1613 if it is wrong).
			continue
		}
		if !known[ref] {
			if stored := sameNameOtherCase(known, ref); stored != "" {
				errors = append(errors, fmt.Sprintf(
					"image not found: %s — image names are case-sensitive; write %s",
					ref, stored))
				continue
			}
			msg := fmt.Sprintf("image not found: %s in image collection %s", parts[2], collection)
			if inCollection := imagesInCollection(known, collection); len(inCollection) > 0 {
				msg += fmt.Sprintf(" — it holds: %s", strings.Join(inCollection, ", "))
			} else {
				msg += fmt.Sprintf(" — `describe image collection %s` lists its images", collection)
			}
			errors = append(errors, msg)
		}
	}
	return errors
}

// sameNameOtherCase returns the name in set that equals name except for case,
// or "" when there is none — the stored spelling a case-only miss meant.
func sameNameOtherCase(set map[string]bool, name string) string {
	for stored := range set {
		if strings.EqualFold(stored, name) {
			return stored
		}
	}
	return ""
}

// imagesInCollection returns the image names known for one collection, as
// stored, so the diagnostic can show what is actually there instead of only
// what is not. Capped, since a collection can hold hundreds.
func imagesInCollection(known map[string]bool, collection string) []string {
	prefix := collection + "."
	var names []string
	for qn := range known {
		if strings.HasPrefix(qn, prefix) {
			names = append(names, strings.TrimPrefix(qn, prefix))
		}
	}
	sort.Strings(names)
	const max = 10
	if len(names) > max {
		names = append(names[:max:max], "…")
	}
	return names
}

// buildImageQualifiedNames returns the project's images as three-part
// qualified names, and its image collections as two-part ones, spelled as
// stored. Both are nil when the collections could not be read at all — the
// caller distinguishes that from "there are none".
func buildImageQualifiedNames(ctx *ExecContext) (images map[string]bool, collections map[string]bool) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, nil
	}
	ics, err := ctx.Backend.ListImageCollections()
	if err != nil {
		return nil, nil
	}
	images = make(map[string]bool)
	collections = make(map[string]bool)
	for _, ic := range ics {
		qn := h.GetQualifiedName(ic.ContainerID, ic.Name)
		collections[qn] = true
		for _, img := range ic.Images {
			images[qn+"."+img.Name] = true
		}
	}
	addSystemImages(images, collections)
	return images, collections
}

// addSystemImages adds the System module's image collections, which ship with
// the platform and are never in the project's listing: `System.Images.Completed`
// was "image collection not found" although mx check accepts it
// (ako/mxcli#2235). Their images are listed as measured on one version.
func addSystemImages(images, collections map[string]bool) {
	for qn, names := range meta.SystemImageCollections {
		collections[qn] = true
		for _, n := range names {
			images[qn+"."+n] = true
		}
	}
}
