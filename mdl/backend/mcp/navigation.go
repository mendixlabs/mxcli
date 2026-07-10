// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// navigationDocType is the PED type of the project-level navigation document.
// It is project-level: ped_read_document / ped_update_document take it via
// documentType with documentName OMITTED (sending an empty documentName makes
// PED look up a named document and fail). Verified against Studio Pro 11.12.
const navigationDocType = "Navigation$NavigationDocument"

// GetNavigation returns the navigation document from the local reader.
//
// Reads are hybrid (served from the last-saved .mpr), so a profile edited via
// PED this session is not reflected here until the user saves in Studio Pro —
// the same consistency hole the rest of the MCP backend lives with. The
// executor calls this before UpdateNavigationProfile only to confirm the
// profile exists and recover the document ID, for which the on-disk copy is
// sufficient.
func (b *Backend) GetNavigation() (*types.NavigationDocument, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("not connected")
	}
	return b.reader.GetNavigation()
}

// ListNavigationDocuments returns the navigation documents from the local reader.
func (b *Backend) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("not connected")
	}
	return b.reader.ListNavigationDocuments()
}

// UpdateNavigationProfile replaces a profile's home page, login page, not-found
// page, and menu tree over PED, mirroring the local engine's "create or replace
// navigation". It authors against Studio Pro's live model through the generic
// document tools (the navigation document has no dedicated PED tool); the
// navDocID is ignored (the project-level document is addressed by type).
//
// PED constraints shape the choreography (all verified against 11.12):
//   - `set` works on primitive/reference leaves, and on an element-valued
//     property only when it is currently null. So scalar leaves (homePage.page,
//     loginPageSettings.page) are set in place, while a currently-null element
//     (notFoundHomepage) is set whole with a constructor.
//   - `set` can NEVER target an array; the menu lives in menuItemCollection.items
//     (an array). Replacing it is therefore remove-all-then-add. PED forbids
//     batching add and remove on the SAME array path in one call, so the menu is
//     cleared (descending removes) in the first update and rebuilt (appends) in a
//     second.
//
// ped_update_document validates references and applies atomically (a bad page
// ref fails the whole op), and the project-level navigation document is not
// addressable by ped_check_errors, so the update result itself is the gate.
func (b *Backend) UpdateNavigationProfile(_ model.ID, profileName string, spec types.NavigationProfileSpec) error {
	if b.client == nil {
		return fmt.Errorf("not connected")
	}
	st, err := b.navProfileState(profileName)
	if err != nil {
		return err
	}
	if st.isNative {
		return fmt.Errorf("navigation profile %q is a native profile; the MCP backend authors web profiles only (native home pages and bottom-bar items are not wired yet) — edit native navigation against a local .mpr", profileName)
	}
	for _, hp := range spec.HomePages {
		if hp.ForRole != "" {
			return fmt.Errorf("role-based home pages (home ... for %s) are not yet authored over the MCP backend; set them against a local .mpr", hp.ForRole)
		}
	}

	// --- Phase 1: scalar/leaf sets + clear the existing menu (descending). ---
	var ops []pedOpEntry

	for _, hp := range spec.HomePages {
		field := "page"
		if !hp.IsPage {
			field = "microflow"
		}
		if st.homePageNull {
			// homePage is unset: set the whole element with a constructor.
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/profiles/%d/homePage", st.index),
				Operation: pedOperation{Type: "set", Value: map[string]any{"$Type": "Navigation$HomePage", field: hp.Target}},
			})
		} else {
			// Set only the chosen target. The unused alternative (page vs
			// microflow) is a reference leaf, and PED rejects an empty string as
			// an invalid reference — it cannot be cleared with a set. The home
			// page is a single choice; Studio Pro keeps the one that is set.
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/profiles/%d/homePage/%s", st.index, field),
				Operation: pedOperation{Type: "set", Value: hp.Target},
			})
		}
	}

	if spec.LoginPage != "" {
		// loginPageSettings (Pages$PageSettings) always exists on a web profile;
		// set its page leaf.
		ops = append(ops, pedOpEntry{
			Path:      fmt.Sprintf("/profiles/%d/loginPageSettings/page", st.index),
			Operation: pedOperation{Type: "set", Value: spec.LoginPage},
		})
	}

	if spec.NotFoundPage != "" {
		if st.notFoundNull {
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/profiles/%d/notFoundHomepage", st.index),
				Operation: pedOperation{Type: "set", Value: map[string]any{"$Type": "Navigation$NotFoundHomePage", "page": spec.NotFoundPage}},
			})
		} else {
			ops = append(ops, pedOpEntry{
				Path:      fmt.Sprintf("/profiles/%d/notFoundHomepage/page", st.index),
				Operation: pedOperation{Type: "set", Value: spec.NotFoundPage},
			})
		}
	}

	if spec.HasMenu {
		// Clear existing items high index -> low so each remove leaves the lower
		// indices valid (safe whether PED validates indices against the original
		// or the running length).
		for i := st.menuItemCount - 1; i >= 0; i-- {
			idx := i
			ops = append(ops, pedOpEntry{
				Path:      navItemsPath(st.index),
				Operation: pedOperation{Type: "remove", Index: &idx},
			})
		}
	}

	if len(ops) > 0 {
		if err := b.pedUpdateNav(ops...); err != nil {
			return err
		}
	}

	// --- Phase 2: rebuild the menu (appends only — never batched with the
	// removes above, per PED's same-array-path rule). ---
	if spec.HasMenu && len(spec.MenuItems) > 0 {
		addOps := make([]pedOpEntry, 0, len(spec.MenuItems))
		for _, mi := range spec.MenuItems {
			addOps = append(addOps, pedOpEntry{
				Path:      navItemsPath(st.index),
				Operation: pedOperation{Type: "add", Value: navMenuItemValue(mi)},
			})
		}
		if err := b.pedUpdateNav(addOps...); err != nil {
			return err
		}
	}

	return nil
}

