// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// UpdateNavigationProfile patches a navigation profile's home pages, login page,
// not-found page, and menu in place, preserving the rest of the navigation
// document byte-for-byte (read-unmarshal-patch-marshal). Mirrors the legacy
// writer field-for-field; pure bson.D manipulation, no codec rebuild.
func (b *Backend) UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateNavigationProfile: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(navDocID))
	if err != nil {
		return fmt.Errorf("UpdateNavigationProfile: load unit: %w", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("UpdateNavigationProfile: unmarshal: %w", err)
	}

	profiles := navGetArray(doc, "Profiles")
	if profiles == nil {
		return fmt.Errorf("no Profiles array found in navigation document")
	}
	found := false
	for i, item := range profiles {
		profDoc, ok := item.(bson.D)
		if !ok {
			continue // the leading int32 marker
		}
		if !strings.EqualFold(navGetString(profDoc, "Name"), profileName) {
			continue
		}
		found = true
		if navGetString(profDoc, "$Type") == "Navigation$NativeNavigationProfile" {
			profiles[i] = navPatchNativeProfile(profDoc, spec)
		} else {
			profiles[i] = navPatchWebProfile(profDoc, spec)
		}
		break
	}
	if !found {
		return fmt.Errorf("navigation profile not found: %s", profileName)
	}
	doc = navSetField(doc, "Profiles", profiles)

	out, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("UpdateNavigationProfile: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(navDocID), out)
}

// --- small bson.D helpers (pure) ---

func navGetArray(doc bson.D, key string) bson.A {
	for _, e := range doc {
		if e.Key == key {
			if a, ok := e.Value.(bson.A); ok {
				return a
			}
		}
	}
	return nil
}

func navSetField(doc bson.D, key string, value any) bson.D {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: value})
}

func navGetString(doc bson.D, key string) string {
	for _, e := range doc {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

func navID() any { return bsonutil.NewIDBsonBinary() }

// --- profile patchers ---

func navPatchWebProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
	var defaultHome *types.NavHomePageSpec
	var roleHomes []types.NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		doc = navSetField(doc, "HomePage", navHomePageBson(defaultHome.IsPage, defaultHome.Target, ""))
	} else {
		doc = navSetField(doc, "HomePage", navHomePageBson(false, "", ""))
	}

	homeItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		homeItems = append(homeItems, navHomePageBson(rh.IsPage, rh.Target, rh.ForRole))
	}
	doc = navSetField(doc, "HomeItems", homeItems)

	doc = navSetField(doc, "LoginPageSettings", navFormSettingsBson(spec.LoginPage))

	if spec.NotFoundPage != "" {
		doc = navSetField(doc, "NotFoundHomepage", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: spec.NotFoundPage},
		})
	} else {
		doc = navSetField(doc, "NotFoundHomepage", nil)
	}

	if spec.HasMenu {
		menuItems := bson.A{int32(1)}
		for _, mi := range spec.MenuItems {
			menuItems = append(menuItems, navMenuItemBson(mi))
		}
		doc = navSetField(doc, "Menu", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: menuItems},
		})
	}
	return doc
}

func navPatchNativeProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
	var defaultHome *types.NavHomePageSpec
	var roleHomes []types.NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		page, nf := navSplitTarget(defaultHome.IsPage, defaultHome.Target)
		doc = navSetField(doc, "NativeHomePage", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$NativeHomePage"},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nf},
		})
	}

	roleItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		page, nf := navSplitTarget(rh.IsPage, rh.Target)
		roleItems = append(roleItems, bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$RoleBasedNativeHomePage"},
			{Key: "UserRole", Value: rh.ForRole},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nf},
		})
	}
	doc = navSetField(doc, "RoleBasedNativeHomePages", roleItems)
	return doc
}

func navSplitTarget(isPage bool, target string) (page, nanoflow string) {
	if isPage {
		return target, ""
	}
	return "", target
}

// navHomePageBson builds a Navigation$HomePage (default) or
// Navigation$RoleBasedHomePage (when forRole is set).
func navHomePageBson(isPage bool, target, forRole string) bson.D {
	page, mf := navSplitTarget(isPage, target)
	d := bson.D{{Key: "$ID", Value: navID()}}
	if forRole == "" {
		d = append(d, bson.E{Key: "$Type", Value: "Navigation$HomePage"})
		d = append(d, bson.E{Key: "Microflow", Value: mf}, bson.E{Key: "Page", Value: page})
		return d
	}
	d = append(d, bson.E{Key: "$Type", Value: "Navigation$RoleBasedHomePage"})
	d = append(d, bson.E{Key: "Microflow", Value: mf}, bson.E{Key: "Page", Value: page}, bson.E{Key: "UserRole", Value: forRole})
	return d
}

func navFormSettingsBson(formName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: formName},
		{Key: "ParameterMappings", Value: bson.A{int32(1)}},
		{Key: "TitleOverride", Value: navEmptyTextTemplate()},
	}
}

func navMenuItemBson(mi types.NavMenuItemSpec) bson.D {
	item := bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: navMenuAction(mi)},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: navCaptionBson(mi.Caption)},
		{Key: "Icon", Value: nil},
	}
	subItems := bson.A{int32(1)}
	for _, sub := range mi.Items {
		subItems = append(subItems, navMenuItemBson(sub))
	}
	item = append(item, bson.E{Key: "Items", Value: subItems})
	return item
}

func navCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			int32(1),
			bson.D{
				{Key: "$ID", Value: navID()},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: text},
			},
		}},
	}
}

func navMenuAction(mi types.NavMenuItemSpec) bson.D {
	if mi.Page != "" {
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "FormSettings", Value: navFormSettingsBson(mi.Page)},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{int32(1)}},
		}
	}
	if mi.Microflow != "" {
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: navID()},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: mi.Microflow},
			}},
		}
	}
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$NoAction"},
	}
}

func navEmptyTextTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Microflows$TextTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Text", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(2)}},
		}},
	}
}
