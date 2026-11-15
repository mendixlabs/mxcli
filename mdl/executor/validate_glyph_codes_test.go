// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// A glyph code is a bare integer, so nothing resolved it: `icon glyph 57562`
// passed `mxcli check` AND `mx check` at 0 errors and broke the deploy with a
// message naming a layout nobody had touched (ako/CapTrackV4 007, R2).
//
// MEASURED on 11.14.0, two full builds of the same project differing only in the
// code:
//
//	57562 (0xEBDA)  ERROR: An exception occurred while exporting layout
//	                'CapTrack.App_Default'
//	                -> System.InvalidOperationException: Sequence contains no
//	                   matching element
//	                   at ...Forms.Icons.GlyphFont.GetClass(Int32 code)
//	57377 (0xE021)  pages and layouts export cleanly
//
// GetClass is a LINQ `.First(...)` over mxbuild's glyph table, which throws
// rather than reporting. The table this rule checks is the cmap of the font
// Atlas_Core ships (glyphicons-halflings-regular.woff): 247 private-use codes
// and 11 named icons outside that range (ako/mxcli#2112), and it agrees with
// mxbuild on every measured point.

func glyphStmt(code int) *ast.CreateNavigationStmt {
	return &ast.CreateNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{{
			Caption:  "Overview",
			IconKind: types.MenuIconGlyph,
			IconCode: code,
		}},
	}
}

func TestMDL078_ReportsACodeTheFontDoesNotDefine(t *testing.T) {
	v := validateGlyphCodes(glyphStmt(57562))
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1 — this code breaks the deploy and every "+
			"static gate passes it", len(v))
	}
	if !strings.Contains(v[0].Message, "57562") {
		t.Errorf("the message does not name the code: %s", v[0].Message)
	}
	// The author cannot get from mxbuild's stack trace to the cause, so the rule
	// has to carry the way out.
	if !strings.Contains(v[0].Suggestion, "icon collection") {
		t.Errorf("the suggestion does not offer the checked alternative: %s", v[0].Suggestion)
	}
}

// CONTROL: the code the blank app ships must not be reported. Without this, a
// rule that flagged every glyph would pass the test above and reject every
// working navigation in existence.
func TestMDL078_SilentOnCodesTheFontDefines(t *testing.T) {
	for _, code := range []int{
		57377, // 0xE021 — the measured working case
		57345, // 0xE001 — first code in the font
		57952, // 0xE260 — last icon
		63743, // 0xF8FF — the outlier at the end of the private use area
		57440, // 0xE060 — a single-code run, the shape a range check gets wrong
	} {
		if v := validateGlyphCodes(glyphStmt(code)); len(v) != 0 {
			t.Errorf("glyph %d was reported but the font defines it: %s", code, v[0].Message)
		}
	}
}

// The gaps are real: the private-use range is 247 codes in 35 runs, not one range. A check
// written as "between the first and last code" would accept all of these.
func TestMDL078_ReportsCodesInsideTheGaps(t *testing.T) {
	for _, code := range []int{
		57348, // 0xE004 — between the first two runs
		57441, // 0xE061 — the single-code gap after 0xE060
		57750, // 0xE196 — inside the 0xE190-0xE199 gap
		57800, // 0xE1C8 — between the 0xE1xx and 0xE2xx blocks
	} {
		if v := validateGlyphCodes(glyphStmt(code)); len(v) != 1 {
			t.Errorf("glyph %d was accepted, but it falls in a gap the font does not fill", code)
		}
	}
}

