// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §7: mxcli could not create a navigation profile, so a
// device-specific front end could not be built from MDL at all — Mendix routes a
// phone to a Phone PROFILE on User-Agent, not on viewport width.
//
// The shape below is pinned against Studio Pro 11.14 over PED, measured by adding
// a profile through Studio Pro's OWN model API and reading back what it filled
// in — not by copying a profile the reference project already had configured.
// That distinction cost a bug: the reference Phone profile carries PWA settings,
// but a profile Studio Pro has just created does not.
package modelsdkbackend

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func profileKey(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

func TestNewWebProfileMatchesTheStudioProKeySet(t *testing.T) {
	// Every profile in the reference document stores the same fourteen keys,
	// whatever its kind — an offline profile included. A key mxcli invents, or one
	// it omits, is the failure that passes mx check and then will not open in
	// Studio Pro.
	want := []string{
		"$ID", "$Type", "AppIcon", "AppTitle", "HomeItems", "HomePage", "Kind",
		"LoginPageSettings", "Menu", "Name", "NotFoundHomepage",
		"OfflineEntityConfigs", "ProgressiveWebAppSettings", "ThrowPartialSyncError",
	}
	for _, kind := range types.WebProfileKindNames() {
		got := newWebProfileBson(kind)
		if len(got) != len(want) {
			t.Errorf("%s: %d keys, want %d", kind, len(got), len(want))
		}
		for _, k := range want {
			if _, ok := profileKey(got, k); !ok {
				t.Errorf("%s: missing key %q", kind, k)
			}
		}
		if v, _ := profileKey(got, "Kind"); v != kind {
			t.Errorf("%s: Kind = %v", kind, v)
		}
		if v, _ := profileKey(got, "Name"); v != kind {
			t.Errorf("%s: Name = %v", kind, v)
		}
	}
}

func TestOfflineProfileIsStructurallyIdenticalToItsOnlineTwin(t *testing.T) {
	// Measured over PED: a freshly created PhoneOffline differs from a Phone in
	// Kind and Name and in nothing else. If that ever stops being true the offline
	// kinds need their own builder, and this test is what says so.
	for _, tc := range []struct{ online, offline string }{
		{"Responsive", "ResponsiveOffline"},
		{"Phone", "PhoneOffline"},
		{"Tablet", "TabletOffline"},
	} {
		on, off := newWebProfileBson(tc.online), newWebProfileBson(tc.offline)
		if len(on) != len(off) {
			t.Fatalf("%s/%s: key counts differ (%d vs %d)", tc.online, tc.offline, len(on), len(off))
		}
		for i := range on {
			if on[i].Key != off[i].Key {
				t.Errorf("key %d: %s has %q, %s has %q", i, tc.online, on[i].Key, tc.offline, off[i].Key)
				continue
			}
			switch on[i].Key {
			case "$ID", "Kind", "Name":
				continue // $ID is minted per call; Kind and Name are the difference.
			}
			if fmt.Sprintf("%v", on[i].Value) != fmt.Sprintf("%v", off[i].Value) {
				// $ID appears inside sub-elements too, so compare only the keys we
				// can compare — a sub-document differing solely by minted IDs is
				// expected and not a finding.
				if _, isDoc := on[i].Value.(bson.D); isDoc {
					continue
				}
				t.Errorf("%s differs: %s=%v, %s=%v", on[i].Key, tc.online, on[i].Value, tc.offline, off[i].Value)
			}
		}
	}
}

func TestNoKindCarriesProgressiveWebAppSettings(t *testing.T) {
	// The reference project's Phone and TabletOffline profiles BOTH store
	// {Precaching: true, InstallPrompt: true}, and copying that was the bug. The
	// schema defaults Precaching to false and makes the whole element optional, and
	// a profile added through Studio Pro's own API materialises null — so those
	// values are the user's, not the platform's. Writing them would turn precaching
	// on for every profile mxcli creates.
	for _, kind := range types.WebProfileKindNames() {
		v, ok := profileKey(newWebProfileBson(kind), "ProgressiveWebAppSettings")
		if !ok {
			t.Errorf("%s: no ProgressiveWebAppSettings key at all", kind)
			continue
		}
		if v != nil {
			t.Errorf("%s: ProgressiveWebAppSettings = %#v, want an explicit null", kind, v)
		}
	}
}

func TestResolveProfileKind(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wantKind   string
		wantNative bool
		wantOK     bool
	}{
		{"phone", "Phone", false, true},
		{" Responsive ", "Responsive", false, true},
		{"TabletOffline", "TabletOffline", false, true},
		{"nativephone", "NativePhone", true, true},
		{" NativePhone ", "NativePhone", true, true},
		{"Mobile", "", false, false},
		{"Hybrid", "", false, false},
	} {
		kind, isNative, ok := resolveProfileKind(tc.name)
		if ok != tc.wantOK || kind != tc.wantKind || isNative != tc.wantNative {
			t.Errorf("resolveProfileKind(%q) = (%q, %v, %v), want (%q, %v, %v)",
				tc.name, kind, isNative, ok, tc.wantKind, tc.wantNative, tc.wantOK)
		}
	}
}

