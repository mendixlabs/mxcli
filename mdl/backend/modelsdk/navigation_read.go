// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMenus "github.com/mendixlabs/mxcli/modelsdk/gen/menus"
	genNative "github.com/mendixlabs/mxcli/modelsdk/gen/nativepages"
	genNav "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
	genPages "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetNavigation reads the Navigation$NavigationDocument unit and converts it to
// the semantic type, mirroring the legacy (*mpr.Reader).GetNavigation /
// parseNavigationDocument. Web (Navigation$NavigationProfile) and native
// (Navigation$NativeNavigationProfile) profiles, their home pages, role-based
// home pages, login/not-found pages, recursive menu items and offline entities
// are all populated. Field sources differ from legacy only where the codec
// metamodel uses canonical names (e.g. page actions read PageSettings, not
// FormSettings; captions read Texts$Translation items).
func (b *Backend) GetNavigation() (*types.NavigationDocument, error) {
	units, err := mprread.ListUnitsWithContainer[*genNav.NavigationDocument](b.reader)
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		return nil, nil
	}
	u := units[0]
	g := u.Element
	nav := &types.NavigationDocument{
		ContainerID: model.ID(u.ContainerID),
	}
	nav.ID = model.ID(g.ID())
	nav.TypeName = "Navigation$NavigationDocument"

	for _, profEl := range g.ProfilesItems() {
		if p := navProfileFromGen(profEl); p != nil {
			nav.Profiles = append(nav.Profiles, p)
		}
	}
	return nav, nil
}

// navProfileFromGen converts a gen navigation profile (web or native) to the
// semantic NavigationProfile.
func navProfileFromGen(el element.Element) *types.NavigationProfile {
	switch p := el.(type) {
	case *genNav.NavigationProfile:
		return webNavProfileFromGen(p)
	case *genNav.NativeNavigationProfile:
		return nativeNavProfileFromGen(p)
	default:
		return nil
	}
}

func webNavProfileFromGen(p *genNav.NavigationProfile) *types.NavigationProfile {
	profile := &types.NavigationProfile{
		Name: p.Name(),
		Kind: p.Kind(),
	}
	if hp, ok := p.HomePage().(*genNav.HomePage); ok && hp != nil {
		page, mf := hp.PageQualifiedName(), hp.MicroflowQualifiedName()
		if page != "" || mf != "" {
			profile.HomePage = &types.NavHomePage{Page: page, Microflow: mf}
		}
	}
	for _, rbEl := range p.RoleBasedHomePagesItems() {
		if rb, ok := rbEl.(*genNav.RoleBasedHomePage); ok {
			h := &types.NavRoleBasedHome{
				UserRole:  rb.UserRoleQualifiedName(),
				Page:      rb.PageQualifiedName(),
				Microflow: rb.MicroflowQualifiedName(),
			}
			if h.UserRole != "" {
				profile.RoleBasedHomePages = append(profile.RoleBasedHomePages, h)
			}
		}
	}
	profile.LoginPage = navLoginPageOf(p.LoginPageSettings())
	profile.NotFoundPage = navNotFoundPageOf(p.NotFoundHomepage())
	if mic, ok := p.MenuItemCollection().(*genMenus.MenuItemCollection); ok && mic != nil {
		for _, itemEl := range mic.ItemsItems() {
			if mi := navMenuItemFromGen(itemEl); mi != nil {
				profile.MenuItems = append(profile.MenuItems, mi)
			}
		}
	}
	appendOfflineEntities(profile, p.OfflineEntityConfigsItems())
	profile.ThrowPartialSyncError = throwPartialSyncError(p.Raw())
	// Null until "Progressive web app" is ticked -- and then an offline
	// profile has no service worker and no page opens offline (#1377).
	if pwa, ok := p.ProgressiveWebAppSettings().(*genNav.ProgressiveWebAppSettings); ok && pwa != nil {
		profile.ProgressiveWebApp = &types.NavPWASettings{Precaching: pwa.Precaching(), InstallPrompt: pwa.InstallPrompt()}
	}
	return profile
}

