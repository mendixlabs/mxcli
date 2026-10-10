// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// execAlterNavigation handles CREATE [OR REPLACE] NAVIGATION <profile> command.
// It fully replaces the profile's home pages, login page, not-found page, and menu tree.
func execAlterNavigation(ctx *ExecContext, s *ast.AlterNavigationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Resolve `FOR <user role>` before anything is written — including the
	// profile this may add below. A module-qualified role here produces a project
	// Mendix cannot load at all, so refusing is the only useful outcome
	// (mendixlabs/mxcli#1001). Same function `check` calls.
	if err := validateNavigationRoleForExec(ctx, s); err != nil {
		return err
	}

	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	// The menu is converted before anything is written — adding a profile
	// included — so an action the builder refuses leaves the project as it was.
	// Each item is paired with the stored item it replaces (ako/mxcli#980).
	var stored []*types.NavMenuItem
	isNative := false
	storedKind := ""
	var storedPWA *types.NavPWASettings
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, s.ProfileName) {
			stored, isNative = p.MenuItems, p.IsNative
			storedKind, storedPWA = p.Kind, p.ProgressiveWebApp
			break
		}
	}
	if err := checkProfileClauses(ctx, s, isNative); err != nil {
		return err
	}
	kept := map[string]string{}
	menuItems, err := convertMenuItemDefs(newMenuActionBuilder(ctx), s.MenuItems, stored, "", kept)
	if err != nil {
		return err
	}

	// Verify the profile exists
	createdProfile := false
	createdKind := ""
	profileFound := false
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, s.ProfileName) {
			profileFound = true
			break
		}
	}
	if !profileFound {
		// CREATE OR REPLACE could only ever REPLACE: a profile that did not exist
		// was an error, and nothing else in mxcli added one. That put a whole class
		// of app out of reach, because Mendix routes a phone to a Phone PROFILE on
		// User-Agent — not on viewport width — so a device-specific front end could
		// not be built from MDL at all (ako/mxcli-maintenance §7).
		//
		// Mendix's web profiles are a closed set, so an unknown name is still an
		// error: creating "Phone" is adding the profile the platform defines,
		// while creating "Mobile" would be inventing one that can never route.
		kind, ok := types.CanonicalProfileKind(s.ProfileName)
		if !ok {
			return mdlerrors.NewNotFoundMsg("navigation profile", s.ProfileName,
				fmt.Sprintf("navigation profile not found: %s (available: %s; Mendix's web profiles are %s)",
					s.ProfileName, profileNames(nav), strings.Join(types.WebProfileKindNames(), ", ")))
		}
		if err := ctx.Backend.AddNavigationProfile(nav.ID, kind); err != nil {
			return mdlerrors.NewBackend("create navigation profile", err)
		}
		createdProfile, createdKind = true, kind
		// Re-read: the spec below is applied against the stored document, and the
		// profile it targets has only just come into existence.
		if nav, err = ctx.Backend.GetNavigation(); err != nil {
			return mdlerrors.NewBackend("reload navigation", err)
		}
	}

	// Convert AST types to writer spec
	spec := types.NavigationProfileSpec{
		HasMenu:   s.HasMenuBlock,
		MenuItems: menuItems,
	}

	for _, hp := range s.HomePages {
		hpSpec := types.NavHomePageSpec{
			IsPage: hp.IsPage,
			Target: hp.Target.String(),
		}
		if hp.ForRole != nil {
			hpSpec.ForRole = hp.ForRole.String()
		}
		spec.HomePages = append(spec.HomePages, hpSpec)
	}

	if s.LoginPage != nil {
		spec.LoginPage = s.LoginPage.String()
	}
	if s.NotFoundPage != nil {
		spec.NotFoundPage = s.NotFoundPage.String()
	}

	spec.ThrowSyncError = s.ThrowSyncError
	spec.ProgressiveWebApp = s.ProgressiveWebApp
	spec.HasSync = s.HasSyncBlock
	for _, se := range s.SyncEntries {
		spec.OfflineEntities = append(spec.OfflineEntities, types.NavOfflineEntitySpec{
			Entity:     se.Entity.String(),
			SyncMode:   se.Mode,
			Constraint: se.Constraint,
		})
	}

	if err := ctx.Backend.UpdateNavigationProfile(nav.ID, s.ProfileName, spec); err != nil {
		return mdlerrors.NewBackend("update navigation profile", err)
	}

	// Say which of the two happened. A run that silently reports "updated" after
	// CREATING a profile hides the fact that the project gained a routing target
	// it did not have — and a phone profile changes which pages a phone lands on.
	if createdProfile {
		fmt.Fprintf(ctx.Output, "Navigation profile %s created.\n", mdlQuoted(s.ProfileName))
		// An offline profile changes what the platform demands of pages this
		// statement never mentioned. Say so now, not at the next build.
		warnOfflineIncompatiblePages(ctx, createdKind)
	} else if !ctx.reportWrite("navigation profile "+mdlQuoted(s.ProfileName),
		"Navigation profile %s updated.", mdlQuoted(s.ProfileName)) {
		// A re-run whose write was elided rewrote nothing, so it carried nothing
		// either (ako/mxcli#890 is the same rule for security and settings).
		// The profile is still unusable offline, though, so that is said again.
		warnOfflineWithoutPWA(ctx, s, storedKind, storedPWA)
		return nil
	}
	reportKeptMenuActions(ctx, kept)
	kind := storedKind
	if createdProfile {
		kind = createdKind
	}
	warnOfflineWithoutPWA(ctx, s, kind, storedPWA)
	return nil
}

