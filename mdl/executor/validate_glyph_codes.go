// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateGlyphCodes (MDL078) flags a glyph icon whose code the Mendix glyph
// font does not define — on a navigation menu item, a menu document, or a page
// widget.
//
// # Why this needs its own rule
//
// A glyph code is a bare integer: nothing resolves it, so it passes `mxcli check`
// AND `mx check` at 0 errors, and fails only at `mxbuild --target=deploy` — which
// is what `mxcli run --local` does. The message it fails with names a document
// that is not the problem:
//
//	ERROR: One or more errors occurred.
//	(An exception occurred while exporting layout 'CapTrack.App_Default.')
//
// Neither the navigation nor the menu item is mentioned. Bisecting it cost three
// build cycles (ako/CapTrackV4 007, R2).
//
// A glyph on a page WIDGET fails the same way and names the page instead —
// measured on 11.13.0 with `Icon: glyph 57562` on an action button:
//
//	(An exception occurred while exporting page 'MyFirstModule.IconProbe'.)
//	 ---> System.InvalidOperationException: Sequence contains no matching element
//
// So the message must not promise "layout": the document named is whichever one
// contains the icon, which is exactly the document the author will not suspect.
//
// # The mechanism, and why a code-point table is the right check
//
// mxbuild resolves the code through GlyphFont.GetClass(Int32), which is a LINQ
// `.First(...)` over its glyph table and throws `InvalidOperationException:
// Sequence contains no matching element` when the code is absent. Measured on
// 11.14.0 with two builds: 57562 (0xEBDA, past the font's 0xE260 end) produces
// exactly that stack, and 57377 (0xE021, in the font) exports pages and layouts
// cleanly. The font's cmap and mxbuild's table agree on both points.
//
// # An error, not a warning (mendixlabs/mxcli#1522)
//
// This rule was a warning, on the reasoning that the table is a snapshot of a
// Mendix asset and a font Mendix later extended would make a correct code look
// wrong. The cost of that caution turned out to be the larger one: `exec`
// wrote the code, `mx check` passed it at 0 errors, and `mxbuild
// --target=deploy` died on the unhandled InvalidOperationException with exit 1
// and NO error file — so a build gate that reads the error file had nothing to
// report, and the author had a stack trace naming the wrong document.
//
// The snapshot risk is measured, not assumed. mxbuild does not read the font
// Atlas_Core ships; it bundles its own `modeler/glyphicons-halflings-regular.ttf`,
// and that file is byte-identical (sha256 e3950440…57456) in mxbuild 9.24, 10.12,
// 11.6.6 and 11.15.0. Its cmap's private-use range is exactly the 247
// private-use codes in glyphIcons, and the two measured builds (11.14.0) agree
// with it on both points. The cmap also maps code points OUTSIDE that range,
// and the table first left them out, so MDL078 refused glyph 9999 (pencil) and
// 43 (plus), which Marketplace modules store and which build
// (ako/mxcli#2112). One deploy build per code on 11.12.6 settled each of them:
// the 11 the stylesheet names build, the whitespace ones and U+25FC crash.
// See glyphIcons. The font has been frozen for the whole span
// mxcli supports, so refusing a code outside it refuses a build crash, not a
// valid icon. If a later Mendix ever does extend it, glyphIcons is the one
// table to update, and `--no-check` is the escape hatch meanwhile.
//
// Every carrier goes through this one rule — navigation menu items, menu
// documents and page/snippet/layout widgets, via iconRefsInStatement — so they
// all refuse alike.
//
// # It needs no project
//
// The code is in the script. This runs in the project-free pass alongside
// MDL077, which is how CI reaches it.
//
// # One walk
//
// The statements are enumerated by iconRefsInStatement, the package's single
// icon walk, rather than by a second switch here. This rule covered menu items
// only while menus were the only place a glyph could be authored; the day
// `Icon: glyph <n>` landed on a widget (mendixlabs/mxcli#1059) a private walk
// would have kept passing every widget glyph in silence, which is the drift the
// shared walk exists to prevent.
func validateGlyphCodes(stmt ast.Statement) []linter.Violation {
	var out []linter.Violation
	for _, ref := range iconRefsInStatement(stmt) {
		if ref.kind != types.MenuIconGlyph || glyphCodeDefined(ref.code) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL078",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s uses glyph %d, which the Mendix glyph font does not define — "+
					"`mxbuild --target=deploy` crashes on it (\"Sequence contains no matching "+
					"element\", exit 1, no error file) while `mx check` passes",
				ref.where, ref.code),
			Suggestion: "check the icon: " + glyphNeighbours(ref.code) +
				"`list glyphs like '<name>'` finds a glyph by name. Better, use an icon collection " +
				"reference, which is resolved before anything is written: `Atlas_Core.Atlas.<name>` " +
				"(list them with `describe icon collection Atlas_Core.Atlas`).",
		})
	}
	return out
}

// glyphNeighbours names the defined glyphs either side of an undefined code,
// e.g. "the nearest defined glyphs are 57385 (play-circle) and 57392 (repeat); ".
// The codes are sparse, so "a nearby number" is usually undefined too; naming
// real neighbours is the useful version of that hint.
func glyphNeighbours(code int) string {
	i := sort.Search(len(glyphIcons), func(i int) bool { return glyphIcons[i].Code >= code })
	var near []string
	if i > 0 {
		g := glyphIcons[i-1]
		near = append(near, fmt.Sprintf("%d (%s)", g.Code, g.Name))
	}
	if i < len(glyphIcons) {
		g := glyphIcons[i]
		near = append(near, fmt.Sprintf("%d (%s)", g.Code, g.Name))
	}
	switch len(near) {
	case 2:
		return "the nearest defined glyphs are " + near[0] + " and " + near[1] + "; "
	case 1:
		return "the nearest defined glyph is " + near[0] + "; "
	}
	return ""
}
