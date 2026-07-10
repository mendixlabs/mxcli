// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMenus "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/menus"
	genNative "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/nativepages"
	genNav "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/navigation"
	genPages "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	genTexts "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
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
	if lps, ok := p.LoginPageSettings().(*genNav.NavigationProfileLoginFormSettings); ok && lps != nil {
		profile.LoginPage = lps.LoginPageQualifiedName()
	}
	if nfp, ok := p.NotFoundHomepage().(*genNav.NotFoundHomePage); ok && nfp != nil {
		profile.NotFoundPage = nfp.PageQualifiedName()
		if profile.NotFoundPage == "" {
			profile.NotFoundPage = nfp.MicroflowQualifiedName()
		}
	}
	if mic, ok := p.MenuItemCollection().(*genMenus.MenuItemCollection); ok && mic != nil {
		for _, itemEl := range mic.ItemsItems() {
			if mi := navMenuItemFromGen(itemEl); mi != nil {
				profile.MenuItems = append(profile.MenuItems, mi)
			}
		}
	}
	appendOfflineEntities(profile, p.OfflineEntityConfigsItems())
	return profile
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
				Page:    bar.PageQualifiedName(),
			}
			if mi.Caption != "" || mi.Page != "" {
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
			Entity:     oe.EntityQualifiedName(),
			SyncMode:   oe.SyncMode(),
			Constraint: oe.Constraint(),
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

// resolveMenuAction sets the action type / target on a NavMenuItem from a gen
// client-action element, mirroring the legacy action-type dispatch.
func resolveMenuAction(item *types.NavMenuItem, action element.Element) {
	if action == nil {
		return
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
