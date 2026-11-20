// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// menuMDL renders items through the DESCRIBE emitter.
func menuMDL(items []*types.NavMenuItem) string {
	var b bytes.Buffer
	printMenuMDL(nil, &b, items, 0, "CREATE NAVIGATION")
	return b.String()
}

// nativeMenuMDL is menuMDL for a NativePhone profile's bottom bar.
func nativeMenuMDL(items []*types.NavMenuItem) string {
	var b bytes.Buffer
	printMenuMDLFor(nil, &b, items, 0, "CREATE NAVIGATION", true)
	return b.String()
}

// DESCRIBE emits re-executable MDL, so an icon it read must come back out — the
// alternative is output that silently rewrites the menu when replayed. The
// hyphenated segment has to be re-quoted: `align-center` lexes as HYPHENATED_ID,
// not IDENTIFIER, so an unquoted emission would be output the parser rejects.
func TestPrintMenuMDL_RoundTripsAnIconCollectionIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption:  "Dashboard",
		Page:     "M.Dash",
		Icon:     "Atlas_Core.Atlas.align-center",
		IconType: "Forms$IconCollectionIcon",
	}})
	want := "menu item 'Dashboard' ( OnClick: show page M.Dash, Icon: Atlas_Core.Atlas.\"align-center\" )\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A name a qualifiedName segment can hold is emitted bare, and quoting it
// anyway would be noise. `home` is a KEYWORD token, not an IDENTIFIER — and
// `identifierOrKeyword` accepts keywords, so it must NOT be quoted. Most short
// Atlas names are in this bucket (`home`, `user`, `add`, `folder`), which is why
// the emitter tests parser acceptance rather than reusing mdlIdent's
// lexes-as-IDENTIFIER rule.
func TestPrintMenuMDL_LeavesAKeywordIconNameUnquoted(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption: "Home", Page: "M.Home",
		Icon: "Atlas_Core.Atlas.home", IconType: "Forms$IconCollectionIcon",
	}})
	want := "menu item 'Home' ( OnClick: show page M.Home, Icon: Atlas_Core.Atlas.home )\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A sub-menu carries its icon in its property list, before the { } of its
// items, matching the grammar's second alternative.
func TestPrintMenuMDL_RoundTripsASubMenuIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption:  "Reports",
		Icon:     "Atlas_Core.Atlas.list-bullets",
		IconType: "Forms$IconCollectionIcon",
		Items:    []*types.NavMenuItem{{Caption: "Monthly", Page: "M.Monthly"}},
	}})
	if !strings.HasPrefix(got, "menu 'Reports' ( Icon: Atlas_Core.Atlas.\"list-bullets\" ) {\n") {
		t.Errorf("sub-menu header lost its icon: %q", got)
	}
	if !strings.Contains(got, "menu item 'Monthly' ( OnClick: show page M.Monthly )") {
		t.Errorf("sub-items went missing: %q", got)
	}
}

// All three variants round-trip now. They used to be flagged with a comment
// instead, and because CREATE NAVIGATION is a full replacement, re-running that
// output DELETED the icon the comment had just declined to describe — measured
// on testdata/expr-checker, a glyph icon destroyed at exit 0.
//
// Each form is emitted with its own keyword, so replay rebuilds the same
// ELEMENT. Writing `Icon: System.Images.Close` for an ImageIcon would have
// converted it to an IconCollectionIcon: a silent variant swap, which is why the
// bare form was not simply widened to cover all three.
func TestPrintMenuMDL_EmitsEachIconVariant(t *testing.T) {
	for _, tc := range []struct{ name, iconType, icon, want string }{
		{"collection icon", "Forms$IconCollectionIcon", "Atlas_Core.Atlas.home", "Icon: Atlas_Core.Atlas.home"},
		{"image icon", "Forms$ImageIcon", "System.Images.Close", "Icon: image System.Images.Close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := menuMDL([]*types.NavMenuItem{{
				Caption: "Close", Page: "M.Close", Icon: tc.icon, IconType: tc.iconType,
			}})
			if !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to contain %q", got, tc.want)
			}
			if strings.Contains(got, "-- icon") {
				t.Errorf("still flagged as unreproducible: %q", got)
			}
		})
	}

	t.Run("glyph icon", func(t *testing.T) {
		got := menuMDL([]*types.NavMenuItem{{
			Caption: "Close", Page: "M.Close", IconType: "Forms$GlyphIcon", IconCode: 57345,
		}})
		if !strings.Contains(got, "Icon: glyph 57345") {
			t.Errorf("got %q, want it to contain `Icon: glyph 57345`", got)
		}
		if strings.Contains(got, "-- icon") {
			t.Errorf("still flagged as unreproducible: %q", got)
		}
	})
}

// The note survives for what is genuinely beyond the language, and that is the
// control: without a case that still flags, "emits everything" and "flags
// nothing" are indistinguishable.
//
// A glyph with no Code cannot be rebuilt — the code IS the glyph's identity — and
// a $Type this build does not know must never be guessed at, because emitting a
// clause for it would rebuild a different element.
func TestPrintMenuMDL_StillFlagsWhatItCannotRebuild(t *testing.T) {
	for _, tc := range []struct{ name, iconType, icon string }{
		{"glyph with no code", "Forms$GlyphIcon", ""},
		{"unknown variant", "Forms$SomeFutureIcon", "M.X.y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := menuMDL([]*types.NavMenuItem{{
				Caption: "Close", Page: "M.Close", Icon: tc.icon, IconType: tc.iconType,
			}})
			stmt := strings.SplitN(got, "\n", 2)[0]
			if strings.Contains(stmt, "Icon:") {
				t.Errorf("emitted a clause for %s, which replay could not rebuild: %q", tc.iconType, got)
			}
			if !strings.Contains(got, "-- icon") {
				t.Errorf("dropped silently: %q", got)
			}
		})
	}
}