// warnOfflineWithoutPWA says when an offline profile is left without Progressive
// web app settings. Mendix then registers no service worker -- index.js carries
// "registerServiceWorker": false -- so no page opens without a network, while
// check, exec and mx check all pass (mendixlabs/mxcli#1377).
func warnOfflineWithoutPWA(ctx *ExecContext, s *ast.AlterNavigationStmt, kind string, stored *types.NavPWASettings) {
	if !types.IsOfflineProfileKind(kind) {
		return
	}
	has := stored != nil
	if s.ProgressiveWebApp != nil {
		has = !s.ProgressiveWebApp.Off
	}
	if has {
		return
	}
	fmt.Fprintf(ctx.Output, "  warning: navigation %s is an offline profile without Progressive web app settings: Mendix registers "+
		"no service worker for it, so no page opens without a network -- add `progressive web app ( Precaching: true )` to the statement\n",
		mdlQuoted(s.ProfileName))
}

// checkProfileClauses refuses what the statement says that the profile cannot
// take, before anything is written (ako/mxcli#980).
//
// A native profile's writer sets its home pages and offline sync and nothing
// else, so a menu block, login or not-found page or on-sync-error clause on one
// was dropped in silence: the statement reported "updated" and the bottom bar
// stayed as it was. Its flow home is a nanoflow — `home microflow` there is
// what describe used to print, so it is read as the nanoflow and warned about —
// while a web profile's is a microflow, so `home nanoflow` there is refused.
func checkProfileClauses(ctx *ExecContext, s *ast.AlterNavigationStmt, isNative bool) error {
	if !isNative {
		for _, hp := range s.HomePages {
			if hp.IsNanoflow {
				return mdlerrors.NewValidationf("navigation %s: home nanoflow %s — a web profile's home is a page or a microflow; "+
					"a nanoflow home belongs to a native profile", s.ProfileName, hp.Target.String())
			}
		}
		return nil
	}
	refuse := func(what string) error {
		return mdlerrors.NewValidationf("navigation %s is a native profile: %s is not written by mxcli for a native profile, "+
			"and leaving it out is how it used to be dropped without a word — remove it from the statement and set it in Studio Pro", s.ProfileName, what)
	}
	switch {
	case s.HasMenuBlock:
		return refuse("its bottom bar (the { } block)")
	case s.LoginPage != nil:
		return refuse("login page")
	case s.NotFoundPage != nil:
		return refuse("not found page")
	case s.ThrowSyncError != nil:
		return refuse("on sync error")
	case s.ProgressiveWebApp != nil:
		return refuse("progressive web app")
	}
	for _, hp := range s.HomePages {
		if !hp.IsPage && !hp.IsNanoflow {
			fmt.Fprintf(ctx.Output, "  warning: navigation %s is a native profile, whose home is a page or a nanoflow — "+
				"home microflow %s is written as the nanoflow %s; write home nanoflow\n", s.ProfileName, hp.Target.String(), hp.Target.String())
		}
	}
	return nil
}

// reportKeptMenuActions says which stored actions a rewrite carried because the
// script could not state them, so a kept action is never a surprise.
func reportKeptMenuActions(ctx *ExecContext, kept map[string]string) {
	paths := make([]string, 0, len(kept))
	for p := range kept {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(ctx.Output, "  kept the stored action of menu item %s (%s): MDL cannot express it, and the item states none\n", p, kept[p])
	}
}

