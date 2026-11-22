// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// AddNavigationProfile appends a new web navigation profile to the navigation
// document. It is the half `CREATE OR REPLACE NAVIGATION` was missing: the
// statement could only ever replace a profile that already existed, so a
// device-specific front end — the whole reason Phone and Tablet profiles exist —
// could not be built from MDL at all (ako/mxcli-maintenance §7).
//
// The shape is pinned against Studio Pro 11.14 over PED, and the measurement that
// matters was taken by adding a profile through Studio Pro's OWN model API and
// reading back what it materialised, rather than by copying a profile the user
// had already configured:
//
//	                            a profile Studio Pro has just created
//	ProgressiveWebAppSettings   null
//	OfflineEntityConfigs        []
//	ThrowPartialSyncError       true
//	AppIcon                     ""
//
// That distinction is the whole point. The reference project's Phone and
// TabletOffline profiles both carry {Precaching: true, InstallPrompt: true} — but
// the schema's default for Precaching is FALSE and ProgressiveWebAppSettings is
// optional, so those values are the user's choices, not Studio Pro's. Copying
// them would have shipped every Phone profile with precaching silently on.
//
// Every kind stores the same fourteen keys; only Kind and Name differ. An offline
// profile is not a different document — what changes is what Mendix then demands
// of the pages it reaches (CE6206), which is why creating one runs MDL-OFFLINE01
// over the project instead of just writing the profile.
func (b *Backend) AddNavigationProfile(navDocID model.ID, name string) error {
	if b.writer == nil {
		return fmt.Errorf("AddNavigationProfile: not connected for writing")
	}
	kind, isNative, ok := resolveProfileKind(name)
	if !ok {
		return fmt.Errorf("cannot create navigation profile %q: Mendix's web profiles are %s, and its native profiles are %s",
			name, strings.Join(types.WebProfileKindNames(), ", "), strings.Join(types.NativeProfileKindNames(), ", "))
	}

	raw, err := b.reader.GetRawUnitBytes(string(navDocID))
	if err != nil {
		return fmt.Errorf("AddNavigationProfile: load unit: %w", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("AddNavigationProfile: unmarshal: %w", err)
	}

	profiles := navGetArray(doc, "Profiles")
	if profiles == nil {
		return fmt.Errorf("no Profiles array found in navigation document")
	}
	for _, item := range profiles {
		p, ok := item.(bson.D)
		if !ok {
			continue // the leading typed-array marker
		}
		if strings.EqualFold(navGetString(p, "Name"), kind) {
			return fmt.Errorf("navigation profile %s already exists", kind)
		}
	}

	var profile bson.D
	if isNative {
		profile = newNativeProfileBson(kind)
	} else {
		profile = newWebProfileBson(kind)
	}
	if !b.declaresThrowPartialSyncError() {
		profile = withoutKey(profile, "ThrowPartialSyncError")
	}
	doc = navSetField(doc, "Profiles", append(profiles, profile))
	out, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("AddNavigationProfile: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(navDocID), out)
}

// declaresThrowPartialSyncError reports whether the project's metamodel has
// NavigationProfile.ThrowPartialSyncError — from 11.12.0 (measured: the profile
// `mx create-project` writes carries it on 11.12.0 and not on 11.11.0). On an
// older project the key is one Studio Pro cannot open the document with
// (mendixlabs/mxcli#1373), and the storage layer refuses it, since a new profile
// sets it true. An unreadable version writes it, as before.
func (b *Backend) declaresThrowPartialSyncError() bool {
	pv := b.ProjectVersion()
	return pv == nil || pv.MajorVersion == 0 || pv.IsAtLeast(11, 12)
}

// withoutKey returns d without its top-level key.
func withoutKey(d bson.D, key string) bson.D {
	out := make(bson.D, 0, len(d))
	for _, e := range d {
		if e.Key != key {
			out = append(out, e)
		}
	}
	return out
}

// resolveProfileKind maps a user-written profile name to its canonical Kind (web)
// or Name (native) and reports which family it belongs to. The two lookups are
// kept apart rather than merged into one map: a web profile is identified by its
// Kind property, a native profile has no Kind at all and is identified purely by
// $Type + Name (see the doc comment on newNativeProfileBson) — blurring the two
// would make it easy to build a profile with the wrong shape for its own $Type.
func resolveProfileKind(name string) (kind string, isNative bool, ok bool) {
	if k, ok := types.CanonicalProfileKind(name); ok {
		return k, false, true
	}
	if k, ok := types.CanonicalNativeProfileKind(name); ok {
		return k, true, true
	}
	return "", false, false
}

// newWebProfileBson builds an empty profile of the given kind, with every key a
// Studio Pro-authored one carries and nothing it does not.
//
// The empty sub-elements are written rather than omitted because that is what
// the reference document has: a profile Studio Pro has just added already holds
// an empty HomePage, AppTitle, LoginPageSettings and MenuItemCollection. The
// caller then fills them through the ordinary UpdateNavigationProfile path, so
// this function deliberately knows nothing about menus or home pages.
func newWebProfileBson(kind string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "AppIcon", Value: ""},
		{Key: "AppTitle", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		// 2, not the 3 the PED session this profile shape was pinned against
		// recorded: ako/TestApp's Studio Pro-authored profile carries
		// HomeItems [marker 2] with two RoleBasedHomePages in it. See
		// navMarkerHomeItems.
		{Key: "HomeItems", Value: bson.A{navMarkerHomeItems}},
		{Key: "HomePage", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: ""},
		}},
		{Key: "Kind", Value: kind},
		{Key: "LoginPageSettings", Value: navFormSettingsBson("")},
		{Key: "Menu", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			// Menus$MenuItemCollection.Items is 3 in every Studio Pro document
			// (153 occurrences); this was the one list here still on 1.
			{Key: "Items", Value: bson.A{navMarkerItems}},
		}},
		{Key: "Name", Value: kind},
		{Key: "NotFoundHomepage", Value: nil},
		{Key: "OfflineEntityConfigs", Value: bson.A{int32(3)}},
		// Optional in the schema, and null on every profile Studio Pro has just
		// created — including the offline kinds, which is the opposite of what the
		// reference project suggested. Turning a PWA on is a user decision.
		{Key: "ProgressiveWebAppSettings", Value: nil},
		{Key: "ThrowPartialSyncError", Value: true},
	}
}

