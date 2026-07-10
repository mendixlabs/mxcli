// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// TestUpdateNavigationProfile_RoundTrip patches the login page of an existing
// navigation profile and confirms the change persists (the rest of the document
// is preserved by the in-place patch).
func TestUpdateNavigationProfile_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	nav, err := b.GetNavigation()
	if err != nil || nav == nil || len(nav.Profiles) == 0 {
		t.Skipf("no navigation profiles in fixture: %v", err)
	}
	prof := nav.Profiles[0].Name

	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{
		HomePages: []types.NavHomePageSpec{{IsPage: true, Target: "MyFirstModule.Page"}},
	}); err != nil {
		t.Fatalf("UpdateNavigationProfile: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	nav2, err := b2.GetNavigation()
	if err != nil {
		t.Fatalf("GetNavigation(2): %v", err)
	}
	var p *types.NavigationProfile
	for _, x := range nav2.Profiles {
		if x.Name == prof {
			p = x
		}
	}
	if p == nil {
		t.Fatalf("profile %q gone after update", prof)
	}
	if p.HomePage == nil || p.HomePage.Page != "MyFirstModule.Page" {
		t.Errorf("home page not round-tripped: %+v", p.HomePage)
	}
}
