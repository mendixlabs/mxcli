// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// PatchNavigationProfile applies a navigation profile patch to raw BSON bytes,
// returning the new bytes. Pure BSON manipulation — no database access required.
func PatchNavigationProfile(rawBytes []byte, profileName string, spec types.NavigationProfileSpec) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(rawBytes, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}
	doc, err := navpPatchProfileDoc(doc, profileName, spec)
	if err != nil {
		return nil, err
	}
	newBytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal BSON: %w", err)
	}
	return newBytes, nil
}

// navpPatchProfileDoc applies the navigation profile patch to a parsed BSON document.
func navpPatchProfileDoc(doc bson.D, profileName string, spec types.NavigationProfileSpec) (bson.D, error) {
	profiles := navpGetBsonArray(doc, "Profiles")
	if profiles == nil {
		return doc, fmt.Errorf("no Profiles array found in navigation document")
	}

	found := false
	for i, item := range profiles {
		profDoc, ok := item.(bson.D)
		if !ok {
			continue
		}

		// Match profile by name (case-insensitive)
		name := ""
		for _, f := range profDoc {
			if f.Key == "Name" {
				name, _ = f.Value.(string)
				break
			}
		}
		if !strings.EqualFold(name, profileName) {
			continue
		}
		found = true

		// Determine if this is a native profile
		isNative := false
		for _, f := range profDoc {
			if f.Key == "$Type" {
				typeName, _ := f.Value.(string)
				isNative = typeName == "Navigation$NativeNavigationProfile"
				break
			}
		}

		if isNative {
			profDoc = navpPatchNativeProfile(profDoc, spec)
		} else {
			profDoc = navpPatchWebProfile(profDoc, spec)
		}

		profiles[i] = profDoc
		break
	}

	if !found {
		return doc, fmt.Errorf("navigation profile not found: %s", profileName)
	}

	return navpSetBsonField(doc, "Profiles", profiles), nil
}

// navpPatchWebProfile applies the spec to a web navigation profile.
func navpPatchWebProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
	// --- HomePage (default home) ---
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
		doc = navpSetBsonField(doc, "HomePage", navpBuildHomePageBson(defaultHome))
	} else {
		// Clear default home page
		doc = navpSetBsonField(doc, "HomePage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: ""},
		})
	}

	// --- HomeItems (role-based homes) ---
	homeItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		homeItems = append(homeItems, navpBuildRoleBasedHomeBson(rh))
	}
	doc = navpSetBsonField(doc, "HomeItems", homeItems)

	// --- LoginPageSettings ---
	if spec.LoginPage != "" {
		doc = navpSetBsonField(doc, "LoginPageSettings", navpBuildFormSettingsBson(spec.LoginPage))
	} else {
		doc = navpSetBsonField(doc, "LoginPageSettings", navpBuildFormSettingsBson(""))
	}

	// --- NotFoundHomepage ---
	if spec.NotFoundPage != "" {
		doc = navpSetBsonField(doc, "NotFoundHomepage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: spec.NotFoundPage},
		})
	} else {
		// Mendix uses null when not set
		doc = navpSetBsonField(doc, "NotFoundHomepage", nil)
	}

	// --- Menu ---
	if spec.HasMenu {
		menuItems := bson.A{int32(1)}
		for _, mi := range spec.MenuItems {
			menuItems = append(menuItems, navpBuildMenuItemBson(mi))
		}
		doc = navpSetBsonField(doc, "Menu", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: menuItems},
		})
	}

	return doc
}