// newNativeProfileBson builds an empty NativePhone profile.
//
// Unlike the web kinds, this shape is NOT pinned by adding a profile through
// Studio Pro's own model API and reading back what it materialised — that
// channel does not exist for native profiles at all, on any machine: PED/MCP
// explicitly rejects native profile authoring (docs/03-development/
// PED_MCP_CAPABILITIES.md, mdl/backend/mcp/navigation.go), independent of this
// repo's own environment. The best available evidence instead is a real
// Studio-Pro-authored NativePhone profile from native-pwa-migrator's
// fixtures/native-phone-minimal (a raw BSON dump of the .mxunit file, not a PED
// read) — but that document is a CONFIGURED profile, not a freshly-created one,
// so it proves the key set, $Type names, and storage keys, not which values are
// Studio Pro's create-time defaults versus this fixture author's choices.
//
// A native profile has no Kind property at all (confirmed against
// modelsdk/gen/navigation/types.go: NativeNavigationProfile carries no `kind`
// field, unlike NavigationProfile) — it is distinguished purely by $Type + Name.
//
// Per-field provenance, so a future re-pin knows what to re-check:
//   - Name, $Type: definitional.
//   - OfflineEntityConfigs, RoleBasedNativeHomePages: the fixture's own document
//     carries these EMPTY (marker 3 / marker 1 respectively) even though it is
//     otherwise configured — direct evidence of the unconfigured state, not an
//     inference.
//   - NativeHomePage: written explicit nil here, NOT the fixture's configured
//     home page — this is exactly what UpdateNavigationProfile fills in
//     afterwards (mirrors NotFoundHomepage/ProgressiveWebAppSettings on the web
//     side: an unset single child is an explicit null, never an empty
//     placeholder sub-document). Backed by a real signal in the decoder, not a
//     guess: NativeNavigationProfile.InitFromRaw only sets nativeHomePage
//     `if child, err := codec.DecodeChild(raw, "NativeHomePage"); err == nil`,
//     i.e. the shipped read path already treats a missing/null NativeHomePage as
//     an ordinary, non-error state.
//   - BottomBarItems: empty, marker 2 — the fixture's own (non-empty) BottomBarItems
//     uses marker 2, and the marker is per-field-type rather than count-dependent
//     (proven by newWebProfileBson itself: HomeItems/AppTitle.Items/
//     OfflineEntityConfigs are all empty in that SAME reference document yet use
//     three different markers — 3, 3, 3 vs Menu.Items' 1 — so an empty
//     collection keeps its type's marker rather than switching to some
//     "generic empty" one). UpdateNavigationProfile does not patch BottomBarItems
//     for native profiles (a separate, already-tracked gap), so this value
//     persists until Studio Pro or a future writer sets it.
//   - ApplyScreenTransition, EncryptionDbEnabled, LocalFileEncryptionEnabled,
//     LoggingEnabled, NativeNavigationEnabled, OtaEnabled,
//     SessionCookieEncryptionEnabled: false, and ScreenNavigationTransition/
//     PopupNavigationTransition: "SystemDefault" — carried from the fixture.
//     These are opt-in toggles a minimal fixture is unlikely to have touched,
//     but that is inference, not measurement.
//   - NetworkTimeoutMs (60000) and ThrowPartialSyncError (false): carried from
//     the fixture as observed values. Flagged as the least-certain fields here —
//     unlike the booleans above, there is no structural reason to expect these
//     specific values are Studio Pro's create-time defaults rather than this
//     fixture author's settings that happen not to have been touched.
//   - HermesEnabled: deliberately OMITTED. modelsdk/gen exposes a setter for it,
//     but the fixture's real document does not carry this key at all — writing
//     a value here would be exactly the "assembled from a guess" failure mode
//     this whole feature exists to avoid.
func newNativeProfileBson(kind string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Navigation$NativeNavigationProfile"},
		{Key: "ApplyScreenTransition", Value: false},
		{Key: "BottomBarItems", Value: bson.A{int32(2)}},
		{Key: "EncryptionDbEnabled", Value: false},
		{Key: "LocalFileEncryptionEnabled", Value: false},
		{Key: "LoggingEnabled", Value: false},
		{Key: "Name", Value: kind},
		{Key: "NativeHomePage", Value: nil},
		{Key: "NativeNavigationEnabled", Value: false},
		{Key: "NetworkTimeoutMs", Value: int32(60000)},
		{Key: "OfflineEntityConfigs", Value: bson.A{int32(3)}},
		{Key: "OtaEnabled", Value: false},
		{Key: "PopupNavigationTransition", Value: "SystemDefault"},
		{Key: "RoleBasedNativeHomePages", Value: bson.A{int32(1)}},
		{Key: "ScreenNavigationTransition", Value: "SystemDefault"},
		{Key: "SessionCookieEncryptionEnabled", Value: false},
		{Key: "ThrowPartialSyncError", Value: false},
	}
}
