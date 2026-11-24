// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateMenuItemIcons (MDL077) flags a navigation menu item that specifies no
// icon.
//
// # Why this is worth a warning
//
// Mendix's navigation sidebar collapses to an icon rail, and that is the state
// most users leave it in. A collapsed item shows its icon; an item without one
// falls back to the first few characters of its caption, which is rarely enough
// to tell "Orders" from "Order lines". The menu still builds, `mx check` passes,
// and the only symptom is a column of truncated words in a browser.
//
// The icon is optional in the grammar and nothing said anything about leaving it
// out, so an iconless menu was the easiest one to write.
//
// # Every item, at every depth
//
// A sub-item is a menu item: it renders in the flyout the collapsed rail opens,
// and a submenu's PARENT sits directly on the rail, so it needs one most of all.
// The rule is uniform rather than scoped to the top level — if that proves noisy
// on a deep menu, narrowing it is a one-line change to the walk, but reporting
// too little is the failure that is hard to notice.
//
// # A warning, not an error
//
// The project builds and runs. This is a usability defect, not a broken model,
// and `exec` refuses only on errors — a rule that blocked the script would make
// an opinion about design into a gate.
//
// # It needs no project
//
// The icon's PRESENCE is in the script; only its target has to be resolved
// against the project's icon collections, and MDL-ICON01 already does that
// separately. So this runs in the project-free pass and fires under
// `mxcli check` with no `-p` — which is how CI runs it, and how the four widget
// rules that were inert in CI got missed.
func validateMenuItemIcons(stmt ast.Statement) []linter.Violation {
	var items []ast.NavMenuItemDef
	var where string

	switch s := stmt.(type) {
	case *ast.CreateNavigationStmt:
		// A native profile's { } block is its bottom bar, not a sidebar that
		// collapses to icons; its icon rules are MDL-NATIVEBAR01's
		// (ako/mxcli#2561), and the collection icon this rule suggests is one
		// the bottom bar refuses.
		if isNativeProfileName(s.ProfileName) {
			return nil
		}
		items, where = s.MenuItems, "navigation "+s.ProfileName
	case *ast.CreateMenuStmt:
		items, where = s.Items, "menu "+s.Name.String()
	case *ast.AlterNavigationStmt:
		// Only the items the statement adds: a stored item it does not name is
		// not re-judged, and a `set` names an item that already exists.
		items, where = menuAlterFragmentItems(s.Operations), "navigation "+s.ProfileName
	case *ast.AlterMenuStmt:
		items, where = menuAlterFragmentItems(s.Operations), "menu "+s.Name.String()
	default:
		return nil
	}

	var out []linter.Violation
	var walk func(list []ast.NavMenuItemDef)
	walk = func(list []ast.NavMenuItemDef) {
		for _, item := range list {
			// NOT `item.Icon == ""`. A glyph icon carries a numeric code and no
			// name, so the obvious test reports an item that plainly has an
			// icon — and every Studio Pro-authored menu is full of them. Ask the
			// kind, which is what the writer and DESCRIBE also read.
			if item.IconKind == types.MenuIconNone {
				out = append(out, linter.Violation{
					RuleID:   "MDL077",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf(
						"%s: menu item %q specifies no icon — a collapsed navigation sidebar "+
							"shows only the icon, so this item appears as a few characters of its caption",
						where, item.Caption),
					Suggestion: "add `icon Atlas_Core.Atlas.<name>` (list them with " +
						"`describe icon collection Atlas_Core.Atlas`)",
				})
			}
			walk(item.Items)
		}
	}
	walk(items)
	return out
}