// CONTROL: a menu item with no glyph, or with an icon-collection reference, is
// not this rule's business — MDL077 owns the missing-icon case and MDL-ICON01
// resolves collection references.
func TestMDL078_IgnoresNonGlyphIcons(t *testing.T) {
	for _, kind := range []types.MenuIconKind{types.MenuIconNone, types.MenuIconCollection} {
		stmt := glyphStmt(57562)
		stmt.MenuItems[0].IconKind = kind
		if v := validateGlyphCodes(stmt); len(v) != 0 {
			t.Errorf("icon kind %v was reported by the glyph rule: %s", kind, v[0].Message)
		}
	}
	// ...and neither is a statement of another type.
	if v := validateGlyphCodes(&ast.CreateEntityStmt{}); len(v) != 0 {
		t.Errorf("a non-navigation statement was reported: %+v", v)
	}
}

// Sub-items render in the flyout a collapsed rail opens, so a bad code there
// breaks the same build. The walk is recursive for the same reason MDL077's is.
func TestMDL078_WalksSubItems(t *testing.T) {
	stmt := &ast.CreateNavigationStmt{
		ProfileName: "Responsive",
		MenuItems: []ast.NavMenuItemDef{{
			Caption:  "Group",
			IconKind: types.MenuIconGlyph,
			IconCode: 57377, // fine
			Items: []ast.NavMenuItemDef{{
				Caption:  "Nested",
				IconKind: types.MenuIconGlyph,
				IconCode: 57562, // not fine
			}},
		}},
	}
	v := validateGlyphCodes(stmt)
	if len(v) != 1 || !strings.Contains(v[0].Message, "Nested") {
		t.Errorf("a sub-item's bad glyph was missed: %+v", v)
	}
}

// The table is the whole set MDL078 accepts AND the set `show glyphs` lists, so
// a regeneration that lost entries would quietly narrow both at once.
func TestGlyphIconsMatchTheFont(t *testing.T) {
	icons := GlyphIcons()
	if len(icons) != 258 {
		t.Errorf("table holds %d icons, want 258 (the private-use range of "+
			"glyphicons-halflings-regular.woff and its 11 named icons outside it, ako/mxcli#2112)", len(icons))
	}
	seenCode := map[int]bool{}
	seenName := map[string]bool{}
	prev := -1
	for i, g := range icons {
		if g.Code <= prev {
			t.Fatalf("entry %d (%d) is out of order — LookupGlyph binary-searches, "+
				"so the table must be sorted by code", i, g.Code)
		}
		prev = g.Code
		if seenCode[g.Code] {
			t.Errorf("code %d appears twice", g.Code)
		}
		seenCode[g.Code] = true
		if g.Name == "" {
			t.Errorf("code %d has no name — every code in the font is named by Atlas's "+
				"own bootstrap stylesheet, so a blank one means the extraction lost it", g.Code)
		}
		for _, n := range append([]string{g.Name}, g.Aliases...) {
			if seenName[n] {
				t.Errorf("name %q appears twice — `describe glyph %q` would be ambiguous "+
					"between two codes", n, n)
			}
			seenName[n] = true
		}
	}
	// The measured endpoints, and the alias the extraction has to preserve.
	for code, name := range map[int]string{57345: "glass", 57377: "home", 57952: "menu-up", 63743: "apple"} {
		g, ok := LookupGlyph(code)
		if !ok || g.Name != name {
			t.Errorf("LookupGlyph(%d) = %+v, %v; want name %q", code, g, ok, name)
		}
	}
	if g, _ := LookupGlyph(57895); len(g.Aliases) != 2 {
		t.Errorf("bitcoin lost its aliases: %+v — `show glyphs like 'btc'` then finds nothing", g)
	}
}