// convertMenuItemDefs converts a list of sibling menu items, pairing each with
// the stored item of the same caption at the same place in the tree.
//
// The pairing is what keeps a rewrite from deleting what MDL cannot spell
// (ako/mxcli#980):
//   - an item that states no action keeps a stored action describe cannot
//     print (KeepAction) instead of being given Forms$NoAction — that
//     replacement used to be silent, and a renamed menu lost its nanoflow item;
//   - an item that states an action carries the stored one alongside
//     (StoredAction), so the writer can keep what the script cannot say about
//     the same action, such as a page title override.
//
// Each stated action is built by the page builder, as a button's is, so it is
// resolved and validated the same way. kept collects the carried items.
func convertMenuItemDefs(pb *menuActionBuilder, defs []ast.NavMenuItemDef, stored []*types.NavMenuItem, path string, kept map[string]string) ([]types.NavMenuItemSpec, error) {
	used := make([]bool, len(stored))
	var out []types.NavMenuItemSpec
	for _, def := range defs {
		match := pairStoredMenuItem(def.Caption, stored, used)
		itemPath := path + "'" + def.Caption + "'"
		spec := convertMenuItemDef(def)
		action, err := pb.build(def, itemPath)
		if err != nil {
			return nil, err
		}
		spec.Action = action
		var storedSubs []*types.NavMenuItem
		if match != nil {
			storedSubs = match.Items
			spec.StoredAction = match.StoredAction
			if keep, kind := keepStoredMenuAction(pb.ctx, def, match); keep {
				spec.KeepAction = match.StoredAction
				kept[itemPath] = kind
			}
		}
		spec.Items, err = convertMenuItemDefs(pb, def.Items, storedSubs, itemPath+" > ", kept)
		if err != nil {
			return nil, err
		}
		out = append(out, spec)
	}
	return out, nil
}

// pairStoredMenuItem finds the first stored sibling with the caption that no
// earlier item of the script has claimed.
func pairStoredMenuItem(caption string, stored []*types.NavMenuItem, used []bool) *types.NavMenuItem {
	for i, st := range stored {
		if !used[i] && st.Caption == caption {
			used[i] = true
			return st
		}
	}
	return nil
}

// keepStoredMenuAction reports whether the stored item's action is carried
// verbatim: the script states none, and describe cannot print the stored one.
func keepStoredMenuAction(ctx *ExecContext, def ast.NavMenuItemDef, match *types.NavMenuItem) (bool, string) {
	if menuItemStatesAction(def) || len(match.StoredAction) == 0 {
		return false, ""
	}
	// A sub-menu cannot have an action at all — mx check CE0548 "Items with
	// subitems cannot have an action themselves" — so nothing stored is kept on
	// it, least of all a page item's action paired by caption
	// (mendixlabs/mxcli#1341).
	if len(def.Items) > 0 {
		return false, ""
	}
	if _, note := menuItemActionMDL(ctx, match); note != "" {
		return true, menuActionTypeName(match)
	}
	return false, ""
}

// menuItemStatesAction reports whether the script gave the item an action.
func menuItemStatesAction(def ast.NavMenuItemDef) bool {
	return def.Action != nil || def.Page != nil || def.Microflow != nil || def.SignOut
}

// menuActionTypeName is the stored action's $Type, for a message.
func menuActionTypeName(item *types.NavMenuItem) string {
	if t, _ := item.ActionDoc["$Type"].(string); t != "" {
		return t
	}
	return item.ActionType
}

// menuItemActionMDL renders a stored menu item's action with the client-action
// renderer a button's goes through (ako/mxcli#980): onClick is the `OnClick:`
// value ("" for no action or Forms$NoAction), note a comment for what MDL
// cannot express — an action with no MDL form at all, or one whose page title
// override or link type a rewrite can only keep while the action is unchanged.
func menuItemActionMDL(ctx *ExecContext, item *types.NavMenuItem) (onClick, note string) {
	doc := item.ActionDoc
	if doc == nil {
		if len(item.StoredAction) > 0 {
			return "", fmt.Sprintf("-- menu item %s: its action (%s) could not be read; "+
				"a rewrite keeps the stored action while the item states none", menuCaptionNote(item.Caption), item.ActionType)
		}
		return "", ""
	}
	typeName, _ := doc["$Type"].(string)
	rendered := renderClientActionMDL(ctx, doc)
	switch {
	case strings.HasPrefix(rendered, "--"):
		return "", fmt.Sprintf("-- menu item %s: %s; a rewrite keeps the stored action while the item states none",
			menuCaptionNote(item.Caption), strings.TrimSpace(strings.TrimPrefix(rendered, "--")))
	case rendered == "" && !isNoMenuAction(typeName):
		return "", fmt.Sprintf("-- menu item %s: its action (%s) has no MDL form; "+
			"a rewrite keeps the stored action while the item states none", menuCaptionNote(item.Caption), typeName)
	}
	if rendered != "" && !menuOnClickParses(rendered) {
		// A stored action with its target unset renders as a bare `show page`
		// or `create object`, which is not MDL: say so and keep it.
		return "", fmt.Sprintf("-- menu item %s: its action (%s, %s) has no target MDL can name; "+
			"a rewrite keeps the stored action while the item states none", menuCaptionNote(item.Caption), typeName, rendered)
	}
	if extras := menuActionExtras(ctx, doc); len(extras) > 0 {
		note = fmt.Sprintf("-- menu item %s: %s has no MDL form; a rewrite keeps it while the item's action is unchanged",
			menuCaptionNote(item.Caption), strings.Join(extras, " and "))
	}
	return rendered, note
}

