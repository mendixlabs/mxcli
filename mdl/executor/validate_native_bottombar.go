// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateNativeBottomBar (MDL-NATIVEBAR01) refuses, in check, the native
// bottom bar items exec refuses (ako/mxcli#2561).
//
// A NativePhone profile's { } block is its bottom bar
// (NativePages$BottomBarItem): flat, each item opening a page by name, with a
// glyph icon or none. exec refused a nested `menu`, a collection or image icon
// and any OnClick but `show page` — one at a time, the first it met — while
// check reported nothing but MDL077 warnings about a web sidebar the profile
// does not have, suggesting the collection icon exec then refused.
//
// Every one is in the script, so it needs no project, and each is reported:
// fixing them one exec at a time is what the check exists to save.
func validateNativeBottomBar(stmt ast.Statement) []linter.Violation {
	s, ok := stmt.(*ast.CreateNavigationStmt)
	if !ok || !isNativeProfileName(s.ProfileName) {
		return nil
	}
	where := "navigation " + s.ProfileName
	var out []linter.Violation
	refuse := func(item ast.NavMenuItemDef, why, fix string) {
		out = append(out, linter.Violation{
			RuleID:     "MDL-NATIVEBAR01",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: bottom bar item %q %s — the native bottom bar is flat, opens a page, and shows a glyph icon", where, item.Caption, why),
			Location:   linter.Location{DocumentType: "navigation", DocumentName: s.ProfileName},
			Suggestion: fix,
		})
	}
	for _, item := range s.MenuItems {
		if len(item.Items) > 0 {
			refuse(item, "is a menu with sub-items", "list each page as a bottom bar item of its own")
		}
		if item.IconKind == types.MenuIconCollection || item.IconKind == types.MenuIconImage {
			refuse(item, "has a collection or image icon", "write `Icon: glyph <code>` (e.g. glyph 57377 for the home glyph), or leave the icon out")
		}
		if what := nativeBarActionProblem(item); what != "" {
			refuse(item, what, "write `OnClick: show page Module.Page` with no arguments")
		}
	}
	return out
}

// nativeBarActionProblem says what is wrong with a bottom bar item's OnClick,
// "" when it opens a page by name. A menu (an item with sub-items) has no
// action of its own and is reported for its sub-items instead.
func nativeBarActionProblem(item ast.NavMenuItemDef) string {
	if len(item.Items) > 0 {
		return ""
	}
	switch {
	case item.Microflow != nil:
		return "calls a microflow"
	case item.SignOut:
		return "signs out"
	}
	if a := item.Action; a != nil {
		switch a.Type {
		case "showPage":
			if len(a.Args) > 0 {
				return "passes arguments to its page"
			}
			return ""
		case "signOut":
			return "signs out"
		default:
			return fmt.Sprintf("has a %s action", a.Type)
		}
	}
	if item.Page == nil {
		return "opens no page"
	}
	return ""
}

// isNativeProfileName reports whether a profile name is a native one.
func isNativeProfileName(name string) bool {
	_, ok := types.CanonicalNativeProfileKind(name)
	return ok
}
