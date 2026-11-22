// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// mendixlabs/mxcli#1522: an omitted clause in `create or modify navigation`
// keeps the stored value, as the menu block, the sync block, `on sync error`
// and `progressive web app` already did. The writer overwrote the home page,
// the role-based home pages, the login page and the fallback page on every
// statement, so `create or modify navigation Responsive { menu item … }` left
// a profile with no home page: CE0527 at the next build.
func TestUpdateNavigationProfile_OmittedClausesKeepStored(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	nav, err := b.GetNavigation()
	if err != nil || nav == nil {
		t.Fatalf("GetNavigation: %v", err)
	}
	prof := ""
	for _, p := range nav.Profiles {
		if !p.IsNative {
			prof = p.Name
			break
		}
	}
	if prof == "" {
		t.Fatal("fixture has no web profile")
	}

	full := types.NavigationProfileSpec{
		HomePages: []types.NavHomePageSpec{
			{IsPage: true, Target: "MyFirstModule.Home"},
			{IsPage: true, Target: "MyFirstModule.AdminHome", ForRole: "Administrator"},
			{IsPage: false, Target: "MyFirstModule.UserHome", ForRole: "User"},
		},
		LoginPage:    "MyFirstModule.Login",
		NotFoundPage: "MyFirstModule.NotFound",
	}
	if err := b.UpdateNavigationProfile(nav.ID, prof, full); err != nil {
		t.Fatalf("first update: %v", err)
	}
	// The issue's second statement: a menu and nothing else.
	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{
		HasMenu:   true,
		MenuItems: []types.NavMenuItemSpec{{Caption: "B", Page: "MyFirstModule.B"}},
	}); err != nil {
		t.Fatalf("menu-only update: %v", err)
	}

	p := reloadProfile(t, proj, prof)
	if p.HomePage == nil || p.HomePage.Page != "MyFirstModule.Home" {
		t.Errorf("home page not kept: %+v", p.HomePage)
	}
	if p.LoginPage != "MyFirstModule.Login" {
		t.Errorf("login page not kept: %q", p.LoginPage)
	}
	if p.NotFoundPage != "MyFirstModule.NotFound" {
		t.Errorf("not-found page not kept: %q", p.NotFoundPage)
	}
	if len(p.RoleBasedHomePages) != 2 {
		t.Fatalf("role-based home pages = %d, want 2 kept", len(p.RoleBasedHomePages))
	}
	if len(p.MenuItems) != 1 || p.MenuItems[0].Caption != "B" {
		t.Errorf("menu not written: %+v", p.MenuItems)
	}
}

func reloadProfile(t *testing.T, proj, name string) *types.NavigationProfile {
	t.Helper()
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	nav, err := b.GetNavigation()
	if err != nil {
		t.Fatalf("GetNavigation: %v", err)
	}
	for _, p := range nav.Profiles {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("profile %q gone", name)
	return nil
}

// A stated clause still writes: the default home page and one role's home page
// replace what is stored for them, and a role the statement does not mention
// keeps its home page.
func TestNavPatchWebProfile_StatedClausesReplace(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "HomePage", Value: navHomePageBson(true, "M.OldHome", "")},
		{Key: "HomeItems", Value: bson.A{navMarkerHomeItems,
			navHomePageBson(true, "M.AdminOld", "Administrator"),
			navHomePageBson(true, "M.UserHome", "User"),
		}},
		{Key: "LoginPageSettings", Value: navFormSettingsBson("M.OldLogin")},
		{Key: "NotFoundHomepage", Value: nil},
	}
	out, err := navPatchWebProfile(stored, types.NavigationProfileSpec{
		HomePages: []types.NavHomePageSpec{
			{IsPage: true, Target: "M.NewHome"},
			{IsPage: true, Target: "M.AdminNew", ForRole: "administrator"},
			{IsPage: true, Target: "M.Guest", ForRole: "Guest"},
		},
		LoginPage:    "M.NewLogin",
		NotFoundPage: "M.Missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := navGetString(navGetDoc(out, "HomePage"), "Page"); got != "M.NewHome" {
		t.Errorf("home page = %q", got)
	}
	if got := navGetString(navGetDoc(out, "LoginPageSettings"), "Form"); got != "M.NewLogin" {
		t.Errorf("login page = %q", got)
	}
	if got := navGetString(navGetDoc(out, "NotFoundHomepage"), "Page"); got != "M.Missing" {
		t.Errorf("not-found page = %q", got)
	}
	items := navGetArray(out, "HomeItems")
	if len(items) != 4 || items[0] != navMarkerHomeItems {
		t.Fatalf("HomeItems = %v, want marker + 3", items)
	}
	byRole := map[string]string{}
	for _, it := range items[1:] {
		d := it.(bson.D)
		byRole[navGetString(d, "UserRole")] = navGetString(d, "Page")
	}
	if byRole["Administrator"] != "" || byRole["administrator"] != "M.AdminNew" {
		t.Errorf("stated role not replaced in place of the stored one: %v", byRole)
	}
	if byRole["User"] != "M.UserHome" {
		t.Errorf("unmentioned role's home page not kept: %v", byRole)
	}
	if byRole["Guest"] != "M.Guest" {
		t.Errorf("new role's home page not added: %v", byRole)
	}
}

// The native writer has the same shape: it kept an omitted default home page
// but rebuilt RoleBasedNativeHomePages from the statement alone.
func TestNavPatchNativeProfile_OmittedRoleHomesKeepStored(t *testing.T) {
	roleHome := bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Navigation$RoleBasedNativeHomePage"},
		{Key: "UserRole", Value: "User"},
		{Key: "HomePagePage", Value: "M.UserHome"},
		{Key: "HomePageNanoflow", Value: ""},
	}
	doc := bson.D{
		{Key: "$Type", Value: "Navigation$NativeNavigationProfile"},
		{Key: "Name", Value: "NativePhone"},
		{Key: "RoleBasedNativeHomePages", Value: bson.A{navMarkerHomeItems, roleHome}},
	}
	out, err := navPatchNativeProfile(doc, types.NavigationProfileSpec{
		HomePages: []types.NavHomePageSpec{{IsPage: true, Target: "M.Home"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := navGetArray(out, "RoleBasedNativeHomePages")
	if len(items) != 2 || navGetString(items[1].(bson.D), "HomePagePage") != "M.UserHome" {
		t.Errorf("role-based native home page not kept: %v", items)
	}
}