// menuCaptionNote writes a menu caption inside a `--` note the way the item
// line writes it, apostrophes doubled (mendixlabs/mxcli#1343), so a note reads
// against its item. The escaped spelling keeps a line break in the caption from
// ending the comment and leaving the rest of the note to be parsed as MDL.
func menuCaptionNote(caption string) string {
	return mdl0Quote(caption)
}

// menuOnClickParses reports whether an OnClick value reads back as a menu
// item's action.
func menuOnClickParses(onClick string) bool {
	_, errs := visitor.Build("create or modify navigation P {\n  menu item 'x' ( OnClick: " + onClick + " )\n};")
	return len(errs) == 0
}

// isNoMenuAction reports Studio Pro's "Do nothing" ($Type Forms$NoAction).
func isNoMenuAction(typeName string) bool {
	switch typeName {
	case "", "Forms$NoAction", "Forms$NoClientAction", "Pages$NoClientAction":
		return true
	}
	return false
}

// menuActionExtras names what a stored action carries that the action
// expression cannot spell. The writer keeps those through a rewrite when the
// rest of the action is unchanged (menuActionBSON's maskedMenuActionKeys).
func menuActionExtras(ctx *ExecContext, doc map[string]any) []string {
	var out []string
	for _, key := range []string{"FormSettings", "PageSettings"} {
		if fs := actionMapForKey(doc, key); fs != nil {
			if to := actionMapForKey(fs, "TitleOverride"); to != nil {
				title := settingText(ctx, actionMapForKey(to, "Text"))
				if title == "" {
					title = settingText(ctx, to)
				}
				out = append(out, fmt.Sprintf("a page title override (%s)", mdlQuote(ctx, title)))
			}
		}
	}
	if n, _ := doc["NumberOfPagesToClose2"].(string); n != "" {
		out = append(out, fmt.Sprintf("closing %s page(s)", n))
	}
	if lt, _ := doc["LinkType"].(string); lt != "" && lt != "Web" {
		out = append(out, fmt.Sprintf("link type %s", lt))
	}
	return out
}

// menuActionBuilder builds a menu item's stated action with the page builder,
// the way a button's action is built: resolved, validated, settings applied.
type menuActionBuilder struct {
	ctx *ExecContext
	pb  *pageBuilder
}

func newMenuActionBuilder(ctx *ExecContext) *menuActionBuilder {
	return &menuActionBuilder{ctx: ctx, pb: &pageBuilder{
		ctx:       ctx,
		backend:   ctx.Backend,
		execCache: ctx.Cache,
		// A menu has no context object, so a page argument is refused
		// (MDL-PAGEARG01) rather than stored as one nothing can bind.
		argCtx: atDocumentRoot(),
		// Pages, microflows and nanoflows are stored by name, as the menu
		// writer always has: a target the script creates later still resolves.
		tolerateDanglingRefs: true,
	}}
}

// execCtx is the builder's context, nil for a nil builder.
func (m *menuActionBuilder) execCtx() *ExecContext {
	if m == nil {
		return nil
	}
	return m.ctx
}

// build returns the item's action as a pages.ClientAction, or nil when the
// script states none.
func (m *menuActionBuilder) build(def ast.NavMenuItemDef, itemPath string) (any, error) {
	if m == nil {
		return nil, nil // a conversion with no project to resolve against
	}
	act := def.Action
	if act == nil {
		// An AST built by hand (or by an older visitor) carries only the
		// simple targets.
		switch {
		case def.Page != nil:
			act = &ast.ActionV3{Type: "showPage", Target: def.Page.String()}
		case def.Microflow != nil:
			act = &ast.ActionV3{Type: "microflow", Target: def.Microflow.String()}
		case def.SignOut:
			act = &ast.ActionV3{Type: "signOut"}
		default:
			return nil, nil
		}
	}
	m.pb.currentWidget = "menu item " + itemPath
	a, err := m.pb.buildClientActionV3(act)
	if err != nil {
		return nil, fmt.Errorf("menu item %s: %w", itemPath, err)
	}
	return a, nil
}

// convertMenuItemDef converts an AST NavMenuItemDef to a writer NavMenuItemSpec.
func convertMenuItemDef(def ast.NavMenuItemDef) types.NavMenuItemSpec {
	spec := types.NavMenuItemSpec{
		Caption:  def.Caption,
		Icon:     def.Icon,
		IconKind: def.IconKind,
		IconCode: def.IconCode,
	}
	if def.Page != nil {
		spec.Page = def.Page.String()
	}
	if def.Microflow != nil {
		spec.Microflow = def.Microflow.String()
	}
	spec.SignOut = def.SignOut
	for _, sub := range def.Items {
		spec.Items = append(spec.Items, convertMenuItemDef(sub))
	}
	return spec
}

