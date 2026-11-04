// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// Wiring `Image:` gave MDL a new reference to get wrong, and the first thing it
// did was reproduce this session's recurring shape: a typo passed
// `mxcli check --references` and failed the build.
//
//	image imgTypo (Image: 'MyFirstModule.Images.NoSuchImage')
//
//	mxcli check --references -> Check passed!
//	mx check                 -> [CE1613] "The selected image
//	                            'MyFirstModule.Images.NoSuchImage' no longer exists."
//
// Every other qualified name a widget can carry — microflow, nanoflow, page,
// snippet, entity — is resolved by validateWidgetReferences. An image is now
// one of them.
//
// The name has THREE parts, unlike the rest: Module.Collection.Image. So both
// halves have to be checked, and the message has to say which one is wrong —
// "collection not found" and "that collection has no such image" send the
// reader to different places.

func TestImageRefErrors_UnknownImageIsReported(t *testing.T) {
	known := map[string]bool{"MyFirstModule.Images._1": true}
	collections := map[string]bool{"MyFirstModule.Images": true}

	errs := imageRefErrors([]string{"MyFirstModule.Images.NoSuchImage"}, known, collections)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "NoSuchImage") {
		t.Errorf("the message should name the image: %s", errs[0])
	}
	// The collection exists, so the reader needs to know it is the image that is
	// wrong — and what the collection does hold.
	if !strings.Contains(errs[0], "MyFirstModule.Images") {
		t.Errorf("the message should name the collection: %s", errs[0])
	}
}

// A missing COLLECTION is a different mistake and gets a different message.
func TestImageRefErrors_UnknownCollectionSaysSo(t *testing.T) {
	errs := imageRefErrors([]string{"Nope.Missing.Img"}, map[string]bool{}, map[string]bool{})
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "image collection") || !strings.Contains(errs[0], "Nope.Missing") {
		t.Errorf("the message should name the missing collection: %s", errs[0])
	}
}

// CONTROL: an image that exists is not reported. Without this the check just
// forbids the feature.
func TestImageRefErrors_KnownImageIsClean(t *testing.T) {
	known := map[string]bool{"MyFirstModule.Images._1": true}
	collections := map[string]bool{"MyFirstModule.Images": true}

	if errs := imageRefErrors([]string{"MyFirstModule.Images._1"}, known, collections); len(errs) != 0 {
		t.Errorf("an existing image was reported: %v", errs)
	}
}

// Mendix resolves an image reference case-SENSITIVELY, in every part. Measured
// on a blank 11.13.0 app, whose Atlas_Core.Content holds `Mendix`: each of
// `Atlas_Core.Content.mendix`, `Atlas_Core.content.Mendix` and
// `atlas_core.Content.Mendix` is CE1613 "The selected image … no longer
// exists", and `Atlas_Core.Content.Mendix` is clean. The check folded case and
// passed all three, and its "it holds:" hint printed `mendix` — so following
// the hint produced the CE1613. A case-only miss names the stored spelling.
func TestImageRefErrors_CaseMismatchIsReportedWithTheStoredSpelling(t *testing.T) {
	known := map[string]bool{"Atlas_Core.Content.Mendix": true}
	collections := map[string]bool{"Atlas_Core.Content": true}
	for _, tc := range []struct{ ref, want string }{
		{"Atlas_Core.Content.mendix", "Atlas_Core.Content.Mendix"},
		{"Atlas_Core.content.Mendix", "Atlas_Core.Content"},
		{"atlas_core.Content.Mendix", "Atlas_Core.Content"},
	} {
		errs := imageRefErrors([]string{tc.ref}, known, collections)
		if len(errs) != 1 {
			t.Errorf("%s: got %d errors, want 1 — Mendix reports CE1613 for it: %v", tc.ref, len(errs), errs)
			continue
		}
		for _, want := range []string{tc.want, "case-sensitive"} {
			if !strings.Contains(errs[0], want) {
				t.Errorf("%s: the message should say %q: %s", tc.ref, want, errs[0])
			}
		}
	}
}

// The "it holds:" hint lists the images as stored. Lower-cased, it named
// `mendix` for a stored `Mendix`, and copying it from the message was CE1613.
func TestImageRefErrors_HintListsImagesAsStored(t *testing.T) {
	known := map[string]bool{"Atlas_Core.Content.Mendix": true}
	collections := map[string]bool{"Atlas_Core.Content": true}
	errs := imageRefErrors([]string{"Atlas_Core.Content.mendix_logo"}, known, collections)
	if len(errs) != 1 || !strings.Contains(errs[0], "it holds: Mendix") {
		t.Errorf("want the hint to list Mendix as stored, got %v", errs)
	}
}

// A name that is not three parts cannot be resolved, and is reported as the
// shape mistake it is rather than as a missing image.
func TestImageRefErrors_MalformedNameIsReported(t *testing.T) {
	for _, ref := range []string{"JustOne", "Module.Collection"} {
		errs := imageRefErrors([]string{ref}, map[string]bool{}, map[string]bool{})
		if len(errs) != 1 {
			t.Fatalf("%q: got %d errors, want 1: %v", ref, len(errs), errs)
		}
		if !strings.Contains(errs[0], "Module.Collection.ImageName") {
			t.Errorf("%q: the message should show the expected shape: %s", ref, errs[0])
		}
	}
}

// CONTROL: with no image collections readable at all, nothing is reported.
// A project whose collections could not be listed must not have every image
// called missing — that is the "guessing wrong rejects working input" direction.
func TestImageRefErrors_NoCollectionsMeansNoOpinion(t *testing.T) {
	if errs := imageRefErrors([]string{"Mod.Coll.Img"}, nil, nil); len(errs) != 0 {
		t.Errorf("reported an image with nothing to resolve against: %v", errs)
	}
}