// A native profile's bottom bar cannot author an IconCollectionIcon
// (navBottomBarItemBson accepts only a glyph) even though the web Menu
// reproduces one fine. DESCRIBE must flag it instead of emitting a property
// that would fail on replay.
func TestPrintMenuMDL_NativeNeverEmitsAnIconCollectionIcon(t *testing.T) {
	got := nativeMenuMDL([]*types.NavMenuItem{{
		Caption:  "Home",
		Page:     "NativeMobile.Home_Native",
		Icon:     "Atlas_Core.Atlas.home",
		IconType: "Forms$IconCollectionIcon",
	}})
	stmt := strings.SplitN(got, "\n", 2)[0]
	if strings.Contains(stmt, "Icon:") {
		t.Errorf("emitted an Icon: property for a native bottom bar item, which replay would reject: %q", got)
	}
	if !strings.Contains(got, "-- icon") || !strings.Contains(got, "Atlas_Core.Atlas.home") {
		t.Errorf("the unreproducible native icon was dropped silently: %q", got)
	}
}

// A native bottom bar item's glyph IS reproducible — the one icon shape
// navBottomBarItemBson accepts — so it gets `Icon: glyph <code>`, not a note,
// and that output parses back to the same glyph.
func TestPrintMenuMDL_NativeRoundTripsAGlyphIcon(t *testing.T) {
	got := nativeMenuMDL([]*types.NavMenuItem{{
		Caption:  "Home",
		Page:     "NativeMobile.Home_Native",
		IconType: "Forms$GlyphIcon",
		IconCode: 57377,
	}})
	want := "menu item 'Home' ( OnClick: show page NativeMobile.Home_Native, Icon: glyph 57377 )\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	script := "create or modify navigation NativePhone {\n" + got + "};"
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted output its own parser rejects:\n%s\nerrors: %v", script, errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateNavigationStmt)
	if !ok || len(stmt.MenuItems) != 1 {
		t.Fatalf("re-parsed to %T", prog.Statements[0])
	}
	if it := stmt.MenuItems[0]; it.IconKind != types.MenuIconGlyph || it.IconCode != 57377 {
		t.Errorf("round trip changed the glyph: got (%v, %d), want (glyph, 57377)", it.IconKind, it.IconCode)
	}
}

// Exec hands a native item's glyph to the writer as IconKind/IconCode, the
// same fields a web item's glyph travels in.
func TestExecNativeNavigation_PassesTheGlyphIcon(t *testing.T) {
	var got types.NavigationProfileSpec
	mb := nativeNav()
	mb.UpdateNavigationProfileFunc = func(_ model.ID, _ string, spec types.NavigationProfileSpec) error { got = spec; return nil }
	if _, err := runNav(t, mb, "create or modify navigation NativePhone { menu item 'Home' ( OnClick: show page M.P, Icon: glyph 57377 ) };"); err != nil {
		t.Fatal(err)
	}
	if len(got.MenuItems) != 1 || got.MenuItems[0].IconKind != types.MenuIconGlyph || got.MenuItems[0].IconCode != 57377 {
		t.Errorf("spec.MenuItems = %+v, want one item with a glyph 57377", got.MenuItems)
	}
}

// No icon means no clause and no note — the common case must stay clean.
func TestPrintMenuMDL_SilentWhenThereIsNoIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{Caption: "Dashboard", Page: "M.Dash"}})
	if got != "menu item 'Dashboard' ( OnClick: show page M.Dash )\n" {
		t.Errorf("got %q", got)
	}
}

// The point of quoting is that DESCRIBE output re-executes. Assert that
// directly by feeding the emitted statement back through the parser, rather
// than trusting a hand-written expectation about which names need quotes —
// which is exactly the thing that was wrong twice while writing this.
func TestPrintMenuMDL_EmittedIconsReParse(t *testing.T) {
	for _, icon := range []string{
		"Atlas_Core.Atlas.align-center", // HYPHENATED_ID: needs quoting
		"Atlas_Core.Atlas.home",         // keyword token: accepted bare
		"Atlas_Core.Atlas.add",          // keyword token
		"Atlas_Core.Atlas_Filled.user",  // keyword token, non-default collection
	} {
		t.Run(icon, func(t *testing.T) {
			body := menuMDL([]*types.NavMenuItem{{
				Caption: "X", Page: "M.P", Icon: icon, IconType: "Forms$IconCollectionIcon",
			}})
			script := "create or modify navigation Responsive {\n" + body + "};"
			prog, errs := visitor.Build(script)
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted output its own parser rejects:\n%s\nerrors: %v", script, errs)
			}
			stmt, ok := prog.Statements[0].(*ast.CreateNavigationStmt)
			if !ok || len(stmt.MenuItems) != 1 {
				t.Fatalf("re-parsed to %T", prog.Statements[0])
			}
			if got := stmt.MenuItems[0].Icon; got != icon {
				t.Errorf("round trip changed the icon: %q -> %q", icon, got)
			}
		})
	}
}