// navLoginPageOf reads a profile's login page out of whatever element the codec
// decoded for LoginPageSettings.
//
// The stored $Type is Forms$FormSettings -- the storage name of
// Pages$PageSettings -- with the page under "Form", which is what a blank 11.12
// app's own navigation document carries and what generated/metamodel declares
// (NavigationProfile.LoginPageSettings *PagesPageSettings). So it decodes to
// *genPages.PageSettings.
//
// gen additionally declares Navigation$NavigationProfileLoginFormSettings, with
// the page under "LoginPage". Reading only that type is why DESCRIBE NAVIGATION
// silently dropped the `login page` clause on this engine while legacy printed
// it. No document carries it (0 of 54 local projects, 0 of the mx-test-projects,
// and not the blank app), but it is cheap to accept and costs nothing if some
// Mendix version does write it.
func navLoginPageOf(el element.Element) string {
	switch lps := el.(type) {
	case *genPages.PageSettings:
		if lps != nil {
			return lps.PageQualifiedName()
		}
	case *genNav.NavigationProfileLoginFormSettings:
		if lps != nil {
			return lps.LoginPageQualifiedName()
		}
	}
	return ""
}

// navNotFoundPageOf reads a profile's not-found page, accepting both $Types that
// occur in the wild.
//
// generated/metamodel and gen agree that this slot holds a
// Navigation$NotFoundHomePage, and the reader only accepted that -- but all three
// of mxcli's navigation writers emit a plain Navigation$HomePage there, so every
// not-found page mxcli has ever written read back empty and was dropped by a
// DESCRIBE -> exec round-trip.
//
// Settled by ako/TestApp: its "Fallback page" is stored as
// Navigation$NotFoundHomePage, so gen and generated/metamodel were right and the
// writers were wrong. They now emit that $Type.
//
// Both are still accepted here, and must stay that way -- but not because the
// old spelling is benign. Mendix cannot LOAD a project carrying it: `mx check`
// and `mxbuild --target=deploy` both exit 1 with "Object of type
// '...Navigation.HomePage' cannot be converted to type
// '...Navigation.NotFoundHomePage'" (measured on 11.13). So a project mxcli
// wrote before the writer fix does not build at all, and reading the old
// spelling here is what lets mxcli open that project and repair it -- mxcli
// parses the BSON directly, so it is not bound by Mendix's load validation.
// Dropping this branch would strand exactly the projects that need fixing.
//
// Nothing caught the bug because nothing ever BUILT a project with a fallback
// page set: the automated mx-check coverage runs doctype-tests/ only, and no
// script there sets one.
func navNotFoundPageOf(el element.Element) string {
	var page, microflow string
	switch nfp := el.(type) {
	case *genNav.NotFoundHomePage:
		if nfp != nil {
			page, microflow = nfp.PageQualifiedName(), nfp.MicroflowQualifiedName()
		}
	case *genNav.HomePage:
		if nfp != nil {
			page, microflow = nfp.PageQualifiedName(), nfp.MicroflowQualifiedName()
		}
	}
	if page != "" {
		return page
	}
	return microflow
}