// profileNames returns a comma-separated list of profile names for error messages.
func profileNames(nav *types.NavigationDocument) string {
	names := make([]string, len(nav.Profiles))
	for i, p := range nav.Profiles {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

// listNavigation handles SHOW NAVIGATION command.
// Displays an overview of all navigation profiles with their home pages and menu item counts.
func listNavigation(ctx *ExecContext) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	if len(nav.Profiles) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No navigation profiles found.")
		return nil
	}

	type row struct {
		name      string
		kind      string
		homePage  string
		loginPage string
		menuItems int
		roleHomes int
	}
	var rows []row

	for _, p := range nav.Profiles {
		homePage := ""
		if p.HomePage != nil {
			if p.HomePage.Page != "" {
				homePage = p.HomePage.Page
			} else if p.HomePage.Microflow != "" {
				homePage = "MF:" + p.HomePage.Microflow
			}
		}

		loginPage := p.LoginPage
		if loginPage == "" {
			loginPage = "-"
		}

		menuCount := countMenuItems(p.MenuItems)

		kind := p.Kind
		if p.IsNative {
			kind += " (native)"
		}

		rows = append(rows, row{p.Name, kind, homePage, loginPage, menuCount, len(p.RoleBasedHomePages)})
	}

	result := &TableResult{
		Columns: []string{"Profile", "Kind", "HomePage", "LoginPage", "MenuItems", "RoleHomes"},
		Summary: fmt.Sprintf("(%d navigation profiles)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.kind, r.homePage, r.loginPage, r.menuItems, r.roleHomes})
	}
	return writeResult(ctx, result)
}

// listNavigationMenu handles SHOW NAVIGATION MENU [profile] command.
// Displays the menu tree for a specific profile, or all profiles if none specified.
func listNavigationMenu(ctx *ExecContext, profileName *ast.QualifiedName) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	for _, p := range nav.Profiles {
		if profileName != nil && !strings.EqualFold(p.Name, profileName.Name) {
			continue
		}

		fmt.Fprintf(ctx.Output, "-- Navigation Menu: %s (%s)\n", p.Name, p.Kind)
		if len(p.MenuItems) == 0 {
			fmt.Fprintln(ctx.Output, "  (no menu items)")
		} else {
			printMenuTree(ctx.Output, p.MenuItems, 0)
		}
		fmt.Fprintln(ctx.Output)
	}

	return nil
}

// listNavigationHomes handles SHOW NAVIGATION HOMES command.
// Displays all home page configurations including role-based overrides.
func listNavigationHomes(ctx *ExecContext) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	for _, p := range nav.Profiles {
		fmt.Fprintf(ctx.Output, "-- Profile: %s (%s)\n", p.Name, p.Kind)

		// Default home page
		if p.HomePage != nil {
			if p.HomePage.Page != "" {
				fmt.Fprintf(ctx.Output, "  Default Home: page %s\n", p.HomePage.Page)
			} else if p.HomePage.Microflow != "" {
				fmt.Fprintf(ctx.Output, "  Default Home: %s %s\n", flowHomeWord(p), p.HomePage.Microflow)
			}
		} else {
			fmt.Fprintln(ctx.Output, "  Default Home: (none)")
		}

		// Role-based home pages
		if len(p.RoleBasedHomePages) > 0 {
			fmt.Fprintln(ctx.Output, "  Role-Based Homes:")
			for _, rh := range p.RoleBasedHomePages {
				target := ""
				if rh.Page != "" {
					target = "page " + rh.Page
				} else if rh.Microflow != "" {
					target = flowHomeWord(p) + " " + rh.Microflow
				}
				fmt.Fprintf(ctx.Output, "    %s -> %s\n", rh.UserRole, target)
			}
		}

		fmt.Fprintln(ctx.Output)
	}

	return nil
}

// flowHomeWord names the kind of flow a profile's home can be.
func flowHomeWord(p *types.NavigationProfile) string {
	if p.IsNative {
		return "nanoflow"
	}
	return "microflow"
}

// describeNavigation handles DESCRIBE NAVIGATION [profile] command.
// Outputs a complete MDL-style description of a navigation profile.
func describeNavigation(ctx *ExecContext, name ast.QualifiedName) error {
	nav, err := ctx.Backend.GetNavigation()
	if err != nil {
		return mdlerrors.NewBackend("get navigation", err)
	}

	// If no profile name, describe all profiles
	if name.Name == "" {
		for _, p := range nav.Profiles {
			outputNavigationProfile(ctx, p)
		}
		return nil
	}

	// Find specific profile
	for _, p := range nav.Profiles {
		if strings.EqualFold(p.Name, name.Name) {
			outputNavigationProfile(ctx, p)
			return nil
		}
	}

	return mdlerrors.NewNotFound("navigation profile", name.Name)
}

