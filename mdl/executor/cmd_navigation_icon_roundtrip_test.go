// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// DESCRIBE -> exec must not destroy a menu icon.
//
// # The failure this pins
//
// Measured on testdata/expr-checker, whose Home item carries a glyph icon:
//
//	describe  menu item 'Home' page …;
//	          -- icon a numeric glyph code (Forms$GlyphIcon) is not reproducible …
//	exec      Navigation profile 'Responsive' updated.
//	describe  menu item 'Home' page …;        <- comment gone: the icon was DELETED
//
// CREATE NAVIGATION is a full replacement, so an icon the writer could not emit
// was an icon the statement removed. Exit 0, success message, silent loss — the
// same shape as the pluggable-widget body loss in mendixlabs/mxcli#1036.
//
// # Why this test is at this layer
//
// The round trip is describe -> parse -> write, and the defect lived in the fact
// that the three stages disagreed about what an icon IS. Asserting on the MDL
// text is what catches that: the emitted clause has to carry enough to rebuild
// the SAME element, not merely something that parses.
func TestMenuIconMDL_RoundTripsEveryVariant(t *testing.T) {
	cases := []struct {
		name string
		item types.NavMenuItem
		want string
	}{
		{
			"collection",
			types.NavMenuItem{IconType: "Forms$IconCollectionIcon", Icon: "Atlas_Core.Atlas.home"},
			"Atlas_Core.Atlas.home",
		},
		{
			// The one that was being destroyed. The code IS the glyph's identity;
			// reading only the $Type left DESCRIBE with nothing to say.
			"glyph",
			types.NavMenuItem{IconType: "Forms$GlyphIcon", IconCode: 57377},
			"glyph 57377",
		},
		{
			// An image icon points into an IMAGE collection, a different document
			// from an icon collection — so it needs its own keyword, or replay
			// would rebuild it as the wrong element.
			"image",
			types.NavMenuItem{IconType: "Forms$ImageIcon", Icon: "System.Images.Close"},
			"image System.Images.Close",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := menuItemIconMDL(&tc.item, false)
			if got != tc.want {
				t.Fatalf("menuItemIconMDL = %q, want %q", got, tc.want)
			}
			// It must also re-parse to the same kind, or DESCRIBE emits something
			// that reads back as a different icon — which is what the bare form
			// would have done for an image icon.
			if note := menuItemIconNote(&tc.item, "CREATE NAVIGATION", false); note != "" {
				t.Errorf("still flagged as unreproducible: %q", note)
			}
		})
	}
}

// The control. Without a case that still cannot be rebuilt, "emits a clause for
// everything" and "correctly reproduces everything" look identical — and the
// wrong one of those is how an unknown future variant would get silently
// converted into something else.
func TestMenuIconMDL_DeclinesWhatItCannotRebuild(t *testing.T) {
	for _, item := range []types.NavMenuItem{
		// The code is the glyph's whole identity; without it there is nothing to
		// emit that rebuilds the same icon.
		{IconType: "Forms$GlyphIcon"},
		// A $Type this build does not know must never be guessed at.
		{IconType: "Forms$SomeFutureIcon", Icon: "M.X.y"},
	} {
		if got := menuItemIconMDL(&item, false); got != "" {
			t.Errorf("%s: emitted %q, want no clause", item.IconType, got)
		}
		if note := menuItemIconNote(&item, "CREATE NAVIGATION", false); !strings.Contains(note, "not reproducible") {
			t.Errorf("%s: dropped silently instead of being flagged: %q", item.IconType, note)
		}
	}
}