func nativeNavProfileFromGen(p *genNav.NativeNavigationProfile) *types.NavigationProfile {
	profile := &types.NavigationProfile{
		Name:     p.Name(),
		IsNative: true,
	}
	if hp, ok := p.NativeHomePage().(*genNav.NativeHomePage); ok && hp != nil {
		page, nf := hp.HomePagePageQualifiedName(), hp.HomePageNanoflowQualifiedName()
		if page != "" || nf != "" {
			profile.HomePage = &types.NavHomePage{Page: page, Microflow: nf}
		}
	}
	for _, rbEl := range p.RoleBasedNativeHomePagesItems() {
		if rb, ok := rbEl.(*genNav.RoleBasedNativeHomePage); ok {
			h := &types.NavRoleBasedHome{
				UserRole:  rb.UserRoleQualifiedName(),
				Page:      rb.HomePagePageQualifiedName(),
				Microflow: rb.HomePageNanoflowQualifiedName(),
			}
			if h.UserRole != "" {
				profile.RoleBasedHomePages = append(profile.RoleBasedHomePages, h)
			}
		}
	}
	for _, barEl := range p.BottomBarItemsItems() {
		if bar, ok := barEl.(*genNative.BottomBarItem); ok {
			mi := &types.NavMenuItem{
				Caption: textOf(bar.Caption()),
			}
			// A bottom bar item carries a client action and an icon like a menu
			// item; reading only its legacy Page left describe without either
			// (ako/mxcli#980).
			mi.IconType, mi.Icon, mi.IconCode = menuIconOf(bar.Icon())
			resolveMenuAction(mi, bar.Action())
			if mi.Page == "" {
				mi.Page = bar.PageQualifiedName()
			}
			if mi.Caption != "" || mi.Page != "" || mi.StoredAction != nil {
				profile.MenuItems = append(profile.MenuItems, mi)
			}
		}
	}
	appendOfflineEntities(profile, p.OfflineEntityConfigsItems())
	return profile
}

// appendOfflineEntities converts gen OfflineEntityConfig elements onto a profile.
func appendOfflineEntities(profile *types.NavigationProfile, items []element.Element) {
	for _, oeEl := range items {
		oe, ok := oeEl.(*genNav.OfflineEntityConfig)
		if !ok {
			continue
		}
		e := &types.NavOfflineEntity{
			Entity:            oe.EntityQualifiedName(),
			SyncMode:          oe.SyncMode(),
			Constraint:        oe.Constraint(),
			CompatibilityMode: oe.CompatibilityMode(),
		}
		if e.Entity != "" {
			profile.OfflineEntities = append(profile.OfflineEntities, e)
		}
	}
}

// navMenuItemFromGen recursively converts a Menus$MenuItem to a NavMenuItem,
// mirroring the legacy parseNavMenuItem.
func navMenuItemFromGen(el element.Element) *types.NavMenuItem {
	mi, ok := el.(*genMenus.MenuItem)
	if !ok {
		return nil
	}
	item := &types.NavMenuItem{
		Caption: textOf(mi.Caption()),
	}
	item.IconType, item.Icon, item.IconCode = menuIconOf(mi.Icon())
	resolveMenuAction(item, mi.Action())
	for _, subEl := range mi.ItemsItems() {
		if sub := navMenuItemFromGen(subEl); sub != nil {
			item.Items = append(item.Items, sub)
		}
	}
	if item.Caption == "" && item.Page == "" && len(item.Items) == 0 {
		return nil
	}
	return item
}

// menuIconOf reports a menu item's icon storage $Type and, for the two variants
// that carry one, its qualified image name. Forms$GlyphIcon has a numeric Code
// and no name, so it yields a type with an empty name — which is what keeps
// DESCRIBE from emitting an ICON clause it cannot faithfully round-trip.
//
// The name is read off the element's raw BSON rather than a typed accessor,
// because the variants decode inconsistently: an unregistered type arrives as a
// bare *element.Base while a registered one arrives as a generated struct that
// merely embeds it. Asserting on *element.Base therefore silently returns
// nothing for exactly the registered variants — which is what made DESCRIBE drop
// every Atlas icon under this engine while the legacy engine printed them. Match
// on the Raw() method instead, which both shapes satisfy.
func menuIconOf(icon element.Element) (typeName, image string, code int) {
	if icon == nil {
		return "", "", 0
	}
	typeName = icon.TypeName()
	raw, ok := icon.(interface{ Raw() bson.Raw })
	if !ok {
		return typeName, "", 0
	}
	if v, err := raw.Raw().LookupErr("Image"); err == nil {
		if s, ok := v.StringValueOK(); ok {
			image = s
		}
	}
	// Forms$GlyphIcon's Code is the ONLY thing identifying which glyph it is.
	// Reading the $Type alone told a caller a glyph was there and nothing more,
	// so DESCRIBE could not re-emit it and a rewrite replaced it with nothing.
	if v, err := raw.Raw().LookupErr("Code"); err == nil {
		if i, ok := v.Int32OK(); ok {
			code = int(i)
		} else if i, ok := v.Int64OK(); ok {
			code = int(i)
		}
	}
	return typeName, image, code
}