// navProfileState holds the live shape of one navigation profile needed to plan
// the update: where it is, whether it is native, which element properties are
// currently null (so a set targets the whole element vs a leaf), and how many
// menu items must be cleared before a rebuild.
type navProfileState struct {
	index         int
	isNative      bool
	homePageNull  bool
	notFoundNull  bool
	menuItemCount int
}

// navProfileState reads the live navigation document to locate the named profile
// and capture the state the update planner needs.
func (b *Backend) navProfileState(name string) (*navProfileState, error) {
	byPath, err := b.pedReadNav("/profiles")
	if err != nil {
		return nil, err
	}
	var profiles []struct {
		QName string `json:"$QualifiedName"`
		Name  string `json:"name"`
		Type  string `json:"$Type"`
	}
	if err := json.Unmarshal(byPath["/profiles"], &profiles); err != nil {
		return nil, fmt.Errorf("parse navigation profiles: %w", err)
	}
	idx := -1
	isNative := false
	var names []string
	for i, p := range profiles {
		n := p.QName
		if n == "" {
			n = p.Name
		}
		names = append(names, n)
		if strings.EqualFold(n, name) {
			idx = i
			isNative = p.Type == "Navigation$NativeNavigationProfile"
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("navigation profile not found: %s (available: %s)", name, strings.Join(names, ", "))
	}

	st := &navProfileState{index: idx, isNative: isNative}
	if isNative {
		return st, nil // native profile shape differs; caller rejects before using the rest
	}

	byPath, err = b.pedReadNav(
		fmt.Sprintf("/profiles/%d", idx),
		navItemsPath(idx),
	)
	if err != nil {
		return nil, err
	}
	var prof struct {
		HomePage         json.RawMessage `json:"homePage"`
		NotFoundHomepage json.RawMessage `json:"notFoundHomepage"`
	}
	if err := json.Unmarshal(byPath[fmt.Sprintf("/profiles/%d", idx)], &prof); err != nil {
		return nil, fmt.Errorf("parse navigation profile %s: %w", name, err)
	}
	st.homePageNull = isJSONNull(prof.HomePage)
	st.notFoundNull = isJSONNull(prof.NotFoundHomepage)

	var items []json.RawMessage
	if raw := byPath[navItemsPath(idx)]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &items)
	}
	st.menuItemCount = len(items)
	return st, nil
}