// outputNavigationProfile outputs a single profile in round-trippable CREATE OR REPLACE NAVIGATION format.
func outputNavigationProfile(ctx *ExecContext, p *types.NavigationProfile) {
	fmt.Fprintf(ctx.Output, "-- navigation PROFILE: %s\n", p.Name)
	fmt.Fprintf(ctx.Output, "--   Kind: %s\n", p.Kind)
	if p.IsNative {
		fmt.Fprintf(ctx.Output, "--   Native: Yes\n")
	}

	fmt.Fprintf(ctx.Output, "create or modify navigation %s\n", p.Name)

	// Home page. A native profile's flow home is a nanoflow; the reader keeps
	// it in Microflow, and printing it as `home microflow` named the wrong kind
	// of flow (ako/mxcli#980).
	flowHome := "microflow"
	if p.IsNative {
		flowHome = "nanoflow"
	}
	if p.HomePage != nil {
		if p.HomePage.Page != "" {
			fmt.Fprintf(ctx.Output, "  home page %s\n", p.HomePage.Page)
		} else if p.HomePage.Microflow != "" {
			fmt.Fprintf(ctx.Output, "  home %s %s\n", flowHome, p.HomePage.Microflow)
		}
	}

	// Role-based home pages
	for _, rh := range p.RoleBasedHomePages {
		if rh.Page != "" {
			fmt.Fprintf(ctx.Output, "  home page %s for %s\n", rh.Page, rh.UserRole)
		} else if rh.Microflow != "" {
			fmt.Fprintf(ctx.Output, "  home %s %s for %s\n", flowHome, rh.Microflow, rh.UserRole)
		}
	}

	// Login page
	if p.LoginPage != "" {
		fmt.Fprintf(ctx.Output, "  login page %s\n", p.LoginPage)
	}

	// Not-found page
	if p.NotFoundPage != "" {
		fmt.Fprintf(ctx.Output, "  not found page %s\n", p.NotFoundPage)
	}

	// Only emitted when it differs from the platform default, so the clause
	// appears exactly when it carries information. Describing every profile
	// with `on sync error throw` would add a line to every navigation script
	// that says what would happen anyway.
	// Web profiles only: the reader does not read it for a native profile
	// (unmeasured there), and exec refuses the clause on one.
	if !p.ThrowPartialSyncError && !p.IsNative {
		fmt.Fprintln(ctx.Output, "  on sync error continue")
	}

	// Emitted when the settings are stored (null is the default), with only
	// the keys that differ from the platform defaults (R12).
	if pwa := p.ProgressiveWebApp; pwa != nil && !p.IsNative {
		var keys []string
		if pwa.Precaching != types.NavPWADefaultPrecaching {
			keys = append(keys, fmt.Sprintf("Precaching: %t", pwa.Precaching))
		}
		if pwa.InstallPrompt != types.NavPWADefaultInstallPrompt {
			keys = append(keys, fmt.Sprintf("InstallPrompt: %t", pwa.InstallPrompt))
		}
		if len(keys) == 0 {
			fmt.Fprintln(ctx.Output, "  progressive web app")
		} else {
			fmt.Fprintf(ctx.Output, "  progressive web app ( %s )\n", strings.Join(keys, ", "))
		}
	}

	// Offline entities. These are re-executable now, so they are emitted as a
	// SYNC block rather than as the commented-out approximation that made
	// describe -> exec lossy for every project using offline sync.
	if len(p.OfflineEntities) > 0 {
		fmt.Fprintln(ctx.Output, "  sync (")
		for _, oe := range p.OfflineEntities {
			fmt.Fprintf(ctx.Output, "    sync %s %s;\n", oe.Entity, syncModeMDL(ctx, oe.SyncMode, oe.Constraint))
		}
		fmt.Fprintln(ctx.Output, "  )")
		// CompatibilityMode has no syntax: it is carried through a rewrite
		// untouched, but a reader should know it is set rather than discover it
		// missing later. Flagged, never silently dropped.
		for _, oe := range p.OfflineEntities {
			if oe.CompatibilityMode {
				fmt.Fprintf(ctx.Output,
					"  -- %s has compatibility mode on; mxcli preserves it but cannot author it\n", oe.Entity)
			}
		}
	}

	// A native profile's bottom bar is not written by mxcli, so it is not a
	// { } block exec would have to refuse: it is listed as comments, each item
	// as it would read (ako/mxcli#980).
	if p.IsNative && len(p.MenuItems) > 0 {
		fmt.Fprintln(ctx.Output, "  -- bottom bar (not written by mxcli; set it in Studio Pro):")
		var bar strings.Builder
		printMenuMDL(ctx, &bar, p.MenuItems, 0, "CREATE NAVIGATION")
		for _, line := range strings.Split(strings.TrimRight(bar.String(), "\n"), "\n") {
			fmt.Fprintf(ctx.Output, "  --   %s\n", strings.TrimPrefix(line, "-- "))
		}
		fmt.Fprintln(ctx.Output, ";")
		fmt.Fprintln(ctx.Output)
		return
	}

	// Menu items: the profile's children, in { } after its clauses (R2).
	if len(p.MenuItems) > 0 {
		fmt.Fprintln(ctx.Output, "{")
		printMenuMDL(ctx, ctx.Output, p.MenuItems, 1, "CREATE NAVIGATION")
		fmt.Fprintln(ctx.Output, "};")
	} else {
		fmt.Fprintln(ctx.Output, ";")
	}
	fmt.Fprintln(ctx.Output)
}