func TestNewNativeProfileMatchesTheFixtureKeySet(t *testing.T) {
	// Every key native-pwa-migrator's fixtures/native-phone-minimal carries on its
	// real, Studio-Pro-authored NativePhone profile, and nothing it does not —
	// same discipline as TestNewWebProfileMatchesTheStudioProKeySet, except this
	// reference is a configured document (see newNativeProfileBson's doc comment
	// for why no fresher one exists). A key mxcli invents, or one it omits, is
	// the failure that passes mx check and then will not open in Studio Pro.
	want := []string{
		"$ID", "$Type", "ApplyScreenTransition", "BottomBarItems",
		"EncryptionDbEnabled", "LocalFileEncryptionEnabled", "LoggingEnabled",
		"Name", "NativeHomePage", "NativeNavigationEnabled", "NetworkTimeoutMs",
		"OfflineEntityConfigs", "OtaEnabled", "PopupNavigationTransition",
		"RoleBasedNativeHomePages", "ScreenNavigationTransition",
		"SessionCookieEncryptionEnabled", "ThrowPartialSyncError",
	}
	got := newNativeProfileBson("NativePhone")
	if len(got) != len(want) {
		t.Errorf("NativePhone: %d keys, want %d", len(got), len(want))
	}
	for _, k := range want {
		if _, ok := profileKey(got, k); !ok {
			t.Errorf("NativePhone: missing key %q", k)
		}
	}
	if v, _ := profileKey(got, "Name"); v != "NativePhone" {
		t.Errorf("NativePhone: Name = %v", v)
	}
	if v, _ := profileKey(got, "$Type"); v != "Navigation$NativeNavigationProfile" {
		t.Errorf("NativePhone: $Type = %v", v)
	}
	// A native profile has no Kind property at all — confirmed against
	// modelsdk/gen/navigation/types.go, which gives NativeNavigationProfile no
	// `kind` field. Unlike the web builder, Kind must never appear here.
	if _, ok := profileKey(got, "Kind"); ok {
		t.Error("NativePhone: has a Kind key, but native profiles have none")
	}
}

func TestNativeProfileStartsWithNoHomePage(t *testing.T) {
	// UpdateNavigationProfile's navPatchNativeProfile only overwrites
	// NativeHomePage when the CREATE statement gives one — so a bare
	// `create navigation NativePhone;` leaves this constructor's value in place.
	// It must be an explicit null, not the fixture's configured home page.
	v, ok := profileKey(newNativeProfileBson("NativePhone"), "NativeHomePage")
	if !ok {
		t.Fatal("NativePhone: no NativeHomePage key at all")
	}
	if v != nil {
		t.Errorf("NativePhone: NativeHomePage = %#v, want an explicit null", v)
	}
}

func TestNativeProfileStartsWithEmptyChildCollections(t *testing.T) {
	// OfflineEntityConfigs and RoleBasedNativeHomePages are stored EMPTY on the
	// fixture's own (otherwise configured) document — direct evidence of the
	// unconfigured state. BottomBarItems is only rewritten by
	// UpdateNavigationProfile when the CREATE statement has a MENU block (see
	// navPatchNativeProfile), so a freshly created profile must start empty here
	// rather than carrying the fixture's configured bottom bar item forward.
	for _, tc := range []struct {
		key    string
		marker int32
	}{
		{"OfflineEntityConfigs", 3},
		{"RoleBasedNativeHomePages", 1},
		{"BottomBarItems", 2},
	} {
		v, ok := profileKey(newNativeProfileBson("NativePhone"), tc.key)
		if !ok {
			t.Fatalf("NativePhone: no %s key at all", tc.key)
		}
		arr, ok := v.(bson.A)
		if !ok {
			t.Fatalf("NativePhone: %s is %T, want bson.A", tc.key, v)
		}
		if len(arr) != 1 {
			t.Errorf("NativePhone: %s has %d entries beyond the marker, want 0", tc.key, len(arr)-1)
		}
		if len(arr) == 1 && arr[0] != tc.marker {
			t.Errorf("NativePhone: %s marker = %v, want %d", tc.key, arr[0], tc.marker)
		}
	}
}

func TestNativeProfileHasNoHermesEnabledKey(t *testing.T) {
	// modelsdk/gen exposes a HermesEnabled setter, but the fixture's real
	// document does not carry this key — writing a value here would be exactly
	// the "assembled from a guess" failure mode this feature exists to avoid.
	if _, ok := profileKey(newNativeProfileBson("NativePhone"), "HermesEnabled"); ok {
		t.Error("NativePhone: has a HermesEnabled key, but the reference document does not")
	}
}

func TestOfflineProfilesStartWithNoEntityConfigs(t *testing.T) {
	// Studio Pro DERIVES the offline entity list from the pages the profile
	// reaches; only rows that deviate from the defaults are stored. An empty
	// collection is therefore the correct starting state, not an incomplete one —
	// measured on a freshly created PhoneOffline, whose offlineEntityConfigs is [].
	for _, kind := range []string{"ResponsiveOffline", "PhoneOffline", "TabletOffline"} {
		v, _ := profileKey(newWebProfileBson(kind), "OfflineEntityConfigs")
		arr, ok := v.(bson.A)
		if !ok {
			t.Fatalf("%s: OfflineEntityConfigs is %T, want bson.A", kind, v)
		}
		// Just the typed-array marker, no entries.
		if len(arr) != 1 {
			t.Errorf("%s: OfflineEntityConfigs has %d entries beyond the marker, want 0", kind, len(arr)-1)
		}
	}
}