// navMenuItemValue builds a Menus$MenuItem constructor for ped_update_document.
// The caption is a plain string (PED wraps it into a Texts$Text); the action is
// chosen from the item's target; sub-items recurse.
func navMenuItemValue(mi types.NavMenuItemSpec) map[string]any {
	item := map[string]any{
		"$Type":   "Menus$MenuItem",
		"caption": mi.Caption,
	}
	if act := navMenuAction(mi); act != nil {
		item["action"] = act
	}
	if len(mi.Items) > 0 {
		subs := make([]any, 0, len(mi.Items))
		for _, sub := range mi.Items {
			subs = append(subs, navMenuItemValue(sub))
		}
		item["items"] = subs
	}
	return item
}

// navMenuAction maps a menu item's target to its PED client-action constructor.
// A page target becomes a Pages$PageClientAction (page ref in pageSettings); a
// microflow target a Pages$MicroflowClientAction; a bare container item with no
// target a Pages$NoClientAction.
func navMenuAction(mi types.NavMenuItemSpec) map[string]any {
	switch {
	case mi.Page != "":
		return map[string]any{
			"$Type":        "Pages$PageClientAction",
			"pageSettings": map[string]any{"$Type": "Pages$PageSettings", "page": mi.Page},
		}
	case mi.Microflow != "":
		return map[string]any{
			"$Type":             "Pages$MicroflowClientAction",
			"microflowSettings": map[string]any{"$Type": "Pages$MicroflowSettings", "microflow": mi.Microflow},
		}
	default:
		return map[string]any{"$Type": "Pages$NoClientAction"}
	}
}

// pedReadNav reads JSON-pointer paths from the project-level navigation document.
func (b *Backend) pedReadNav(paths ...string) (map[string]json.RawMessage, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": navigationDocType,
		"paths":        paths,
	})
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || strings.HasPrefix(strings.TrimSpace(text), "ERROR") {
		return nil, fmt.Errorf("ped_read_document %s: %s", navigationDocType, text)
	}
	return parsePedResults(text)
}

// pedUpdateNav applies operations to the project-level navigation document.
// documentName is omitted (project-level); the update result text is the gate.
func (b *Backend) pedUpdateNav(ops ...pedOpEntry) error {
	res, err := b.client.CallTool("ped_update_document", map[string]any{
		"documentType": navigationDocType,
		"operations":   ops,
	})
	if err != nil {
		// Studio Pro sometimes answers a navigation write with "Request timed out"
		// (its own ~30s server limit) even though the edit applies in the
		// background — re-rendering navigation is slow. The op is not idempotent
		// (the menu is cleared then rebuilt), so we cannot safely auto-retry;
		// surface a clear hint instead of a bare timeout.
		if strings.Contains(strings.ToLower(err.Error()), "timed out") {
			return fmt.Errorf("%w — Studio Pro timed out applying the navigation change; it may have applied anyway. "+
				"Verify the menu in Studio Pro (and re-run if it is incomplete)", err)
		}
		return err
	}
	return pedOpError("ped_update_document", navigationDocType, res)
}

// navItemsPath is the JSON pointer to a profile's top-level menu items array.
func navItemsPath(profileIndex int) string {
	return fmt.Sprintf("/profiles/%d/menuItemCollection/items", profileIndex)
}

// isJSONNull reports whether a raw JSON value is absent or literally null.
func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || strings.TrimSpace(string(raw)) == "null"
}