// countMenuItems counts the total number of menu items recursively.
func countMenuItems(items []*types.NavMenuItem) int {
	count := len(items)
	for _, item := range items {
		count += countMenuItems(item.Items)
	}
	return count
}

// printMenuTree prints a menu tree with indentation to an io.Writer.
func printMenuTree(w io.Writer, items []*types.NavMenuItem, depth int) {
	indent := strings.Repeat("  ", depth+1)
	for _, item := range items {
		target := menuItemTarget(item)
		fmt.Fprintf(w, "%s%s%s\n", indent, item.Caption, target)
		if len(item.Items) > 0 {
			printMenuTree(w, item.Items, depth+1)
		}
	}
}

// menuItemTarget returns a display string for a menu item's action target.
func menuItemTarget(item *types.NavMenuItem) string {
	if item.Page != "" {
		return " -> " + item.Page
	}
	if item.Microflow != "" {
		return " -> MF:" + item.Microflow
	}
	if item.ActionType == "SignOutAction" {
		return " -> sign out"
	}
	return ""
}

// printMenuMDL prints menu items in MDL-style format. reproducer names the
// construct an icon note should point at — navigation menus are authored by
// CREATE NAVIGATION, while a standalone menu document cannot be authored at all.
//
// Each item is a child with the shape every child has (R2, ako/mxcli#754):
// `menu item 'X' ( OnClick: show page M.P, Icon: I )`, and a sub-menu
// `menu 'X' ( Icon: I ) { … }`. A child ends in `)` or `}`, or in its caption
// when it has no properties, so no separator is written.
//
// The action is rendered by the client-action renderer a button's goes through
// (renderClientActionMDL), so every kind a menu item stores — a nanoflow call,
// open link, create object — and every `with ( … )` setting prints, and what
// MDL cannot express is said in a comment rather than left out (ako/mxcli#980).
func printMenuMDL(ctx *ExecContext, w io.Writer, items []*types.NavMenuItem, depth int, reproducer string) {
	indent := strings.Repeat("  ", depth)
	for _, item := range items {
		var props []string
		onClick, actionNote := menuItemActionMDL(ctx, item)
		if onClick == "" && item.ActionDoc == nil && len(item.StoredAction) == 0 {
			// An item built without a stored document (a hand-built model).
			onClick = legacyMenuItemOnClick(item)
		}
		if onClick != "" && len(item.Items) == 0 {
			props = append(props, "OnClick: "+onClick)
		} else if onClick != "" {
			// Studio Pro lets a sub-menu keep an action it no longer runs; MDL
			// has no OnClick on one, so the rewrite keeps it only as stored.
			actionNote = fmt.Sprintf("-- menu %s: its action (%s) belongs to a sub-menu and has no MDL form there", menuCaptionNote(item.Caption), onClick)
		}
		if icon := menuItemIconMDL(item); icon != "" {
			props = append(props, "Icon: "+icon)
		}
		propList := ""
		if len(props) > 0 {
			propList = " ( " + strings.Join(props, ", ") + " )"
		}
		if len(item.Items) > 0 {
			// Sub-menu container
			fmt.Fprintf(w, "%smenu %s%s {\n", indent, mdlQuote(ctx, item.Caption), propList)
			printMenuMDL(ctx, w, item.Items, depth+1, reproducer)
			fmt.Fprintf(w, "%s}\n", indent)
		} else {
			fmt.Fprintf(w, "%smenu item %s%s\n", indent, mdlQuote(ctx, item.Caption), propList)
		}
		if note := menuItemIconNote(item, reproducer); note != "" {
			fmt.Fprintf(w, "%s%s\n", indent, note)
		}
		if actionNote != "" {
			// Said, not dropped: what the item line above cannot state is kept
			// by a rewrite of it (convertMenuItemDefs).
			fmt.Fprintf(w, "%s%s\n", indent, actionNote)
		}
	}
}

// legacyMenuItemOnClick is the OnClick of an item that carries only the
// simple targets — one not read from storage.
func legacyMenuItemOnClick(item *types.NavMenuItem) string {
	switch {
	case item.Page != "":
		return "show page " + item.Page
	case item.Microflow != "":
		return "call microflow " + item.Microflow
	case item.ActionType == "SignOutAction":
		return "sign out"
	}
	return ""
}

