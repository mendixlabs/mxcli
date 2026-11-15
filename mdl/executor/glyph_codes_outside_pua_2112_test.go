// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2112: MDL078 refused glyph 9999 and glyph 43, which Marketplace
// modules store (ConversationalUI, TreeGraphWidgets), so a describe → exec round
// trip of those pages was refused. The table held only the font's private-use
// range; Glyphicons Halflings also maps eleven named icons at ordinary code
// points, and mxbuild 11.12.6 deploy-builds each of them (measured one build per
// code, see glyph_icons.go). The whitespace code points the font also maps
// crash the build and stay refused.
func TestMDL078_AcceptsTheNamedGlyphsOutsideThePrivateUseRange_Issue2112(t *testing.T) {
	for code, name := range map[int]string{
		42: "asterisk", 43: "plus", 165: "yen", 8364: "euro", 8381: "ruble", 8722: "minus",
		8987: "hourglass", 9729: "cloud", 9978: "tent", 9993: "envelope", 9999: "pencil",
	} {
		if v := validateGlyphCodes(glyphStmt(code)); len(v) != 0 {
			t.Errorf("glyph %d (%s) was refused, but mxbuild deploy-builds it: %s", code, name, v[0].Message)
		}
		if g, ok := LookupGlyph(code); !ok || g.Name != name {
			t.Errorf("LookupGlyph(%d) = %+v, %v; want %q", code, g, ok, name)
		}
	}
}

// CONTROL: the font maps these too, and the deploy build crashes on them
// ("Sequence contains no matching element"), so the rule must keep refusing
// them — a fix that accepted the whole cmap would let them through.
func TestMDL078_StillRefusesTheWhitespaceCodesTheFontMaps_Issue2112(t *testing.T) {
	for _, code := range []int{32, 160, 8192, 8239} {
		if v := validateGlyphCodes(glyphStmt(code)); len(v) != 1 {
			t.Errorf("glyph %d was accepted, but the deploy build crashes on it", code)
		}
	}
}