// TestMDL078_ReachesAWidgetGlyph is the reason this rule stopped walking the
// statements itself.
//
// A glyph on a page widget fails the build exactly as a glyph on a menu item
// does — mxbuild resolves both through the same GlyphFont.GetClass — but the
// rule enumerated navigation and menu statements only, so the day
// `Icon: glyph <n>` became authorable on a widget (mendixlabs/mxcli#1059) every
// widget glyph would have passed in silence. It now reads the package's single
// icon walk, so a statement kind that walk learns about, this rule learns about.
func TestMDL078_ReachesAWidgetGlyph(t *testing.T) {
	btn := func(name string, code int) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "ACTIONBUTTON", Name: name, Properties: map[string]any{
			"Icon": &ast.WidgetIcon{Kind: types.MenuIconGlyph, Code: code},
		}}
	}
	stmt := &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
		btn("btnGood", 57377), // in the font
		{Type: "CONTAINER", Name: "c1", Children: []*ast.WidgetV3{
			btn("btnBad", 57562), // past the font's end
		}},
	}}

	v := validateGlyphCodes(stmt)
	if len(v) != 1 {
		t.Fatalf("got %d violations, want exactly 1 (the nested bad code): %+v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "btnBad") {
		t.Errorf("the violation does not name the widget: %q", v[0].Message)
	}
	if !strings.Contains(v[0].Message, "57562") {
		t.Errorf("the violation does not name the code: %q", v[0].Message)
	}
	if v[0].RuleID != "MDL078" {
		t.Errorf("RuleID = %q, want MDL078", v[0].RuleID)
	}
}

// mendixlabs/mxcli#1522: MDL078 is an ERROR, so `check` fails and `exec`
// refuses the script before anything is written. As a warning, exec applied it
// and `mxbuild --target=deploy` then died with an unhandled
// InvalidOperationException ("Sequence contains no matching element", exit 1)
// and wrote no error file, while `mx check` passed -- so a build gate had
// nothing to report.
func TestMDL078_IsAnError(t *testing.T) {
	v := validateGlyphCodes(glyphStmt(57388)) // the issue's code
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1", len(v))
	}
	if v[0].Severity != linter.SeverityError {
		t.Errorf("MDL078 severity = %v, want error -- exec only refuses on errors", v[0].Severity)
	}
	if !strings.Contains(v[0].Message, "57388") || !strings.Contains(v[0].Message, "Overview") {
		t.Errorf("the message does not name the glyph and where it is: %s", v[0].Message)
	}
	// The way out names real glyphs: the defined codes either side of it.
	// 57388 (0xE02C) sits in the gap between 57385 play-circle and 57392 repeat.
	for _, want := range []string{"57385 (play-circle)", "57392 (repeat)", "list glyphs"} {
		if !strings.Contains(v[0].Suggestion, want) {
			t.Errorf("the suggestion does not mention %q: %s", want, v[0].Suggestion)
		}
	}
}

// Past the font's last icon there is no code above to offer, only below.
func TestMDL078_SuggestsOnlyTheNeighboursThatExist(t *testing.T) {
	v := validateGlyphCodes(glyphStmt(70000))
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1", len(v))
	}
	if !strings.Contains(v[0].Suggestion, "63743") {
		t.Errorf("the suggestion does not offer the font's last code: %s", v[0].Suggestion)
	}
}

// The gate exec runs is ValidateProgram: a script with an undefined glyph on a
// menu item, a menu document or a page widget reports MDL078 as an error there,
// which is what makes exec refuse it.
func TestValidateProgram_UndefinedGlyphRefusesExec(t *testing.T) {
	for name, src := range map[string]string{
		"navigation": `create or modify navigation Responsive
  home page M.P
{ menu item 'A' ( OnClick: show page M.P, Icon: glyph 57388 ) };`,
		"menu document": `create or modify menu M.Menu {
  menu item 'A' ( OnClick: show page M.P, Icon: glyph 57388 )
};`,
		"page widget": `create page M.Q (title: 'Q', layout: Atlas_Core.Atlas_Default)
{ actionbutton btn (caption: 'Go', action: nothing, icon: glyph 57388) };`} {
		t.Run(name, func(t *testing.T) {
			got := ruleSeverities(ValidateProgram(build(t, src), ""))
			sev, ok := got["MDL078"]
			if !ok {
				t.Fatalf("no MDL078; got %v", got)
			}
			if sev != linter.SeverityError {
				t.Errorf("MDL078 severity = %v, want error", sev)
			}
		})
	}
}