// menuItemIconMDL renders a menu item's `Icon:` value, or "" when there is
// nothing CREATE NAVIGATION can reproduce.
//
// All three of Mendix's icon elements have a form now. Only the collection one
// used to, so DESCRIBE emitted a comment for a glyph or image icon — and since
// CREATE NAVIGATION is a full replacement, re-running that output DELETED the
// icon it had just declined to describe.
func menuItemIconMDL(item *types.NavMenuItem) string {
	switch types.MenuIconKindOf(item.IconType) {
	case types.MenuIconGlyph:
		// The code is the whole identity of a glyph. Without it there is nothing
		// to emit that would rebuild the same icon, so fall through to the note.
		if item.IconCode == 0 {
			return ""
		}
		return fmt.Sprintf("glyph %d", item.IconCode)
	case types.MenuIconImage:
		if item.Icon == "" {
			return ""
		}
		return "image " + quoteQualifiedName(item.Icon)
	case types.MenuIconCollection:
		if item.Icon == "" {
			return ""
		}
		return quoteQualifiedName(item.Icon)
	}
	return ""
}

// menuItemIconNote flags an icon DESCRIBE still cannot round-trip, so re-running
// the output loses it visibly rather than silently.
//
// All three icon elements are reproducible now, so this fires only on what is
// genuinely beyond the language: a stored $Type this build does not know, or a
// variant whose payload is missing (a glyph with no Code, a named icon with no
// name) — where emitting a clause would rebuild a DIFFERENT icon rather than the
// same one. Guessing between polymorphic variants is the failure mode that
// produces a document mxbuild accepts and Studio Pro cannot open.
func menuItemIconNote(item *types.NavMenuItem, reproducer string) string {
	if item.IconType == "" {
		return ""
	}
	// If there is a clause for it, there is nothing to flag.
	if menuItemIconMDL(item) != "" {
		return ""
	}
	target := item.Icon
	if target == "" {
		target = "a numeric glyph code"
	}
	return fmt.Sprintf("-- icon %s (%s) is not reproducible by %s; set it in Studio Pro",
		target, item.IconType, reproducer)
}

// singleLine folds a stored multi-line value onto one line so it can appear
// inside a `--` comment. Studio Pro writes an offline sync constraint with
// embedded newlines and indentation; emitting it verbatim would terminate the
// comment mid-XPath and leave the remainder parsed as MDL.
func singleLine(s string) string {
	// Collapsing whitespace with strings.Fields would also collapse it INSIDE
	// string literals, so a constraint containing 'two  spaces' would come back
	// as 'two spaces' — a silent change to the value being matched on, in a
	// place nothing would look. Quote state is tracked so only whitespace
	// outside literals is folded.
	var b strings.Builder
	inLiteral := false
	pendingSpace := false

	// write emits one byte, flushing a deferred separator first. Deferring is
	// what keeps a fold from landing INSIDE the literal that follows it.
	write := func(c byte) {
		if pendingSpace {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
		}
		b.WriteByte(c)
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			// A doubled quote inside a literal is an escaped quote, not a
			// close: copy both and stay in the literal.
			if inLiteral && i+1 < len(s) && s[i+1] == '\'' {
				write('\'')
				b.WriteByte('\'')
				i++
				continue
			}
			write(c)
			inLiteral = !inLiteral
		case !inLiteral && (c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			pendingSpace = true
		default:
			write(c)
		}
	}
	return b.String()
}

// syncModeMDL renders a stored sync mode as the MDL that reproduces it.
//
// The inverse of the visitor's mapping, and the reason describe -> exec now
// round-trips: emitting the stored member verbatim would produce `sync X
// Constrained`, which is not MDL, and emitting a Studio Pro caption would
// produce a document mxbuild refuses.
func syncModeMDL(ctx *ExecContext, mode, constraint string) string {
	switch mode {
	case "Online":
		return "online"
	case "All":
		return "all"
	case "Never":
		return "never"
	case "None":
		return "none"
	case "NoneAndPreserveData":
		return "none preserve data"
	case "Constrained":
		// The bracket form, so nothing is escaped. A stored constraint already
		// carries Mendix's own quote escaping; wrapping it in a quoted MDL
		// string doubles every one of those again, and the reference document's
		// came back as six consecutive quotes — correct, unreadable, and the
		// thing mendixlabs/mxcli#750 is about.
		//
		// Studio Pro stores the constraint bracketed, so the folded value is
		// normally already `[...]`; one without them is wrapped rather than
		// assumed to have them.
		//
		// A string in it is spelled for the describe language (describeXPath,
		// ako/mxcli#825).
		x := singleLine(constraint)
		if !strings.HasPrefix(x, "[") || !strings.HasSuffix(x, "]") {
			x = "[" + x + "]"
		}
		return "where " + describeXPath(ctx, x)
	default:
		// An unknown member is not guessed at. Emitting a mode MDL cannot spell
		// would produce a script that fails at check; saying so is honest and
		// keeps the rest of the block re-executable.
		return fmt.Sprintf("all -- UNKNOWN MODE %q, not reproducible", mode)
	}
}
