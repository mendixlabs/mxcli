// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// UpdateNavigationProfile patches a navigation profile's home pages, login page,
// not-found page, and menu in place, preserving the rest of the navigation
// document byte-for-byte (read-unmarshal-patch-marshal). Mirrors the legacy
// writer field-for-field; pure bson.D manipulation, no codec rebuild.
// Typed-array markers for the lists these writers emit.
//
// The leading int32 of a Mendix array is a per-FIELD constant, not a function of
// the list's contents: Forms$FormSettings.ParameterMappings is 2 in 816 empty
// and 306 non-empty real occurrences alike. So each list below takes the value
// Studio Pro writes for that field, censused over 19,078 unit files in 54
// projects on this machine:
//
//	Forms$FormSettings.ParameterMappings        2  (1122 documents)
//	Forms$FormAction.PagesForSpecializations    2  (357)
//	Menus$MenuItemCollection.Items              3  (153)
//	Menus$MenuItem.Items                        3  (459)
//	Texts$Text.Items                            3  (169,486 vs 7 at 2)
//	Navigation$NavigationProfile.HomeItems      2  (51)
//
// These writers previously emitted 1 for all of them. Note what that was NOT:
// 1 is a perfectly legitimate Mendix marker -- a Marketplace .mpk mxcli has
// never touched carries it on CustomWidgets$WidgetValueType.AllowedTypes (212k
// occurrences) and on Forms$Page.AllowedModuleRoles. debug-bson.md's rule that
// "any other value is invalid and Studio Pro ignores the array" is too strong
// and is corrected there. The defect is narrower: for THESE fields, no
// Studio Pro document uses 1, and mxcli's own menu-document codec path already
// writes 3 for the same Menus$ item collections, so the two paths disagreed.
//
// HomeItems needed a second source, because all 51 census observations are empty
// lists and navigation_profile_add.go wrote 3 there from a PED session that
// cannot be re-run here. ako/TestApp settles it: its Studio Pro-authored profile
// carries HomeItems [marker 2] holding two Navigation$RoleBasedHomePage
// elements -- a NON-empty list, which is the case the census could not reach.
// navigation_profile_add.go now writes 2 as well.
const (
	navMarkerItems             = int32(3)
	navMarkerParameterMappings = int32(2)
	navMarkerHomeItems         = int32(2)
)

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
			patched, err := navPatchWebProfile(profDoc, spec)
			if err != nil {
				return fmt.Errorf("UpdateNavigationProfile: %w", err)
			}
			profiles[i] = patched
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

func navPatchWebProfile(doc bson.D, spec types.NavigationProfileSpec) (bson.D, error) {
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

	homeItems := bson.A{navMarkerHomeItems}
	for _, rh := range roleHomes {
		homeItems = append(homeItems, navHomePageBson(rh.IsPage, rh.Target, rh.ForRole))
	}
	doc = navSetField(doc, "HomeItems", homeItems)

	doc = navSetField(doc, "LoginPageSettings", navFormSettingsBson(spec.LoginPage))

	if spec.NotFoundPage != "" {
		doc = navSetField(doc, "NotFoundHomepage", bson.D{
			{Key: "$ID", Value: navID()},
			// Studio Pro's "Fallback page". The $Type is
			// Navigation$NotFoundHomePage, not the Navigation$HomePage the home
			// page slot takes -- measured on ako/TestApp, whose fallback page
			// Studio Pro stored as Navigation$NotFoundHomePage/Page.
			//
			// The wrong $Type here is not cosmetic: Mendix cannot LOAD the
			// project. Both `mx check` and `mxbuild --target=deploy` exit 1 with
			// "Object of type '...Navigation.HomePage' cannot be converted to
			// type '...Navigation.NotFoundHomePage'" (measured on 11.13, against
			// a build of this file emitting the old spelling). Nothing caught it
			// because nothing ever BUILT a project with a fallback page set --
			// the automated mx-check coverage runs doctype-tests/ only, and no
			// script there sets one.
			{Key: "$Type", Value: "Navigation$NotFoundHomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: spec.NotFoundPage},
		})
	} else {
		doc = navSetField(doc, "NotFoundHomepage", nil)
	}

	if spec.HasMenu {
		menuItems := bson.A{navMarkerItems}
		for _, mi := range spec.MenuItems {
			item, err := navMenuItemBson(mi)
			if err != nil {
				return nil, err
			}
			menuItems = append(menuItems, item)
		}
		doc = navSetField(doc, "Menu", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: menuItems},
		})
	}

	if spec.HasSync {
		doc = navSetField(doc, "OfflineEntityConfigs",
			navOfflineConfigs(navGetArray(doc, "OfflineEntityConfigs"), spec.OfflineEntities))
	}

	// ThrowPartialSyncError is declared by neither generated source, so it can
	// only be written as a raw key — which is why nil means "the statement said
	// nothing" and the stored value is left exactly as it is. A non-pointer
	// would reset the flag on every rewrite that never mentions it.
	if spec.ThrowSyncError != nil {
		doc = navSetField(doc, "ThrowPartialSyncError", *spec.ThrowSyncError)
	}

	if spec.ProgressiveWebApp != nil {
		doc = navSetField(doc, "ProgressiveWebAppSettings", navPWASettings(navGetDoc(doc, "ProgressiveWebAppSettings"), *spec.ProgressiveWebApp))
	}
	return doc, nil
}