// navpPatchNativeProfile applies the spec to a native navigation profile.
func navpPatchNativeProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
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
		page := ""
		nanoflow := ""
		if defaultHome.IsPage {
			page = defaultHome.Target
		} else {
			nanoflow = defaultHome.Target
		}
		doc = navpSetBsonField(doc, "NativeHomePage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$NativeHomePage"},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nanoflow},
		})
	}

	// Role-based native home pages
	roleItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		page := ""
		nanoflow := ""
		if rh.IsPage {
			page = rh.Target
		} else {
			nanoflow = rh.Target
		}
		roleItems = append(roleItems, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$RoleBasedNativeHomePage"},
			{Key: "UserRole", Value: rh.ForRole},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nanoflow},
		})
	}
	doc = navpSetBsonField(doc, "RoleBasedNativeHomePages", roleItems)

	return doc
}

// navpBuildHomePageBson builds a Navigation$HomePage BSON document.
func navpBuildHomePageBson(hp *types.NavHomePageSpec) bson.D {
	page := ""
	mf := ""
	if hp.IsPage {
		page = hp.Target
	} else {
		mf = hp.Target
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Navigation$HomePage"},
		{Key: "Microflow", Value: mf},
		{Key: "Page", Value: page},
	}
}

// navpBuildRoleBasedHomeBson builds a Navigation$RoleBasedHomePage BSON document.
func navpBuildRoleBasedHomeBson(rh types.NavHomePageSpec) bson.D {
	page := ""
	mf := ""
	if rh.IsPage {
		page = rh.Target
	} else {
		mf = rh.Target
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Navigation$RoleBasedHomePage"},
		{Key: "Microflow", Value: mf},
		{Key: "Page", Value: page},
		{Key: "UserRole", Value: rh.ForRole},
	}
}

// navpBuildFormSettingsBson builds a Forms$FormSettings BSON document with required fields.
func navpBuildFormSettingsBson(formName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: formName},
		{Key: "ParameterMappings", Value: bson.A{int32(1)}},
		{Key: "TitleOverride", Value: navpEmptyTextTemplate()},
	}
}

// navpBuildMenuItemBson builds a Menus$MenuItem BSON document recursively.
func navpBuildMenuItemBson(mi types.NavMenuItemSpec) bson.D {
	item := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: navpBuildMenuAction(mi)},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: navpBuildCaptionBson(mi.Caption)},
		{Key: "Icon", Value: nil},
	}

	// Sub-items
	subItems := bson.A{int32(1)}
	for _, sub := range mi.Items {
		subItems = append(subItems, navpBuildMenuItemBson(sub))
	}
	item = append(item, bson.E{Key: "Items", Value: subItems})

	return item
}

// navpBuildCaptionBson builds a Texts$Text BSON document with a single en_US translation.
func navpBuildCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			int32(1),
			bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: text},
			},
		}},
	}
}

// navpBuildMenuAction builds the Action BSON for a menu item based on its target.
func navpBuildMenuAction(mi types.NavMenuItemSpec) bson.D {
	if mi.Page != "" {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "FormSettings", Value: navpBuildFormSettingsBson(mi.Page)},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{int32(1)}},
		}
	}
	if mi.Microflow != "" {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: mi.Microflow},
			}},
		}
	}
	// No action (sub-menu container or plain item)
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$NoAction"},
	}
}

// navpEmptyTextTemplate returns an empty Microflows$TextTemplate embedded BSON document.
// Used for TitleOverride on Forms$FormSettings.
func navpEmptyTextTemplate() bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Microflows$TextTemplate"},
		{Key: "Parameters", Value: bson.A{int32(2)}},
		{Key: "Text", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(2)}},
		}},
	}
}

// navpSetBsonField sets a top-level field in a bson.D, adding it if not found.
func navpSetBsonField(doc bson.D, key string, value any) bson.D {
	for i, elem := range doc {
		if elem.Key == key {
			doc[i].Value = value
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: value})
}

// navpGetBsonArray returns the bson.A value for a named field in a bson.D.
func navpGetBsonArray(doc bson.D, key string) bson.A {
	for _, elem := range doc {
		if elem.Key == key {
			if arr, ok := elem.Value.(bson.A); ok {
				return arr
			}
		}
	}
	return nil
}