// resolveMenuAction sets the action type / target on a NavMenuItem from a gen
// client-action element, mirroring the legacy action-type dispatch.
func resolveMenuAction(item *types.NavMenuItem, action element.Element) {
	if action == nil {
		return
	}
	// The stored document itself, whatever its $Type: what describe renders
	// and what a rewrite carries when MDL cannot express it (ako/mxcli#980).
	if raw := action.Raw(); len(raw) > 0 {
		item.StoredAction = append([]byte(nil), raw...)
		if doc, err := mmpr.RawDocumentMap(raw); err == nil {
			item.ActionDoc = doc
		}
	}
	switch a := action.(type) {
	case *genPages.PageClientAction:
		item.ActionType = "PageAction"
		if ps, ok := a.PageSettings().(*genPages.PageSettings); ok && ps != nil {
			item.Page = ps.PageQualifiedName()
		}
	case *genPages.MicroflowClientAction:
		item.ActionType = "MicroflowAction"
		if ms, ok := a.MicroflowSettings().(*genPages.MicroflowSettings); ok && ms != nil {
			item.Microflow = ms.MicroflowQualifiedName()
		}
	case *genPages.SignOutClientAction:
		// Named explicitly rather than falling to the raw-type-name default
		// below: DESCRIBE and the writer both key on "SignOutAction", and a
		// round trip only closes if the reader produces the name the writer
		// consumes.
		item.ActionType = "SignOutAction"
	default:
		t := action.TypeName()
		switch {
		case strings.HasSuffix(t, "OpenLinkAction") || strings.HasSuffix(t, "OpenLinkClientAction"):
			item.ActionType = "OpenLinkAction"
		case strings.HasSuffix(t, "NoAction") || strings.HasSuffix(t, "NoClientAction"):
			item.ActionType = "NoAction"
		default:
			item.ActionType = t
		}
	}
}

// textOf extracts the first non-empty translation from a Texts$Text element,
// mirroring the legacy extractTextFromBson.
func textOf(el element.Element) string {
	t, ok := el.(*genTexts.Text)
	if !ok || t == nil {
		return ""
	}
	for _, trEl := range t.TranslationsItems() {
		if tr, ok := trEl.(*genTexts.Translation); ok {
			if s := tr.Text(); s != "" {
				return s
			}
		}
	}
	return ""
}

// throwPartialSyncError reads a property NEITHER modelsdk/gen NOR
// generated/metamodel declares — measured on ako/TestApp, zero occurrences in
// each — so there is no typed accessor to call. element.Base keeps the raw
// document, which is what makes reading it possible without a second load.
//
// Absent means TRUE: every reference profile carries true and Studio Pro's box
// is checked by default, so a document without the key must not be read as
// "do not throw".
//
// Web profiles only. Both of ako/TestApp's carry it; whether a native profile
// does is unmeasured, and nativeNavProfileFromGen is deliberately left alone
// rather than given a default nothing has verified.
func throwPartialSyncError(raw bson.Raw) bool {
	v, err := raw.LookupErr("ThrowPartialSyncError")
	if err != nil {
		return true
	}
	b, ok := v.BooleanOK()
	if !ok {
		return true
	}
	return b
}