// navPWASettings is the profile's ProgressiveWebAppSettings after the clause:
// null for OFF; otherwise the settings the clause states, a key it leaves out
// taking its platform default -- the same reading describe relies on when it
// omits a default (R1, R12), so describe -> exec writes nothing. Only the stored
// element's $ID is kept. Keys are in the alphabetical order Studio Pro stores a
// profile's properties in.
func navPWASettings(stored bson.D, spec types.NavPWASpec) any {
	if spec.Off {
		return nil
	}
	var id any = navID()
	if stored != nil && navGetString(stored, "$Type") == "Navigation$ProgressiveWebAppSettings" {
		for _, e := range stored {
			if e.Key == "$ID" {
				id = e.Value
			}
		}
	}
	installPrompt, precaching := types.NavPWADefaultInstallPrompt, types.NavPWADefaultPrecaching
	if spec.InstallPrompt != nil {
		installPrompt = *spec.InstallPrompt
	}
	if spec.Precaching != nil {
		precaching = *spec.Precaching
	}
	return bson.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: "Navigation$ProgressiveWebAppSettings"},
		{Key: "InstallPrompt", Value: installPrompt},
		{Key: "Precaching", Value: precaching},
	}
}

// navGetDoc is the value of a field that holds an embedded document, or nil.
func navGetDoc(doc bson.D, key string) bson.D {
	for _, e := range doc {
		if e.Key == key {
			if d, ok := e.Value.(bson.D); ok {
				return d
			}
		}
	}
	return nil
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

	roleItems := bson.A{navMarkerHomeItems}
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

	// A native profile stores its offline configs as a web profile does. The
	// executor refuses the clauses this writer cannot apply to a native
	// profile (its bottom bar, login and not-found pages, on sync error), so
	// none is ignored in silence (ako/mxcli#980).
	if spec.HasSync {
		doc = navSetField(doc, "OfflineEntityConfigs",
			navOfflineConfigs(navGetArray(doc, "OfflineEntityConfigs"), spec.OfflineEntities))
	}
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
		{Key: "ParameterMappings", Value: bson.A{navMarkerParameterMappings}},
		// No override is an explicit null. An empty template overrides the page
		// title with "" and produces CW0263 for every authored menu item (#812).
		{Key: "TitleOverride", Value: nil},
	}
}

func navMenuItemBson(mi types.NavMenuItemSpec) (bson.D, error) {
	action, err := navMenuAction(mi)
	if err != nil {
		return nil, fmt.Errorf("menu item '%s': %w", mi.Caption, err)
	}
	item := bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: action},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: navCaptionBson(mi.Caption)},
		{Key: "Icon", Value: navMenuIconBson(mi)},
	}
	subItems := bson.A{navMarkerItems}
	for _, sub := range mi.Items {
		subDoc, err := navMenuItemBson(sub)
		if err != nil {
			return nil, err
		}
		subItems = append(subItems, subDoc)
	}
	item = append(item, bson.E{Key: "Items", Value: subItems})
	return item, nil
}

// navMenuIconBson mirrors sdk/mpr's buildMenuIconBson. The storage names are
// Forms$… (not the metamodel's Pages$…) — "Form" was the original term for
// "Page".
//
// All THREE variants are emitted. Only the icon-collection one used to be, so a
// glyph or image icon read off a real project came back as no icon at all, and
// `create or replace navigation` — a full replacement — wrote that nothing over
// the user's icon. Measured on testdata/expr-checker: exec of DESCRIBE's own
// output destroyed the Home item's glyph icon at exit 0.
func navMenuIconBson(spec types.NavMenuItemSpec) interface{} {
	kind := spec.IconKind
	if kind == types.MenuIconNone && spec.Icon != "" {
		// A spec built before the kind existed carries a name and nothing else.
		// That name has only ever meant an icon-collection icon.
		kind = types.MenuIconCollection
	}
	storage := types.MenuIconStorageType(kind)
	if storage == "" {
		return nil
	}
	doc := bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: storage},
	}
	if kind == types.MenuIconGlyph {
		// A glyph with no code identifies no glyph; writing one would store an
		// icon nobody can see. Emit no icon rather than an empty element.
		if spec.IconCode == 0 {
			return nil
		}
		return append(doc, bson.E{Key: "Code", Value: int32(spec.IconCode)})
	}
	if spec.Icon == "" {
		return nil
	}
	return append(doc, bson.E{Key: "Image", Value: spec.Icon})
}

func navCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			navMarkerItems,
			bson.D{
				{Key: "$ID", Value: navID()},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: model.AuthoringLanguage()},
				{Key: "Text", Value: text},
			},
		}},
	}
}

func navMenuAction(mi types.NavMenuItemSpec) (bson.D, error) {
	// An action the script could not state, carried from storage verbatim
	// (ako/mxcli#980). Falling through to Forms$NoAction below is what deleted a
	// nanoflow menu item's action on every describe -> exec.
	if len(mi.KeepAction) > 0 {
		var kept bson.D
		if err := bson.Unmarshal(mi.KeepAction, &kept); err != nil {
			return nil, fmt.Errorf("kept action: %w", err)
		}
		return kept, nil
	}
	// The script's action, written by the widget client-action serializer.
	if mi.Action != nil {
		raw, err := menuActionBSON(mi.Action, mi.StoredAction)
		if err != nil {
			return nil, err
		}
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		return d, nil
	}
	return navMenuActionFromTargets(mi), nil
}

// navMenuActionFromTargets builds the action of a spec that carries only the
// simple targets (no built Action) — a caller other than the executor.
func navMenuActionFromTargets(mi types.NavMenuItemSpec) bson.D {
	if mi.Page != "" {
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "FormSettings", Value: navFormSettingsBson(mi.Page)},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{navMarkerParameterMappings}},
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
	if mi.SignOut {
		// Same element a sign-out BUTTON carries, pinned against the sign-out
		// menu item in ako/TestApp: two properties and nothing else.
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$SignOutClientAction"},
			{Key: "DisabledDuringExecution", Value: true},
		}
	}
	// DisabledDuringExecution true is on every Forms$NoAction Studio Pro stores
	// on a menu item (3 of 3 in ako/TestApp's navigation, 3 of 3 in
	// testapp-views) and is what the widget serializer writes; leaving it out
	// made a describe -> exec of any item without an action a rewrite.
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$NoAction"},
		{Key: "DisabledDuringExecution", Value: true},
	}
}

// navOfflineConfigs rebuilds the OfflineEntityConfigs list from the spec while
// carrying forward the properties MDL cannot express.
//
// The carry is the whole point. Studio Pro writes four properties on a web
// profile's config and MDL can spell three; CompatibilityMode is read (phase 1)
// and put back here, keyed by entity. Building the element from the spec alone
// would clear it on every rewrite — silently, because the document stays valid
// and `mx check` reports 0 errors either way. That is the defect that had
// `create or modify entity` deleting access rules.
//
// DownloadMode and ShouldDownload are NOT written, though modelsdk/gen declares
// them: they occur zero times in ako/TestApp's configs. A property absent from
// every real document is one Studio Pro fills in on load, and emitting it is
// how a document mxbuild accepts becomes one Studio Pro cannot open.
func navOfflineConfigs(stored bson.A, specs []types.NavOfflineEntitySpec) bson.A {
	// Index what is stored by entity so a rewrite of the same entity keeps its
	// unauthorable properties. An entity the spec adds has no stored config and
	// takes the default every reference config carries.
	compat := map[string]bool{}
	constraint := map[string]string{}
	for _, item := range stored {
		cfg, ok := item.(bson.D)
		if !ok {
			continue // the leading typed-array marker
		}
		if e := navGetString(cfg, "Entity"); e != "" {
			compat[e] = navGetBool(cfg, "CompatibilityMode")
			constraint[e] = navGetString(cfg, "Constraint")
		}
	}

	out := bson.A{navMarkerItems}
	for _, s := range specs {
		// Studio Pro lays a constraint out over several lines; describe folds it
		// onto one, and the rewrite stored the folded text — a change to every
		// constrained entity on each describe -> exec. The same constraint
		// written with other whitespace keeps the stored layout.
		if st, ok := constraint[s.Entity]; ok && st != s.Constraint &&
			xpathWithoutLayout(st) == xpathWithoutLayout(s.Constraint) {
			s.Constraint = st
		}
		out = append(out, bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$OfflineEntityConfig"},
			{Key: "CompatibilityMode", Value: compat[s.Entity]},
			{Key: "Constraint", Value: s.Constraint},
			{Key: "Entity", Value: s.Entity},
			{Key: "SyncMode", Value: s.SyncMode},
		})
	}
	return out
}

// navGetBool reads a bool field, defaulting to false for an absent or
// wrong-typed value — which is what every reference config carries.
func navGetBool(doc bson.D, key string) bool {
	for _, e := range doc {
		if e.Key == key {
			if b, ok := e.Value.(bool); ok {
				return b
			}
		}
	}
	return false
}

// xpathWithoutLayout is an XPath constraint with the whitespace outside its
// string literals removed, so two layouts of the same constraint compare equal.
// A doubled quote inside a literal toggles twice and stays inside it.
func xpathWithoutLayout(x string) string {
	var b strings.Builder
	inLiteral := false
	for _, r := range x {
		switch {
		case r == '\'':
			inLiteral = !inLiteral
			b.WriteRune(r)
		case !inLiteral && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
